package nlqexec

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
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
