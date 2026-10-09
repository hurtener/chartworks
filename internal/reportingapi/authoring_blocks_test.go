package reportingapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestAuthoringBlockSchemaClosedAndCatalogParity(t *testing.T) {
	entries := authoringBlockEntries(nil)
	if len(entries) != 4 {
		t.Fatal("unexpected block authoring operations")
	}
	for _, entry := range entries {
		if entry.schemaErr != nil {
			t.Fatal(entry.definition.ID, entry.schemaErr)
		}
		for _, private := range []string{`"sql"`, `"instructions"`, `"prompt_version"`, `"attempt"`} {
			if strings.Contains(string(entry.definition.Response.Document()), private) {
				t.Fatal("private field in authoring response contract", private)
			}
		}
		if entry.definition.ResourceLoader == "" || entry.definition.Audit == "" || entry.definition.Effect == "" {
			t.Fatal("missing native authority/effect registration")
		}
	}
	p := reporting.AuthoringBlockCopyRequest{Block: "source", NewBlock: "copy", ExpectedVersion: 1, Revision: 1, Digest: strings.Repeat("a", 64), Output: "selected", Mapping: reporting.AuthoringChartMapping{Kind: charts.Bar, Bindings: charts.Bindings{Category: "category", Value: "value"}, Order: []charts.Order{}, Options: charts.DefaultOptions()}}
	raw, _ := json.Marshal(p)
	for _, kind := range charts.Catalog() {
		p.Mapping.Kind = kind.Kind
		raw, _ := json.Marshal(p)
		if err := entries[2].definition.Request.Validate(raw, MaxBodyBytes); err != nil {
			t.Fatal("catalog/schema kind drift", kind.Kind, err)
		}
	}
	p.Mapping.Kind = "waterfall"
	bad, _ := json.Marshal(p)
	if entries[2].definition.Request.Validate(bad, MaxBodyBytes) == nil {
		t.Fatal("non-catalog kind advertised")
	}
	for _, tc := range []struct {
		name  string
		path  []string
		value any
	}{
		{"tenant", []string{"tenant"}, "foreign"}, {"authority", []string{"scopes"}, []string{"reporting.write"}},
		{"whole definition", []string{"definition"}, map[string]any{"sql": "SELECT secret"}},
		{"sql", []string{"sql"}, "SELECT secret"}, {"rows", []string{"mapping", "rows"}, [][]string{{"secret"}}},
		{"columns", []string{"mapping", "columns"}, []map[string]any{{"id": "forged", "type": "number"}}},
		{"provenance", []string{"mapping", "provenance"}, map[string]any{"source": "foreign"}},
		{"options callback", []string{"mapping", "options", "formatter"}, "function(){}"},
		{"binding injection", []string{"mapping", "bindings", "sql"}, "SELECT secret"},
		{"disclosure erase", []string{"mapping", "amount_completeness"}, []any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			at := payload
			for _, key := range tc.path[:len(tc.path)-1] {
				at = at[key].(map[string]any)
			}
			at[tc.path[len(tc.path)-1]] = tc.value
			bad, _ := json.Marshal(payload)
			if entries[2].definition.Request.Validate(bad, MaxBodyBytes) == nil {
				t.Fatal("closed authoring schema admitted injection")
			}
		})
	}
	for _, field := range []string{"expected_version", "revision", "digest", "output", "new_block"} {
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		delete(payload, field)
		bad, _ := json.Marshal(payload)
		if entries[2].definition.Request.Validate(bad, MaxBodyBytes) == nil {
			t.Fatal("missing required exact coordinate", field)
		}
	}
	// The mapping payload itself has only the seven explicit presentation keys.
	typ := reflect.TypeFor[reporting.AuthoringChartMapping]()
	if typ.NumField() != 7 {
		t.Fatal("unexpected expanded mapping mutation surface")
	}
}

func TestAuthoringBlockValidationSchemaIsBoundedAndExact(t *testing.T) {
	entry := authoringBlockEntries(nil)[3]
	in := reporting.AuthoringBlockValidateRequest{Block: "private", ExpectedVersion: 1, Revision: 1, Digest: strings.Repeat("a", 64), Arguments: []reporting.Argument{}}
	raw, _ := json.Marshal(in)
	if err := entry.definition.Request.Validate(raw, MaxBodyBytes); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"sql", "mapping", "rows", "definition", "tenant", "evidence", "attempt", "session"} {
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		payload[field] = "forged"
		bad, _ := json.Marshal(payload)
		if entry.definition.Request.Validate(bad, MaxBodyBytes) == nil {
			t.Fatal("validation authority/data injected", field)
		}
	}
	for _, field := range []string{"expected_version", "revision", "digest"} {
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		delete(payload, field)
		bad, _ := json.Marshal(payload)
		if entry.definition.Request.Validate(bad, MaxBodyBytes) == nil {
			t.Fatal("validation coordinate omitted", field)
		}
	}
	if entry.definition.Action != "reporting.validate" || entry.definition.Effect != "explicit_bounded_source_read_private_evidence" {
		t.Fatal("validation action/effect drift")
	}
}
