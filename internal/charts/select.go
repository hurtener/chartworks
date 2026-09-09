package charts

import (
	"context"
	"errors"
	"sort"
)

// Select uses stable rules and exact suitability checks. It does not access a
// model, and excludes invalid alternatives rather than disguising them as tables.
func Select(ctx context.Context, d Data, limits Limits) (Selection, error) {
	if err := ValidateData(ctx, d, limits); err != nil {
		return Selection{}, err
	}
	var dimensions, times, measures []string
	for _, c := range d.Columns {
		if measure(c) {
			measures = append(measures, c.ID)
		}
		if c.Type == "temporal" {
			times = append(times, c.ID)
		}
		if category(c) && c.Type != "temporal" {
			dimensions = append(dimensions, c.ID)
		}
	}
	var candidates []Candidate
	for _, e := range Catalog() {
		if e.Kind == Table {
			continue
		}
		var b Bindings
		var order []Order
		score, reason := 60, "categorical_comparison"
		switch e.Kind {
		case KPI:
			if len(d.Rows) > 1 || len(measures) != 1 {
				continue
			}
			b.Value, score, reason = measures[0], 100, "single_value"
		case Line, Area:
			if len(times) == 0 || len(measures) == 0 {
				continue
			}
			b.Category, b.Value = times[0], measures[0]
			order = []Order{{Column: b.Category, Direction: "asc"}}
			score, reason = 95, "time_series"
			if e.Kind == Area {
				score = 85
			}
		case Scatter:
			if len(measures) < 2 {
				continue
			}
			b.X, b.Y, score, reason = measures[0], measures[1], 90, "numeric_relationship"
		case Heatmap:
			if len(dimensions) < 2 || len(measures) < 1 {
				continue
			}
			b.X, b.Y, b.Value, score, reason = dimensions[0], dimensions[1], measures[0], 85, "two_dimensional_intensity"
		case GroupedBar, StackedBar, StackedColumn:
			if len(dimensions) < 2 || len(measures) < 1 {
				continue
			}
			b.Category, b.Series, b.Value, score, reason = dimensions[0], dimensions[1], measures[0], 90, "category_by_series"
			if e.Kind != GroupedBar {
				score = 80
			}
		default:
			if len(dimensions) == 0 || len(measures) == 0 {
				continue
			}
			b.Category, b.Value = dimensions[0], measures[0]
			if e.Kind == Bar {
				score = 90
			}
			if e.Kind == ColumnChart {
				score = 85
			}
			if e.Kind == Treemap && len(dimensions) > 1 {
				b.Parent = dimensions[1]
				score, reason = 75, "bounded_hierarchy"
			}
			if e.Kind == Pie || e.Kind == Donut {
				score, reason = 65, "nonnegative_parts"
			}
		}
		m := Mapping{Version: Version, Kind: e.Kind, Bindings: b, Order: order, Options: DefaultOptions()}
		for _, id := range bound(b) {
			m.Columns = append(m.Columns, d.Columns[columnIndex(d.Columns, id)])
		}
		if err := validateMapping(ctx, d, m, limits); err != nil {
			if errors.Is(err, ErrUnsuitable) {
				continue
			}
			return Selection{}, err
		}
		if score >= limits.SelectionFloor {
			candidates = append(candidates, Candidate{Mapping: m, Score: score, Reason: reason})
		}
	}
	if len(candidates) == 0 {
		ids := make([]string, len(d.Columns))
		for i := range d.Columns {
			ids[i] = d.Columns[i].ID
		}
		mapping := Mapping{Version: Version, Kind: Table, Columns: append([]Column(nil), d.Columns...), Bindings: Bindings{Columns: ids}, Options: DefaultOptions()}
		if err := validateMapping(ctx, d, mapping, limits); err != nil {
			return Selection{}, err
		}
		return Selection{Selected: Candidate{Mapping: mapping, Score: 0, Reason: "table_fallback"}, Alternatives: []Candidate{}, Fallback: true, Reason: "no_suitable_chart_above_floor"}, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	count := len(candidates) - 1
	if count > limits.MaxAlternatives {
		count = limits.MaxAlternatives
	}
	return Selection{Selected: candidates[0], Alternatives: append([]Candidate{}, candidates[1:1+count]...), Reason: "rules_first"}, nil
}
