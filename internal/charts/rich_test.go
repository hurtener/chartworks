package charts_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func richFixture(t *testing.T, name string) chartfixtures.RichFixture {
	t.Helper()
	for _, f := range chartfixtures.RichCases() {
		if f.Name == name {
			return f
		}
	}
	t.Fatal("missing rich fixture", name)
	return chartfixtures.RichFixture{}
}

func richBuild(t *testing.T, f chartfixtures.RichFixture) (charts.Mapping, charts.Output) {
	t.Helper()
	m, err := charts.Bind(context.Background(), f.Data, f.Kind, f.Bindings, f.Order, charts.DefaultOptions(), charts.Defaults())
	if err != nil {
		t.Fatal(f.Name, err)
	}
	out, err := charts.Build(context.Background(), f.Data, m, charts.Defaults())
	if err != nil {
		t.Fatal(f.Name, err)
	}
	return m, out
}

func TestRichSaveReadBuild(t *testing.T) {
	for _, f := range chartfixtures.RichCases() {
		t.Run(f.Name, func(t *testing.T) {
			original, _ := json.Marshal(f.Data)
			m, expected := richBuild(t, f)
			wire, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var saved charts.Mapping
			if err = json.Unmarshal(wire, &saved); err != nil {
				t.Fatal(err)
			}
			out, err := charts.Build(context.Background(), f.Data, saved, charts.Defaults())
			if err != nil || !reflect.DeepEqual(out, expected) {
				t.Fatal("saved binding changed behavior", err)
			}
			if saved.Version != 2 || out.Version != 2 || out.Transformation == nil || len(out.Rows) != len(f.Data.Rows) || len(out.RowIndices) != len(f.Data.Rows) {
				t.Fatal("rich retained rows/versions lost")
			}
			for i, index := range out.RowIndices {
				for j, column := range out.Columns {
					for k, input := range f.Data.Columns {
						if input.ID == column.ID && out.Rows[i][j] != f.Data.Rows[index][k] {
							t.Fatal("exact wide value changed")
						}
					}
				}
			}
			for _, total := range out.Totals {
				if f.Data.Completeness.Status == "truncated" && total.Scope != "returned_rows" {
					t.Fatal("total claims complete source")
				}
			}
			after, _ := json.Marshal(f.Data)
			if string(after) != string(original) {
				t.Fatal("input mutated")
			}
			if len(saved.Bindings.Values) > 0 {
				saved.Bindings.Values[0] = "mutated"
				if expected.Mapping.Bindings.Values[0] == "mutated" || f.Bindings.Values[0] == "mutated" {
					t.Fatal("aliased saved list")
				}
			}
		})
	}
}

func TestRichOrderedSeriesGapsAndExactTotals(t *testing.T) {
	_, out := richBuild(t, richFixture(t, "line_two_units"))
	if len(out.Series) != 2 || out.Series[0].Measure != "revenue" || out.Series[1].Measure != "quantity" || out.Series[0].Format.Currency != "USD" || out.Series[1].Format.Unit != "items" {
		t.Fatal("named ordered distinct units lost", out.Series)
	}
	if len(out.Points) != 6 || out.Points[0].Category.Value != "2026-01-01" || !out.Points[1].Value.Null || out.Points[1].Row != 2 || out.Points[0].Value.Exact != "9007199254740993.0100" || out.Points[3].Value.Exact != "9223372036854775807" {
		t.Fatal("time order, exact values or null gap lost", out.Points)
	}
	if out.Totals[0].Value.Value != "9007199254741023.0400" || out.Totals[1].Value.Value != "9223372036854775857" {
		t.Fatal("inexact additive totals", out.Totals)
	}
	_, out = richBuild(t, richFixture(t, "line_measure_series"))
	if len(out.Series) != 4 || len(out.Points) != 12 || out.Transformation.GapPoints != 2 || out.Transformation.MissingPoints != 2 {
		t.Fatal("series cross-product did not preserve missing observations", out.Transformation)
	}
	gaps := 0
	for _, p := range out.Points {
		if p.Row == -1 {
			gaps++
			if !p.Value.Null || p.Value.Coordinate != nil || p.Category.Value != "2026-01-02" || p.Series.Value != "South" {
				t.Fatal("invented gap values")
			}
		}
	}
	if gaps != 2 {
		t.Fatal("untruthful derived-row provenance")
	}
	for _, name := range []string{"bar_multi_measure", "column_multi_measure", "grouped_bar_multi_measure"} {
		m, b := richBuild(t, richFixture(t, name))
		if m.Bindings.Series != "" || len(b.Series) != 2 || b.Series[0].Measure != "returned" || b.Series[1].Measure != "ordered" || len(b.Points) != 6 || b.Transformation.MissingPoints != 1 {
			t.Fatal("artificial dimension or first-measure loss", name)
		}
	}
	_, out = richBuild(t, richFixture(t, "bar_category_series"))
	if len(out.Series) != 2 || len(out.Points) != 4 || out.Points[0].Category.Value != out.Points[1].Category.Value || out.Points[0].SeriesID == out.Points[1].SeriesID {
		t.Fatal("categorical split collapsed")
	}
}

func TestRichBubbleAndHierarchyMeaning(t *testing.T) {
	_, b := richBuild(t, richFixture(t, "bubble_series"))
	if len(b.Points) != 2 || b.OmittedRows != 2 || b.Transformation.ZeroSizePoints != 1 || b.Transformation.SizeEncoding != "area" || b.Points[0].Size.Exact != "4" || b.Points[1].Size.Exact != "16" || b.Points[0].X.Exact != "1.0000000000000001" || !b.Points[0].X.Approximate || b.Points[0].SeriesID == b.Points[1].SeriesID {
		t.Fatal("bubble semantics/exact values lost", b)
	}
	_, h := richBuild(t, richFixture(t, "hierarchy_three_levels"))
	if h.OmittedRows != 1 || len(h.Points) != 3 || len(h.Hierarchy) != 7 || h.Transformation.Aggregation != "sum" || h.Transformation.DuplicatePolicy != "reject" {
		t.Fatal("deep hierarchy shape lost", h)
	}
	leaves := map[string]bool{}
	for _, node := range h.Hierarchy {
		if node.Depth != len(node.Path)-1 || node.Scope != "returned_complete_paths" || len(node.Rows) == 0 {
			t.Fatal("unattributed hierarchy aggregate")
		}
		if node.Depth == 0 && node.Path[0].Value == "North" && (node.Value.Exact != "9007199254740993.0300" || len(node.Rows) != 2) {
			t.Fatal("inexact hierarchy aggregation", node)
		}
		if node.Depth == 2 {
			if leaves[node.ID] {
				t.Fatal("equal labels merged across branches")
			}
			leaves[node.ID] = true
		}
	}
	if len(leaves) != 3 || h.Totals[0].Value.Value != "9007199254741003.0300" || h.Totals[0].Scope != "returned_rows" {
		t.Fatal("omitted-path total conflated with drawn tree", h.Totals)
	}
}

func TestRichInvalidAndAmbiguousShapes(t *testing.T) {
	tests := []struct {
		name, fixture string
		edit          func(*chartfixtures.RichFixture)
		target        error
	}{
		{"scalar_and_list", "line_two_units", func(f *chartfixtures.RichFixture) { f.Bindings.Value = "revenue" }, charts.ErrInvalid},
		{"duplicate_measure", "line_two_units", func(f *chartfixtures.RichFixture) { f.Bindings.Values = []string{"revenue", "revenue"} }, charts.ErrInvalid},
		{"text_measure", "bar_multi_measure", func(f *chartfixtures.RichFixture) {
			f.Bindings.Values = []string{"category", "returned"}
			f.Bindings.Category = "ordered"
		}, charts.ErrUnsuitable},
		{"negative_size", "bubble_series", func(f *chartfixtures.RichFixture) { f.Data.Rows[2][2].Value = "-1" }, charts.ErrUnsuitable},
		{"negative_size_null_x", "bubble_series", func(f *chartfixtures.RichFixture) {
			f.Data.Rows[2][0] = charts.Cell{Null: true}
			f.Data.Rows[2][2].Value = "-1"
		}, charts.ErrUnsuitable},
		{"identifier_size", "bubble_series", func(f *chartfixtures.RichFixture) { f.Data.Columns[2].Role = "identifier" }, charts.ErrUnsuitable},
		{"percent_size", "bubble_series", func(f *chartfixtures.RichFixture) { f.Data.Columns[2].Format.Percent = "whole" }, charts.ErrUnsuitable},
		{"size_overflow", "bubble_series", func(f *chartfixtures.RichFixture) { f.Data.Rows[0][2].Value = strings.Repeat("9", 400) }, charts.ErrUnsuitable},
		{"duplicate_hierarchy_path", "hierarchy_three_levels", func(f *chartfixtures.RichFixture) {
			f.Data.Rows = append(f.Data.Rows, append([]charts.Cell{}, f.Data.Rows[0]...))
		}, charts.ErrUnsuitable},
		{"duplicate_null_leaf", "hierarchy_three_levels", func(f *chartfixtures.RichFixture) {
			r := append([]charts.Cell{}, f.Data.Rows[0]...)
			r[3] = charts.Cell{Null: true}
			f.Data.Rows = append(f.Data.Rows, r)
		}, charts.ErrUnsuitable},
		{"negative_null_path", "hierarchy_three_levels", func(f *chartfixtures.RichFixture) { f.Data.Rows[3][3].Value = "-1" }, charts.ErrUnsuitable},
		{"nonadditive_hierarchy", "hierarchy_three_levels", func(f *chartfixtures.RichFixture) { f.Data.Columns[3].Aggregation = "average" }, charts.ErrUnsuitable},
		{"percent_hierarchy", "hierarchy_three_levels", func(f *chartfixtures.RichFixture) {
			f.Data.Columns[3].Format.Currency = ""
			f.Data.Columns[3].Format.Percent = "fraction"
		}, charts.ErrUnsuitable},
		{"old_parent_and_hierarchy", "hierarchy_three_levels", func(f *chartfixtures.RichFixture) { f.Bindings.Parent = "country" }, charts.ErrInvalid},
		{"duplicate_time_series", "line_measure_series", func(f *chartfixtures.RichFixture) {
			r := append([]charts.Cell{}, f.Data.Rows[1]...)
			r[0].Value = "2026-01-01T01:00:00+01:00"
			f.Data.Rows = append(f.Data.Rows, r)
		}, charts.ErrUnsuitable},
		{"bad_time", "line_two_units", func(f *chartfixtures.RichFixture) { f.Data.Rows[0][0].Value = "not-time" }, charts.ErrUnsuitable},
		{"wrong_time_order", "line_two_units", func(f *chartfixtures.RichFixture) { f.Order = []charts.Order{{Column: "revenue", Direction: "asc"}} }, charts.ErrUnsuitable},
		{"unknown_repeat_kind", "line_two_units", func(f *chartfixtures.RichFixture) { f.Kind = charts.Pie }, charts.ErrInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := richFixture(t, tc.fixture)
			tc.edit(&f)
			_, err := charts.Bind(context.Background(), f.Data, f.Kind, f.Bindings, f.Order, charts.DefaultOptions(), charts.Defaults())
			if !errors.Is(err, tc.target) {
				t.Fatalf("wanted %v, got %v", tc.target, err)
			}
		})
	}
	// All tuple validators must reject a duplicate even when one value is null.
	for _, kind := range []charts.Kind{charts.Bar, charts.ColumnChart, charts.GroupedBar, charts.StackedBar, charts.StackedColumn, charts.Heatmap, charts.Pie, charts.Donut, charts.Treemap} {
		d, b, o := chartfixtures.Fixture(kind, "binding")
		row := append([]charts.Cell{}, d.Rows[0]...)
		row[1] = charts.Cell{Null: true}
		d.Rows = append(d.Rows, row)
		if _, err := charts.Bind(context.Background(), d, kind, b, o, charts.DefaultOptions(), charts.Defaults()); !errors.Is(err, charts.ErrUnsuitable) {
			t.Fatal("null tuple evaded uniqueness", kind, err)
		}
	}
}

func TestRichPinsRebindingAndConcurrentReuse(t *testing.T) {
	for _, name := range []string{"line_two_units", "bubble_series", "hierarchy_three_levels"} {
		f := richFixture(t, name)
		m, want := richBuild(t, f)
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := charts.Build(context.Background(), f.Data, m, charts.Defaults())
				if err != nil || !reflect.DeepEqual(out, want) {
					t.Error("concurrent build changed", err)
				}
			}()
		}
		wg.Wait()
		for _, edit := range []func(*charts.Column){func(c *charts.Column) { c.Format.Unit = "changed" }, func(c *charts.Column) { c.Provenance.SourceRevision++ }, func(c *charts.Column) { c.Provenance.Source = "other" }, func(c *charts.Column) { c.Provenance.TopicVersion = "v4" }, func(c *charts.Column) { c.Provenance.SemanticID = "other" }, func(c *charts.Column) { c.Name = "Other label" }} {
			d := f.Data
			d.Columns = append([]charts.Column{}, f.Data.Columns...)
			edit(&d.Columns[0])
			if _, err := charts.Build(context.Background(), d, m, charts.Defaults()); !errors.Is(err, charts.ErrMappingChanged) {
				t.Fatal("saved pin drift accepted", name, err)
			}
		}
		// Change every ID and revision without changing approved semantic meaning.
		d := f.Data
		d.Columns = append([]charts.Column{}, d.Columns...)
		for i := range d.Columns {
			d.Columns[i].ID += "_next"
			d.Columns[i].Provenance.SourceRevision++
		}
		proposal, err := charts.Rebind(context.Background(), d, m, charts.Defaults())
		if err != nil || proposal.Status != "review_required" {
			t.Fatal("rich rebind proposal failed", err)
		}
		out, err := charts.Build(context.Background(), d, proposal.Mapping, charts.Defaults())
		if err != nil || len(out.Rows) != len(want.Rows) || !reflect.DeepEqual(out.Rows, want.Rows) {
			t.Fatal("rebind lost repeated slots or values", err)
		}
		if len(proposal.Mapping.Bindings.Values) > 0 && proposal.Mapping.Bindings.Values[0] != m.Bindings.Values[0]+"_next" {
			t.Fatal("measure list not rebound")
		}
		if len(proposal.Mapping.Bindings.Hierarchy) > 0 && proposal.Mapping.Bindings.Hierarchy[2] != m.Bindings.Hierarchy[2]+"_next" {
			t.Fatal("hierarchy order not rebound")
		}
		if m.Bindings.Size != "" && proposal.Mapping.Bindings.Size != m.Bindings.Size+"_next" {
			t.Fatal("size not rebound")
		}
		m.Version = 1
		if _, err = charts.Build(context.Background(), f.Data, m, charts.Defaults()); !errors.Is(err, charts.ErrInvalid) {
			t.Fatal("rich fields accepted by old mapping version", err)
		}
	}
}

func TestRichExpansionAndHierarchyBounds(t *testing.T) {
	f := richFixture(t, "line_measure_series")
	l := charts.Defaults()
	l.MaxCategories = 1000
	l.MaxSeries = 128
	f.Data.Rows = nil
	for i := range 501 {
		f.Data.Rows = append(f.Data.Rows, []charts.Cell{{Value: fmt.Sprintf("2026-01-01T%02d:%02d:00Z", i/60, i%60)}, {Value: "1"}, {Value: "2"}, {Value: fmt.Sprintf("s%d", i%20)}})
	}
	if _, err := charts.Bind(context.Background(), f.Data, f.Kind, f.Bindings, nil, charts.DefaultOptions(), l); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("unbounded gap expansion", err)
	}
	f = richFixture(t, "hierarchy_three_levels")
	l = charts.Defaults()
	l.MaxCategories = 2
	if _, err := charts.Bind(context.Background(), f.Data, f.Kind, f.Bindings, nil, charts.DefaultOptions(), l); !errors.Is(err, charts.ErrUnsuitable) {
		t.Fatal("node bound ignored", err)
	}
	f = richFixture(t, "line_two_units")
	l = charts.Defaults()
	l.MaxSeries = 1
	if _, err := charts.Bind(context.Background(), f.Data, f.Kind, f.Bindings, nil, charts.DefaultOptions(), l); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("repeated-measure bound ignored", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := charts.SelectWithIntent(ctx, f.Data, "trend", charts.Defaults()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
}
