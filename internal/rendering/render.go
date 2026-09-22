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
	Version      string `json:"version"`
	Format       string `json:"format"`
	MediaType    string `json:"media_type"`
	Theme        string `json:"theme"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	SourceDigest string `json:"source_digest"`
	Digest       string `json:"digest"`
	Bytes        int    `json:"bytes"`
	Content      string `json:"content"`
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
	var content []byte
	media := ""
	switch in.Format {
	case "json":
		content, err = json.Marshal(view.Output)
		media = "application/json"
	case "csv":
		content, err = renderCSV(view.Output)
		media = "text/csv; charset=utf-8"
	case "html":
		content, err = renderHTML(view.Output, in.Theme)
		media = "text/html; charset=utf-8"
	case "svg":
		content, err = renderSVG(view.Output, in.Theme, in.Width, in.Height)
		media = "image/svg+xml"
	}
	if err != nil {
		return Rendition{}, err
	}
	if len(content) > s.maxBytes {
		return Rendition{}, reporting.ErrBudget
	}
	sum := sha256.Sum256(content)
	return Rendition{Version: Version, Format: in.Format, MediaType: media, Theme: in.Theme, Width: in.Width, Height: in.Height, SourceDigest: view.Output.RetainedDigest, Digest: hex.EncodeToString(sum[:]), Bytes: len(content), Content: string(content)}, ctx.Err()
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
	trim := strings.TrimLeft(v, " \t\r\n")
	if trim != "" && strings.ContainsRune("=+-@", rune(trim[0])) {
		return "'" + v
	}
	return v
}

func renderCSV(out *reporting.ViewerOutput) ([]byte, error) {
	if out.Table == nil {
		return nil, ErrInvalid
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	head := make([]string, len(out.Table.Columns))
	for i, c := range out.Table.Columns {
		head[i] = label(c)
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
			values[i] = exportCell(formatCell(c, out.Table.Columns[i]))
		}
		if err := w.Write(values); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func renderHTML(out *reporting.ViewerOutput, theme string) ([]byte, error) {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"color-scheme\" content=\"")
	b.WriteString(theme)
	b.WriteString("\"><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'\"><title>Retained output</title></head><body>")
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
			b.WriteString("<tr>")
			for i, c := range row {
				b.WriteString("<td>")
				if i < len(out.Table.Columns) {
					b.WriteString(html.EscapeString(formatCell(c, out.Table.Columns[i])))
				}
				b.WriteString("</td>")
			}
			b.WriteString("</tr>")
		}
		b.WriteString("</tbody></table>")
	case out.Chart != nil && out.Chart.Kind == charts.KPI:
		renderKPIHTML(&b, out.Chart)
	case out.Chart != nil:
		svg, err := renderChartSVG(out.Chart, 800, 420)
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
	b.WriteString("</body></html>")
	return []byte(b.String()), nil
}

func renderKPIHTML(b *strings.Builder, c *charts.Output) {
	if c.KPIResult == nil {
		return
	}
	valueColumn := chartColumn(c, c.Mapping.Bindings.Value)
	targetColumn := chartColumn(c, c.Mapping.Bindings.Target)
	b.WriteString("<section aria-label=\"KPI\"><strong>")
	b.WriteString(html.EscapeString(formatValue(c.KPIResult.Value, valueColumn)))
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
			b.WriteString(html.EscapeString(formatValue(*v.value, v.column)))
			b.WriteString("</p>")
		}
	}
	if c.KPIResult.ThresholdState != "" {
		b.WriteString("<p data-threshold=\"")
		b.WriteString(html.EscapeString(c.KPIResult.ThresholdState))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(c.KPIResult.ThresholdLabel))
		b.WriteString("</p>")
	}
	b.WriteString("</section>")
}

func chartColumn(c *charts.Output, id string) charts.Column {
	for _, column := range c.Columns {
		if column.ID == id {
			return column
		}
	}
	return charts.Column{}
}

func formatValue(value charts.Value, column charts.Column) string {
	return formatCell(charts.Cell{Null: value.Null, Value: value.Exact}, column)
}

func renderSVG(out *reporting.ViewerOutput, theme string, width, height int) ([]byte, error) {
	if out.Chart == nil {
		return nil, ErrInvalid
	}
	body, err := renderChartSVG(out.Chart, width, height)
	if err != nil {
		return nil, err
	}
	return []byte(body), nil
}
func renderChartSVG(c *charts.Output, width, height int) (string, error) {
	if c == nil || c.Mapping.Kind != c.Kind || c.Version != c.Mapping.Version {
		return "", ErrInvalid
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" role=\"img\" viewBox=\"0 0 %d %d\"><title>%s</title>", width, height, html.EscapeString(c.Mapping.Options.Title))
	y := 28
	if c.Kind == charts.KPI && c.KPIResult != nil {
		fmt.Fprintf(&b, "<text x=\"16\" y=\"%d\">%s</text>", y, html.EscapeString(formatValue(c.KPIResult.Value, chartColumn(c, c.Mapping.Bindings.Value))))
	} else {
		for _, p := range c.Points {
			if y > height-8 {
				break
			}
			parts := []string{}
			for _, v := range []charts.Value{p.X, p.Y, p.Value} {
				if !v.Null && v.Exact != "" {
					parts = append(parts, v.Exact)
				}
			}
			fmt.Fprintf(&b, "<text x=\"16\" y=\"%d\">%s</text>", y, html.EscapeString(strings.Join(parts, " · ")))
			y += 18
		}
	}
	b.WriteString("</svg>")
	return b.String(), nil
}
