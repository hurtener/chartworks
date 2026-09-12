package postgres

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// CancelComposition records intent under the same operation lock as completion.
// This receipt does not claim that the native database query has already stopped.
func (d *DB) CancelComposition(ctx context.Context, e identity.Envelope, id string) (out reporting.CompositionView, err error) {
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	if !e.Has("jobs.cancel") {
		return out, access.ErrForbidden
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		task, err := readOwnedRequestTx(ctx, tx, e, id, true)
		if err != nil {
			return err
		}
		kind := "report"
		if task.Input.Kind == "dashboard.run" {
			kind = "dashboard"
		} else if task.Input.Kind != "report.run" {
			return store.ErrNotFound
		}
		if err := access.Require(e, "jobs.cancel", access.Resource{Tenant: e.Tenant(), Kind: kind, Permission: "execute", ID: task.Input.Target}); err != nil {
			return err
		}
		var private bool
		var state string
		if err := tx.QueryRow(ctx, `SELECT private,state FROM chartworks.composition_runs WHERE tenant_id=$1 AND operation_id=$2 AND actor_id=$3 AND session_id=$4 FOR UPDATE`, e.Tenant(), id, e.User(), e.Session()).Scan(&private, &state); err != nil {
			return err
		}
		if private {
			if err := reporting.RequireDocument(e, kind, task.Input.Target, reporting.Preview); err != nil {
				return err
			}
		}
		grants, err := blockGrants(e)
		if err != nil {
			return err
		}
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM chartworks.composition_run_references dep WHERE dep.tenant_id=$1 AND dep.operation_id=$2 AND dep.action='reporting.read' AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'=dep.kind AND g->>'permission'=dep.permission AND g->>'id' IN(dep.resource_id,'*')))`, e.Tenant(), id, grants).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return access.ErrNotFound
		}
		out = reporting.CompositionView{ID: id, Kind: kind, Document: task.Input.Target, Private: private, State: state, Pages: []reporting.CompositionPageSummary{}}
		if task.State == "succeeded" || state == "expired" || state == "cancelled" {
			return nil
		}
		if _, err := cancelRequestTaskTx(ctx, tx, e, task); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT group_id FROM chartworks.composition_run_groups WHERE tenant_id=$1 AND operation_id=$2 ORDER BY ordinal`, e.Tenant(), id)
		if err != nil {
			return err
		}
		groups := []string{}
		for rows.Next() {
			var group string
			if err := rows.Scan(&group); err != nil {
				rows.Close()
				return err
			}
			groups = append(groups, group)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, group := range groups {
			key := "composition:" + id + ":" + group
			clientKey := digestValue([]string{"request-v1", key})
			if _, err := tx.Exec(ctx, `UPDATE chartworks.read_attempts SET cancel_requested=true WHERE tenant_id=$1 AND actor_id=$2 AND finished_at IS NULL AND (operation_id=$3 OR operation_id IN(SELECT operation_id FROM chartworks.operations WHERE tenant_id=$1 AND actor_id=$2 AND initiator_session=$4 AND kind='reporting.run' AND client_key=$5))`, e.Tenant(), e.User(), key, e.Session(), clientKey); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE chartworks.composition_runs SET state='cancelled',code='cancelled',complete=false,reserved_bytes=0,finished_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), id); err != nil {
			return err
		}
		if err := frozenAudit(ctx, tx, e, "composition.cancelled", id); err != nil {
			return err
		}
		out.State, out.Code = "cancelled", "cancelled"
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.CompositionView{}, err
	}
	return out, nil
}

// ExpireCompositions erases composition-owned values and static renditions.
// Content-free metadata and signed-reach requirements survive as tombstones.
func (d *DB) ExpireCompositions(ctx context.Context, e identity.Envelope, limit int) (removed int64, err error) {
	if limit < 1 || limit > 100 {
		return 0, store.ErrInvalid
	}
	if err := access.Require(e, "reporting.retention", access.Tenant(e, "erase")); err != nil {
		return 0, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return 0, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT operation_id FROM chartworks.composition_runs WHERE tenant_id=$1 AND expires_at<=$2 AND state<>'expired' ORDER BY expires_at,operation_id LIMIT $3 FOR UPDATE SKIP LOCKED`, e.Tenant(), time.Now(), limit)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			for _, query := range []string{
				`DELETE FROM chartworks.composition_run_widgets WHERE tenant_id=$1 AND operation_id=$2`,
				`DELETE FROM chartworks.composition_run_groups WHERE tenant_id=$1 AND operation_id=$2`,
				`DELETE FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`,
				`UPDATE chartworks.composition_runs SET state='expired',code='retention_expired',complete=false,retained_bytes=0,reserved_bytes=0 WHERE tenant_id=$1 AND operation_id=$2`,
			} {
				if _, err := tx.Exec(ctx, query, e.Tenant(), id); err != nil {
					return err
				}
			}
			if err := frozenAudit(ctx, tx, e, "composition.expired", id); err != nil {
				return err
			}
			removed++
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}
