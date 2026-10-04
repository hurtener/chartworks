package rendering

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"html"
	"image"
	"image/png"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func TestPresentationTableNativeRenderersPreserveExactRetainedData(t *testing.T) {
	d := charts.Data{Version: charts.Version, Columns: []charts.Column{
		{ID: "group", Name: "group", DisplayLabel: "Group", Type: "text", Role: "dimension", Provenance: charts.Provenance{Version: 1}},
		{ID: "amount", Name: "amount", DisplayLabel: "Reviewed amount", Type: "decimal", Role: "measure", Aggregation: "sum", Format: charts.Format{FractionDigits: 5, Currency: "USD", CurrencySymbol: "US$", Locale: "en-US"}, Provenance: charts.Provenance{Version: 1}},
		{ID: "hidden", Name: "hidden", Type: "decimal", Role: "measure", Provenance: charts.Provenance{Version: 1}},
	}, Rows: [][]charts.Cell{
		{{Value: "First"}, {Value: "9007199254740993.12567"}, {Value: "99"}},
		{{Value: "Second"}, {Value: "0.00001"}, {Value: "100"}},
	}, Completeness: charts.Completeness{Status: "complete_result"}}
	options := charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "group", Visible: true}, {Column: "amount", Visible: true}, {Column: "hidden", Visible: false}}, PageSize: 25, ShowTotals: true}
	m, err := charts.BindDisplay(t.Context(), d, charts.Table, charts.Bindings{Columns: []string{"group", "amount", "hidden"}}, nil, charts.DefaultOptions(), nil, &options, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	beforeData, beforeMapping := presentationJSON(t, d), presentationJSON(t, m)
	base := presentationBuild(t, d, m)
	displayLabel, digits := "Amount <reviewed> & net", 2
	patched, err := charts.ApplyPresentationPatch(t.Context(), m, charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: "amount", Set: &charts.ColumnPresentationSet{DisplayLabel: &displayLabel, FractionDigits: &digits}}}}, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	built := presentationBuild(t, d, patched)
	if !bytes.Equal(beforeData, presentationJSON(t, d)) || !bytes.Equal(beforeMapping, presentationJSON(t, m)) {
		t.Fatal("presentation changed canonical input data or the original mapping")
	}
	presentationAssertCanonical(t, base, built)
	if len(built.Columns) != 2 || built.Columns[1].DisplayLabel != displayLabel || built.Columns[1].Format.FractionDigits != digits {
		t.Fatal("visible output columns did not receive the presentation", built.Columns)
	}
	if built.Rows[0][1].Value != "9007199254740993.12567" || len(built.Totals) != 1 || built.Totals[0].Value.Value != "9007199254740993.12568" {
		t.Fatal("large exact amount or total changed", built.Rows, built.Totals)
	}
	out, baseOut := presentationRetainedViewer(t, built), presentationRetainedViewer(t, base)
	wantAmount := "9,007,199,254,740,993.13 US$"
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			presentationAssertStatic(t, out, theme, displayLabel, wantAmount)
			scene, raster := presentationRaster(t, out, theme)
			presentationAssertSceneText(t, scene, displayLabel, wantAmount, displayLabel+": "+wantAmount+" (complete_result)")
			_, baseline := presentationRaster(t, baseOut, theme)
			// Check actual decoded pixels in the changed header and value rows.
			// Text primitives alone would not prove that the rasterizer drew them.
			presentationAssertPixelChange(t, baseline, raster, image.Rect(400, 4, 800, 26))
			presentationAssertPixelChange(t, baseline, raster, image.Rect(400, 28, 800, 50))
		})
	}
	csvData, err := renderCSV(out, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(csvData)).ReadAll()
	if err != nil || records[0][1] != displayLabel || records[1][1] != wantAmount {
		t.Fatal("CSV did not use the same retained presentation", records, err)
	}
	reset, err := charts.ApplyPresentationPatch(t.Context(), patched, charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: "amount", Reset: []charts.PresentationField{"display_label", "fraction_digits"}}}}, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	restored := presentationBuild(t, d, reset)
	if !bytes.Equal(presentationJSON(t, base), presentationJSON(t, restored)) {
		t.Fatal("reset did not restore the canonical native output")
	}
	_, restoredPNG := presentationRaster(t, presentationRetainedViewer(t, restored), "light")
	_, originalPNG := presentationRaster(t, baseOut, "light")
	presentationAssertPixelsEqual(t, originalPNG, restoredPNG, originalPNG.Bounds())
}

func TestPresentationKPINativeRenderersKeepCanonicalPercentDelta(t *testing.T) {
	d := charts.Data{Version: charts.Version, Columns: []charts.Column{
		{ID: "period", Name: "period", Type: "temporal", Role: "time", Grain: "month", Provenance: charts.Provenance{Version: 1}},
		{ID: "actual", Name: "actual", Type: "decimal", Role: "measure", Format: charts.Format{FractionDigits: 8}, Provenance: charts.Provenance{Version: 1}},
		{ID: "comparison", Name: "comparison", Type: "decimal", Role: "measure", Provenance: charts.Provenance{Version: 1}},
		{ID: "target", Name: "target", Type: "decimal", Role: "measure", Format: charts.Format{FractionDigits: 6}, Provenance: charts.Provenance{Version: 1}},
	}, Rows: [][]charts.Cell{
		{{Value: "2026-01"}, {Value: "100.98765432109876543210"}, {Value: "90"}, {Value: "120"}},
		{{Value: "2026-02"}, {Value: "130.12345678901234567890"}, {Value: "110.00000000000000000000"}, {Value: "125.98765432109876543210"}},
	}, Completeness: charts.Completeness{Status: "complete_result"}}
	options := charts.KPIOptions{ValueRow: "last", ComparisonMode: "comparison_column", ShowDelta: true, ShowPercentDelta: true, ShowTargetDifference: true, Sparkline: true}
	m, err := charts.BindDisplay(t.Context(), d, charts.KPI, charts.Bindings{Category: "period", Value: "actual", Comparison: "comparison", Target: "target"}, []charts.Order{{Column: "period", Direction: "asc"}}, charts.DefaultOptions(), &options, nil, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	base := presentationBuild(t, d, m)
	actualDigits, targetDigits := 2, 4
	patched, err := charts.ApplyPresentationPatch(t.Context(), m, charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{
		{Column: "actual", Set: &charts.ColumnPresentationSet{FractionDigits: &actualDigits}},
		{Column: "target", Set: &charts.ColumnPresentationSet{FractionDigits: &targetDigits}},
	}}, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	built := presentationBuild(t, d, patched)
	presentationAssertCanonical(t, base, built)
	if built.KPIResult == nil || built.KPIResult.Value.Exact != "130.12345678901234567890" || built.KPIResult.Delta == nil || built.KPIResult.Delta.Exact != "20.12345678901234567890" || built.KPIResult.PercentDelta == nil || built.KPIResult.PercentDelta.Exact != "18.29405163" || built.KPIResult.TargetDifference == nil || built.KPIResult.TargetDifference.Exact != "4.13580246791358024680" {
		t.Fatalf("canonical high-precision KPI calculations changed: %+v", built.KPIResult)
	}
	out := presentationRetainedViewer(t, built)
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			presentationAssertStatic(t, out, theme, "130.12", "110.00", "20.12", "18.29405163%", "125.9877", "4.14", "100.99 → 130.12")
			scene, raster := presentationRaster(t, out, theme)
			presentationAssertSceneText(t, scene, "Value: 130.12", "Comparison: 110.00", "Change: 20.12", "Percent change: 18.29405163%", "Target: 125.9877", "Difference from target: 4.14", "Trend: 100.99 → 130.12")
			_, baseline := presentationRaster(t, presentationRetainedViewer(t, base), theme)
			presentationAssertPixelChange(t, baseline, raster, image.Rect(16, 14, 700, 31))
			// Percent delta was calculated using canonical precision. Its visible
			// pixels must remain identical even while the value precision changes.
			presentationAssertPixelsEqual(t, baseline, raster, image.Rect(16, 68, 700, 85))
		})
	}
}

func TestPresentationChartCatalogNativeRendererParity(t *testing.T) {
	for _, kind := range sceneCatalog {
		if kind == charts.KPI { // The richer native KPI path is covered separately.
			continue
		}
		t.Run(string(kind), func(t *testing.T) {
			d, bindings, order := chartfixtures.Fixture(kind, "binding")
			m, err := charts.Bind(t.Context(), d, kind, bindings, order, charts.DefaultOptions(), charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			base := presentationBuild(t, d, m)
			digits := 1
			patched, err := charts.ApplyPresentationPatch(t.Context(), m, charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: "value", Set: &charts.ColumnPresentationSet{FractionDigits: &digits}}}}, charts.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			built := presentationBuild(t, d, patched)
			presentationAssertCanonical(t, base, built)
			out := presentationRetainedViewer(t, built)
			presentationAssertStatic(t, out, "light", "3.3 items")
			scene, raster := presentationRaster(t, out, "light")
			presentationAssertSceneContains(t, scene, "3.3 items")
			_, baseline := presentationRaster(t, presentationRetainedViewer(t, base), "light")
			presentationAssertPixelChange(t, baseline, raster, image.Rect(0, 4, 800, 78))
		})
	}
}

func TestPresentationRichMeasureNativeRendererParity(t *testing.T) {
	d, _, _ := chartfixtures.Fixture(charts.Bar, "binding")
	d.Rows[0][1].Value, d.Rows[0][3].Value = "1.234567890123456789", "6.543219876543219876"
	m, err := charts.Bind(t.Context(), d, charts.Bar, charts.Bindings{Category: "category", Values: []string{"value", "x"}}, nil, charts.DefaultOptions(), charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	base := presentationBuild(t, d, m)
	firstDigits, secondDigits := 2, 4
	patched, err := charts.ApplyPresentationPatch(t.Context(), m, charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{
		{Column: "value", Set: &charts.ColumnPresentationSet{FractionDigits: &firstDigits}},
		{Column: "x", Set: &charts.ColumnPresentationSet{FractionDigits: &secondDigits}},
	}}, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	built := presentationBuild(t, d, patched)
	presentationAssertCanonical(t, base, built)
	if built.Version != charts.RichVersion || len(built.Points) != 6 || built.Points[0].Value.Exact != "1.234567890123456789" {
		t.Fatal("rich transformation or exact point changed", built.Points)
	}
	out := presentationRetainedViewer(t, built)
	presentationAssertStatic(t, out, "light", "1.23 items", "6.5432 items")
	scene, raster := presentationRaster(t, out, "light")
	presentationAssertSceneContains(t, scene, "1.23 items", "6.5432 items")
	_, baseline := presentationRaster(t, presentationRetainedViewer(t, base), "light")
	presentationAssertPixelChange(t, baseline, raster, image.Rect(0, 4, 800, 135))
}

func presentationBuild(t *testing.T, data charts.Data, mapping charts.Mapping) charts.Output {
	t.Helper()
	out, err := charts.Build(t.Context(), data, mapping, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func presentationJSON(t *testing.T, value any) []byte {
	t.Helper()
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func presentationAssertCanonical(t *testing.T, before, after charts.Output) {
	t.Helper()
	if !reflect.DeepEqual(before.Mapping.Columns, after.Mapping.Columns) {
		t.Fatal("presentation mutated canonical column pins")
	}
	// Only output display metadata and the retained overlay may change. This
	// covers exact rows, totals, points, coordinates, series, derived KPI values,
	// ordering, completeness, warnings, and transformation/provenance together.
	after.Columns = before.Columns
	after.Mapping.Presentation = before.Mapping.Presentation
	if !reflect.DeepEqual(before, after) {
		t.Fatal("presentation changed canonical retained calculations or geometry")
	}
}

// This round-trip exercises the actual retained JSON contract before supplying
// public ViewerOutput fields to native renderers. Delivery.projectOutput is a
// reporting-private method; this helper makes no claim to exercise its authority
// or paging path, which belongs to reporting integration tests.
func presentationRetainedViewer(t *testing.T, built charts.Output) *reporting.ViewerOutput {
	t.Helper()
	retained := reporting.RetainedOutput{ID: "synthetic-output", Kind: string(built.Kind), State: "succeeded", Digest: "synthetic-digest", Chart: &built}
	var decoded reporting.RetainedOutput
	if err := json.Unmarshal(presentationJSON(t, retained), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Chart == nil || !bytes.Equal(presentationJSON(t, built), presentationJSON(t, decoded.Chart)) {
		t.Fatal("retained JSON changed the built output")
	}
	out := &reporting.ViewerOutput{ID: decoded.ID, Kind: decoded.Kind, State: decoded.State, RetainedDigest: decoded.Digest}
	if built.Kind == charts.Table {
		c := decoded.Chart
		out.Table = &reporting.ViewerTable{Columns: c.Columns, Rows: c.Rows, Totals: c.Totals, Completeness: c.Completeness, Warnings: c.Warnings}
	} else {
		out.Chart = decoded.Chart
	}
	return out
}

func presentationAssertStatic(t *testing.T, out *reporting.ViewerOutput, theme string, want ...string) {
	t.Helper()
	before := presentationJSON(t, out)
	page := reporting.ViewerPage{Limit: 25}
	if out.Table != nil {
		page.Total = len(out.Table.Rows)
	}
	htmlData, err := renderHTML(out, page, theme, 800, 420, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	svgData, err := renderSVG(out, theme, 800, 420, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, presentationJSON(t, out)) {
		t.Fatal("static rendering changed retained presentation data")
	}
	for _, rendered := range []struct{ kind, data string }{{"html", string(htmlData)}, {"svg", string(svgData)}} {
		if !safeStatic(rendered.kind, rendered.data) {
			t.Fatalf("presentation produced unsafe %s", rendered.kind)
		}
		for _, text := range want {
			if !strings.Contains(rendered.data, html.EscapeString(text)) {
				t.Fatalf("%s omitted presentation text %q: %s", rendered.kind, text, rendered.data)
			}
		}
	}
}

func presentationRaster(t *testing.T, out *reporting.ViewerOutput, theme string) (*drawingScene, image.Image) {
	t.Helper()
	before := presentationJSON(t, out)
	scene, err := outputDrawing(t.Context(), out, theme, 800, 420, "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := rasterScene(t.Context(), scene, 800, 420, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(wire))
	if err != nil || decoded == nil || decoded.Bounds() != image.Rect(0, 0, 800, 420) {
		t.Fatal("invalid native PNG", err)
	}
	if !bytes.Equal(before, presentationJSON(t, out)) {
		t.Fatal("native PNG rendering changed retained presentation data")
	}
	return scene, decoded
}

func presentationAssertSceneText(t *testing.T, scene *drawingScene, want ...string) {
	t.Helper()
	for _, text := range want {
		found := false
		for _, node := range scene.nodes {
			found = found || node.kind == "text" && node.text == text
		}
		if !found {
			t.Fatalf("native raster drawing omitted exact text %q", text)
		}
	}
}

func presentationAssertSceneContains(t *testing.T, scene *drawingScene, want ...string) {
	t.Helper()
	for _, text := range want {
		found := false
		for _, node := range scene.nodes {
			found = found || node.kind == "text" && strings.Contains(node.text, text)
		}
		if !found {
			t.Fatalf("native raster drawing omitted formatted role %q", text)
		}
	}
}

func presentationAssertPixelChange(t *testing.T, before, after image.Image, region image.Rectangle) {
	t.Helper()
	changed, ink := 0, 0
	background := after.At(0, 0)
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if before.At(x, y) != after.At(x, y) {
				changed++
			}
			if after.At(x, y) != background {
				ink++
			}
		}
	}
	if changed == 0 || ink == 0 {
		t.Fatalf("presentation did not change actual visible PNG pixels in %v: changed=%d ink=%d", region, changed, ink)
	}
}

func presentationAssertPixelsEqual(t *testing.T, before, after image.Image, region image.Rectangle) {
	t.Helper()
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if before.At(x, y) != after.At(x, y) {
				t.Fatalf("unchanged presentation pixels differ at (%d,%d)", x, y)
			}
		}
	}
}
