package charts

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
)

func buildDisplay(ctx context.Context, d Data, m Mapping, limits Limits) (Output, error) {
	out := Output{Version: DisplayVersion, Kind: m.Kind, Mapping: cloneMapping(m), State: "empty", InputRows: len(d.Rows), Completeness: d.Completeness, Warnings: []string{}, Points: []Point{}, Rows: [][]Cell{}, Totals: []Total{}}
	if d.Completeness.Status == "truncated" {
		out.Warnings = append(out.Warnings, "truncated_result_not_full_source")
	}
	if m.Kind == Table {
		if err := buildDisplayTable(ctx, d, m, &out); err != nil {
			return Output{}, err
		}
	} else if err := buildDisplayKPI(ctx, d, m, &out); err != nil {
		return Output{}, err
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return Output{}, ErrInvalid
	}
	if len(encoded) > limits.MaxBytes*4 {
		return Output{}, ErrLimit
	}
	return out, ctx.Err()
}

func buildDisplayTable(ctx context.Context, d Data, m Mapping, out *Output) error {
	visible := map[string]bool{}
	for _, item := range m.Table.Columns {
		visible[item.Column] = item.Visible
	}
	for _, column := range m.Columns {
		if visible[column.ID] {
			out.Columns = append(out.Columns, column)
		}
	}
	for _, index := range orderedRows(ctx, d, m) {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := []Cell{}
		for _, id := range m.Bindings.Columns {
			if visible[id] {
				row = append(row, at(d, d.Rows[index], id))
			}
		}
		out.Rows = append(out.Rows, row)
	}
	out.TablePageSize, out.ShowTotals = m.Table.PageSize, m.Table.ShowTotals
	if m.Table.ShowTotals {
		var err error
		out.Totals, err = totals(ctx, d, out.Columns)
		if err != nil {
			return err
		}
	}
	if len(out.Rows) > 0 {
		out.State = "ready"
	}
	return nil
}

func buildDisplayKPI(ctx context.Context, d Data, m Mapping, out *Output) error {
	out.Columns = append([]Column(nil), m.Columns...)
	if len(d.Rows) == 0 {
		return nil
	}
	indexes := orderedRows(ctx, d, m)
	selected := 0
	if m.KPI.ValueRow == "last" {
		selected = len(indexes) - 1
	}
	row := d.Rows[indexes[selected]]
	value, err := numberValue(at(d, row, m.Bindings.Value))
	if err != nil {
		return err
	}
	result := &KPIResult{Value: value, Sparkline: []Value{}}
	if m.KPI.Sparkline {
		for _, i := range indexes {
			v, e := numberValue(at(d, d.Rows[i], m.Bindings.Value))
			if e != nil {
				return e
			}
			result.Sparkline = append(result.Sparkline, v)
		}
	}
	var comparison *Value
	switch m.KPI.ComparisonMode {
	case "comparison_column":
		v, e := numberValue(at(d, row, m.Bindings.Comparison))
		if e != nil {
			return e
		}
		comparison = &v
	case "previous_row":
		prior := selected - 1
		if prior >= 0 && prior < len(indexes) {
			v, e := numberValue(at(d, d.Rows[indexes[prior]], m.Bindings.Value))
			if e != nil {
				return e
			}
			comparison = &v
		}
	}
	result.Comparison = comparison
	if comparison != nil && !value.Null && !comparison.Null {
		delta, e := exactDifference(value.Exact, comparison.Exact)
		if e != nil {
			return e
		}
		if m.KPI.ShowDelta {
			result.Delta = &delta
		}
		if m.KPI.ShowPercentDelta {
			p, e := exactPercent(delta.Exact, comparison.Exact, m.Columns[columnIndex(m.Columns, m.Bindings.Value)].Format.FractionDigits)
			if e != nil {
				return e
			}
			result.PercentDelta = p
		}
	}
	if m.Bindings.Target != "" {
		target, e := numberValue(at(d, row, m.Bindings.Target))
		if e != nil {
			return e
		}
		result.Target = &target
		if !value.Null && !target.Null {
			difference, e := exactDifference(value.Exact, target.Exact)
			if e != nil {
				return e
			}
			result.TargetDifference = &difference
		}
	}
	if !value.Null {
		v, _, _ := decimal(value.Exact)
		for _, threshold := range m.KPI.Thresholds {
			t, _, _ := decimal(threshold.Value)
			cmp := v.Cmp(t)
			matched := threshold.Operator == "lt" && cmp < 0 || threshold.Operator == "lte" && cmp <= 0 || threshold.Operator == "gt" && cmp > 0 || threshold.Operator == "gte" && cmp >= 0
			if matched {
				result.ThresholdState, result.ThresholdLabel = threshold.State, threshold.Label
				break
			}
		}
	}
	out.KPIResult, out.Points = result, []Point{{Row: indexes[selected], Value: value}}
	if !value.Null {
		out.State = "ready"
	} else {
		out.State = "no_values"
	}
	return nil
}

func exactDifference(left, right string) (Value, error) {
	a, as, err := decimal(left)
	if err != nil {
		return Value{}, err
	}
	b, bs, err := decimal(right)
	if err != nil {
		return Value{}, err
	}
	scale := as
	if bs > scale {
		scale = bs
	}
	return valueFromRat(new(big.Rat).Sub(a, b), scale)
}

func exactPercent(delta, base string, digits int) (*Value, error) {
	a, _, err := decimal(delta)
	if err != nil {
		return nil, err
	}
	b, _, err := decimal(base)
	if err != nil {
		return nil, err
	}
	if b.Sign() == 0 {
		return nil, nil
	}
	if digits < 0 {
		digits = 0
	}
	if digits > 20 {
		digits = 20
	}
	returnValue, err := valueFromRat(new(big.Rat).Mul(new(big.Rat).Quo(a, b), big.NewRat(100, 1)), digits)
	if err != nil {
		return nil, err
	}
	return &returnValue, nil
}

func valueFromRat(r *big.Rat, scale int) (Value, error) {
	exact := r.FloatString(scale)
	f, ok := r.Float64()
	if math.IsInf(f, 0) || math.IsNaN(f) || f == 0 && r.Sign() != 0 {
		return Value{}, ErrUnsuitable
	}
	return Value{Exact: exact, Coordinate: &f, Approximate: !ok}, nil
}
