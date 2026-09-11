package postgres

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// ReplaceSchedule preserves accepted occurrence rows and changes only future
// scheduling. Replays must match the same actor/session, effect key and content.
func (d *DB) ReplaceSchedule(ctx context.Context, scope store.Scope, session, id string, expected int64, key string, request jobs.ScheduleRequest, l jobs.Limits) (out jobs.Schedule, err error) {
	if !scope.Valid() || !identity.Identifier(session) || !identity.Identifier(id) || !identity.Identifier(key) || expected < 1 || expected >= 1<<62 || request.Validate() != nil || l.Validate() != nil {
		return out, jobs.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := queueLock(ctx, tx, l); err != nil {
			return err
		}
		current, e := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2 FOR UPDATE`, scope.Tenant(), id))
		if e != nil {
			return e
		}
		if current.Revision == expected+1 {
			var same bool
			if e = tx.QueryRow(ctx, `SELECT change_key=$3 AND change_actor=$4 AND change_session=$5 AND change_revision=revision FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2`, scope.Tenant(), id, key, scope.Actor(), session).Scan(&same); e != nil {
				return e
			}
			if same && digestValue(current.Request) == digestValue(request) {
				out = current
				return nil
			}
		}
		if current.Revision != expected {
			return store.ErrConflict
		}
		var now time.Time
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return e
		}
		previous, next, e := scheduleTimes(request.Spec, now)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE chartworks.job_schedules SET revision=revision+1,request=$3,request_hash=$4,previous_due=$5,next_due=$6,change_key=$7,change_actor=$8,change_session=$9,change_revision=revision+1 WHERE tenant_id=$1 AND schedule_id=$2`, scope.Tenant(), id, request.JSON(), digestValue(request), previous, next, key, scope.Actor(), session)
		if e != nil {
			return e
		}
		current.Revision++
		current.Request, current.PreviousDue, current.NextDue = request, previous, next
		out = current
		return auditJob(ctx, tx, scope, "schedule.updated", id)
	})
	return out, err
}
