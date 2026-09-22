package charts_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
)

func TestCW02HierarchyEquivalentParentLabels(t *testing.T) {
	f := richFixture(t, "hierarchy_equivalent_parents")
	_, out := richBuild(t, f)
	nodes := make(map[string]charts.HierarchyNode)
	for _, node := range out.Hierarchy {
		if node.Depth > 0 {
			parent, ok := nodes[node.Parent]
			if !ok || !reflect.DeepEqual(node.Path[:node.Depth], parent.Path) {
				t.Fatalf("equivalent parent spellings produced an unreadable tree: child=%+v parent=%+v", node, parent)
			}
		}
		nodes[node.ID] = node
	}
	if len(nodes) != 4 || out.Hierarchy[0].Value.Exact != "30.00" {
		t.Fatalf("typed-equal parents did not share exact aggregation: %+v", out.Hierarchy)
	}
	for _, p := range out.Points {
		if !reflect.DeepEqual(p.Path, nodes[p.CategoryKey].Path) {
			t.Fatal("point path disagrees with its retained node")
		}
	}
	for i, sourceRow := range out.RowIndices {
		for j, column := range out.Columns {
			for k, source := range f.Data.Columns {
				if source.ID == column.ID && out.Rows[i][j] != f.Data.Rows[sourceRow][k] {
					t.Fatal("representative labels rewrote exact retained source values")
				}
			}
		}
	}
	// Equivalent spellings may merge parents, never distinct observations at a leaf.
	f.Data.Rows[1][2] = f.Data.Rows[0][2]
	if _, err := charts.Bind(context.Background(), f.Data, f.Kind, f.Bindings, f.Order, charts.DefaultOptions(), charts.Defaults()); !errors.Is(err, charts.ErrUnsuitable) {
		t.Fatalf("typed-equal duplicate leaf was accepted: %v", err)
	}
}

func TestCW02HierarchyOrderPrefix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order []charts.Order
		valid bool
	}{
		{"default", nil, true},
		{"root_prefix", []charts.Order{{Column: "region", Direction: "desc"}}, true},
		{"full_prefix", []charts.Order{{Column: "region", Direction: "desc"}, {Column: "country", Direction: "asc"}, {Column: "product", Direction: "desc"}}, true},
		{"measure_after_path", []charts.Order{{Column: "region", Direction: "asc"}, {Column: "country", Direction: "asc"}, {Column: "product", Direction: "asc"}, {Column: "value", Direction: "desc"}}, true},
		{"measure_first", []charts.Order{{Column: "value", Direction: "desc"}}, false},
		{"child_first", []charts.Order{{Column: "country", Direction: "asc"}, {Column: "region", Direction: "asc"}}, false},
		{"measure_interrupts_path", []charts.Order{{Column: "region", Direction: "asc"}, {Column: "value", Direction: "asc"}}, false},
		{"skipped_level", []charts.Order{{Column: "region", Direction: "asc"}, {Column: "product", Direction: "asc"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := richFixture(t, "hierarchy_three_levels")
			m, err := charts.Bind(context.Background(), f.Data, f.Kind, f.Bindings, tc.order, charts.DefaultOptions(), charts.Defaults())
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = charts.Build(context.Background(), f.Data, m, charts.Defaults()); err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, charts.ErrUnsuitable) {
				t.Fatalf("non-prefix hierarchy ordering was accepted: %v", err)
			}
			m, _ = charts.Bind(context.Background(), f.Data, f.Kind, f.Bindings, nil, charts.DefaultOptions(), charts.Defaults())
			m.Order = tc.order
			if _, err = charts.Build(context.Background(), f.Data, m, charts.Defaults()); !errors.Is(err, charts.ErrUnsuitable) {
				t.Fatalf("saved build bypassed hierarchy order validation: %v", err)
			}
		})
	}
}
