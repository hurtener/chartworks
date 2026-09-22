package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// RuntimeConfig is protected, reviewed model/prompt configuration applied to one run.
type RuntimeConfig struct {
	Digest            string         `json:"digest"`
	Model             string         `json:"model"`
	Models            []RuntimeModel `json:"models,omitempty"`
	SystemInstruction string         `json:"system_instruction"`
	AttemptCostUSD    float64        `json:"attempt_cost_usd"`
}

// RuntimeModel binds one gateway role to its reviewed model.
type RuntimeModel struct {
	Role  string `json:"role"`
	Model string `json:"model"`
}

type runtimeConfigKey struct{}

// ConfigurationDigest derives the immutable identity of effective runtime inputs.
func ConfigurationDigest(cfg RuntimeConfig) string {
	raw, err := json.Marshal(struct {
		Model             string         `json:"model"`
		Models            []RuntimeModel `json:"models,omitempty"`
		SystemInstruction string         `json:"system_instruction"`
		AttemptCostUSD    float64        `json:"attempt_cost_usd"`
	}{cfg.Model, cfg.Models, cfg.SystemInstruction, cfg.AttemptCostUSD})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// WithRuntimeConfig binds reviewed configuration to downstream gateway calls.
func WithRuntimeConfig(ctx context.Context, cfg RuntimeConfig) (context.Context, error) {
	if ctx == nil || len(cfg.Digest) != 64 || strings.Trim(cfg.Digest, "0123456789abcdef") != "" || cfg.Digest != ConfigurationDigest(cfg) || cfg.Model == "" || len(cfg.Model) > 256 || len(cfg.Models) > 16 || len(cfg.SystemInstruction) > 8192 || cfg.AttemptCostUSD < 0 {
		return nil, ErrInput
	}
	seen := map[string]bool{}
	for _, binding := range cfg.Models {
		if binding.Role == "" || binding.Model == "" || len(binding.Role) > 64 || len(binding.Model) > 256 || seen[binding.Role] {
			return nil, ErrInput
		}
		seen[binding.Role] = true
	}
	return context.WithValue(ctx, runtimeConfigKey{}, cfg), nil
}

// ApplyRuntimeConfig returns the exact reviewed model and prompt instruction for a role.
func ApplyRuntimeConfig(ctx context.Context, role, configuredModel, system string) (string, string, string) {
	cfg, ok := ctx.Value(runtimeConfigKey{}).(RuntimeConfig)
	if !ok {
		return configuredModel, system, ""
	}
	model := cfg.Model
	for _, binding := range cfg.Models {
		if binding.Role == role {
			model = binding.Model
			break
		}
	}
	if cfg.SystemInstruction != "" {
		system += "\n" + cfg.SystemInstruction
	}
	return model, system, cfg.Digest
}

// RuntimeAttemptCost returns the reviewed pessimistic cost reservation per provider attempt.
func RuntimeAttemptCost(ctx context.Context) (float64, bool) {
	cfg, ok := ctx.Value(runtimeConfigKey{}).(RuntimeConfig)
	return cfg.AttemptCostUSD, ok && cfg.AttemptCostUSD > 0
}
