package semantics

import (
	"encoding/json"
	"log/slog"
)

// ClarificationProblem is the bounded repair contract shared by HTTP, MCP and
// clients. It contains reviewed presentation and content-free field diagnostics,
// never supplied values, SQL, bearer credentials or executable matchers.
type ClarificationProblem struct {
	Outcome   ClarificationOutcome       `json:"outcome"`
	Reason    string                     `json:"reason"`
	Questions []ClarificationSlotOutcome `json:"questions,omitempty"`
	Fields    []ClarificationFieldError  `json:"fields,omitempty"`
}

func (ClarificationProblem) LogValue() slog.Value {
	return slog.StringValue("clarification-problem(redacted)")
}
func (ClarificationAnswer) LogValue() slog.Value {
	return slog.StringValue("clarification-answer(redacted)")
}
func (ClarificationResolution) LogValue() slog.Value {
	return slog.StringValue("clarification-resolution(redacted)")
}

// PublicClarificationProblem detaches mutable metadata and excludes governed
// scalar dictionaries. Clients receive field types, effects, explanations and
// localized repair hints without canonical answer values.
func PublicClarificationProblem(in ClarificationProblem) *ClarificationProblem {
	raw, err := json.Marshal(in)
	if err != nil || len(raw) > 262144 {
		return nil
	}
	var out ClarificationProblem
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	for i := range out.Questions {
		if out.Questions[i].Effect != nil {
			out.Questions[i].Effect.Values = nil
		}
	}
	if !ValidClarificationProblem(out) {
		return nil
	}
	return &out
}

// ValidClarificationProblem bounds remote repair data without interpreting it
// as authority or permitting a diagnostic to alter a plan.
func ValidClarificationProblem(in ClarificationProblem) bool {
	switch in.Outcome {
	case ClarificationMissing, ClarificationInvalid, ClarificationConflicting:
	default:
		return false
	}
	if len(in.Reason) > 128 || len(in.Questions) > 64 || len(in.Fields) > 64 {
		return false
	}
	check := func(f ClarificationFieldError) bool {
		return len(f.Topic) <= 128 && len(f.Pattern) <= 128 && len(f.Slot) <= 128 && len(f.Field) <= 256 && len(f.Code) <= 128 && len(f.Message) <= 1024
	}
	for _, f := range in.Fields {
		if !check(f) {
			return false
		}
	}
	for _, q := range in.Questions {
		if len(q.Topic) > 128 || len(q.Pattern) > 128 || len(q.Slot) > 128 || len(q.Prompt) > 1024 || len(q.Why) > 4096 || len(q.Choices) > 64 || len(q.Errors) > 64 {
			return false
		}
		if q.Effect != nil && len(q.Effect.Values) != 0 {
			return false
		}
		for _, f := range q.Errors {
			if !check(f) {
				return false
			}
		}
		for _, c := range q.Choices {
			if len(c.ID) > 128 || len(c.Label) > 1024 || len(c.LabelES) > 1024 {
				return false
			}
		}
	}
	raw, err := json.Marshal(in)
	return err == nil && len(raw) <= 65536
}
