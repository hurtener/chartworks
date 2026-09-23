package config

import "strings"

// ModelWindow is an operator-owned context limit for an exact route/model pair.
// It is not a discovered model capability or an input supplied by a query author.
type ModelWindow struct {
	Provider              string `json:"provider"`
	Model                 string `json:"model"`
	ContextTokens         int    `json:"context_tokens"`
	ProtocolReserveTokens int    `json:"protocol_reserve_tokens"`
}

func validateModelWindows(g Gateway) error {
	if len(g.ModelWindows) > 128 {
		return invalid("gateway.model_windows", "at most 128 exact model windows")
	}
	providers := map[string]bool{}
	for _, p := range g.Bifrost.Providers {
		if NativeProvider(p) == "openai" || NativeProvider(p) == "openrouter" {
			providers[p.Name] = true
		}
	}
	seen := map[[2]string]ModelWindow{}
	for _, w := range g.ModelWindows {
		key := [2]string{w.Provider, w.Model}
		if !providers[w.Provider] || w.Model == "" || len(w.Model) > 256 || strings.TrimSpace(w.Model) != w.Model || strings.ContainsAny(w.Model, "\x00\r\n\t") || w.ContextTokens < 1024 || w.ContextTokens > 16<<20 || w.ProtocolReserveTokens < 64 || w.ProtocolReserveTokens > 8192 || w.ProtocolReserveTokens >= w.ContextTokens {
			return invalid("gateway.model_windows", "exact chat route/model and bounded context/protocol limits required")
		}
		if _, exists := seen[key]; exists {
			return invalid("gateway.model_windows", "duplicate route/model window")
		}
		seen[key] = w
	}
	if len(seen) == 0 {
		return nil // Legacy byte/operation bounds remain; model capacity is unknown.
	}
	for name, role := range g.Roles {
		if name == "embedding" || name == "rerank" || OptionalRole(name) && !role.Enabled {
			continue
		}
		w, ok := seen[[2]string{role.Provider, role.Model}]
		if !ok || role.MaxTokens >= w.ContextTokens-w.ProtocolReserveTokens {
			return invalid("gateway.model_windows", "every enabled chat role needs a window with room for input and output")
		}
	}
	return nil
}
