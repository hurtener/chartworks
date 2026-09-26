package chartworks

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
)

// GenerationProblem distinguishes user clarification from missing reviewed
// context. It is never an executable plan or an authority/choice-binding token.
type GenerationProblem = generationdecision.Problem

// DecodeGenerationProblem validates HTTP/MCP problem metadata without copying
// arbitrary error text into Error(). It cannot certify prose redaction.
func DecodeGenerationProblem(raw []byte) *GenerationProblem {
	// Eight 512-byte questions may expand to six JSON bytes per ASCII byte.
	// Bound the encoded object without rejecting a valid escaped projection.
	if _, err := gateway.DecodeJSON(raw, 32<<10); err != nil {
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var p GenerationProblem
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF {
		return nil
	}
	return generationdecision.Public(p)
}

func readGenerationProblem(raw []byte) *GenerationProblem {
	if _, err := gateway.DecodeJSON(raw, 128<<10); err != nil {
		return nil
	}
	var wrapper struct {
		Error      string          `json:"error"`
		Generation json.RawMessage `json:"generation"`
	}
	if json.Unmarshal(raw, &wrapper) != nil {
		return nil
	}
	p := DecodeGenerationProblem(wrapper.Generation)
	if !generationdecision.MatchesCode(p, wrapper.Error) {
		return nil
	}
	return p
}
