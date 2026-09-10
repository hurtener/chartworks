package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/chartdata"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
)

func (s *Runs) buildRetainedChart(ctx context.Context, m RunManifest, result exec.Result, mapping charts.Mapping) (charts.Output, error) {
	values := s.blocks.limits
	values.PreviewRows, values.PreviewBytes = m.Limits.MaxRows, m.Limits.MaxResultBytes
	limits := chartLimits(values)
	normalized, err := chartdata.FromReadResult(ctx, result, limits)
	if err != nil { return charts.Output{}, err }
	positions := make(map[string]int, len(result.Schema))
	for i, field := range result.Schema { positions[field.Name] = i }
	data := charts.Data{Version: charts.Version, Columns: clone(mapping.Columns), Rows: make([][]charts.Cell, len(normalized.Rows)), Completeness: normalized.Completeness}
	for rowIndex, row := range normalized.Rows {
		if err = ctx.Err(); err != nil { return charts.Output{}, err }
		data.Rows[rowIndex] = make([]charts.Cell, len(data.Columns))
		for column, field := range data.Columns {
			index, found := positions[field.Name]
			if !found || index >= len(row) { return charts.Output{}, ErrStale }
			data.Rows[rowIndex][column] = row[index]
		}
	}
	return charts.Build(ctx, data, mapping, limits)
}
