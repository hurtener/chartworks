package postgres

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.CompositionCatalog = (*DB)(nil)

// ListCompositionArtifacts filters signed parent/run/private reach before any
// protected metadata leaves SQL. It reads no manifest, result or narrative bytes
// and has no source/model dependency. Page contexts are independently redacted
// by the same projection used by ordinary retained composition reads.
func (d *DB) ListCompositionArtifacts(ctx context.Context, e identity.Envelope, kind, resource, after string, limit int) (out reporting.ReportingRunsResult, err error) {
	out = reporting.ReportingRunsResult{Version: reporting.DeliveryVersion, Items: []reporting.ReportingRunSummary{}}
	if (kind != "report" && kind != "dashboard") || resource != "" && !identity.Identifier(resource) || after != "" && !identity.Identifier(after) || limit < 1 || limit > 100 {
		return out, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	if !e.Has("reporting.read") {
		return out, access.ErrForbidden
	}
	grants, err := blockGrants(e)
	if err != nil {
		return out, err
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, queryErr := tx.Query(ctx, `SELECT c.operation_id FROM chartworks.composition_runs c
 WHERE c.tenant_id=$1 AND c.kind=$2 AND ($3='' OR c.document_id=$3) AND c.operation_id>$4
 AND ((NOT c.private AND EXISTS(SELECT 1 FROM jsonb_array_elements($5::jsonb) g
   WHERE g->>'permission'='read' AND ((g->>'kind'='run' AND g->>'id' IN(c.operation_id,'*'))
    OR (g->>'kind'=c.kind AND g->>'id' IN(c.document_id,'*')))))
 OR (c.private AND c.actor_id=$6 AND c.session_id=$7 AND $8::boolean
   AND EXISTS(SELECT 1 FROM jsonb_array_elements($5::jsonb) g WHERE g->>'kind'='run' AND g->>'permission'='read' AND g->>'id' IN(c.operation_id,'*'))
   AND EXISTS(SELECT 1 FROM jsonb_array_elements($5::jsonb) g WHERE g->>'kind'=c.kind AND g->>'permission'='preview' AND g->>'id' IN(c.document_id,'*'))))
 ORDER BY c.operation_id LIMIT $9`, e.Tenant(), kind, resource, after, grants, e.User(), e.Session(), e.Has("reporting.preview"), limit+1)
		if queryErr != nil {
			return queryErr
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if queryErr = rows.Scan(&id); queryErr != nil {
				rows.Close()
				return queryErr
			}
			ids = append(ids, id)
		}
		queryErr = rows.Err()
		rows.Close()
		if queryErr != nil {
			return queryErr
		}
		if len(ids) > limit {
			ids = ids[:limit]
			out.Next = ids[len(ids)-1]
		}
		for _, id := range ids {
			head, readErr := compositionHeadTx(ctx, tx, e, id, false, false)
			if readErr != nil {
				return readErr
			}
			view, readErr := compositionViewTx(ctx, tx, e, head)
			if readErr != nil {
				return readErr
			}
			out.Items = append(out.Items, reporting.CompositionRunSummary(view))
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.ReportingRunsResult{}, err
	}
	return out, nil
}
