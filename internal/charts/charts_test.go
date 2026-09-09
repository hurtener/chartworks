package charts_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func TestCatalogGoldens(t *testing.T) { chartfixtures.Verify(t) }

func bind(t *testing.T, kind charts.Kind) (charts.Data, charts.Mapping) {
	t.Helper()
	d, b, o := chartfixtures.Fixture(kind, "binding")
	m, err := charts.Bind(context.Background(), d, kind, b, o, charts.DefaultOptions(), charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return d, m
}

func TestPureSelectionAndCopyIsolation(t *testing.T) {
	ctx := context.Background()
	l := charts.Defaults()
	for _, entry := range charts.Catalog() {
		d, _, _ := chartfixtures.Fixture(entry.Kind, "binding")
		original, _ := json.Marshal(d)
		want, err := charts.Select(ctx, d, l)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 12 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := charts.Select(ctx, d, l)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Errorf("unstable selection: %v", err)
				}
				got.Selected.Mapping.Columns[0].Name = "mutated"
			}()
		}
		wg.Wait()
		after, _ := json.Marshal(d)
		if string(after) != string(original) {
			t.Fatal("selection mutated input")
		}
	}
	d, m := bind(t, charts.Table)
	m.Bindings.Columns[0] = "changed"
	if d.Columns[0].ID == "changed" {
		t.Fatal("mapping aliases data")
	}
	out, err := charts.Select(ctx, charts.Data{Version: 1, Columns: []charts.Column{{ID: "text", Name: "Text", Type: "text", Role: "dimension", Provenance: charts.Provenance{Version: 1}}}, Rows: [][]charts.Cell{{{Value: "no measures"}}}, Completeness: charts.Completeness{Status: "complete_result"}}, l)
	if err != nil || !out.Fallback || out.Selected.Mapping.Kind != charts.Table || out.Reason != "no_suitable_chart_above_floor" {
		t.Fatalf("unlabeled fallback: %+v %v", out, err)
	}
	d, _, _ = chartfixtures.Fixture(charts.Line, "binding")
	sel, err := charts.Select(ctx, d, l)
	if err != nil || sel.Selected.Mapping.Kind != charts.Line {
		t.Fatalf("temporal rule %+v %v", sel, err)
	}
	l.SelectionFloor = 100
	sel, err = charts.Select(ctx, d, l)
	if err != nil || !sel.Fallback {
		t.Fatal("floor ignored")
	}
}

func TestExactLabelsTotalsAndPrecisionLoss(t *testing.T) {
	ctx := context.Background()
	l := charts.Defaults()
	d, m := bind(t, charts.Bar)
	d.Rows[0][1].Value = "9007199254740993.0100"
	d.Rows[1][1].Value = "0.0200"
	d.Rows[2][1].Value = "-0.0001"
	d.Completeness = charts.Completeness{Status: "truncated", Reason: "rows"}
	out, err := charts.Build(ctx, d, m, l)
	if err != nil {
		t.Fatal(err)
	}
	if out.Points[0].Value.Exact != "9007199254740993.0100" || !out.Points[0].Value.Approximate || out.Totals[0].Value.Value != "9007199254740993.0299" || out.Totals[0].Scope != "returned_rows" {
		t.Fatalf("lossy labels/totals: %+v", out)
	}
	for _, n := range []string{"1e1000", "1e-1000"} {
		d.Rows[0][1].Value = n
		if _, err = charts.Build(ctx, d, m, l); !errors.Is(err, charts.ErrUnsuitable) {
			t.Fatalf("invalid geometry for %s: %v", n, err)
		}
		m.Kind = charts.Table
		m.Bindings = charts.Bindings{Columns: []string{"category", "value"}}
		out, err = charts.Build(ctx, d, m, l)
		if err != nil || out.Rows[0][1].Value != n {
			t.Fatalf("table exact text %s: %v", n, err)
		}
		m.Kind = charts.Bar
		m.Bindings = charts.Bindings{Category: "category", Value: "value"}
	}
	d, m = bind(t, charts.KPI)
	d.Columns[1].Type = "integer"
	m.Columns[0] = d.Columns[1]
	d.Rows[0][1].Value = "9223372036854775807"
	out, err = charts.Build(ctx, d, m, l)
	if err != nil || out.Points[0].Value.Exact != "9223372036854775807" || out.Totals[0].Value.Value != "9223372036854775807" {
		t.Fatalf("integer roundtrip: %+v %v", out, err)
	}
	d, m = bind(t, charts.KPI)
	d.Columns[1].Format.Percent = "fraction"
	m.Columns[0] = d.Columns[1]
	out, err = charts.Build(ctx, d, m, l)
	if err != nil || len(out.Totals) != 0 {
		t.Fatal("percent summed")
	}
	for _, k := range []charts.Kind{charts.Pie, charts.Donut, charts.Treemap} {
		d, m = bind(t, k)
		for i := range d.Rows {
			d.Rows[i][1].Value = "0"
		}
		out, err = charts.Build(ctx, d, m, l)
		if err != nil || out.State != "no_positive_values" {
			t.Fatal("zero parts invented", err)
		}
	}
}

func TestSavedMappingAndReviewOnlyRebinding(t *testing.T) {
	ctx := context.Background()
	l := charts.Defaults()
	d, m := bind(t, charts.Bar)
	original, _ := json.Marshal(m)
	d.Columns[1].ID = "amount_v2"
	d.Columns[1].Name = "New label"
	d.Columns[1].Provenance.TopicVersion = "v2"
	d.Columns[1].Provenance.SourceRevision = 2
	if _, err := charts.Build(ctx, d, m, l); !errors.Is(err, charts.ErrMappingChanged) {
		t.Fatal("approved mapping silently rebound", err)
	}
	proposal, err := charts.Rebind(ctx, d, m, l)
	if err != nil || proposal.Status != "review_required" || proposal.Mapping.Bindings.Value != "amount_v2" || len(proposal.Changes) != 1 {
		t.Fatalf("proposal: %+v %v", proposal, err)
	}
	after, _ := json.Marshal(m)
	if string(after) != string(original) {
		t.Fatal("approved mapping mutated")
	}
	if _, err = charts.Build(ctx, d, proposal.Mapping, l); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*charts.Data){
		func(d *charts.Data) { d.Columns[1].Format.Currency = "USD" }, func(d *charts.Data) { d.Columns[1].Provenance.Source = "other" }, func(d *charts.Data) { d.Columns[1].Provenance.SemanticID = "different" }, func(d *charts.Data) { d.Columns[1].Aggregation = "average" }, func(d *charts.Data) { d.Columns[1].Type = "number" },
	} {
		next := d
		next.Columns = append([]charts.Column{}, d.Columns...)
		edit(&next)
		if _, err = charts.Rebind(ctx, next, m, l); !errors.Is(err, charts.ErrMappingChanged) {
			t.Fatal("incompatible rebind accepted", err)
		}
	}
	duplicate := d.Columns[1]
	duplicate.ID = "ambiguous"
	d.Columns = append(d.Columns, duplicate)
	for i := range d.Rows {
		d.Rows[i] = append(d.Rows[i], d.Rows[i][1])
	}
	if _, err = charts.Rebind(ctx, d, m, l); !errors.Is(err, charts.ErrMappingChanged) {
		t.Fatal("ambiguous rebind")
	}
	d, m = bind(t, charts.Table)
	m.Columns[0], m.Columns[1] = m.Columns[1], m.Columns[0]
	if _, err = charts.Build(ctx, d, m, l); !errors.Is(err, charts.ErrInvalid) {
		t.Fatal("row/metadata order confused")
	}
	d, m = bind(t, charts.Line)
	d.Rows[0][0].Value = "2026-09-01T01:00:00+01:00"
	d.Rows[1][0].Value = "2026-09-01T00:00:00Z"
	if _, err = charts.Build(ctx, d, m, l); !errors.Is(err, charts.ErrUnsuitable) {
		t.Fatal("duplicate temporal coordinate")
	}
}

func TestBoundedInvalidDataAndMappings(t *testing.T) {
	ctx := context.Background()
	l := charts.Defaults()
	for _, edit := range []func(*charts.Data){
		func(d *charts.Data) { d.Version = 0 }, func(d *charts.Data) { d.Columns = nil }, func(d *charts.Data) { d.Columns[1].ID = d.Columns[0].ID }, func(d *charts.Data) { d.Rows[0] = d.Rows[0][:1] }, func(d *charts.Data) { d.Rows[0][1] = charts.Cell{Null: true, Value: "not null"} },
		func(d *charts.Data) { d.Completeness = charts.Completeness{Status: "full_source"} }, func(d *charts.Data) { d.Completeness.Reason = "rows" }, func(d *charts.Data) { d.Completeness = charts.Completeness{Status: "truncated"} },
		func(d *charts.Data) { d.Columns[1].Type = "javascript" }, func(d *charts.Data) { d.Columns[1].Role = "invalid" }, func(d *charts.Data) { d.Columns[1].Provenance.Version = 0 }, func(d *charts.Data) { d.Columns[1].Provenance.SourceRevision = 0 }, func(d *charts.Data) { d.Columns[1].Provenance.TopicVersion = "" },
		func(d *charts.Data) { d.Columns[1].Format.Currency = "usd" }, func(d *charts.Data) { d.Columns[1].Format.Currency = "US" }, func(d *charts.Data) { d.Columns[1].Format.Currency = "USD"; d.Columns[1].Format.Percent = "whole" }, func(d *charts.Data) { d.Columns[1].Format.FractionDigits = 21 }, func(d *charts.Data) { d.Columns[0].Aggregation = "sum" }, func(d *charts.Data) { d.Columns[1].Grain = "day" },
		func(d *charts.Data) { d.Columns[1].ID = "bad/id" }, func(d *charts.Data) { d.Columns[1].Name = "" }, func(d *charts.Data) { d.Columns[1].Name = "\x00" }, func(d *charts.Data) { d.Rows[0][0].Value = "\xff" },
	} {
		d, _, _ := chartfixtures.Fixture(charts.Bar, "binding")
		edit(&d)
		if charts.ValidateData(ctx, d, l) == nil {
			t.Fatalf("invalid data accepted: %+v", d)
		}
	}
	for _, value := range []string{"NaN", "Infinity", "1/2", "", "+", ".1", "1.", "1e", "1e99999", "1e999999", "0x1", "1e+1x", "-", "1.2.3"} {
		d, _, _ := chartfixtures.Fixture(charts.Bar, "binding")
		d.Rows[0][1].Value = value
		if charts.ValidateData(ctx, d, l) == nil {
			t.Fatalf("invalid numeric %q", value)
		}
	}
	for _, edit := range []func(*charts.Mapping){
		func(m *charts.Mapping) { m.Version = 0 }, func(m *charts.Mapping) { m.Kind = "script" }, func(m *charts.Mapping) { m.Columns = nil }, func(m *charts.Mapping) { m.Bindings.Value = "" }, func(m *charts.Mapping) { m.Bindings.X = "x" }, func(m *charts.Mapping) { m.Bindings.Value = "missing" }, func(m *charts.Mapping) { m.Bindings.Value = "category" },
		func(m *charts.Mapping) { m.Order = []charts.Order{{Column: "missing", Direction: "asc"}} }, func(m *charts.Mapping) { m.Order = []charts.Order{{Column: "value", Direction: "invalid"}} }, func(m *charts.Mapping) {
			m.Order = []charts.Order{{Column: "value", Direction: "asc"}, {Column: "value", Direction: "desc"}}
		},
		func(m *charts.Mapping) { m.Options.Title = "<script>alert(1)</script>" }, func(m *charts.Mapping) { m.Options.Title = "https://remote.example" }, func(m *charts.Mapping) { m.Options.Title = "javascript:alert(1)" }, func(m *charts.Mapping) { m.Options.Title = "data:text/html,x" }, func(m *charts.Mapping) { m.Options.Legend.Position = "https://remote.example" }, func(m *charts.Mapping) { m.Options.LabelMaxRunes = 0 },
	} {
		d, m := bind(t, charts.Bar)
		edit(&m)
		if charts.ValidateMapping(ctx, d, m, l) == nil {
			t.Fatalf("invalid mapping accepted: %+v", m)
		}
	}
	for _, edit := range []func(*charts.Limits){func(l *charts.Limits) { l.MaxRows = 0 }, func(l *charts.Limits) { l.MaxColumns = 0 }, func(l *charts.Limits) { l.MaxBytes = 0 }, func(l *charts.Limits) { l.MaxCellBytes = 0 }, func(l *charts.Limits) { l.MaxCategories = 0 }, func(l *charts.Limits) { l.MaxSeries = 0 }, func(l *charts.Limits) { l.MaxAlternatives = 14 }, func(l *charts.Limits) { l.SelectionFloor = 0 }, func(l *charts.Limits) { l.MaxOptionsBytes = 0 }, func(l *charts.Limits) { l.MaxOptionsDepth = 0 }} {
		bad := l
		edit(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid limit")
		}
	}
	d, m := bind(t, charts.Bar)
	for _, edit := range []func(*charts.Limits){func(l *charts.Limits) { l.MaxRows = 1 }, func(l *charts.Limits) { l.MaxColumns = 1 }, func(l *charts.Limits) { l.MaxBytes = 1024 }, func(l *charts.Limits) { l.MaxCellBytes = 16 }} {
		small := l
		edit(&small)
		d.Rows[0][0].Value = strings.Repeat("x", 20)
		if charts.ValidateData(ctx, d, small) == nil {
			t.Fatal("size bound ignored")
		}
	}
	l = charts.Defaults()
	l.MaxOptionsDepth = 1
	if _, err := charts.Build(ctx, d, m, l); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("depth bound")
	}
	l = charts.Defaults()
	l.MaxOptionsBytes = 128
	m.Options.Title = strings.Repeat("x", 200)
	if _, err := charts.Build(ctx, d, m, l); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("option byte bound")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := charts.Select(cancelled, d, charts.Defaults()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
	if charts.ValidateData(nil, d, charts.Defaults()) == nil {
		t.Fatal("nil context")
	}
}

func TestOrderingCategorySeriesAndEmptyTotals(t *testing.T) {
	ctx := context.Background()
	l := charts.Defaults()
	d, m := bind(t, charts.Table)
	m.Order = []charts.Order{{Column: "value", Direction: "desc"}, {Column: "category", Direction: "asc"}}
	d.Rows[0][1].Value = "10"
	d.Rows[1][1].Value = "2"
	d.Rows[2][1] = charts.Cell{Null: true}
	out, err := charts.Build(ctx, d, m, l)
	if err != nil || out.Rows[0][1].Value != "10" || !out.Rows[2][1].Null {
		t.Fatal("numeric/null ordering", err)
	}
	for i := range d.Rows {
		d.Rows[i][1] = charts.Cell{Null: true}
	}
	out, err = charts.Build(ctx, d, m, l)
	if err != nil || !out.Totals[0].Value.Null {
		t.Fatal("all-null total fabricated", err)
	}
	d, m = bind(t, charts.GroupedBar)
	l.MaxSeries = 1
	if _, err = charts.Build(ctx, d, m, l); !errors.Is(err, charts.ErrUnsuitable) {
		t.Fatal("series cap")
	}
	l = charts.Defaults()
	l.MaxCategories = 1
	if _, err = charts.Build(ctx, d, m, l); !errors.Is(err, charts.ErrUnsuitable) {
		t.Fatal("category cap")
	}
	d, m = bind(t, charts.Bar)
	d.Rows[1][0] = d.Rows[0][0]
	if _, err = charts.Build(ctx, d, m, charts.Defaults()); !errors.Is(err, charts.ErrUnsuitable) {
		t.Fatal("duplicate aggregation invented")
	}
	d, m = bind(t, charts.Treemap)
	d.Rows[0][2] = d.Rows[0][0]
	if _, err = charts.Build(ctx, d, m, charts.Defaults()); !errors.Is(err, charts.ErrUnsuitable) {
		t.Fatal("self-parent accepted")
	}
	d, m = bind(t, charts.Line)
	d.Rows[0][0].Value = "not a timestamp"
	if _, err = charts.Build(ctx, d, m, charts.Defaults()); !errors.Is(err, charts.ErrUnsuitable) {
		t.Fatal("invalid time accepted")
	}
}

func FuzzDataAndMappings(f *testing.F) {
	for _, k := range charts.Catalog() {
		d, b, o := chartfixtures.Fixture(k.Kind, "binding")
		m, err := charts.Bind(context.Background(), d, k.Kind, b, o, charts.DefaultOptions(), charts.Defaults())
		if err != nil {
			f.Fatal(err)
		}
		raw, _ := json.Marshal(struct {
			Data    charts.Data
			Mapping charts.Mapping
		}{d, m})
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		var in struct {
			Data    charts.Data
			Mapping charts.Mapping
		}
		if json.Unmarshal(raw, &in) != nil {
			return
		}
		l := charts.Defaults()
		l.MaxRows = 20
		l.MaxColumns = 8
		l.MaxBytes = 32768
		_, _ = charts.Select(context.Background(), in.Data, l)
		_, _ = charts.Build(context.Background(), in.Data, in.Mapping, l)
		_, _ = charts.Rebind(context.Background(), in.Data, in.Mapping, l)
	})
}

func TestEncodedCellsAndPreallocationBounds(t *testing.T) {
	ctx := context.Background()
	limits := charts.Defaults()
	for _, test := range []struct {
		typ, value string
		valid      bool
	}{
		{"binary", "cafe", true}, {"binary", "", true}, {"binary", "CAFE", true},
		{"binary", "a", false}, {"binary", "AQID", false}, {"structured", `{"n":9007199254740993}`, true},
		{"structured", "null", true}, {"structured", "[", false},
	} {
		d := charts.Data{Version: 1, Columns: []charts.Column{{ID: "c", Name: "Value", Type: test.typ, Role: "unknown", Provenance: charts.Provenance{Version: 1}}}, Rows: [][]charts.Cell{{{Value: test.value}}}, Completeness: charts.Completeness{Status: "complete_result"}}
		if err := charts.ValidateData(ctx, d, limits); (err == nil) != test.valid {
			t.Fatalf("%s/%s: %v", test.typ, test.value, err)
		}
	}
	d, m := bind(t, charts.Table)
	for _, title := range []string{"ftp://example.invalid/file", "//example.invalid", "file:/tmp/file", "blob:private"} {
		m.Options.Title = title
		if err := charts.ValidateMapping(ctx, d, m, limits); !errors.Is(err, charts.ErrInvalid) {
			t.Fatal("unsafe option", title, err)
		}
	}
	if _, err := charts.Bind(ctx, d, charts.Table, charts.Bindings{Columns: make([]string, limits.MaxColumns+1)}, nil, charts.DefaultOptions(), limits); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("binding list not bounded before construction", err)
	}
	if _, err := charts.Bind(ctx, d, charts.Table, charts.Bindings{Columns: []string{d.Columns[0].ID}}, make([]charts.Order, limits.MaxColumns+1), charts.DefaultOptions(), limits); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("order list not bounded", err)
	}
}
