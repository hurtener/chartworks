package rendering

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func TestStaticSVGDrawsEveryChartCatalogKind(t *testing.T) {
	markers := map[charts.Kind]string{
		charts.Area:          `class="area"`,
		charts.Bar:           `class="bar"`,
		charts.ColumnChart:   `class="column"`,
		charts.Donut:         `class="donut-hole"`,
		charts.GroupedBar:    `class="bar"`,
		charts.Heatmap:       `class="heatmap"`,
		charts.KPI:           `class="exact-labels"`,
		charts.Line:          `class="line"`,
		charts.Pie:           `class="slice"`,
		charts.Scatter:       `class="scatter"`,
		charts.StackedBar:    `class="bar"`,
		charts.StackedColumn: `class="column"`,
		charts.Treemap:       `class="treemap"`,
	}
	for kind, marker := range markers {
		t.Run(string(kind), func(t *testing.T) {
			golden := chartfixtures.Produce(kind, "binding")
			if golden.Output == nil {
				t.Fatal("fixture did not produce the advertised chart kind", golden.Error)
			}
			svg, err := renderChartSVG(golden.Output, "light", 800, 420, "UTC")
			if err != nil || !strings.Contains(svg, marker) || !strings.Contains(svg, `data-kind="`+string(kind)+`"`) || strings.Contains(svg, "<script") {
				t.Fatal("catalog kind did not produce static geometry", err, svg)
			}
		})
	}
}

func chartValue(exact string, coordinate float64) charts.Value {
	return charts.Value{Exact: exact, Coordinate: &coordinate}
}
func chartCell(value string) charts.Cell { return charts.Cell{Value: value} }

func TestGroupedAndStackedGeometryUsesCategorySeriesSemantics(t *testing.T) {
	points := []charts.Point{
		{Category: chartCell("A"), CategoryKey: "a", Series: chartCell("S1"), SeriesID: "s1", Value: chartValue("2", 2)},
		{Category: chartCell("A"), CategoryKey: "a", Series: chartCell("S2"), SeriesID: "s2", Value: chartValue("3", 3)},
		{Category: chartCell("B"), CategoryKey: "b", Series: chartCell("S1"), SeriesID: "s1", Value: chartValue("-4", -4)},
		{Category: chartCell("B"), CategoryKey: "b", Series: chartCell("S2"), SeriesID: "s2", Value: chartValue("1", 1)},
	}
	for _, tc := range []struct {
		kind   charts.Kind
		tokens []string
	}{{charts.GroupedBar, []string{`data-layout="grouped"`, `data-category="a" data-series="s1"`, `data-category="a" data-series="s2"`}}, {charts.StackedBar, []string{`data-layout="stacked"`, `data-category="a" data-start="0" data-end="2"`, `data-category="a" data-start="2" data-end="5"`, `data-category="b" data-start="0" data-end="-4"`}}} {
		out := &charts.Output{Version: charts.DisplayVersion, Kind: tc.kind, State: "ready", Points: points}
		var b strings.Builder
		if err := drawChartGeometry(&b, out, 800, 420, "#111", "#fff", "UTC"); err != nil {
			t.Fatal(err)
		}
		svg := b.String()
		for _, token := range tc.tokens {
			if !strings.Contains(svg, token) {
				t.Fatalf("%s missing %s: %s", tc.kind, token, svg)
			}
		}
	}
}

func TestHeatmapUsesTrueXYMatrix(t *testing.T) {
	points := []charts.Point{{X: chartValue("x1", 1), Y: chartValue("y1", 1), Value: chartValue("1", 1)}, {X: chartValue("x2", 2), Y: chartValue("y1", 1), Value: chartValue("2", 2)}, {X: chartValue("x1", 1), Y: chartValue("y2", 2), Value: chartValue("3", 3)}}
	out := &charts.Output{Version: charts.DisplayVersion, Kind: charts.Heatmap, State: "ready", Points: points}
	var b strings.Builder
	if err := drawChartGeometry(&b, out, 800, 420, "#111", "#fff", "UTC"); err != nil {
		t.Fatal(err)
	}
	svg := b.String()
	for _, token := range []string{`data-x="x1" data-y="y1" x="48.0" y="36.0"`, `data-x="x2" data-y="y1" x="416.0" y="36.0"`, `data-x="x1" data-y="y2" x="48.0" y="208.0"`} {
		if !strings.Contains(svg, token) {
			t.Fatalf("matrix missing %s: %s", token, svg)
		}
	}
}
