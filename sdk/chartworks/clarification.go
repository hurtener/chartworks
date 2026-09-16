package chartworks

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"io"
)

// These aliases preserve the same versioned contract for HTTP, MCP and in-process
// clients. Only the service can resolve them; they never confer read authority.
type ClarificationAnswer = semantics.ClarificationAnswer
type ClarificationValue = semantics.ClarificationValue
type ClarificationTimeInput = semantics.ClarificationTimeInput
type ClarificationNumberInput = semantics.ClarificationNumberInput
type ClarificationProblem = semantics.ClarificationProblem
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
