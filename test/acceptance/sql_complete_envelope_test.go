package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
)

// Fictional model windows exercise policy, not vendor capacity qualification.
func syntheticModelWindows(cfg *config.Gateway) {
	for name, role := range cfg.Roles {
		if name != "embedding" && name != "rerank" {
			cfg.ModelWindows = append(cfg.ModelWindows, config.ModelWindow{Provider: role.Provider, Model: role.Model, ContextTokens: 16384, ProtocolReserveTokens: 1024})
		}
	}
}

func TestSQLRecoveryCompleteProviderEnvelope(t *testing.T) {
	t.Run("exact_window_and_output_reserve", func(t *testing.T) {
		probe := newGatewayFixture(t, nil)
		system, prompt := "reviewed system", strings.Repeat("Niñez \"mensual\" <importe>\n", 20)
		envelope, err := probe.engine.GenerationEnvelope(context.Background(), "sqlgen", system, probe.schema)
		if err != nil {
			t.Fatal(err)
		}
		measurement, fits, err := envelope.Measure(prompt)
		if err != nil || !fits || measurement.ContextTokens != 0 || measurement.OutputReserve != 64 {
			t.Fatal("legacy policy must report unknown model capacity and full output reserve", err)
		}
		for _, under := range []bool{false, true} {
			f := newGatewayFixture(t, func(cfg *config.Gateway) {
				syntheticModelWindows(cfg)
				for i := range cfg.ModelWindows {
					if cfg.ModelWindows[i].Model == "model-sqlgen" {
						cfg.ModelWindows[i].ContextTokens = measurement.InputUpperBound + measurement.OutputReserve
						if under {
							cfg.ModelWindows[i].ContextTokens--
						}
					}
				}
			})
			var reserved []int
			ctx, err := gateway.WithAttemptReservation(context.Background(), func(_ context.Context, _ gateway.Call, tokens int) error {
				reserved = append(reserved, tokens)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			b := gatewayBudget(t, f.call, 2)
			out, err := f.engine.Generate(ctx, f.call, b, "sqlgen", system, prompt, f.schema)
			if under {
				calls, _ := b.Used()
				if !errors.Is(err, gateway.ErrBudget) || f.requests.Load() != 0 || calls != 0 || len(reserved) != 0 || len(out.Receipt.Calls) != 0 {
					t.Fatal("one-token overflow reached reservation/provider", err)
				}
				continue
			}
			if err != nil || f.requests.Load() != 1 || len(reserved) != 1 || reserved[0] != measurement.InputUpperBound+measurement.OutputReserve {
				t.Fatal("exact complete-envelope reservation failed", err)
			}
			usage := out.Receipt.Calls[0]
			if usage.Envelope == nil || usage.Envelope.ContextTokens != reserved[0] || usage.Envelope.InputUpperBound != measurement.InputUpperBound || usage.InputTokens == nil || *usage.InputTokens != 10 {
				t.Fatal("admission estimate confused with actual provider usage")
			}
			assertEnvelopeWire(t, f, system, prompt, "model-sqlgen", *usage.Envelope)
		}
	})
	t.Run("runtime_override_uses_its_own_policy", func(t *testing.T) {
		for _, known := range []bool{false, true} {
			f := newGatewayFixture(t, func(cfg *config.Gateway) {
				syntheticModelWindows(cfg)
				if known {
					cfg.ModelWindows = append(cfg.ModelWindows, config.ModelWindow{Provider: "primary", Model: "small-runtime", ContextTokens: 2048, ProtocolReserveTokens: 1024})
				}
			})
			cfg := gateway.RuntimeConfig{Model: "model-sqlgen", Models: []gateway.RuntimeModel{{Role: "sqlgen", Model: "small-runtime"}}, SystemInstruction: strings.Repeat("reviewed guidance ", 100)}
			cfg.Digest = gateway.ConfigurationDigest(cfg)
			ctx, err := gateway.WithRuntimeConfig(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Models[0].Model = "model-sqlgen" // Must not restore the larger configured window.
			b := gatewayBudget(t, f.call, 1)
			out, err := f.engine.Generate(ctx, f.call, b, "sqlgen", "system", "question", f.schema)
			want := gateway.ErrInput
			if known {
				want = gateway.ErrBudget
			}
			calls, _ := b.Used()
			if !errors.Is(err, want) || f.requests.Load() != 0 || calls != 0 || len(out.Receipt.Calls) != 0 {
				t.Fatal("runtime policy fell back to another model or spent a call", err)
			}
		}
	})
	t.Run("optional_refit_reaches_actual_wire", func(t *testing.T) {
		f := newGatewayFixture(t, func(cfg *config.Gateway) {
			syntheticModelWindows(cfg)
			cfg.ModelWindows = append(cfg.ModelWindows, config.ModelWindow{Provider: "primary", Model: "compact-runtime", ContextTokens: 2300, ProtocolReserveTokens: 1024})
		})
		cfg := gateway.RuntimeConfig{Model: "compact-runtime", SystemInstruction: "reviewed additional system guidance"}
		cfg.Digest = gateway.ConfigurationDigest(cfg)
		ctx, err := gateway.WithRuntimeConfig(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		a, err := nlq.NewDefaultContextAssembler()
		if err != nil {
			t.Fatal(err)
		}
		assembled, err := a.Assemble(ctx, nlq.ContextInput{
			Topic: "sales", TopicVersion: "v1", Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Question: "show sales",
			Metrics:     []nlq.PinnedMetric{{ID: "revenue", Text: "Sum booked sales"}},
			Constraints: &nlq.ConstraintState{Allowed: true, Required: []nlq.MandatoryConstraint{{ID: "population", Kind: "required", Text: "Use the reviewed population"}}},
		}, nlq.TierLow)
		if err != nil {
			t.Fatal(err)
		}
		original, err := a.ResolvePrecedence(ctx, nlq.GenerationInput{Context: assembled, Examples: []nlq.Instruction{{Key: "demo", Text: strings.Repeat("optional example ", 150)}}, Default: []nlq.Instruction{{Key: "default", Text: "Preserve the selected analytical meaning"}}})
		if err != nil {
			t.Fatal(err)
		}
		const system, suffix = "Use reviewed context", "\ndialect:postgres\nsource_context:fixture"
		b := gatewayBudget(t, f.call, 1)
		if _, err := f.engine.Generate(ctx, f.call, b, "sqlgen", system, original.Prompt+suffix, f.schema); !errors.Is(err, gateway.ErrBudget) || f.requests.Load() != 0 {
			t.Fatal("unfitted direct call escaped adapter gate", err)
		}
		envelope, err := f.engine.GenerationEnvelope(ctx, "sqlgen", system, f.schema)
		if err != nil {
			t.Fatal(err)
		}
		fitted, err := a.RefitGeneration(ctx, original, func(prompt string) (bool, error) {
			_, fits, err := envelope.Measure(prompt + suffix)
			return fits, err
		})
		if err != nil || fitted.Strategy != nlq.GenerationDefault || !strings.Contains(fitted.Prompt, "Sum booked sales") || !strings.Contains(fitted.Prompt, "reviewed population") || strings.Contains(fitted.Prompt, "optional example") {
			t.Fatal("refit lost required context or failed to remove optional material", err)
		}
		if used, _ := b.Used(); used != 0 || f.requests.Load() != 0 {
			t.Fatal("local fitting spent inference budget")
		}
		out, err := f.engine.Generate(ctx, f.call, b, "sqlgen", system, fitted.Prompt+suffix, f.schema)
		if err != nil || len(out.Receipt.Calls) != 1 || out.Receipt.Calls[0].Envelope == nil {
			t.Fatal("fitted request did not dispatch", err)
		}
		measurement, fits, err := envelope.Measure(fitted.Prompt + suffix)
		if err != nil || !fits || *out.Receipt.Calls[0].Envelope != measurement || out.Receipt.Calls[0].ConfigurationDigest != cfg.Digest {
			t.Fatal("dispatch and local fitting used different envelopes", err)
		}
		assertEnvelopeWire(t, f, system+"\n"+cfg.SystemInstruction, fitted.Prompt+suffix, cfg.Model, measurement)
	})
	t.Run("retry_recharges_complete_envelope", func(t *testing.T) {
		f := newGatewayFixture(t, syntheticModelWindows)
		f.mode.Store("error")
		var reservations []int
		ctx, err := gateway.WithAttemptReservation(context.Background(), func(_ context.Context, _ gateway.Call, tokens int) error {
			reservations = append(reservations, tokens)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		out, err := f.engine.Generate(ctx, f.call, gatewayBudget(t, f.call, 2), "sqlgen", "system", "question", f.schema)
		if !errors.Is(err, gateway.ErrUnavailable) || len(out.Receipt.Calls) != 2 || len(reservations) != 2 || f.requests.Load() != 2 {
			t.Fatal("retry attempts not bounded/retained", err)
		}
		for i, usage := range out.Receipt.Calls {
			if usage.Envelope == nil || reservations[i] != usage.Envelope.InputUpperBound+usage.Envelope.OutputReserve || *usage.Envelope != *out.Receipt.Calls[0].Envelope {
				t.Fatal("retry undercharged or changed the effective envelope")
			}
		}
		out.Receipt.Calls[0].Envelope.Digest = "mutated"
		if out.Receipt.Calls[1].Envelope.Digest == "mutated" {
			t.Fatal("attempt receipts share mutable envelope memory")
		}
	})
}

func assertEnvelopeWire(t *testing.T, f *gatewayFixture, system, prompt, model string, usage gateway.PromptEnvelopeUsage) {
	t.Helper()
	f.mu.Lock()
	body := f.requestBodies[0]
	f.mu.Unlock()
	var wire struct {
		Model          string                           `json:"model"`
		Messages       []struct{ Role, Content string } `json:"messages"`
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
	if json.Unmarshal([]byte(body), &wire) != nil || wire.Model != model || len(wire.Messages) != 2 || wire.Messages[0].Role != "system" || wire.Messages[0].Content != system || wire.Messages[1].Role != "user" || wire.Messages[1].Content != prompt {
		t.Fatal("fitted model/system/prompt do not match the provider wire")
	}
	var actual, expected any
	if json.Unmarshal(wire.ResponseFormat.JSONSchema.Schema, &actual) != nil || json.Unmarshal(f.schema.Document(), &expected) != nil || !reflect.DeepEqual(actual, expected) || wire.ResponseFormat.Type != "json_schema" || !wire.ResponseFormat.JSONSchema.Strict || wire.ResponseFormat.JSONSchema.Name != f.schema.Name() {
		t.Fatal("fitting changed the strict response schema")
	}
	if wire.MaxTokens != usage.OutputReserve && wire.MaxCompletionTokens != usage.OutputReserve || len(body) > usage.InputUpperBound || usage.InputUpperBound+usage.OutputReserve > usage.ContextTokens {
		t.Fatal("actual recorded provider framing or output exceeds the admitted window")
	}
}
