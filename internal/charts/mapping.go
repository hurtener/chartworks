package charts

import (
	"context"
	"encoding/json"
	"time"
)

// Catalog returns a detached, stable inventory. No kind is a table alias.
func Catalog() []CatalogEntry {
	return []CatalogEntry{
		{Area, []string{"category", "value"}, nil, "allowed", "gap"},
		{Bar, []string{"category", "value"}, nil, "allowed", "omit"},
		{ColumnChart, []string{"category", "value"}, nil, "allowed", "omit"},
		{Donut, []string{"category", "value"}, nil, "rejected", "omit"},
		{GroupedBar, []string{"category", "value", "series"}, nil, "allowed", "omit"},
		{Heatmap, []string{"x", "y", "value"}, nil, "allowed", "omit"},
		{KPI, []string{"value"}, nil, "allowed", "empty"},
		{Line, []string{"category", "value"}, nil, "allowed", "gap"},
		{Pie, []string{"category", "value"}, nil, "rejected", "omit"},
		{Scatter, []string{"x", "y"}, []string{"series"}, "allowed", "omit"},
		{StackedBar, []string{"category", "value", "series"}, nil, "allowed", "omit"},
		{StackedColumn, []string{"category", "value", "series"}, nil, "allowed", "omit"},
		{Table, []string{"columns"}, nil, "allowed", "preserve"},
		{Treemap, []string{"category", "value"}, []string{"parent"}, "rejected", "omit"},
	}
}

func entry(kind Kind) (CatalogEntry, bool) {
	for _, item := range Catalog() {
		if item.Kind == kind {
			return item, true
		}
	}
	return CatalogEntry{}, false
}

func slot(b Bindings, name string) string {
	switch name {
	case "category":
		return b.Category
	case "value":
		return b.Value
	case "series":
		return b.Series
	case "x":
		return b.X
	case "y":
		return b.Y
	case "parent":
		return b.Parent
	case "columns":
		if len(b.Columns) > 0 {
			return b.Columns[0]
		}
	}
	return ""
}

func bound(b Bindings) []string {
	ids := make([]string, 0, 6+len(b.Columns))
	for _, s := range []string{b.Category, b.Value, b.Series, b.X, b.Y, b.Parent} {
		if s != "" {
			ids = append(ids, s)
		}
	}
	return append(ids, b.Columns...)
}

func cloneMapping(m Mapping) Mapping {
	m.Columns = append([]Column(nil), m.Columns...)
	m.Bindings.Columns = append([]string(nil), m.Bindings.Columns...)
	m.Order = append([]Order(nil), m.Order...)
	return m
}

// Bind constructs an explicit saved mapping with exact column pins. It neither
// chooses a different kind nor changes any supplied data or saved definition.
func Bind(ctx context.Context, d Data, kind Kind, b Bindings, order []Order, options Options, limits Limits) (Mapping, error) {
	if err := ValidateData(ctx, d, limits); err != nil {
		return Mapping{}, err
	}
	if len(b.Columns) > limits.MaxColumns || len(order) > limits.MaxColumns {
		return Mapping{}, ErrLimit
	}
	m := Mapping{Version: Version, Kind: kind, Bindings: b, Order: order, Options: options}
	for _, id := range bound(b) {
		i := columnIndex(d.Columns, id)
		if i < 0 {
			return Mapping{}, ErrInvalid
		}
		m.Columns = append(m.Columns, d.Columns[i])
	}
	if err := validateMapping(ctx, d, m, limits); err != nil {
		return Mapping{}, err
	}
	return cloneMapping(m), nil
}

// ValidateMapping enforces the saved schema and suitability, without rebinding.
func ValidateMapping(ctx context.Context, d Data, m Mapping, limits Limits) error {
	if err := ValidateData(ctx, d, limits); err != nil {
		return err
	}
	return validateMapping(ctx, d, m, limits)
}

func validateMapping(ctx context.Context, d Data, m Mapping, l Limits) error {
	if len(m.Bindings.Columns) > l.MaxColumns {
		return ErrLimit
	}
	e, ok := entry(m.Kind)
	if !ok || m.Version != Version || len(m.Columns) == 0 || len(m.Columns) > l.MaxColumns || len(m.Order) > len(m.Columns) {
		return ErrInvalid
	}
	if err := validateOptions(m.Options, l); err != nil {
		return err
	}
	for _, required := range e.RequiredSlots {
		if slot(m.Bindings, required) == "" {
			return ErrUnsuitable
		}
	}
	for _, name := range []string{"category", "value", "series", "x", "y", "parent", "columns"} {
		if slot(m.Bindings, name) != "" && !oneOf(name, e.RequiredSlots...) && !oneOf(name, e.OptionalSlots...) {
			return ErrInvalid
		}
	}
	ids := bound(m.Bindings)
	if len(ids) != len(m.Columns) {
		return ErrInvalid
	}
	seen := make(map[string]bool, len(ids))
	for ordinal, id := range ids {
		if m.Columns[ordinal].ID != id {
			return ErrInvalid
		}
		if !identifier(id) || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
		i, j := columnIndex(d.Columns, id), columnIndex(m.Columns, id)
		if j < 0 || validateColumn(m.Columns[j]) != nil {
			return ErrInvalid
		}
		if i < 0 || d.Columns[i] != m.Columns[j] {
			return ErrMappingChanged
		}
	}
	ordered := map[string]bool{}
	for _, o := range m.Order {
		if !seen[o.Column] || ordered[o.Column] || !oneOf(o.Direction, "asc", "desc") {
			return ErrInvalid
		}
		ordered[o.Column] = true
	}
	b := m.Bindings
	for _, id := range []string{b.Category, b.Series, b.Parent} {
		if id != "" && !category(d.Columns[columnIndex(d.Columns, id)]) {
			return ErrUnsuitable
		}
	}
	if b.Value != "" && !numeric(d.Columns[columnIndex(d.Columns, b.Value)].Type) {
		return ErrUnsuitable
	}
	if m.Kind == Line || m.Kind == Area {
		if d.Columns[columnIndex(d.Columns, b.Category)].Type != "temporal" {
			return ErrUnsuitable
		}
	}
	if m.Kind == Heatmap || m.Kind == Scatter {
		for _, id := range []string{b.X, b.Y} {
			c := d.Columns[columnIndex(d.Columns, id)]
			if m.Kind == Scatter && !numeric(c.Type) || m.Kind == Heatmap && !category(c) {
				return ErrUnsuitable
			}
		}
	}
	if m.Kind == KPI && len(d.Rows) > 1 {
		return ErrUnsuitable
	}
	return validateShape(ctx, d, m, l, e)
}

func at(d Data, row []Cell, id string) Cell {
	if id == "" {
		return Cell{Null: true}
	}
	return row[columnIndex(d.Columns, id)]
}

func omitted(d Data, row []Cell, m Mapping) bool {
	b := m.Bindings
	switch m.Kind {
	case Table, KPI:
		return false
	case Line, Area:
		return at(d, row, b.Category).Null
	case Scatter:
		return at(d, row, b.X).Null || at(d, row, b.Y).Null || b.Series != "" && at(d, row, b.Series).Null
	case Heatmap:
		return at(d, row, b.X).Null || at(d, row, b.Y).Null || at(d, row, b.Value).Null
	default:
		return at(d, row, b.Category).Null || at(d, row, b.Value).Null || b.Series != "" && at(d, row, b.Series).Null || b.Parent != "" && at(d, row, b.Parent).Null
	}
}

func validateShape(ctx context.Context, d Data, m Mapping, l Limits, e CatalogEntry) error {
	if m.Kind == Table {
		return nil
	}
	categories, series, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, row := range d.Rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		b := m.Bindings
		for _, id := range []string{b.Value, b.X, b.Y} {
			if id == "" || !numeric(d.Columns[columnIndex(d.Columns, id)].Type) {
				continue
			}
			cell := at(d, row, id)
			if _, err := numberValue(cell); err != nil {
				return err
			}
			if !cell.Null && e.Negative == "rejected" {
				r, _, _ := decimal(cell.Value)
				if r.Sign() < 0 {
					return ErrUnsuitable
				}
			}
		}
		if omitted(d, row, m) {
			continue
		}
		if m.Kind == Line || m.Kind == Area {
			if _, ok := temporal(at(d, row, b.Category).Value); !ok {
				return ErrUnsuitable
			}
		}
		if m.Kind == KPI {
			continue
		}
		if b.Category != "" {
			categories[at(d, row, b.Category).Value] = true
		}
		if m.Kind == Heatmap {
			categories["x:"+at(d, row, b.X).Value] = true
			categories["y:"+at(d, row, b.Y).Value] = true
		}
		if b.Parent != "" {
			parent := at(d, row, b.Parent).Value
			if parent == at(d, row, b.Category).Value {
				return ErrUnsuitable
			}
			series[parent] = true
		}
		if b.Series != "" {
			series[at(d, row, b.Series).Value] = true
		}
		if len(categories) > l.MaxCategories || len(series) > l.MaxSeries {
			return ErrUnsuitable
		}
		// Scatter preserves repeated observations. Other plots require unambiguous
		// category/series cells, rather than silently inventing an aggregation.
		if m.Kind != Scatter {
			parts := []Cell{at(d, row, b.Category), at(d, row, b.Series), at(d, row, b.Parent)}
			if m.Kind == Line || m.Kind == Area {
				instant, _ := temporal(parts[0].Value)
				parts[0].Value = instant.UTC().Format(time.RFC3339Nano)
			}
			if m.Kind == Heatmap {
				parts = []Cell{at(d, row, b.X), at(d, row, b.Y)}
			}
			key, _ := json.Marshal(parts)
			if keys[string(key)] {
				return ErrUnsuitable
			}
			keys[string(key)] = true
		}
	}
	return nil
}

func sameMeaning(old, current Column) bool {
	return old.Type == current.Type && old.Role == current.Role && old.Grain == current.Grain && old.Aggregation == current.Aggregation && old.Format == current.Format &&
		old.Provenance.Source == current.Provenance.Source && old.Provenance.Topic != "" && old.Provenance.Topic == current.Provenance.Topic && old.Provenance.SemanticID == current.Provenance.SemanticID
}

// Rebind proposes an unambiguous same-meaning replacement for authoring. Stable
// semantic IDs, not display-name similarity, establish candidate associations.
// Even unchanged mappings are detached, review-required proposals.
func Rebind(ctx context.Context, d Data, original Mapping, limits Limits) (Proposal, error) {
	if err := ValidateData(ctx, d, limits); err != nil {
		return Proposal{}, err
	}
	oldData := Data{Version: Version, Columns: original.Columns, Rows: [][]Cell{}, Completeness: Completeness{Status: "complete_result"}}
	if err := ValidateMapping(ctx, oldData, original, limits); err != nil {
		return Proposal{}, err
	}
	m := cloneMapping(original)
	changes := []Change{}
	replacements := map[string]string{}
	for _, old := range original.Columns {
		if i := columnIndex(d.Columns, old.ID); i >= 0 && d.Columns[i] == old {
			replacements[old.ID] = old.ID
			continue
		}
		var matches []Column
		for _, current := range d.Columns {
			if sameMeaning(old, current) {
				matches = append(matches, current)
			}
		}
		if len(matches) != 1 {
			return Proposal{}, ErrMappingChanged
		}
		replacements[old.ID] = matches[0].ID
		changes = append(changes, Change{From: old.ID, To: matches[0].ID})
	}
	b := &m.Bindings
	for _, id := range []*string{&b.Category, &b.Value, &b.Series, &b.X, &b.Y, &b.Parent} {
		if *id != "" {
			*id = replacements[*id]
		}
	}
	for i, id := range b.Columns {
		b.Columns[i] = replacements[id]
	}
	for i := range m.Order {
		m.Order[i].Column = replacements[m.Order[i].Column]
	}
	m.Columns = nil
	for _, id := range bound(m.Bindings) {
		m.Columns = append(m.Columns, d.Columns[columnIndex(d.Columns, id)])
	}
	if err := validateMapping(ctx, d, m, limits); err != nil {
		return Proposal{}, err
	}
	return Proposal{Status: "review_required", Mapping: m, Changes: changes}, nil
}
