package chartfixtures

import (
	"fmt"

	"github.com/hurtener/chartworks/internal/charts"
)

// RichFixture is a synthetic declared binding, not a claim about model quality.
type RichFixture struct {
	Name     string
	Kind     charts.Kind
	Data     charts.Data
	Bindings charts.Bindings
	Order    []charts.Order
}

func richColumn(id, name, typ, role string) charts.Column {
	c := charts.Column{ID: id, Name: name, Type: typ, Role: role, Provenance: charts.Provenance{Version: 1, Source: "warehouse", SourceRevision: 7, Topic: "metrics", TopicVersion: "v3", SemanticID: id}}
	if typ == "integer" || typ == "decimal" {
		c.Aggregation = "sum"
	}
	if typ == "temporal" {
		c.Grain = "day"
	}
	return c
}

func richRow(values ...string) []charts.Cell {
	row := make([]charts.Cell, len(values))
	for i, value := range values {
		if value == "<null>" {
			row[i].Null = true
		} else {
			row[i].Value = value
		}
	}
	return row
}

func richTime(series bool) charts.Data {
	d := charts.Data{Version: 1, Completeness: charts.Completeness{Status: "truncated", Reason: "rows"}, Columns: []charts.Column{
		richColumn("day", "Observed day", "temporal", "time"), richColumn("revenue", "Revenue", "decimal", "measure"), richColumn("quantity", "Quantity", "integer", "measure")},
		Rows: [][]charts.Cell{richRow("2026-01-03", "30.0300", "30"), richRow("2026-01-01", "9007199254740993.0100", "9223372036854775807"), richRow("2026-01-02", "<null>", "20")}}
	d.Columns[1].Format.Currency = "USD"
	d.Columns[2].Format.Unit = "items"
	if series {
		d.Columns = append(d.Columns, richColumn("region", "Region", "text", "dimension"))
		d.Rows = [][]charts.Cell{richRow("2026-01-03", "30.0300", "300", "South"), richRow("2026-01-01", "10.0100", "100", "North"), richRow("2026-01-01", "20.0200", "200", "South"), richRow("2026-01-03", "40.0400", "<null>", "North"), richRow("2026-01-02", "<null>", "150", "North")}
	}
	return d
}

func richCategories(series bool) charts.Data {
	d := charts.Data{Version: 1, Completeness: charts.Completeness{Status: "complete_result"}, Columns: []charts.Column{
		richColumn("category", "Full category label", "text", "dimension"), richColumn("ordered", "Ordered items", "decimal", "measure"), richColumn("returned", "Returned items", "integer", "measure")},
		Rows: [][]charts.Cell{richRow("Beta", "10.100", "2"), richRow("Alpha", "20.200", "4"), richRow("Gamma", "<null>", "3")}}
	d.Columns[1].Format.Unit, d.Columns[2].Format.Unit = "items", "items"
	if series {
		d.Columns = append(d.Columns, richColumn("region", "Region", "text", "dimension"))
		d.Rows = [][]charts.Cell{richRow("Alpha", "10.100", "1", "North"), richRow("Alpha", "20.200", "2", "South"), richRow("Beta", "30.300", "3", "North"), richRow("Beta", "<null>", "4", "South")}
	}
	return d
}

func richBubble() charts.Data {
	d := charts.Data{Version: 1, Completeness: charts.Completeness{Status: "complete_result"}, Columns: []charts.Column{
		richColumn("x", "Exposure", "decimal", "measure"), richColumn("y", "Response", "decimal", "measure"), richColumn("size", "Population", "integer", "measure"), richColumn("segment", "Full segment label", "text", "dimension")},
		Rows: [][]charts.Cell{richRow("1.0000000000000001", "2.00", "4", "North <literal>"), richRow("2", "4.00", "16", "South"), richRow("3", "8.00", "0", "North <literal>"), richRow("4", "16.00", "<null>", "South")}}
	d.Columns[2].Format.Unit = "people"
	return d
}

func richHierarchy() charts.Data {
	d := charts.Data{Version: 1, Completeness: charts.Completeness{Status: "truncated", Reason: "source_limit"}, Columns: []charts.Column{
		richColumn("region", "Region", "text", "dimension"), richColumn("country", "Country", "text", "dimension"), richColumn("product", "Product", "text", "dimension"), richColumn("value", "Amount", "decimal", "measure")},
		Rows: [][]charts.Cell{richRow("North", "A", "Device", "9007199254740993.0100"), richRow("North", "A", "Sensor", "0.0200"), richRow("South", "B", "Device", "3.0000"), richRow("<null>", "B", "Orphan", "7.0000")}}
	d.Columns[3].Format.Currency = "USD"
	return d
}

// RichCases exercises variants separately from the fourteen-kind catalog. Every
// call creates detached rows, ordered slots and reviewed metadata for tests.
func RichCases() []RichFixture {
	out := []RichFixture{}
	for _, kind := range []charts.Kind{charts.Line, charts.Area} {
		out = append(out, RichFixture{Name: string(kind) + "_two_units", Kind: kind, Data: richTime(false), Bindings: charts.Bindings{Category: "day", Values: []string{"revenue", "quantity"}}})
		out = append(out, RichFixture{Name: string(kind) + "_measure_series", Kind: kind, Data: richTime(true), Bindings: charts.Bindings{Category: "day", Values: []string{"revenue", "quantity"}, Series: "region"}})
		out = append(out, RichFixture{Name: string(kind) + "_category_series", Kind: kind, Data: richTime(true), Bindings: charts.Bindings{Category: "day", Value: "revenue", Series: "region"}})
	}
	for _, kind := range []charts.Kind{charts.Bar, charts.ColumnChart, charts.GroupedBar} {
		out = append(out, RichFixture{Name: string(kind) + "_multi_measure", Kind: kind, Data: richCategories(false), Bindings: charts.Bindings{Category: "category", Values: []string{"returned", "ordered"}}})
		out = append(out, RichFixture{Name: string(kind) + "_measure_series", Kind: kind, Data: richCategories(true), Bindings: charts.Bindings{Category: "category", Values: []string{"ordered", "returned"}, Series: "region"}})
		if kind != charts.GroupedBar {
			out = append(out, RichFixture{Name: string(kind) + "_category_series", Kind: kind, Data: richCategories(true), Bindings: charts.Bindings{Category: "category", Value: "ordered", Series: "region"}})
		}
	}
	out = append(out, RichFixture{Name: "bubble_series", Kind: charts.Scatter, Data: richBubble(), Bindings: charts.Bindings{X: "x", Y: "y", Size: "size", Series: "segment"}})
	out = append(out, RichFixture{Name: "hierarchy_three_levels", Kind: charts.Treemap, Data: richHierarchy(), Bindings: charts.Bindings{Hierarchy: []string{"region", "country", "product"}, Value: "value"}})
	balanced := richHierarchy()
	for i, value := range []string{"100.00", "50.00", "80.00", "7.00"} {
		balanced.Rows[i][3].Value = value
	}
	out = append(out, RichFixture{Name: "hierarchy_balanced", Kind: charts.Treemap, Data: balanced, Bindings: charts.Bindings{Hierarchy: []string{"region", "country", "product"}, Value: "value"}})
	// Equal typed coordinates can arrive with different exact source spellings.
	// Shared nodes must use consistent representative labels without rewriting rows.
	aliases := richHierarchy()
	aliases.Columns[0].Type = "decimal"
	aliases.Columns[1].Type = "temporal"
	aliases.Rows = [][]charts.Cell{
		richRow("1.0", "2026-01-01T00:00:00Z", "Device", "10.00"),
		richRow("1e0", "2026-01-01T01:00:00+01:00", "Sensor", "20.00"),
	}
	out = append(out, RichFixture{Name: "hierarchy_equivalent_parents", Kind: charts.Treemap, Data: aliases, Bindings: charts.Bindings{Hierarchy: []string{"region", "country", "product"}, Value: "value"}})
	missing := richTime(false)
	for i := range missing.Rows {
		missing.Rows[i][1], missing.Rows[i][2] = charts.Cell{Null: true}, charts.Cell{Null: true}
	}
	out = append(out, RichFixture{Name: "line_all_missing", Kind: charts.Line, Data: missing, Bindings: charts.Bindings{Category: "day", Values: []string{"revenue", "quantity"}}})
	zero := richBubble()
	for i := range zero.Rows {
		zero.Rows[i][2] = charts.Cell{Value: "0"}
	}
	out = append(out, RichFixture{Name: "bubble_all_zero", Kind: charts.Scatter, Data: zero, Bindings: charts.Bindings{X: "x", Y: "y", Size: "size", Series: "segment"}})
	nullPath := richHierarchy()
	for i := range nullPath.Rows {
		nullPath.Rows[i][0] = charts.Cell{Null: true}
	}
	out = append(out, RichFixture{Name: "hierarchy_all_null", Kind: charts.Treemap, Data: nullPath, Bindings: charts.Bindings{Hierarchy: []string{"region", "country", "product"}, Value: "value"}})
	paged := richCategories(true)
	paged.Rows = [][]charts.Cell{}
	for category := 0; category < 25; category++ {
		for series := 0; series < 5; series++ {
			paged.Rows = append(paged.Rows, richRow(fmt.Sprintf("Category %02d", category), fmt.Sprint(category+1), fmt.Sprint(series+1), fmt.Sprintf("Series %d", series)))
		}
	}
	out = append(out, RichFixture{Name: "bar_retained_paging", Kind: charts.Bar, Data: paged, Bindings: charts.Bindings{Category: "category", Values: []string{"ordered", "returned"}, Series: "region"}})
	dense := richCategories(true)
	dense.Rows = [][]charts.Cell{}
	for category := 0; category < 100; category++ {
		for series := 0; series < 5; series++ {
			dense.Rows = append(dense.Rows, richRow(fmt.Sprintf("Category %02d", category), fmt.Sprint(category+1), fmt.Sprint(series+1), fmt.Sprintf("Series %d", series)))
		}
	}
	out = append(out, RichFixture{Name: "bar_dense_series", Kind: charts.Bar, Data: dense, Bindings: charts.Bindings{Category: "category", Values: []string{"ordered", "returned"}, Series: "region"}})
	return out
}
