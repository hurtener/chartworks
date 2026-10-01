package charts_test

import (
	"context"
	"encoding/json"
	"github.com/hurtener/chartworks/internal/charts"
	"reflect"
	"testing"
)

func TestTableSourceRowsPreserveSortedTiesAndNulls(t *testing.T) {
	ctx := context.Background()
	limits := charts.Defaults()
	c := charts.Column{ID: "value", Name: "value", Type: "integer", Role: "measure", Provenance: charts.Provenance{Version: 1}}
	d := charts.Data{Version: charts.Version, Columns: []charts.Column{c}, Rows: [][]charts.Cell{{{Value: "2"}}, {{Value: "1"}}, {{Value: "2"}}, {{Null: true}}}, Completeness: charts.Completeness{Status: "complete_result"}}
	order := []charts.Order{{Column: "value", Direction: "desc"}}
	for _, display := range []bool{false, true} {
		var m charts.Mapping
		var err error
		if display {
			m, err = charts.BindDisplay(ctx, d, charts.Table, charts.Bindings{Columns: []string{"value"}}, order, charts.DefaultOptions(), nil, &charts.TableOptions{Columns: []charts.TableColumnIntent{{Column: "value", Visible: true}}, PageSize: 2}, limits)
		} else {
			m, err = charts.Bind(ctx, d, charts.Table, charts.Bindings{Columns: []string{"value"}}, order, charts.DefaultOptions(), limits)
		}
		if err != nil {
			t.Fatal(err)
		}
		legacy, err := charts.Build(ctx, d, m, limits)
		if err != nil {
			t.Fatal(err)
		}
		old, _ := json.Marshal(legacy)
		out, err := charts.BuildWithSourceRows(ctx, d, m, limits)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(out.RowIndices, []int{0, 2, 1, 3}) || len(out.Rows) != 4 || !out.Rows[3][0].Null {
			t.Fatal("wrong source identities", out.RowIndices)
		}
		for i, source := range out.RowIndices {
			if !reflect.DeepEqual(out.Rows[i], d.Rows[source]) {
				t.Fatal("row borrowed evidence", i, source)
			}
		}
		out.RowIndices = nil
		now, _ := json.Marshal(out)
		if string(now) != string(old) {
			t.Fatal("ordinary chart bytes changed")
		}
	}
}

func TestSourceRowsKeepOmittedChartPointIdentity(t *testing.T) {
	ctx := context.Background()
	d, m := bind(t, charts.Bar)
	// A category-null row is omitted by the ordinary bar builder. Source identity
	// is the surviving Point.Row, never the visible point's array offset.
	category := -1
	for i, c := range d.Columns {
		if c.ID == m.Bindings.Category {
			category = i
		}
	}
	if category < 0 || len(d.Rows) < 2 {
		t.Fatal("fixture missing category")
	}
	d.Rows[0][category] = charts.Cell{Null: true}
	ordinary, err := charts.Build(ctx, d, m, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	out, err := charts.BuildWithSourceRows(ctx, d, m, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, ordinary) || out.OmittedRows < 1 || len(out.Points) == 0 || out.Points[0].Row == 0 {
		t.Fatal("omitted source row was reassigned")
	}
}
