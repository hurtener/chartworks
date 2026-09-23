package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/identity"
)

// LegacyClarificationChoice preserves the old exact-reference choice input only.
// Non-reference values are rejected rather than treated as nonempty choices.
type LegacyClarificationChoice struct {
	Pattern string `json:"pattern,omitempty"`
	Slot    string `json:"slot"`
	Value   string `json:"value"`
}

// ClarificationReferenceSelection distinguishes selected roots from expression
// dependencies when auto-resolving a reviewed choice. Nil preserves legacy behavior.
type ClarificationReferenceSelection struct {
	References []Reference `json:"references"`
}

// ClarificationInput is pure interpretation input after topic admission. It has
// no identity, token, source credential, provider or executable matcher fields.
type ClarificationInput struct {
	Selection     *ClarificationReferenceSelection `json:"selection,omitempty"`
	Locale        string                           `json:"locale"`
	Question      string                           `json:"question"`
	References    []Reference                      `json:"references,omitempty"`
	Answers       []ClarificationAnswer            `json:"answers,omitempty"`
	LegacyChoices []LegacyClarificationChoice      `json:"choices,omitempty"`
}

type clarificationKey struct{ pattern, slot string }

type rankedClarificationPattern struct {
	pattern     ClarificationPattern
	active      bool
	specificity int
	required    bool
}

// ResolveClarifications evaluates one exact compiled ruleset without any I/O.
// Services remain responsible for admitting its current reviewed publication and
// complete source-context reach before calling this function.
func ResolveClarifications(model RuleModel, input ClarificationInput) ClarificationEvaluation {
	out := ClarificationEvaluation{SchemaVersion: ClarificationSchemaVersion, Outcome: ClarificationNotApplicable, Slots: []ClarificationSlotOutcome{}}
	definition := model.Definition()
	if model.Digest() == "" || (input.Locale != "en" && input.Locale != "es") || !validClarificationText(input.Question, 16<<10) || len(input.Answers)+len(input.LegacyChoices) > 64 || len(input.References) > 128 {
		out.Outcome = ClarificationInvalid
		out.Errors = []ClarificationFieldError{*clarificationError(input.Locale, "question", "invalid_union")}
		return out
	}
	for _, ref := range input.References {
		if !ref.Valid() {
			out.Outcome = ClarificationInvalid
			out.Errors = []ClarificationFieldError{*clarificationError(input.Locale, "references", "foreign_answer")}
			return out
		}
	}
	choiceReferences := input.References
	if input.Selection != nil {
		choiceReferences = input.Selection.References
		if len(choiceReferences) > 128 {
			out.Outcome = ClarificationInvalid
			out.Errors = []ClarificationFieldError{*clarificationError(input.Locale, "selection", "invalid_union")}
			return out
		}
		for _, ref := range choiceReferences {
			if !ref.Valid() || !hasReference(input.References, ref) {
				out.Outcome = ClarificationInvalid
				out.Errors = []ClarificationFieldError{*clarificationError(input.Locale, "selection", "foreign_answer")}
				return out
			}
		}
	}
	answers, origins, errors := prepareClarificationAnswers(definition, input)
	if len(errors) != 0 {
		out.Outcome, out.Errors = ClarificationInvalid, errors
		return out
	}
	patterns := make([]rankedClarificationPattern, 0, len(definition.Patterns))
	for _, pattern := range definition.Patterns {
		active, specificity := matchClarificationPattern(pattern, input.Question, input.References)
		required := false
		for _, slot := range pattern.Slots {
			required = required || slot.Required
		}
		patterns = append(patterns, rankedClarificationPattern{pattern: pattern, active: active, specificity: specificity, required: required})
	}
	sort.Slice(patterns, func(i, j int) bool {
		a, b := patterns[i], patterns[j]
		if a.active != b.active {
			return a.active
		}
		if a.required != b.required {
			return a.required
		}
		if a.specificity != b.specificity {
			return a.specificity > b.specificity
		}
		ap, bp := 0, 0
		if a.pattern.Policy != nil {
			ap = a.pattern.Policy.Priority
		}
		if b.pattern.Policy != nil {
			bp = b.pattern.Policy.Priority
		}
		if ap != bp {
			return ap > bp
		}
		return a.pattern.ID < b.pattern.ID
	})
	out.References = append([]Reference(nil), input.References...)
	invalidInput, missing, conflicting, applicable := false, false, false, false
	for _, ranked := range patterns {
		pattern := ranked.pattern
		out.Dispositions = append(out.Dispositions, pattern.ID+":"+ClarificationMigrationDisposition(pattern))
		slots, err := orderedClarificationSlots(pattern.Slots)
		if err != nil {
			out.Outcome = ClarificationInvalid
			out.Resolutions, out.References = nil, nil
			out.Errors = append(out.Errors, *clarificationError(input.Locale, "policy", "conflicting_policies"))
			return out
		}
		satisfied := map[string]bool{}
		for slotIndex, slot := range slots {
			key := clarificationKey{pattern.ID, slot.ID}
			answer, supplied := answers[key]
			state := clarificationSlotView(definition, pattern, slot, input.Locale)
			state.Specificity, state.Order = ranked.specificity, slotIndex
			if pattern.Policy != nil {
				state.Priority = pattern.Policy.Priority
			}
			active := ranked.active
			if pattern.Policy == nil {
				active = supplied && slot.Kind == SlotChoice
				state.Reason = "legacy_reference_only_review_required"
			} else if pattern.Policy.Disabled {
				state.Reason = "reviewed_disabled"
			}
			if !active {
				if supplied {
					field := clarificationError(input.Locale, "value", "not_applicable")
					annotateClarificationError(field, definition.Topic, pattern.ID, slot.ID)
					state.Outcome, state.Errors = ClarificationInvalid, []ClarificationFieldError{*field}
					out.Errors = append(out.Errors, *field)
					invalidInput = true
				}
				out.Slots = append(out.Slots, state)
				continue
			}
			applicable = true
			var value *ClarificationValue
			origin := "answer"
			if supplied {
				value, origin = answer.Value, origins[key]
			} else if slot.Kind == SlotChoice {
				for _, choice := range slot.Choices {
					if choice.Target == nil || !hasReference(choiceReferences, *choice.Target) {
						continue
					}
					if value != nil && value.OptionID != choice.ID {
						field := clarificationError(input.Locale, "references", "conflicting_policies")
						annotateClarificationError(field, definition.Topic, pattern.ID, slot.ID)
						state.Outcome, state.Errors = ClarificationConflicting, []ClarificationFieldError{*field}
						out.Errors = append(out.Errors, *field)
						conflicting = true
						break
					}
					value, origin = &ClarificationValue{OptionID: choice.ID}, "typed_interpretation"
				}
			}
			if state.Outcome == ClarificationConflicting {
				out.Slots = append(out.Slots, state)
				continue
			}
			dependenciesReady := true
			for _, dep := range slot.DependsOn {
				dependenciesReady = dependenciesReady && satisfied[dep]
			}
			if value == nil && dependenciesReady && slot.Default != nil {
				value, origin, state.Defaulted = slot.Default, "reviewed_default", true
			}
			if value == nil {
				state.Reason = "optional_omitted"
				if slot.Required {
					state.Outcome, state.Reason, missing = ClarificationMissing, "required_answer", true
					if !dependenciesReady {
						state.Reason, state.Prompt = "dependency_missing", ""
					}
				}
				out.Slots = append(out.Slots, state)
				continue
			}
			locale := input.Locale
			if origin == "reviewed_default" {
				locale = "en"
			}
			resolution, parseErr := ResolveClarificationValue(slot, *value, locale)
			if parseErr != nil {
				// A reviewed default is parsed in its canonical locale, but
				// every user-facing field error uses the request locale.
				field := clarificationError(input.Locale, parseErr.Field, parseErr.Code)
				annotateClarificationError(field, definition.Topic, pattern.ID, slot.ID)
				state.Outcome, state.Reason = ClarificationInvalid, field.Code
				if field.Code == "unresolved_value" {
					state.Outcome, missing = ClarificationMissing, true
				} else {
					invalidInput = true
				}
				state.Errors = []ClarificationFieldError{*field}
				out.Errors = append(out.Errors, *field)
				out.Slots = append(out.Slots, state)
				continue
			}
			if !dependenciesReady {
				state.Outcome, state.Reason, state.Prompt = ClarificationMissing, "dependency_missing", ""
				missing = true
				out.Slots = append(out.Slots, state)
				continue
			}
			resolution.Topic, resolution.TopicVersion, resolution.PackDigest = definition.Topic, definition.TopicVersion, definition.PackDigest
			resolution.RulesetVersion, resolution.RulesetDigest = definition.Version, model.Digest()
			resolution.Pattern, resolution.PatternVersion, resolution.Slot = pattern.ID, pattern.Version, slot.ID
			resolution.QuestionDigest, resolution.Provenance = clarificationHash([]byte(input.Question)), origin
			if resolution.Reference != nil {
				resolution.Value = value.OptionID
				if !hasReference(out.References, *resolution.Reference) {
					out.References = append(out.References, *resolution.Reference)
				}
			}
			resolution.ID = ClarificationResolutionDigest(resolution)
			state.Outcome, state.Reason = ClarificationSatisfied, origin
			satisfied[slot.ID] = true
			out.Resolutions = append(out.Resolutions, resolution)
			out.Slots = append(out.Slots, state)
		}
	}
	if invalidInput {
		out.Outcome, out.Resolutions, out.References = ClarificationInvalid, nil, nil
		return out
	}
	conflicts := clarificationConflicts(model, patterns, out.Resolutions)
	if len(conflicts) != 0 {
		conflicting = true
		for i := range out.Slots {
			state := &out.Slots[i]
			if !conflicts[clarificationKey{state.Pattern, state.Slot}] {
				continue
			}
			field := clarificationError(input.Locale, "policy", "conflicting_policies")
			annotateClarificationError(field, state.Topic, state.Pattern, state.Slot)
			state.Outcome, state.Reason = ClarificationConflicting, field.Code
			state.Errors = append(state.Errors, *field)
			out.Errors = append(out.Errors, *field)
		}
	}
	switch {
	case conflicting:
		out.Outcome, out.Resolutions, out.References = ClarificationConflicting, nil, nil
	case missing:
		out.Outcome = ClarificationMissing
	case applicable:
		out.Outcome = ClarificationSatisfied
	}
	return out
}

func prepareClarificationAnswers(definition RuleSetDefinition, input ClarificationInput) (map[clarificationKey]ClarificationAnswer, map[clarificationKey]string, []ClarificationFieldError) {
	answers := map[clarificationKey]ClarificationAnswer{}
	origins := map[clarificationKey]string{}
	patterns := map[string]ClarificationPattern{}
	known := map[clarificationKey]ClarificationSlot{}
	for _, pattern := range definition.Patterns {
		patterns[pattern.ID] = pattern
		for _, slot := range pattern.Slots {
			known[clarificationKey{pattern.ID, slot.ID}] = slot
		}
	}
	var errors []ClarificationFieldError
	addError := func(pattern, slot, field, code string) {
		err := clarificationError(input.Locale, field, code)
		if !identity.Identifier(pattern) {
			pattern = ""
		}
		if !identity.Identifier(slot) {
			slot = ""
		}
		annotateClarificationError(err, definition.Topic, pattern, slot)
		errors = append(errors, *err)
	}
	for _, answer := range input.Answers {
		key := clarificationKey{answer.Pattern, answer.Slot}
		pattern, exists := patterns[answer.Pattern]
		if answer.Topic != definition.Topic || !exists {
			addError(answer.Pattern, answer.Slot, "topic", "foreign_answer")
			continue
		}
		if _, ok := known[key]; !ok {
			addError(answer.Pattern, answer.Slot, "slot", "foreign_answer")
			continue
		}
		if answer.TopicVersion != definition.TopicVersion || answer.RulesetVersion != definition.Version || answer.PatternVersion != pattern.Version {
			addError(answer.Pattern, answer.Slot, "version", "stale_answer")
			continue
		}
		if answer.Remove || answer.Value == nil {
			addError(answer.Pattern, answer.Slot, "value", "invalid_union")
			continue
		}
		if _, exists := answers[key]; exists {
			addError(answer.Pattern, answer.Slot, "value", "duplicate_answer")
			continue
		}
		answer.Value = cloneClarificationValue(answer.Value)
		answers[key], origins[key] = answer, "answer"
	}
	for _, legacy := range input.LegacyChoices {
		key := clarificationKey{legacy.Pattern, legacy.Slot}
		if legacy.Pattern == "" {
			count := 0
			for candidate := range known {
				if candidate.slot == legacy.Slot {
					key, count = candidate, count+1
				}
			}
			if count != 1 {
				addError(legacy.Pattern, legacy.Slot, "pattern", "foreign_answer")
				continue
			}
		}
		slot, exists := known[key]
		if !exists {
			addError(legacy.Pattern, legacy.Slot, "slot", "foreign_answer")
			continue
		}
		if slot.Kind != SlotChoice {
			addError(key.pattern, key.slot, "value", "migration_required")
			continue
		}
		if _, exists := answers[key]; exists {
			addError(key.pattern, key.slot, "value", "duplicate_answer")
			continue
		}
		if !identity.Identifier(legacy.Value) {
			addError(key.pattern, key.slot, "option_id", "invalid_choice")
			continue
		}
		pattern := patterns[key.pattern]
		answers[key] = ClarificationAnswer{Topic: definition.Topic, TopicVersion: definition.TopicVersion, RulesetVersion: definition.Version, Pattern: key.pattern, PatternVersion: pattern.Version, Slot: key.slot, Value: &ClarificationValue{OptionID: legacy.Value}}
		origins[key] = "legacy_reference_choice"
	}
	return answers, origins, errors
}

func matchClarificationPattern(pattern ClarificationPattern, question string, refs []Reference) (bool, int) {
	if pattern.Policy == nil || pattern.Policy.Disabled {
		return false, 0
	}
	specificity := 0
	question = clarificationTokens(question)
	for _, term := range pattern.Policy.When.AnyTerms {
		if strings.Contains(question, clarificationTokens(term)) {
			score := len(strings.Fields(term))
			if score > specificity {
				specificity = score
			}
		}
	}
	for _, ref := range pattern.Policy.When.AnyReferences {
		if hasReference(refs, ref) {
			specificity += 100
		}
	}
	return specificity > 0, specificity
}

func clarificationSlotView(definition RuleSetDefinition, pattern ClarificationPattern, slot ClarificationSlot, locale string) ClarificationSlotOutcome {
	out := ClarificationSlotOutcome{Topic: definition.Topic, TopicVersion: definition.TopicVersion, RulesetVersion: definition.Version, Pattern: pattern.ID, PatternVersion: pattern.Version, Slot: slot.ID, Outcome: ClarificationNotApplicable, Reason: "question_not_matched", Prompt: slot.Prompt, Required: slot.Required, Kind: slot.Kind, Choices: append([]ClarificationChoice(nil), slot.Choices...), Effect: cloneClarificationEffect(slot.Effect), DependsOn: append([]string(nil), slot.DependsOn...)}
	if pattern.Policy != nil {
		out.Why = pattern.Policy.Why
		if locale == "es" && pattern.Policy.WhySpanish != "" {
			out.Why = pattern.Policy.WhySpanish
		}
	}
	if locale == "es" && slot.PromptES != "" {
		out.Prompt = slot.PromptES
	}
	for i := range out.Choices {
		if slot.Choices[i].Target != nil {
			ref := *slot.Choices[i].Target
			out.Choices[i].Target = &ref
		}
		if locale == "es" && out.Choices[i].LabelES != "" {
			out.Choices[i].Label = out.Choices[i].LabelES
		}
	}
	return out
}

func annotateClarificationError(err *ClarificationFieldError, topic, pattern, slot string) {
	err.Topic, err.Pattern, err.Slot = topic, pattern, slot
}

func clarificationHash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// ClarificationResolutionDigest binds every canonical business field and pin.
// It is content evidence, not a signature or a source-read authorization proof.
func ClarificationResolutionDigest(resolution ClarificationResolution) string {
	resolution.ID = ""
	raw, err := json.Marshal(resolution)
	if err != nil {
		return ""
	}
	return clarificationHash(raw)
}

// CanonicalClarificationAnswers replaces raw retained input with parsed values.
// Defaults and interpretation-derived references are reevaluated, not promoted
// into user answers. Removal therefore cannot leave an old default or filter.
func CanonicalClarificationAnswers(resolutions []ClarificationResolution) []ClarificationAnswer {
	var out []ClarificationAnswer
	for _, resolution := range resolutions {
		if resolution.Provenance != "answer" && resolution.Provenance != "legacy_reference_choice" {
			continue
		}
		value := &ClarificationValue{}
		switch {
		case resolution.Reference != nil:
			value.OptionID = resolution.Value
		case resolution.Null:
			value.Null = true
		case resolution.Time != nil:
			value.Time = &ClarificationTimeInput{Start: resolution.Time.LocalStart, End: resolution.Time.LocalEnd, Calendar: resolution.Time.Calendar, TimeZone: resolution.Time.TimeZone, Grain: resolution.Time.Grain}
		case resolution.Effect != nil && resolution.Effect.Kind == "number":
			value.Number = &ClarificationNumberInput{Value: resolution.Value, Upper: resolution.Upper, Unit: resolution.Effect.Unit}
		case resolution.Effect != nil && resolution.Effect.Kind == "boolean":
			item := resolution.Value
			value.Boolean = &item
		default:
			item := resolution.Value
			value.Text = &item
		}
		out = append(out, ClarificationAnswer{Topic: resolution.Topic, TopicVersion: resolution.TopicVersion, RulesetVersion: resolution.RulesetVersion, Pattern: resolution.Pattern, PatternVersion: resolution.PatternVersion, Slot: resolution.Slot, Value: value})
	}
	return out
}
