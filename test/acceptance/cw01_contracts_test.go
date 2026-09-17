package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
)

func cw01Pattern(t *testing.T, definition semantics.RuleSetDefinition, id string) semantics.ClarificationPattern {
	t.Helper()
	definition = semantics.CloneRuleSetDefinition(definition)
	for _, p := range definition.Patterns {
		if p.ID == id {
			return p
		}
	}
	t.Fatal("synthetic pattern missing")
	return semantics.ClarificationPattern{}
}

func cw01OrderingAcceptance(t *testing.T, f *cw01Fixture) {
	t.Helper()
	ctx := context.Background()
	question := f.question("Show large sales and active sales", nlq.LanguageEnglish)
	before := f.model.requests.Load()
	pending := f.preflight(t, question)
	if pending.Route.Clarification == nil || len(pending.Route.Clarification.Questions) != 2 || f.model.requests.Load() != before {
		t.Fatal("independent blockers not grouped before provider")
	}
	for _, q := range pending.Route.Clarification.Questions {
		if q.Prompt == "" || q.Why == "" {
			t.Fatal("independent blocker has no explanation")
		}
	}
	definition := semantics.CloneRuleSetDefinition(f.definition)
	cases := []semantics.ClarificationInput{{Locale: "en", Question: question.Question}}
	left, err := f.rules.PreviewClarifications(ctx, f.e, rulesets.ClarificationPreviewRequest{Definition: definition, Cases: cases})
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(definition.Patterns)-1; i < j; i, j = i+1, j-1 {
		definition.Patterns[i], definition.Patterns[j] = definition.Patterns[j], definition.Patterns[i]
	}
	right, err := f.rules.PreviewClarifications(ctx, f.e, rulesets.ClarificationPreviewRequest{Definition: definition, Cases: cases})
	if err != nil || left.RuleDigest != right.RuleDigest || !reflect.DeepEqual(left.Cases, right.Cases) {
		t.Fatal("ordering depends on persistence order", err)
	}
	lower := cw01Pattern(t, f.definition, "amount-default")
	lower.ID = "lower"
	lower.Policy.When.AnyTerms = []string{"conflict sales"}
	lower.Slots[0].Default = &semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: "10", Unit: "USD"}}
	upper := cw01Pattern(t, f.definition, "amount-default")
	upper.ID = "upper"
	upper.Policy.When.AnyTerms = []string{"conflict sales"}
	upper.Slots[0].Effect.Operator = "lt"
	upper.Slots[0].Default = &semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: "5", Unit: "USD"}}
	definition.Patterns = []semantics.ClarificationPattern{upper, lower}
	conflict, err := f.rules.PreviewClarifications(ctx, f.e, rulesets.ClarificationPreviewRequest{Definition: definition, Cases: []semantics.ClarificationInput{{Locale: "en", Question: "Show conflict sales"}}})
	if err != nil || conflict.Cases[0].Outcome != semantics.ClarificationConflicting || len(conflict.Cases[0].Resolutions) != 0 || f.model.requests.Load() != before {
		t.Fatal("incompatible defaults did not fail atomically", err)
	}
	// Dependent slots are not presented alongside their unanswered prerequisite.
	dependent := cw01Pattern(t, f.definition, "amount-required")
	dependent.ID = "ordered"
	dependent.Policy.When.AnyTerms = []string{"dependent sales"}
	dependent.Slots[0].ID = "minimum"
	next := cw01Pattern(t, f.definition, "state").Slots[0]
	next.DependsOn = []string{"minimum"}
	dependent.Slots = append(dependent.Slots, next)
	dependent.Targets = append(dependent.Targets, next.Effect.Target)
	definition.Patterns = []semantics.ClarificationPattern{dependent}
	order, err := f.rules.PreviewClarifications(ctx, f.e, rulesets.ClarificationPreviewRequest{Definition: definition, Cases: []semantics.ClarificationInput{{Locale: "en", Question: "Show dependent sales"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(order.Cases[0].Slots) != 2 || order.Cases[0].Slots[0].Slot != "minimum" || order.Cases[0].Slots[1].Reason != "dependency_missing" {
		t.Fatal("dependent evaluation lost prerequisite ordering")
	}
}

func cw01DefaultsAcceptance(t *testing.T, f *cw01Fixture) {
	t.Helper()
	ctx := context.Background()
	defaults, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show default sales", nlq.LanguageEnglish)})
	if err != nil || len(defaults.Route.Resolutions) != 1 || defaults.Route.Resolutions[0].Provenance != "reviewed_default" || len(defaults.Route.Request.Answers) != 0 {
		t.Fatal("default is not visible or attributed", err)
	}
	visible := false
	for _, g := range defaults.Route.Clarifications {
		for _, slot := range g.Slots {
			visible = visible || slot.Defaulted && slot.Pattern == "amount-default"
		}
	}
	if !visible {
		t.Fatal("default omitted from user contract")
	}
	f.run(t, defaults, 1, true)
	required := f.plan(t, f.question("Show large sales", nlq.LanguageEnglish), "amount-required", cw01Number("10"))
	remove := f.answer(t, "amount-required", cw01Number("10"))
	remove.Value = nil
	remove.Remove = true
	before := f.model.requests.Load()
	out, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: required.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{remove}}})
	var failure *nlqroute.Clarification
	if !errors.As(err, &failure) || failure.Outcome != semantics.ClarificationMissing || out.QueryID != "" || f.model.requests.Load() != before {
		t.Fatal("required removal silently skipped or defaulted", err)
	}
}

func cw01IsolationAcceptance(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	f := newCW01Fixture(t)
	q := f.question("Show large sales", nlq.LanguageEnglish)
	pending := f.preflight(t, q)
	q.AnswerContext = pending.Route.AnswerContext
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, "amount-required", cw01Number("10"))}
	before := f.model.requests.Load()
	stale := q
	stale.Answers = semantics.CloneClarificationAnswers(q.Answers)
	stale.Answers[0].PatternVersion = "old-version"
	if _, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: stale}); err == nil {
		t.Fatal("stale pattern version accepted")
	}
	foreign := phase18Envelope(t, f.phase17Fixture, f.e.User(), "other-session", true)
	if _, err := f.query.Plan(ctx, foreign, nlqexec.PlanRequest{QuestionRequest: q}); err == nil {
		t.Fatal("same-actor cross-session answers accepted")
	}
	claims := f.model.token.claims(f.e.Tenant(), f.e.User(), []string{"query.plan", "query.execute", "topics.read", "cw.topic.read:" + f.pack.Topic})
	claims["session"] = f.e.Session()
	limited, err := f.model.token.verifier.Verify(ctx, f.model.token.sign(t, claims, nil), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.query.Plan(ctx, limited, nlqexec.PlanRequest{QuestionRequest: q}); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("current bearer reach was not rechecked", err)
	}
	wrong := q
	wrong.Context = "another-context"
	if _, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: wrong}); err == nil {
		t.Fatal("foreign source context accepted")
	}
	if f.model.requests.Load() != before {
		t.Fatal("invalid version/session/context reached provider")
	}
	planned, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal(err)
	}
	before = f.model.requests.Load()
	tenantClaims := f.model.token.claims("other-tenant", f.e.User(), phase18Scopes("other-tenant", true))
	tenantClaims["session"] = f.e.Session()
	tenant, err := f.model.token.verifier.Verify(ctx, f.model.token.sign(t, tenantClaims, nil), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.query.Refine(ctx, tenant, nlqexec.RefineRequest{QueryID: planned.QueryID}); err == nil {
		t.Fatal("cross-tenant query was replayed")
	}
	if _, err = f.query.Refine(ctx, foreign, nlqexec.RefineRequest{QueryID: planned.QueryID}); !errors.Is(err, nlqexec.ErrForeignSession) {
		t.Fatal("retained query crossed its session", err)
	}
	newer := semantics.CloneRuleSetDefinition(f.definition)
	newer.Version = "rules-v2"
	f.publishRules(t, newer, 1)
	if _, err = f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: planned.QueryID}); err == nil {
		t.Fatal("publication change silently reused old answer")
	}
	if f.model.requests.Load() != before {
		t.Fatal("stale or foreign replay reached provider")
	}
	t.Run("source-revision", func(t *testing.T) {
		source := newCW01Fixture(t)
		plan := source.plan(t, source.question("Show large sales", nlq.LanguageEnglish), "amount-required", cw01Number("10"))
		binding := source.pack.Datasets[0].Source
		if _, err := source.f.s.Rotate(ctx, source.f.e, binding.Source, binding.SourceRevision); err != nil {
			t.Fatal("rotate fixture source", err)
		}
		before := source.model.requests.Load()
		if _, err := source.query.Refine(ctx, source.e, nlqexec.RefineRequest{QueryID: plan.QueryID}); err == nil {
			t.Fatal("rotated source accepted stale answers")
		}
		if source.model.requests.Load() != before {
			t.Fatal("source-stale replay reached provider")
		}
	})
}

func cw01BudgetPrivacyAcceptance(t *testing.T, f *cw01Fixture) {
	t.Helper()
	ctx := context.Background()
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.mu.Unlock()
	sensitive := f.plan(t, f.question("Show named sales", nlq.LanguageEnglish), "customer", cw01Text("alias-secret-731"))
	f.run(t, sensitive, 1, true)
	f.model.mu.Lock()
	bodies := strings.Join(append([]string(nil), f.model.requestBodies[start:]...), "\n")
	f.model.mu.Unlock()
	for _, value := range []string{"alias-secret-731", "cw-alpha-731", "cw-beta-731"} {
		if strings.Contains(bodies, value) {
			t.Fatal("sensitive clarification value reached provider")
		}
	}
	scope, err := store.NewScope(f.e.Tenant(), f.e.User())
	if err != nil {
		t.Fatal(err)
	}
	record, err := f.f.db.ReadQuery(ctx, scope, sensitive.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(record.Route.Request)
	if strings.Contains(string(encoded), "alias-secret-731") || len(record.Route.Resolutions) != 1 || record.Route.Resolutions[0].Sensitivity != semantics.LiteralSensitive || record.Route.Resolutions[0].ParserVersion != "clarification-values-v1" || record.Route.Resolutions[0].Locale != "en" {
		t.Fatal("raw answer retained or classification/parser provenance lost")
	}
	diagnostic := fmt.Sprintf("%v %#v", f.answer(t, "customer", cw01Text("alias-secret-731")), record.Route.Resolutions[0])
	if strings.Contains(diagnostic, "alias-secret-731") || strings.Contains(diagnostic, "cw-alpha-731") {
		t.Fatal("ordinary diagnostics leak protected values")
	}
	unresolved := f.question("Show named sales", nlq.LanguageEnglish)
	pending := f.preflight(t, unresolved)
	unresolved.AnswerContext = pending.Route.AnswerContext
	unresolved.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("unresolved-private-731"))}
	before := f.model.requests.Load()
	repair, err := f.query.Preflight(ctx, f.e, nlqexec.PreflightRequest{QuestionRequest: unresolved})
	if err != nil || repair.Route.Clarification == nil || f.model.requests.Load() != before {
		t.Fatal("unresolved sensitive answer executed", err)
	}
	saved, err := f.f.db.ReadQuery(ctx, scope, repair.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), "unresolved-private-731") {
		t.Fatal("unresolved raw answer persisted")
	}
	t.Run("mandatory-group", func(t *testing.T) {
		bounded := newCW01Fixture(t)
		definition := semantics.CloneRuleSetDefinition(bounded.definition)
		definition.Version = "budget-rules"
		prototype := cw01Pattern(t, definition, "amount-default")
		definition.Patterns = nil
		for i := 0; i < 64; i++ {
			candidate := semantics.CloneRuleSetDefinition(semantics.RuleSetDefinition{Patterns: []semantics.ClarificationPattern{prototype}}).Patterns[0]
			candidate.ID = fmt.Sprintf("budget-%02d", i)
			candidate.Policy.When.AnyTerms = []string{"budget sales"}
			definition.Patterns = append(definition.Patterns, candidate)
		}
		bounded.publishRules(t, definition, 1)
		before := bounded.model.requests.Load()
		out, err := bounded.query.Plan(ctx, bounded.e, nlqexec.PlanRequest{QuestionRequest: bounded.question("Show budget sales", nlq.LanguageEnglish)})
		if !errors.Is(err, nlq.ErrInsufficientContext) || out.QueryID != "" || out.Bindings != nil || bounded.model.requests.Load() != before {
			t.Fatalf("mandatory group was partially admitted or reached provider: %v", err)
		}
	})
}
