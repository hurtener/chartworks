package nlqexec

import (
	"context"
	"sort"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func compileGroupingSelection(ctx context.Context, a admission, contract exec.AnalyticalContract) (*exec.AnalyticalGrain, error) {
	in := a.route.Request.Grouping
	if in == nil {
		return nil, nil
	}
	if err := nlqroute.ValidateGrouping(in); err != nil {
		return nil, err
	}
	if len(a.publications) != 1 {
		return nil, analyticalUnsupported("analytical_grain_unsupported")
	}
	def := a.publications[0].Definition
	compiler := analyticalCompiler{ctx: ctx, definition: def, binding: a.binding, dataset: contract.Dataset}
	out := &exec.AnalyticalGrain{Policy: exec.AnalyticalGroupingPolicy, Columns: []string{}, Dimensions: []string{}}
	columns, ids, buckets := map[string]bool{}, map[string]bool{}, map[string]exec.AnalyticalBucket{}
	for _, key := range in.Keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if key.Topic != def.Topic {
			return nil, exec.ErrBinding
		}
		found := false
		for _, d := range def.Dimensions {
			if d.ID != key.Dimension {
				continue
			}
			if len(d.Filters) > 0 || key.Grain == "" && d.Role == semantics.DimensionTemporal || key.Grain != "" && d.Role != semantics.DimensionTemporal {
				return nil, analyticalUnsupported("analytical_grain_unsupported")
			}
			selected := false
			for _, topic := range a.route.Selection.Topics {
				if topic.Topic == key.Topic {
					for _, root := range topic.Roots {
						selected = selected || (root.Reference.Kind == semantics.KindDimension && root.Reference.ID == d.ID && root.Reason != "required_rule")
					}
				}
			}
			if !selected {
				return nil, exec.ErrBinding
			}
			column, err := compiler.column(d.Field)
			if err != nil {
				return nil, err
			}
			ids[key.Topic+":dimension:"+d.ID] = true
			if key.Grain == "" {
				columns[column.SourceName] = true
			} else {
				if d.Temporal == nil {
					return nil, exec.ErrBinding
				}
				dim := grainDimension{id: d.ID, field: d.Field, role: d.Role, filters: d.Filters, temporal: d.Temporal}
				bucket, err := compileCalendarBucket(dim, column, string(key.Grain))
				if err != nil {
					return nil, err
				}
				buckets[exec.Hash(bucket)] = bucket
			}
			found = true
		}
		if !found {
			return nil, exec.ErrBinding
		}
	}
	for column := range columns {
		out.Columns = append(out.Columns, column)
	}
	for id := range ids {
		out.Dimensions = append(out.Dimensions, id)
	}
	for _, bucket := range buckets {
		out.Buckets = append(out.Buckets, bucket)
	}
	sort.Strings(out.Columns)
	sort.Strings(out.Dimensions)
	sort.Slice(out.Buckets, func(i, j int) bool { return exec.Hash(out.Buckets[i]) < exec.Hash(out.Buckets[j]) })
	return out, nil
}
