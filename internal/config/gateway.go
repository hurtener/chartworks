package config

import (
	"strings"
	"time"
)

// GatewayLimits bounds each process; operation budgets also bound every SDK attempt.
type GatewayLimits struct {
	Concurrency       int      `json:"concurrency"`
	TenantConcurrency int      `json:"tenant_concurrency"`
	MaxInputBytes     int      `json:"max_input_bytes"`
	MaxOutputBytes    int      `json:"max_output_bytes"`
	CacheEntries      int      `json:"cache_entries"`
	CacheBytes        int      `json:"cache_bytes"`
	CacheTTL          Duration `json:"cache_ttl"`
}

// DefaultGatewayLimits returns conservative process, input, response and cache bounds.
func DefaultGatewayLimits() GatewayLimits {
	return GatewayLimits{Concurrency: 8, TenantConcurrency: 4, MaxInputBytes: 256 << 10, MaxOutputBytes: 1 << 20, CacheEntries: 1024, CacheBytes: 16 << 20, CacheTTL: Duration(10 * time.Minute)}
}

// OptionalRole identifies operations that require explicit enablement.
func OptionalRole(name string) bool {
	return name == "rerank" || name == "narrative" || name == "visual_rank"
}

// RoleNames returns a detached inventory of the implemented model roles.
func RoleNames() []string {
	return []string{"embedding", "enhance", "sqlgen", "sqlfix", "clarify", "pipeline_draft", "profile_summary", "rerank", "narrative", "visual_rank"}
}

// NativeProvider resolves the trusted route alias to its native Bifrost provider.
func NativeProvider(p Provider) string {
	if p.Type != "" {
		return p.Type
	}
	return p.Name
}

// ValidateGateway checks the production SDK routing contract, including inactive excerpts.
func ValidateGateway(g Gateway, enabled bool) error {
	if g.Driver != "bifrost" || g.MaxAttemptsPerCall < 1 || g.MaxAttemptsPerCall > 4 {
		return invalid("gateway", "Bifrost driver and 1-4 attempts required")
	}
	l := g.Limits
	if l.Concurrency < 1 || l.Concurrency > 64 || l.TenantConcurrency < 1 || l.TenantConcurrency > l.Concurrency || l.MaxInputBytes < 1 || l.MaxInputBytes > 4<<20 || l.MaxOutputBytes < 1 || l.MaxOutputBytes > 4<<20 || l.CacheEntries < 0 || l.CacheEntries > 8192 || l.CacheBytes < 0 || l.CacheBytes > 128<<20 || l.CacheTTL < Duration(time.Second) || l.CacheTTL > Duration(time.Hour) {
		return invalid("gateway.limits", "bounds exceeded")
	}
	if len(g.Bifrost.Providers) > 8 {
		return invalid("gateway.bifrost.providers", "at most eight routes")
	}
	providers := map[string]Provider{}
	for _, p := range g.Bifrost.Providers {
		if p.Name == "" || len(p.Name) > 64 || strings.ContainsAny(p.Name, " /\r\n\t") || providers[p.Name].Name != "" {
			return invalid("gateway.bifrost.providers", "unique bounded route names required")
		}
		switch NativeProvider(p) {
		case "openai", "openrouter", "cohere":
		default:
			return invalid("gateway.bifrost.providers", "unsupported remote SDK provider")
		}
		if _, err := reference(p.APIKey); err != nil {
			return invalid("gateway.bifrost.providers.api_key", "environment secret reference required")
		}
		if p.BaseURL != "" && !secureURL(p.BaseURL) {
			return invalid("gateway.bifrost.providers.base_url", "trusted HTTPS endpoint required")
		}
		providers[p.Name] = p
	}
	names := map[string]bool{}
	for _, n := range RoleNames() {
		names[n] = true
	}
	for name, r := range g.Roles {
		p, ok := providers[r.Provider]
		if r.MaxTokens < 0 || r.MaxTokens > 65536 {
			return invalid("gateway.roles.max_tokens", "output-token bound exceeded")
		}
		if !names[name] || !ok || r.Model == "" || len(r.Model) > 256 || strings.TrimSpace(r.Model) != r.Model || strings.ContainsAny(r.Model, "\r\n\t") || r.Timeout <= 0 || r.Timeout > Duration(5*time.Minute) {
			return invalid("gateway.roles", "known role, route, model and bounded timeout required")
		}
		if r.OnFailure != "" && (name != "rerank" || r.OnFailure != "fail" && r.OnFailure != "preserve_candidates") {
			return invalid("gateway.roles.on_failure", "only rerank has an explicit fallback policy")
		}
		switch name {
		case "embedding":
			if NativeProvider(p) == "cohere" || r.Dimensions < 1 || r.Dimensions > 16384 || r.MaxBatchItems < 1 || r.MaxBatchItems > 1024 || r.MaxBatchBytes < 1 || r.MaxBatchBytes > 4<<20 {
				return invalid("gateway.roles.embedding", "invalid embedding limits or provider")
			}
			if enabled && (r.ModelRevision == "" || len(r.ModelRevision) > 128) {
				return invalid("gateway.roles.embedding.model_revision", "operator-owned generation revision required")
			}
		case "rerank":
			if NativeProvider(p) != "cohere" || r.MaxCandidates < 1 || r.MaxCandidates > 1024 {
				return invalid("gateway.roles.rerank", "rerank-capable route and bounded candidates required")
			}
		default:
			if NativeProvider(p) == "cohere" || r.MaxTokens < 1 || r.MaxTokens > 65536 {
				return invalid("gateway.roles", "structured role requires a chat route and output-token cap")
			}
		}
	}
	if enabled {
		for _, n := range RoleNames() {
			if !OptionalRole(n) {
				if _, ok := g.Roles[n]; !ok {
					return invalid("gateway.roles", "all required roles must be configured")
				}
			}
		}
	}
	return nil
}
