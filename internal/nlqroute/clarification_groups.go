package nlqroute

import (
	"sort"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

type clarificationPhysicalField struct{ Dataset, Column, Aggregation string }
type clarificationPhysicalGroup struct {
	Effects     []semantics.ClarificationEffect
	Questions   []semantics.ClarificationSlotOutcome
	Resolutions []semantics.ClarificationResolution
}

// Cross-topic policies are composed by exact live physical mapping, never by
// display label or independently budgeted topic groups. Aggregate and row-level
// predicates stay distinct. Conflicts precede provider work and plan issuance.
func crossTopicClarificationConflict(admitted []admittedTopic, locale nlq.Language) error {
	groups := map[clarificationPhysicalField]*clarificationPhysicalGroup{}
	for _, item := range admitted {
		if item.clarifications == nil {
			continue
		}
		for _, slot := range item.clarifications.Slots {
			if slot.Effect == nil || slot.Outcome == semantics.ClarificationNotApplicable || !slot.Required && !slot.Defaulted && slot.Outcome != semantics.ClarificationSatisfied {
				continue
			}
			var resolved *semantics.ClarificationResolution
			for i := range item.clarifications.Resolutions {
				r := &item.clarifications.Resolutions[i]
				if r.Pattern == slot.Pattern && r.Slot == slot.Slot {
					resolved = r
					break
				}
			}
			fragment := semantics.ClarificationResolution{Effect: slot.Effect}
			if resolved != nil {
				fragment = *resolved
			}
			mapped, err := businessConstraint(item, fragment)
			if err != nil {
				return err
			}
			key := clarificationPhysicalField{mapped.Dataset, mapped.Column, mapped.Aggregation}
			group := groups[key]
			if group == nil {
				group = &clarificationPhysicalGroup{}
				groups[key] = group
			}
			group.Effects = append(group.Effects, *slot.Effect)
			group.Questions = append(group.Questions, slot)
			if resolved != nil {
				group.Resolutions = append(group.Resolutions, *resolved)
			}
		}
	}
	failure := &Clarification{Reason: "conflicting_policies", Outcome: semantics.ClarificationConflicting}
	for _, group := range groups {
		if !semantics.ClarificationEffectsConflict(group.Effects) && !semantics.ClarificationResolutionGroupConflict(group.Resolutions) {
			continue
		}
		for _, question := range group.Questions {
			question.Outcome, question.Reason = semantics.ClarificationConflicting, "conflicting_policies"
			failure.Questions = append(failure.Questions, question)
		}
	}
	if len(failure.Questions) == 0 {
		return nil
	}
	sortClarificationQuestions(failure.Questions)
	for _, q := range failure.Questions {
		message := "Reviewed policies impose incompatible constraints on the same field. Resolve the policy conflict before continuing."
		if locale == nlq.LanguageSpanish {
			message = "Las políticas revisadas imponen restricciones incompatibles sobre el mismo campo. Resolvé el conflicto antes de continuar."
		}
		failure.Errors = append(failure.Errors, semantics.ClarificationFieldError{Topic: q.Topic, Pattern: q.Pattern, Slot: q.Slot, Field: "policy", Code: "conflicting_policies", Message: message})
	}
	return failure
}

func sortClarificationQuestions(questions []semantics.ClarificationSlotOutcome) {
	sort.SliceStable(questions, func(i, j int) bool {
		a, b := questions[i], questions[j]
		if a.Required != b.Required {
			return a.Required
		}
		if a.Specificity != b.Specificity {
			return a.Specificity > b.Specificity
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.Topic != b.Topic {
			return a.Topic < b.Topic
		}
		if a.Pattern != b.Pattern {
			return a.Pattern < b.Pattern
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.Slot < b.Slot
	})
}
