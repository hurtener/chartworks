package rendering

import (
	"fmt"
	"html"
	"math"
	"strings"

	"github.com/hurtener/chartworks/internal/charts"
)

// drawingScene is a closed set of internally constructed primitives. It accepts
// neither markup nor external resources and is shared by SVG and raster output.
type drawingScene struct {
	nodes                 []drawingPrimitive
	invalid               bool
	pointCount, textBytes int
}
type drawingPoint struct{ x, y float64 }
type drawingPrimitive struct {
	kind, class, layout, fill, stroke, title, category, series, text string
	x, y, w, h, r, start, end, opacity                               float64
	points                                                           []drawingPoint
	large                                                            int
}

func (s *drawingScene) add(p drawingPrimitive) {
	if s.invalid {
		return
	}
	s.pointCount += len(p.points)
	s.textBytes += len(p.text) + len(p.title) + len(p.category) + len(p.series)
	if len(s.nodes) >= 16384 || s.pointCount > 65536 || s.textBytes > 1<<20 {
		s.invalid = true
		return
	}
	s.nodes = append(s.nodes, p)
}
func (s *drawingScene) valid() bool {
	if s.invalid || len(s.nodes) > 16384 {
		return false
	}
	points, chars := 0, 0
	for _, p := range s.nodes {
		points += len(p.points)
		chars += len(p.text) + len(p.title) + len(p.category) + len(p.series)
		if points > 65536 || chars > 1<<20 {
			return false
		}
		for _, v := range []float64{p.x, p.y, p.w, p.h, p.r, p.start, p.end, p.opacity} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return false
			}
		}
		for _, v := range []float64{p.x, p.y, p.w, p.h, p.r} {
			if math.Abs(v) > 1e7 {
				return false
			}
		}
		if p.kind == "slice" && len(p.points) != 2 {
			return false
		}
		for _, v := range p.points {
			if math.IsNaN(v.x) || math.IsNaN(v.y) || math.IsInf(v.x, 0) || math.IsInf(v.y, 0) || math.Abs(v.x) > 1e7 || math.Abs(v.y) > 1e7 {
				return false
			}
		}
		if !oneOf(p.kind, "axis", "rect", "circle", "polygon", "polyline", "slice", "text", "labels_start", "group_end") {
			return false
		}
	}
	return true
}
func drawingPoints(points []drawingPoint) string {
	var b strings.Builder
	for i, p := range points {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%.1f,%.1f", p.x, p.y)
	}
	return b.String()
}
func (s *drawingScene) svg(b *strings.Builder) {
	for _, p := range s.nodes {
		switch p.kind {
		case "axis":
			fmt.Fprintf(b, "<g class=\"plot\" stroke=\"%s\" fill=\"none\"><path d=\"M%.1f %.1fV%.1fH%.1f\"/></g>", p.stroke, p.x, p.y, p.h, p.w)
		case "rect":
			fmt.Fprintf(b, "<rect class=\"%s\"", p.class)
			switch p.layout {
			case "grouped":
				fmt.Fprintf(b, " data-layout=\"grouped\" data-category=\"%s\" data-series=\"%s\"", html.EscapeString(p.category), html.EscapeString(p.series))
			case "stacked":
				fmt.Fprintf(b, " data-layout=\"stacked\" data-category=\"%s\" data-start=\"%.6g\" data-end=\"%.6g\"", html.EscapeString(p.category), p.start, p.end)
			case "heatmap":
				fmt.Fprintf(b, " data-x=\"%s\" data-y=\"%s\"", html.EscapeString(p.category), html.EscapeString(p.series))
			}
			fmt.Fprintf(b, " x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"", p.x, p.y, p.w, p.h, p.fill)
			if p.layout == "heatmap" {
				fmt.Fprintf(b, " fill-opacity=\"%.3f\"", p.opacity)
			}
			fmt.Fprintf(b, "><title>%s</title></rect>", html.EscapeString(p.title))
		case "circle":
			fmt.Fprintf(b, "<circle class=\"%s\" cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\" fill=\"%s\"", p.class, p.x, p.y, p.r, p.fill)
			if p.class == "donut-hole" {
				b.WriteString("/>")
			} else {
				fmt.Fprintf(b, "><title>%s</title></circle>", html.EscapeString(p.title))
			}
		case "polygon":
			fmt.Fprintf(b, "<polygon class=\"area\" points=\"%s\" fill=\"%s\" fill-opacity=\"0.28\"/>", drawingPoints(p.points), p.fill)
		case "polyline":
			fmt.Fprintf(b, "<polyline class=\"%s\" points=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"2\"", p.class, drawingPoints(p.points), p.stroke)
			if p.class == "sparkline" {
				b.WriteString("/>")
			} else {
				fmt.Fprintf(b, "><title>%s</title></polyline>", html.EscapeString(p.title))
			}
		case "slice":
			fmt.Fprintf(b, "<path class=\"slice\" d=\"M%.1f %.1fL%.1f %.1fA%.1f %.1f 0 %d 1 %.1f %.1fZ\" fill=\"%s\"><title>%s</title></path>", p.x, p.y, p.points[0].x, p.points[0].y, p.r, p.r, p.large, p.points[1].x, p.points[1].y, p.fill, html.EscapeString(p.title))
		case "labels_start":
			b.WriteString("<g class=\"exact-labels\" fill=\"" + p.fill + "\">")
		case "group_end":
			b.WriteString("</g>")
		case "text":
			fmt.Fprintf(b, "<text x=\"%.0f\" y=\"%.0f\">%s</text>", p.x, p.y, html.EscapeString(p.text))
		}
	}
}

func drawChartGeometry(b *strings.Builder, c *charts.Output, width, height int, foreground, background, timezone string) error {
	if c == nil || len(c.Points) > 65536 || len(c.Hierarchy) > 16384 {
		return ErrInvalid
	}
	s := &drawingScene{}
	if err := appendChartGeometry(s, c, width, height, foreground, background, timezone); err != nil {
		return err
	}
	if !s.valid() {
		return ErrInvalid
	}
	s.svg(b)
	return nil
}
func drawSparkline(b *strings.Builder, values []charts.Value, width, height int) {
	s := &drawingScene{}
	appendSparkline(s, values, width, height)
	if s.valid() {
		s.svg(b)
	}
}
