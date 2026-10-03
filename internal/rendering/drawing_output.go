package rendering

import (
	"context"
	"fmt"
	"strings"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
)

// outputDrawing builds only a single retained projection. The caller supplies
// already-validated disclosure lines; raster output cannot omit them to fit.
func outputDrawing(ctx context.Context, out *reporting.ViewerOutput, theme string, width, height int, timezone string, disclosure []string) (*drawingScene, error) {
	if ctx == nil || out == nil || !oneOf(theme, "light", "dark") || width < 320 || width > 4096 || height < 200 || height > 4096 || len(disclosure) > 128 {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	background, foreground := "#ffffff", "#17211f"
	if theme == "dark" {
		background, foreground = "#17211f", "#f6f1e7"
	}
	meaning, err := rasterMeaning(out)
	if err != nil {
		return nil, err
	}
	disclosure = append(append([]string{}, disclosure...), meaning...)
	disclosure = wrapDisclosureLines(disclosure, width)
	if len(disclosure) > 128 {
		return nil, ErrInvalid
	}
	contentHeight := height - len(disclosure)*18
	if contentHeight < 100 {
		return nil, ErrInvalid
	}
	s := &drawingScene{}
	s.add(drawingPrimitive{kind: "rect", class: "background", x: 0, y: 0, w: float64(width), h: float64(height), fill: background})
	switch {
	case out.Table != nil:
		table := out.Table
		if len(table.Columns) == 0 || len(table.Columns) > 100 || len(table.Rows) > 1000 {
			return nil, ErrInvalid
		}
		// A raster has no scrolling. Reject an unrepresentable requested page rather
		// than crop rows while claiming the full retained projection was rendered.
		if 30+len(table.Rows)*22+len(table.Totals)*20 > contentHeight {
			return nil, ErrInvalid
		}
		cellWidth := float64(width) / float64(len(table.Columns))
		for i, c := range table.Columns {
			s.add(drawingPrimitive{kind: "text", class: "header", x: 8 + float64(i)*cellWidth, y: 22, w: cellWidth - 12, fill: foreground, text: label(c)})
		}
		for ri, row := range table.Rows {
			if len(row) != len(table.Columns) {
				return nil, ErrInvalid
			}
			for ci, cell := range row {
				s.add(drawingPrimitive{kind: "text", x: 8 + float64(ci)*cellWidth, y: float64(46 + ri*22), w: cellWidth - 12, fill: foreground, text: rasterCell(cell, table.Columns[ci], timezone)})
			}
		}
		y := float64(46 + len(table.Rows)*22)
		for _, total := range table.Totals {
			c := chartColumnByID(table.Columns, total.Column)
			if c.ID == "" {
				return nil, ErrInvalid
			}
			s.add(drawingPrimitive{kind: "text", x: 8, y: y, fill: foreground, text: fmt.Sprintf("%s: %s (%s)", label(c), formatCell(total.Value, c, timezone), total.Scope)})
			y += 20
		}
	case out.Chart != nil:
		c := out.Chart
		if c.Mapping.Kind != c.Kind || c.Version != c.Mapping.Version || len(c.Points) > 65536 || len(c.Hierarchy) > 16384 {
			return nil, ErrInvalid
		}
		if c.Kind == charts.KPI && c.KPIResult != nil {
			lines := rasterKPILines(c, timezone)
			if c.Mapping.Options.Title != "" {
				lines = append([]kpiLine{{"title", c.Mapping.Options.Title}}, lines...)
			}
			if 28+len(lines)*18 > contentHeight-60 {
				return nil, ErrInvalid
			}
			for i, line := range lines {
				s.add(drawingPrimitive{kind: "text", x: 16, y: float64(28 + i*18), fill: foreground, text: rasterKPILabel(line)})
			}
			appendSparkline(s, c.KPIResult.Sparkline, width, contentHeight)
		} else if err := appendRasterChart(s, c, width, contentHeight, foreground, background, timezone); err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalid
	}
	for i, line := range disclosure {
		s.add(drawingPrimitive{kind: "text", x: 8, y: float64(contentHeight + 12 + i*18), fill: foreground, text: line})
	}
	if !s.valid() {
		return nil, ErrInvalid
	}
	return s, nil
}

// Raster legends are visible text rather than SVG hover titles. Reserve their
// space explicitly so exact values never overlap the plot or disappear.
func appendRasterChart(s *drawingScene, c *charts.Output, width, height int, foreground, background, timezone string) error {
	if len(c.Points) > 128 {
		return ErrInvalid
	}
	labels := make([]string, 0, len(c.Points)+1)
	if c.Mapping.Options.Title != "" {
		labels = append(labels, c.Mapping.Options.Title)
	}
	for _, p := range c.Points {
		if text := pointLabel(c, p, timezone); text != "" {
			labels = append(labels, text)
		}
	}
	legendHeight := len(labels)*18 + 12
	if len(labels) > 129 || height-legendHeight < 120 {
		return ErrInvalid
	}
	plot := &drawingScene{}
	if err := appendChartGeometry(plot, c, width, height-legendHeight, foreground, background, timezone); err != nil {
		return err
	}
	if !plot.valid() {
		return ErrInvalid
	}
	for _, p := range plot.nodes {
		if p.kind == "labels_start" {
			break
		}
		p.y += float64(legendHeight)
		if p.kind == "axis" {
			p.h += float64(legendHeight)
		}
		for i := range p.points {
			p.points[i].y += float64(legendHeight)
		}
		s.add(p)
	}
	for i, text := range labels {
		s.add(drawingPrimitive{kind: "text", x: 8, y: float64(22 + i*18), fill: foreground, text: text})
	}
	return nil
}

func rasterKPILabel(line kpiLine) string {
	if line.kind == "title" {
		return line.text
	}
	labels := map[string]string{"value": "Value", "comparison": "Comparison", "delta": "Change", "percent_delta": "Percent change", "target": "Target", "target_difference": "Difference from target", "threshold": "Threshold", "sparkline": "Trend"}
	return labels[line.kind] + ": " + line.text
}

// Pixel exports have no SVG attributes or hover titles. Keep result scope and
// non-ready states visible, separately from amount-completeness evidence.
func rasterMeaning(out *reporting.ViewerOutput) ([]string, error) {
	var completeness charts.Completeness
	var warnings []string
	var lines []string
	if out.Table != nil {
		completeness = out.Table.Completeness
		warnings = out.Table.Warnings
		if len(out.Table.Rows) == 0 {
			lines = append(lines, "No rows in this page")
		}
	} else if out.Chart != nil {
		c := out.Chart
		completeness = c.Completeness
		warnings = c.Warnings
		switch c.State {
		case "ready", "":
		case "empty":
			lines = append(lines, "No rows in this result")
		case "no_values":
			lines = append(lines, "No values to plot")
		case "no_positive_values":
			lines = append(lines, "No positive values to plot")
		default:
			return nil, ErrInvalid
		}
		if c.OmittedRows < 0 || c.OmittedRows > 100000 {
			return nil, ErrInvalid
		}
		if c.OmittedRows > 0 {
			lines = append(lines, fmt.Sprintf("Rows omitted from chart: %d", c.OmittedRows))
		}
	}
	switch completeness.Status {
	case "complete_result":
		if completeness.Reason != "" {
			return nil, ErrInvalid
		}
		lines = append(lines, "All query result rows retained")
	case "truncated":
		if !oneOf(completeness.Reason, "rows", "bytes", "source_limit", "unknown") {
			return nil, ErrInvalid
		}
		lines = append(lines, "Truncated query result: "+strings.ReplaceAll(completeness.Reason, "_", " "))
	case "":
		lines = append(lines, "Query row coverage unknown")
	default:
		return nil, ErrInvalid
	}
	if len(warnings) > 128 || len(completeness.Reason) > 4096 {
		return nil, ErrInvalid
	}
	for _, warning := range warnings {
		if len(warning) > 4096 {
			return nil, ErrInvalid
		}
		lines = append(lines, "Warning: "+strings.ReplaceAll(warning, "_", " "))
	}
	return lines, nil
}

func rasterPageScope(p Projection, rows int) string {
	if rows == 0 {
		return fmt.Sprintf("Displayed rows: 0 of %d retained", p.Total)
	}
	return fmt.Sprintf("Displayed rows: %d-%d of %d retained", p.Offset+1, p.Offset+rows, p.Total)
}

func rasterKPILines(c *charts.Output, timezone string) []kpiLine {
	lines := kpiLines(c, timezone)
	present := map[string]bool{}
	for i := range lines {
		present[lines[i].kind] = true
		if lines[i].text == "" {
			lines[i].text = "NULL"
		}
	}
	if c.Mapping.KPI != nil {
		for _, item := range []struct {
			kind     string
			required bool
		}{{"comparison", c.Mapping.KPI.ComparisonMode != "" && c.Mapping.KPI.ComparisonMode != "none"}, {"delta", c.Mapping.KPI.ShowDelta}, {"percent_delta", c.Mapping.KPI.ShowPercentDelta}, {"target_difference", c.Mapping.KPI.ShowTargetDifference}} {
			if item.required && !present[item.kind] {
				lines = append(lines, kpiLine{item.kind, "unavailable"})
			}
		}
	}
	return lines
}

func rasterCell(cell charts.Cell, column charts.Column, timezone string) string {
	if cell.Null {
		return "NULL"
	}
	return formatCell(cell, column, timezone)
}
