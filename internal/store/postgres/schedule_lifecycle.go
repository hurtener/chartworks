package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// RetireSchedule serializes with tick/fire/edit on the existing schedule row.
// Revision history is inserted by the same transaction's immutable trigger;
// an audit failure rolls back retirement, the revision and history together.
func (d *DB) RetireSchedule(ctx context.Context, scope store.Scope, id string, expected int64) (out jobs.Schedule, err error) {
	if !scope.Valid() || !identity.Identifier(id) || expected < 1 || expected >= 1<<62 {
		return out, jobs.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		current, e := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2 FOR UPDATE`, scope.Tenant(), id))
		if e != nil {
			return e
		}
		if current.Revision != expected || current.Retired {
			return store.ErrConflict
		}
		if _, e = tx.Exec(ctx, `UPDATE chartworks.job_schedules SET revision=revision+1,enabled=false,retired=true WHERE tenant_id=$1 AND schedule_id=$2`, scope.Tenant(), id); e != nil {
			return e
		}
		if e = auditJob(ctx, tx, scope, "schedule.updated", id); e != nil {
			return e
		}
		current.Revision++
		current.Enabled, current.Retired = false, true
		out = current
		return nil
	})
	if err != nil {
		return jobs.Schedule{}, err
	}
	return out, nil
}

// ScheduleHistory never loads SQL, artifact values, credentials or tokens. All
// predicates apply the caller's tenant before ordering and applying the limit.
func (d *DB) ScheduleHistory(ctx context.Context, scope store.Scope, id string, request jobs.ScheduleHistoryRequest) (out jobs.ScheduleHistory, err error) {
	if !scope.Valid() || !identity.Identifier(id) || !request.Valid() {
		return out, jobs.ErrInvalid
	}
	out = jobs.ScheduleHistory{ScheduleID: id, Kind: request.Kind, Revisions: []jobs.ScheduleRevision{}, Occurrences: []jobs.ScheduleOccurrence{}}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT true FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2 FOR SHARE`, scope.Tenant(), id).Scan(&exists); err != nil {
			return err
		}
		if request.Kind == "revisions" {
			return scheduleRevisionHistory(ctx, tx, scope.Tenant(), id, request, &out)
		}
		return scheduleOccurrenceHistory(ctx, tx, scope.Tenant(), id, request, &out)
	})
	if err != nil {
		return jobs.ScheduleHistory{}, err
	}
	return out, nil
}

func scheduleRevisionHistory(ctx context.Context, tx pgx.Tx, tenant, id string, request jobs.ScheduleHistoryRequest, out *jobs.ScheduleHistory) error {
	rows, err := tx.Query(ctx, `SELECT revision,enabled,retired,request,recorded_at FROM chartworks.job_schedule_revisions
 WHERE tenant_id=$1 AND schedule_id=$2 AND ($3::bigint=0 OR revision<$3) ORDER BY revision DESC LIMIT $4`, tenant, id, request.BeforeRevision, request.Limit+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var revision jobs.ScheduleRevision
		var raw []byte
		if err = rows.Scan(&revision.Revision, &revision.Enabled, &revision.Retired, &raw, &revision.RecordedAt); err != nil {
			return err
		}
		if len(raw) > 8192 || json.Unmarshal(raw, &revision.Request) != nil || revision.Request.Validate() != nil {
			return store.ErrUnavailable
		}
		out.Revisions = append(out.Revisions, revision)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(out.Revisions) > request.Limit {
		out.Revisions = out.Revisions[:request.Limit]
		out.NextBeforeRevision = out.Revisions[len(out.Revisions)-1].Revision
	}
	return nil
}

func scheduleOccurrenceHistory(ctx context.Context, tx pgx.Tx, tenant, id string, request jobs.ScheduleHistoryRequest, out *jobs.ScheduleHistory) error {
	rows, err := tx.Query(ctx, `SELECT due_at,window_start,window_end,skipped_through,disposition,COALESCE(operation_id,'') FROM chartworks.job_occurrences
 WHERE tenant_id=$1 AND schedule_id=$2 AND ($3::timestamptz IS NULL OR due_at<$3) ORDER BY due_at DESC LIMIT $4`, tenant, id, request.BeforeDue, request.Limit+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var occurrence jobs.ScheduleOccurrence
		if err = rows.Scan(&occurrence.DueAt, &occurrence.WindowStart, &occurrence.WindowEnd, &occurrence.SkippedThrough, &occurrence.Disposition, &occurrence.JobID); err != nil {
			return err
		}
		out.Occurrences = append(out.Occurrences, occurrence)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(out.Occurrences) > request.Limit {
		out.Occurrences = out.Occurrences[:request.Limit]
		cursor := out.Occurrences[len(out.Occurrences)-1].DueAt
		out.NextBeforeDue = &cursor
	}
	return nil
}

var _ jobs.ScheduleLifecycleRepository = (*DB)(nil)
