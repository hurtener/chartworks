package nlqexec

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestSQLRecoveryInferredRootsSupportTypedRemovalAndRestore(t *testing.T) {
	metric := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	dimension := semantics.Reference{Kind: semantics.KindDimension, ID: "region"}
	required := semantics.Reference{Kind: semantics.KindDimension, ID: "required_region"}
	old := QueryRecord{Question: "Revenue by region", Topic: "topic", Topics: []string{"topic"}, Context: "context", Locale: nlq.LanguageEnglish,
		Route: nlqroute.RouteResult{Selection: &nlqroute.SemanticSelection{Version: "catalog-selection-v1", Topics: []nlqroute.SelectedTopic{{Topic: "topic", Roots: []nlqroute.SelectedRoot{{Reference: metric, Reason: "catalog_term"}, {Reference: dimension, Reason: "catalog_term"}, {Reference: required, Reason: "required_rule"}}}}}}}
	before, _ := json.Marshal(old)
	q := refinementQuestion(old, QuestionRequest{})
	retainCatalogSelection(old, &q)
	if len(q.References) != 2 || len(q.MetricIDs) != 1 {
		t.Fatal("inferred roots not addressable by existing edit API, or rule root promoted")
	}
	edits := []ReferenceEdit{{Action: "remove", Target: metric}}
	metricEdits := []MetricEdit{{Action: "remove", Target: metric.ID}}
	if err := applyReferenceEdits(&q, edits); err != nil {
		t.Fatal(err)
	}
	if err := applyMetricEdits(&q, metricEdits); err != nil {
		t.Fatal(err)
	}
	retainSelectionOmissions(&q, edits)
	canonicalizeQuestion(&q)
	if err := validateMetricReferenceCoherence(q, edits, metricEdits); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(q.OmittedRoots, []semantics.Reference{metric}) || q.Question != old.Question || len(q.MetricIDs) != 0 {
		t.Fatal("removal failed to suppress re-inference from the unchanged utterance")
	}
	if !reflect.DeepEqual(q.routeRequest().OmittedRoots, q.OmittedRoots) {
		t.Fatal("route adapter discarded semantic omission")
	}
	edits = []ReferenceEdit{{Action: "add", Target: metric}}
	if err := applyReferenceEdits(&q, edits); err != nil {
		t.Fatal(err)
	}
	retainSelectionOmissions(&q, edits)
	if len(q.OmittedRoots) != 0 {
		t.Fatal("explicit restoration left an active omission")
	}
	after, _ := json.Marshal(old)
	if string(before) != string(after) {
		t.Fatal("refinement mutated retained parent selection")
	}
}

func TestSQLRecoverySelectionOmissionsSurviveSavedRouting(t *testing.T) {
	omitted := semantics.Reference{Kind: semantics.KindDimension, ID: "region"}
	saved := SavedQuestion{Durability: "replayable", Context: "context", Question: "Revenue by region", Topics: []SavedTopic{{Topic: "topic"}}, Selections: &SavedSelections{OmittedRoots: []semantics.Reference{omitted}}}
	q := savedRouting(saved, nlq.LanguageEnglish)
	if !reflect.DeepEqual(q.OmittedRoots, saved.Selections.OmittedRoots) {
		t.Fatal("saved routing lost explicit removal")
	}
	q.OmittedRoots[0].ID = "changed"
	if saved.Selections.OmittedRoots[0] != omitted {
		t.Fatal("saved routing retained caller slice alias")
	}
	// Canonicalization must not mutate caller memory while sorting a detached value.
	q = QuestionRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "z"}, {Kind: semantics.KindDimension, ID: "a"}}, MetricIDs: []string{"z", "a"}, OmittedRoots: []semantics.Reference{{Kind: semantics.KindDimension, ID: "z"}, {Kind: semantics.KindDimension, ID: "a"}}}
	before, _ := json.Marshal(q)
	copy := q
	canonicalizeQuestion(&copy)
	after, _ := json.Marshal(q)
	if string(before) != string(after) {
		t.Fatal("canonical ordering mutated caller selection")
	}
}

func TestSQLRecoveryClarificationCannotChangeOmittedRoots(t *testing.T) {
	e := unitEnvelope(t)
	repo := newUnitRepository()
	omitted := semantics.Reference{Kind: semantics.KindDimension, ID: "region"}
	pending := unitQuery(e, "pending-omission", "topic", "v1", "context", false)
	pending.Status, pending.SQL = "preflight", ""
	pending.Route.AnswerContext = "answer-context"
	pending.Route.Request = nlqroute.RouteRequest{Question: "Revenue", Locale: nlq.LanguageEnglish, Context: "context", Topics: []string{"topic"}, OmittedRoots: []semantics.Reference{omitted}}
	repo.queries[pending.ID] = pending
	service := &Service{repo: repo}
	input := QuestionRequest{Question: "Revenue", Locale: nlq.LanguageEnglish, Context: "context", Topics: []string{"topic"}, ClarificationQuery: pending.ID, AnswerContext: "answer-context", Answers: []semantics.ClarificationAnswer{{Topic: "topic"}}, OmittedRoots: []semantics.Reference{omitted}}
	if err := service.validateClarificationOrigin(context.Background(), e, input, "query.plan"); err != nil {
		t.Fatal("same preflight omission rejected", err)
	}
	input.OmittedRoots = nil
	if err := service.validateClarificationOrigin(context.Background(), e, input, "query.plan"); err == nil {
		t.Fatal("answer reused a preflight while changing the analytical interpretation")
	}
}
