package charts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"time"
)

// Rich bindings share the existing byte, column, category and series budgets.
// Expansion has a separate fixed ceiling matching the retained Apps consumer.
const (
	MaxTransformedPoints = 10000
	MaxHierarchyDepth    = 8
)

// BindingVariant describes an executable shape, not another catalog kind.
// RequiredSlots are conjunctive within a variant; variants are alternatives.
type BindingVariant struct {
	ID            string   `json:"id"`
	Version       int      `json:"version"`
	RequiredSlots []string `json:"required_slots"`
	OptionalSlots []string `json:"optional_slots"`
}

// SeriesDefinition gives a stable identity to a measure and optional breakdown.
// Its position is authoritative order. Format does not alter any exact value.
type SeriesDefinition struct {
	ID        string `json:"id"`
	Measure   string `json:"measure"`
	Name      string `json:"name"`
	Breakdown Cell   `json:"breakdown"`
	Format    Format `json:"format"`
}

// HierarchyNode is a flat, bounded tree node. Paths preserve full exact labels;
// IDs distinguish identical labels at different levels or in different branches.
// Rows identifies every contributing original retained row, never warehouse rows.
type HierarchyNode struct {
	ID          string `json:"id"`
	Parent      string `json:"parent"`
	Depth       int    `json:"depth"`
	Path        []Cell `json:"path"`
	Value       Value  `json:"value"`
	Rows        []int  `json:"rows"`
	Aggregation string `json:"aggregation"`
	Scope       string `json:"scope"`
}

// Transformation records deterministic retained-data work. GapPoints are absent
// category/series observations, not zeroes or newly queried rows. MissingPoints
// counts explicit null measure values; ZeroSizePoints are undrawn zero-area bubbles.
type Transformation struct {
	Version         int    `json:"version"`
	Method          string `json:"method"`
	DuplicatePolicy string `json:"duplicate_policy"`
	NullPolicy      string `json:"null_policy"`
	Aggregation     string `json:"aggregation,omitempty"`
	SizeEncoding    string `json:"size_encoding,omitempty"`
	Scope           string `json:"scope"`
	MissingPoints   int    `json:"missing_points"`
	GapPoints       int    `json:"gap_points"`
	ZeroSizePoints  int    `json:"zero_size_points"`
}

func richBindings(kind Kind, b Bindings) bool {
	return len(b.Values) != 0 || len(b.Hierarchy) != 0 || b.Size != "" ||
		b.Series != "" && (kind == Line || kind == Area || kind == Bar || kind == ColumnChart)
}

func variantID(kind Kind, b Bindings) string {
	switch {
	case len(b.Hierarchy) > 0:
		return "ordered_hierarchy"
	case b.Size != "":
		return "bubble"
	case len(b.Values) > 0 && b.Series != "":
		return "multi_measure_series"
	case len(b.Values) > 0:
		return "multi_measure"
	case b.Series != "":
		return "series_breakdown"
	case kind == Treemap && b.Parent != "":
		return "parent_category"
	default:
		return "scalar"
	}
}

func bindingVariants(e CatalogEntry) []BindingVariant {
	out := []BindingVariant{{ID: "scalar", Version: Version, RequiredSlots: append([]string{}, e.RequiredSlots...), OptionalSlots: append([]string{}, e.OptionalSlots...)}}
	switch e.Kind {
	case GroupedBar, StackedBar, StackedColumn:
		out[0].ID = "series_breakdown"
	case Scatter:
		out[0].OptionalSlots = []string{}
		out = append(out, BindingVariant{ID: "series_breakdown", Version: Version, RequiredSlots: []string{"x", "y", "series"}, OptionalSlots: []string{}})
	case Treemap:
		out[0].OptionalSlots = []string{}
		out = append(out, BindingVariant{ID: "parent_category", Version: Version, RequiredSlots: []string{"category", "value", "parent"}, OptionalSlots: []string{}})
	}
	switch e.Kind {
	case Line, Area, Bar, ColumnChart, GroupedBar:
		out = append(out,
			BindingVariant{ID: "multi_measure", Version: RichVersion, RequiredSlots: []string{"category", "values"}, OptionalSlots: []string{}},
			BindingVariant{ID: "multi_measure_series", Version: RichVersion, RequiredSlots: []string{"category", "values", "series"}, OptionalSlots: []string{}})
		if e.Kind != GroupedBar {
			out = append(out, BindingVariant{ID: "series_breakdown", Version: RichVersion, RequiredSlots: []string{"category", "value", "series"}, OptionalSlots: []string{}})
		}
	case Scatter:
		out = append(out, BindingVariant{ID: "bubble", Version: RichVersion, RequiredSlots: []string{"x", "y", "size"}, OptionalSlots: []string{"series"}})
	case Treemap:
		out = append(out, BindingVariant{ID: "ordered_hierarchy", Version: RichVersion, RequiredSlots: []string{"hierarchy", "value"}, OptionalSlots: []string{}})
	}
	return out
}

func validateSlots(m Mapping, e CatalogEntry) error {
	b := m.Bindings
	if !richBindings(m.Kind, b) {
		if m.Version != Version {
			return ErrInvalid
		}
		for _, required := range e.RequiredSlots {
			if slot(b, required) == "" {
				return ErrUnsuitable
			}
		}
		for _, name := range []string{"category", "value", "series", "x", "y", "parent", "columns"} {
			if slot(b, name) != "" && !oneOf(name, e.RequiredSlots...) && !oneOf(name, e.OptionalSlots...) {
				return ErrInvalid
			}
		}
		return nil
	}
	if m.Version != RichVersion {
		return ErrInvalid
	}
	switch m.Kind {
	case Line, Area, Bar, ColumnChart, GroupedBar:
		if b.X != "" || b.Y != "" || b.Size != "" || b.Parent != "" || len(b.Columns) != 0 || len(b.Hierarchy) != 0 {
			return ErrInvalid
		}
		if b.Category == "" || b.Value == "" && len(b.Values) == 0 {
			return ErrUnsuitable
		}
		if b.Value != "" && len(b.Values) != 0 {
			return ErrInvalid
		}
		if m.Kind == GroupedBar && b.Series == "" && len(b.Values) < 2 {
			return ErrUnsuitable
		}
	case Scatter:
		if b.Category != "" || b.Value != "" || b.Parent != "" || len(b.Values) != 0 || len(b.Columns) != 0 || len(b.Hierarchy) != 0 {
			return ErrInvalid
		}
		if b.X == "" || b.Y == "" || b.Size == "" {
			return ErrUnsuitable
		}
	case Treemap:
		if b.Category != "" || b.Parent != "" || b.Series != "" || b.X != "" || b.Y != "" || b.Size != "" || len(b.Values) != 0 || len(b.Columns) != 0 {
			return ErrInvalid
		}
		if b.Value == "" || len(b.Hierarchy) == 0 {
			return ErrUnsuitable
		}
	default:
		return ErrInvalid
	}
	return nil
}

func valueIDs(b Bindings) []string {
	if len(b.Values) != 0 {
		return b.Values
	}
	if b.Value != "" {
		return []string{b.Value}
	}
	return nil
}

// identityKey canonicalizes typed coordinates only, never display text. In
// particular equal instants in different offsets must not become separate cells.
func identityKey(d Data, row []Cell, ids []string) string {
	type part struct {
		Column, Type string
		Cell         Cell
	}
	parts := make([]part, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		c := d.Columns[columnIndex(d.Columns, id)]
		v := at(d, row, id)
		if !v.Null {
			if numeric(c.Type) {
				r, _, _ := decimal(v.Value)
				v.Value = r.RatString()
			}
			if c.Type == "temporal" {
				if instant, ok := temporal(v.Value); ok {
					v.Value = instant.UTC().Format(time.RFC3339Nano)
				}
			}
		}
		parts = append(parts, part{Column: id, Type: c.Type, Cell: v})
	}
	wire, _ := json.Marshal(parts)
	return stableID(string(wire))
}

func stableID(key string) string {
	hash := sha256.Sum256([]byte(key))
	return "s-" + hex.EncodeToString(hash[:])
}

func coordinateMissing(d Data, row []Cell, ids []string) bool {
	for _, id := range ids {
		if id != "" && at(d, row, id).Null {
			return true
		}
	}
	return false
}

func richCoordinates(m Mapping) []string {
	b := m.Bindings
	if len(b.Hierarchy) > 0 {
		return b.Hierarchy
	}
	if m.Kind == Scatter {
		return []string{b.X, b.Y, b.Series, b.Size}
	}
	return []string{b.Category, b.Series}
}

func validateRichShape(ctx context.Context, d Data, m Mapping, l Limits) error {
	b := m.Bindings
	values := valueIDs(b)
	if len(d.Rows) > MaxTransformedPoints || len(values) > 0 && len(d.Rows) > MaxTransformedPoints/len(values) {
		return ErrLimit
	}
	if b.Size != "" {
		c := d.Columns[columnIndex(d.Columns, b.Size)]
		if !measure(c) || c.Format.Percent != "" {
			return ErrUnsuitable
		}
	}
	if len(b.Hierarchy) > 0 {
		c := d.Columns[columnIndex(d.Columns, b.Value)]
		if !oneOf(c.Aggregation, "sum", "count") || c.Format.Percent != "" {
			return ErrUnsuitable
		}
		// A custom sort may stop at a hierarchy prefix, but may not skip or
		// reorder levels or place a measure before the complete path.
		for i, order := range m.Order {
			if i < len(b.Hierarchy) && order.Column != b.Hierarchy[i] {
				return ErrUnsuitable
			}
		}
	}
	if m.Kind == Line || m.Kind == Area {
		if len(m.Order) == 0 || m.Order[0].Column != b.Category {
			return ErrUnsuitable
		}
	}
	keys, categories, breakdowns := map[string]bool{}, map[string]bool{}, map[string]bool{}
	sums := map[string]*big.Rat{}
	for _, row := range d.Rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, id := range append(append([]string{}, values...), b.X, b.Y, b.Size) {
			if id == "" {
				continue
			}
			cell := at(d, row, id)
			if _, err := numberValue(cell); err != nil {
				return err
			}
			if !cell.Null && (len(b.Hierarchy) > 0 || id == b.Size) {
				r, _, _ := decimal(cell.Value)
				if r.Sign() < 0 {
					return ErrUnsuitable
				}
			}
		}
		if coordinateMissing(d, row, richCoordinates(m)) {
			continue
		}
		if m.Kind == Line || m.Kind == Area {
			if _, ok := temporal(at(d, row, b.Category).Value); !ok {
				return ErrUnsuitable
			}
		}
		if b.Category != "" {
			categories[identityKey(d, row, []string{b.Category})] = true
		}
		if b.Series != "" {
			breakdowns[identityKey(d, row, []string{b.Series})] = true
		}
		if len(categories) > l.MaxCategories || len(breakdowns) > l.MaxSeries {
			return ErrUnsuitable
		}
		if m.Kind == Scatter {
			continue
		}
		key := identityKey(d, row, richCoordinates(m))
		// Null values do not erase a tuple's identity or authorize last-row wins.
		if keys[key] {
			return ErrUnsuitable
		}
		keys[key] = true
		if len(b.Hierarchy) > 0 {
			cell := at(d, row, b.Value)
			for level := range b.Hierarchy {
				prefix := identityKey(d, row, b.Hierarchy[:level+1])
				if sums[prefix] == nil {
					sums[prefix] = new(big.Rat)
				}
				if !cell.Null {
					r, _, _ := decimal(cell.Value)
					sums[prefix].Add(sums[prefix], r)
				}
			}
			if len(sums) > l.MaxCategories {
				return ErrUnsuitable
			}
		}
	}
	seriesCount := len(breakdowns)
	if b.Series == "" {
		seriesCount = 1
	}
	if len(values) > 0 && seriesCount > l.MaxSeries/len(values) {
		return ErrUnsuitable
	}
	if (m.Kind == Line || m.Kind == Area) && len(categories)*seriesCount*len(values) > MaxTransformedPoints {
		return ErrLimit
	}
	for _, sum := range sums {
		if f, _ := sum.Float64(); math.IsInf(f, 0) {
			return ErrUnsuitable
		}
	}
	return ctx.Err()
}
