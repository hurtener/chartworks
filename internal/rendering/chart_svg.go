package rendering

import (
	"fmt"
	"math"
	"strings"

	"github.com/hurtener/chartworks/internal/charts"
)

var chartPalette = []string{"#16877c", "#db6b4f", "#5d6fb6", "#d5a62e", "#7b5aa6", "#4d8b43"}

type plotBox struct{ x, y, w, h float64 }

func appendChartGeometry(b *drawingScene, c *charts.Output, width, height int, foreground, background, timezone string) error {
	box := plotBox{48, 36, math.Max(1, float64(width-64)), math.Max(1, float64(height-76))}
	b.add(drawingPrimitive{kind: "axis", stroke: foreground, x: box.x, y: box.y, h: box.y + box.h, w: box.x + box.w})
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

func drawGroupedBars(b *drawingScene, c *charts.Output, box plotBox, horizontal bool, timezone string) {
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
			label := pointLabel(c, p, timezone)
			if horizontal {
				x := scaled(value, lo, hi, box.x, box.w)
				b.add(drawingPrimitive{kind: "rect", class: "bar", layout: "grouped", category: category, series: seriesKey(p), x: math.Min(x, zeroX), y: box.y + float64(ci)*outer + float64(si)*inner + 1, w: math.Abs(x - zeroX), h: math.Max(1, inner-2), fill: color, title: label})
			} else {
				y := box.y + box.h - scaled(value, lo, hi, 0, box.h)
				b.add(drawingPrimitive{kind: "rect", class: "column", layout: "grouped", category: category, series: seriesKey(p), x: box.x + float64(ci)*outer + float64(si)*inner + 1, y: math.Min(y, zeroY), w: math.Max(1, inner-2), h: math.Abs(y - zeroY), fill: color, title: label})
			}
		}
	}
}

func drawStackedBars(b *drawingScene, c *charts.Output, box plotBox, horizontal bool, timezone string) {
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
			label := pointLabel(c, p, timezone)
			if horizontal {
				x1, x2 := scaled(start, lo, hi, box.x, box.w), scaled(end, lo, hi, box.x, box.w)
				b.add(drawingPrimitive{kind: "rect", class: "bar", layout: "stacked", category: category, start: start, end: end, x: math.Min(x1, x2), y: box.y + float64(ci)*band + 2, w: math.Abs(x2 - x1), h: math.Max(1, band-4), fill: color, title: label})
			} else {
				y1, y2 := box.y+box.h-scaled(start, lo, hi, 0, box.h), box.y+box.h-scaled(end, lo, hi, 0, box.h)
				b.add(drawingPrimitive{kind: "rect", class: "column", layout: "stacked", category: category, start: start, end: end, x: box.x + float64(ci)*band + 2, y: math.Min(y1, y2), w: math.Max(1, band-4), h: math.Abs(y2 - y1), fill: color, title: label})
			}
		}
	}
}

func drawExactLabels(b *drawingScene, c *charts.Output, foreground, timezone string) {
	b.add(drawingPrimitive{kind: "labels_start", fill: foreground})
	for index, point := range c.Points {
		if index >= 8 {
			break
		}
		label := pointLabel(c, point, timezone)
		if label != "" {
			b.add(drawingPrimitive{kind: "text", x: 56, y: float64(52 + index*16), text: label})
		}
	}
	b.add(drawingPrimitive{kind: "group_end"})
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

func drawBars(b *drawingScene, c *charts.Output, box plotBox, horizontal bool, timezone string) {
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
		label := pointLabel(c, point, timezone)
		if horizontal {
			x := scaled(value, lo, hi, box.x, box.w)
			b.add(drawingPrimitive{kind: "rect", class: "bar", x: math.Min(x, zeroX), y: box.y + float64(index)*band + 2, w: math.Abs(x - zeroX), h: math.Max(1, band-4), fill: color, title: label})
		} else {
			y := box.y + box.h - scaled(value, lo, hi, 0, box.h)
			b.add(drawingPrimitive{kind: "rect", class: "column", x: box.x + float64(index)*band + 2, y: math.Min(y, zeroY), w: math.Max(1, band-4), h: math.Abs(y - zeroY), fill: color, title: label})
		}
	}
}

func drawLines(b *drawingScene, c *charts.Output, box plotBox, area bool, timezone string) {
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
		segments, labels := [][]drawingPoint{}, [][]string{}
		coords, segmentLabels := []drawingPoint{}, []string{}
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
			coords = append(coords, drawingPoint{x, y})
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
				polygon := append([]drawingPoint{{segment[0].x, box.y + box.h}}, segment...)
				polygon = append(polygon, drawingPoint{segment[len(segment)-1].x, box.y + box.h})
				b.add(drawingPrimitive{kind: "polygon", class: "area", points: polygon, fill: color, opacity: 0.28})
			}
			b.add(drawingPrimitive{kind: "polyline", class: "line", points: segment, stroke: color, title: strings.Join(labels[segmentIndex], " | ")})
		}
	}
}

func drawScatter(b *drawingScene, c *charts.Output, box plotBox, timezone string) {
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
		b.add(drawingPrimitive{kind: "circle", class: "scatter", x: scaled(xv, xlo, xhi, box.x, box.w), y: box.y + box.h - scaled(yv, ylo, yhi, 0, box.h), r: radius, fill: chartPalette[index%len(chartPalette)], title: pointLabel(c, point, timezone)})
	}
}

func drawHeatmap(b *drawingScene, c *charts.Output, box plotBox, timezone string) {
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
		b.add(drawingPrimitive{kind: "rect", class: "heatmap", layout: "heatmap", category: x, series: y, x: box.x + float64(xi[x])*cellW, y: box.y + float64(yi[y])*cellH, w: math.Max(1, cellW-2), h: math.Max(1, cellH-2), fill: "#16877c", opacity: opacity, title: pointLabel(c, point, timezone)})
	}
}

func drawPie(b *drawingScene, c *charts.Output, box plotBox, donut bool, background, timezone string) {
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
		b.add(drawingPrimitive{kind: "slice", class: "slice", x: cx, y: cy, r: radius, start: angle, end: next, large: large, points: []drawingPoint{{x1, y1}, {x2, y2}}, fill: chartPalette[index%len(chartPalette)], title: pointLabel(c, point, timezone)})
		angle = next
	}
	if donut {
		b.add(drawingPrimitive{kind: "circle", class: "donut-hole", x: cx, y: cy, r: radius * 0.5, fill: background})
	}
}

func drawTreemap(b *drawingScene, c *charts.Output, box plotBox, timezone string) {
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
			b.add(drawingPrimitive{kind: "rect", class: "treemap", x: x, y: box.y, w: math.Max(1, width-2), h: box.h, fill: chartPalette[index%len(chartPalette)], title: pointLabel(c, point, timezone)})
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
		b.add(drawingPrimitive{kind: "rect", class: "treemap", x: x, y: box.y, w: math.Max(1, width-2), h: box.h, fill: chartPalette[index%len(chartPalette)], title: label})
		x += width
	}
}

func appendSparkline(b *drawingScene, values []charts.Value, width, height int) {
	points := make([]charts.Point, len(values))
	for index, value := range values {
		points[index].Value = value
	}
	lo, hi, found := valueRange(points, func(p charts.Point) charts.Value { return p.Value }, false)
	if !found {
		return
	}
	coords := []drawingPoint{}
	for index, value := range values {
		coordinate, ok := coordinate(value)
		if ok {
			coords = append(coords, drawingPoint{16 + float64(index)*float64(width-32)/float64(max(1, len(values)-1)), float64(height-16) - scaled(coordinate, lo, hi, 0, 48)})
		}
	}
	if len(coords) != 0 {
		b.add(drawingPrimitive{kind: "polyline", class: "sparkline", points: coords, stroke: "#16877c"})
	}
}
