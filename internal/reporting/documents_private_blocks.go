package reporting

import (
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// CheckDocumentBlockReference validates immutable reference custody without
// executing validation or a query. Private references keep their original actor
// fence even if that block revision is subsequently published. Fresh data
// validation is checked separately at explicit preview admission/execution.
func CheckDocumentBlockReference(e identity.Envelope, w BlockWidget, snapshot Snapshot) error {
	if snapshot.State.Archived || snapshot.State.ID != w.Block || w.Revision > 0 && snapshot.Revision.Number != w.Revision {
		return ErrStale
	}
	if w.Policy != "private_preview" {
		if w.Digest != "" || snapshot.PublishedAt == nil {
			return ErrStale
		}
		return nil
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	if w.Revision < 1 || !hashValid(w.Digest) || snapshot.Revision.Digest != w.Digest {
		return ErrStale
	}
	if err := Require(e, w.Block, Read); err != nil {
		return err
	}
	if snapshot.Revision.Actor != e.User() {
		return access.ErrNotFound
	}
	return Require(e, w.Block, Preview)
}

// HasPrivateBlockReferences is lifecycle eligibility, never an authority hint.
// Every such reference must be explicitly replaced by a published policy before
// the containing report can enter review or publication.
func HasPrivateBlockReferences(d DocumentDefinition) bool {
	for _, canvas := range ReportCanvases(d) {
		for _, w := range canvas.Definition.Widgets {
			if w.Block != nil && w.Block.Policy == "private_preview" {
				return true
			}
		}
	}
	return false
}
