// Package chartfixtures contains synthetic shared specification fixtures. It is
// also the sealed input catalog for later renderer tests; no pixels are claimed.
package chartfixtures

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
)

//go:embed goldens/*.json
var goldens embed.FS

// Cases is the mandatory behavior matrix for each of the fourteen kinds.
func Cases() []string { return []string{"binding", "order", "empty", "negative", "null", "long_label"} }

// Fixture returns independent input data and an explicit author-chosen mapping.
func Fixture(kind charts.Kind, scenario string) (charts.Data, charts.Bindings, []charts.Order) {
	column := func(id, typ, role string) charts.Column {
		c := charts.Column{ID: id, Name: id, Type: typ, Role: role, Provenance: charts.Provenance{Version: 1, Source: "fixture", SourceRevision: 1, Topic: "sales", TopicVersion: "v1", SemanticID: id}}
		if typ == "decimal" {
			c.Aggregation = "sum"
			c.Format = charts.Format{Unit: "items", FractionDigits: 3}
		}
		if typ == "temporal" {
			c.Grain = "day"
		}
		return c
	}
	c := func(v string) charts.Cell { return charts.Cell{Value: v} }
	d := charts.Data{Version: 1, Columns: []charts.Column{column("category", "text", "dimension"), column("value", "decimal", "measure"), column("series", "text", "dimension"), column("x", "decimal", "measure")},
		Rows: [][]charts.Cell{{c("Beta"), c("3.250"), c("West"), c("3")}, {c("Alpha"), c("1.125"), c("East"), c("1")}, {c("Gamma"), c("2.500"), c("West"), c("2")}}, Completeness: charts.Completeness{Status: "complete_result"}}
	b := charts.Bindings{Category: "category", Value: "value"}
	order := []charts.Order{{Column: "category", Direction: "asc"}}
	switch kind {
	case charts.Area, charts.Line:
		d.Columns[0] = column("category", "temporal", "time")
		for i, v := range []string{"2026-09-03", "2026-09-01", "2026-09-02"} {
			d.Rows[i][0] = c(v)
		}
	case charts.GroupedBar, charts.StackedBar, charts.StackedColumn:
		b.Series = "series"
	case charts.Heatmap:
		b = charts.Bindings{X: "category", Y: "series", Value: "value"}
	case charts.KPI:
		b = charts.Bindings{Value: "value"}
		order = nil
		d.Rows = d.Rows[:1]
	case charts.Scatter:
		b = charts.Bindings{X: "x", Y: "value", Series: "series"}
		order = []charts.Order{{Column: "x", Direction: "asc"}}
	case charts.Table:
		b = charts.Bindings{Columns: []string{"category", "value", "series", "x"}}
	case charts.Treemap:
		b.Parent = "series"
	}
	if scenario != "order" {
		order = nil
	}
	switch scenario {
	case "empty":
		d.Rows = [][]charts.Cell{}
	case "negative":
		d.Rows[0][1] = c("-3.250")
	case "null":
		d.Rows[0][1] = charts.Cell{Null: true}
	case "long_label":
		d.Columns[0].Name = strings.Repeat("Descripción ", 50)
		// Temporal values stay timestamps; a long display label is metadata there.
		if d.Columns[0].Type == "text" {
			d.Rows[0][0] = c(strings.Repeat("Categoría extendida <literal> ", 30))
		}
		if kind == charts.KPI {
			d.Columns[1].Name = strings.Repeat("Importe ", 60)
		}
	}
	return d, b, order
}

// Golden is deliberately an output-or-typed-error union, never a table substitute.
type Golden struct {
	Error  string         `json:"error,omitempty"`
	Output *charts.Output `json:"output,omitempty"`
}

// Produce executes the real core and preserves explicit unsuitable outcomes.
func Produce(kind charts.Kind, scenario string) Golden {
	d, b, o := Fixture(kind, scenario)
	m, err := charts.Bind(context.Background(), d, kind, b, o, charts.DefaultOptions(), charts.Defaults())
	if errors.Is(err, charts.ErrUnsuitable) {
		return Golden{Error: "unsuitable_binding"}
	}
	if err != nil {
		panic(err)
	}
	out, err := charts.Build(context.Background(), d, m, charts.Defaults())
	if err != nil {
		panic(err)
	}
	return Golden{Output: &out}
}

// Verify checks static goldens and independently asserts the essential behavior.
// This function does not update goldens, including when CI runs as an author.
func Verify(t *testing.T) {
	t.Helper()
	if len(charts.Catalog()) != 14 {
		t.Fatal("catalog is not fourteen real kinds")
	}
	for _, entry := range charts.Catalog() {
		t.Run(string(entry.Kind), func(t *testing.T) {
			for _, scenario := range Cases() {
				t.Run(scenario, func(t *testing.T) {
					b, err := goldens.ReadFile("goldens/" + string(entry.Kind) + "-" + scenario + ".json")
					if err != nil {
						t.Fatal(err)
					}
					var want Golden
					if err = json.Unmarshal(b, &want); err != nil {
						t.Fatal(err)
					}
					got := Produce(entry.Kind, scenario)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("golden mismatch: %s", b)
					}
					rejected := scenario == "negative" && (entry.Kind == charts.Pie || entry.Kind == charts.Donut || entry.Kind == charts.Treemap)
					if rejected {
						if got.Error != "unsuitable_binding" {
							t.Fatal("negative part accepted")
						}
						return
					}
					if got.Output == nil || got.Output.Kind != entry.Kind || got.Output.Mapping.Kind != entry.Kind {
						t.Fatal("kind substituted")
					}
					out := got.Output
					if scenario == "empty" && (out.State != "empty" || len(out.Rows) != 0 || len(out.Points) != 0) {
						t.Fatal("invented values for empty input")
					}
					if scenario == "order" && entry.Kind != charts.KPI {
						if entry.Kind == charts.Table {
							if out.Rows[0][0].Value != "Alpha" {
								t.Fatal("table order")
							}
						} else if out.Points[0].Row != 1 {
							t.Fatal("point order")
						}
					}
					if scenario == "null" {
						switch entry.Kind {
						case charts.Table:
							if !out.Rows[0][1].Null {
								t.Fatal("null table cell replaced")
							}
						case charts.Line, charts.Area:
							if !out.Points[0].Value.Null || out.OmittedRows != 0 {
								t.Fatal("line gap replaced")
							}
						case charts.KPI:
							if out.State != "no_values" || !out.Points[0].Value.Null {
								t.Fatal("null KPI replaced")
							}
						default:
							if out.OmittedRows != 1 || len(out.Points) != 2 {
								t.Fatal("null point not omitted")
							}
						}
					}
					if scenario == "long_label" {
						found := false
						for _, c := range out.Columns {
							if len(c.Name) > 300 {
								found = true
							}
						}
						if entry.Kind == charts.Scatter {
							found = true
						} // long unbound metadata is intentionally not projected.
						if !found {
							t.Fatal("full label lost")
						}
					}
				})
			}
		})
	}
}
