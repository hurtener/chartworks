package charts

import (
	"context"
	"encoding/json"
	"math/big"
)

// richBudget bounds incremental expansion before storing another drawing record.
// Final encoded size is checked too; neither a large series cross-product nor a
// long repeated label may allocate an unbounded intermediate output.
type richBudget struct{ used, max int }

func (b *richBudget) take(value any) error {
	wire, err := json.Marshal(value)
	if err != nil {
		return ErrInvalid
	}
	if len(wire)+1 > b.max-b.used {
		return ErrLimit
	}
	b.used += len(wire) + 1
	return nil
}

func emptyPoint(row int) Point {
	return Point{Row: row, Category: Cell{Null: true}, Series: Cell{Null: true}, Parent: Cell{Null: true}, X: Value{Null: true}, Y: Value{Null: true}, Value: Value{Null: true}}
}

func resultScope(d Data) string {
	if d.Completeness.Status == "truncated" {
		return "returned_rows"
	}
	return "complete_result"
}

func buildRich(ctx context.Context, d Data, m Mapping, limits Limits) (Output, error) {
	out := Output{Version: RichVersion, Kind: m.Kind, Mapping: cloneMapping(m), Columns: append([]Column(nil), m.Columns...),
		Rows: [][]Cell{}, Points: []Point{}, Totals: []Total{}, State: "empty", InputRows: len(d.Rows), Completeness: d.Completeness, Warnings: []string{},
		Transformation: &Transformation{Version: 1, Method: "retained_wide_to_series", DuplicatePolicy: "reject", NullPolicy: "retain_values_omit_missing_coordinates", Scope: resultScope(d)}}
	if d.Completeness.Status == "truncated" {
		out.Warnings = append(out.Warnings, "truncated_result_not_full_source")
	}
	budget := &richBudget{max: limits.MaxBytes * 4, used: 1024}
	if err := budget.take(out); err != nil {
		return Output{}, err
	}
	indexes := orderedRows(ctx, d, m)
	for _, i := range indexes {
		if err := ctx.Err(); err != nil {
			return Output{}, err
		}
		row := make([]Cell, len(m.Columns))
		for j, column := range m.Columns {
			row[j] = at(d, d.Rows[i], column.ID)
		}
		if err := budget.take(row); err != nil {
			return Output{}, err
		}
		if err := budget.take(i); err != nil {
			return Output{}, err
		}
		out.Rows = append(out.Rows, row)
		out.RowIndices = append(out.RowIndices, i)
	}
	var err error
	switch {
	case len(m.Bindings.Hierarchy) > 0:
		err = buildHierarchy(ctx, d, m, indexes, &out, budget)
	case m.Kind == Scatter:
		err = buildBubbles(ctx, d, m, indexes, &out, budget)
	default:
		err = buildMeasures(ctx, d, m, indexes, &out, budget)
	}
	if err != nil {
		return Output{}, err
	}
	nonNull, positive, approximate := false, false, false
	for _, p := range out.Points {
		for _, v := range []*Value{&p.Value, &p.X, &p.Y, p.Size} {
			if v == nil {
				continue
			}
			approximate = approximate || v.Approximate
			if !v.Null {
				nonNull = true
			}
			if v.Coordinate != nil && *v.Coordinate > 0 {
				positive = true
			}
		}
	}
	zeroArea := false
	for _, node := range out.Hierarchy {
		zeroArea = zeroArea || node.Value.Coordinate != nil && *node.Value.Coordinate == 0
		approximate = approximate || node.Value.Approximate
	}
	if len(d.Rows) > 0 {
		out.State = "no_values"
	}
	if nonNull {
		out.State = "ready"
	}
	if m.Kind == Treemap && nonNull && !positive {
		out.State = "no_positive_values"
	}
	if out.OmittedRows > out.Transformation.ZeroSizePoints {
		out.Warnings = append(out.Warnings, "null_coordinates_omitted")
	}
	if out.Transformation.ZeroSizePoints > 0 {
		out.Warnings = append(out.Warnings, "zero_size_not_drawn")
	}
	if out.Transformation.MissingPoints > 0 {
		out.Warnings = append(out.Warnings, "missing_values_preserved")
	}
	if out.Transformation.GapPoints > 0 {
		out.Warnings = append(out.Warnings, "missing_series_observations_are_gaps")
	}
	if zeroArea {
		out.Warnings = append(out.Warnings, "zero_area_nodes_not_drawn")
	}
	if approximate {
		out.Warnings = append(out.Warnings, "geometry_approximate_labels_exact")
	}
	out.Totals, err = totals(ctx, d, m.Columns)
	if err != nil {
		return Output{}, err
	}
	wire, err := json.Marshal(out)
	if err != nil {
		return Output{}, ErrInvalid
	}
	if len(wire) > limits.MaxBytes*4 {
		return Output{}, ErrLimit
	}
	return out, ctx.Err()
}

func seriesID(d Data, row []Cell, measure, series string) string {
	wire, _ := json.Marshal([]string{measure, identityKey(d, row, []string{series})})
	return stableID(string(wire))
}

func makeSeries(d Data, row []Cell, measure, series string) SeriesDefinition {
	c := d.Columns[columnIndex(d.Columns, measure)]
	breakdown := at(d, row, series)
	name := c.Name
	if series != "" {
		name += " — " + breakdown.Value
	}
	return SeriesDefinition{ID: seriesID(d, row, measure, series), Measure: measure, Name: name, Breakdown: breakdown, Format: c.Format}
}

func buildMeasures(ctx context.Context, d Data, m Mapping, indexes []int, out *Output, budget *richBudget) error {
	b := m.Bindings
	values := valueIDs(b)
	breakdowns, seenBreakdowns := []int{}, map[string]bool{}
	for _, i := range indexes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if coordinateMissing(d, d.Rows[i], richCoordinates(m)) {
			continue
		}
		key := identityKey(d, d.Rows[i], []string{b.Series})
		if !seenBreakdowns[key] {
			seenBreakdowns[key] = true
			breakdowns = append(breakdowns, i)
		}
	}
	// Measures are the outer order, then first-observed categorical breakdowns.
	for _, measure := range values {
		for _, i := range breakdowns {
			definition := makeSeries(d, d.Rows[i], measure, b.Series)
			if err := budget.take(definition); err != nil {
				return err
			}
			out.Series = append(out.Series, definition)
		}
	}
	type pointKey struct{ category, series string }
	type categoryLabel struct {
		key   string
		label Cell
	}
	categories, seenCategories := []categoryLabel{}, map[string]bool{}
	observations := map[pointKey]Point{}
	line := m.Kind == Line || m.Kind == Area
	for _, i := range indexes {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := d.Rows[i]
		if coordinateMissing(d, row, richCoordinates(m)) {
			out.OmittedRows++
			continue
		}
		key := identityKey(d, row, []string{b.Category})
		if !seenCategories[key] {
			seenCategories[key] = true
			categories = append(categories, categoryLabel{key, at(d, row, b.Category)})
		}
		for _, measure := range values {
			p := emptyPoint(i)
			p.Category, p.Series = at(d, row, b.Category), at(d, row, b.Series)
			p.Measure, p.SeriesID, p.CategoryKey = measure, seriesID(d, row, measure, b.Series), key
			value, err := numberValue(at(d, row, measure))
			if err != nil {
				return err
			}
			p.Value = value
			if value.Null {
				out.Transformation.MissingPoints++
			}
			if err = budget.take(p); err != nil {
				return err
			}
			if line {
				observations[pointKey{key, p.SeriesID}] = p
			} else {
				out.Points = append(out.Points, p)
			}
		}
	}
	if line {
		for _, series := range out.Series {
			for _, category := range categories {
				if err := ctx.Err(); err != nil {
					return err
				}
				p, found := observations[pointKey{category.key, series.ID}]
				if !found {
					p = emptyPoint(-1)
					p.Category, p.CategoryKey, p.Series = category.label, category.key, series.Breakdown
					p.Measure, p.SeriesID = series.Measure, series.ID
					out.Transformation.GapPoints++
					if err := budget.take(p); err != nil {
						return err
					}
				}
				out.Points = append(out.Points, p)
			}
		}
	}
	return nil
}

func buildBubbles(ctx context.Context, d Data, m Mapping, indexes []int, out *Output, budget *richBudget) error {
	b := m.Bindings
	out.Transformation.Method, out.Transformation.DuplicatePolicy = "retained_bubbles", "preserve_observations"
	out.Transformation.SizeEncoding, out.Transformation.NullPolicy = "area", "omit_missing_coordinates_or_size"
	seen := map[string]bool{}
	for _, i := range indexes {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := d.Rows[i]
		if coordinateMissing(d, row, richCoordinates(m)) {
			out.OmittedRows++
			continue
		}
		size, err := numberValue(at(d, row, b.Size))
		if err != nil {
			return err
		}
		if *size.Coordinate == 0 {
			out.OmittedRows++
			out.Transformation.ZeroSizePoints++
			continue
		}
		p := emptyPoint(i)
		p.Series, p.Size = at(d, row, b.Series), &size
		p.X, err = numberValue(at(d, row, b.X))
		if err != nil {
			return err
		}
		p.Y, err = numberValue(at(d, row, b.Y))
		if err != nil {
			return err
		}
		definition := makeSeries(d, row, b.Y, b.Series)
		p.SeriesID = definition.ID
		if !seen[definition.ID] {
			seen[definition.ID] = true
			if err = budget.take(definition); err != nil {
				return err
			}
			out.Series = append(out.Series, definition)
		}
		if err = budget.take(p); err != nil {
			return err
		}
		out.Points = append(out.Points, p)
	}
	return nil
}

func buildHierarchy(ctx context.Context, d Data, m Mapping, indexes []int, out *Output, budget *richBudget) error {
	b := m.Bindings
	out.Transformation.Method, out.Transformation.Aggregation = "ordered_hierarchy_sum", "sum"
	out.Transformation.NullPolicy = "omit_incomplete_path_or_value"
	type accumulated struct {
		node  HierarchyNode
		sum   *big.Rat
		scale int
		leaf  Cell
	}
	nodes, ordered := map[string]*accumulated{}, []*accumulated{}
	for _, i := range indexes {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := d.Rows[i]
		cell := at(d, row, b.Value)
		if coordinateMissing(d, row, b.Hierarchy) || cell.Null {
			out.OmittedRows++
			continue
		}
		r, scale, err := decimal(cell.Value)
		if err != nil {
			return err
		}
		path := make([]Cell, len(b.Hierarchy))
		parent := ""
		for level, id := range b.Hierarchy {
			path[level] = at(d, row, id)
			key := identityKey(d, row, b.Hierarchy[:level+1])
			a := nodes[key]
			if a == nil {
				a = &accumulated{node: HierarchyNode{ID: key, Parent: parent, Depth: level, Path: append([]Cell{}, path[:level+1]...), Aggregation: "sum", Scope: "returned_complete_paths"}, sum: new(big.Rat)}
				nodes[key] = a
				ordered = append(ordered, a)
			}
			// Typed-equal parent coordinates share the first ordered spelling.
			// Reuse that label in descendants so their paths agree with the
			// shared node; out.Rows still retains every original source cell.
			path[level] = a.node.Path[level]
			a.sum.Add(a.sum, r)
			if scale > a.scale {
				a.scale = scale
			}
			a.node.Rows = append(a.node.Rows, i)
			if level == len(b.Hierarchy)-1 {
				a.node.Aggregation = "identity"
				a.leaf = cell
			}
			parent = key
		}
		p := emptyPoint(i)
		p.Path, p.CategoryKey, p.Category, p.Measure = path, parent, path[len(path)-1], b.Value
		if len(path) > 1 {
			p.Parent = path[len(path)-2]
		}
		p.Value, err = numberValue(cell)
		if err != nil {
			return err
		}
		if err = budget.take(p); err != nil {
			return err
		}
		out.Points = append(out.Points, p)
	}
	for _, a := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		cell := a.leaf
		if a.node.Aggregation == "sum" {
			cell = Cell{Value: a.sum.FloatString(a.scale)}
		}
		v, err := numberValue(cell)
		if err != nil {
			return err
		}
		a.node.Value = v
		if err = budget.take(a.node); err != nil {
			return err
		}
		out.Hierarchy = append(out.Hierarchy, a.node)
	}
	return nil
}
