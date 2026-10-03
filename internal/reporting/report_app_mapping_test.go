package reporting_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
)

// TestReportAppRetainedMappingIdentity isolates the exact immutable mapping
// guard used by frozen reports. Empty JSON arrays are valid approved intent;
// cloning them must not silently turn them into null and alter the digest.
func TestReportAppRetainedMappingIdentity(t *testing.T) {
	column := charts.Column{ID: "amount", Name: "amount", Type: "decimal", Role: "measure", Format: charts.Format{Currency: "USD", FractionDigits: 3}, Provenance: charts.Provenance{Version: 1}}
	data := charts.Data{Version: charts.Version, Columns: []charts.Column{column}, Rows: [][]charts.Cell{{{Value: "42.125"}}}, Completeness: charts.Completeness{Status: "complete_result"}}
	for _, tc := range []struct {
		name       string
		thresholds []charts.KPIThreshold
		order      []charts.Order
	}{
		{name: "nil-arrays"},
		{name: "empty-thresholds", thresholds: []charts.KPIThreshold{}},
		{name: "empty-order", order: []charts.Order{}},
		{name: "both-empty", thresholds: []charts.KPIThreshold{}, order: []charts.Order{}},
		{name: "populated-copy-isolation", thresholds: []charts.KPIThreshold{{Operator: "gte", Value: "1", State: "good", Label: "Approved threshold"}}, order: []charts.Order{{Column: column.ID, Direction: "asc"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapping := charts.Mapping{Version: charts.DisplayVersion, Kind: charts.KPI, Columns: []charts.Column{column}, Bindings: charts.Bindings{Value: column.ID}, Order: tc.order, Options: charts.DefaultOptions(), KPI: &charts.KPIOptions{ValueRow: "first", ComparisonMode: "none", Thresholds: tc.thresholds}}
			if err := charts.ValidateMapping(t.Context(), data, mapping, charts.Defaults()); err != nil {
				t.Fatal("ordinary approved KPI mapping is invalid", err)
			}
			built, err := charts.Build(t.Context(), data, mapping, charts.Defaults())
			if err != nil {
				t.Fatal("deterministic KPI build", err)
			}
			saved := reporting.Output{ID: "kpi-main", Kind: "kpi", Mapping: &mapping}
			retained := reporting.RetainedOutput{ID: saved.ID, Kind: saved.Kind, State: "succeeded", Chart: &built}
			retained.Digest = retained.ContentDigest()
			if err := reporting.CheckFrozenOutput(reporting.RunManifest{Outputs: []reporting.Output{saved}}, retained, false); err != nil {
				before, _ := json.Marshal(mapping)
				after, _ := json.Marshal(built.Mapping)
				t.Fatalf("valid KPI changed frozen mapping identity: %v\nsaved: %s\nbuilt: %s", err, before, after)
			}
			if !reflect.DeepEqual(built.Mapping, mapping) || built.KPIResult == nil || built.KPIResult.Value.Exact != "42.125" {
				t.Fatal("saved mapping or exact KPI value changed", built)
			}
			if built.Mapping.KPI == mapping.KPI {
				t.Fatal("retained KPI options alias the saved definition")
			}
			built.Mapping.Columns[0].Name = "changed"
			if mapping.Columns[0].Name != "amount" {
				t.Fatal("retained columns alias the saved definition")
			}
			if len(built.Mapping.Order) != 0 {
				built.Mapping.Order[0].Direction = "desc"
				if mapping.Order[0].Direction != "asc" {
					t.Fatal("retained sort aliases the saved definition")
				}
			}
			if len(built.Mapping.KPI.Thresholds) != 0 {
				built.Mapping.KPI.Thresholds[0].Label = "changed"
				if mapping.KPI.Thresholds[0].Label != "Approved threshold" {
					t.Fatal("retained thresholds alias the saved definition")
				}
			}
		})
	}
}
