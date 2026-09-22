package charts_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
)

func displayData() charts.Data {
	columns := []charts.Column{
		{ID: "period", Name: "period", DisplayLabel: "Month", Type: "temporal", Role: "time", Grain: "month", Format: charts.Format{Locale: "es-AR", DatePattern: "year_month"}, Provenance: charts.Provenance{Version: 1}},
		{ID: "actual", Name: "actual", DisplayLabel: "Revenue", Type: "decimal", Role: "kpi", Aggregation: "sum", Format: charts.Format{Currency: "USD", CurrencySymbol: "US$", Locale: "es-AR", FractionDigits: 2}, Provenance: charts.Provenance{Version: 1}},
		{ID: "baseline", Name: "baseline", Type: "decimal", Role: "measure", Aggregation: "sum", Provenance: charts.Provenance{Version: 1}},
		{ID: "target", Name: "target", Type: "decimal", Role: "measure", Aggregation: "sum", Provenance: charts.Provenance{Version: 1}},
	}
	return charts.Data{Version: 1, Columns: columns, Rows: [][]charts.Cell{
		{{Value: "2026-01"}, {Value: "100.00"}, {Value: "95.00"}, {Value: "120.00"}},
		{{Value: "2026-02"}, {Value: "130.00"}, {Value: "110.00"}, {Value: "125.00"}},
	}, Completeness: charts.Completeness{Status: "complete_result"}}
}

func TestDisplayKPIExactDerivedIntent(t *testing.T) {
	d := displayData()
	options := charts.KPIOptions{ValueRow: "last", ComparisonMode: "comparison_column", ShowDelta: true, ShowPercentDelta: true, ShowTargetDifference: true, Sparkline: true, Thresholds: []charts.KPIThreshold{{Operator: "gte", Value: "125", State: "good", Label: "On target"}}}
	m, err := charts.BindDisplay(context.Background(), d, charts.KPI, charts.Bindings{Category: "period", Value: "actual", Comparison: "baseline", Target: "target"}, []charts.Order{{Column: "period", Direction: "asc"}}, charts.DefaultOptions(), &options, nil, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	out, err := charts.Build(context.Background(), d, m, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != charts.DisplayVersion || out.KPIResult == nil || out.KPIResult.Value.Exact != "130.00" || out.KPIResult.Delta.Exact != "20.00" || out.KPIResult.PercentDelta.Exact != "18.18" || out.KPIResult.TargetDifference.Exact != "5.00" || out.KPIResult.ThresholdState != "good" || len(out.KPIResult.Sparkline) != 2 {
		t.Fatalf("unexpected KPI: %+v", out.KPIResult)
	}
}

func TestDisplayTableVisibilityOrderAndTotals(t *testing.T) {
	d := displayData()
	options := charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "period", Visible: true}, {Column: "actual", Visible: true}, {Column: "baseline", Visible: false}}, PageSize: 25, ShowTotals: true}
	m, err := charts.BindDisplay(context.Background(), d, charts.Table, charts.Bindings{Columns: []string{"period", "actual", "baseline"}}, []charts.Order{{Column: "actual", Direction: "desc"}}, charts.DefaultOptions(), nil, &options, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	out, err := charts.Build(context.Background(), d, m, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if out.TablePageSize != 25 || !out.ShowTotals || len(out.Columns) != 2 || out.Columns[0].DisplayLabel != "Month" || !reflect.DeepEqual(out.Rows[0], []charts.Cell{{Value: "2026-02"}, {Value: "130.00"}}) || len(out.Totals) != 1 || out.Totals[0].Column != "actual" {
		t.Fatalf("unexpected table: %+v", out)
	}
}

func TestDisplayIntentRejectsExecutableAndIncoherentFields(t *testing.T) {
	d := displayData()
	bad := charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "period", Visible: true}}, PageSize: 10}
	if _, err := charts.BindDisplay(context.Background(), d, charts.Table, charts.Bindings{Columns: []string{"period"}}, nil, charts.DefaultOptions(), nil, &bad, charts.Defaults()); err != nil {
		t.Fatal(err)
	}
	d.Columns[0].DisplayLabel = "<script>"
	if charts.ValidateData(context.Background(), d, charts.Defaults()) != nil {
		t.Fatal("plain escaped display label should remain valid data")
	}
	d.Columns[0].Format.DatePattern = "javascript:alert(1)"
	if charts.ValidateData(context.Background(), d, charts.Defaults()) == nil {
		t.Fatal("executable date formatter accepted")
	}
}

func TestDisplayKPIDerivedCoordinateOverflowRejects(t *testing.T) {
	d := displayData()
	d.Rows = [][]charts.Cell{{{Value: "2026-01"}, {Value: "1e308"}, {Value: "-1e308"}, {Value: "1"}}}
	options := charts.KPIOptions{ValueRow: "last", ComparisonMode: "comparison_column", ShowDelta: true}
	m, err := charts.BindDisplay(context.Background(), d, charts.KPI, charts.Bindings{Value: "actual", Comparison: "baseline"}, nil, charts.DefaultOptions(), &options, nil, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := charts.Build(context.Background(), d, m, charts.Defaults()); err == nil {
		t.Fatal("non-finite derived drawing coordinate accepted")
	}
}

func TestDisplayRebindUpdatesEveryPolicyReference(t *testing.T) {
	d := displayData()
	options := charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "period", Visible: true}, {Column: "actual", Visible: true}}, PageSize: 25}
	m, err := charts.BindDisplay(context.Background(), d, charts.Table, charts.Bindings{Columns: []string{"period", "actual"}}, []charts.Order{{Column: "actual", Direction: "desc"}}, charts.DefaultOptions(), nil, &options, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for i := range d.Columns {
		if d.Columns[i].ID == "actual" {
			d.Columns[i].ID = "actual_v2"
			d.Columns[i].Name = "renamed_actual"
			d.Columns[i].Provenance.Topic = "topic"
			d.Columns[i].Provenance.TopicVersion = "v1"
			d.Columns[i].Provenance.SemanticID = "actual"
			m.Columns[1].Provenance.Topic = "topic"
			m.Columns[1].Provenance.TopicVersion = "v0"
			m.Columns[1].Provenance.SemanticID = "actual"
		}
	}
	proposal, err := charts.Rebind(context.Background(), d, m, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Mapping.Bindings.Columns[1] != "actual_v2" || proposal.Mapping.Order[0].Column != "actual_v2" || proposal.Mapping.Table.Columns[1].Column != "actual_v2" {
		t.Fatal("display policy retained stale column ID", proposal.Mapping)
	}

	d = displayData()
	kpi := charts.KPIOptions{ValueRow: "last", ComparisonMode: "comparison_column", ShowTargetDifference: true}
	m, err = charts.BindDisplay(context.Background(), d, charts.KPI, charts.Bindings{Value: "actual", Comparison: "baseline", Target: "target"}, nil, charts.DefaultOptions(), &kpi, nil, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"baseline", "target"} {
		old := -1
		for i := range m.Columns {
			if m.Columns[i].ID == id {
				old = i
				m.Columns[i].Provenance.Topic, m.Columns[i].Provenance.TopicVersion, m.Columns[i].Provenance.SemanticID = "topic", "v0", id
			}
		}
		for i := range d.Columns {
			if d.Columns[i].ID == id {
				d.Columns[i].ID = id + "_v2"
				d.Columns[i].Name = "renamed_" + id
				d.Columns[i].Provenance.Topic, d.Columns[i].Provenance.TopicVersion, d.Columns[i].Provenance.SemanticID = "topic", "v1", id
			}
		}
		if old < 0 {
			t.Fatal("missing fixture column", id)
		}
	}
	proposal, err = charts.Rebind(context.Background(), d, m, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Mapping.Bindings.Comparison != "baseline_v2" || proposal.Mapping.Bindings.Target != "target_v2" {
		t.Fatal("KPI policy retained stale column IDs", proposal.Mapping.Bindings)
	}
}
