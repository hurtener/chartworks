package reportingapi

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

func presentationUnionJSON(member string, copyRequest bool) string {
	copyField := ""
	if copyRequest {
		copyField = `,"new_block":"copy"`
	}
	return `{"block":"source","expected_version":1,"revision":1,"digest":"` + strings.Repeat("a", 64) + `","output":"selected"` + copyField + member + `}`
}

func TestAuthoringBlockUnionClosedWireContract(t *testing.T) {
	entries := authoringBlockEntries(nil)
	for _, copyRequest := range []bool{false, true} {
		index := 1
		if copyRequest {
			index = 2
		}
		entry := entries[index]
		if entry.schemaErr != nil {
			t.Fatal(entry.schemaErr)
		}
		for _, member := range []string{
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":0}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"display_label":""}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"display_label":"<script>literal</script>","fraction_digits":20}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","reset":["display_label","fraction_digits"]}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":2},"reset":["display_label"]}]}`,
		} {
			raw := []byte(presentationUnionJSON(member, copyRequest))
			if err := entry.definition.Request.Validate(raw, MaxBodyBytes); err != nil {
				t.Fatalf("valid explicit presentation rejected: %s: %v", raw, err)
			}
			var value any = &authoringBlockMappingTransport{}
			if copyRequest {
				value = &authoringBlockCopyTransport{}
			}
			if err := json.Unmarshal(raw, value); err != nil {
				t.Fatalf("typed decode rejected: %s: %v", raw, err)
			}
			roundTrip, err := json.Marshal(value)
			if err != nil || entry.definition.Request.Validate(roundTrip, MaxBodyBytes) != nil {
				t.Fatalf("explicit zero/empty/reset lost: %s: %v", roundTrip, err)
			}
			var before, after any
			_ = json.Unmarshal(raw, &before)
			_ = json.Unmarshal(roundTrip, &after)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("explicit patch changed on round trip", string(raw), string(roundTrip))
			}
		}
		for _, member := range []string{
			``, `,"mapping":null`, `,"presentation":null`, `,"mapping":null,"presentation":null`,
			`,"mapping":{},"presentation":{"version":1,"edits":[{"column":"value","reset":["display_label"]}]}`,
			`,"mapping":null,"presentation":{"version":1,"edits":[{"column":"value","reset":["display_label"]}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","reset":["display_label"]}]},"purpose":"mapping"`,
			`,"presentation":{"version":2,"edits":[{"column":"value","reset":["display_label"]}]}`,
			`,"presentation":{"version":null,"edits":[{"column":"value","reset":["display_label"]}]}`,
			`,"presentation":{"edits":[{"column":"value","reset":["display_label"]}]}`,
			`,"presentation":{"version":1}`, `,"presentation":{"version":1,"edits":null}`, `,"presentation":{"version":1,"edits":[]}`,
			`,"presentation":{"version":1,"edits":[null]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value"}]}`,
			`,"presentation":{"version":1,"edits":[{"set":{"fraction_digits":2}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":null,"set":{"fraction_digits":2}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":null}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"display_label":null}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":null}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":-1}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":21}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":1.5}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","reset":null}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","reset":[]}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","reset":[null]}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","reset":["currency"]}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","reset":["display_label","display_label"]}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":2},"reset":["fraction_digits"]}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"display_label":""},"reset":["display_label"]}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"format":{"currency":"USD"}}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","set":{"display_label":"a","display_label":"b"}}]}`,
			`,"presentation":{"version":1,"edits":[{"column":"value","column":"other","set":{"fraction_digits":2}}]}`,
			`,"presentation":{"version":1,"version":1,"edits":[{"column":"value","set":{"fraction_digits":2}}]}`,
			`,"presentation":{},"presentation":{}`,
		} {
			raw := []byte(presentationUnionJSON(member, copyRequest))
			if entry.definition.Request.Validate(raw, MaxBodyBytes) == nil {
				t.Fatalf("invalid presentation schema admitted: %s", raw)
			}
			var value any = &authoringBlockMappingTransport{}
			if copyRequest {
				value = &authoringBlockCopyTransport{}
			}
			if json.Unmarshal(raw, value) == nil {
				t.Fatalf("typed decoder admitted invalid presentation: %s", raw)
			}
		}
		good := presentationUnionJSON(`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":0}}]}`, copyRequest)
		for _, field := range []string{"block", "expected_version", "revision", "digest", "output", "new_block"} {
			if field == "new_block" && !copyRequest {
				continue
			}
			for _, null := range []bool{false, true} {
				var object map[string]any
				_ = json.Unmarshal([]byte(good), &object)
				delete(object, field)
				if null {
					object[field] = nil
				}
				raw, _ := json.Marshal(object)
				if entry.definition.Request.Validate(raw, MaxBodyBytes) == nil {
					t.Fatal("exact coordinate optional or null", field, null)
				}
			}
		}
		for _, level := range []string{"presentation", "edit", "set"} {
			for _, field := range []string{"id", "name", "type", "role", "grain", "aggregation", "unit", "currency", "currency_symbol", "percent", "locale", "date_pattern", "provenance", "columns", "rows", "sql", "formatter"} {
				var object map[string]any
				_ = json.Unmarshal([]byte(good), &object)
				at := object["presentation"].(map[string]any)
				if level != "presentation" {
					at = at["edits"].([]any)[0].(map[string]any)
				}
				if level == "set" {
					at = at["set"].(map[string]any)
				}
				at[field] = "private-canary"
				raw, _ := json.Marshal(object)
				if entry.definition.Request.Validate(raw, MaxBodyBytes) == nil {
					t.Fatal("presentation admitted semantic/unsupported field", level, field)
				}
			}
		}

		for _, field := range []string{"kind", "bindings", "order", "options", "kpi", "table", "intent", "columns", "rows", "sql", "scopes", "tenant", "definition", "version"} {
			var object map[string]any
			_ = json.Unmarshal([]byte(good), &object)
			object[field] = map[string]any{}
			raw, _ := json.Marshal(object)
			if entry.definition.Request.Validate(raw, MaxBodyBytes) == nil {
				t.Fatal("presentation admitted top-level mapping/authority field", field)
			}
		}
	}
}

func TestAuthoringBlockUnionLegacyMappingCompatibility(t *testing.T) {
	mapping := reporting.AuthoringChartMapping{Kind: charts.Bar, Bindings: charts.Bindings{Category: "category", Value: "value"}, Options: charts.DefaultOptions()}
	for _, order := range [][]charts.Order{nil, {}} {
		mapping.Order = order
		legacy := reporting.AuthoringBlockCopyRequest{Block: "source", ExpectedVersion: 1, Revision: 1, Digest: strings.Repeat("a", 64), Output: "selected", NewBlock: "copy", Mapping: mapping}
		raw, _ := json.Marshal(legacy)
		entry := authoringBlockEntries(nil)[2]
		if err := entry.definition.Request.Validate(raw, MaxBodyBytes); err != nil {
			t.Fatal("legacy wire rejected", err)
		}
		var got authoringBlockCopyTransport
		if err := json.Unmarshal(raw, &got); err != nil || got.Presentation != nil || got.Mapping == nil || !reflect.DeepEqual(*got.Mapping, mapping) {
			t.Fatal("legacy mapping nilness/content changed", got, err)
		}
		oldSchema, err := api.SchemaFor("legacyMappingRequest", reflect.TypeFor[reporting.AuthoringBlockCopyRequest](), false, api.NullableCollections)
		if err != nil {
			t.Fatal(err)
		}
		var old, current map[string]any
		_ = json.Unmarshal(oldSchema.Document(), &old)
		_ = json.Unmarshal(entry.definition.Request.Document(), &current)
		oldMapping := old["properties"].(map[string]any)["mapping"]
		currentMapping := current["properties"].(map[string]any)["mapping"].(map[string]any)
		// The optional transport pointer adds null in reflection; the union's
		// allOf independently forbids it. All actual legacy member fields agree.
		currentMapping["type"] = "object"
		if !reflect.DeepEqual(oldMapping, currentMapping) {
			t.Fatal("legacy mapping member schema changed")
		}
	}
}

func TestAuthoringBlockUnionDispatchRejectsAmbiguousInProcess(t *testing.T) {
	for _, mapping := range []*reporting.AuthoringChartMapping{nil, {}} {
		var patch *charts.PresentationPatch
		if mapping != nil {
			patch = &charts.PresentationPatch{}
		}
		if _, err := amendAuthoringBlock(nil)(t.Context(), identity.Envelope{}, authoringBlockMappingTransport{Mapping: mapping, Presentation: patch}); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("amend dispatcher admitted both/neither", err)
		}
		if _, err := copyAuthoringBlock(nil)(t.Context(), identity.Envelope{}, authoringBlockCopyTransport{Mapping: mapping, Presentation: patch}); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("copy dispatcher admitted both/neither", err)
		}
	}
}

func TestAuthoringBlockUnionHTTPMCPOpenAPISchemaParity(t *testing.T) {
	registry, err := AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := AuthoringMCPBindings(&reporting.Authoring{})
	if err != nil {
		t.Fatal("typed MCP schema no longer matches HTTP", err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil || len(mcpRegistry.Manifest()) != 25 || mcpserver.MaxRegisteredTools != 96 {
		t.Fatal("union changed tool inventory", err)
	}
	document, err := registry.OpenAPI("Synthetic presentation union", "1")
	if err != nil {
		t.Fatal(err)
	}
	var openapi struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
	}
	if err = json.Unmarshal(document, &openapi); err != nil {
		t.Fatal(err)
	}
	for _, entry := range authoringBlockEntries(nil)[1:3] {
		var want any
		_ = json.Unmarshal(entry.definition.Request.Document(), &want)
		var advertised any
		_ = json.Unmarshal(openapi.Paths[entry.definition.Path]["post"].RequestBody.Content["application/json"].Schema, &advertised)
		if !reflect.DeepEqual(want, advertised) {
			t.Fatal("OpenAPI schema differs from installed operation", entry.definition.ID)
		}
		found := false
		for _, tool := range mcpRegistry.Manifest() {
			if tool.Name != entry.definition.ID {
				continue
			}
			found = true
			raw, _ := json.Marshal(tool.InputSchema)
			var got any
			_ = json.Unmarshal(raw, &got)
			if !reflect.DeepEqual(want, got) {
				t.Fatal("MCP schema differs from installed HTTP contract", tool.Name)
			}
			compiled, err := gateway.NewSchema("mcpUnion", raw)
			if err != nil || compiled.Validate([]byte(presentationUnionJSON(`,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":0}}]}`, strings.Contains(tool.Name, "copy"))), MaxBodyBytes) != nil {
				t.Fatal("MCP union unusable", err)
			}
		}
		if !found {
			t.Fatal("existing union tool removed", entry.definition.ID)
		}
	}
}
