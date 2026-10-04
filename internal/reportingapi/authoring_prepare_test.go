package reportingapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestAuthoringPreparationSchemasRejectClientExecutionMaterial(t *testing.T) {
	entries := authoringPrepareEntries(nil)
	if len(entries) != 5 {
		t.Fatal("inventory")
	}
	for _, entry := range entries {
		if entry.schemaErr != nil {
			t.Fatal(entry.definition.ID, entry.schemaErr)
		}
		for _, private := range []string{`"sql"`, `"statement"`, `"record"`, `"remote"`, `"session"`} {
			if strings.Contains(string(entry.definition.Response.Document()), private) {
				t.Fatal("private custody in response", private)
			}
		}
	}
	request := reporting.AuthoringPrepareRequest{NewBlock: "chart", Operation: "once", Intent: reporting.AuthoringDatasetIntent{Topic: reporting.TopicPin{Topic: "topic", Version: "v1", Digest: strings.Repeat("a", 64)}, Dataset: "sales", Dimensions: []string{"region"}, Measure: "revenue", Mapping: reporting.AuthoringChartMapping{Kind: charts.Bar, Bindings: charts.Bindings{Category: "d", Value: "m"}, Order: []charts.Order{}, Options: charts.DefaultOptions()}}, Metadata: []reporting.Localized{{Locale: "en", Title: "Revenue", Question: "Revenue", Aliases: []string{}}}}
	raw, _ := json.Marshal(request)
	if err := entries[1].definition.Request.Validate(raw, MaxBodyBytes); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"sql", "schema", "rows", "scopes", "tenant", "actor", "source", "context", "binding", "parameters"} {
		var object map[string]any
		json.Unmarshal(raw, &object)
		object[field] = "untrusted"
		bad, _ := json.Marshal(object)
		if entries[1].definition.Request.Validate(bad, MaxBodyBytes) == nil {
			t.Fatal("open execution field", field)
		}
	}
	for _, field := range []string{"filters", "aggregation", "expression", "joins", "columns", "limit"} {
		var object map[string]any
		json.Unmarshal(raw, &object)
		object["intent"].(map[string]any)[field] = "untrusted"
		bad, _ := json.Marshal(object)
		if entries[1].definition.Request.Validate(bad, MaxBodyBytes) == nil {
			t.Fatal("open intent", field)
		}
	}
}
