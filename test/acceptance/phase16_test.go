package acceptance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/topicapi"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

type phase16AcceptanceFixture struct {
	f         *engineeringFixture
	e         identity.Envelope
	pack      semantics.TopicPack
	published topics.Published
	rules     *rulesets.Service
	route     *nlqroute.Service
	model     *gatewayFixture
	client    *sdk.Client
	pending   rulesets.Draft
}

func newPhase16AcceptanceFixture(t *testing.T) *phase16AcceptanceFixture {
	t.Helper()
	f, draftsService, topicService, model, pack := publicationFixture(t)
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	published := phase17PublishTopic(t, draftsService, topicService, e, pack)
	rules, err := rulesets.New(f.db, f.db, f.db)
	if err != nil {
		t.Fatal("rules service", err)
	}
	definition := phase16RuleDefinition(published)
	draft, err := rules.Save(context.Background(), e, rulesets.SaveRequest{Definition: definition, Change: "Phase 16 reviewed constraints"})
	if err != nil {
		t.Fatal("initial rule draft", err)
	}
	review, err := rules.Review(context.Background(), e, pack.Topic, rulesets.ReviewRequest{DraftRevision: draft.Revision, Digest: draft.Digest, Decision: "approve", Note: "Phase 16 reviewed constraints"})
	if err != nil {
		t.Fatal("initial rule review", err)
	}
	if _, err = rules.Publish(context.Background(), e, pack.Topic, rulesets.PublishRequest{Review: review.ID}); err != nil {
		t.Fatal("initial rule publication", err)
	}
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal("vector index", err)
	}
	route, err := nlqroute.New(topicService, rules, index, model.engine)
	if err != nil {
		t.Fatal("routing service", err)
	}
	registry, err := topicapi.Registry()
	if err != nil {
		t.Fatal("topic registry", err)
	}
	handler := assertRegisteredWireSchemas(t, registry, topicapi.Handler(f.token.verifier, draftsService, topicService, rules, http.NotFoundHandler()))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		return f.token.sign(t, f.token.claims(e.Tenant(), e.User(), e.Scopes()), nil), nil
	})
	if err != nil {
		t.Fatal("topic client", err)
	}
	return &phase16AcceptanceFixture{f: f, e: e, pack: pack, published: published, rules: rules, route: route, model: model, client: client}
}

func phase16PublicPack(p topics.Published) semantics.TopicPack {
	out := semantics.TopicPack{
		SchemaVersion:     p.Definition.SchemaVersion,
		Topic:             p.Definition.Topic,
		Version:           p.Definition.Version,
		Name:              p.Definition.Name,
		Description:       p.Definition.Description,
		Measures:          p.Definition.Measures,
		Dimensions:        p.Definition.Dimensions,
		KPIs:              p.Definition.KPIs,
		Joins:             p.Definition.Joins,
		CanonicalEntities: p.Definition.CanonicalEntities,
	}
	for _, dataset := range p.Definition.Datasets {
		out.Datasets = append(out.Datasets, semantics.Dataset{ID: dataset.ID, Name: dataset.Name, Columns: append([]semantics.Column(nil), dataset.Columns...)})
	}
	return out
}

func phase16RuleDefinition(p topics.Published) semantics.RuleSetDefinition {
	measure := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	amount := semantics.Reference{Kind: semantics.KindColumn, Dataset: p.Definition.Datasets[0].ID, ID: "amount"}
	return semantics.RuleSetDefinition{
		SchemaVersion: semantics.SchemaVersion,
		ID:            "phase16-rules",
		Version:       "rules-v1",
		Topic:         p.State.Topic,
		TopicVersion:  p.State.Version,
		PackDigest:    p.Digest,
		Rules: []semantics.RuleDefinition{
			{ID: "require-revenue", Version: "v1", Category: semantics.RuleComputation, Class: semantics.RuleExecutionConstraint, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 100, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "phase16-review"}, Constraint: &semantics.Constraint{Kind: semantics.ConstraintRequireReference, Target: measure}},
			{ID: "revenue-advisory", Version: "v1", Category: semantics.RuleSemantic, Class: semantics.RuleAdvisoryContext, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 50, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "phase16-review"}, Guidance: &semantics.AdvisoryGuidance{Text: "Use the reviewed revenue definition.", Sensitivity: semantics.LiteralNonSensitive}},
		},
		Patterns: []semantics.ClarificationPattern{{
			ID: "metric-choice", Version: "v1", Targets: []semantics.Reference{measure, amount}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "phase16-review"},
			Slots: []semantics.ClarificationSlot{{
				ID: "metric", Prompt: "Choose a metric", Required: true, Kind: semantics.SlotChoice, Sensitivity: semantics.LiteralNonSensitive,
				Choices: []semantics.ClarificationChoice{{ID: "revenue", Label: "Revenue", Target: &measure}, {ID: "amount", Label: "Amount", Target: &amount}},
			}},
		}},
	}
}

func (f *phase16AcceptanceFixture) routeRequest(question string) nlqroute.RouteRequest {
	return nlqroute.RouteRequest{Topic: f.pack.Topic, Context: f.pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: question, Kinds: []string{"measure"}, LimitPerKind: 1, MetricIDs: []string{"revenue"}, Choices: []nlqroute.ChoiceSelection{{Pattern: "metric-choice", Slot: "metric", Value: "revenue"}}}
}

func TestPhase16(t *testing.T) {
	fixture := newPhase16AcceptanceFixture(t)
	ctx := context.Background()

	t.Run("AC01", func(t *testing.T) {
		definition := phase16RuleDefinition(fixture.published)
		definition.Version = "rules-v2"
		definition.Rules = append(definition.Rules, semantics.RuleDefinition{ID: "exclude-row-id", Version: "v1", Category: semantics.RuleStructural, Class: semantics.RuleExecutionConstraint, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 50, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "phase16-review-2"}, Constraint: &semantics.Constraint{Kind: semantics.ConstraintExcludeReference, Target: semantics.Reference{Kind: semantics.KindColumn, Dataset: fixture.pack.Datasets[0].ID, ID: "id"}}})
		draft, err := fixture.rules.Save(ctx, fixture.e, rulesets.SaveRequest{Expected: 1, Definition: definition, Change: "Prepare reviewed comparison candidate"})
		if err != nil || draft.Revision != 2 || draft.Digest == "" {
			t.Fatalf("candidate draft: %#v %v", draft, err)
		}
		fixture.pending = draft
		if _, err = fixture.rules.Publish(ctx, fixture.e, fixture.pack.Topic, rulesets.PublishRequest{Review: draft.Digest, Expected: 1}); err == nil {
			t.Fatal("draft self-approval was accepted")
		}
		narrow := fixture.f.token.envelope(t, fixture.e.Tenant(), fixture.e.User(), "topics.read", "cw.topic.read:"+fixture.pack.Topic, "cw.execution_context.use:"+fixture.pack.Datasets[0].Source.Context)
		if _, err = fixture.rules.Save(ctx, narrow, rulesets.SaveRequest{Expected: 2, Definition: definition, Change: "Missing signed write reach"}); err == nil {
			t.Fatal("rule draft ignored signed resource scope")
		}
		active, err := fixture.client.PublishedRules(ctx, fixture.pack.Topic)
		if err != nil || active.State.Version != "rules-v1" {
			t.Fatalf("candidate changed active state: %#v %v", active, err)
		}
	})

	t.Run("AC02", func(t *testing.T) {
		definition := phase16RuleDefinition(fixture.published)
		definition.Rules = append(definition.Rules, semantics.RuleDefinition{ID: "exclude-revenue", Version: "v1", Category: semantics.RuleStructural, Class: semantics.RuleExecutionConstraint, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 1, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceImport, Evidence: "phase16-conflict"}, Constraint: &semantics.Constraint{Kind: semantics.ConstraintExcludeReference, Target: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}}})
		subject, err := semantics.NewRuleSubject(phase16PublicPack(fixture.published), fixture.published.Digest)
		if err != nil {
			t.Fatal("rule subject", err)
		}
		_, err = semantics.CompilePublishedRules(subject, definition)
		var conflict *semantics.RuleConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("contradictory mandatory rules were accepted: %v", err)
		}
		evaluation, err := fixture.client.EvaluateRules(ctx, fixture.pack.Topic, sdk.RuleEvaluationRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}})
		if err != nil || !evaluation.Result.Allowed || len(evaluation.Result.Required) != 1 {
			t.Fatalf("active mandatory constraint was not evaluated: %#v %v", evaluation, err)
		}
		assembler, err := nlq.NewDefaultContextAssembler()
		if err != nil {
			t.Fatal(err)
		}
		assembled, err := assembler.Assemble(ctx, nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Question: "show revenue", Constraints: &nlq.ConstraintState{Allowed: true, Required: []nlq.MandatoryConstraint{{ID: "measure:revenue", Kind: "required", Text: "use the approved revenue measure"}}, Excluded: []nlq.MandatoryConstraint{{ID: "column:customer", Kind: "excluded", Text: "exclude raw customer"}}}}, nlq.TierLow)
		if err != nil || assembled.Constraints == nil || len(assembled.Constraints.Required) != 1 || len(assembled.Constraints.Excluded) != 1 {
			t.Fatalf("mandatory choices were dropped from context: %#v %v", assembled, err)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		before := fixture.model.requests.Load()
		request := fixture.routeRequest("What is the approved metric?")
		request.Choices = nil
		out, err := fixture.route.Route(ctx, fixture.e, request)
		if err != nil || out.Outcome != nlq.StrategyClarify || out.Clarification == nil || out.Clarification.Reason != "required_slot" || out.Clarification.Slot != "metric" {
			t.Fatalf("required slot was not a typed terminal outcome: %#v %v", out, err)
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("clarification reached the model gateway")
		}
		out, err = fixture.route.Route(ctx, fixture.e, fixture.routeRequest("What is revenue?"))
		if err != nil || out.Context == nil || out.Outcome == nlq.StrategyClarify || out.Context.Strategy == nlq.StrategyClarify {
			t.Fatalf("supplied choice did not permit routing: %#v %v", out, err)
		}
		if len(out.Context.Metrics) != 1 || out.Context.Metrics[0].ID != fixture.pack.Topic+":revenue" {
			t.Fatalf("selected metric was not preserved: %#v", out.Context.Metrics)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		examples := make([]nlq.OptionalItem, 12)
		for i := range examples {
			examples[i] = nlq.OptionalItem{ID: "example-" + strconv.Itoa(i), Text: "approved revenue example", Priority: i}
		}
		request := fixture.routeRequest("¿Qué ingresos fueron aprobados?")
		request.Locale = nlq.LanguageSpanish
		request.Examples = examples
		out, err := fixture.route.Route(ctx, fixture.e, request)
		if err != nil || out.Context == nil || out.Context.Locale != nlq.LanguageSpanish || out.Context.Tokens < 1 || out.Context.Tokens > out.Context.Budget {
			t.Fatalf("real tokenizer context was not bounded: %#v %v", out, err)
		}
		if out.Context.Constraints == nil || len(out.Context.Constraints.Required) != 1 || out.Context.Constraints.Required[0].ID != "measure:revenue" {
			t.Fatalf("active mandatory constraint was lost: %#v", out.Context.Constraints)
		}
		if len(out.Context.Advisory) != 1 || len(out.Context.Examples) > nlq.MaxExamples || len(out.Audit.Omitted) > nlq.MaxOmissions || out.Audit.OmittedCount == 0 {
			t.Fatalf("advisory/example bounds were not audited: context=%#v audit=%#v", out.Context, out.Audit)
		}
		assembler, err := nlq.NewDefaultContextAssembler()
		if err != nil {
			t.Fatal(err)
		}
		for _, tier := range []nlq.Tier{nlq.TierLow, nlq.TierMedium, nlq.TierHigh} {
			assembled, assembleErr := assembler.Assemble(ctx, nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Question: "tiered question", Metrics: []nlq.PinnedMetric{{ID: "revenue", Text: "approved revenue"}}}, tier)
			if assembleErr != nil || assembled.Tokens < 1 || assembled.Tokens > tier.Budget() {
				t.Fatalf("tier %s did not use the pinned token budget: %#v %v", tier, assembled, assembleErr)
			}
		}
		required := []nlq.MandatoryConstraint{{ID: "required-a", Kind: "required", Text: strings.Repeat("required ", 700)}, {ID: "required-b", Kind: "required", Text: strings.Repeat("required ", 700)}, {ID: "required-c", Kind: "required", Text: strings.Repeat("required ", 700)}}
		_, err = assembler.Assemble(ctx, nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Question: "bounded question", Constraints: &nlq.ConstraintState{Allowed: true, Required: required}}, nlq.TierLow)
		if !errors.Is(err, nlq.ErrInsufficient) {
			t.Fatalf("combined valid mandatory entries did not produce typed insufficiency: %v", err)
		}
	})

	t.Run("AC05", func(t *testing.T) {
		if fixture.pending.Digest == "" {
			t.Fatal("AC01 did not preserve candidate draft for comparison")
		}
		review, err := fixture.rules.Review(ctx, fixture.e, fixture.pack.Topic, rulesets.ReviewRequest{DraftRevision: fixture.pending.Revision, Digest: fixture.pending.Digest, Decision: "approve", Note: "Approve shadow candidate"})
		if err != nil {
			t.Fatal("candidate review", err)
		}
		published, err := fixture.rules.Publish(ctx, fixture.e, fixture.pack.Topic, rulesets.PublishRequest{Review: review.ID, Expected: 1})
		if err != nil || published.State.Version != "rules-v2" {
			t.Fatalf("candidate publication: %#v %v", published, err)
		}
		refs := []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}, {Kind: semantics.KindColumn, Dataset: fixture.pack.Datasets[0].ID, ID: "id"}}
		comparison, err := fixture.client.ShadowRules(ctx, fixture.pack.Topic, sdk.RuleShadowRequest{BaselineRuleVersion: "rules-v1", CandidateRuleVersion: "rules-v2", TopicVersion: fixture.pack.Version, References: refs})
		if err != nil || comparison.Candidate == nil || !comparison.Changed || !comparison.Baseline.Result.Allowed || comparison.Candidate.Result.Allowed {
			t.Fatalf("retained shadow did not preserve exact changed results: %#v %v", comparison, err)
		}
		replay, err := fixture.client.ReplayRules(ctx, fixture.pack.Topic, sdk.RuleReplayRequest{RuleVersion: "rules-v1", TopicVersion: fixture.pack.Version, References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}})
		if err != nil || replay.Baseline.RuleVersion != "rules-v1" || !replay.Baseline.Result.Allowed || replay.Baseline.PackDigest != fixture.published.Digest {
			t.Fatalf("historical replay lost its retained pin: %#v %v", replay, err)
		}
		current, err := fixture.client.PublishedRules(ctx, fixture.pack.Topic)
		if err != nil || current.State.Version != "rules-v2" {
			t.Fatalf("shadow changed active production rules: %#v %v", current, err)
		}
		metadata := support.Raw(t, fixture.f.dsn)
		if _, err = metadata.Exec(ctx, `UPDATE chartworks.topic_rule_comparison_evidence SET changed=NOT changed WHERE tenant_id=$1 AND comparison_id=$2`, fixture.e.Tenant(), comparison.ID); err == nil {
			t.Fatal("comparison evidence was mutable")
		}
	})

	t.Run("AC06", func(t *testing.T) {
		before, err := fixture.client.RuleInvalidations(ctx, fixture.pack.Topic, sdk.RuleInvalidationRequest{Limit: 8})
		if err != nil || len(before) != 2 || before[0].Kind != "publish" || before[1].OldRuleVersion != "rules-v1" || before[1].NewRuleVersion != "rules-v2" {
			t.Fatalf("publish invalidation ledger incomplete: %#v %v", before, err)
		}
		if _, err = fixture.client.RuleInvalidations(ctx, fixture.pack.Topic, sdk.RuleInvalidationRequest{After: 1, Limit: 8}); err != nil {
			t.Fatal("invalidation cursor read", err)
		}
		retired, err := fixture.client.RetireRules(ctx, fixture.pack.Topic, sdk.RetireRulesRequest{Expected: 2, Note: "Retire candidate rules"})
		if err != nil || !retired.Retired || retired.Version != "rules-v2" {
			t.Fatalf("rule retirement: %#v %v", retired, err)
		}
		all, err := fixture.client.RuleInvalidations(ctx, fixture.pack.Topic, sdk.RuleInvalidationRequest{Limit: 8})
		if err != nil || len(all) != 3 || all[2].Kind != "retire" || all[2].OldRuleVersion != "rules-v2" || all[2].NewRuleVersion != "" || all[2].TopicVersion != fixture.pack.Version {
			t.Fatalf("retirement invalidation fence incomplete: %#v %v", all, err)
		}
		if _, err = fixture.client.PublishedRules(ctx, fixture.pack.Topic); !sdkConflict(err) {
			t.Fatalf("retired current rules remained active: %v", err)
		}
		if _, err = fixture.client.PublishedRuleVersion(ctx, fixture.pack.Topic, "rules-v1"); err != nil {
			t.Fatalf("historical rule read was lost after invalidation: %v", err)
		}
		metadata := support.Raw(t, fixture.f.dsn)
		if _, err = metadata.Exec(ctx, `DELETE FROM chartworks.topic_rule_evidence_invalidations WHERE tenant_id=$1 AND invalidation_id=$2`, fixture.e.Tenant(), all[2].ID); err == nil {
			t.Fatal("invalidation ledger was mutable")
		}
	})
}
