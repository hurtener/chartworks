package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
)

// These are recorded provider-wire fixtures through the actual Bifrost SDK,
// not live model quality evidence or a second generation implementation.
func TestSQLRecoveryProviderEnvelope(t *testing.T) {
	t.Run("runtime_addition_is_bounded_before_dispatch", func(t *testing.T) {
		f := newGatewayFixture(t, func(cfg *config.Gateway) { cfg.Limits.MaxInputBytes = 512 })
		cfg := gateway.RuntimeConfig{Model: "recorded-model", SystemInstruction: strings.Repeat("x", 600)}
		cfg.Digest = gateway.ConfigurationDigest(cfg)
		ctx, err := gateway.WithRuntimeConfig(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		result, err := f.engine.Generate(ctx, f.call, gatewayBudget(t, f.call, 1), "sqlgen", "system", "question", f.schema)
		if !errors.Is(err, gateway.ErrInput) || f.requests.Load() != 0 || len(result.Receipt.Calls) != 0 {
			t.Fatal("runtime system addition escaped the final input bound or spent a provider call", err)
		}
	})
	t.Run("effective_model_system_schema_and_output_reserve", func(t *testing.T) {
		f := newGatewayFixture(t, nil)
		cfg := gateway.RuntimeConfig{Model: "recorded-runtime-model", SystemInstruction: "reviewed runtime guidance"}
		cfg.Digest = gateway.ConfigurationDigest(cfg)
		ctx, err := gateway.WithRuntimeConfig(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		out, err := f.engine.Generate(ctx, f.call, gatewayBudget(t, f.call, 1), "sqlgen", "base instruction", "exact fitted user packet", f.schema)
		if err != nil || len(out.Receipt.Calls) != 1 || f.requests.Load() != 1 {
			t.Fatal("recorded dispatch failed", err)
		}
		if out.Receipt.Calls[0].ConfigurationDigest != cfg.Digest {
			t.Fatal("effective configuration missing from receipt")
		}
		f.mu.Lock()
		body := f.requestBodies[0]
		f.mu.Unlock()
		var wire struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Name   string          `json:"name"`
					Strict bool            `json:"strict"`
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
			MaxTokens           int `json:"max_tokens"`
			MaxCompletionTokens int `json:"max_completion_tokens"`
		}
		if json.Unmarshal([]byte(body), &wire) != nil {
			t.Fatal("invalid serialized wire")
		}
		if wire.Model != cfg.Model || len(wire.Messages) != 2 || wire.Messages[0].Role != "system" || wire.Messages[0].Content != "base instruction\n"+cfg.SystemInstruction || wire.Messages[1].Role != "user" || wire.Messages[1].Content != "exact fitted user packet" {
			t.Fatal("effective wire request differs from the admitted input")
		}
		if wire.ResponseFormat.Type != "json_schema" || !wire.ResponseFormat.JSONSchema.Strict || wire.ResponseFormat.JSONSchema.Name != f.schema.Name() || len(wire.ResponseFormat.JSONSchema.Schema) == 0 {
			t.Fatal("missing actual response schema")
		}
		reserve := f.cfg.Roles["sqlgen"].MaxTokens
		if wire.MaxTokens != reserve && wire.MaxCompletionTokens != reserve {
			t.Fatal("missing configured output ceiling")
		}
	})
}
