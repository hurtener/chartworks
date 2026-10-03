package acceptance

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"strconv"
	"strings"
	"testing"
)

// encodeRecordedStrictResponse changes only recorded chat fixture transport.
// Domain fixture content and independent query/result oracles stay unchanged.
// The raw: path deliberately remains untouched for malformed-wire regressions.
func encodeRecordedStrictResponse(t *testing.T, response string, request map[string]any) string {
	t.Helper()
	format, _ := request["response_format"].(map[string]any)
	definition, _ := format["json_schema"].(map[string]any)
	schema, _ := definition["schema"].(map[string]any)
	if schema == nil {
		return response
	}
	var envelope map[string]any
	if json.Unmarshal([]byte(response), &envelope) != nil {
		return response
	}
	choices, _ := envelope["choices"].([]any)
	for _, choice := range choices {
		c, _ := choice.(map[string]any)
		message, _ := c["message"].(map[string]any)
		content, ok := message["content"].(string)
		if !ok {
			continue
		}
		value, err := gateway.DecodeJSON([]byte(content), 1<<20)
		if err != nil {
			continue
		}
		schema = recordedStrictReferences(t, schema, schema)
		encoded, _ := json.Marshal(recordedStrictValue(t, schema, value))
		message["content"] = string(encoded)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
func recordedStrictValue(t *testing.T, schema map[string]any, value any) any {
	t.Helper()
	description, _ := schema["description"].(string)
	branches, _ := schema["anyOf"].([]any)
	if strings.HasPrefix(description, "Optional nullable domain field.") {
		wrapper := branches[0].(map[string]any)
		inner := wrapper["properties"].(map[string]any)["value"].(map[string]any)
		return map[string]any{"value": recordedStrictValue(t, inner, value)}
	}
	if strings.HasPrefix(description, "Null omits.") {
		if value == nil {
			t.Error("recorded domain fixture explicitly nulls a non-null optional field")
			return value
		}
		child := make(map[string]any, len(schema))
		for key, item := range schema {
			child[key] = item
		}
		delete(child, "description")
		return recordedStrictValue(t, child, value)
	}
	if len(branches) != 0 {
		for _, branch := range branches {
			candidate := recordedStrictValue(t, branch.(map[string]any), value)
			document, _ := json.Marshal(branch)
			compiled, err := gateway.NewSchema("recorded_branch", document)
			raw, _ := json.Marshal(candidate)
			if err == nil && compiled.Validate(raw, 1<<20) == nil {
				return candidate
			}
		}
		return value // Preserve invalid authored fixtures; never invent required data.
	}
	switch value := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, v := range value {
			out[key] = v
		}
		properties, _ := schema["properties"].(map[string]any)
		for key, raw := range properties {
			child := raw.(map[string]any)
			v, exists := value[key]
			description, _ := child["description"].(string)
			if !exists && (strings.HasPrefix(description, "Null omits.") || strings.HasPrefix(description, "Optional nullable domain field.")) {
				out[key] = nil
				continue
			}
			if exists {
				out[key] = recordedStrictValue(t, child, v)
			}
		}
		return out
	case []any:
		item, _ := schema["items"].(map[string]any)
		if item == nil {
			return value
		}
		out := make([]any, len(value))
		for i, v := range value {
			out[i] = recordedStrictValue(t, item, v)
		}
		return out
	}
	return value
}

// Resolve only generated local, acyclic and fully inlined definitions.
func recordedStrictReferences(t *testing.T, schema, root map[string]any) map[string]any {
	t.Helper()
	if ref, ok := schema["$ref"].(string); ok {
		const prefix = "#/$defs/"
		if !strings.HasPrefix(ref, prefix) {
			t.Fatal("nonlocal fixture reference")
		}
		name := strings.TrimPrefix(ref, prefix)
		index, err := strconv.Atoi(strings.TrimPrefix(name, "s"))
		if err != nil || index < 0 || name != "s"+strconv.Itoa(index) {
			t.Fatal("nongenerated fixture reference")
		}
		defs, _ := root["$defs"].(map[string]any)
		resolved, _ := defs[name].(map[string]any)
		if resolved == nil || resolved["$ref"] != nil {
			t.Fatal("missing or recursive fixture reference")
		}
		if !recordedDefinitionInlined(resolved) {
			t.Fatal("non-inlined fixture definition")
		}
		return resolved
	}
	out := map[string]any{}
	for key, value := range schema {
		out[key] = value
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		copy := map[string]any{}
		for key, value := range properties {
			copy[key] = recordedStrictReferences(t, value.(map[string]any), root)
		}
		out["properties"] = copy
	}
	if items, ok := schema["items"].(map[string]any); ok {
		out["items"] = recordedStrictReferences(t, items, root)
	}
	if branches, ok := schema["anyOf"].([]any); ok {
		copy := make([]any, len(branches))
		for i, value := range branches {
			copy[i] = recordedStrictReferences(t, value.(map[string]any), root)
		}
		out["anyOf"] = copy
	}
	return out
}

// recordedLiveChatCaps mirrors the reviewed live cohort's nonsecret caps. This
// changes only model configuration, never authored domain fixtures or oracles.
func recordedLiveChatCaps(c *config.Gateway) {
	for name, cap := range map[string]int{"enhance": 4096, "topic_review": 4096, "sqlgen": 4096, "sqlfix": 4096, "clarify": 1024} {
		role := c.Roles[name]
		role.MaxTokens = cap
		c.Roles[name] = role
	}
	c.Limits.MaxInputBytes = 65536
}

func recordedDefinitionInlined(node map[string]any) bool {
	if node["$ref"] != nil || node["$defs"] != nil {
		return false
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		if node["additionalProperties"] != false {
			return false
		}
		for _, value := range properties {
			if !recordedDefinitionInlined(value.(map[string]any)) {
				return false
			}
		}
	}
	if item, ok := node["items"].(map[string]any); ok {
		if !recordedDefinitionInlined(item) {
			return false
		}
	}
	if branches, ok := node["anyOf"].([]any); ok {
		for _, value := range branches {
			if !recordedDefinitionInlined(value.(map[string]any)) {
				return false
			}
		}
	}
	return true
}

func assertRecordedAuthoringEnvelopes(t *testing.T, report *generatedTopicReport) {
	t.Helper()
	maximum, requestBytes, calls := 0, 0, 0
	for _, step := range report.Steps {
		for _, usage := range step.Receipt.Calls {
			if usage.Role != "enhance" && usage.Role != "topic_review" {
				continue
			}
			e := usage.Envelope
			if e == nil || e.OutputReserve != 4096 || e.SchemaProjection != "strict-optional-v1" || len(e.DomainSchemaDigest) != 64 || len(e.WireSchemaDigest) != 64 {
				t.Fatal("recorded authoring did not use actual live caps/projection")
			}
			reservation := e.InputUpperBound + e.OutputReserve
			if reservation > 65536 || e.RequestBytes > 65536 {
				t.Fatal("recorded authoring exceeded unchanged admission caps")
			}
			if reservation > maximum {
				maximum = reservation
				requestBytes = e.RequestBytes
			}
			calls++
		}
	}
	if calls == 0 {
		t.Fatal("no authoring envelopes observed")
	}
	t.Logf("recorded live-cap envelopes: calls=%d maximum_reservation=%d maximum_request_bytes=%d output_reserve=4096 operation_cap=65536", calls, maximum, requestBytes)
}
