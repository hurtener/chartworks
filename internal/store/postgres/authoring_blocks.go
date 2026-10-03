package postgres

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

var _ reporting.AuthoringBlockCopyRepository = (*DB)(nil)

// ReadBlockForAuthoringCopy is purpose-specific internal custody for a private
// copy. Source read alone cannot transfer hidden SQL into a new editable block.
// Native preview eligibility supplies full stored bytes without executing a
// preview; tenant, parent, dependencies and private actor checks remain in SQL.
// Ordinary ReadBlock(Read) continues to redact SQL before loading the payload.
func (d *DB) ReadBlockForAuthoringCopy(ctx context.Context, e identity.Envelope, id string, ref reporting.Reference) (reporting.Snapshot, error) {
	if ref.Revision < 1 || ref.Revision > 256 || ref.Draft {
		return reporting.Snapshot{}, reporting.ErrInvalid
	}
	for _, a := range []reporting.Access{reporting.Read, reporting.Preview} {
		if err := reporting.Require(e, id, a); err != nil {
			return reporting.Snapshot{}, err
		}
	}
	return d.ReadBlock(ctx, e, id, ref, reporting.Preview)
}
