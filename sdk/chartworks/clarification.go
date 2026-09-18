package chartworks

import (
	"encoding/json"
	"io"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
)

// ClarificationAnswer preserves the versioned HTTP, MCP and in-process input.
// Only the service can resolve it; it never confers read authority.
type ClarificationAnswer = semantics.ClarificationAnswer

// ClarificationValue preserves the service's closed typed-answer union.
type ClarificationValue = semantics.ClarificationValue

// ClarificationTimeInput carries explicit calendar, timezone and interval inputs.
type ClarificationTimeInput = semantics.ClarificationTimeInput

// ClarificationNumberInput preserves exact decimal text and the declared unit.
type ClarificationNumberInput = semantics.ClarificationNumberInput

// ClarificationProblem carries a bounded localized repair response.
type ClarificationProblem = semantics.ClarificationProblem

// ClarificationFieldError identifies a field and a value-free repair message.
type ClarificationFieldError = semantics.ClarificationFieldError

// DecodeClarificationProblem validates an isolated repair payload from an HTTP
// error or MCP fault. It never copies arbitrary server error text into Error().
func DecodeClarificationProblem(raw []byte) *ClarificationProblem {
	if _, err := gateway.DecodeJSON(raw, 65536); err != nil {
		return nil
	}
	var problem ClarificationProblem
	if json.Unmarshal(raw, &problem) != nil || !semantics.ValidClarificationProblem(problem) {
		return nil
	}
	return semantics.PublicClarificationProblem(problem)
}

func readClarificationProblem(body io.Reader) *ClarificationProblem {
	raw, err := io.ReadAll(io.LimitReader(body, 131073))
	if err != nil || len(raw) > 131072 {
		return nil
	}
	if _, err := gateway.DecodeJSON(raw, 131072); err != nil {
		return nil
	}
	var wrapper struct {
		Clarification json.RawMessage `json:"clarification"`
	}
	if json.Unmarshal(raw, &wrapper) != nil {
		return nil
	}
	return DecodeClarificationProblem(wrapper.Clarification)
}
