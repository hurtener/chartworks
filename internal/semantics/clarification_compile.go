package semantics

import (
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

func validateClarificationShape(p RuleSetDefinition) error {
	for i, pattern := range p.Patterns {
		path := "patterns[" + itoa(i) + "]"
		if pattern.Policy == nil {
			// Retained legacy bytes remain readable and digest-stable. New
			// effects cannot accidentally activate without a reviewed policy.
			for _, slot := range pattern.Slots {
				if slot.Effect != nil || slot.Default != nil || len(slot.DependsOn) != 0 {
					return invalid(CodeInvalidValue, path+".policy")
				}
			}
			continue
		}
		policy := pattern.Policy
		if policy.SchemaVersion != ClarificationSchemaVersion || policy.Priority < -1000 || policy.Priority > 1000 || !validLine(policy.Why, 512) || (policy.WhySpanish != "" && !validLine(policy.WhySpanish, 512)) {
			return invalid(CodeInvalidValue, path+".policy")
		}
		if len(policy.When.AnyTerms) > 32 || len(policy.When.AnyReferences) > 32 || (!policy.Disabled && len(policy.When.AnyTerms)+len(policy.When.AnyReferences) == 0) {
			return invalid(CodeLimit, path+".policy.when")
		}
		terms := map[string]bool{}
		for _, term := range policy.When.AnyTerms {
			normalized := clarificationTokens(term)
			if !validLine(term, 128) || normalized == "  " || terms[normalized] || len(strings.Fields(normalized)) > 8 {
				return invalid(CodeInvalidValue, path+".policy.when.any_terms")
			}
			terms[normalized] = true
		}
		refs := map[Reference]bool{}
		for _, ref := range policy.When.AnyReferences {
			if !ref.Valid() || refs[ref] {
				return invalid(CodeInvalidReference, path+".policy.when.any_references")
			}
			refs[ref] = true
		}
		for j, slot := range pattern.Slots {
			slotPath := path + ".slots[" + itoa(j) + "]"
			if slot.PromptES != "" && !validLine(slot.PromptES, 1024) {
				return invalid(CodeInvalidValue, slotPath+".prompt_es")
			}
			if len(slot.DependsOn) > 16 || (slot.Required && slot.Default != nil) {
				return invalid(CodeInvalidValue, slotPath+".default")
			}
			if slot.Kind == SlotChoice {
				if slot.Effect != nil {
					return invalid(CodeInvalidValue, slotPath+".effect")
				}
				for _, choice := range slot.Choices {
					if choice.Target == nil || !choice.Target.Valid() || (choice.LabelES != "" && !validLine(choice.LabelES, 256)) {
						return invalid(CodeInvalidReference, slotPath+".choices")
					}
				}
			} else if err := validateClarificationEffect(slot, slotPath); err != nil {
				return err
			}
			if slot.Default != nil {
				if _, err := ResolveClarificationValue(slot, *slot.Default, "en"); err != nil {
					return invalid(CodeInvalidValue, slotPath+".default")
				}
			}
		}
		if _, err := orderedClarificationSlots(pattern.Slots); err != nil {
			return invalid(CodeReferenceCycle, path+".slots.depends_on")
		}
	}
	return nil
}

func validateClarificationEffect(slot ClarificationSlot, path string) error {
	e := slot.Effect
	if e == nil || !e.Target.Valid() || (e.Target.Kind != KindColumn && e.Target.Kind != KindMeasure && e.Target.Kind != KindDimension) {
		return invalid(CodeInvalidReference, path+".effect.target")
	}
	if e.Nulls != "exclude" && e.Nulls != "include" && e.Nulls != "only" {
		return invalid(CodeInvalidValue, path+".effect.nulls")
	}
	if len(e.Grains) > 5 || len(e.Values) > 64 {
		return invalid(CodeLimit, path+".effect")
	}
	switch slot.Kind {
	case SlotDate:
		if e.Kind != "time_window" || e.Operator != "range" || e.Bounds != "[)" || e.Calendar != "gregorian" || len(e.TimeZone) == 0 || len(e.TimeZone) > 128 || len(e.Grains) == 0 || (e.TemporalType != "date" && e.TemporalType != "timestamp" && e.TemporalType != "timestamptz") {
			return invalid(CodeInvalidValue, path+".effect.time")
		}
		if _, err := time.LoadLocation(e.TimeZone); err != nil {
			return invalid(CodeInvalidValue, path+".effect.time_zone")
		}
		seen := map[string]bool{}
		for _, grain := range e.Grains {
			if seen[grain] || !containsClarificationString([]string{"day", "week", "month", "quarter", "year"}, grain) {
				return invalid(CodeInvalidValue, path+".effect.grains")
			}
			seen[grain] = true
		}
		if e.Unit != "" || e.Precision != 0 || e.Scale != 0 || e.MaxLength != 0 || len(e.Values) != 0 {
			return invalid(CodeInvalidValue, path+".effect")
		}
	case SlotNumber:
		if e.Kind != "number" || !containsClarificationString([]string{"eq", "ne", "gt", "gte", "lt", "lte", "range"}, e.Operator) || !validLine(e.Unit, 64) || e.Precision < 1 || e.Precision > 76 || e.Scale < 0 || e.Scale > e.Precision || e.Scale > 38 {
			return invalid(CodeInvalidValue, path+".effect.number")
		}
		if e.Operator == "range" {
			if !containsClarificationString([]string{"[]", "[)", "(]", "()"}, e.Bounds) {
				return invalid(CodeInvalidValue, path+".effect.bounds")
			}
		} else if e.Bounds != "" {
			return invalid(CodeInvalidValue, path+".effect.bounds")
		}
		if e.Calendar != "" || e.TimeZone != "" || e.TemporalType != "" || len(e.Grains) != 0 || e.MaxLength != 0 || len(e.Values) != 0 {
			return invalid(CodeInvalidValue, path+".effect")
		}
	case SlotBoolean:
		if e.Kind != "boolean" || e.Operator != "eq" || e.Unit != "" || e.Precision != 0 || e.Scale != 0 || e.Bounds != "" || e.Calendar != "" || e.TimeZone != "" || e.TemporalType != "" || len(e.Grains) != 0 || e.MaxLength != 0 || len(e.Values) != 0 {
			return invalid(CodeInvalidValue, path+".effect.boolean")
		}
	case SlotText:
		if (e.Kind != "entity" && e.Kind != "text") || (e.Operator != "eq" && e.Operator != "ne") || e.MaxLength < 1 || e.MaxLength > 512 || len(e.Values) == 0 || e.Unit != "" || e.Precision != 0 || e.Scale != 0 || e.Bounds != "" || e.Calendar != "" || e.TimeZone != "" || e.TemporalType != "" || len(e.Grains) != 0 {
			return invalid(CodeInvalidValue, path+".effect.text")
		}
		seen := map[string]string{}
		canonical := map[string]bool{}
		for _, value := range e.Values {
			if !validClarificationText(value.Canonical, e.MaxLength) || canonical[value.Canonical] || !validLine(value.Label, 256) || (value.LabelES != "" && !validLine(value.LabelES, 256)) || len(value.Aliases) > 16 {
				return invalid(CodeInvalidValue, path+".effect.values")
			}
			canonical[value.Canonical] = true
			for _, spelling := range append([]string{value.Canonical}, value.Aliases...) {
				if !validClarificationText(spelling, e.MaxLength) {
					return invalid(CodeInvalidValue, path+".effect.values.aliases")
				}
				key := normalizeClarificationTerm(spelling)
				if previous, ok := seen[key]; ok && previous != value.Canonical {
					return invalid(CodeAmbiguousTerm, path+".effect.values.aliases")
				}
				seen[key] = value.Canonical
			}
		}
	default:
		return invalid(CodeInvalidValue, path+".kind")
	}
	return nil
}

func validateClarificationReferences(subject RuleSubject, p RuleSetDefinition) error {
	for i, pattern := range p.Patterns {
		if pattern.Policy == nil {
			continue
		}
		path := "patterns[" + itoa(i) + "]"
		for _, ref := range pattern.Policy.When.AnyReferences {
			if _, ok := subject.refs[ref.key()]; !ok || !hasReference(pattern.Targets, ref) {
				return invalid(CodeMissingReference, path+".policy.when")
			}
		}
		for j, slot := range pattern.Slots {
			if slot.Effect == nil {
				continue
			}
			ref := slot.Effect.Target
			if _, ok := subject.refs[ref.key()]; !ok || !hasReference(pattern.Targets, ref) {
				return invalid(CodeMissingReference, path+".slots["+itoa(j)+"].effect.target")
			}
			if ref.Kind != KindColumn {
				deps := subject.graph[ref]
				if len(deps) != 1 || deps[0].Kind != KindColumn {
					return invalid(CodeInvalidReference, path+".slots["+itoa(j)+"].effect.target")
				}
			}
		}
	}
	return nil
}

func orderedClarificationSlots(slots []ClarificationSlot) ([]ClarificationSlot, error) {
	slots = clarificationPresentationOrder(slots)
	known := map[string]bool{}
	for _, slot := range slots {
		if !identity.Identifier(slot.ID) || known[slot.ID] {
			return nil, invalid(CodeDuplicateID, "clarification.slots")
		}
		known[slot.ID] = true
	}
	for _, slot := range slots {
		seen := map[string]bool{}
		for _, dep := range slot.DependsOn {
			if !known[dep] || dep == slot.ID || seen[dep] {
				return nil, invalid(CodeInvalidReference, "clarification.depends_on")
			}
			seen[dep] = true
		}
	}
	out := make([]ClarificationSlot, 0, len(slots))
	done := map[string]bool{}
	for len(out) < len(slots) {
		progress := false
		for _, slot := range slots {
			if done[slot.ID] {
				continue
			}
			ready := true
			for _, dep := range slot.DependsOn {
				ready = ready && done[dep]
			}
			if ready {
				out = append(out, slot)
				done[slot.ID], progress = true, true
			}
		}
		if !progress {
			return nil, invalid(CodeReferenceCycle, "clarification.depends_on")
		}
	}
	return out, nil
}

func cloneClarificationPolicy(policy *ClarificationPolicy) *ClarificationPolicy {
	if policy == nil {
		return nil
	}
	copy := *policy
	copy.When.AnyTerms = append([]string(nil), policy.When.AnyTerms...)
	copy.When.AnyReferences = append([]Reference(nil), policy.When.AnyReferences...)
	return &copy
}

// ClarificationMigrationDisposition is explicit in preview/export metadata.
// Legacy patterns retain their old digest and accept only exact reference choices
// until an author adds a conditional policy and obtains a fresh review.
func ClarificationMigrationDisposition(pattern ClarificationPattern) string {
	if pattern.Policy == nil {
		return "legacy_reference_only_review_required"
	}
	if pattern.Policy.Disabled {
		return "reviewed_disabled"
	}
	return "conditional_v1"
}
