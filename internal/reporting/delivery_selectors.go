package reporting

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// describeBlockSelectors uses independently authorized metadata reads only.
// These are descriptions, not accepted manifests or authority to read values.
// Floating references are resolved again, atomically, at actual run admission.
func (s *Delivery) describeBlockSelectors(ctx context.Context, e identity.Envelope, page *CompositionPageSummary, d DocumentDefinition) error {
	for index, widget := range d.Widgets {
		if widget.Block == nil {
			continue
		}
		target := &page.Widgets[index]
		block, err := s.blocks.Read(ctx, e, widget.Block.Block, Reference{Revision: widget.Block.Revision})
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, store.ErrUnavailable) || errors.Is(err, access.ErrUnauthenticated) {
				return err
			}
			target.Code = compositionFailure(err)
			continue
		}
		if block.Private || block.State.Archived {
			target.Code = "dependency_unavailable"
			continue
		}
		_, selection, err := ResolveOutputSelection(Definition{SchemaVersion: block.SchemaVersion, Metadata: block.Metadata, Outputs: block.Outputs}, widget.Block.Outputs)
		if err != nil {
			target.Code = compositionFailure(err)
			// A published floating block can lose all enabled defaults. Preserve
			// its already-authorized choices for explanation, not execution.
			// Explicit invalid requests have no resolved selection to expose.
			if widget.Block.Outputs == nil && SelectionErrorCode(err) == "output_selection_empty" && selection.Version == 2 {
				target.Selection = &selection
				target.Outputs = clone(selection.Selected)
			}
			continue
		}
		caps, err := resolveQueryLimits(s.runs.limits, 3, block.QueryLimits, widget.Block.Limits)
		if err != nil {
			return err
		}
		target.Selection, target.QueryLimits = &selection, &caps
		target.Outputs = clone(selection.Selected)
		target.Trust = clone(&block.Trust)
	}
	return nil
}
