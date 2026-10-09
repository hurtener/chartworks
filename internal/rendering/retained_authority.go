package rendering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"sort"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// These digest inputs and ordering are the existing composition provenance
// contract. Sharing them with retained authorization preserves historical bytes
// while binding every read to the exact pages and widgets originally rendered.
func compositionProvenance(root reporting.DeliveryViewResult) hash.Hash {
	provenance := sha256.New()
	wire, _ := json.Marshal(struct {
		Summary reporting.DeliveryRunSummary
		Pages   []reporting.CompositionPageSummary
	}{root.Summary, root.Pages})
	_, _ = provenance.Write(wire)
	return provenance
}

func orderedCompositionWidgets(page reporting.CompositionPageSummary) []reporting.CompositionWidgetSummary {
	widgets := append([]reporting.CompositionWidgetSummary(nil), page.Widgets...)
	sort.SliceStable(widgets, func(i, j int) bool {
		if widgets[i].Grid.Row == widgets[j].Grid.Row {
			return widgets[i].Grid.Column < widgets[j].Grid.Column
		}
		return widgets[i].Grid.Row < widgets[j].Grid.Row
	})
	return widgets
}

func appendCompositionProvenance(provenance hash.Hash, view reporting.DeliveryViewResult) {
	switch {
	case view.Text != nil:
		textSum := sha256.Sum256([]byte(view.Text.Format + "\x00" + view.Text.Text))
		_, _ = provenance.Write(textSum[:])
	case view.Output != nil:
		_, _ = provenance.Write([]byte(view.Output.RetainedDigest))
	}
}

// authorizeRendition performs only retained reads. A successful root View may
// legitimately be redacted metadata; it does not authorize a previously saved
// full composition. Each included child is rechecked through Delivery, and the
// complete current projection must match the saved immutable source digest.
func (s *Service) authorizeRendition(ctx context.Context, e identity.Envelope, request Request, sourceDigest string) (reporting.DeliveryViewResult, error) {
	if request.Full {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.options.MaxTime)
		defer cancel()
	}
	root, err := s.viewer.View(ctx, e, request.View)
	if err != nil {
		return reporting.DeliveryViewResult{}, err
	}
	// An omitted selector may now resolve to a different visible child with an
	// identical content digest. Historical ambiguous selections therefore need
	// the whole composition's current reach; explicit child selections do not.
	if root.Redacted && (request.Full || request.View.Kind != "block" && (request.View.Page == "" || request.View.Widget == "")) {
		return reporting.DeliveryViewResult{}, access.ErrNotFound
	}
	actual := ""
	if request.Full {
		if root.Redacted || len(root.Pages) == 0 {
			return reporting.DeliveryViewResult{}, access.ErrNotFound
		}
		count := 0
		for _, page := range root.Pages {
			count += len(page.Widgets)
		}
		if count > s.options.MaxWidgets {
			return reporting.DeliveryViewResult{}, reporting.ErrBudget
		}
		provenance, inputBytes := compositionProvenance(root), 0
		for _, page := range root.Pages {
			for _, widget := range orderedCompositionWidgets(page) {
				if widget.State != "completed" {
					continue
				}
				selection := request.View
				selection.Page, selection.Widget, selection.Output = page.ID, widget.ID, ""
				view, err := s.viewer.View(ctx, e, selection)
				if err != nil {
					return reporting.DeliveryViewResult{}, err
				}
				wire, _ := json.Marshal(view)
				inputBytes += len(wire)
				if inputBytes > s.options.MaxInputBytes {
					return reporting.DeliveryViewResult{}, reporting.ErrBudget
				}
				appendCompositionProvenance(provenance, view)
			}
		}
		actual = hex.EncodeToString(provenance.Sum(nil))
	} else if root.Output != nil && root.Output.State == "succeeded" {
		actual = root.Output.RetainedDigest
	}
	if actual == "" || actual != sourceDigest {
		return reporting.DeliveryViewResult{}, access.ErrNotFound
	}
	if !e.Valid() {
		return reporting.DeliveryViewResult{}, access.ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return reporting.DeliveryViewResult{}, err
	}
	return root, nil
}
