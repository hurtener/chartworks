package charts_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

func comparisonData(rows int) charts.Data {
	d, _, _ := chartfixtures.Fixture(charts.Bar, "binding")
	d.Columns = d.Columns[:2]
	d.Rows = nil
	for i := range rows {
		d.Rows = append(d.Rows, []charts.Cell{{Value: fmt.Sprintf("Category %03d", i)}, {Value: fmt.Sprintf("%d.000", i+1)}})
	}
	return d
}

func TestIntentAndCardinalityBeforeCandidateSealing(t *testing.T) {
	d := comparisonData(3)
	for _, tc := range []struct {
		intent     string
		kind       charts.Kind
		classified string
	}{
		{"Compare categories", charts.Bar, "comparison"}, {"Comparar categorías", charts.Bar, "comparison"},
		{"Composition share", charts.Pie, "composition"}, {"Participación", charts.Pie, "composition"},
		{"Table", charts.Table, "table"}, {"not a pie, compare categories", charts.Bar, "comparison"},
	} {
		t.Run(tc.intent, func(t *testing.T) {
			out, err := charts.SelectWithIntent(context.Background(), d, tc.intent, charts.Defaults())
			if err != nil || out.Selected.Mapping.Kind != tc.kind || out.Evidence == nil || out.Evidence.Intent != tc.classified || out.Evidence.Version != charts.RulesVersion || out.Evidence.Cardinality[0].Distinct != 3 || out.Evidence.TieBreak != "catalog_order" {
				t.Fatalf("pre-seal intent not applied: %+v %v", out, err)
			}
			if out.Fallback || len(out.Alternatives) > charts.Defaults().MaxAlternatives || len(out.Evidence.Evaluations) != 14 {
				t.Fatal("bounded candidate inventory missing")
			}
			for range 4 {
				again, err := charts.SelectWithIntent(context.Background(), d, tc.intent, charts.Defaults())
				if err != nil || !reflect.DeepEqual(again, out) {
					t.Fatal("selection is unstable", err)
				}
			}
		})
	}
	out, err := charts.SelectWithIntent(context.Background(), comparisonData(50), "composition", charts.Defaults())
	if err != nil || out.Selected.Mapping.Kind != charts.Bar {
		t.Fatal("high-cardinality composition chosen", err)
	}
	for _, e := range out.Evidence.Evaluations {
		if (e.Kind == charts.Pie || e.Kind == charts.Donut) && e.Outcome != "below_floor" {
			t.Fatal("cardinality was only applied after sealing", e)
		}
	}
	for _, c := range out.Alternatives {
		if c.Mapping.Kind == charts.Pie || c.Mapping.Kind == charts.Donut {
			t.Fatal("excluded composition reachable by ranker")
		}
	}
	// A safe composition that was formerly below the floor must become eligible
	// in rules, not be left for an optional model to recover.
	l := charts.Defaults()
	l.SelectionFloor = 99
	out, err = charts.SelectWithIntent(context.Background(), d, "composition", l)
	if err != nil || out.Fallback || out.Selected.Mapping.Kind != charts.Pie || out.Evidence.AboveFloor != 1 {
		t.Fatal("intent did not precede floor admission", err)
	}
	out, err = charts.Select(context.Background(), d, l)
	if err != nil || !out.Fallback {
		t.Fatal("unspecified intent invented a score boost", err)
	}
}

func TestSemanticSignalsAndRichCandidateBindings(t *testing.T) {
	d := comparisonData(3)
	d.Columns[1].Aggregation = "average"
	out, err := charts.SelectWithIntent(context.Background(), d, "composition", charts.Defaults())
	if err != nil || out.Selected.Mapping.Kind == charts.Pie || out.Selected.Mapping.Kind == charts.Donut {
		t.Fatal("nonadditive semantic signal ignored", err)
	}
	for _, evaluation := range out.Evidence.Evaluations {
		if (evaluation.Kind == charts.Pie || evaluation.Kind == charts.Donut) && evaluation.Outcome != "below_floor" {
			t.Fatal("nonadditive composition was sealed", evaluation)
		}
	}
	for _, tc := range []struct {
		name, intent string
		kind         charts.Kind
		variant      string
	}{
		{"line_two_units", "trend", charts.Line, "multi_measure"},
		{"line_measure_series", "trend", charts.Line, "multi_measure_series"},
		{"grouped_bar_multi_measure", "compare", charts.GroupedBar, "multi_measure"},
		{"bubble_series", "bubble relationship", charts.Scatter, "bubble"},
		{"hierarchy_three_levels", "hierarchy", charts.Treemap, "ordered_hierarchy"},
	} {
		f := richFixture(t, tc.name)
		selected, err := charts.SelectWithIntent(context.Background(), f.Data, tc.intent, charts.Defaults())
		if err != nil || selected.Selected.Mapping.Kind != tc.kind || selected.Selected.Variant != tc.variant {
			t.Fatalf("rich candidate missing before rank: %s %+v %v", tc.name, selected, err)
		}
		built, err := charts.Build(context.Background(), f.Data, selected.Selected.Mapping, charts.Defaults())
		if err != nil || built.Kind != tc.kind || built.Version != 2 {
			t.Fatal("selected shape cannot build", err)
		}
		if tc.name == "line_two_units" && len(selected.Selected.Mapping.Bindings.Values) != 2 {
			t.Fatal("selection discarded a measure")
		}
		if tc.name == "bubble_series" && selected.Selected.Mapping.Bindings.Size != "size" {
			t.Fatal("size inferred only after sealing")
		}
		if tc.name == "hierarchy_three_levels" && len(selected.Selected.Mapping.Bindings.Hierarchy) != 3 {
			t.Fatal("hierarchy was not declared")
		}
	}
	f := richFixture(t, "bubble_series")
	plain, err := charts.SelectWithIntent(context.Background(), f.Data, "correlation", charts.Defaults())
	if err != nil || plain.Selected.Mapping.Kind != charts.Scatter || plain.Selected.Mapping.Bindings.Size != "" {
		t.Fatal("bubble size invented without intent", err)
	}
	found := false
	for _, id := range plain.Selected.UnusedColumns {
		if id == "size" {
			found = true
		}
	}
	if !found {
		t.Fatal("unused measure silently dropped from selection provenance")
	}
}

func TestIntentBoundsStableTiesAndConflictingCues(t *testing.T) {
	d := comparisonData(3)
	for _, intent := range []string{strings.Repeat("x", 1025), "compare\x00secret", string([]byte{0xff})} {
		if _, err := charts.SelectWithIntent(context.Background(), d, intent, charts.Defaults()); !errors.Is(err, charts.ErrInvalid) {
			t.Fatal("unbounded intent accepted", err)
		}
	}
	defaultChoice, err := charts.Select(context.Background(), d, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	mixed, err := charts.SelectWithIntent(context.Background(), d, "trend and composition", charts.Defaults())
	if err != nil || mixed.Evidence.Intent != "mixed" || mixed.Selected.Mapping.Kind != defaultChoice.Selected.Mapping.Kind {
		t.Fatal("conflicting intent guessed a shape", err)
	}
	if len(defaultChoice.Alternatives) != 3 || defaultChoice.Alternatives[1].Mapping.Kind != charts.Donut || defaultChoice.Alternatives[2].Mapping.Kind != charts.Pie {
		t.Fatal("catalog tie order changed", defaultChoice.Alternatives)
	}
}

func TestUnaccentedSpanishIntentCompatibility(t *testing.T) {
	composition, err := charts.SelectWithIntent(context.Background(), comparisonData(3), "composicion", charts.Defaults()) //nolint:misspell // Intentional unaccented Spanish input.
	if err != nil || composition.Selected.Mapping.Kind != charts.Pie || composition.Evidence.Intent != "composition" {
		t.Fatal("unaccented composition cue lost", err)
	}
	f := richFixture(t, "bubble_series")
	relationship, err := charts.SelectWithIntent(context.Background(), f.Data, "correlacion", charts.Defaults()) //nolint:misspell // Intentional unaccented Spanish input.
	if err != nil || relationship.Selected.Mapping.Kind != charts.Scatter || relationship.Selected.Mapping.Bindings.Size != "" || relationship.Evidence.Intent != "relationship" {
		t.Fatal("unaccented relationship cue must not invent size", err)
	}
}
