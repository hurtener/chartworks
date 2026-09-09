package charts

import (
	"context"
	"encoding/json"
	"math/big"
	"sort"
	"strings"
)

// Build applies an exact saved mapping without selection, rebinding, SQL or models.
// Returned rows/definitions are detached; callers cannot mutate shared input.
func Build(ctx context.Context, d Data, m Mapping, limits Limits) (Output, error) {
	if err := ValidateMapping(ctx, d, m, limits); err != nil {
		return Output{}, err
	}
	out := Output{Version: Version, Kind: m.Kind, Mapping: cloneMapping(m), Columns: append([]Column(nil), m.Columns...),
		Rows: [][]Cell{}, Points: []Point{}, Totals: []Total{}, State: "empty", InputRows: len(d.Rows), Completeness: d.Completeness, Warnings: []string{}}
	if d.Completeness.Status == "truncated" {
		out.Warnings = append(out.Warnings, "truncated_result_not_full_source")
	}
	indexes := make([]int, len(d.Rows))
	for i := range indexes {
		indexes[i] = i
	}
	if len(m.Order) > 0 {
		sort.SliceStable(indexes, func(i, j int) bool {
			if ctx.Err() != nil {
				return false
			}
			for _, order := range m.Order {
				col := columnIndex(d.Columns, order.Column)
				a, b := d.Rows[indexes[i]][col], d.Rows[indexes[j]][col]
				if a.Null != b.Null {
					return !a.Null
				}
				if a.Null {
					continue
				}
				cmp := compare(a.Value, b.Value, d.Columns[col].Type)
				if cmp == 0 {
					continue
				}
				if order.Direction == "desc" {
					return cmp > 0
				}
				return cmp < 0
			}
			return false
		})
	}
	nonNull, positive, approximate := false, false, false
	for _, i := range indexes {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		row := d.Rows[i]
		if omitted(d, row, m) {
			out.OmittedRows++
			continue
		}
		if m.Kind == Table {
			projected := make([]Cell, len(m.Bindings.Columns))
			for j, id := range m.Bindings.Columns {
				projected[j] = at(d, row, id)
			}
			out.Rows = append(out.Rows, projected)
			nonNull = true
			continue
		}
		b := m.Bindings
		point := Point{Row: i, Category: at(d, row, b.Category), Series: at(d, row, b.Series), Parent: at(d, row, b.Parent), X: Value{Null: true}, Y: Value{Null: true}, Value: Value{Null: true}}
		for _, item := range []struct {
			id string
			to *Value
		}{{b.Value, &point.Value}, {b.X, &point.X}, {b.Y, &point.Y}} {
			if item.id == "" {
				continue
			}
			cell := at(d, row, item.id)
			if numeric(d.Columns[columnIndex(d.Columns, item.id)].Type) {
				value, err := numberValue(cell)
				if err != nil {
					return Output{}, err
				}
				*item.to = value
			} else {
				*item.to = Value{Null: cell.Null, Exact: cell.Value}
			}
			approximate = approximate || item.to.Approximate
			if !cell.Null {
				nonNull = true
				if item.to.Coordinate != nil && *item.to.Coordinate > 0 {
					positive = true
				}
			}
		}
		out.Points = append(out.Points, point)
	}
	if nonNull {
		out.State = "ready"
	}
	if len(d.Rows) > 0 && !nonNull {
		out.State = "no_values"
	}
	if len(d.Rows) > 0 && !positive && (m.Kind == Pie || m.Kind == Donut || m.Kind == Treemap) && nonNull {
		out.State = "no_positive_values"
	}
	if out.OmittedRows > 0 {
		out.Warnings = append(out.Warnings, "null_coordinates_omitted")
	}
	if approximate {
		out.Warnings = append(out.Warnings, "geometry_approximate_labels_exact")
	}
	var err error
	out.Totals, err = totals(ctx, d, m.Columns)
	if err != nil {
		return Output{}, err
	}
	// Bound the actual serialized renderer input, not only source row counts.
	encoded, err := json.Marshal(out)
	if err != nil {
		return Output{}, ErrInvalid
	}
	if len(encoded) > limits.MaxBytes*4 {
		return Output{}, ErrLimit
	}
	return out, ctx.Err()
}

func compare(a, b, typ string) int {
	if numeric(typ) {
		x, _, _ := decimal(a)
		y, _, _ := decimal(b)
		return x.Cmp(y)
	}
	if typ == "temporal" {
		x, ok := temporal(a)
		y, has := temporal(b)
		if ok && has {
			return x.Compare(y)
		}
	}
	return strings.Compare(a, b)
}

func totals(ctx context.Context, d Data, columns []Column) ([]Total, error) {
	out := []Total{}
	for _, c := range columns {
		if !numeric(c.Type) || c.Format.Percent != "" || !oneOf(c.Aggregation, "sum", "count") {
			continue
		}
		sum, scale, count := new(big.Rat), 0, 0
		for _, row := range d.Rows {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			v := at(d, row, c.ID)
			if v.Null {
				continue
			}
			r, s, err := decimal(v.Value)
			if err != nil {
				return nil, err
			}
			if s > scale {
				scale = s
			}
			sum.Add(sum, r)
			count++
		}
		v := Cell{Null: count == 0}
		if count > 0 {
			v.Value = sum.FloatString(scale)
		}
		scope := "complete_result"
		if d.Completeness.Status == "truncated" {
			scope = "returned_rows"
		}
		out = append(out, Total{Column: c.ID, Value: v, Scope: scope})
	}
	return out, nil
}
