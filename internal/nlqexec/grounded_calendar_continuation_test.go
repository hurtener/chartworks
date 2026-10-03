package nlqexec

import (
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"testing"
)

func TestGroundedCalendarPolicyRetainedAfterEmptyRefine(t *testing.T) {
	parent := QueryRecord{Route: nlqroute.RouteResult{Request: nlqroute.RouteRequest{InterpretationPolicy: nlqroute.GroundedCalendarPolicy}, Interpretation: &nlqroute.Interpretation{Parser: "deterministic-grounded-calendar-v2", Anchor: "2026-10-01"}}}
	question := QuestionRequest{}
	retainInferredInterpretation(parent, QuestionRequest{}, &question)
	if question.InterpretationPolicy != nlqroute.GroundedCalendarPolicy || question.InterpretationAnchor != "2026-10-01" {
		t.Fatal("empty refine downgraded parser")
	}
	explicit := QuestionRequest{InterpretationPolicy: nlqroute.InterpretationContinuationPolicy}
	question = explicit
	retainInferredInterpretation(parent, explicit, &question)
	if question.InterpretationPolicy != explicit.InterpretationPolicy {
		t.Fatal("explicit policy choice ignored")
	}
	saved := SavedQuestion{Selections: &SavedSelections{InterpretationPolicy: nlqroute.GroundedCalendarPolicy, InterpretationAnchor: "2026-10-01"}}
	if savedRouting(saved, nlq.LanguageEnglish).InterpretationPolicy != nlqroute.GroundedCalendarPolicy {
		t.Fatal("saved policy lost")
	}
}
