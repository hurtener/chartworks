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
