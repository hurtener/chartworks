package rendering

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

const maxRasterPixels = 4096 * 4096

func sceneColor(value string, opacity float64) (color.RGBA, error) {
	if len(value) != 7 || value[0] != '#' || opacity < 0 || opacity > 1 {
		return color.RGBA{}, ErrInvalid
	}
	var c [3]uint8
	for i := 0; i < 3; i++ {
		for _, b := range []byte(value[1+i*2 : 3+i*2]) {
			c[i] *= 16
			switch {
			case b >= '0' && b <= '9':
				c[i] += b - '0'
			case b >= 'a' && b <= 'f':
				c[i] += b - 'a' + 10
			case b >= 'A' && b <= 'F':
				c[i] += b - 'A' + 10
			default:
				return color.RGBA{}, ErrInvalid
			}
		}
	}
	a := uint8(math.Round(opacity * 255))
	return color.RGBA{uint8(uint16(c[0]) * uint16(a) / 255), uint8(uint16(c[1]) * uint16(a) / 255), uint8(uint16(c[2]) * uint16(a) / 255), a}, nil
}

type rasterWriter struct {
	ctx   context.Context
	b     bytes.Buffer
	limit int
}

func (w *rasterWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > w.limit-w.b.Len() {
		return 0, ErrInvalid
	}
	return w.b.Write(p)
}

func rasterScene(ctx context.Context, s *drawingScene, width, height, maxBytes int) ([]byte, error) {
	if ctx == nil || s == nil || !s.valid() || width < 1 || height < 1 || width > 4096 || height > 4096 || int64(width)*int64(height) > maxRasterPixels || maxBytes < 1 || maxBytes > 64<<20 {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, ErrInvalid
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 16, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, ErrInvalid
	}
	defer face.Close()
	bf, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, ErrInvalid
	}
	bold, err := opentype.NewFace(bf, &opentype.FaceOptions{Size: 16, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, ErrInvalid
	}
	defer bold.Close()
	// Validate labels before allocating pixels. No system-font fallback or glyph
	// substitution may silently change retained labels.
	for _, p := range s.nodes {
		if p.kind == "text" {
			active := face
			if p.class == "header" {
				active = bold
			}
			limit := float64(width) - p.x
			if p.w > 0 {
				limit = p.w
			}
			if p.x < 0 || p.y < 16 || p.y > float64(height) || float64(font.MeasureString(active, p.text))/64 > limit {
				return nil, ErrInvalid
			}
			inkBounds, _ := font.BoundString(active, p.text)
			if p.x+float64(inkBounds.Min.X)/64 < 0 || p.y+float64(inkBounds.Min.Y)/64 < 0 || p.x+float64(inkBounds.Max.X)/64 > float64(width) || p.y+float64(inkBounds.Max.Y)/64 > float64(height) {
				return nil, ErrInvalid
			}
			for _, r := range p.text {
				if unicode.IsControl(r) {
					return nil, ErrInvalid
				}
				if _, ok := active.GlyphAdvance(r); !ok {
					return nil, ErrInvalid
				}
			}
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	foreground := "#17211f"
	for _, p := range s.nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch p.kind {
		case "labels_start":
			foreground = p.fill
		case "group_end":
		case "text":
			ink, e := sceneColor(foreground, 1)
			if p.fill != "" {
				ink, e = sceneColor(p.fill, 1)
			}
			if e != nil {
				return nil, e
			}
			active := face
			if p.class == "header" {
				active = bold
			}
			d := font.Drawer{Dst: dst, Src: image.NewUniform(ink), Face: active, Dot: fixed.Point26_6{X: fixed.Int26_6(math.Round(p.x * 64)), Y: fixed.Int26_6(math.Round(p.y * 64))}}
			d.DrawString(p.text)
		case "axis":
			ink, e := sceneColor(p.stroke, 1)
			if e != nil {
				return nil, e
			}
			strokePath(dst, []drawingPoint{{p.x, p.y}, {p.x, p.h}, {p.w, p.h}}, 1, ink)
		case "rect":
			opacity := 1.0
			if p.layout == "heatmap" {
				opacity = p.opacity
			}
			ink, e := sceneColor(p.fill, opacity)
			if e != nil {
				return nil, e
			}
			if p.w < 0 || p.h < 0 {
				return nil, ErrInvalid
			}
			fillPath(dst, []drawingPoint{{p.x, p.y}, {p.x + p.w, p.y}, {p.x + p.w, p.y + p.h}, {p.x, p.y + p.h}}, ink)
		case "circle":
			ink, e := sceneColor(p.fill, 1)
			if e != nil {
				return nil, e
			}
			if p.r < 0 {
				return nil, ErrInvalid
			}
			fillPath(dst, circlePoints(p.x, p.y, p.r), ink)
		case "polygon":
			ink, e := sceneColor(p.fill, p.opacity)
			if e != nil {
				return nil, e
			}
			fillPath(dst, p.points, ink)
		case "polyline":
			ink, e := sceneColor(p.stroke, 1)
			if e != nil {
				return nil, e
			}
			strokePath(dst, p.points, 2, ink)
		case "slice":
			if p.r < 0 || p.end < p.start || p.end-p.start > 2*math.Pi+1e-9 {
				return nil, ErrInvalid
			}
			ink, e := sceneColor(p.fill, 1)
			if e != nil {
				return nil, e
			}
			n := max(2, min(1024, int(math.Ceil((p.end-p.start)*math.Max(1, p.r)/2))))
			points := make([]drawingPoint, 0, n+2)
			points = append(points, drawingPoint{p.x, p.y})
			for i := 0; i <= n; i++ {
				a := p.start + (p.end-p.start)*float64(i)/float64(n)
				points = append(points, drawingPoint{p.x + p.r*math.Cos(a), p.y + p.r*math.Sin(a)})
			}
			fillPath(dst, points, ink)
		default:
			return nil, ErrInvalid
		}
	}
	w := &rasterWriter{ctx: ctx, limit: maxBytes}
	if err := png.Encode(w, dst); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, ErrInvalid
	}
	return w.b.Bytes(), nil
}

func circlePoints(x, y, r float64) []drawingPoint {
	n := max(12, min(512, int(math.Ceil(r*2))))
	points := make([]drawingPoint, n)
	for i := range points {
		a := 2 * math.Pi * float64(i) / float64(n)
		points[i] = drawingPoint{x + r*math.Cos(a), y + r*math.Sin(a)}
	}
	return points
}
func strokePath(dst *image.RGBA, points []drawingPoint, width float64, ink color.RGBA) {
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		dx, dy := b.x-a.x, b.y-a.y
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		nx, ny := -dy*width/(2*l), dx*width/(2*l)
		fillPath(dst, []drawingPoint{{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny}, {b.x - nx, b.y - ny}, {a.x - nx, a.y - ny}}, ink)
	}
}
func fillPath(dst *image.RGBA, points []drawingPoint, ink color.RGBA) {
	if len(points) < 3 {
		return
	}
	minX, minY, maxX, maxY := points[0].x, points[0].y, points[0].x, points[0].y
	for _, p := range points {
		minX = math.Min(minX, p.x)
		minY = math.Min(minY, p.y)
		maxX = math.Max(maxX, p.x)
		maxY = math.Max(maxY, p.y)
	}
	bounds := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1).Intersect(dst.Bounds())
	if bounds.Empty() {
		return
	}
	z := vector.NewRasterizer(bounds.Dx(), bounds.Dy())
	z.DrawOp = draw.Over
	z.MoveTo(float32(points[0].x-float64(bounds.Min.X)), float32(points[0].y-float64(bounds.Min.Y)))
	for _, p := range points[1:] {
		z.LineTo(float32(p.x-float64(bounds.Min.X)), float32(p.y-float64(bounds.Min.Y)))
	}
	z.ClosePath()
	z.Draw(dst, bounds, image.NewUniform(ink), image.Point{})
}
