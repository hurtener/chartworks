// Package rendering turns an already-authorized retained output into bounded
// static bytes. It has no source, model, network, URL, script or credential seam.
package rendering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"
	"unicode"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// Version pins deterministic retained rendition semantics.
const Version = "static-retained-v1"

// ErrInvalid rejects open, malformed or unsupported render requests.
var ErrInvalid = errors.New("rendering: invalid request")

// Viewer reads one artifact through the normal reporting authority boundary.
type Viewer interface {
	View(context.Context, identity.Envelope, reporting.DeliveryViewRequest) (reporting.DeliveryViewResult, error)
}

// Request selects one retained output and one closed static representation.
type Request struct {
	View   reporting.DeliveryViewRequest `json:"view"`
	Format string                        `json:"format" jsonschema:"enum=json,enum=csv,enum=html,enum=svg"`
	Theme  string                        `json:"theme" jsonschema:"enum=light,enum=dark"`
	Width  int                           `json:"width"`
	Height int                           `json:"height"`
}

// Rendition carries bounded static bytes and their retained-input provenance.
type Rendition struct {
	Version      string     `json:"version"`
	Format       string     `json:"format"`
	MediaType    string     `json:"media_type"`
	Theme        string     `json:"theme"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	SourceDigest string     `json:"source_digest"`
	Projection   Projection `json:"projection"`
	Digest       string     `json:"digest"`
	Bytes        int        `json:"bytes"`
	Content      string     `json:"content"`
}

// Projection identifies the exact retained window used for this rendition.
// SourceDigest still identifies the full retained output and must not be used as
// a digest of a paged table response.
type Projection struct {
	Offset       int                 `json:"offset"`
	Limit        int                 `json:"limit"`
	Total        int                 `json:"total"`
	Next         *int                `json:"next,omitempty"`
	Truncated    bool                `json:"truncated"`
	Completeness charts.Completeness `json:"completeness"`
	Warnings     []string            `json:"warnings"`
	Digest       string              `json:"digest"`
}

// Service renders retained values and owns no source, model or network client.
type Service struct {
	viewer   Viewer
	maxBytes int
}

// New constructs a bounded retained-only renderer.
func New(viewer Viewer, maxBytes int) (*Service, error) {
	if viewer == nil || maxBytes < 1024 || maxBytes > 64<<20 {
		return nil, ErrInvalid
	}
	return &Service{viewer, maxBytes}, nil
}

// Export rechecks exact run reach, reads the retained artifact and renders it.
func (s *Service) Export(ctx context.Context, e identity.Envelope, in Request) (Rendition, error) {
	if s == nil || ctx == nil || !e.Valid() {
		return Rendition{}, access.ErrUnauthenticated
	}
	if !e.Has("reporting.export") || !e.Has("reporting.read") {
		return Rendition{}, access.ErrForbidden
	}
	if !oneOf(in.Format, "json", "csv", "html", "svg") || !oneOf(in.Theme, "light", "dark") || in.Width < 320 || in.Width > 4096 || in.Height < 200 || in.Height > 4096 || in.View.Limit < 0 || in.View.Limit > 1000 {
		return Rendition{}, ErrInvalid
	}
	if err := access.Require(e, "reporting.export", access.Resource{Tenant: e.Tenant(), Kind: "run", Permission: "export", ID: in.View.Run}); err != nil {
		return Rendition{}, err
	}
	view, err := s.viewer.View(ctx, e, in.View)
	if err != nil {
		return Rendition{}, err
	}
	if view.Output == nil || view.Output.State != "succeeded" || view.Output.RetainedDigest == "" {
		return Rendition{}, reporting.ErrIncomplete
	}
	timezone := view.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return Rendition{}, ErrInvalid
	}
	projection, err := projectionFor(view, timezone)
	if err != nil {
		return Rendition{}, err
	}
	var content []byte
	media := ""
	switch in.Format {
	case "json":
		content, err = json.Marshal(struct {
			Output     *reporting.ViewerOutput `json:"output"`
			PageBounds reporting.ViewerPage    `json:"page_bounds"`
			Locale     string                  `json:"locale"`
			Timezone   string                  `json:"timezone"`
		}{view.Output, view.PageBounds, view.Locale, timezone})
		media = "application/json"
	case "csv":
		content, err = renderCSV(view.Output, timezone)
		media = "text/csv; charset=utf-8"
	case "html":
		content, err = renderHTML(view.Output, view.PageBounds, in.Theme, in.Width, in.Height, timezone)
		media = "text/html; charset=utf-8"
	case "svg":
		content, err = renderSVG(view.Output, in.Theme, in.Width, in.Height, timezone)
		media = "image/svg+xml"
	}
	if err != nil {
		return Rendition{}, err
	}
	if len(content) > s.maxBytes {
		return Rendition{}, reporting.ErrBudget
	}
	sum := sha256.Sum256(content)
	return Rendition{Version: Version, Format: in.Format, MediaType: media, Theme: in.Theme, Width: in.Width, Height: in.Height, SourceDigest: view.Output.RetainedDigest, Projection: projection, Digest: hex.EncodeToString(sum[:]), Bytes: len(content), Content: string(content)}, ctx.Err()
}

func projectionFor(view reporting.DeliveryViewResult, timezone string) (Projection, error) {
	p := Projection{}
	if view.Output.Table != nil {
		b := view.PageBounds
		end := b.Offset + len(view.Output.Table.Rows)
		if b.Offset < 0 || b.Limit < 1 || b.Offset > b.Total || len(view.Output.Table.Rows) > b.Limit || b.Total < end || b.Next != nil && (*b.Next != end || *b.Next >= b.Total) || b.Next == nil && end < b.Total {
			return Projection{}, ErrInvalid
		}
		p.Offset, p.Limit, p.Total, p.Next = b.Offset, b.Limit, b.Total, b.Next
		p.Truncated = b.Offset > 0 || end < b.Total
		p.Completeness = view.Output.Table.Completeness
		p.Warnings = append([]string(nil), view.Output.Table.Warnings...)
	}
	wire, err := json.Marshal(struct {
		Output     *reporting.ViewerOutput `json:"output"`
		PageBounds reporting.ViewerPage    `json:"page_bounds"`
		Locale     string                  `json:"locale"`
		Timezone   string                  `json:"timezone"`
	}{view.Output, view.PageBounds, view.Locale, timezone})
	if err != nil {
		return Projection{}, err
	}
	sum := sha256.Sum256(wire)
	p.Digest = hex.EncodeToString(sum[:])
	return p, nil
}

func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func label(c charts.Column) string {
	if c.DisplayLabel != "" {
		return c.DisplayLabel
	}
	return c.Name
}
func exportCell(v string) string {
	trim := strings.TrimLeftFunc(v, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
	})
	if trim != "" && strings.ContainsRune("=+-@", rune(trim[0])) {
		return "'" + v
	}
	return v
}

func renderCSV(out *reporting.ViewerOutput, timezone string) ([]byte, error) {
	if out.Table == nil {
		return nil, ErrInvalid
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	head := make([]string, len(out.Table.Columns))
	for i, c := range out.Table.Columns {
		head[i] = exportCell(label(c))
	}
	if err := w.Write(head); err != nil {
		return nil, err
	}
	for _, row := range out.Table.Rows {
		if len(row) != len(head) {
			return nil, ErrInvalid
		}
		values := make([]string, len(row))
		for i, c := range row {
			values[i] = exportCell(formatCell(c, out.Table.Columns[i], timezone))
		}
		if err := w.Write(values); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func renderHTML(out *reporting.ViewerOutput, page reporting.ViewerPage, theme string, width, height int, timezone string) ([]byte, error) {
	var b strings.Builder
	background, foreground := "#ffffff", "#17211f"
	if theme == "dark" {
		background, foreground = "#17211f", "#f6f1e7"
	}
	fmt.Fprintf(&b, "<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"color-scheme\" content=\"%s\"><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; style-src 'unsafe-inline'\"><style>html,body{margin:0;background:%s;color:%s}main{box-sizing:border-box;width:min(100%%,%dpx);min-height:%dpx;padding:16px}table{border-collapse:collapse}th,td{padding:4px 8px}</style><title>Retained output</title></head><body><main>", theme, background, foreground, width, height)
	switch {
	case out.Table != nil:
		b.WriteString("<table><thead><tr>")
		for _, c := range out.Table.Columns {
			b.WriteString("<th>")
			b.WriteString(html.EscapeString(label(c)))
			b.WriteString("</th>")
		}
		b.WriteString("</tr></thead><tbody>")
		for _, row := range out.Table.Rows {
			if len(row) != len(out.Table.Columns) {
				return nil, ErrInvalid
			}
			b.WriteString("<tr>")
			for i, c := range row {
				b.WriteString("<td>")
				b.WriteString(html.EscapeString(formatCell(c, out.Table.Columns[i], timezone)))
				b.WriteString("</td>")
			}
			b.WriteString("</tr>")
		}
		b.WriteString("</tbody></table>")
		for _, total := range out.Table.Totals {
			column := chartColumnByID(out.Table.Columns, total.Column)
			if column.ID != "" {
				fmt.Fprintf(&b, "<p data-total=\"%s\">%s: %s <span data-scope=\"%s\">%s</span></p>", html.EscapeString(total.Column), html.EscapeString(label(column)), html.EscapeString(formatCell(total.Value, column, timezone)), html.EscapeString(total.Scope), html.EscapeString(total.Scope))
			}
		}
		end := page.Offset + len(out.Table.Rows)
		fmt.Fprintf(&b, "<p data-page=\"true\">rows %d-%d / %d</p><p data-completeness=\"%s\">%s %s</p>", page.Offset, end, page.Total, html.EscapeString(out.Table.Completeness.Status), html.EscapeString(out.Table.Completeness.Status), html.EscapeString(out.Table.Completeness.Reason))
		for _, warning := range out.Table.Warnings {
			fmt.Fprintf(&b, "<p data-warning=\"true\">%s</p>", html.EscapeString(warning))
		}
	case out.Chart != nil && out.Chart.Kind == charts.KPI:
		renderKPIHTML(&b, out.Chart, timezone)
	case out.Chart != nil:
		svg, err := renderChartSVG(out.Chart, theme, width, height, timezone)
		if err != nil {
			return nil, err
		}
		b.WriteString(svg)
	case out.Narrative != nil:
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(out.Narrative.Text))
		b.WriteString("</p>")
	default:
		return nil, ErrInvalid
	}
	b.WriteString("</main></body></html>")
	return []byte(b.String()), nil
}

func renderKPIHTML(b *strings.Builder, c *charts.Output, timezone string) {
	if c.KPIResult == nil {
		return
	}
	valueColumn := chartColumn(c, c.Mapping.Bindings.Value)
	targetColumn := chartColumn(c, c.Mapping.Bindings.Target)
	b.WriteString("<section aria-label=\"KPI\"><strong>")
	b.WriteString(html.EscapeString(formatValue(c.KPIResult.Value, valueColumn, timezone)))
	b.WriteString("</strong>")
	for _, v := range []struct {
		name   string
		value  *charts.Value
		column charts.Column
	}{{"comparison", c.KPIResult.Comparison, valueColumn}, {"delta", c.KPIResult.Delta, valueColumn}, {"percent_delta", c.KPIResult.PercentDelta, charts.Column{Type: "decimal", Format: charts.Format{Percent: "whole"}}}, {"target", c.KPIResult.Target, targetColumn}, {"target_difference", c.KPIResult.TargetDifference, valueColumn}} {
		if v.value != nil {
			b.WriteString("<p data-kind=\"")
			b.WriteString(v.name)
			b.WriteString("\">")
			b.WriteString(html.EscapeString(formatValue(*v.value, v.column, timezone)))
			b.WriteString("</p>")
		}
	}
	if c.KPIResult.ThresholdState != "" {
		b.WriteString("<p data-threshold=\"")
		b.WriteString(html.EscapeString(c.KPIResult.ThresholdState))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(strings.TrimSpace(c.KPIResult.ThresholdLabel + " " + c.KPIResult.ThresholdState)))
		b.WriteString("</p>")
	}
	if len(c.KPIResult.Sparkline) > 0 {
		values := make([]string, len(c.KPIResult.Sparkline))
		for i, value := range c.KPIResult.Sparkline {
			values[i] = formatValue(value, valueColumn, timezone)
		}
		fmt.Fprintf(b, "<p data-kind=\"sparkline\">%s</p>", html.EscapeString(strings.Join(values, " → ")))
	}
	b.WriteString("</section>")
}

func chartColumnByID(columns []charts.Column, id string) charts.Column {
	for _, column := range columns {
		if column.ID == id {
			return column
		}
	}
	return charts.Column{}
}

func chartColumn(c *charts.Output, id string) charts.Column {
	for _, column := range c.Columns {
		if column.ID == id {
			return column
		}
	}
	return charts.Column{}
}

func formatValue(value charts.Value, column charts.Column, timezone string) string {
	return formatCell(charts.Cell{Null: value.Null, Value: value.Exact}, column, timezone)
}

func renderSVG(out *reporting.ViewerOutput, theme string, width, height int, timezone string) ([]byte, error) {
	if out.Chart == nil {
		return nil, ErrInvalid
	}
	body, err := renderChartSVG(out.Chart, theme, width, height, timezone)
	if err != nil {
		return nil, err
	}
	return []byte(body), nil
}
func renderChartSVG(c *charts.Output, theme string, width, height int, timezone string) (string, error) {
	if c == nil || c.Mapping.Kind != c.Kind || c.Version != c.Mapping.Version {
		return "", ErrInvalid
	}
	var b strings.Builder
	background, foreground := "#ffffff", "#17211f"
	if theme == "dark" {
		background, foreground = "#17211f", "#f6f1e7"
	}
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" role=\"img\" data-theme=\"%s\" viewBox=\"0 0 %d %d\"><title>%s</title><rect width=\"%d\" height=\"%d\" fill=\"%s\"/><g fill=\"%s\">", theme, width, height, html.EscapeString(c.Mapping.Options.Title), width, height, background, foreground)
	y := 28
	if c.Kind == charts.KPI && c.KPIResult != nil {
		for _, line := range kpiLines(c, timezone) {
			if y > height-8 {
				break
			}
			fmt.Fprintf(&b, "<text x=\"16\" y=\"%d\" data-kind=\"%s\">%s</text>", y, html.EscapeString(line.kind), html.EscapeString(line.text))
			y += 18
		}
	} else {
		for _, p := range c.Points {
			if y > height-8 {
				break
			}
			parts := []string{}
			if !p.Category.Null {
				parts = append(parts, formatCell(p.Category, chartColumn(c, c.Mapping.Bindings.Category), timezone))
			}
			for _, item := range []struct {
				value   charts.Value
				id      string
				measure bool
			}{{p.X, c.Mapping.Bindings.X, false}, {p.Y, c.Mapping.Bindings.Y, false}, {p.Value, c.Mapping.Bindings.Value, true}} {
				if !item.value.Null && item.value.Exact != "" {
					column := chartColumn(c, item.id)
					if p.Measure != "" && item.measure {
						column = chartColumn(c, p.Measure)
					}
					parts = append(parts, formatValue(item.value, column, timezone))
				}
			}
			fmt.Fprintf(&b, "<text x=\"16\" y=\"%d\">%s</text>", y, html.EscapeString(strings.Join(parts, " · ")))
			y += 18
		}
	}
	b.WriteString("</g></svg>")
	return b.String(), nil
}

type kpiLine struct{ kind, text string }

func kpiLines(c *charts.Output, timezone string) []kpiLine {
	k := c.KPIResult
	valueColumn, targetColumn := chartColumn(c, c.Mapping.Bindings.Value), chartColumn(c, c.Mapping.Bindings.Target)
	out := []kpiLine{{"value", formatValue(k.Value, valueColumn, timezone)}}
	for _, item := range []struct {
		kind   string
		value  *charts.Value
		column charts.Column
	}{{"comparison", k.Comparison, valueColumn}, {"delta", k.Delta, valueColumn}, {"percent_delta", k.PercentDelta, charts.Column{Type: "decimal", Format: charts.Format{Percent: "whole"}}}, {"target", k.Target, targetColumn}, {"target_difference", k.TargetDifference, valueColumn}} {
		if item.value != nil {
			out = append(out, kpiLine{item.kind, formatValue(*item.value, item.column, timezone)})
		}
	}
	if k.ThresholdState != "" {
		out = append(out, kpiLine{"threshold", strings.TrimSpace(k.ThresholdLabel + " " + k.ThresholdState)})
	}
	if len(k.Sparkline) > 0 {
		values := make([]string, len(k.Sparkline))
		for i, v := range k.Sparkline {
			values[i] = formatValue(v, valueColumn, timezone)
		}
		out = append(out, kpiLine{"sparkline", strings.Join(values, " → ")})
	}
	return out
}
