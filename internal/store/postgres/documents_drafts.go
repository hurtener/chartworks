package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.DocumentDraftRepository = (*DB)(nil)

// ListDocumentDrafts is a separate private authoring catalog. The signed
// tenant, exact report-write selection and complete stored dependencies are
// applied in SQL before metadata projection or pagination. Pending review stays
// recoverable after the native transition clears the draft pointer. Published-only
// ListDocuments and its consumer semantics remain unchanged.
func (d *DB) ListDocumentDrafts(ctx context.Context, e identity.Envelope, after string, limit int) (out reporting.DraftList, err error) {
	out.Items = []reporting.DraftSummary{}
	if limit < 1 || limit > 100 || after != "" && !identity.Identifier(after) {
		return out, store.ErrInvalid
	}
	if !e.Valid() {
		return out, access.ErrUnauthenticated
	}
	for _, reach := range e.Reach() {
		if reach.ID == "*" {
			return out, access.ErrForbidden
		}
	}
	selection, err := access.Constrain(e, "reporting.write", "report", "write")
	if err != nil {
		return out, err
	}
	grants, err := blockGrants(e)
	if err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT h.document_id,h.version,r.revision,r.definition->'metadata',h.updated_at,
 CASE WHEN h.draft_revision IS NOT NULL THEN 'draft' ELSE 'review' END,
 COALESCE(h.draft_revision,0),COALESCE(h.review_revision,0)
 FROM chartworks.document_heads h JOIN chartworks.document_revisions r
 ON(r.tenant_id,r.kind,r.document_id,r.revision)=(h.tenant_id,h.kind,h.document_id,COALESCE(h.draft_revision,h.review_revision))
 WHERE h.tenant_id=$1 AND h.kind='report' AND h.document_id>$2 AND NOT h.archived AND NOT h.deleted
 AND ($3::boolean OR h.document_id=ANY($4::text[]))
 AND NOT EXISTS(SELECT 1 FROM chartworks.document_publications p WHERE (p.tenant_id,p.kind,p.document_id,p.revision)=(r.tenant_id,r.kind,r.document_id,r.revision))
 AND `+documentReferenceEligibility+` AND `+documentPrivateBlockEligibility("$7", "$8")+`
 ORDER BY h.document_id LIMIT $5`, e.Tenant(), after, selection.All(), selection.IDs(), limit+1, grants, e.User(), e.Has("reporting.preview"))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item reporting.DraftSummary
			var metadata []byte
			if err := rows.Scan(&item.ID, &item.Version, &item.Revision, &metadata, &item.Updated, &item.Stage, &item.DraftRevision, &item.ReviewRevision); err != nil {
				return err
			}
			if json.Unmarshal(metadata, &item.Metadata) != nil {
				return store.ErrInvalid
			}
			if len(out.Items) == limit {
				out.Next = out.Items[len(out.Items)-1].ID
				break
			}
			out.Items = append(out.Items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.DraftList{}, err
	}
	return out, nil
}
