package rendering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	widgetCount := 0
	for _, page := range pages {
		widgetCount += len(page.Widgets)
	}
	if widgetCount > s.options.MaxWidgets {
		return Rendition{}, reporting.ErrBudget
	}
	inputBytes := 0
	actualHeight := in.Height
	if in.Format == "svg" {
		actualHeight = compositionHeight(pages)
	}
	provenance := sha256.New()
	rootWire, _ := json.Marshal(struct {
		Summary reporting.DeliveryRunSummary
		Pages   []reporting.CompositionPageSummary
	}{root.Summary, pages})
	_, _ = provenance.Write(rootWire)
	var b strings.Builder
	if in.Format == "html" {
		fmt.Fprintf(&b, "<!doctype html><html><head><meta charset=\"utf-8\"><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; style-src 'unsafe-inline'\"><style>body{margin:0}main{width:%dpx}.page{display:grid;grid-template-columns:repeat(12,1fr);grid-auto-rows:32px;gap:8px}.widget{overflow:hidden;border:1px solid #ddd;padding:8px}</style></head><body><main>", in.Width)
	} else {
		fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" role=\"img\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">", in.Width, actualHeight, in.Width, actualHeight)
	}
	pageY := 0
	for _, page := range pages {
		widgets := append([]reporting.CompositionWidgetSummary(nil), page.Widgets...)
		sort.SliceStable(widgets, func(i, j int) bool {
			if widgets[i].Grid.Row == widgets[j].Grid.Row {
				return widgets[i].Grid.Column < widgets[j].Grid.Column
			}
			return widgets[i].Grid.Row < widgets[j].Grid.Row
		})
		if in.Format == "html" {
			fmt.Fprintf(&b, "<section class=\"page\" data-page=\"%s\"><h1 style=\"grid-column:1/13\">%s</h1>", html.EscapeString(page.ID), html.EscapeString(page.Title))
		} else {
			fmt.Fprintf(&b, "<text x=\"8\" y=\"%d\">%s</text>", pageY+24, html.EscapeString(page.Title))
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
			request.Width = max(320, in.Width*widget.Grid.Width/12)
			request.Height = max(200, widget.Grid.Height*32)
			view, err := s.viewer.View(ctx, e, request.View)
			if err != nil {
				return Rendition{}, err
			}
			content := ""
			viewWire, _ := json.Marshal(view)
			inputBytes += len(viewWire)
			if inputBytes > s.options.MaxInputBytes {
				return Rendition{}, reporting.ErrBudget
			}
			switch {
			case view.Text != nil:
				content = html.EscapeString(view.Text.Text)
				textSum := sha256.Sum256([]byte(view.Text.Format + "\x00" + view.Text.Text))
				_, _ = provenance.Write(textSum[:])
			case view.Output != nil:
				rendered, renderErr := s.render(ctx, request, view)
				if renderErr != nil {
					return Rendition{}, renderErr
				}
				content = rendered.Content
				_, _ = provenance.Write([]byte(view.Output.RetainedDigest))
			default:
				continue
			}
			if b.Len()+len(content) > s.maxBytes {
				return Rendition{}, reporting.ErrBudget
			}
			if in.Format == "html" {
				fmt.Fprintf(&b, "<article class=\"widget\" data-widget=\"%s\" style=\"grid-column:%d/span %d;grid-row:%d/span %d\"><h2>%s</h2>%s</article>", html.EscapeString(widget.ID), widget.Grid.Column+1, widget.Grid.Width, widget.Grid.Row+2, widget.Grid.Height, html.EscapeString(widget.Presentation.Title), htmlFragment(content, view.Text != nil))
			} else {
				x := in.Width * widget.Grid.Column / 12
				y := pageY + 40 + widget.Grid.Row*40
				width := in.Width * widget.Grid.Width / 12
				height := widget.Grid.Height * 40
				fmt.Fprintf(&b, "<text x=\"%d\" y=\"%d\">%s</text>", x+8, y+18, html.EscapeString(widget.Presentation.Title))
				if view.Text != nil {
					fmt.Fprintf(&b, "<text x=\"%d\" y=\"%d\">%s</text>", x+8, y+38, content)
				} else {
					b.WriteString(placeSVG(content, x, y+24, width, max(1, height-24)))
				}
			}
		}
		if in.Format == "html" {
			b.WriteString("</section>")
		} else {
			pageY += pageHeight(page)
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
	request := in
	request.Height = actualHeight
	work := SealedWork{Version: WorkerProtocolVersion, Request: request, View: root, Composition: &SealedComposition{Content: content, SourceDigest: hex.EncodeToString(provenance.Sum(nil))}}
	work.Digest = sealedDigest(work)
	return s.processor.Process(ctx, work)
}

func htmlFragment(s string, text bool) string {
	if text {
		return "<p>" + s + "</p>"
	}
	start := strings.Index(s, "<body>")
	end := strings.LastIndex(s, "</body>")
	if start >= 0 && end > start {
		return s[start+len("<body>") : end]
	}
	return html.EscapeString(s)
}

func placeSVG(s string, x, y, width, height int) string {
	if !strings.HasPrefix(s, "<svg ") || !strings.HasSuffix(s, "</svg>") {
		return ""
	}
	return fmt.Sprintf("<svg x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" ", x, y, width, height) + strings.TrimPrefix(s, "<svg ")
}

func pageHeight(page reporting.CompositionPageSummary) int {
	rows := 1
	for _, widget := range page.Widgets {
		rows = max(rows, widget.Grid.Row+widget.Grid.Height)
	}
	return 48 + rows*40
}

func compositionHeight(pages []reporting.CompositionPageSummary) int {
	height := 0
	for _, page := range pages {
		height += pageHeight(page)
	}
	return max(1, height)
}
