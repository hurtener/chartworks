package bifrost

import (
	"encoding/json"
	"math"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
)

// wire consumes the real SDK's raw-response observation, never another network client.
// It is discarded after validation and never placed in public results, logs or caches.
func wire(raw any, limit int) (map[string]any, error) {
	var data []byte
	switch v := raw.(type) {
	case json.RawMessage:
		data = v
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return nil, gateway.ErrOutput
	}
	value, err := gateway.DecodeJSON(data, limit)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, gateway.ErrOutput
	}
	return object, nil
}

func indexedWire(raw any, field, value string, count, limit int) error {
	object, err := wire(raw, limit)
	if err != nil {
		return err
	}
	items, ok := object[field].([]any)
	if !ok || len(items) != count {
		return gateway.ErrOutput
	}
	seen := make(map[int64]bool, count)
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return gateway.ErrOutput
		}
		index, ok := row["index"].(json.Number)
		if !ok {
			return gateway.ErrOutput
		}
		n, err := index.Int64()
		if err != nil || n < 0 || n >= int64(count) || seen[n] {
			return gateway.ErrOutput
		}
		seen[n] = true
		if value == "embedding" {
			if _, ok := row[value].([]any); !ok {
				return gateway.ErrOutput
			}
		} else {
			score, ok := row[value].(json.Number)
			if !ok {
				return gateway.ErrOutput
			}
			f, err := score.Float64()
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return gateway.ErrOutput
			}
		}
	}
	return nil
}
func observedModel(model string) string {
	if len(model) == 0 || len(model) > 256 {
		return ""
	}
	for _, c := range model {
		if c < 33 || c > 126 {
			return ""
		}
	}
	return model
}
func countValue(v any) *int {
	n, ok := v.(json.Number)
	if !ok {
		return nil
	}
	value, err := n.Int64()
	if err != nil || value < 0 || value > 1<<40 {
		return nil
	}
	out := int(value)
	return &out
}
func costValue(v any) *float64 {
	n, ok := v.(json.Number)
	if !ok {
		return nil
	}
	value, err := n.Float64()
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}
func usage(role string, p route, model, actual string, start time.Time, raw any) gateway.Usage {
	out := gateway.Usage{Role: role, Provider: p.name, RequestedModel: model, ActualModel: actual, Attempts: 1, DurationMS: time.Since(start).Milliseconds()}
	object, err := wire(raw, 4<<20)
	if err != nil {
		return out
	}
	u, ok := object["usage"].(map[string]any)
	if !ok { // Cohere reports token counts inside meta.tokens, not billed search units.
		meta, _ := object["meta"].(map[string]any)
		u, _ = meta["tokens"].(map[string]any)
		out.InputTokens = countValue(u["input_tokens"])
		out.OutputTokens = countValue(u["output_tokens"])
		return out
	}
	out.InputTokens = countValue(u["prompt_tokens"])
	out.OutputTokens = countValue(u["completion_tokens"])
	// Prefer an explicitly present total, including zero. Never sum a total with components.
	if total := costValue(u["cost"]); total != nil {
		out.CostUSD = total
		return out
	}
	if cost, ok := u["cost"].(map[string]any); ok {
		if total := costValue(cost["total_cost"]); total != nil {
			out.CostUSD = total
			return out
		}
		// Components can overlap across provider billing models. Without an explicit total,
		// leave cost unknown rather than invent an aggregate from potentially overlapping fields.
	}
	return out
}
func observe(b *gateway.Budget, call gateway.Call, reserved int, receipt gateway.Receipt) error {
	u := receipt.Calls[len(receipt.Calls)-1]
	reported := 0
	if u.InputTokens != nil {
		reported += *u.InputTokens
	}
	if u.OutputTokens != nil {
		reported += *u.OutputTokens
	}
	return b.Observe(call, reserved, reported)
}
