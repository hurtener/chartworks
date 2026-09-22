package rendering

import (
	"fmt"
	"html"
	"math"
	"strings"

	"github.com/hurtener/chartworks/internal/charts"
)

var chartPalette = []string{"#16877c", "#db6b4f", "#5d6fb6", "#d5a62e", "#7b5aa6", "#4d8b43"}

type plotBox struct{ x, y, w, h float64 }

func drawChartGeometry(b *strings.Builder, c *charts.Output, width, height int, foreground, background, timezone string) error {
	box := plotBox{48, 36, math.Max(1, float64(width-64)), math.Max(1, float64(height-76))}
	fmt.Fprintf(b, "<g class=\"plot\" stroke=\"%s\" fill=\"none\"><path d=\"M%.1f %.1fV%.1fH%.1f\"/></g>", foreground, box.x, box.y, box.y+box.h, box.x+box.w)
	switch c.Kind {
	case charts.Line, charts.Area:
		drawLines(b, c, box, c.Kind == charts.Area, timezone)
	case charts.Bar:
		drawBars(b, c, box, true, timezone)
	case charts.ColumnChart:
		drawBars(b, c, box, false, timezone)
	case charts.GroupedBar:
		drawGroupedBars(b, c, box, true, timezone)
	case charts.StackedBar:
		drawStackedBars(b, c, box, true, timezone)
	case charts.StackedColumn:
		drawStackedBars(b, c, box, false, timezone)
	case charts.Scatter:
		drawScatter(b, c, box, timezone)
	case charts.Heatmap:
		drawHeatmap(b, c, box, timezone)
	case charts.Pie, charts.Donut:
		drawPie(b, c, box, c.Kind == charts.Donut, background, timezone)
	case charts.Treemap:
		drawTreemap(b, c, box, timezone)
	case charts.KPI:
		// Version-one KPI outputs retain their value as a point. Exact labels below
		// preserve backward readability; version three uses the richer KPI branch.
	default:
		return ErrInvalid
	}
	drawExactLabels(b, c, foreground, timezone)
	return nil
}

func categoryKey(p charts.Point) string {
	if p.CategoryKey != "" {
		return p.CategoryKey
	}
	return p.Category.Value
}
func seriesKey(p charts.Point) string {
	if p.SeriesID != "" {
		return p.SeriesID
	}
	if p.Series.Value != "" {
		return p.Series.Value
	}
	return p.Measure
}
func orderedGroups(points []charts.Point) ([]string, []string, map[string][]charts.Point) {
	categories, series := []string{}, []string{}
	seenC, seenS := map[string]bool{}, map[string]bool{}
	groups := map[string][]charts.Point{}
	for _, p := range points {
		c, s := categoryKey(p), seriesKey(p)
		if !seenC[c] {
			seenC[c] = true
			categories = append(categories, c)
		}
		if !seenS[s] {
			seenS[s] = true
			series = append(series, s)
		}
		groups[c] = append(groups[c], p)
	}
	return categories, series, groups
}

func drawGroupedBars(b *strings.Builder, c *charts.Output, box plotBox, horizontal bool, timezone string) {
	lo, hi, found := valueRange(c.Points, func(p charts.Point) charts.Value { return p.Value }, true)
	if !found {
		return
	}
	categories, series, groups := orderedGroups(c.Points)
	outer := map[bool]float64{true: box.h / float64(max(1, len(categories))), false: box.w / float64(max(1, len(categories)))}[horizontal]
	inner := outer / float64(max(1, len(series)))
	zeroX := scaled(0, lo, hi, box.x, box.w)
	zeroY := box.y + box.h - scaled(0, lo, hi, 0, box.h)
	seriesIndex := map[string]int{}
	for i, s := range series {
		seriesIndex[s] = i
	}
	for ci, category := range categories {
		for _, p := range groups[category] {
			value, ok := coordinate(p.Value)
			if !ok {
				continue
			}
			si := seriesIndex[seriesKey(p)]
			color := chartPalette[si%len(chartPalette)]
			label := html.EscapeString(pointLabel(c, p, timezone))
			if horizontal {
				x := scaled(value, lo, hi, box.x, box.w)
				fmt.Fprintf(b, "<rect class=\"bar\" data-layout=\"grouped\" data-category=\"%s\" data-series=\"%s\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"><title>%s</title></rect>", html.EscapeString(category), html.EscapeString(seriesKey(p)), math.Min(x, zeroX), box.y+float64(ci)*outer+float64(si)*inner+1, math.Abs(x-zeroX), math.Max(1, inner-2), color, label)
			} else {
				y := box.y + box.h - scaled(value, lo, hi, 0, box.h)
				fmt.Fprintf(b, "<rect class=\"column\" data-layout=\"grouped\" data-category=\"%s\" data-series=\"%s\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"><title>%s</title></rect>", html.EscapeString(category), html.EscapeString(seriesKey(p)), box.x+float64(ci)*outer+float64(si)*inner+1, math.Min(y, zeroY), math.Max(1, inner-2), math.Abs(y-zeroY), color, label)
			}
		}
	}
}

func drawStackedBars(b *strings.Builder, c *charts.Output, box plotBox, horizontal bool, timezone string) {
	categories, series, groups := orderedGroups(c.Points)
	lo, hi, found := 0.0, 0.0, false
	for _, category := range categories {
		pos, neg := 0.0, 0.0
		for _, p := range groups[category] {
			if v, ok := coordinate(p.Value); ok {
				found = true
				if v >= 0 {
					pos += v
				} else {
					neg += v
				}
			}
		}
		lo = math.Min(lo, neg)
		hi = math.Max(hi, pos)
	}
	if !found {
		return
	}
	if lo == hi {
		hi = lo + 1
	}
	band := map[bool]float64{true: box.h / float64(max(1, len(categories))), false: box.w / float64(max(1, len(categories)))}[horizontal]
	seriesIndex := map[string]int{}
	for i, s := range series {
		seriesIndex[s] = i
	}
	for ci, category := range categories {
		pos, neg := 0.0, 0.0
		for _, p := range groups[category] {
			v, ok := coordinate(p.Value)
			if !ok {
				continue
			}
			start := pos
			if v >= 0 {
				pos += v
			} else {
				start = neg
				neg += v
			}
			end := start + v
			color := chartPalette[seriesIndex[seriesKey(p)]%len(chartPalette)]
			label := html.EscapeString(pointLabel(c, p, timezone))
			if horizontal {
				x1, x2 := scaled(start, lo, hi, box.x, box.w), scaled(end, lo, hi, box.x, box.w)
				fmt.Fprintf(b, "<rect class=\"bar\" data-layout=\"stacked\" data-category=\"%s\" data-start=\"%.6g\" data-end=\"%.6g\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"><title>%s</title></rect>", html.EscapeString(category), start, end, math.Min(x1, x2), box.y+float64(ci)*band+2, math.Abs(x2-x1), math.Max(1, band-4), color, label)
			} else {
				y1, y2 := box.y+box.h-scaled(start, lo, hi, 0, box.h), box.y+box.h-scaled(end, lo, hi, 0, box.h)
				fmt.Fprintf(b, "<rect class=\"column\" data-layout=\"stacked\" data-category=\"%s\" data-start=\"%.6g\" data-end=\"%.6g\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"><title>%s</title></rect>", html.EscapeString(category), start, end, box.x+float64(ci)*band+2, math.Min(y1, y2), math.Max(1, band-4), math.Abs(y2-y1), color, label)
			}
		}
	}
}

func drawExactLabels(b *strings.Builder, c *charts.Output, foreground, timezone string) {
	b.WriteString("<g class=\"exact-labels\" fill=\"" + foreground + "\">")
	for index, point := range c.Points {
		if index >= 8 {
			break
		}
		label := pointLabel(c, point, timezone)
		if label != "" {
			fmt.Fprintf(b, "<text x=\"56\" y=\"%d\">%s</text>", 52+index*16, html.EscapeString(label))
		}
	}
	b.WriteString("</g>")
}

func coordinate(v charts.Value) (float64, bool) {
	returnValue := 0.0
	if v.Null || v.Coordinate == nil || math.IsNaN(*v.Coordinate) || math.IsInf(*v.Coordinate, 0) {
		return returnValue, false
	}
	return *v.Coordinate, true
}

func valueRange(points []charts.Point, pick func(charts.Point) charts.Value, includeZero bool) (float64, float64, bool) {
	lo, hi, found := 0.0, 0.0, false
	for _, point := range points {
		value, ok := coordinate(pick(point))
		if !ok {
			continue
		}
		if !found || value < lo {
			lo = value
		}
		if !found || value > hi {
			hi = value
		}
		found = true
	}
	if includeZero {
		lo = math.Min(lo, 0)
		hi = math.Max(hi, 0)
	}
	if found && lo == hi {
		hi = lo + 1
	}
	return lo, hi, found
}

func scaled(value, lo, hi, start, size float64) float64 { return start + (value-lo)/(hi-lo)*size }

func pointLabel(c *charts.Output, point charts.Point, timezone string) string {
	parts := []string{}
	if !point.Category.Null {
		parts = append(parts, formatCell(point.Category, chartColumn(c, c.Mapping.Bindings.Category), timezone))
	}
	if !point.Series.Null {
		parts = append(parts, point.Series.Value)
	}
	valueID := point.Measure
	if valueID == "" {
		valueID = c.Mapping.Bindings.Value
	}
	for _, item := range []struct {
		value charts.Value
		id    string
	}{{point.X, c.Mapping.Bindings.X}, {point.Y, c.Mapping.Bindings.Y}, {point.Value, valueID}} {
		if !item.value.Null && item.value.Exact != "" {
			parts = append(parts, formatValue(item.value, chartColumn(c, item.id), timezone))
		}
	}
	return strings.Join(parts, " · ")
}

func drawBars(b *strings.Builder, c *charts.Output, box plotBox, horizontal bool, timezone string) {
	lo, hi, found := valueRange(c.Points, func(p charts.Point) charts.Value { return p.Value }, true)
	if !found {
		return
	}
	count := max(1, len(c.Points))
	band := map[bool]float64{true: box.h / float64(count), false: box.w / float64(count)}[horizontal]
	zeroX := scaled(0, lo, hi, box.x, box.w)
	zeroY := box.y + box.h - scaled(0, lo, hi, 0, box.h)
	for index, point := range c.Points {
		value, ok := coordinate(point.Value)
		if !ok {
			continue
		}
		color := chartPalette[index%len(chartPalette)]
		label := html.EscapeString(pointLabel(c, point, timezone))
		if horizontal {
			x := scaled(value, lo, hi, box.x, box.w)
			fmt.Fprintf(b, "<rect class=\"bar\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"><title>%s</title></rect>", math.Min(x, zeroX), box.y+float64(index)*band+2, math.Abs(x-zeroX), math.Max(1, band-4), color, label)
		} else {
			y := box.y + box.h - scaled(value, lo, hi, 0, box.h)
			fmt.Fprintf(b, "<rect class=\"column\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"><title>%s</title></rect>", box.x+float64(index)*band+2, math.Min(y, zeroY), math.Max(1, band-4), math.Abs(y-zeroY), color, label)
		}
	}
}

func drawLines(b *strings.Builder, c *charts.Output, box plotBox, area bool, timezone string) {
	lo, hi, found := valueRange(c.Points, func(p charts.Point) charts.Value { return p.Value }, false)
	if !found {
		return
	}
	groups, order := map[string][]charts.Point{}, []string{}
	for _, point := range c.Points {
		key := point.SeriesID
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], point)
	}
	for seriesIndex, key := range order {
		points := groups[key]
		segments, labels := [][]string{}, [][]string{}
		coords, segmentLabels := []string{}, []string{}
		for index, point := range points {
			value, ok := coordinate(point.Value)
			if !ok {
				if len(coords) != 0 {
					segments, labels = append(segments, coords), append(labels, segmentLabels)
					coords, segmentLabels = nil, nil
				}
				continue
			}
			x := box.x + float64(index)*box.w/float64(max(1, len(points)-1))
			y := box.y + box.h - scaled(value, lo, hi, 0, box.h)
			coords = append(coords, fmt.Sprintf("%.1f,%.1f", x, y))
			segmentLabels = append(segmentLabels, pointLabel(c, point, timezone))
		}
		if len(coords) != 0 {
			segments, labels = append(segments, coords), append(labels, segmentLabels)
		}
		if len(segments) == 0 {
			continue
		}
		color := chartPalette[seriesIndex%len(chartPalette)]
		for segmentIndex, segment := range segments {
			if area {
				firstX, lastX := strings.SplitN(segment[0], ",", 2)[0], strings.SplitN(segment[len(segment)-1], ",", 2)[0]
				polygon := fmt.Sprintf("%s,%.1f %s %s,%.1f", firstX, box.y+box.h, strings.Join(segment, " "), lastX, box.y+box.h)
				fmt.Fprintf(b, "<polygon class=\"area\" points=\"%s\" fill=\"%s\" fill-opacity=\"0.28\"/>", polygon, color)
			}
			fmt.Fprintf(b, "<polyline class=\"line\" points=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"2\"><title>%s</title></polyline>", strings.Join(segment, " "), color, html.EscapeString(strings.Join(labels[segmentIndex], " | ")))
		}
	}
}

func drawScatter(b *strings.Builder, c *charts.Output, box plotBox, timezone string) {
	xlo, xhi, xfound := valueRange(c.Points, func(p charts.Point) charts.Value { return p.X }, false)
	ylo, yhi, yfound := valueRange(c.Points, func(p charts.Point) charts.Value { return p.Y }, false)
	if !xfound || !yfound {
		return
	}
	for index, point := range c.Points {
		xv, xok := coordinate(point.X)
		yv, yok := coordinate(point.Y)
		if !xok || !yok {
			continue
		}
		radius := 5.0
		if point.Size != nil {
			if size, ok := coordinate(*point.Size); ok && size > 0 {
				radius = math.Min(18, 4+math.Sqrt(size))
			}
		}
		fmt.Fprintf(b, "<circle class=\"scatter\" cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\" fill=\"%s\"><title>%s</title></circle>", scaled(xv, xlo, xhi, box.x, box.w), box.y+box.h-scaled(yv, ylo, yhi, 0, box.h), radius, chartPalette[index%len(chartPalette)], html.EscapeString(pointLabel(c, point, timezone)))
	}
}

func drawHeatmap(b *strings.Builder, c *charts.Output, box plotBox, timezone string) {
	lo, hi, found := valueRange(c.Points, func(p charts.Point) charts.Value { return p.Value }, false)
	if !found {
		return
	}
	if lo == hi {
		hi = lo + 1
	}
	xs, ys := []string{}, []string{}
	xi, yi := map[string]int{}, map[string]int{}
	axis := func(v charts.Value) string {
		if v.Exact != "" {
			return v.Exact
		}
		if n, ok := coordinate(v); ok {
			return fmt.Sprintf("%.12g", n)
		}
		return ""
	}
	for _, point := range c.Points {
		x, y := axis(point.X), axis(point.Y)
		if _, ok := xi[x]; !ok {
			xi[x] = len(xs)
			xs = append(xs, x)
		}
		if _, ok := yi[y]; !ok {
			yi[y] = len(ys)
			ys = append(ys, y)
		}
	}
	cellW, cellH := box.w/float64(max(1, len(xs))), box.h/float64(max(1, len(ys)))
	for _, point := range c.Points {
		value, ok := coordinate(point.Value)
		if !ok {
			continue
		}
		opacity := 0.2 + 0.8*(value-lo)/(hi-lo)
		x, y := axis(point.X), axis(point.Y)
		fmt.Fprintf(b, "<rect class=\"heatmap\" data-x=\"%s\" data-y=\"%s\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"#16877c\" fill-opacity=\"%.3f\"><title>%s</title></rect>", html.EscapeString(x), html.EscapeString(y), box.x+float64(xi[x])*cellW, box.y+float64(yi[y])*cellH, math.Max(1, cellW-2), math.Max(1, cellH-2), opacity, html.EscapeString(pointLabel(c, point, timezone)))
	}
}

func drawPie(b *strings.Builder, c *charts.Output, box plotBox, donut bool, background, timezone string) {
	total := 0.0
	for _, point := range c.Points {
		if value, ok := coordinate(point.Value); ok && value > 0 {
			total += value
		}
	}
	if total <= 0 {
		return
	}
	cx, cy, radius, angle := box.x+box.w/2, box.y+box.h/2, math.Min(box.w, box.h)*0.42, -math.Pi/2
	for index, point := range c.Points {
		value, ok := coordinate(point.Value)
		if !ok || value <= 0 {
			continue
		}
		next := angle + 2*math.Pi*value/total
		x1, y1, x2, y2 := cx+radius*math.Cos(angle), cy+radius*math.Sin(angle), cx+radius*math.Cos(next), cy+radius*math.Sin(next)
		large := 0
		if next-angle > math.Pi {
			large = 1
		}
		fmt.Fprintf(b, "<path class=\"slice\" d=\"M%.1f %.1fL%.1f %.1fA%.1f %.1f 0 %d 1 %.1f %.1fZ\" fill=\"%s\"><title>%s</title></path>", cx, cy, x1, y1, radius, radius, large, x2, y2, chartPalette[index%len(chartPalette)], html.EscapeString(pointLabel(c, point, timezone)))
		angle = next
	}
	if donut {
		fmt.Fprintf(b, "<circle class=\"donut-hole\" cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\" fill=\"%s\"/>", cx, cy, radius*0.5, background)
	}
}

func drawTreemap(b *strings.Builder, c *charts.Output, box plotBox, timezone string) {
	total := 0.0
	for _, node := range c.Hierarchy {
		if value, ok := coordinate(node.Value); ok && value > 0 {
			total += value
		}
	}
	if len(c.Hierarchy) == 0 {
		for _, point := range c.Points {
			if value, ok := coordinate(point.Value); ok && value > 0 {
				total += value
			}
		}
	}
	if total <= 0 {
		return
	}
	x := box.x
	if len(c.Hierarchy) == 0 {
		for index, point := range c.Points {
			value, ok := coordinate(point.Value)
			if !ok || value <= 0 {
				continue
			}
			width := box.w * value / total
			fmt.Fprintf(b, "<rect class=\"treemap\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"><title>%s</title></rect>", x, box.y, math.Max(1, width-2), box.h, chartPalette[index%len(chartPalette)], html.EscapeString(pointLabel(c, point, timezone)))
			x += width
		}
		return
	}
	for index, node := range c.Hierarchy {
		value, ok := coordinate(node.Value)
		if !ok || value <= 0 {
			continue
		}
		width := box.w * value / total
		labels := []string{}
		for _, item := range node.Path {
			if !item.Null {
				labels = append(labels, item.Value)
			}
		}
		label := strings.Join(labels, " / ") + " · " + formatValue(node.Value, chartColumn(c, c.Mapping.Bindings.Value), timezone)
		fmt.Fprintf(b, "<rect class=\"treemap\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\"><title>%s</title></rect>", x, box.y, math.Max(1, width-2), box.h, chartPalette[index%len(chartPalette)], html.EscapeString(label))
		x += width
	}
}

func drawSparkline(b *strings.Builder, values []charts.Value, width, height int) {
	points := make([]charts.Point, len(values))
	for index, value := range values {
		points[index].Value = value
	}
	lo, hi, found := valueRange(points, func(p charts.Point) charts.Value { return p.Value }, false)
	if !found {
		return
	}
	coords := []string{}
	for index, value := range values {
		coordinate, ok := coordinate(value)
		if ok {
			coords = append(coords, fmt.Sprintf("%.1f,%.1f", 16+float64(index)*float64(width-32)/float64(max(1, len(values)-1)), float64(height-16)-scaled(coordinate, lo, hi, 0, 48)))
		}
	}
	if len(coords) != 0 {
		fmt.Fprintf(b, "<polyline class=\"sparkline\" points=\"%s\" fill=\"none\" stroke=\"#16877c\" stroke-width=\"2\"/>", strings.Join(coords, " "))
	}
}
