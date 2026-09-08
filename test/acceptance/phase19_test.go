package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func phase19Scopes() []string {
	return []string{"query.context", "query.submit", "topics.read", "sources.read", "sources.query", "cw.topic.read:*", "cw.source.read:*", "cw.source.query:*", "cw.dataset.query:*", "cw.execution_context.use:*"}
}

func phase19Authority(t *testing.T, f *phase17Fixture, tenant, user, session string, scopes []string) (identity.Envelope, string) {
	t.Helper()
	claims := f.model.token.claims(tenant, user, scopes)
	claims["session"] = session
	token := f.model.token.sign(t, claims, nil)
	e, err := f.model.token.verifier.Verify(context.Background(), token, auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	return e, token
}

func phase19Service(t *testing.T, f *phase17Fixture, limits config.QueryBundles, now func() time.Time, router nlqbyo.Router, executor nlqbyo.Executor) (*nlqbyo.Service, *topics.Service, *rulesets.Service) {
	t.Helper()
	index, err := vindex.New(f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	published, err := topics.New(f.f.db, f.f.s, index, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.f.db, f.f.db, f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	if executor == nil {
		executor = f.f.executor
	}
	s, err := nlqbyo.New(router, published, rules, f.f.s, f.f.validator, executor, f.f.db, limits, config.DefaultReadValidation(), now)
	if err != nil {
		t.Fatal(err)
	}
	return s, published, rules
}

func phase19Create(t *testing.T, s *nlqbyo.Service, f *phase17Fixture, e identity.Envelope) nlqbyo.Bundle {
	t.Helper()
	result, err := s.Create(context.Background(), e, nlqbyo.CreateRequest{SchemaVersion: 1, Route: nlqroute.RouteRequest{Topic: f.pack.Topic, Context: f.context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 1}})
	if err != nil || result.Bundle == nil {
		t.Fatalf("context construction: %#v %v", result, err)
	}
	return *result.Bundle
}

type phase19CountingExecutor struct {
	delegate nlqbyo.Executor
	calls    atomic.Int64
}

func (x *phase19CountingExecutor) Execute(ctx context.Context, e identity.Envelope, p readexec.Plan, o readexec.Options) (readexec.ExecutionReport, error) {
	x.calls.Add(1)
	return x.delegate.Execute(ctx, e, p, o)
}

func TestPhase19(t *testing.T) {
	f := newPhase18Fixture(t)
	f.model.embeddingMode.Store("fixed")
	f.model.rerankMode.Store("fixed")
	limits := config.DefaultQueryBundles()
	limits.MaxSteps = 32
	counter := &phase19CountingExecutor{delegate: f.f.executor}
	s, published, rules := phase19Service(t, f, limits, nil, f.service, counter)
	ctx := context.Background()
	e, token := phase19Authority(t, f, f.e.Tenant(), f.e.User(), "phase19-session", phase19Scopes())
	pub, err := published.Read(ctx, f.e, f.pack.Topic, "")
	if err != nil {
		t.Fatal(err)
	}
	phase17PublishRules(t, rules, f.e, pub)

	t.Run("AC01", func(t *testing.T) {
		in := nlqbyo.CreateRequest{SchemaVersion: 1, Route: nlqroute.RouteRequest{Topic: f.pack.Topic, Context: f.context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 1, MetricIDs: []string{"revenue"}, Examples: []nlq.OptionalItem{{ID: "caller-example", Text: "Synthetic external example", Source: "reviewed", Priority: 1}}}}
		out, err := s.Create(ctx, e, in)
		if err != nil || out.Bundle == nil {
			t.Fatalf("context: %#v %v", out, err)
		}
		b := out.Bundle
		if b.SchemaVersion != 1 || !nlqbyo.ReferenceValid(b.Reference) || b.Context.Constraints == nil || len(b.Context.Constraints.Required) != 1 || b.Semantics[0].RuleVersion != "rules-v1" || b.Context.Tokens > b.Context.Budget || len(b.Context.Metrics) != 1 {
			t.Fatalf("missing mandatory/version/budget evidence: %#v", b)
		}
		if len(b.Context.Examples) != 1 || b.Context.Examples[0].Source != "external_input_unreviewed" || in.Route.Examples[0].Source != "reviewed" {
			t.Fatal("example provenance was forged or input mutated")
		}
		for _, r := range b.Requirements.Relations {
			for _, c := range r.Columns {
				if c.Name == "name" || c.Name == "secret" {
					t.Fatal("undeclared semantic column exposed")
				}
			}
		}
		view, err := s.Lookup(ctx, e, b.Reference)
		if err != nil || !reflect.DeepEqual(view.Bundle, *b) || len(view.Steps) != 0 {
			t.Fatalf("exact persisted bundle mismatch: %v", err)
		}
		b.Context.Constraints.Required[0].Text = "caller mutation"
		view, err = s.Lookup(ctx, e, b.Reference)
		if err != nil || view.Bundle.Context.Constraints.Required[0].Text == "caller mutation" {
			t.Fatal("shared bundle mutation", err)
		}
		in.Route.Locale = nlq.LanguageSpanish
		in.Route.Question = "¿Cuál es el ingreso?"
		spanish, err := s.Create(ctx, e, in)
		if err != nil || spanish.Bundle == nil || spanish.Bundle.Context.Locale != nlq.LanguageSpanish {
			t.Fatal("Spanish contract", err)
		}
		in.SchemaVersion = 2
		before := f.model.requests.Load()
		if _, err = s.Create(ctx, e, in); !errors.Is(err, nlqbyo.ErrInvalid) || f.model.requests.Load() != before {
			t.Fatal("unknown schema performed work", err)
		}
	})

	t.Run("AC02", func(t *testing.T) {
		b := phase19Create(t, s, f, e)
		before := counter.calls.Load()
		for _, v := range []struct{ tenant, user, session string }{{"foreign", e.User(), e.Session()}, {e.Tenant(), "foreign", e.Session()}, {e.Tenant(), e.User(), "foreign"}} {
			other, _ := phase19Authority(t, f, v.tenant, v.user, v.session, phase19Scopes())
			if _, err := s.Lookup(ctx, other, b.Reference); !errors.Is(err, nlqbyo.ErrReplan) {
				t.Fatalf("foreign binding disclosed: %v", err)
			}
			if _, err := s.Submit(ctx, other, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: "foreign", SQL: "SELECT id FROM analytics.sales"}); !errors.Is(err, nlqbyo.ErrReplan) {
				t.Fatalf("foreign submission: %v", err)
			}
		}
		bad := b.Reference
		bad.Context = "other-context"
		if _, err := s.Lookup(ctx, e, bad); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("context substitution", err)
		}
		changed := phase19Scopes()
		for i, v := range changed {
			if v == "cw.dataset.query:*" {
				changed[i] = "cw.dataset.query:" + f.pack.Datasets[0].ID
			}
		}
		narrow, _ := phase19Authority(t, f, e.Tenant(), e.User(), e.Session(), changed)
		if _, err := s.Lookup(ctx, narrow, b.Reference); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("changed data reach reused context", err)
		}
		if counter.calls.Load() != before {
			t.Fatal("denials reached execution")
		}
		// Reference possession and serialized envelopes are not capabilities.
		refOnly, _ := phase19Authority(t, f, e.Tenant(), e.User(), e.Session(), nil)
		if _, err := s.Lookup(ctx, refOnly, b.Reference); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("reference granted authority", err)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		b := phase19Create(t, s, f, e)
		before := counter.calls.Load()
		cases := []struct {
			name, sql  string
			parameters []readexec.Parameter
		}{
			{"write", "UPDATE analytics.sales SET amount=0", nil},
			{"write-cte", "WITH removed AS (DELETE FROM analytics.sales RETURNING id) SELECT id FROM removed", nil},
			{"select-into", "SELECT id INTO analytics.escaped FROM analytics.sales", nil},
			{"multi-statement", "SELECT id FROM analytics.sales; SELECT id FROM analytics.sales", nil},
			{"undeclared-relation", "SELECT id FROM analytics.foreign_rows", nil},
			{"system-catalog", "SELECT oid FROM pg_catalog.pg_class", nil},
			{"filesystem-function", "SELECT pg_read_file('/etc/passwd')", nil},
			{"side-effect-function", "SELECT pg_advisory_lock(1)", nil},
			{"dialect-escape", "SELECT TOP 1 id FROM analytics.sales", nil},
			{"missing-parameter", "SELECT id FROM analytics.sales WHERE id=$1", nil},
			{"extra-parameter", "SELECT id FROM analytics.sales", []readexec.Parameter{{Kind: "integer", Value: "1"}}},
			{"parameter-as-relation", "SELECT id FROM $1", []readexec.Parameter{{Kind: "text", Value: "analytics.sales"}}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := f.f.validator.Validate(ctx, e, readexec.Request{Source: b.Source, Context: b.Reference.Context, SQL: tc.sql, Parameters: tc.parameters}); err == nil {
					t.Fatal("baseline validator unexpectedly admitted escape")
				}
				out, err := s.Submit(ctx, e, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: tc.name, SQL: tc.sql, Parameters: tc.parameters})
				if err != nil || out.Step.Status != "rejected" || out.Result != nil || out.Step.Execution != nil {
					t.Fatalf("BYO did not retain safety rejection: %#v %v", out, err)
				}
			})
		}
		// Signed whole-source reach is deliberately broader than reviewed semantics.
		sql := "SELECT name FROM analytics.sales"
		if _, err := f.f.validator.Validate(ctx, e, readexec.Request{Source: b.Source, Context: b.Reference.Context, SQL: sql}); err != nil {
			t.Fatal("broad source fixture", err)
		}
		out, err := s.Submit(ctx, e, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: "hidden-column", SQL: sql})
		if err != nil || out.Step.Status != "rejected" || counter.calls.Load() != before {
			t.Fatal("semantic allowlist widened", err)
		}
		valid := []struct {
			sql        string
			parameters []readexec.Parameter
		}{
			{"WITH selected AS (SELECT id, amount FROM analytics.sales WHERE id>$1) SELECT id, amount FROM selected ORDER BY id", []readexec.Parameter{{Kind: "integer", Value: "0"}}},
			{"SELECT sum(amount) AS total FROM analytics.sales", nil},
			{"SELECT id FROM analytics.sales WHERE id=999", nil},
		}
		for i, tc := range valid {
			out, err := s.Submit(ctx, e, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: fmt.Sprintf("valid-%d", i), SQL: tc.sql, Parameters: tc.parameters})
			if err != nil || out.Result == nil || !out.ValuesAvailable || out.Step.Execution == nil || (out.Step.Status != "succeeded" && out.Step.Status != "empty") {
				t.Fatalf("valid common read: %#v %v", out, err)
			}
		}
		var count int
		if err := support.Raw(t, f.f.warehouse).QueryRow(ctx, "SELECT count(*) FROM analytics.sales").Scan(&count); err != nil || count != 2 {
			t.Fatal("baseline data changed", err, count)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		contextScopes := []string{}
		for _, scope := range phase19Scopes() {
			if scope != "query.submit" && scope != "sources.query" && scope != "cw.source.query:*" {
				contextScopes = append(contextScopes, scope)
			}
		}
		contextOnly, _ := phase19Authority(t, f, e.Tenant(), e.User(), "context-only", contextScopes)
		b := phase19Create(t, s, f, contextOnly)
		before := counter.calls.Load()
		if _, err := s.Submit(ctx, contextOnly, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: "no-action", SQL: "SELECT id FROM analytics.sales"}); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("context executed", err)
		}
		submitNoSource, _ := phase19Authority(t, f, e.Tenant(), e.User(), "context-only", append(append([]string{}, contextScopes...), "query.submit"))
		if _, err := s.Submit(ctx, submitNoSource, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: "no-source", SQL: "SELECT id FROM analytics.sales"}); err == nil {
			t.Fatal("submit action manufactured source authority")
		}
		if counter.calls.Load() != before {
			t.Fatal("authority denial executed")
		}
		// Fresh action permission may enable the same data snapshot; no context
		// action is necessary at submission and data reach cannot be broadened.
		scopes := []string{}
		for _, scope := range phase19Scopes() {
			if scope != "query.context" {
				scopes = append(scopes, scope)
			}
		}
		submitOnly, _ := phase19Authority(t, f, e.Tenant(), e.User(), "context-only", scopes)
		out, err := s.Submit(ctx, submitOnly, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: "allowed", SQL: "SELECT id FROM analytics.sales ORDER BY id"})
		if err != nil || out.Step.Status != "succeeded" {
			t.Fatalf("separate submit action: %#v %v", out, err)
		}
		registry, err := nlqapi.BYORegistry(true)
		if err != nil {
			t.Fatal(err)
		}
		h := assertRegisteredWireSchemas(t, registry, nlqapi.BYOHandler(f.model.token.verifier, s, http.NotFoundHandler()))
		server := httptest.NewServer(h)
		defer server.Close()
		client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
		if err != nil {
			t.Fatal(err)
		}
		created, err := client.GetQueryContext(ctx, sdk.QueryContextRequest{SchemaVersion: 1, Route: nlqroute.RouteRequest{Topic: f.pack.Topic, Context: f.context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 1}})
		if err != nil || created.Bundle == nil {
			t.Fatal("SDK context", err)
		}
		view, err := client.ReadQueryContext(ctx, created.Bundle.Reference)
		if err != nil || view.Bundle.ID != created.Bundle.ID {
			t.Fatal("SDK lookup", err)
		}
		run, err := client.SubmitSQL(ctx, sdk.SQLSubmission{Reference: created.Bundle.Reference, Operation: "sdk-step", SQL: "SELECT id FROM analytics.sales ORDER BY id"})
		if err != nil || run.Step.Status != "succeeded" || len(run.Result.Rows) != 2 {
			t.Fatalf("SDK actual SQL: %#v %v", run, err)
		}
		for _, field := range []string{`"tenant":"foreign"`, `"source":"foreign"`, `"dialect":"mysql"`, `"limits":{"rows":999999}`, `"schema_version":2`} {
			body := fmt.Sprintf(`{"schema_version":1,"bundle_id":%q,"context":%q,"operation":"injected","sql":"SELECT id FROM analytics.sales","parameters":[],%s}`, created.Bundle.ID, f.context, field)
			w := callProtected(t, h, "POST", "/v1/nlq/sql", token, body, nil)
			if w.Code != 400 {
				t.Fatalf("closed submit schema: %d %s", w.Code, w.Body.String())
			}
		}
	})

	t.Run("AC05", func(t *testing.T) {
		bounded := limits
		bounded.MaxSteps = 2
		limited, _, _ := phase19Service(t, f, bounded, nil, f.service, counter)
		b := phase19Create(t, limited, f, e)
		// A restarted process has neither the original router nor any gateway.
		offline, _, _ := phase19Service(t, f, bounded, nil, nil, counter)
		before := f.model.requests.Load()
		rawBefore := counter.calls.Load()
		request := nlqbyo.SubmitRequest{Reference: b.Reference, Operation: "concurrent-step", SQL: "SELECT id FROM analytics.sales ORDER BY id"}
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := offline.Submit(ctx, e, request)
				if err == nil && out.Step.Number != 1 {
					err = errors.New("duplicate step number")
				}
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal("concurrent submit", err)
			}
		}
		if counter.calls.Load() != rawBefore+1 {
			t.Fatal("duplicate physical read")
		}
		replay, err := offline.Submit(ctx, e, request)
		if err != nil || !replay.Replayed || replay.ValuesAvailable || replay.Result != nil || replay.Step.Status != "succeeded" {
			t.Fatalf("replay fabricated values or work: %#v %v", replay, err)
		}
		request.SQL = "SELECT amount FROM analytics.sales"
		if _, err = offline.Submit(ctx, e, request); !errors.Is(err, store.ErrConflict) {
			t.Fatal("operation input rebound", err)
		}
		request.Operation = "second-step"
		second, err := offline.Submit(ctx, e, request)
		if err != nil || second.Step.Number != 2 || !reflect.DeepEqual(second.Step.Semantics, b.Semantics) {
			t.Fatal("step evidence/budget", err)
		}
		request.Operation = "third-step"
		if _, err = offline.Submit(ctx, e, request); !errors.Is(err, nlqbyo.ErrBudget) {
			t.Fatal("step ceiling escaped", err)
		}
		view, err := offline.Lookup(ctx, e, b.Reference)
		if err != nil || len(view.Steps) != 2 || view.Steps[0].BundleDigest != readexec.Hash(b) || view.Steps[0].InputDigest == view.Steps[1].InputDigest {
			t.Fatal("durable per-step provenance missing", err)
		}
		if f.model.requests.Load() != before {
			t.Fatal("lookup/submit called a model")
		}
		raw, err := json.Marshal(view.Steps)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "SELECT ") || strings.Contains(string(raw), token) || strings.Contains(string(raw), `"result"`) {
			t.Fatal("step receipt persisted protected input/values/token")
		}
		if offline.CanCreate() {
			t.Fatal("unavailable context constructor advertised")
		}
		if _, err = offline.Create(ctx, e, nlqbyo.CreateRequest{SchemaVersion: 1, Route: nlqroute.RouteRequest{Context: f.context}}); !errors.Is(err, nlqbyo.ErrUnavailable) {
			t.Fatal("offline context became a stub", err)
		}
	})

	t.Run("AC06", func(t *testing.T) {
		var nanos atomic.Int64
		nanos.Store(time.Now().UnixNano())
		clock := func() time.Time { return time.Unix(0, nanos.Load()).UTC() }
		expiring := limits
		expiring.TTL = config.Duration(time.Second)
		timed, _, _ := phase19Service(t, f, expiring, clock, f.service, counter)
		b := phase19Create(t, timed, f, e)
		nanos.Store(b.ExpiresAt.UnixNano())
		before := f.model.requests.Load()
		readBefore := counter.calls.Load()
		if _, err := timed.Lookup(ctx, e, b.Reference); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("exact expiry admitted", err)
		}
		if _, err := timed.Submit(ctx, e, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: "expired", SQL: "SELECT id FROM analytics.sales"}); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("expired submission", err)
		}
		invalid := b.Reference
		invalid.ID = strings.Repeat("a", 64)
		if _, err := s.Lookup(ctx, e, invalid); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("invalid reference replanned silently", err)
		}
		if f.model.requests.Load() != before || counter.calls.Load() != readBefore {
			t.Fatal("expiry/absence performed new work")
		}
		// An exact recorded ruleset may not be silently replaced by its retired state.
		b = phase19Create(t, s, f, e)
		if _, err := rules.Retire(ctx, f.e, f.pack.Topic, rulesets.RetireRequest{Expected: 1, Note: "Synthetic phase 19 rule transition"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Lookup(ctx, e, b.Reference); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("rule pin substituted", err)
		}
		b = phase19Create(t, s, f, e)
		state, err := published.Read(ctx, f.e, f.pack.Topic, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = published.Archive(ctx, f.e, f.pack.Topic, state.State.Revision, "Synthetic phase 19 archive"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Submit(ctx, e, nlqbyo.SubmitRequest{Reference: b.Reference, Operation: "archived", SQL: "SELECT id FROM analytics.sales"}); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("archived pin executed", err)
		}
	})
}
