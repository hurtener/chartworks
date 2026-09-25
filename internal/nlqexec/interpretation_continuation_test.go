package nlqexec

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestSQLRecoveryRefinementRetainsResolvedInterpretation(t *testing.T) {
	p := QueryRecord{Question: "Revenue north and not south last month", Parameters: []exec.Parameter{{Kind: "text", Value: "private-not-a-selection"}}, Route: nlqroute.RouteResult{Interpretation: &nlqroute.Interpretation{Anchor: "2026-09-22", Values: []nlqroute.ValueInterpretation{{Topic: "topic", Dimension: "region", GovernedValue: "north", Operator: "eq"}, {Topic: "topic", Dimension: "region", GovernedValue: "south", Operator: "ne"}}, Temporal: []nlqroute.TemporalInterpretation{{Topic: "topic", Dimension: "date", Grain: "month", LocalStart: "2026-08-01", LocalEnd: "2026-09-01"}}}}}
	before, _ := json.Marshal(p)
	q := refinementQuestion(p, QuestionRequest{Question: "Now show rows"})
	retainInferredInterpretation(p, QuestionRequest{Question: "Now show rows"}, &q)
	if len(q.InterpretationSelections) != 3 || q.InterpretationAnchor != "2026-09-22" {
		t.Fatal("lost same-dimension values or anchor")
	}
	if !reflect.DeepEqual(q.routeRequest().InterpretationSelections, q.InterpretationSelections) {
		t.Fatal("route adapter dropped resolved state")
	}
	q.InterpretationSelections[0].Value = "changed"
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("parent mutated")
	}
	override := QuestionRequest{InterpretationSelections: []nlqroute.InterpretationSelection{{Topic: "topic", Dimension: "region", Value: "east", Operator: "eq"}}}
	retainInferredInterpretation(p, override, &q)
	if len(q.InterpretationSelections) != 2 || q.InterpretationSelections[0].Value != "east" {
		t.Fatal("explicit dimension replacement kept stale choices")
	}
}
func TestSQLRecoveryRefinementDoesNotReapplyObsoleteInterpretationEdits(t *testing.T) {
	p := QueryRecord{Question: "North", Route: nlqroute.RouteResult{Request: nlqroute.RouteRequest{InterpretationEdits: []nlqroute.InterpretationEdit{{Target: "topic:region:north", Action: "remove"}}}, Interpretation: &nlqroute.Interpretation{Anchor: "2026-09-22"}}}
	delta := QuestionRequest{Question: "North again"}
	q := refinementQuestion(p, delta)
	retainInferredInterpretation(p, delta, &q)
	if len(q.InterpretationEdits) != 0 {
		t.Fatal("old removal suppressed a new explicit question")
	}
	q = refinementQuestion(p, QuestionRequest{})
	retainInferredInterpretation(p, QuestionRequest{}, &q)
	if len(q.InterpretationEdits) != 1 {
		t.Fatal("same-question continuation resurrected removed inference")
	}
}
func TestSQLRecoverySavedInterpretationSelectionsAreExplicit(t *testing.T) {
	selected := &SavedSelections{InterpretationAnchor: "2026-09-22", InterpretationSelections: []nlqroute.InterpretationSelection{{Topic: "topic", Dimension: "date", Period: &nlqroute.InterpretationPeriod{Start: "2026-01-01", End: "2027-01-01", Grain: "year"}}}}
	q := savedRouting(SavedQuestion{Question: "Records", Context: "ctx", Topics: []SavedTopic{{Topic: "topic"}}, Selections: selected}, nlq.LanguageEnglish)
	if q.InterpretationAnchor != "2026-09-22" || len(q.InterpretationSelections) != 1 {
		t.Fatal("saved reconstruction lost state")
	}
	q.InterpretationSelections[0].Period.Start = "changed"
	if selected.InterpretationSelections[0].Period.Start != "2026-01-01" {
		t.Fatal("saved state alias")
	}
}

func TestSQLRecoveryContinuationCannotBorrowClarificationOrigin(t *testing.T) {
	e := unitEnvelope(t)
	repo := newUnitRepository()
	pending := unitQuery(e, "pending-continuation", "topic", "v1", "context", false)
	pending.Status, pending.SQL = "preflight", ""
	pending.Route.AnswerContext = "answer-context"
	selection := []nlqroute.InterpretationSelection{{Topic: "topic", Dimension: "date", Period: &nlqroute.InterpretationPeriod{Start: "2026-01-01", End: "2026-04-01", Grain: "quarter"}}}
	pending.Route.Request = nlqroute.RouteRequest{Question: "Revenue", Locale: nlq.LanguageEnglish, Context: "context", Topics: []string{"topic"}, InterpretationAnchor: "2026-09-22", InterpretationSelections: selection}
	repo.queries[pending.ID] = pending
	service := &Service{repo: repo}
	input := QuestionRequest{Question: "Revenue", Locale: nlq.LanguageEnglish, Context: "context", Topics: []string{"topic"}, ClarificationQuery: pending.ID, AnswerContext: "answer-context", Answers: []semantics.ClarificationAnswer{{Topic: "topic"}}, InterpretationAnchor: "2026-09-22", InterpretationSelections: nlqroute.CloneInterpretationSelections(selection)}
	if err := service.validateClarificationOrigin(context.Background(), e, input, "query.plan"); err != nil {
		t.Fatal("same pending interpretation rejected", err)
	}
	for _, change := range []func(*QuestionRequest){
		func(q *QuestionRequest) { q.InterpretationSelections = nil },
		func(q *QuestionRequest) {
			q.InterpretationSelections[0].Period.Start = "2026-04-01"
			q.InterpretationSelections[0].Period.End = "2026-07-01"
		},
		func(q *QuestionRequest) { q.InterpretationAnchor = "2026-12-22" },
		func(q *QuestionRequest) { q.InterpretationAnchor = "" },
		func(q *QuestionRequest) {
			q.InterpretationEdits = []nlqroute.InterpretationEdit{{Target: "topic:date:time", Action: "remove"}}
		},
	} {
		altered := input
		altered.InterpretationSelections = nlqroute.CloneInterpretationSelections(input.InterpretationSelections)
		change(&altered)
		if err := service.validateClarificationOrigin(context.Background(), e, altered, "query.plan"); err == nil {
			t.Fatal("changed intent reused pending question proof")
		}
	}
}

func TestSQLRecoveryContinuationConcurrentCopies(t *testing.T) {
	parent := QueryRecord{Question: "Revenue in north", Route: nlqroute.RouteResult{Interpretation: &nlqroute.Interpretation{Anchor: "2026-09-22", Values: []nlqroute.ValueInterpretation{{Topic: "topic", Dimension: "region", GovernedValue: "north", Operator: "eq"}}, Temporal: []nlqroute.TemporalInterpretation{{Topic: "topic", Dimension: "date", LocalStart: "2026-01-01", LocalEnd: "2026-04-01", Grain: "quarter"}}}}}
	before := exec.Hash(parent)
	results := make(chan bool, 8)
	for i := 0; i < cap(results); i++ {
		go func() {
			q := QuestionRequest{}
			retainInferredInterpretation(parent, QuestionRequest{Question: "Next"}, &q)
			valid := len(q.InterpretationSelections) == 2 && q.InterpretationAnchor == "2026-09-22"
			for j := range q.InterpretationSelections {
				if q.InterpretationSelections[j].Period != nil {
					q.InterpretationSelections[j].Period.Start = "changed"
				}
			}
			results <- valid
		}()
	}
	for i := 0; i < cap(results); i++ {
		if !<-results {
			t.Error("concurrent intent changed")
		}
	}
	if exec.Hash(parent) != before {
		t.Fatal("shared parent mutated")
	}
}

func TestSQLRecoverySavedInterpretationAnchorPolicy(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		for _, seeded := range []bool{false, true} {
			in := savedSelectionFixture()
			in.Selections = &SavedSelections{}
			if pinned {
				in.Selections.InterpretationAnchor = "2026-09-22"
			}
			if seeded {
				in.Selections.InterpretationSelections = []nlqroute.InterpretationSelection{{Topic: "sales", Dimension: "date", Period: &nlqroute.InterpretationPeriod{Start: "2025-03-01", End: "2025-04-01", Grain: "month"}}}
			}
			q := QueryRecord{Context: in.Context, Topics: []string{"sales"}, TopicVersions: []string{"v1"}, SQL: "SELECT amount FROM analytics.sales"}
			q.Route.Request = savedRouting(in, nlq.LanguageEnglish).routeRequest()
			q.Route.Request.InterpretationAnchor = "2026-09-22" // Server-populated anchor.
			if !savedRecordMatches(q, in) {
				t.Fatal("saved anchor semantics rejected exact routing", pinned, seeded)
			}
			q.Route.Request.InterpretationAnchor = "2026-10-22"
			if savedRecordMatches(q, in) == pinned {
				t.Fatal("explicit anchor and server default were conflated", pinned, seeded)
			}
		}
	}
}
