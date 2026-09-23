package nlqroute

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

const (
	maxSemanticNodes = 4096
	maxSemanticDepth = 64
	maxSemanticBytes = 256 << 10
)

// semanticClosure is the shared, bounded catalog expansion for selected metrics
// and retrieved semantic candidates. Candidates remain candidates: expanding a
// reference cannot select it, activate a rule, or widen its source authority.
func semanticClosure(ctx context.Context, def topics.Definition, roots []semantics.Reference) ([]nlq.MetricDependency, error) {
	if ctx == nil || len(roots) == 0 || len(roots) > maxSemanticNodes {
		return nil, ErrMetricContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	measures := map[string]semantics.Measure{}
	kpis := map[string]semantics.KPI{}
	dimensions := map[string]semantics.Dimension{}
	columns := map[string]semantics.Column{}
	dimensionsByColumn := map[string][]semantics.Dimension{}
	catalogNodes := len(def.Datasets) + len(def.Measures) + len(def.KPIs) + len(def.Dimensions) + len(def.Joins)
	for _, dataset := range def.Datasets {
		catalogNodes += len(dataset.Columns)
	}
	if catalogNodes > maxSemanticNodes {
		return nil, nlq.ErrInsufficient
	}
	for _, dataset := range def.Datasets {
		for _, column := range dataset.Columns {
			key := dataset.ID + "\x00" + column.ID
			if _, duplicate := columns[key]; duplicate {
				return nil, ErrMetricContext
			}
			columns[key] = column
		}
	}
	for _, value := range def.Measures {
		if _, duplicate := measures[value.ID]; duplicate {
			return nil, ErrMetricContext
		}
		measures[value.ID] = value
	}
	for _, value := range def.Dimensions {
		if _, duplicate := dimensions[value.ID]; duplicate {
			return nil, ErrMetricContext
		}
		dimensions[value.ID] = value
		key := value.Field.Dataset + "\x00" + value.Field.ID
		dimensionsByColumn[key] = append(dimensionsByColumn[key], value)
	}
	for _, value := range def.KPIs {
		if _, duplicate := kpis[value.ID]; duplicate {
			return nil, ErrMetricContext
		}
		kpis[value.ID] = value
	}
	seen, traversed, activeKPI := map[string]bool{}, map[semantics.Reference]bool{}, map[string]bool{}
	datasets := map[string]bool{}
	out := []nlq.MetricDependency{}
	bytes := 0
	add := func(kind, id string, value any) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := kind + "\x00" + id
		if seen[key] {
			return nil
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return ErrMetricContext
		}
		bytes += len(raw)
		if len(raw) > 16<<10 || bytes > maxSemanticBytes || len(out) >= maxSemanticNodes {
			return nlq.ErrInsufficient
		}
		seen[key] = true
		out = append(out, nlq.MetricDependency{Kind: kind, ID: id, Text: string(raw)})
		return nil
	}
	addColumn := func(ref semantics.Reference) error {
		value, ok := columns[ref.Dataset+"\x00"+ref.ID]
		if !ok || ref.Kind != semantics.KindColumn || !ref.Valid() {
			return ErrMetricContext
		}
		return add("column", ref.Dataset+":"+ref.ID, struct {
			Dataset string           `json:"dataset"`
			Column  semantics.Column `json:"column"`
		}{ref.Dataset, value})
	}
	var visit func(semantics.Reference, int) error
	visitFilters := func(filters []semantics.SemanticFilter, depth int) error {
		for _, filter := range filters {
			if err := visit(filter.Field, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	visit = func(ref semantics.Reference, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !ref.Valid() || depth > maxSemanticDepth {
			return ErrMetricContext
		}
		// Column/dimension associations may point back to their own field. KPI
		// expression cycles, unlike those metadata associations, are invalid.
		if ref.Kind == semantics.KindKPI && activeKPI[ref.ID] {
			return ErrMetricContext
		}
		if traversed[ref] {
			return nil
		}
		traversed[ref] = true
		switch ref.Kind {
		case semantics.KindKPI:
			value, ok := kpis[ref.ID]
			if !ok || len(value.Inputs) == 0 {
				return ErrMetricContext
			}
			activeKPI[ref.ID] = true
			defer delete(activeKPI, ref.ID)
			if err := add("kpi", value.ID, value); err != nil {
				return err
			}
			for _, input := range value.Inputs {
				if err := visit(input, depth+1); err != nil {
					return err
				}
			}
			return visitFilters(value.Filters, depth)
		case semantics.KindMeasure:
			value, ok := measures[ref.ID]
			if !ok {
				return ErrMetricContext
			}
			if err := add("measure", value.ID, value); err != nil {
				return err
			}
			if err := visit(value.Field, depth+1); err != nil {
				return err
			}
			return visitFilters(value.Filters, depth)
		case semantics.KindDimension:
			value, ok := dimensions[ref.ID]
			if !ok {
				return ErrMetricContext
			}
			if err := add("dimension", value.ID, value); err != nil {
				return err
			}
			if err := visit(value.Field, depth+1); err != nil {
				return err
			}
			return visitFilters(value.Filters, depth)
		case semantics.KindColumn:
			if err := addColumn(ref); err != nil {
				return err
			}
			datasets[ref.Dataset] = true
			for _, dimension := range dimensionsByColumn[ref.Dataset+"\x00"+ref.ID] {
				if err := visit(semantics.Reference{Kind: semantics.KindDimension, ID: dimension.ID}, depth+1); err != nil {
					return err
				}
			}
			return nil
		default:
			return ErrMetricContext
		}
	}
	for _, root := range roots {
		if err := visit(root, 0); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	connecting, err := uniqueJoinSubgraph(def.Joins, datasets)
	if err != nil {
		return nil, err
	}
	for _, join := range connecting {
		if err := add("join", join.ID, join); err != nil {
			return nil, err
		}
		for _, ref := range []semantics.Reference{join.Left, join.Right} {
			if err := addColumn(ref); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
