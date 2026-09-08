package acceptance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

type phase17Fixture struct {
	f       *engineeringFixture
	e       identity.Envelope
	pack    semantics.TopicPack
	service *nlqroute.Service
	model   *gatewayFixture
	context string
}

func newPhase17Fixture(t *testing.T) *phase17Fixture {
	t.Helper()
	f, draftsService, topicsService, model, pack := publicationFixture(t)
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	draft, err := draftsService.Save(context.Background(), e, drafts.SaveRequest{Pack: pack, Change: "Phase 17 routing fixture"})
	if err != nil {
		t.Fatal("save topic", err)
	}
	review, err := topicsService.Review(context.Background(), e, pack.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Phase 17 routing fixture"})
	if err != nil {
		t.Fatal("review topic", err)
	}
	if _, err = topicsService.Publish(context.Background(), e, pack.Topic, topics.PublishRequest{Review: review.ID}); err != nil {
		t.Fatal("publish topic", err)
	}
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal("index", err)
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal("rules", err)
	}
	route, err := nlqroute.New(topicsService, rules, index, model.engine)
	if err != nil {
		t.Fatal("route", err)
	}
	return &phase17Fixture{f: f, e: e, pack: pack, service: route, model: model, context: pack.Datasets[0].Source.Context}
}

func TestPhase17(t *testing.T) {
	fixture := newPhase17Fixture(t)
	ctx := context.Background()
	question := nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure", "entity"}, LimitPerKind: 2}

	t.Run("AC01", func(t *testing.T) {
		before := fixture.model.requests.Load()
		out, err := fixture.service.Route(ctx, fixture.e, question)
		if err != nil || out.Context == nil || len(out.TopicVersions) != 1 || out.TopicVersions[0] != fixture.pack.Version {
			t.Fatalf("current route admission failed: out=%#v err=%v", out, err)
		}
		if fixture.model.requests.Load() <= before || len(out.RemoteCalls) == 0 {
			t.Fatal("real Bifrost embedding was not attributed")
		}
		narrow := fixture.f.token.envelope(t, fixture.e.Tenant(), fixture.e.User(), "topics.read", "cw.topic.read:"+fixture.pack.Topic, "cw.execution_context.use:"+fixture.context)
		before = fixture.model.requests.Load()
		if _, err = fixture.service.Route(ctx, narrow, question); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("missing dependency reach was not denied: %v", err)
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("denied dependency reached Bifrost")
		}
	})

	t.Run("AC02", func(t *testing.T) {
		before := fixture.model.requests.Load()
		out, err := fixture.service.Route(ctx, fixture.e, nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure", "entity"}, LimitPerKind: 1, Rerank: true})
		if err != nil || out.Context == nil {
			t.Fatalf("rerank route failed: out=%#v err=%v", out, err)
		}
		if fixture.model.requests.Load() < before+2 {
			t.Fatal("embedding and rerank were not both observed")
		}
		fixture.model.mode.Store("duplicate")
		_, err = fixture.service.Route(ctx, fixture.e, nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure", "entity"}, LimitPerKind: 1, Rerank: true})
		fixture.model.mode.Store("normal")
		if !errors.Is(err, gateway.ErrOutput) && !errors.Is(err, gateway.ErrUnavailable) {
			t.Fatalf("malformed rerank was not rejected: %v", err)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		examples := make([]nlq.OptionalItem, nlq.MaxExamples)
		for i := range examples {
			examples[i] = nlq.OptionalItem{ID: "example-" + string(rune('a'+i)), Text: "Ejemplo de ingresos", Priority: i}
		}
		out, err := fixture.service.Route(ctx, fixture.e, nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageSpanish, Question: "¿Qué ingresos hay?", Kinds: []string{"measure"}, LimitPerKind: 1, MetricIDs: []string{"revenue"}, Examples: examples})
		if err != nil || out.Context == nil || out.Context.Locale != nlq.LanguageSpanish || len(out.Context.Metrics) != 1 {
			t.Fatalf("detached Spanish context or pinned metric missing: out=%#v err=%v", out, err)
		}
		if out.Tier.Budget() != nlq.LowBudget && out.Tier.Budget() != nlq.MediumBudget && out.Tier.Budget() != nlq.HighBudget {
			t.Fatalf("unsupported context tier: %s", out.Tier)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		assembler, err := nlq.NewDefaultContextAssembler()
		if err != nil {
			t.Fatal(err)
		}
		_, err = assembler.Assemble(ctx, nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Question: "question", Constraints: &nlq.ConstraintState{Allowed: true, Required: []nlq.MandatoryConstraint{{ID: "required", Kind: "required", Text: strings.Repeat("required ", 7000)}}}}, nlq.TierLow)
		if !errors.Is(err, nlq.ErrInsufficient) {
			t.Fatalf("mandatory budget did not produce typed insufficiency: %v", err)
		}
		out, err := fixture.service.Route(ctx, fixture.e, nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 1, MetricIDs: []string{"revenue"}})
		if err != nil || out.Context == nil || len(out.Context.Metrics) != 1 {
			t.Fatalf("pinned metric route failed: out=%#v err=%v", out, err)
		}
	})

	t.Run("AC05", func(t *testing.T) {
		before := fixture.model.requests.Load()
		duplicate := question
		duplicate.Topic = ""
		duplicate.Topics = []string{fixture.pack.Topic, fixture.pack.Topic}
		if _, err := fixture.service.Route(ctx, fixture.e, duplicate); !errors.Is(err, nlqroute.ErrInvalid) {
			t.Fatalf("duplicate multi-topic selection was admitted: %v", err)
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("invalid multi-topic selection reached Bifrost")
		}
	})

	t.Run("AC06", func(t *testing.T) {
		registry, err := nlqapi.Registry()
		if err != nil {
			t.Fatal(err)
		}
		handler := assertRegisteredWireSchemas(t, registry, nlqapi.Handler(fixture.f.token.verifier, fixture.service, http.NotFoundHandler()))
		server := httptest.NewServer(handler)
		defer server.Close()
		client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
			return fixture.f.token.sign(t, fixture.f.token.claims(fixture.e.Tenant(), fixture.e.User(), fixture.e.Scopes()), nil), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if out, callErr := client.RouteNLQ(ctx, question); callErr != nil || out.Context == nil {
			t.Fatalf("HTTP/SDK route failed: out=%#v err=%v", out, callErr)
		}
		english := question
		spanish := question
		spanish.Locale = nlq.LanguageSpanish
		spanish.Question = "¿Qué ingresos hay?"
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, request := range []nlqroute.RouteRequest{english, spanish} {
			request := request
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := fixture.service.Route(ctx, fixture.e, request)
				if err != nil || out.Context == nil || len(out.Stages) < 3 {
					errs <- errors.New("concurrent route did not return stage attribution")
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
	})
}
