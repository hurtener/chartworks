package nlqexec

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

func TestRefinementSelectionEditsAreExplicitAndLossless(t *testing.T) {
	revenue := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	region := semantics.Reference{Kind: semantics.KindDimension, ID: "region"}
	month := semantics.Reference{Kind: semantics.KindDimension, ID: "month"}
	margin := semantics.Reference{Kind: semantics.KindMeasure, ID: "margin"}
	q := QuestionRequest{References: []semantics.Reference{revenue, region}, MetricIDs: []string{"revenue"}}

	if err := applyReferenceEdits(&q, []ReferenceEdit{{Action: "add", Target: month}, {Action: "replace", Target: revenue, Replacement: &margin}, {Action: "remove", Target: region}}); err != nil {
		t.Fatal(err)
	}
	if err := applyMetricEdits(&q, []MetricEdit{{Action: "replace", Target: "revenue", Replacement: "margin"}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(q.References, []semantics.Reference{margin, month}) || !reflect.DeepEqual(q.MetricIDs, []string{"margin"}) {
		t.Fatalf("continuity delta retained stale intent: %#v", q)
	}
	if err := applyMetricEdits(&q, []MetricEdit{{Action: "remove", Target: "margin"}}); err != nil || len(q.MetricIDs) != 0 {
		t.Fatalf("metric removal failed closed: %#v %v", q.MetricIDs, err)
	}
}

func TestRefinementSelectionEditsRejectAmbiguousOrForeignTargets(t *testing.T) {
	revenue := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	margin := semantics.Reference{Kind: semantics.KindMeasure, ID: "margin"}
	tests := []struct {
		name    string
		refs    []ReferenceEdit
		metrics []MetricEdit
	}{
		{name: "remove absent reference", refs: []ReferenceEdit{{Action: "remove", Target: margin}}},
		{name: "replace absent metric", metrics: []MetricEdit{{Action: "replace", Target: "margin", Replacement: "profit"}}},
		{name: "duplicate target", refs: []ReferenceEdit{{Action: "remove", Target: revenue}, {Action: "add", Target: revenue}}},
		{name: "unknown action", metrics: []MetricEdit{{Action: "merge", Target: "revenue"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := QuestionRequest{References: []semantics.Reference{revenue}, MetricIDs: []string{"revenue"}}
			if err := applyReferenceEdits(&q, tc.refs); len(tc.refs) > 0 && !errors.Is(err, ErrInvalid) {
				t.Fatalf("reference edit returned %v", err)
			}
			if err := applyMetricEdits(&q, tc.metrics); len(tc.metrics) > 0 && !errors.Is(err, ErrInvalid) {
				t.Fatalf("metric edit returned %v", err)
			}
		})
	}
}

func TestRefinementMetricReferenceCoherenceFailsClosed(t *testing.T) {
	revenue := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	margin := semantics.Reference{Kind: semantics.KindMeasure, ID: "margin"}
	q := QuestionRequest{References: []semantics.Reference{revenue}, MetricIDs: []string{"revenue"}}
	if err := applyReferenceEdits(&q, []ReferenceEdit{{Action: "replace", Target: revenue, Replacement: &margin}}); err != nil {
		t.Fatal(err)
	}
	canonicalizeQuestion(&q)
	if err := validateMetricReferenceCoherence(q, []ReferenceEdit{{Action: "replace", Target: revenue, Replacement: &margin}}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unpaired semantic replacement returned %v", err)
	}
	if err := applyMetricEdits(&q, []MetricEdit{{Action: "replace", Target: "revenue", Replacement: "margin"}}); err != nil {
		t.Fatal(err)
	}
	canonicalizeQuestion(&q)
	if err := validateMetricReferenceCoherence(q, nil, []MetricEdit{{Action: "replace", Target: "revenue", Replacement: "margin"}}); err != nil {
		t.Fatalf("paired semantic replacement rejected: %v", err)
	}
}

func TestCanonicalSelectionsArePermutationEquivalent(t *testing.T) {
	a := QuestionRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "margin"}, {Kind: semantics.KindDimension, ID: "region"}}, MetricIDs: []string{"volume", "margin"}}
	b := QuestionRequest{References: []semantics.Reference{{Kind: semantics.KindDimension, ID: "region"}, {Kind: semantics.KindMeasure, ID: "margin"}}, MetricIDs: []string{"margin", "volume"}}
	canonicalizeQuestion(&a)
	canonicalizeQuestion(&b)
	if !reflect.DeepEqual(a.References, b.References) || !reflect.DeepEqual(a.MetricIDs, b.MetricIDs) || exec.Hash(a.routeRequest()) != exec.Hash(b.routeRequest()) {
		t.Fatalf("permutations produced different canonical routing inputs: %#v %#v", a, b)
	}
}

func TestChildCreationFencesExactObservedParent(t *testing.T) {
	e := unitEnvelope(t)
	repo := newUnitRepository()
	parent := unitQuery(e, "parent", "topic", "v1", "context", false)
	repo.queries[parent.ID] = parent
	child := unitQuery(e, "child", "topic", "v1", "context", false)
	child.Parent = parent.ID
	bindParentLineage(&child, &parent)
	mutated := parent
	mutated.Status = "executed"
	mutated.Revision++
	repo.queries[parent.ID] = mutated
	if err := repo.CreateQuery(context.Background(), mustScope(e), child); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale parent lineage committed: %v", err)
	}
	bindParentLineage(&child, &mutated)
	if err := repo.CreateQuery(context.Background(), mustScope(e), child); err != nil {
		t.Fatalf("current parent lineage rejected: %v", err)
	}
}

func TestLineageDigestIncludesProtectedParentState(t *testing.T) {
	e := unitEnvelope(t)
	parent := unitQuery(e, "parent", "topic", "v1", "context", false)
	parent.SQL = "SELECT 1"
	baseline := QueryLineageDigest(parent)
	parent.SQL = "SELECT 2"
	if QueryLineageDigest(parent) == baseline {
		t.Fatal("protected SQL mutation did not change lineage digest")
	}
	parent.SQL = "SELECT 1"
	parent.Parameters = []exec.Parameter{{Kind: "text", Value: "synthetic"}}
	if QueryLineageDigest(parent) == baseline {
		t.Fatal("protected parameter mutation did not change lineage digest")
	}
}

func TestLegacyOrderedPendingClarificationOrigin(t *testing.T) {
	e := unitEnvelope(t)
	repo := newUnitRepository()
	margin := semantics.Reference{Kind: semantics.KindMeasure, ID: "margin"}
	volume := semantics.Reference{Kind: semantics.KindMeasure, ID: "volume"}
	pending := unitQuery(e, "legacy-pending", "topic", "v1", "context", false)
	pending.Status = "preflight"
	pending.SQL = ""
	pending.Route.AnswerContext = "answer-context"
	pending.Route.Request = nlqroute.RouteRequest{
		Question: "What changed?", Locale: nlq.LanguageEnglish, Context: "context", Topics: []string{"topic"},
		References: []semantics.Reference{volume, margin}, MetricIDs: []string{"volume", "margin"},
	}
	wantReferences := append([]semantics.Reference(nil), pending.Route.Request.References...)
	wantMetrics := append([]string(nil), pending.Route.Request.MetricIDs...)
	repo.queries[pending.ID] = pending
	service := &Service{repo: repo}
	submission := QuestionRequest{
		ClarificationQuery: pending.ID, AnswerContext: "answer-context", Answers: []semantics.ClarificationAnswer{{Topic: "topic"}},
		Question: "What changed?", Locale: nlq.LanguageEnglish, Context: "context", Topics: []string{"topic"},
		References: []semantics.Reference{margin, volume}, MetricIDs: []string{"margin", "volume"},
	}
	if err := service.validateClarificationOrigin(context.Background(), e, submission, "query.plan"); err != nil {
		t.Fatalf("equivalent canonical clarification submission rejected: %v", err)
	}
	if !reflect.DeepEqual(wantReferences, repo.queries[pending.ID].Route.Request.References) || !reflect.DeepEqual(wantMetrics, repo.queries[pending.ID].Route.Request.MetricIDs) {
		t.Fatal("legacy retained request was mutated during comparison")
	}
	submission.MetricIDs = []string{"margin", "profit"}
	err := service.validateClarificationOrigin(context.Background(), e, submission, "query.plan")
	clarification, ok := err.(*nlqroute.Clarification)
	if !ok || clarification.Reason != "clarification_question_mismatch" {
		t.Fatalf("mismatched clarification submission returned %v", err)
	}
}

func TestRefinementInterpretationEditsReplacePriorFilterIntent(t *testing.T) {
	base := []nlqroute.InterpretationEdit{{Target: "topic:region:north", Action: "replace", Value: "south"}, {Target: "topic:month:2026-03", Action: "remove"}}
	delta := []nlqroute.InterpretationEdit{{Target: "topic:region:north", Action: "remove"}}
	got := mergeInterpretationEdits(base, delta)
	want := []nlqroute.InterpretationEdit{{Target: "topic:region:north", Action: "remove"}, {Target: "topic:month:2026-03", Action: "remove"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filter correction retained superseded intent: got %#v want %#v", got, want)
	}
}

func TestRefinementHistoryIsBoundedAndSessionScoped(t *testing.T) {
	e := unitEnvelope(t)
	repo := newUnitRepository()
	root := unitQuery(e, "root", "topic", "v1", "context", false)
	root.Route = nlqroute.RouteResult{Outcome: nlq.StrategySingleTopic, Topic: "topic", Topics: []string{"topic"}, TopicVersions: []string{"v1"}}
	repo.queries[root.ID] = root
	current := root
	for i := 1; i < MaxRefinementDepth; i++ {
		next := unitQuery(e, "node"+string(rune('a'+i)), "topic", "v1", "context", false)
		next.Parent = current.ID
		repo.queries[next.ID] = next
		current = next
	}
	s := &Service{repo: repo}
	if err := s.checkRefinementDepth(context.Background(), e, current); err != nil {
		t.Fatalf("maximum permitted lineage rejected: %v", err)
	}
	over := unitQuery(e, "overflow", "topic", "v1", "context", false)
	over.Parent = current.ID
	repo.queries[over.ID] = over
	if err := s.checkRefinementDepth(context.Background(), e, over); !errors.Is(err, ErrRefinementLimit) {
		t.Fatalf("unbounded lineage returned %v", err)
	}

	foreign := unitQuery(e, "foreign", "topic", "v1", "other-context", false)
	foreign.Parent = root.ID
	if err := s.checkRefinementDepth(context.Background(), e, foreign); !errors.Is(err, ErrForeignSession) {
		t.Fatalf("cross-context ancestry returned %v", err)
	}
}
