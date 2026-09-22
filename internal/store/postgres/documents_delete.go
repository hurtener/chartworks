package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.DocumentDeletionRepository = (*DB)(nil)

func documentDeleteRunIDs(ctx context.Context, tx pgx.Tx, tenant, kind, id string, lock bool) ([]string, error) {
	query := `SELECT DISTINCT c.operation_id FROM chartworks.composition_runs c
 LEFT JOIN chartworks.composition_run_pages p ON(p.tenant_id,p.operation_id)=(c.tenant_id,c.operation_id)
 WHERE c.tenant_id=$1 AND ((c.kind=$2 AND c.document_id=$3) OR ($2='report' AND p.report_id=$3))
 ORDER BY c.operation_id`
	if lock {
		query = `SELECT c.operation_id FROM chartworks.composition_runs c
 WHERE c.tenant_id=$1 AND ((c.kind=$2 AND c.document_id=$3) OR ($2='report' AND EXISTS(
  SELECT 1 FROM chartworks.composition_run_pages p WHERE (p.tenant_id,p.operation_id)=(c.tenant_id,c.operation_id) AND p.report_id=$3)))
 ORDER BY c.operation_id FOR UPDATE OF c`
	}
	rows, err := tx.Query(ctx, query, tenant, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var run string
		if err := rows.Scan(&run); err != nil {
			return nil, err
		}
		ids = append(ids, run)
	}
	return ids, rows.Err()
}

func documentDeleteSchedules(ctx context.Context, tx pgx.Tx, tenant, kind, id string, lock, all bool, allowed []string) ([]string, error) {
	if kind != "report" {
		return []string{}, nil
	}
	query := `SELECT schedule_id FROM chartworks.job_schedules
 WHERE tenant_id=$1 AND NOT retired AND request->'target'->'reporting'->>'id'=$2
 AND request->'target'->'reporting'->>'type' IN('report','saved_question')
 AND ($3::boolean OR COALESCE(schedule_id=ANY($4::text[]),false)) ORDER BY schedule_id`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := tx.Query(ctx, query, tenant, id, all, allowed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var schedule string
		if err := rows.Scan(&schedule); err != nil {
			return nil, err
		}
		ids = append(ids, schedule)
	}
	return ids, rows.Err()
}

func documentDeleteImpactTx(ctx context.Context, tx pgx.Tx, tenant, kind, id string, lock bool, scheduleAll bool, scheduleIDs []string, dashboardAll bool, dashboardIDs []string) (reporting.DocumentDeleteImpact, error) {
	out := reporting.DocumentDeleteImpact{Kind: kind, ID: id, MatchingSchedules: []string{}}
	query := `SELECT version,(SELECT count(*) FROM chartworks.document_revisions r WHERE (r.tenant_id,r.kind,r.document_id)=(h.tenant_id,h.kind,h.document_id))
 FROM chartworks.document_heads h WHERE tenant_id=$1 AND kind=$2 AND document_id=$3 AND NOT deleted`
	if lock {
		query += ` FOR UPDATE OF h`
	}
	if err := tx.QueryRow(ctx, query, tenant, kind, id).Scan(&out.Version, &out.RevisionCount); err != nil {
		return out, err
	}
	runs, err := documentDeleteRunIDs(ctx, tx, tenant, kind, id, lock)
	if err != nil {
		return out, err
	}
	out.RetainedRuns = len(runs)
	out.MatchingSchedules, err = documentDeleteSchedules(ctx, tx, tenant, kind, id, lock, scheduleAll, scheduleIDs)
	if err != nil {
		return out, err
	}
	if kind == "report" {
		err = tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.document_page_refs p JOIN chartworks.document_heads h
 ON(h.tenant_id,h.kind,h.document_id)=(p.tenant_id,p.kind,p.document_id)
 WHERE p.tenant_id=$1 AND p.report_id=$2 AND NOT h.deleted
 AND ($3::boolean OR h.document_id=ANY($4::text[]))`, tenant, id, dashboardAll, dashboardIDs).Scan(&out.DashboardLinks)
	}
	return out, err
}

// PreviewDocumentDelete reads only counts and schedule identifiers after target
// authority. Schedule IDs are returned only with their own signed read reach.
func (d *DB) PreviewDocumentDelete(ctx context.Context, e identity.Envelope, kind, id string) (out reporting.DocumentDeleteImpact, err error) {
	if err := reporting.RequireDocument(e, kind, id, reporting.Write); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	scheduleSelection, _ := access.Constrain(e, "scheduling.read", "schedule", "read")
	dashboardSelection, _ := access.Constrain(e, "reporting.read", "dashboard", "read")
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		out, err = documentDeleteImpactTx(ctx, tx, e.Tenant(), kind, id, false, scheduleSelection.All(), scheduleSelection.IDs(), dashboardSelection.All(), dashboardSelection.IDs())
		if err != nil {
			return err
		}
		return ctx.Err()
	})
	return out, err
}

func replayDocumentDeletion(ctx context.Context, tx pgx.Tx, e identity.Envelope, kind, id string, in reporting.DocumentDeleteRequest) (reporting.DocumentDeletion, bool, error) {
	var out reporting.DocumentDeletion
	var key, actor, reason string
	var schedules []byte
	err := tx.QueryRow(ctx, `SELECT deleted_version,erased_revisions,erased_runs,retired_schedules,deleted_at,deletion_key,actor_id,reason
 FROM chartworks.document_deletion_tombstones WHERE tenant_id=$1 AND kind=$2 AND document_id=$3`, e.Tenant(), kind, id).Scan(
		&out.DeletedVersion, &out.ErasedRevisions, &out.ErasedRuns, &schedules, &out.DeletedAt, &key, &actor, &reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.DocumentDeletion{}, false, nil
	}
	if err != nil {
		return reporting.DocumentDeletion{}, false, err
	}
	if key != in.Key || actor != e.User() || reason != in.Reason || out.DeletedVersion != in.ExpectedVersion+1 || json.Unmarshal(schedules, &out.RetiredSchedules) != nil {
		return reporting.DocumentDeletion{}, true, store.ErrConflict
	}
	out.Kind, out.ID = kind, id
	return out, true, nil
}

// DeleteDocument serializes with lifecycle edits, schedule changes, admissions,
// and composition checkpoints in one PostgreSQL transaction.
func (d *DB) DeleteDocument(ctx context.Context, e identity.Envelope, kind, id string, in reporting.DocumentDeleteRequest) (out reporting.DocumentDeletion, err error) {
	if err := reporting.RequireDocument(e, kind, id, reporting.Write); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	var scheduleSelection access.Selection
	if kind == "report" {
		scheduleSelection, _ = access.Constrain(e, "scheduling.write", "schedule", "write")
	}
	dashboardSelection, _ := access.Constrain(e, "reporting.read", "dashboard", "read")
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if replay, found, replayErr := replayDocumentDeletion(ctx, tx, e, kind, id, in); found || replayErr != nil {
			out = replay
			return replayErr
		}
		impact, err := documentDeleteImpactTx(ctx, tx, e.Tenant(), kind, id, true, scheduleSelection.All(), scheduleSelection.IDs(), dashboardSelection.All(), dashboardSelection.IDs())
		if errors.Is(err, pgx.ErrNoRows) {
			replay, found, replayErr := replayDocumentDeletion(ctx, tx, e, kind, id, in)
			if found || replayErr != nil {
				out = replay
				return replayErr
			}
		}
		if err != nil {
			return err
		}
		if impact.Version != in.ExpectedVersion {
			return store.ErrConflict
		}
		if kind == "report" {
			var hidden bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.job_schedules
 WHERE tenant_id=$1 AND NOT retired AND request->'target'->'reporting'->>'id'=$2
 AND request->'target'->'reporting'->>'type' IN('report','saved_question')
 AND NOT ($3::boolean OR COALESCE(schedule_id=ANY($4::text[]),false)))`, e.Tenant(), id, scheduleSelection.All(), scheduleSelection.IDs()).Scan(&hidden); err != nil {
				return err
			}
			if hidden {
				return access.ErrNotFound
			}
		}
		runs, err := documentDeleteRunIDs(ctx, tx, e.Tenant(), kind, id, true)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		schedulesJSON, _ := json.Marshal(impact.MatchingSchedules)
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.document_deletion_tombstones
 (tenant_id,kind,document_id,deletion_key,expected_version,deleted_version,actor_id,reason,erased_revisions,erased_runs,retired_schedules,deleted_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, e.Tenant(), kind, id, in.Key, in.ExpectedVersion, in.ExpectedVersion+1, e.User(), in.Reason, impact.RevisionCount, len(runs), schedulesJSON, now); err != nil {
			return err
		}
		for _, run := range runs {
			for _, query := range []string{
				`DELETE FROM chartworks.composition_run_widgets WHERE tenant_id=$1 AND operation_id=$2`,
				`DELETE FROM chartworks.composition_run_groups WHERE tenant_id=$1 AND operation_id=$2`,
				`DELETE FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`,
				`UPDATE chartworks.composition_run_pages SET summary='{}'::jsonb WHERE tenant_id=$1 AND operation_id=$2`,
				`UPDATE chartworks.composition_runs SET state='expired',code='retention_expired',complete=false,retained_bytes=0,reserved_bytes=0,finished_at=COALESCE(finished_at,clock_timestamp()) WHERE tenant_id=$1 AND operation_id=$2`,
			} {
				if _, err := tx.Exec(ctx, query, e.Tenant(), run); err != nil {
					return err
				}
			}
		}
		for _, schedule := range impact.MatchingSchedules {
			if _, err := tx.Exec(ctx, `UPDATE chartworks.job_schedules SET revision=revision+1,enabled=false,retired=true WHERE tenant_id=$1 AND schedule_id=$2 AND NOT retired`, e.Tenant(), schedule); err != nil {
				return err
			}
			scope, _ := store.NewScope(e.Tenant(), e.User())
			if err := auditJob(ctx, tx, scope, "schedule.updated", schedule); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM chartworks.document_external_refs WHERE tenant_id=$1 AND kind=$2 AND document_id=$3`, e.Tenant(), kind, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE chartworks.document_revisions SET definition='{"deleted":true}'::jsonb,origins='[]'::jsonb WHERE tenant_id=$1 AND kind=$2 AND document_id=$3`, e.Tenant(), kind, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE chartworks.document_heads SET version=version+1,archived=true,deleted=true,deleted_at=$4,draft_revision=NULL,review_revision=NULL,published_revision=NULL,updated_at=$4 WHERE tenant_id=$1 AND kind=$2 AND document_id=$3`, e.Tenant(), kind, id, now); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "document.deleted", id); err != nil {
			return err
		}
		out = reporting.DocumentDeletion{Kind: kind, ID: id, DeletedVersion: in.ExpectedVersion + 1, ErasedRevisions: impact.RevisionCount, ErasedRuns: len(runs), RetiredSchedules: append([]string(nil), impact.MatchingSchedules...), DeletedAt: now}
		sort.Strings(out.RetiredSchedules)
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.DocumentDeletion{}, err
	}
	return out, nil
}
