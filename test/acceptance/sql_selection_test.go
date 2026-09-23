package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

// Real published semantics, rules, Bifrost wire, native SQL validation and
// PostgreSQL persistence/execution. The model response remains a recorded
// synthetic fixture: this test does not claim live language-model quality.
func TestSQLRecoverySelectedIntentAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	metric := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	amount := semantics.Reference{Kind: semantics.KindColumn, Dataset: f.pack.Datasets[0].ID, ID: "amount"}
	definition := f.definition
	definition.Version = "selected-rules-v2"
	definition.Rules = []semantics.RuleDefinition{{ID: "selected-revenue", Version: "v1", Category: semantics.RuleStructural, Class: semantics.RuleExecutionConstraint, Scope: semantics.RuleScope{Kind: semantics.RuleScopeEntities, Targets: []semantics.Reference{metric}}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-selected-review"}, Constraint: &semantics.Constraint{Kind: semantics.ConstraintRequireReference, Target: amount}}}
	for i := range definition.Patterns {
		if definition.Patterns[i].ID == "amount-required" {
			definition.Patterns[i].Policy.When = semantics.ClarificationWhen{AnyReferences: []semantics.Reference{metric}}
		}
	}
	f.publishRules(t, definition, 1)
	f.definition = definition
	question := f.question("Revenue", nlq.LanguageEnglish)
	pending := f.preflight(t, question)
	if pending.Route.Selection == nil || pending.Route.Clarification == nil || len(pending.Route.RemoteCalls) != 0 {
		t.Fatal("catalog-selected metric did not activate clarification before model work")
	}
	planned := f.plan(t, question, "amount-required", semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: "10", Unit: "USD"}})
	if planned.Route.Selection == nil || len(planned.Route.Context.Metrics) != 1 || len(planned.Route.Request.MetricIDs) != 0 {
		t.Fatal("free-text selected intent was replaced by caller metric pins")
	}
	f.run(t, planned, 1, true)
	scope, err := store.NewScope(f.e.Tenant(), f.e.User())
	if err != nil {
		t.Fatal(err)
	}
	record, err := f.f.db.ReadQuery(ctx, scope, planned.QueryID)
	if err != nil || record.Route.Selection == nil {
		t.Fatal("SQL generation lost selection across real persistence", err)
	}
	before, _ := json.Marshal(planned.Route.Selection)
	after, _ := json.Marshal(record.Route.Selection)
	if string(before) != string(after) {
		t.Fatal("persisted semantic selection changed")
	}
	// Editing a selected free-text metric uses the existing paired typed edit
	// contract and records an omission instead of selecting it again from text.
	_, err = f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: planned.QueryID,
		QuestionRequest: nlqexec.QuestionRequest{Question: "Show sales rows"},
		ReferenceEdits:  []nlqexec.ReferenceEdit{{Action: "remove", Target: metric}},
		MetricEdits:     []nlqexec.MetricEdit{{Action: "remove", Target: "revenue"}},
	})
	// The metric-scoped scalar answer cannot silently migrate to an unrelated
	// question. This operation must require explicit answer removal or clarify.
	var clarification *nlqroute.Clarification
	if !errors.As(err, &clarification) || clarification.Reason != "clarification_invalid" {
		t.Fatal("metric removal did not identify the inapplicable retained answer", err)
	}
}
