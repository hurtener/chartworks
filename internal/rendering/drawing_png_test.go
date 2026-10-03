package rendering

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func TestPNGRasterCoversCatalogAndThemes(t *testing.T) {
	for _, kind := range sceneCatalog {
		for _, theme := range []string{"light", "dark"} {
			t.Run(string(kind)+"/"+theme, func(t *testing.T) {
				g := chartfixtures.Produce(kind, "binding")
				out := &reporting.ViewerOutput{Chart: g.Output}
				s, err := outputDrawing(t.Context(), out, theme, 800, 420, "UTC", nil)
				if err != nil {
					t.Fatal(err)
				}
				data, err := rasterScene(t.Context(), s, 800, 420, 2<<20)
				if err != nil {
					t.Fatal(err)
				}
				again, err := rasterScene(t.Context(), s, 800, 420, 2<<20)
				if err != nil || !bytes.Equal(data, again) {
					t.Fatal("nondeterministic PNG", err)
				}
				im, err := png.Decode(bytes.NewReader(data))
				if err != nil || im.Bounds().Dx() != 800 || im.Bounds().Dy() != 420 {
					t.Fatal("invalid PNG", err)
				}
				base := im.At(0, 0)
				distinct := false
				for y := 0; y < 420 && !distinct; y += 2 {
					for x := 0; x < 800; x += 2 {
						if im.At(x, y) != base {
							distinct = true
							break
						}
					}
				}
				if !distinct {
					t.Fatal("empty raster instead of chart")
				}
			})
		}
	}
	table := &reporting.ViewerOutput{Table: &reporting.ViewerTable{Columns: []charts.Column{{ID: "name", Name: "Nombre", Type: "text"}, {ID: "value", Name: "Importe", Type: "decimal", Format: charts.Format{FractionDigits: 3}}}, Rows: [][]charts.Cell{{{Value: "Español"}, {Value: "9007199254740993.125"}}, {{Value: "unknown"}, {Null: true}}, {{Value: "zero"}, {Value: "0"}}}}}
	s, err := outputDrawing(t.Context(), table, "light", 800, 420, "UTC", []string{"incomplete: unknown count 1"})
	if err != nil {
		t.Fatal(err)
	}
	seenExact, seenDisclosure := false, false
	for _, p := range s.nodes {
		seenExact = seenExact || strings.Contains(strings.ReplaceAll(p.text, ",", ""), "9007199254740993.125")
		seenDisclosure = seenDisclosure || strings.Contains(p.text, "unknown count 1")
	}
	if !seenExact || !seenDisclosure {
		t.Fatal("exact amount or disclosure lost")
	}
	if _, err := rasterScene(t.Context(), s, 800, 420, 2<<20); err != nil {
		t.Fatal(err)
	}
}
func TestPNGRasterBudgetsAndCancellation(t *testing.T) {
	s := &drawingScene{}
	s.add(drawingPrimitive{kind: "rect", class: "background", fill: "#ffffff", w: 800, h: 420})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := rasterScene(ctx, s, 800, 420, 2<<20); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored", err)
	}
	for _, test := range []struct{ w, h, b int }{{4097, 420, 1 << 20}, {800, 4097, 1 << 20}, {0, 420, 1 << 20}, {800, 420, 1}} {
		if raw, err := rasterScene(t.Context(), s, test.w, test.h, test.b); err == nil || raw != nil {
			t.Fatal("raster budget ignored", test)
		}
	}
	for _, label := range []string{"東京", "unrepresentable\nlabel", strings.Repeat("W", 1000)} {
		bad := &drawingScene{}
		bad.add(drawingPrimitive{kind: "text", x: 8, y: 22, text: label})
		if _, err := rasterScene(t.Context(), bad, 800, 420, 1<<20); err == nil {
			t.Fatal("silently lost glyph or clipped label")
		}
	}
	clipped := &drawingScene{}
	clipped.add(drawingPrimitive{kind: "text", x: 8, y: 419, text: "descending glyphs"})
	if _, err := rasterScene(t.Context(), clipped, 800, 420, 1<<20); err == nil {
		t.Fatal("descenders silently clipped")
	}
	table := &reporting.ViewerOutput{Table: &reporting.ViewerTable{Columns: []charts.Column{{ID: "value", Name: "Value", Type: "integer"}}, Rows: make([][]charts.Cell, 100)}}
	if _, err := outputDrawing(t.Context(), table, "light", 800, 200, "UTC", nil); err == nil {
		t.Fatal("table silently cropped")
	}
}

func TestPNGRichKPIAndTableTotals(t *testing.T) {
	c := charts.Column{ID: "actual", Name: "actual", DisplayLabel: "Ingresos", Type: "decimal", Role: "measure", Aggregation: "sum", Provenance: charts.Provenance{Version: 1}, Format: charts.Format{Currency: "USD", CurrencySymbol: "US$", Locale: "es-AR", FractionDigits: 2}}
	baseline, target := c, c
	baseline.ID, baseline.Name = "baseline", "baseline"
	target.ID, target.Name = "target", "target"
	d := charts.Data{Version: 1, Columns: []charts.Column{{ID: "period", Name: "period", Type: "temporal", Role: "time", Grain: "month", Format: charts.Format{DatePattern: "year_month"}, Provenance: charts.Provenance{Version: 1}}, c, baseline, target}, Rows: [][]charts.Cell{{{Value: "2026-01"}, {Value: "1"}, {Value: "0"}, {Value: "1500"}}, {{Value: "2026-02"}, {Value: "1234.567"}, {Value: "0"}, {Value: "1500"}}}, Completeness: charts.Completeness{Status: "complete_result"}}
	options := charts.KPIOptions{ValueRow: "last", ComparisonMode: "comparison_column", ShowDelta: true, ShowPercentDelta: true, ShowTargetDifference: true, Sparkline: true}
	mapping, err := charts.BindDisplay(t.Context(), d, charts.KPI, charts.Bindings{Category: "period", Value: "actual", Comparison: "baseline", Target: "target"}, []charts.Order{{Column: "period", Direction: "asc"}}, charts.DefaultOptions(), &options, nil, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	built, err := charts.Build(t.Context(), d, mapping, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	out := &reporting.ViewerOutput{Chart: &built}
	scene, err := outputDrawing(t.Context(), out, "dark", 800, 420, "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	exact := false
	for _, p := range scene.nodes {
		exact = exact || strings.Contains(p.text, "1.234,57 US$")
	}
	if !exact {
		t.Fatal("KPI exact reviewed formatting lost")
	}
	for _, prefix := range []string{"Value: ", "Comparison: ", "Change: ", "Percent change: unavailable", "Target: ", "Difference from target: ", "Trend: "} {
		found := false
		for _, p := range scene.nodes {
			found = found || strings.HasPrefix(p.text, prefix)
		}
		if !found {
			t.Fatal("KPI role lost", prefix)
		}
	}

	if _, err = rasterScene(t.Context(), scene, 800, 420, 2<<20); err != nil {
		t.Fatal(err)
	}
	table := &reporting.ViewerOutput{Table: &reporting.ViewerTable{Columns: []charts.Column{c}, Rows: [][]charts.Cell{{{Value: "1234.567"}}}, Totals: []charts.Total{{Column: "actual", Value: charts.Cell{Value: "1234.567"}, Scope: "complete_result"}}}}
	scene, err = outputDrawing(t.Context(), table, "light", 800, 420, "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := false
	for _, p := range scene.nodes {
		scope = scope || strings.Contains(p.text, "complete_result")
	}
	if !scope {
		t.Fatal("table total scope omitted")
	}
	if _, err = rasterScene(t.Context(), scene, 800, 420, 2<<20); err != nil {
		t.Fatal(err)
	}
}

func TestPNGRasterPreservesResultMeaning(t *testing.T) {
	for _, test := range []struct {
		kind           charts.Kind
		scenario, want string
	}{{charts.Bar, "empty", "No rows in this result"}, {charts.Bar, "null", "No values to plot"}, {charts.Pie, "binding", "No positive values to plot"}} {
		g := chartfixtures.Produce(test.kind, test.scenario)
		if test.kind == charts.Pie || test.scenario == "null" {
			d, b, o := chartfixtures.Fixture(test.kind, "binding")
			m, err := charts.Bind(t.Context(), d, test.kind, b, o, charts.DefaultOptions(), charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			for i := range d.Rows {
				d.Rows[i][1] = charts.Cell{Value: "0"}
				if test.scenario == "null" {
					d.Rows[i][1] = charts.Cell{Null: true}
				}
			}
			built, err := charts.Build(t.Context(), d, m, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			g.Output = &built
		}
		out := &reporting.ViewerOutput{Chart: g.Output}
		scene, err := outputDrawing(t.Context(), out, "light", 800, 420, "UTC", nil)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range scene.nodes {
			found = found || strings.Contains(p.text, test.want)
		}
		if !found {
			t.Fatal("state hidden", test.want)
		}
		if _, err = rasterScene(t.Context(), scene, 800, 420, 2<<20); err != nil {
			t.Fatal(err)
		}
	}
	g := chartfixtures.Produce(charts.Bar, "binding")
	g.Output.Completeness = charts.Completeness{Status: "truncated", Reason: "rows"}
	g.Output.OmittedRows = 2
	g.Output.Warnings = []string{"null_values_omitted"}
	g.Output.Mapping.Options.Title = "Reviewed amount by group"
	scene, err := outputDrawing(t.Context(), &reporting.ViewerOutput{Chart: g.Output}, "light", 800, 420, "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Truncated query result: rows", "Rows omitted from chart: 2", "Warning: null values omitted", "Reviewed amount by group"} {
		found := false
		for _, p := range scene.nodes {
			found = found || p.text == want
		}
		if !found {
			t.Fatal("meaning omitted", want)
		}
	}
	if _, err = rasterScene(t.Context(), scene, 800, 420, 2<<20); err != nil {
		t.Fatal(err)
	}
}

func TestPNGEmptyPageDoesNotInventRowRange(t *testing.T) {
	if got := rasterPageScope(Projection{Offset: 3, Total: 3}, 0); got != "Displayed rows: 0 of 3 retained" {
		t.Fatal(got)
	}
	if got := rasterPageScope(Projection{Offset: 2, Total: 3}, 1); got != "Displayed rows: 3-3 of 3 retained" {
		t.Fatal(got)
	}
}

func TestPNGRejectsMalformedRetainedMeaning(t *testing.T) {
	for _, change := range []func(*charts.Output){func(c *charts.Output) { c.State = "forged" }, func(c *charts.Output) { c.OmittedRows = -1 }, func(c *charts.Output) { c.OmittedRows = 100001 }, func(c *charts.Output) {
		c.Completeness = charts.Completeness{Status: "complete_result", Reason: "rows"}
	}, func(c *charts.Output) { c.Completeness = charts.Completeness{Status: "truncated", Reason: "forged"} }, func(c *charts.Output) { c.Completeness.Status = "forged" }, func(c *charts.Output) { c.Warnings = make([]string, 129) }, func(c *charts.Output) { c.Warnings = []string{strings.Repeat("x", 4097)} }, func(c *charts.Output) { c.Points = make([]charts.Point, 129) }} {
		g := chartfixtures.Produce(charts.Bar, "binding")
		change(g.Output)
		if _, err := outputDrawing(t.Context(), &reporting.ViewerOutput{Chart: g.Output}, "light", 800, 420, "UTC", nil); err == nil {
			t.Fatal("invalid retained meaning rasterized")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	g := chartfixtures.Produce(charts.Bar, "binding")
	if _, err := outputDrawing(ctx, &reporting.ViewerOutput{Chart: g.Output}, "light", 800, 420, "UTC", nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled output still prepared", err)
	}
}

// A retained projection is still an input boundary: malformed scope or table
// metadata must never be converted into a plausible raster.
func TestPNGRasterRejectsMalformedRetainedProjection(t *testing.T) {
	column := charts.Column{ID: "amount", Name: "Amount", Type: "decimal"}
	cases := map[string]*reporting.ViewerOutput{
		"missing output":         {},
		"missing columns":        {Table: &reporting.ViewerTable{}},
		"row width":              {Table: &reporting.ViewerTable{Columns: []charts.Column{column}, Rows: [][]charts.Cell{{{Value: "1"}, {Value: "2"}}}}},
		"unknown total field":    {Table: &reporting.ViewerTable{Columns: []charts.Column{column}, Totals: []charts.Total{{Column: "other"}}}},
		"invalid state":          {Chart: &charts.Output{State: "invented"}},
		"negative omitted rows":  {Chart: &charts.Output{OmittedRows: -1}},
		"excess omitted rows":    {Chart: &charts.Output{OmittedRows: 100001}},
		"contradictory coverage": {Table: &reporting.ViewerTable{Completeness: charts.Completeness{Status: "complete_result", Reason: "rows"}}},
		"unknown truncation":     {Table: &reporting.ViewerTable{Completeness: charts.Completeness{Status: "truncated", Reason: "invented"}}},
		"unknown coverage":       {Table: &reporting.ViewerTable{Completeness: charts.Completeness{Status: "invented"}}},
		"excess warnings":        {Table: &reporting.ViewerTable{Warnings: make([]string, 129)}},
		"oversized warning":      {Table: &reporting.ViewerTable{Warnings: []string{strings.Repeat("x", 4097)}}},
		"kind mismatch":          {Chart: &charts.Output{Kind: charts.Bar}},
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			if scene, err := outputDrawing(t.Context(), out, "light", 800, 420, "UTC", nil); !errors.Is(err, ErrInvalid) || scene != nil {
				t.Fatalf("malformed retained projection admitted: %v", err)
			}
		})
	}
}

func TestPNGRasterRejectsUnrepresentableDisclosure(t *testing.T) {
	unsupported := &charts.Output{Kind: charts.Kind("unsupported")}
	unsupported.Mapping.Kind = unsupported.Kind
	if scene, err := outputDrawing(t.Context(), &reporting.ViewerOutput{Chart: unsupported}, "light", 800, 420, "UTC", nil); !errors.Is(err, ErrInvalid) || scene != nil {
		t.Fatalf("unsupported chart rasterized: %v", err)
	}
	out := &reporting.ViewerOutput{Chart: chartfixtures.Produce(charts.Bar, "binding").Output}
	for _, lines := range [][]string{make([]string, 129), {strings.Repeat("important amount disclosure ", 200)}} {
		if scene, err := outputDrawing(t.Context(), out, "light", 320, 200, "UTC", lines); !errors.Is(err, ErrInvalid) || scene != nil {
			t.Fatalf("disclosure cropped to fit: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if scene, err := outputDrawing(ctx, out, "light", 800, 420, "UTC", nil); !errors.Is(err, context.Canceled) || scene != nil {
		t.Fatalf("canceled drawing constructed: %v", err)
	}
	if scene, err := outputDrawing(t.Context(), out, "light", 320, 200, "UTC", make([]string, 8)); !errors.Is(err, ErrInvalid) || scene != nil {
		t.Fatalf("undersized content area admitted: %v", err)
	}
}

func TestPNGRasterKPIKeepsTitleAndRejectsClippedRoles(t *testing.T) {
	c := chartfixtures.Produce(charts.KPI, "binding").Output
	c.Mapping.Options.Title = "Reviewed revenue"
	out := &reporting.ViewerOutput{Chart: c}
	scene, err := outputDrawing(t.Context(), out, "light", 800, 420, "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range scene.nodes {
		found = found || p.text == "Reviewed revenue"
	}
	if !found {
		t.Fatal("KPI title omitted")
	}
	if scene, err := outputDrawing(t.Context(), out, "light", 800, 200, "UTC", make([]string, 4)); !errors.Is(err, ErrInvalid) || scene != nil {
		t.Fatalf("KPI roles clipped: %v", err)
	}
}
