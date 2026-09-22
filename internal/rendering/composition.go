package rendering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// renderComposition preserves the accepted page/widget geometry. Every widget is
// read separately through Delivery, while each chart crosses the sealed worker.
func (s *Service) renderComposition(ctx context.Context, e identity.Envelope, in Request, root reporting.DeliveryViewResult) (Rendition, error) {
	if len(root.Pages) == 0 {
		return Rendition{}, ErrInvalid
	}
	pages := append([]reporting.CompositionPageSummary(nil), root.Pages...)
	var b strings.Builder
	if in.Format == "html" {
		fmt.Fprintf(&b, "<!doctype html><html><head><meta charset=\"utf-8\"><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; style-src 'unsafe-inline'\"><style>body{margin:0}main{width:%dpx}.page{display:grid;grid-template-columns:repeat(24,1fr);grid-auto-rows:32px;gap:8px}.widget{overflow:hidden;border:1px solid #ddd;padding:8px}</style></head><body><main>", in.Width)
	} else {
		fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" role=\"img\" viewBox=\"0 0 %d %d\">", in.Width, in.Height)
	}
	y := 0
	for _, page := range pages {
		widgets := append([]reporting.CompositionWidgetSummary(nil), page.Widgets...)
		sort.SliceStable(widgets, func(i, j int) bool {
			if widgets[i].Grid.Row == widgets[j].Grid.Row {
				return widgets[i].Grid.Column < widgets[j].Grid.Column
			}
			return widgets[i].Grid.Row < widgets[j].Grid.Row
		})
		if in.Format == "html" {
			fmt.Fprintf(&b, "<section class=\"page\" data-page=\"%s\"><h1 style=\"grid-column:1/25\">%s</h1>", html.EscapeString(page.ID), html.EscapeString(page.Title))
		} else {
			y += 24
			fmt.Fprintf(&b, "<text x=\"8\" y=\"%d\">%s</text>", y, html.EscapeString(page.Title))
		}
		for _, widget := range widgets {
			if widget.State != "completed" {
				continue
			}
			request := in
			request.Full = false
			request.View.Page = page.ID
			request.View.Widget = widget.ID
			request.View.Output = ""
			request.Width = max(320, in.Width*widget.Grid.Width/24)
			request.Height = max(200, widget.Grid.Height*32)
			view, err := s.viewer.View(ctx, e, request.View)
			if err != nil {
				return Rendition{}, err
			}
			content := ""
			if view.Text != nil {
				content = html.EscapeString(view.Text.Text)
			} else if view.Output != nil {
				rendered, renderErr := s.render(ctx, request, view)
				if renderErr != nil {
					return Rendition{}, renderErr
				}
				content = rendered.Content
			} else {
				continue
			}
			if in.Format == "html" {
				fmt.Fprintf(&b, "<article class=\"widget\" data-widget=\"%s\" style=\"grid-column:%d/span %d;grid-row:%d/span %d\"><h2>%s</h2>%s</article>", html.EscapeString(widget.ID), widget.Grid.Column+1, widget.Grid.Width, widget.Grid.Row+2, widget.Grid.Height, html.EscapeString(widget.Presentation.Title), content)
			} else {
				y += 24
				fmt.Fprintf(&b, "<text x=\"16\" y=\"%d\">%s</text><g transform=\"translate(0 %d)\">%s</g>", y, html.EscapeString(widget.Presentation.Title), y, stripSVG(content))
			}
		}
		if in.Format == "html" {
			b.WriteString("</section>")
		}
	}
	if in.Format == "html" {
		b.WriteString("</main></body></html>")
	} else {
		b.WriteString("</svg>")
	}
	content := b.String()
	if len(content) > s.maxBytes {
		return Rendition{}, reporting.ErrBudget
	}
	sum := sha256.Sum256([]byte(content))
	source := root.Summary.Run
	if root.Summary.Target.ID != "" {
		source = root.Summary.Target.ID + ":" + source
	}
	sourceSum := sha256.Sum256([]byte(source))
	return Rendition{State: "succeeded", Version: Version, Format: in.Format, MediaType: map[bool]string{true: "text/html; charset=utf-8", false: "image/svg+xml"}[in.Format == "html"], Theme: in.Theme, Width: in.Width, Height: in.Height, SourceDigest: hex.EncodeToString(sourceSum[:]), Digest: hex.EncodeToString(sum[:]), Bytes: len(content), Content: content}, nil
}

func stripSVG(s string) string {
	start := strings.Index(s, ">")
	end := strings.LastIndex(s, "</svg>")
	if strings.HasPrefix(s, "<svg") && start >= 0 && end > start {
		return s[start+1 : end]
	}
	return html.EscapeString(s)
}
