package nlqexec

import (
	"errors"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"testing"
)

func TestSQLRecoveryGroundedGroupingRunAndSavedPolicy(t *testing.T) {
	r := &groundedReplayProbe{failure: exec.ErrBinding}
	s := Service{router: r}
	q := QueryRecord{Route: nlqroute.RouteResult{GroupingIntent: &nlqroute.GroupingIntentEvidence{}, Request: nlqroute.RouteRequest{GroupingIntentPolicy: nlqroute.GroundedGroupingIntentPolicy}}}
	for i := 0; i < 4; i++ {
		if i == 1 {
			q.Route.GroupingIntent = nil
		}
		if i == 2 {
			q.Route.Request.GroupingIntentPolicy = ""
			q.Route.Selection = &nlqroute.SemanticSelection{GroupingIntent: "retained-origin"}
		}
		if i == 3 {
			q.Route.Selection = &nlqroute.SemanticSelection{Topics: []nlqroute.SelectedTopic{{Roots: []nlqroute.SelectedRoot{{Reason: "grounded_grouping"}}}}}
		}
		if err := s.verifyQueryClarificationBinding(t.Context(), testEnvelope(t), q, admission{}); !errors.Is(err, exec.ErrBinding) || r.calls != i+1 {
			t.Fatal("model origin bypassed replay", err)
		}
	}
	saved := SavedQuestion{Context: "context", Question: "Revenue by month", Durability: "replayable", Topics: []SavedTopic{{Topic: "topic", Version: "v1"}}, Selections: &SavedSelections{GroupingIntentPolicy: nlqroute.GroundedGroupingIntentPolicy}}
	input := savedRouting(saved, nlq.LanguageEnglish)
	q = QueryRecord{Question: input.Question, Route: nlqroute.RouteResult{Request: input.routeRequest(), GroupingIntent: &nlqroute.GroupingIntentEvidence{}}}
	q.Route.Request.Grouping = &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: "topic", Dimension: "date", Grain: "month"}}}
	if !savedSelectionsMatch(q, saved) {
		t.Fatal("derived grouping confused with saved input")
	}
	base := refinementQuestion(q, QuestionRequest{})
	canonicalizeQuestion(&base)
	if base.Grouping == nil || base.GroupingIntentPolicy != "" {
		t.Fatal("unchanged refinement did not pin explicit grouping")
	}
	changed := refinementQuestion(q, QuestionRequest{Question: "Revenue by quarter"})
	canonicalizeQuestion(&changed)
	if changed.Grouping != nil || changed.GroupingIntentPolicy != nlqroute.GroundedGroupingIntentPolicy {
		t.Fatal("changed language reused old intent")
	}
	manual := refinementQuestion(q, QuestionRequest{Grouping: &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy}})
	if manual.GroupingIntentPolicy != "" || manual.Grouping == nil {
		t.Fatal("manual refinement invoked inference")
	}
	saved.Selections.GroupingIntentPolicy = ""
	if savedSelectionsMatch(q, saved) {
		t.Fatal("saved policy silently removed")
	}
	input.GroupingIntentPolicy = "unknown"
	if validateQuestion(input) == nil {
		t.Fatal("unsupported policy accepted")
	}
}
