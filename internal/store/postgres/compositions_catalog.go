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
func (d *DB) ListCompositionArtifacts(ctx context.Context, e identity.Envelope, kind, resource, after string, limit int) (out reporting.DeliveryRunsResult, err error) {
	out = reporting.DeliveryRunsResult{Version: reporting.DeliveryVersion, Items: []reporting.DeliveryRunSummary{}}
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
		return reporting.DeliveryRunsResult{}, err
	}
	return out, nil
}

// ReadArtifactSummary selects one original artifact through the same retained
// eligibility as catalog reads. It never enters actor-private execution Inspect
// or loads manifests, SQL, result rows or narrative payloads.
func (d *DB) ReadArtifactSummary(ctx context.Context, e identity.Envelope, kind, resource, run string) (out reporting.DeliveryRunSummary, err error) {
	if (kind != "block" && kind != "report" && kind != "dashboard") || !identity.Identifier(resource) || !identity.Identifier(run) {
		return out, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	if err = access.Require(e, "reporting.read", access.Resource{Tenant: e.Tenant(), Kind: "run", ID: run, Permission: "read"}); err != nil {
		return out, err
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if kind == "block" {
			record, err := frozenReadTx(ctx, tx, e, run, false, false)
			if err != nil {
				return err
			}
			out = reporting.BlockRunSummary(record.View)
		} else {
			head, err := compositionHeadTx(ctx, tx, e, run, false, false)
			if err != nil {
				return err
			}
			view, err := compositionViewTx(ctx, tx, e, head)
			if err != nil {
				return err
			}
			out = reporting.CompositionRunSummary(view)
		}
		if out.Kind != kind || out.Target.ID != resource {
			return access.ErrNotFound
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.DeliveryRunSummary{}, err
	}
	return out, nil
}
