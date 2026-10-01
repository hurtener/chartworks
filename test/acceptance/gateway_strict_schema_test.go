package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/maximhq/bifrost/core/schemas"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
)

func TestGatewayStrictSchemaRecordedProviderWire(t *testing.T) {
	cases := []struct{ name, role, schema, wire, want string }{
		{"enhancement", "enhance", `{"type":"object","additionalProperties":false,"required":["results"],"properties":{"results":{"type":"array","minItems":1,"items":{"oneOf":[{"type":"object","additionalProperties":false,"required":["kind","name"],"properties":{"kind":{"const":"dimension"},"name":{"type":"string","minLength":1},"geography":{"type":"boolean"}}},{"type":"object","additionalProperties":false,"required":["kind","reason"],"properties":{"kind":{"const":"unresolved"},"reason":{"type":"string"}}}]}},"kpis":{"type":"array","items":{"type":"string"}}}}`, `{"results":[{"kind":"dimension","name":"Region","geography":null}],"kpis":null}`, `{"results":[{"kind":"dimension","name":"Region"}]}`},
		{"review", "topic_review", `{"type":"object","additionalProperties":false,"required":["status","findings"],"properties":{"status":{"enum":["no_findings","needs_review"]},"findings":{"type":"array","uniqueItems":true,"items":{"type":"string"}}}}`, `{"status":"no_findings","findings":[]}`, `{"findings":[],"status":"no_findings"}`},
		{"sql", "sqlgen", `{"type":"object","additionalProperties":false,"required":["decision","sql","parameters"],"properties":{"decision":{"enum":["ready","clarify"]},"sql":{"type":"string","maxLength":32768},"parameters":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["kind","value"],"properties":{"kind":{"enum":["null","text","number"]},"value":{"type":"string"}}}}}}`, `{"decision":"ready","sql":"SELECT 1","parameters":[]}`, `{"decision":"ready","parameters":[],"sql":"SELECT 1"}`},
	}
	for _, tc := range cases {
		for _, provider := range []string{"openrouter", "openai"} {
			t.Run(tc.name+"/"+provider, func(t *testing.T) {
				f := newGatewayFixture(t, func(cfg *config.Gateway) { cfg.Bifrost.Providers[0].Type = provider })
				schema, err := gateway.NewSchema(tc.name, []byte(tc.schema))
				if err != nil {
					t.Fatal(err)
				}
				response, _ := json.Marshal(map[string]any{"model": "recorded-model", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": tc.wire}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5}})
				f.mode.Store("raw:" + string(response))
				envelope, err := f.engine.GenerationEnvelope(context.Background(), tc.role, "system", schema)
				if err != nil {
					t.Fatal(err)
				}
				measured, fit, err := envelope.Measure("prompt")
				if err != nil || !fit {
					t.Fatal(err)
				}
				out, err := f.engine.Generate(context.Background(), f.call, gatewayBudget(t, f.call, 1), tc.role, "system", "prompt", schema)
				if err != nil || string(out.JSON) != tc.want || len(out.Receipt.Calls) != 1 {
					t.Fatalf("recorded generation %s %v", out.JSON, err)
				}
				usage := out.Receipt.Calls[0]
				if usage.Envelope == nil || *usage.Envelope != measured || usage.InputTokens == nil || *usage.InputTokens != 10 {
					t.Fatal("receipt drift", usage)
				}
				f.mu.Lock()
				body := f.requestBodies[0]
				f.mu.Unlock()
				var wire map[string]any
				if json.Unmarshal([]byte(body), &wire) != nil {
					t.Fatal("wire")
				}
				format := wire["response_format"].(map[string]any)
				definition := format["json_schema"].(map[string]any)
				if format["type"] != "json_schema" || definition["strict"] != true {
					t.Fatal("strictness disabled")
				}
				schemaRoot := definition["schema"].(map[string]any)
				assertStrictProviderSubset(t, recordedStrictReferences(t, schemaRoot, schemaRoot))
				projected, _ := json.Marshal(definition["schema"])
				if !bytes.Equal(projected, envelope.SchemaDocument()) {
					t.Fatal("preparation and provider schema disagree")
				}
				if provider == "openrouter" {
					preferences, _ := wire["provider"].(map[string]any)
					if preferences["require_parameters"] != true {
						t.Fatal("unsupported endpoints remain eligible")
					}
				} else if wire["provider"] != nil {
					t.Fatal("OpenRouter preference leaked to OpenAI")
				}
				// Compare measured normalized payload with actual SDK payload fields. SDK
				// max_tokens spelling and its framing remain covered by protocol reserve.
				normalized := map[string]any{"model": wire["model"], "messages": wire["messages"], "response_format": wire["response_format"], "max_completion_tokens": measured.OutputReserve, "store": false}
				if preferences := wire["provider"]; preferences != nil {
					normalized["provider"] = preferences
				}
				raw, _ := json.MarshalIndent(normalized, "", "  ")
				if len(raw) != measured.RequestBytes || len(body) > measured.InputUpperBound {
					t.Fatalf("schema/routing bytes unaccounted: normalized=%d measured=%d wire=%d", len(raw), measured.RequestBytes, len(body))
				}
			})
		}
	}
}

func assertStrictProviderSubset(t *testing.T, node map[string]any) {
	t.Helper()
	for key := range node {
		switch key {
		case "type", "properties", "required", "additionalProperties", "items", "anyOf", "enum", "title", "description", "minLength", "maxLength", "pattern", "format", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minItems", "maxItems", "$defs":
		default:
			t.Fatalf("unsupported keyword on provider wire: %s", key)
		}
	}
	if definitions, ok := node["$defs"].(map[string]any); ok {
		for _, definition := range definitions {
			assertStrictProviderSubset(t, definition.(map[string]any))
		}
	}
	if node["oneOf"] != nil || node["uniqueItems"] != nil {
		t.Fatal("unsupported provider keyword")
	}
	objectType := node["type"] == "object"
	if types, ok := node["type"].([]any); ok {
		for _, kind := range types {
			objectType = objectType || kind == "object"
		}
	}
	if objectType {
		properties := node["properties"].(map[string]any)
		required := node["required"].([]any)
		if node["additionalProperties"] != false || len(properties) != len(required) {
			t.Fatal("provider would reject open/optional object")
		}
		for _, key := range required {
			if properties[key.(string)] == nil {
				t.Fatal("foreign required property")
			}
		}
		for _, value := range properties {
			assertStrictProviderSubset(t, value.(map[string]any))
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		assertStrictProviderSubset(t, items)
	}
	if branches, ok := node["anyOf"].([]any); ok {
		for _, branch := range branches {
			assertStrictProviderSubset(t, branch.(map[string]any))
		}
	}
}

func TestGatewayStrictSchemaCrossRoleRejectionBeforeConsumers(t *testing.T) {
	f := newGatewayFixture(t, nil)
	schema, err := gateway.NewSchema("local_assertion", []byte(`{"type":"object","additionalProperties":false,"required":["items"],"properties":{"items":{"type":"array","uniqueItems":true,"items":{"type":"string"}},"note":{"type":"string"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"enhance", "topic_review", "sqlgen", "sqlfix", "clarify", "pipeline_draft", "profile_summary", "narrative"} {
		t.Run(role, func(t *testing.T) {
			for _, content := range []string{`{"items":["a","a"],"note":null}`, `{"items":null,"note":null}`, `{"note":null}`, `{"items":[],"note":null,"foreign":true}`} {
				response, _ := json.Marshal(map[string]any{"model": "recorded", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}}})
				f.mode.Store("raw:" + string(response))
				out, err := f.engine.Generate(context.Background(), f.call, gatewayBudget(t, f.call, 1), role, "system", "prompt", schema)
				if !errors.Is(err, gateway.ErrOutput) || len(out.JSON) != 0 || len(out.Receipt.Calls) != 1 {
					t.Fatal("invalid domain result escaped", role, err)
				}
				if out.Receipt.Calls[0].CostUSD != nil || out.Receipt.Calls[0].InputTokens != nil {
					t.Fatal("invented provider usage")
				}
			}
		})
	}
	unsupported, err := gateway.NewSchema("unsupported", []byte(`{"type":"object","additionalProperties":false,"required":["value"],"properties":{"value":{"type":"string","not":{"const":"x"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	before := f.requests.Load()
	if _, err := f.engine.GenerationEnvelope(context.Background(), "enhance", "system", unsupported); !errors.Is(err, gateway.ErrInput) {
		t.Fatal("unsupported fit admitted")
	}
	budget := gatewayBudget(t, f.call, 1)
	out, err := f.engine.Generate(context.Background(), f.call, budget, "enhance", "system", "prompt", unsupported)
	if !errors.Is(err, gateway.ErrInput) || len(out.Receipt.Calls) != 0 || f.requests.Load() != before {
		t.Fatal("unsupported schema spent budget", err)
	}
	// Same reservation remains usable after failed schema admission.
	f.mode.Store("normal")
	if _, err = f.engine.Generate(context.Background(), f.call, budget, "enhance", "system", "prompt", f.schema); err != nil {
		t.Fatal("failed schema consumed call", err)
	}
}

func TestGatewayStrictSchemaLargeNumericWireFidelity(t *testing.T) {
	f := newGatewayFixture(t, nil)
	schema, err := gateway.NewSchema("exact_number", []byte(`{"type":"object","additionalProperties":false,"required":["value"],"properties":{"value":{"type":"integer","enum":[9007199254740993],"minimum":9007199254740993,"maximum":9007199254740993}}}`))
	if err != nil {
		t.Fatal(err)
	}
	response := `{"model":"recorded","choices":[{"index":0,"message":{"role":"assistant","content":"{\"value\":9007199254740993}"},"finish_reason":"stop"}]}`
	f.mode.Store("raw:" + response)
	out, err := f.engine.Generate(context.Background(), f.call, gatewayBudget(t, f.call, 1), "sqlgen", "system", "prompt", schema)
	if err != nil || string(out.JSON) != `{"value":9007199254740993}` {
		t.Fatal("large integer changed", string(out.JSON), err)
	}
	f.mu.Lock()
	body := f.requestBodies[0]
	f.mu.Unlock()
	value, err := gateway.DecodeJSON([]byte(body), 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	wire := value.(map[string]any)["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"]
	encoded, _ := json.Marshal(wire)
	envelope, err := f.engine.GenerationEnvelope(context.Background(), "sqlgen", "system", schema)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, envelope.SchemaDocument()) || bytes.Contains(encoded, []byte("9007199254740992")) {
		t.Fatal("SDK schema differed from admitted precision")
	}
}

func TestGatewayRejectsSDKOutputCapExpansionBeforeSpend(t *testing.T) {
	f := newGatewayFixture(t, func(c *config.Gateway) { r := c.Roles["sqlgen"]; r.MaxTokens = 15; c.Roles["sqlgen"] = r })
	if _, err := f.engine.GenerationEnvelope(context.Background(), "sqlgen", "system", f.schema); !errors.Is(err, gateway.ErrInput) {
		t.Fatal("SDK-expanded output cap admitted", err)
	}
	budget := gatewayBudget(t, f.call, 1)
	out, err := f.engine.Generate(context.Background(), f.call, budget, "sqlgen", "system", "prompt", f.schema)
	if !errors.Is(err, gateway.ErrInput) || len(out.Receipt.Calls) != 0 || f.requests.Load() != 0 {
		t.Fatal("undersized output cap reached provider", err)
	}
	if calls, _ := budget.Used(); calls != 0 {
		t.Fatal("cap admission consumed a call")
	}
}

func TestGatewayStrictSchemaVisualRankingRejectsDuplicates(t *testing.T) {
	f := newGatewayFixture(t, nil)
	f.mode.Store(`chat_raw:{"model":"recorded","choices":[{"index":0,"message":{"role":"assistant","content":"{\"order\":[\"first\",\"first\"]}"},"finish_reason":"stop"}]}`)
	out, err := f.engine.VisualRank(context.Background(), f.call, gatewayBudget(t, f.call, 1), "question", f.candidates)
	if !errors.Is(err, gateway.ErrOutput) || len(out.Items) != 0 || len(out.Receipt.Calls) != 1 {
		t.Fatal("duplicate visual permutation escaped", err)
	}
	if out.Receipt.Calls[0].Envelope == nil || out.Receipt.Calls[0].Envelope.LocalAssertions != "uniqueItems" {
		t.Fatal("local uniqueness not disclosed")
	}
}

func TestGatewayStrictSchemaFactoredWireRetained(t *testing.T) {
	f := newGatewayFixture(t, nil)
	properties := map[string]any{}
	required := []string{}
	value := map[string]any{}
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("field_%02d", i)
		properties[name] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "description": "Synthetic bounded identifier"}
		required = append(required, name)
		value[name] = "value"
	}
	document, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required})
	schema, err := gateway.NewSchema("factored", document)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(value)
	response, _ := json.Marshal(map[string]any{"model": "recorded", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}}})
	f.mode.Store("raw:" + string(response))
	out, err := f.engine.Generate(context.Background(), f.call, gatewayBudget(t, f.call, 1), "enhance", "system", "prompt", schema)
	if err != nil || !bytes.Equal(out.JSON, content) {
		t.Fatal("factored generation", err)
	}
	f.mu.Lock()
	body := f.requestBodies[0]
	f.mu.Unlock()
	decoded, err := gateway.DecodeJSON([]byte(body), 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	wire := decoded.(map[string]any)
	definition := wire["response_format"].(map[string]any)["json_schema"].(map[string]any)
	projected := definition["schema"].(map[string]any)
	if projected["$defs"] == nil {
		t.Fatal("SDK lost generated definitions")
	}
	envelope, err := f.engine.GenerationEnvelope(context.Background(), "enhance", "system", schema)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(projected)
	if !bytes.Equal(encoded, envelope.SchemaDocument()) {
		t.Fatal("SDK rewrote factored schema")
	}
	measured, _, _ := envelope.Measure("prompt")
	normalized := map[string]any{"model": wire["model"], "messages": wire["messages"], "response_format": wire["response_format"], "max_completion_tokens": measured.OutputReserve, "store": false, "provider": wire["provider"]}
	effective, _ := json.MarshalIndent(normalized, "", "  ")
	if len(effective) != measured.RequestBytes || len(body) > measured.InputUpperBound {
		t.Fatal("factored wire budget mismatch")
	}
	definition["schema"] = json.RawMessage(document)
	inline, _ := json.MarshalIndent(normalized, "", "  ")
	if measured.RequestBytes >= len(inline) {
		t.Fatal("factoring did not shrink actual envelope")
	}
}

func TestGatewayStrictSchemaInheritedBodyCannotBypassEnvelope(t *testing.T) {
	for _, mode := range []string{"large", "raw", "combined"} {
		t.Run(mode, func(t *testing.T) {
			f := newGatewayFixture(t, nil)
			ctx := context.WithValue(context.Background(), schemas.BifrostContextKeyUseRawRequestBody, mode != "large")
			ctx = context.WithValue(ctx, schemas.BifrostContextKeyLargePayloadMode, mode != "raw")
			ctx = context.WithValue(ctx, schemas.BifrostContextKeyLargePayloadReader, strings.NewReader(`{"model":"foreign-model","messages":[{"role":"user","content":"unmeasured-substitution"}],"provider":{"require_parameters":false}}`))
			out, err := f.engine.Generate(ctx, f.call, gatewayBudget(t, f.call, 1), "sqlgen", "trusted-system", "admitted-prompt", f.schema)
			if err != nil || len(out.Receipt.Calls) != 1 || out.Receipt.Calls[0].Envelope == nil {
				t.Fatal("inherited context changed generation", err)
			}
			f.mu.Lock()
			body := f.requestBodies[0]
			f.mu.Unlock()
			if strings.Contains(body, "unmeasured-substitution") || strings.Contains(body, "foreign-model") {
				t.Fatal("SDK substituted unmeasured body")
			}
			var wire map[string]any
			_ = json.Unmarshal([]byte(body), &wire)
			if wire["provider"].(map[string]any)["require_parameters"] != true || wire["response_format"].(map[string]any)["type"] != "json_schema" {
				t.Fatal("inherited context bypassed strict routing")
			}
			envelope, err := f.engine.GenerationEnvelope(ctx, "sqlgen", "trusted-system", f.schema)
			if err != nil {
				t.Fatal(err)
			}
			measurement, fit, err := envelope.Measure("admitted-prompt")
			if err != nil || !fit || measurement != *out.Receipt.Calls[0].Envelope {
				t.Fatal("inherited context changed receipt")
			}
		})
	}
}
