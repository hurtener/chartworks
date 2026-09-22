package rendering

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
)

func exportRequest(format string) Request {
	return Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: "output"}, Format: format, Theme: "light", Width: 800, Height: 420}
}

func TestPagedExportIdentifiesExactProjection(t *testing.T) {
	view := tableView()
	rows := make([][]charts.Cell, 1000)
	for i := range rows {
		rows[i] = []charts.Cell{{Value: "row"}}
	}
	next := 1000
	view.Output.Table.Rows = rows
	view.Output.Table.Completeness = charts.Completeness{Status: "partial", Reason: "retained_limit"}
	view.Output.Table.Warnings = []string{"retained result is bounded"}
	view.PageBounds = reporting.ViewerPage{Limit: 1000, Total: 1500, Next: &next}
	f := &fixtureViewer{value: view}
	s, _ := New(f, 8<<20)
	r, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), exportRequest("json"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Projection.Truncated || r.Projection.Offset != 0 || r.Projection.Limit != 1000 || r.Projection.Total != 1500 || r.Projection.Next == nil || *r.Projection.Next != 1000 || r.Projection.Digest == "" || r.Projection.Digest == r.SourceDigest {
		t.Fatalf("dishonest projection metadata: %+v", r.Projection)
	}
	for _, want := range []string{`"page_bounds"`, `"limit":1000`, `"total":1500`, `"timezone":"UTC"`} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("JSON projection omitted %s", want)
		}
	}

	// A caller may request the delivery default with limit zero. The renderer
	// reports the actual bounded page returned by the authority-aware viewer.
	view.Output.Table.Rows = rows[:100]
	view.PageBounds = reporting.ViewerPage{Limit: 100, Total: 1000, Next: func() *int { n := 100; return &n }()}
	f.value = view
	r, err = s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), exportRequest("html"))
	if err != nil || !r.Projection.Truncated || r.Projection.Limit != 100 || !strings.Contains(r.Content, "rows 0-100 / 1000") {
		t.Fatalf("default page was not explicit: %+v %v", r.Projection, err)
	}

	view.PageBounds.Total = 999
	view.PageBounds.Next = nil
	f.value = view
	if _, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), exportRequest("csv")); err == nil {
		t.Fatal("incomplete page without continuation accepted")
	}
}

func TestCSVNeutralizesHostileHeadersAndCells(t *testing.T) {
	view := tableView()
	view.Output.Table.Columns[0].DisplayLabel = "\ufeff\u202e\x01=SUM(A:A)"
	view.Output.Table.Rows = [][]charts.Cell{{{Value: "\ufeff\u2066\x02+cmd"}}}
	view.PageBounds = reporting.ViewerPage{Limit: 1, Total: 1}
	f := &fixtureViewer{value: view}
	s, _ := New(f, 1<<20)
	r, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), exportRequest("csv"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(r.Content)).ReadAll()
	if err != nil || !strings.HasPrefix(records[0][0], "'") || !strings.HasPrefix(records[1][0], "'") {
		t.Fatalf("hostile formula prefix survived: %#v %v", records, err)
	}
}

func TestStaticKPIHTMLAndSVGCarryAllRetainedFields(t *testing.T) {
	column := charts.Column{ID: "actual", Name: "actual", DisplayLabel: "Revenue", Type: "decimal", Format: charts.Format{CurrencySymbol: "$", FractionDigits: 2}}
	target := charts.Column{ID: "target", Name: "target", Type: "decimal", Format: charts.Format{CurrencySymbol: "$", FractionDigits: 2}}
	value := func(s string) *charts.Value { return &charts.Value{Exact: s} }
	chart := &charts.Output{Version: charts.DisplayVersion, Kind: charts.KPI, State: "ready", Columns: []charts.Column{column, target}, Mapping: charts.Mapping{Version: charts.DisplayVersion, Kind: charts.KPI, Columns: []charts.Column{column, target}, Bindings: charts.Bindings{Value: "actual", Target: "target"}}, KPIResult: &charts.KPIResult{
		Value: charts.Value{Exact: "10"}, Comparison: value("8"), Delta: value("2"), PercentDelta: value("25"), Target: value("12"), TargetDifference: value("-2"), ThresholdState: "warning", ThresholdLabel: "Below target", Sparkline: []charts.Value{{Exact: "7"}, {Exact: "10"}},
	}}
	f := &fixtureViewer{value: reporting.DeliveryViewResult{Timezone: "UTC", Output: &reporting.ViewerOutput{State: "succeeded", RetainedDigest: strings.Repeat("b", 64), Chart: chart}}}
	s, _ := New(f, 1<<20)
	for _, format := range []string{"html", "svg"} {
		in := exportRequest(format)
		r, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), in)
		if err != nil {
			t.Fatal(format, err)
		}
		for _, want := range []string{"10.00 $", "8.00 $", "2.00 $", "25%", "12.00 $", "-2.00 $", "Below target", "warning", "7.00 $"} {
			if !strings.Contains(r.Content, want) {
				t.Fatalf("%s omitted %q: %s", format, want, r.Content)
			}
		}
	}
}

func TestStaticChartFormattingTimezoneThemeAndViewport(t *testing.T) {
	date := charts.Column{ID: "date", Name: "date", Type: "temporal", Format: charts.Format{DatePattern: "date_short", Locale: "es-AR"}}
	money := charts.Column{ID: "money", Name: "money", Type: "decimal", Format: charts.Format{FractionDigits: 2, Locale: "es-AR", CurrencySymbol: "US$"}}
	chart := &charts.Output{Version: charts.Version, Kind: charts.Bar, State: "ready", Columns: []charts.Column{date, money}, Mapping: charts.Mapping{Version: charts.Version, Kind: charts.Bar, Columns: []charts.Column{date, money}, Bindings: charts.Bindings{Category: "date", Value: "money"}}, Points: []charts.Point{{Category: charts.Cell{Value: "2026-09-22T01:30:00Z"}, Value: charts.Value{Exact: "1234.5"}}}}
	f := &fixtureViewer{value: reporting.DeliveryViewResult{Timezone: "America/Argentina/Buenos_Aires", Output: &reporting.ViewerOutput{State: "succeeded", RetainedDigest: strings.Repeat("c", 64), Chart: chart}}}
	s, _ := New(f, 1<<20)
	in := exportRequest("svg")
	light, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), in)
	if err != nil || !strings.Contains(light.Content, "21/09/2026") || !strings.Contains(light.Content, "1.234,50 US$") || !strings.Contains(light.Content, `viewBox="0 0 800 420"`) {
		t.Fatalf("formatted static chart mismatch: %s %v", light.Content, err)
	}
	in.Theme, in.Width, in.Height = "dark", 640, 320
	dark, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), in)
	if err != nil || dark.Content == light.Content || !strings.Contains(dark.Content, `viewBox="0 0 640 320"`) || !strings.Contains(dark.Content, `fill="#17211f"`) {
		t.Fatalf("theme/viewport ignored: %s %v", dark.Content, err)
	}
}

func TestStaticTableHTMLCarriesTotalsScopeAndCompleteness(t *testing.T) {
	view := tableView()
	view.Output.Table.Columns[0] = charts.Column{ID: "amount", Name: "amount", DisplayLabel: "Amount", Type: "decimal", Format: charts.Format{FractionDigits: 2}}
	view.Output.Table.Rows = [][]charts.Cell{{{Value: "10"}}}
	view.Output.Table.Totals = []charts.Total{{Column: "amount", Value: charts.Cell{Value: "40"}, Scope: "returned_rows"}}
	view.Output.Table.Completeness = charts.Completeness{Status: "partial", Reason: "retained_limit"}
	view.Output.Table.Warnings = []string{"bounded result"}
	next := 1
	view.PageBounds = reporting.ViewerPage{Limit: 1, Total: 4, Next: &next}
	f := &fixtureViewer{value: view}
	s, _ := New(f, 1<<20)
	r, err := s.Export(context.Background(), authority(t, "reporting.read", "reporting.export"), exportRequest("html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"40.00", "returned_rows", "rows 0-1 / 4", "partial", "retained_limit", "bounded result"} {
		if !strings.Contains(r.Content, want) {
			t.Fatalf("HTML omitted %q: %s", want, r.Content)
		}
	}
}
