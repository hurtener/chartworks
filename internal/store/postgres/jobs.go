package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

const jobColumns = `operation_id,tenant_id,kind,binding_id,actor_id,initiator_id,initiator_session,status,error_code,policy_revision,due_at,window_start,window_end,cutoff,batch_limit,attempt_count,max_attempts,COALESCE(schedule_id,''),COALESCE(schedule_revision,0),manifest_hash,deleted_events,deleted_operations,dispatch_manifest`

func scanJob(row pgx.Row) (jobs.Job, error) {
	var j jobs.Job
	var raw []byte
	err := row.Scan(&j.ID, &j.Tenant, &j.Kind, &j.BindingID, &j.Executor, &j.Initiator, &j.InitiatorSession, &j.State, &j.ErrorCode, &j.PolicyRevision, &j.DueAt, &j.WindowStart, &j.WindowEnd, &j.Cutoff, &j.Batch, &j.Attempts, &j.MaxAttempts, &j.ScheduleID, &j.ScheduleRevision, &j.ManifestHash, &j.DeletedEvents, &j.DeletedOperations, &raw)
	if err != nil {
		return j, err
	}
	if len(raw) != 0 {
		var accepted jobs.Job
		if json.Unmarshal(raw, &accepted) != nil || !accepted.Valid() {
			return jobs.Job{}, store.ErrInvalid
		}
		j.Pipeline = accepted.Pipeline
		if j.Digest() != accepted.ManifestHash || j.ManifestHash != accepted.ManifestHash {
			return jobs.Job{}, store.ErrInvalid
		}
	}
	return j, nil
}
func queueLock(ctx context.Context, tx pgx.Tx, l jobs.Limits) error {
	if l.Validate() != nil {
		return jobs.ErrInvalid
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7214060601)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO chartworks.queue_limits(singleton,fingerprint) VALUES(true,$1) ON CONFLICT DO NOTHING`, l.QueueFingerprint()); err != nil {
		return err
	}
	var fingerprint string
	if err := tx.QueryRow(ctx, `SELECT fingerprint FROM chartworks.queue_limits WHERE singleton`).Scan(&fingerprint); err != nil {
		return err
	}
	if fingerprint != l.QueueFingerprint() {
		return store.ErrConflict
	}
	return nil
}

// ConfigureQueue pins shared admission bounds and rejects disagreeing replicas.
func (d *DB) ConfigureQueue(ctx context.Context, l jobs.Limits) error {
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error { return queueLock(ctx, tx, l) })
}
func queueCapacity(ctx context.Context, tx pgx.Tx, tenant string, l jobs.Limits) error {
	var global, local int
	if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE tenant_id=$1) FROM chartworks.operations WHERE dispatch_mode IN ('queued','request') AND status IN ('pending','retry','running')`, tenant).Scan(&global, &local); err != nil {
		return err
	}
	if global >= l.MaxPending || local >= l.MaxPendingPerTenant {
		return jobs.ErrBusy
	}
	return nil
}
func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func auditJob(ctx context.Context, tx pgx.Tx, scope store.Scope, action, id string) error {
	event, err := newID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,$4,$5)`, scope.Tenant(), event, scope.Actor(), action, id)
	return err
}

// AdmitJob reserves the logical key and immutable maintenance manifest in one transaction.
func (d *DB) AdmitJob(ctx context.Context, scope store.Scope, session, key string, request jobs.Submission, l jobs.Limits) (out jobs.Job, err error) {
	if !scope.Valid() || !identity.Identifier(session) || !identity.Identifier(key) || request.Validate() != nil || l.Validate() != nil {
		return out, jobs.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := queueLock(ctx, tx, l); err != nil {
			return err
		}
		var now time.Time
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		var e error
		out, e = admitJob(ctx, tx, scope, session, key, request, l, now, now, "", 0)
		return e
	})
	return out, err
}

// admitJob runs only under the transaction-wide admission lock. Request identity excludes
// server time, while the separately sealed manifest retains the first resolved time forever.
func admitJob(ctx context.Context, tx pgx.Tx, scope store.Scope, session, key string, request jobs.Submission, l jobs.Limits, due, windowStart time.Time, scheduleID string, scheduleRevision int64) (jobs.Job, error) {
	clientKey := digestValue([]string{"queued-v1", scope.Actor(), key})
	requestHash := digestValue([]any{request, l.Batch, scheduleID, scheduleRevision})
	executor := jobs.Executor(request.BindingID)
	row := tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM chartworks.operations WHERE tenant_id=$1 AND initiator_id=$2 AND kind=$4 AND client_key=$3 AND dispatch_mode='queued'`, scope.Tenant(), scope.Actor(), clientKey, request.Kind)
	existing, err := scanJob(row)
	if err == nil {
		var hash string
		if err := tx.QueryRow(ctx, `SELECT request_hash FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, scope.Tenant(), existing.ID).Scan(&hash); err != nil {
			return jobs.Job{}, err
		}
		if hash != requestHash {
			return jobs.Job{}, store.ErrConflict
		}
		if existing.State == "expired" {
			return jobs.Job{}, store.ErrExpired
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return jobs.Job{}, err
	}
	// Enforce the same overlap contract for manual and clock-driven admissions, but
	// only AFTER the replay lookup: retrying an accepted key must return its receipt.
	if scheduleID != "" {
		var overlap string
		if err := tx.QueryRow(ctx, `SELECT request->'spec'->>'overlap' FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2`, scope.Tenant(), scheduleID).Scan(&overlap); err != nil {
			return jobs.Job{}, err
		}
		if overlap == "skip" {
			active, err := activeSchedule(ctx, tx, scope.Tenant(), scheduleID)
			if err != nil {
				return jobs.Job{}, err
			}
			if active {
				return jobs.Job{}, store.ErrConflict
			}
		}
	}
	if err := queueCapacity(ctx, tx, scope.Tenant(), l); err != nil {
		return jobs.Job{}, err
	}
	var revision int64
	var days, hours int
	if err := tx.QueryRow(ctx, `SELECT p.current_revision,r.audit_days,r.operation_hours FROM chartworks.policies p JOIN chartworks.policy_revisions r ON (r.tenant_id,r.revision)=(p.tenant_id,p.current_revision) WHERE p.tenant_id=$1 FOR SHARE OF p`, scope.Tenant()).Scan(&revision, &days, &hours); err != nil {
		return jobs.Job{}, err
	}
	id, err := newID()
	if err != nil {
		return jobs.Job{}, err
	}
	due = due.UTC().Truncate(time.Microsecond)
	windowStart = windowStart.UTC().Truncate(time.Microsecond)
	j := jobs.Job{ID: id, Tenant: scope.Tenant(), Kind: request.Kind, BindingID: request.BindingID, Executor: executor, Initiator: scope.Actor(), InitiatorSession: session, State: "pending", PolicyRevision: revision, DueAt: due, WindowStart: windowStart, WindowEnd: due, Cutoff: due.Add(-time.Duration(days) * 24 * time.Hour), Batch: l.Batch, MaxAttempts: l.MaxAttempts, ScheduleID: scheduleID, ScheduleRevision: scheduleRevision, Pipeline: request.Pipeline}
	j.ManifestHash = j.Digest()
	if !j.Valid() {
		return jobs.Job{}, jobs.ErrInvalid
	}
	var dispatched, input []byte
	if j.Pipeline != nil {
		var published bool
		if err := tx.QueryRow(ctx, `SELECT published_at IS NOT NULL AND manifest_hash=$4 FROM chartworks.pipeline_versions WHERE tenant_id=$1 AND pipeline_id=$2 AND version=$3 FOR SHARE`, j.Tenant, j.Pipeline.ID, j.Pipeline.Version, j.Pipeline.Digest).Scan(&published); err != nil {
			return jobs.Job{}, err
		}
		if !published {
			return jobs.Job{}, store.ErrConflict
		}
		dispatched, _ = json.Marshal(j)
		input, _ = json.Marshal(jobs.RequestInput{Kind: jobs.PipelineKind, Target: j.Pipeline.ID, InputHash: j.Pipeline.Digest})
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.operations(tenant_id,operation_id,actor_id,kind,client_key,request_hash,policy_revision,cutoff,batch_limit,expires_at,dispatch_mode,binding_id,initiator_id,initiator_session,due_at,window_start,window_end,manifest_hash,max_attempts,next_attempt_at,schedule_id,schedule_revision,dispatch_manifest,request_manifest)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,clock_timestamp()+make_interval(hours=>$19),'queued',$10,$11,$12,$13,$14,$13,$15,$16,$13,NULLIF($17,''),NULLIF($18,0),$20,$21)`, j.Tenant, j.ID, j.Executor, j.Kind, clientKey, requestHash, j.PolicyRevision, j.Cutoff, j.Batch, j.BindingID, j.Initiator, j.InitiatorSession, j.DueAt, j.WindowStart, j.ManifestHash, j.MaxAttempts, j.ScheduleID, j.ScheduleRevision, hours, dispatched, input)
	if err != nil {
		return jobs.Job{}, err
	}
	if err = auditJob(ctx, tx, scope, "job.accepted", j.ID); err != nil {
		return jobs.Job{}, err
	}
	return j, nil
}

// ReadJob loads a queued operation only from the supplied tenant partition.
func (d *DB) ReadJob(ctx context.Context, scope store.Scope, id string) (out jobs.Job, err error) {
	if !scope.Valid() || !identity.Identifier(id) {
		return out, store.ErrScope
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		out, e = scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2 AND dispatch_mode='queued'`, scope.Tenant(), id))
		return e
	})
	return out, err
}

// ListJobs applies signed selection predicates before the result limit.
func (d *DB) ListJobs(ctx context.Context, scope store.Scope, selection access.Selection, limit int) (out []jobs.Job, err error) {
	if !scope.Valid() || selection.Tenant() != scope.Tenant() || !selection.All() && len(selection.IDs()) == 0 || limit < 1 || limit > 100 {
		return nil, store.ErrScope
	}
	out = []jobs.Job{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT `+jobColumns+` FROM chartworks.operations WHERE tenant_id=$1 AND dispatch_mode='queued' AND ($2 OR operation_id=ANY($3::text[])) ORDER BY created_at DESC,operation_id LIMIT $4`, scope.Tenant(), selection.All(), selection.IDs(), limit)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			j, e := scanJob(rows)
			if e != nil {
				return e
			}
			out = append(out, j)
		}
		return rows.Err()
	})
	return out, err
}

// CancelJob cancels pending work or a live attempt using the tenant-scoped operation row.
func (d *DB) CancelJob(ctx context.Context, scope store.Scope, id string) (out jobs.Job, err error) {
	if !scope.Valid() || !identity.Identifier(id) {
		return out, store.ErrScope
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		out, e = scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2 AND dispatch_mode='queued' FOR UPDATE`, scope.Tenant(), id))
		if e != nil {
			return e
		}
		if out.State == "cancelled" {
			return nil
		}
		if out.State != "pending" && out.State != "running" && out.State != "retry" {
			return store.ErrConflict
		}
		if _, e = tx.Exec(ctx, `UPDATE chartworks.operation_attempts SET state='cancelled',finished_at=clock_timestamp(),error_code='cancelled' WHERE tenant_id=$1 AND operation_id=$2 AND state='acquiring'`, scope.Tenant(), id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE chartworks.operations SET status='cancelled',error_code='cancelled',finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL WHERE tenant_id=$1 AND operation_id=$2`, scope.Tenant(), id); e != nil {
			return e
		}
		out.State = "cancelled"
		out.ErrorCode = "cancelled"
		return auditJob(ctx, tx, scope, "job.cancelled", id)
	})
	return out, err
}

// ClaimJob claims one eligible operation with a new fence under shared concurrency bounds.
func (d *DB) ClaimJob(ctx context.Context, owner string, l jobs.Limits) (out jobs.Lease, err error) {
	if !identity.Identifier(owner) || l.Validate() != nil {
		return out, jobs.ErrInvalid
	}
	empty := false
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if e := queueLock(ctx, tx, l); e != nil {
			return e
		}
		// A bounded maintenance sweep terminalizes exhausted/expired abandoned jobs, never executes them.
		if _, e := tx.Exec(ctx, `WITH expired AS (SELECT tenant_id,operation_id FROM chartworks.operations WHERE dispatch_mode='queued' AND status IN ('pending','retry','running') AND (lease_until IS NULL OR lease_until<=clock_timestamp()) AND (expires_at<=clock_timestamp() OR attempt_count>=max_attempts) ORDER BY next_attempt_at LIMIT 100 FOR UPDATE SKIP LOCKED)
 UPDATE chartworks.operations o SET status=CASE WHEN o.expires_at<=clock_timestamp() THEN 'expired' ELSE 'failed' END,error_code=CASE WHEN o.expires_at<=clock_timestamp() THEN 'operation_expired' ELSE 'attempts_exhausted' END,finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL FROM expired x WHERE(o.tenant_id,o.operation_id)=(x.tenant_id,x.operation_id)`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `WITH abandoned AS (SELECT a.tenant_id,a.operation_id,a.fence FROM chartworks.operation_attempts a JOIN chartworks.operations o USING(tenant_id,operation_id) WHERE a.state='acquiring' AND o.dispatch_mode='queued' AND o.status IN ('expired','failed') ORDER BY a.started_at LIMIT 100 FOR UPDATE OF a SKIP LOCKED) UPDATE chartworks.operation_attempts a SET state='abandoned',error_code='lease_lost',finished_at=clock_timestamp() FROM abandoned x WHERE(a.tenant_id,a.operation_id,a.fence)=(x.tenant_id,x.operation_id,x.fence)`); e != nil {
			return e
		}
		var active int
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.operations WHERE dispatch_mode IN ('queued','request') AND status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`).Scan(&active); e != nil {
			return e
		}
		if active >= l.GlobalConcurrency {
			empty = true
			return nil
		}
		row := tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM chartworks.operations o WHERE dispatch_mode='queued' AND status IN ('pending','retry','running') AND next_attempt_at<=clock_timestamp() AND due_at<=clock_timestamp() AND expires_at>clock_timestamp() AND attempt_count<max_attempts AND(lease_until IS NULL OR lease_until<=clock_timestamp())
 AND(SELECT count(*) FROM chartworks.operations a WHERE a.dispatch_mode IN ('queued','request') AND a.tenant_id=o.tenant_id AND a.status='running' AND a.lease_until>clock_timestamp())<$1
 AND(o.schedule_id IS NULL OR NOT EXISTS(SELECT 1 FROM chartworks.operations a WHERE a.tenant_id=o.tenant_id AND a.schedule_id=o.schedule_id AND a.operation_id<>o.operation_id AND a.status='running' AND a.lease_until>clock_timestamp()))
 ORDER BY next_attempt_at,due_at,operation_id LIMIT 1 FOR UPDATE OF o SKIP LOCKED`, l.TenantConcurrency)
		j, e := scanJob(row)
		if errors.Is(e, pgx.ErrNoRows) {
			empty = true
			return nil
		}
		if e != nil {
			return e
		}
		if !j.Valid() {
			return jobs.ErrInvalid
		}
		fence, attempt, until, e := claimOperationLease(ctx, tx, j.Tenant, j.ID, owner, l.Lease)
		if e != nil {
			return e
		}
		j.Attempts = attempt
		j.State = "running"
		out = jobs.Lease{Job: j, Owner: owner, Fence: fence, Attempt: j.Attempts, Until: until}
		return nil
	})
	if err == nil && empty {
		return out, jobs.ErrEmpty
	}
	return out, err
}

// HeartbeatJob extends only the current, unexpired owner and fence.
func (d *DB) HeartbeatJob(ctx context.Context, lease jobs.Lease, ttl time.Duration) error {
	if !lease.Job.Valid() || !identity.Identifier(lease.Owner) || lease.Fence < 1 || ttl < time.Second || ttl > time.Minute {
		return jobs.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return renewOperationLease(ctx, tx, operationLease{tenant: lease.Job.Tenant, id: lease.Job.ID, owner: lease.Owner, manifest: lease.Job.ManifestHash, mode: "queued", fence: lease.Fence}, ttl)
	})
}

// FinishAttempt records a bounded retry or terminal outcome only for the live fenced attempt.
func (d *DB) FinishAttempt(ctx context.Context, lease jobs.Lease, code string, permanent bool, delay time.Duration) error {
	if !lease.Job.Valid() || lease.Fence < 1 || !identity.Identifier(lease.Owner) || delay < 0 || delay > time.Minute {
		return jobs.ErrInvalid
	}
	switch code {
	case "authority_blocked", "attempt_failed", "attempt_timeout", "definition_changed":
	default:
		return jobs.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return failOperationLease(ctx, tx, operationLease{tenant: lease.Job.Tenant, id: lease.Job.ID, owner: lease.Owner, manifest: lease.Job.ManifestHash, mode: "queued", fence: lease.Fence}, code, permanent, delay)
	})
}

// CompleteJob proves the fresh service identity and repeats its guard inside the transaction.
// For this first consumer, queue completion, retention effects and audit are one database commit.
func (d *DB) CompleteJob(ctx context.Context, lease jobs.Lease, proof auth.Execution) (out jobs.Job, err error) {
	if jobs.AssertExecution(proof, lease.Job) != nil {
		return out, jobs.ErrAuthority
	}
	if lease.Job.Kind != jobs.MaintenanceKind {
		return out, jobs.ErrInvalid
	}
	e := proof.Envelope()
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return out, err
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var x error
		out, x = scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2 AND dispatch_mode='queued' AND status='running' AND lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp() FOR UPDATE`, e.Tenant(), lease.Job.ID, lease.Owner, lease.Fence))
		if errors.Is(x, pgx.ErrNoRows) {
			return store.ErrConflict
		}
		if x != nil {
			return x
		}
		if out.ManifestHash != lease.Job.ManifestHash || jobs.AssertExecution(proof, out) != nil {
			return jobs.ErrAuthority
		}
		var current int64
		if x = tx.QueryRow(ctx, `SELECT current_revision FROM chartworks.policies WHERE tenant_id=$1 FOR SHARE`, e.Tenant()).Scan(&current); x != nil {
			return x
		}
		if current != out.PolicyRevision {
			return store.ErrConflict
		}
		operation := store.Operation{ID: out.ID, Status: "running", PolicyRevision: out.PolicyRevision, Cutoff: out.Cutoff, Limit: out.Batch}
		if x = sweepRows(ctx, tx, scope, &operation, out.DueAt); x != nil {
			return x
		}
		if !e.Valid() {
			return jobs.ErrAuthority
		}
		tag, x := tx.Exec(ctx, `UPDATE chartworks.operations SET status='succeeded',finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL,error_code='',deleted_events=$5,deleted_operations=$6 WHERE tenant_id=$1 AND operation_id=$2 AND status='running' AND lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`, e.Tenant(), out.ID, lease.Owner, lease.Fence, operation.DeletedEvents, operation.DeletedOperations)
		if x != nil {
			return x
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		event, x := newID()
		if x != nil {
			return x
		}
		if _, x = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id,operation_id) VALUES($1,$2,$3,'retention.sweep',$4,$4)`, e.Tenant(), event, e.User(), out.ID); x != nil {
			return x
		}
		if _, x = tx.Exec(ctx, `UPDATE chartworks.operation_attempts SET state='succeeded',executor_id=$4,finished_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2 AND fence=$3 AND state='acquiring'`, e.Tenant(), out.ID, lease.Fence, e.User()); x != nil {
			return x
		}
		out.State = "succeeded"
		out.DeletedEvents = operation.DeletedEvents
		out.DeletedOperations = operation.DeletedOperations
		return nil
	})
	if err != nil {
		return jobs.Job{}, err
	}
	return out, nil
}

var _ jobs.Repository = (*DB)(nil)
