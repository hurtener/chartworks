package semantics

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// CloneClarificationResolutions returns a detached canonical snapshot.
func CloneClarificationResolutions(in []ClarificationResolution) []ClarificationResolution {
	if in == nil {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out []ClarificationResolution
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

type clarificationAnswerKey struct{ topic, pattern, slot string }

// MergeClarificationAnswers applies a bounded replacement/removal delta. It
// never treats removal as a value, carries two versions of one slot, or silently
// ignores a removal for a value that the session does not own. The caller must
// subsequently reevaluate every retained answer against current publications.
func MergeClarificationAnswers(base, delta []ClarificationAnswer) ([]ClarificationAnswer, error) {
	if len(base) > 64 || len(delta) > 64 {
		return nil, invalid(CodeLimit, "answers")
	}
	values := make(map[clarificationAnswerKey]ClarificationAnswer, len(base)+len(delta))
	for _, answer := range CloneClarificationAnswers(base) {
		key := clarificationAnswerKey{answer.Topic, answer.Pattern, answer.Slot}
		if answer.Remove || answer.Value == nil {
			return nil, invalid(CodeInvalidValue, "answers")
		}
		if _, exists := values[key]; exists {
			return nil, invalid(CodeDuplicateID, "answers")
		}
		values[key] = answer
	}
	seen := map[clarificationAnswerKey]bool{}
	for _, answer := range CloneClarificationAnswers(delta) {
		key := clarificationAnswerKey{answer.Topic, answer.Pattern, answer.Slot}
		if seen[key] || answer.Remove == (answer.Value != nil) {
			return nil, invalid(CodeInvalidValue, "answers")
		}
		seen[key] = true
		previous, exists := values[key]
		if exists && (previous.TopicVersion != answer.TopicVersion || previous.RulesetVersion != answer.RulesetVersion || previous.PatternVersion != answer.PatternVersion) {
			return nil, invalid(CodeEvidenceMismatch, "answers.version")
		}
		if answer.Remove {
			if !exists {
				return nil, invalid(CodeMissingReference, "answers.remove")
			}
			delete(values, key)
		} else {
			values[key] = answer
		}
	}
	if len(values) > 64 {
		return nil, invalid(CodeLimit, "answers")
	}
	out := make([]ClarificationAnswer, 0, len(values))
	for _, answer := range values {
		out = append(out, answer)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Topic != b.Topic {
			return a.Topic < b.Topic
		}
		if a.Pattern != b.Pattern {
			return a.Pattern < b.Pattern
		}
		return a.Slot < b.Slot
	})
	return out, nil
}

// ProviderClarificationText describes the reviewed effect, but never contains a
// supplied scalar, a governed-value dictionary, or a sensitive answer. Values
// are bound by the service after generation, not inferred by the model.
func ProviderClarificationText(resolution ClarificationResolution) (string, error) {
	if resolution.ID == "" || resolution.ID != ClarificationResolutionDigest(resolution) {
		return "", invalid(CodeEvidenceMismatch, "clarification.resolution")
	}
	var effect *ClarificationEffect
	if resolution.Effect != nil {
		x := *resolution.Effect
		x.Values = nil
		effect = &x
	}
	grain := ""
	if resolution.Time != nil {
		grain = resolution.Time.Grain
	}
	view := struct {
		ID             string               `json:"resolution"`
		Topic          string               `json:"topic"`
		TopicVersion   string               `json:"topic_version"`
		RulesetVersion string               `json:"ruleset_version"`
		Pattern        string               `json:"pattern"`
		PatternVersion string               `json:"pattern_version"`
		Slot           string               `json:"slot"`
		Reference      *Reference           `json:"reference,omitempty"`
		Effect         *ClarificationEffect `json:"effect,omitempty"`
		Grain          string               `json:"grain,omitempty"`
		Provenance     string               `json:"provenance"`
		Binding        string               `json:"binding"`
	}{resolution.ID, resolution.Topic, resolution.TopicVersion, resolution.RulesetVersion, resolution.Pattern, resolution.PatternVersion, resolution.Slot, resolution.Reference, effect, grain, resolution.Provenance, "service_owned_constraint; do not invent or repeat its scalar value"}
	raw, err := json.Marshal(view)
	if err != nil || len(raw) > 16<<10 {
		return "", invalid(CodeLimit, "clarification.context")
	}
	return string(raw), nil
}

// ClarificationBudgetText counts the entire selected canonical group in the
// existing tokenizer budget. Unselected dictionary entries are authoring data,
// not additional resolved constraints. This text must never reach a provider.
func ClarificationBudgetText(resolution ClarificationResolution) (string, error) {
	if resolution.ID == "" || resolution.ID != ClarificationResolutionDigest(resolution) {
		return "", invalid(CodeEvidenceMismatch, "clarification.resolution")
	}
	if resolution.Effect != nil {
		x := *resolution.Effect
		x.Values = nil
		resolution.Effect = &x
	}
	raw, err := json.Marshal(resolution)
	if err != nil || len(raw) > 16<<10 {
		return "", invalid(CodeLimit, "clarification.context")
	}
	return string(raw), nil
}

// RedactClarificationText removes known sensitive answer spellings from a
// question or instruction before persistence/provider use. Patterns are always
// quoted literals; neither authors nor callers provide executable matchers.
func RedactClarificationText(text string, answers []ClarificationAnswer, resolutions []ClarificationResolution) string {
	var literals []string
	for _, r := range resolutions {
		if r.Sensitivity != LiteralSensitive {
			continue
		}
		literals = append(literals, r.Value, r.Upper)
		if r.Time != nil {
			literals = append(literals, r.Time.StartUTC, r.Time.EndUTC, r.Time.LocalStart, r.Time.LocalEnd)
		}
		for _, answer := range answers {
			if answer.Topic != r.Topic || answer.Pattern != r.Pattern || answer.Slot != r.Slot || answer.Value == nil {
				continue
			}
			v := answer.Value
			literals = append(literals, v.OptionID)
			if v.Text != nil {
				literals = append(literals, *v.Text)
			}
			if v.Boolean != nil {
				literals = append(literals, *v.Boolean)
			}
			if v.Number != nil {
				literals = append(literals, v.Number.Value, v.Number.Upper)
			}
			if v.Time != nil {
				literals = append(literals, v.Time.Start, v.Time.End, v.Time.Period)
			}
		}
	}
	sort.Slice(literals, func(i, j int) bool {
		if len(literals[i]) != len(literals[j]) {
			return len(literals[i]) > len(literals[j])
		}
		return literals[i] < literals[j]
	})
	seen := map[string]bool{}
	patterns := make([]string, 0, len(literals))
	for _, literal := range literals {
		key := strings.ToLower(literal)
		if literal == "" || len(literal) > 4096 || seen[key] {
			continue
		}
		seen[key] = true
		patterns = append(patterns, regexp.QuoteMeta(literal))
	}
	if len(patterns) == 0 {
		return text
	}
	// One pass cannot reinterpret the inserted marker as another input value.
	matcher, err := regexp.Compile("(?i)(?:" + strings.Join(patterns, "|") + ")")
	if err != nil {
		return "[redacted answer]"
	}
	return matcher.ReplaceAllString(text, "[redacted answer]")
}

// String and GoString keep ordinary structured formatting content-free. JSON
// serialization is reserved for the caller-owned API and protected domain store.
func (ClarificationAnswer) String() string { return "clarification-answer(redacted)" }

// GoString preserves answer redaction for Go-syntax formatting.
func (a ClarificationAnswer) GoString() string { return a.String() }
func (ClarificationResolution) String() string { return "clarification-resolution(redacted)" }

// GoString preserves resolution redaction for Go-syntax formatting.
func (r ClarificationResolution) GoString() string { return r.String() }

func clarificationPresentationOrder(slots []ClarificationSlot) []ClarificationSlot {
	out := append([]ClarificationSlot(nil), slots...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Required != out[j].Required {
			return out[i].Required
		}
		return out[i].ID < out[j].ID
	})
	return out
}
