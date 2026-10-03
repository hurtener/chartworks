package charts

import (
	"context"
	"encoding/json"
)

// BuildWithSourceRows additionally retains table source-row ordinals for an
// explicitly requested evidence consumer. Ordinary Build and historical output
// bytes are unchanged. Equal values never substitute for row identity.
func BuildWithSourceRows(ctx context.Context, d Data, m Mapping, limits Limits) (Output, error) {
	out, err := Build(ctx, d, m, limits)
	if err != nil || m.Kind != Table {
		return out, err
	}
	// Both current table builders retain NULL rows and use this same stable
	// ordering function. A future transformation must prove its own alignment.
	indices := orderedRows(ctx, d, m)
	if len(indices) != len(out.Rows) {
		return Output{}, ErrInvalid
	}
	out.RowIndices = append([]int(nil), indices...)
	encoded, err := json.Marshal(out)
	if err != nil {
		return Output{}, ErrInvalid
	}
	if len(encoded) > limits.MaxBytes*4 {
		return Output{}, ErrLimit
	}
	return out, ctx.Err()
}
