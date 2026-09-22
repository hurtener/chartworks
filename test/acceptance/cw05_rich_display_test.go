package acceptance

import (
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

// TestCW05RichDisplay exercises the actual HTTP registry and public SDK. The
// pure core and retained renderer tests cover the same shapes at their owners.
func TestCW05RichDisplay(t *testing.T) {
	f := newChartHTTP(t, nil, nil)
	data := charts.Data{Version: 1, Columns: []charts.Column{
		{ID: "month", Name: "month", DisplayLabel: "Mes", Type: "temporal", Role: "time", Grain: "month", Format: charts.Format{Locale: "es-AR", DatePattern: "year_month"}, Provenance: charts.Provenance{Version: 1}},
		{ID: "actual", Name: "actual", DisplayLabel: "Ingresos", Type: "decimal", Role: "kpi", Aggregation: "sum", Format: charts.Format{Currency: "USD", CurrencySymbol: "US$", Locale: "es-AR", FractionDigits: 2}, Provenance: charts.Provenance{Version: 1}},
		{ID: "baseline", Name: "baseline", Type: "decimal", Role: "measure", Aggregation: "sum", Provenance: charts.Provenance{Version: 1}},
	}, Rows: [][]charts.Cell{{{Value: "2026-08"}, {Value: "100.00"}, {Value: "80.00"}}, {{Value: "2026-09"}, {Value: "125.00"}, {Value: "100.00"}}}, Completeness: charts.Completeness{Status: "complete_result"}}
	kpi := cw.ChartKPIOptions{ValueRow: "last", ComparisonMode: "comparison_column", ShowDelta: true, ShowPercentDelta: true, Sparkline: true, Thresholds: []charts.KPIThreshold{{Operator: "gte", Value: "120", State: "good", Label: "On target"}}}
	result, err := f.client.SpecifyChart(t.Context(), cw.ChartSpecifyRequest{Data: data, Kind: cw.ChartKPI, Bindings: cw.ChartBindings{Category: "month", Value: "actual", Comparison: "baseline"}, Order: []charts.Order{{Column: "month", Direction: "asc"}}, Options: cw.DefaultChartOptions(), KPI: &kpi})
	if err != nil || result.Output.Version != cw.ChartDisplayVersion || result.Output.KPIResult == nil || result.Output.KPIResult.Delta.Exact != "25.00" || result.Output.KPIResult.PercentDelta.Exact != "25.00" {
		t.Fatal("rich KPI HTTP/SDK", result.Output.KPIResult, err)
	}
	replayed, err := f.client.BuildChart(t.Context(), cw.ChartBuildRequest{Data: data, Mapping: result.Output.Mapping})
	if err != nil || !reflect.DeepEqual(replayed.Output, result.Output) {
		t.Fatal("saved display intent did not replay exactly", err)
	}

	table := cw.ChartTableOptions{Columns: []cw.ChartTableColumnIntent{{Column: "month", Visible: true}, {Column: "actual", Visible: true}, {Column: "baseline", Visible: false}}, PageSize: 1, ShowTotals: true}
	result, err = f.client.SpecifyChart(t.Context(), cw.ChartSpecifyRequest{Data: data, Kind: cw.ChartTable, Bindings: cw.ChartBindings{Columns: []string{"month", "actual", "baseline"}}, Order: []charts.Order{{Column: "actual", Direction: "desc"}}, Options: cw.DefaultChartOptions(), Table: &table})
	if err != nil || result.Output.TablePageSize != 1 || len(result.Output.Columns) != 2 || result.Output.Columns[1].DisplayLabel != "Ingresos" || len(result.Output.Totals) != 1 {
		t.Fatal("rich table HTTP/SDK", result.Output, err)
	}
}
