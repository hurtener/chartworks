package acceptance

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func dependentClarificationFixture(t *testing.T) *cw01Fixture {
	t.Helper()
	f := newCW01Fixture(t)
	d := f.definition
	d.Version = "dependent-rules-v2"
	amount := semantics.Reference{Kind: semantics.KindColumn, Dataset: f.pack.Datasets[0].ID, ID: "amount"}
	active := semantics.Reference{Kind: semantics.KindColumn, Dataset: f.pack.Datasets[0].ID, ID: "active"}
	child := cw01Pattern(t, d, "amount-required")
	child.Policy.When = semantics.ClarificationWhen{AnyReferences: []semantics.Reference{amount}}
	child.Slots[0].Sensitivity = semantics.LiteralSensitive
	gate := semantics.ClarificationPattern{ID: "basis", Version: "v1", Targets: []semantics.Reference{amount, active}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-dependent-review"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"inspect basis", "inspeccionar base"}}, Why: "Choose the reviewed basis before its conditional inputs."}, Slots: []semantics.ClarificationSlot{{ID: "basis", Kind: semantics.SlotChoice, Required: true, Sensitivity: semantics.LiteralNonSensitive, Prompt: "Which basis?", PromptES: "¿Qué base?", Choices: []semantics.ClarificationChoice{{ID: "first", Label: "First basis", Target: &amount}, {ID: "second", Label: "Second basis", Target: &active}}}}}
	d.Patterns = []semantics.ClarificationPattern{child, gate}
	f.publishRules(t, d, 1)
	f.definition = d
	return f
}

// Real PostgreSQL storage, native owned-predicate binding, recorded provider
// requests and restart/replay. No live model-comprehension claim is made.
func TestSQLRecoveryDependentClarificationLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := dependentClarificationFixture(t)
			ctx := context.Background()
			metadata := support.Raw(t, f.f.dsn)
			text := "Inspect basis"
			if locale == nlq.LanguageSpanish {
				text = "Inspeccionar base"
			}
			q := f.question(text, locale)
			before := f.model.requests.Load()
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			first := f.preflight(t, q)
			if first.Route.Clarification == nil || len(first.Route.Clarification.Questions) != 1 || first.Route.Clarification.Questions[0].Pattern != "basis" || f.model.requests.Load() != before {
				t.Fatal("initial conditional choice did not block model work")
			}
			gate := f.answer(t, "basis", semantics.ClarificationValue{OptionID: "first"})
			q.AnswerContext = first.Route.AnswerContext
			q.ClarificationQuery = first.QueryID
			q.Answers = []semantics.ClarificationAnswer{gate}
			second := f.preflight(t, q)
			if second.Route.AnswerContext != first.Route.AnswerContext || second.Route.Clarification == nil || len(second.Route.Clarification.Questions) != 1 || second.Route.Clarification.Questions[0].Pattern != "amount-required" || f.model.requests.Load() != before || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("answer activation lost preflight/source context or issued work")
			}
			scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
			partial, err := f.f.db.ReadQuery(ctx, scope, second.QueryID)
			if err != nil || partial.Parent != first.QueryID || len(partial.Route.Request.Answers) != 1 || partial.SQL != "" {
				t.Fatal("partial answers/lineage not durable", err)
			}
			q.ClarificationQuery = second.QueryID
			threshold := f.answer(t, "amount-required", cw01Number("10.125"))
			if locale == nlq.LanguageSpanish {
				threshold.Value.Number.Value = "10,125"
			}
			q.Answers = []semantics.ClarificationAnswer{threshold, gate}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			f.model.mu.Lock()
			wireStart := len(f.model.requestBodies)
			f.model.mu.Unlock()
			plan, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
			if err != nil || plan.QueryID == "" || plan.Bindings == nil || plan.Bindings.Validation == nil || !plan.Bindings.Validation.Validated || len(plan.Route.Resolutions) != 2 {
				t.Fatal("dependent SQL plan", err)
			}
			result, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-run"})
			if err != nil {
				t.Fatal(err)
			}
			requireParameterIDs(t, result, "1")
			parent, err := f.f.db.ReadQuery(ctx, scope, plan.QueryID)
			if err != nil || len(parent.Parameters) != 1 || parent.Parameters[0].Value != "10.125" {
				t.Fatal("exact private dependent binding not retained", err)
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			calls := f.model.requests.Load()
			attempts = count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-run"})
			if err != nil || f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("dependent terminal replay issued work", err)
			}
			requireParameterIDs(t, replay, "1")
			// Changing the controlling answer must not retain the inactive predicate.
			// Removal is explicit, scoped to this parent's exact reviewed slot identity.
			gate.Value = &semantics.ClarificationValue{OptionID: "second"}
			threshold.Value = nil
			threshold.Remove = true
			child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{gate, threshold}}})
			if err != nil || len(child.Route.Resolutions) != 1 {
				t.Fatal("branch replacement/removal failed", err)
			}
			clean, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil {
				t.Fatal(err)
			}
			requireParameterIDs(t, clean, "1", "2")
			fresh, err := f.f.db.ReadQuery(ctx, scope, child.QueryID)
			if err != nil || len(fresh.Parameters) != 0 || strings.Contains(fresh.SQL, "cw_filter_") {
				t.Fatal("inactive branch retained a service-owned filter", err)
			}
			unchanged, err := f.f.db.ReadQuery(ctx, scope, parent.ID)
			if err != nil || !reflect.DeepEqual(unchanged.Parameters, parent.Parameters) || nlqexec.QueryLineageDigest(unchanged) != nlqexec.QueryLineageDigest(parent) {
				t.Fatal("branch edit rewrote ancestor", err)
			}
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[wireStart:], "\n")
			f.model.mu.Unlock()
			if strings.Contains(wire, "10.125") || strings.Contains(wire, "10,125") {
				t.Fatal("private dependent value entered provider requests")
			}
		})
	}
}

func TestSQLRecoveryDependentClarificationInvalidAcceptance(t *testing.T) {
	f := dependentClarificationFixture(t)
	ctx := context.Background()
	q := f.question("Inspect basis", nlq.LanguageEnglish)
	pending := f.preflight(t, q)
	q.AnswerContext = pending.Route.AnswerContext
	q.ClarificationQuery = pending.QueryID
	metadata := support.Raw(t, f.f.dsn)
	gate := f.answer(t, "basis", semantics.ClarificationValue{OptionID: "first"})
	threshold := f.answer(t, "amount-required", cw01Number("NaN"))
	for _, tc := range []struct {
		name    string
		answers []semantics.ClarificationAnswer
	}{
		{"inactive_child", []semantics.ClarificationAnswer{threshold}},
		{"invalid_active_child", []semantics.ClarificationAnswer{threshold, gate}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := f.model.requests.Load()
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			q.Answers = tc.answers
			plan, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
			var invalid *nlqroute.Clarification
			if !errors.As(err, &invalid) || invalid.Outcome != semantics.ClarificationInvalid || plan.QueryID != "" || f.model.requests.Load() != before || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("invalid conditional answer planned or issued work", err)
			}
		})
	}
}
