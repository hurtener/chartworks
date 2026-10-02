package nlqexec

import "github.com/hurtener/chartworks/internal/nlqroute"

// retainInferredInterpretation runs after the parent has been read under the
// signed actor/session, its current publication/source admission and authenticated
// business replay. Carry only semantic choices and exact civil intervals, never
// private scalar SQL bindings, old SQL predicates or authority from prior prose.
func retainInferredInterpretation(parent QueryRecord, delta QuestionRequest, question *QuestionRequest) {
	inherited := nlqroute.RetainedInterpretationSelections(parent.Route.Interpretation)
	// Explicit typed selections for a dimension replace that dimension as a unit;
	// current question interpretation can independently supersede it during Route.
	selected := nlqroute.CloneInterpretationSelections(delta.InterpretationSelections)
	for _, old := range inherited {
		replaced := false
		for _, fresh := range delta.InterpretationSelections {
			if old.Topic == fresh.Topic && old.Dimension == fresh.Dimension {
				replaced = true
			}
		}
		if !replaced {
			selected = append(selected, old)
		}
	}
	question.InterpretationSelections = selected
	if parent.Route.Interpretation != nil && delta.InterpretationPolicy == "" {
		question.InterpretationPolicy = nlqroute.InterpretationContinuationPolicy
	}
	if parent.Route.Interpretation != nil && delta.InterpretationAnchor == "" {
		question.InterpretationAnchor = parent.Route.Interpretation.Anchor
	}
	// Prior directives were applied to the prior utterance. Its resolved result
	// is the new baseline; replaying old remove/replace edits against new words
	// could suppress an intentional later re-selection of the same value.
	if delta.Question != "" && delta.Question != parent.Question {
		question.InterpretationEdits = nlqroute.CloneInterpretationEdits(delta.InterpretationEdits)
	}
}
