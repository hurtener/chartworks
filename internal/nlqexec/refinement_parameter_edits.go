package nlqexec

import (
	"context"
	"log/slog"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/exec"
)

// ParameterEdit explicitly replaces one model-owned positional value on Refine.
// Positions are one-based in the parent's unbound base, not its final list of
// service-owned predicate bindings. A replacement cannot change the slot kind,
// add/remove a slot, or change its SQL role. Values are private request data.
type ParameterEdit struct {
	Position    int            `json:"position"`
	Replacement exec.Parameter `json:"replacement"`
}

// String avoids rendering private replacement values through ordinary logging.
func (ParameterEdit) String() string { return "parameter-edit(redacted)" }

// GoString preserves redaction for Go-syntax formatting.
func (e ParameterEdit) GoString() string { return e.String() }

// LogValue preserves redaction in structured logging without changing JSON input.
func (e ParameterEdit) LogValue() slog.Value { return slog.StringValue(e.String()) }

// replaceRefinementParameters consumes only an already verified parent's model
// slots. It produces detached values; no user-supplied object becomes authority,
// model context, or a mutation of the original query. Syntax/kind checks precede
// the unchanged native parameter-role and analytical validation downstream.
func replaceRefinementParameters(ctx context.Context, original []exec.Parameter, edits []ParameterEdit) ([]exec.Parameter, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(original) > 64 || len(edits) > 64 {
		return nil, ErrInvalid
	}
	for _, p := range original {
		if !p.Valid() || !utf8.ValidString(p.Value) {
			return nil, exec.ErrBinding
		}
	}
	result := append([]exec.Parameter(nil), original...)
	seen := make([]bool, len(original))
	for _, edit := range edits {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		i := edit.Position - 1
		if edit.Position < 1 || edit.Position > len(original) || seen[i] || !edit.Replacement.Valid() || !utf8.ValidString(edit.Replacement.Value) || edit.Replacement.Kind != original[i].Kind {
			return nil, ErrInvalid
		}
		seen[i] = true
		result[i] = edit.Replacement
	}
	return result, nil
}
