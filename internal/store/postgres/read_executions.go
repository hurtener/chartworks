package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ readexec.AttemptStore = (*DB)(nil)

// ReadCapacity lets the executor reserve independent journal/control connections.
func (d *DB) ReadCapacity() int { return int(d.pool.Config().MaxConns) }

const readAttemptColumns = `attempt_id,attempt_number,manifest,status,remote_query,remote_state,cancel_requested,created_at,deadline,finished_at,rows_returned,bytes_returned,code`

func scanRead(row pgx.Row) (a readexec.Attempt, err error) {
	var manifest, remote []byte
	err = row.Scan(&a.ID, &a.Number, &manifest, &a.Status, &remote, &a.RemoteState, &a.CancelRequested, &a.Created, &a.Deadline, &a.Finished, &a.Rows, &a.Bytes, &a.Code)
	if err != nil {
		return a, err
	}
	if json.Unmarshal(manifest, &a.Manifest) != nil || !a.Manifest.Valid() {
		return readexec.Attempt{}, store.ErrInvalid
	}
	if remote != nil {
		a.Remote = &readexec.RemoteQuery{}
		if json.Unmarshal(remote, a.Remote) != nil || !a.Remote.Valid() {
			return readexec.Attempt{}, store.ErrInvalid
		}
	}
	if a.Finished == nil && time.Now().After(a.Deadline.Add(a.Manifest.Limits.CancelGrace)) {
		a.Status = "uncertain"
		a.RemoteState = "unknown"
		a.Code = "remote_outcome_unknown"
	}
	return a, nil
}

// BeginRead serializes per-tenant admission, immutable operation identity and
// explicit physical attempt order. Same-number replay never executes a query.
func (d *DB) BeginRead(ctx context.Context, s store.Scope, a readexec.Attempt, maximum int) error {
	if !s.Valid() {
		return store.ErrScope
	}
	if !identity.Identifier(a.ID) || len(a.ID) != 32 || !a.Manifest.Valid() || a.Manifest.Receipt.Dialect == "" || a.Number < 1 || maximum < 1 || maximum > 3 || a.Number > maximum || a.Created.IsZero() || !a.Deadline.After(a.Created) || a.Deadline.Sub(a.Created) > time.Minute+time.Second {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061010))`, s.Tenant()); err != nil {
			return err
		}
		// Bounded metadata retention is independent of phase-28 result retention.
		// Uncertain remote attempts are never silently erased before reconciliation.
		if _, err := tx.Exec(ctx, `DELETE FROM chartworks.read_attempts WHERE tenant_id=$1 AND finished_at IS NOT NULL AND status<>'uncertain' AND created_at<clock_timestamp()-interval '24 hours'`, s.Tenant()); err != nil {
			return err
		}
		previous, err := scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 ORDER BY attempt_number DESC LIMIT 1`, s.Tenant(), s.Actor(), a.Manifest.Operation))
		switch {
		case err == nil:
			if comparableManifest(previous.Manifest) != comparableManifest(a.Manifest) {
				return store.ErrConflict
			}
			if a.Number <= previous.Number {
				return readexec.ErrReplay
			}
			if a.Number != previous.Number+1 {
				return store.ErrConflict
			}
			switch previous.Status {
			case "failed", "cancelled", "timed_out", "interrupted":
			default:
				return store.ErrConflict
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		case a.Number != 1:
			return store.ErrConflict
		}
		var total, actor int
		if err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE actor_id=$2) FROM chartworks.read_attempts WHERE tenant_id=$1`, s.Tenant(), s.Actor()).Scan(&total, &actor); err != nil {
			return err
		}
		if total >= 10000 || actor >= 1000 {
			return readexec.ErrLimit
		}
		manifest, _ := json.Marshal(a.Manifest)
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.read_attempts(tenant_id,actor_id,attempt_id,operation_id,attempt_number,source_id,context_id,manifest,manifest_hash,status,remote_state,created_at,deadline) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'accepted','not_issued',$10,$11)`, s.Tenant(), s.Actor(), a.ID, a.Manifest.Operation, a.Number, a.Manifest.Receipt.Source, a.Manifest.Receipt.Context, manifest, readexec.Hash(a.Manifest), a.Created, a.Deadline)
		if err != nil {
			return err
		}
		return auditJob(ctx, tx, s, "read.accepted", a.ID)
	})
}

// comparableManifest treats a retained pre-phase-14 receipt as PostgreSQL. The
// stored manifest itself remains byte-for-byte immutable and keeps its original hash.
func comparableManifest(m readexec.Manifest) string {
	if m.Receipt.Dialect == "postgres" {
		m.Receipt.Dialect = ""
	}
	return readexec.Hash(m)
}

// DispatchRead persists a stable native identity before SQL, then its acknowledgment.
func (d *DB) DispatchRead(ctx context.Context, s store.Scope, id string, q readexec.RemoteQuery, accepted bool) error {
	if !s.Valid() {
		return store.ErrScope
	}
	if !identity.Identifier(id) || !q.Valid() || q.Tag != "cw-read:"+id {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		remote, _ := json.Marshal(q)
		from, to := "accepted", "dispatching"
		var previous []byte
		if accepted {
			from, to = "dispatching", "running"
			var dialect string
			if err := tx.QueryRow(ctx, `SELECT remote_query,COALESCE(manifest#>>'{validation,dialect}','postgres') FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3 AND status='dispatching' FOR UPDATE`, s.Tenant(), s.Actor(), id).Scan(&previous, &dialect); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return readexec.ErrCancelled
				}
				return err
			}
			var submitted readexec.RemoteQuery
			if json.Unmarshal(previous, &submitted) != nil || dialect != q.Driver || !submitted.Acknowledges(q) {
				return store.ErrInvalid
			}
		}
		var tag pgconn.CommandTag
		var err error
		if accepted {
			tag, err = tx.Exec(ctx, `UPDATE chartworks.read_attempts SET status=$4,remote_query=$5,remote_state='running' WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3 AND status=$6 AND NOT cancel_requested AND deadline>clock_timestamp() AND remote_query=$8::jsonb AND (manifest#>>'{validation,dialect}'=$7 OR manifest#>>'{validation,dialect}' IS NULL AND $7='postgres')`, s.Tenant(), s.Actor(), id, to, remote, from, q.Driver, previous)
		} else {
			tag, err = tx.Exec(ctx, `UPDATE chartworks.read_attempts SET status=$4,remote_query=$5,remote_state='running' WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3 AND status=$6 AND NOT cancel_requested AND deadline>clock_timestamp() AND remote_query IS NULL AND (manifest#>>'{validation,dialect}'=$7 OR manifest#>>'{validation,dialect}' IS NULL AND $7='postgres')`, s.Tenant(), s.Actor(), id, to, remote, from, q.Driver)
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return readexec.ErrCancelled
		}
		return nil
	})
}

// GetRead loads only an actor's exact tenant record; no source or model calls occur.
func (d *DB) GetRead(ctx context.Context, s store.Scope, id string) (a readexec.Attempt, err error) {
	if !s.Valid() {
		return a, store.ErrScope
	}
	if !identity.Identifier(id) {
		return a, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		a, e = scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3`, s.Tenant(), s.Actor(), id))
		return e
	})
	if err != nil {
		return readexec.Attempt{}, err
	}
	return a, nil
}

// CancelRead records intent. Sending a backend signal is a separate observable effect.
func (d *DB) CancelRead(ctx context.Context, s store.Scope, id string) error {
	if !s.Valid() {
		return store.ErrScope
	}
	if !identity.Identifier(id) {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chartworks.read_attempts SET cancel_requested=true WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3 AND (finished_at IS NULL OR status='uncertain')`, s.Tenant(), s.Actor(), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return auditJob(ctx, tx, s, "read.cancel_requested", id)
	})
}

// FinishRead seals a content-free outcome. Uncertain reconciliation cannot invent rows.
func (d *DB) FinishRead(ctx context.Context, s store.Scope, a readexec.Attempt, reconcile bool) error {
	if !s.Valid() {
		return store.ErrScope
	}
	if !identity.Identifier(a.ID) || !a.Manifest.Valid() || a.Finished == nil || a.Rows < 0 || a.Rows > a.Manifest.Limits.Rows || a.Bytes < 0 || a.Bytes > a.Manifest.Limits.Bytes {
		return store.ErrInvalid
	}
	switch a.Status {
	case "succeeded", "empty", "truncated", "cancelled", "timed_out", "failed", "uncertain", "interrupted":
	default:
		return store.ErrInvalid
	}
	if reconcile && (a.Status != "interrupted" || a.Rows != 0 || a.Bytes != 0 || a.RemoteState != "stopped" && a.RemoteState != "not_issued") {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chartworks.read_attempts SET status=CASE WHEN cancel_requested AND $4 NOT IN ('uncertain','interrupted') THEN 'cancelled' ELSE $4 END,remote_state=$5,finished_at=$6,rows_returned=CASE WHEN cancel_requested THEN 0 ELSE $7 END,bytes_returned=CASE WHEN cancel_requested THEN 0 ELSE $8 END,code=CASE WHEN cancel_requested AND $4 NOT IN ('uncertain','interrupted') THEN 'cancelled' ELSE $9 END WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3 AND manifest_hash=$10 AND (finished_at IS NULL OR status='uncertain') AND (NOT $11 OR status='uncertain' OR deadline + $12::bigint * interval '1 microsecond'<clock_timestamp())`, s.Tenant(), s.Actor(), a.ID, a.Status, a.RemoteState, a.Finished, a.Rows, a.Bytes, a.Code, readexec.Hash(a.Manifest), reconcile, a.Manifest.Limits.CancelGrace.Microseconds())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		var committed string
		if err = tx.QueryRow(ctx, `SELECT status FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3`, s.Tenant(), s.Actor(), a.ID).Scan(&committed); err != nil {
			return err
		}
		return auditJob(ctx, tx, s, "read."+committed, a.ID)
	})
}

// GetReadOperation finds an actor's latest physical attempt for a retained logical key.
func (d *DB) GetReadOperation(ctx context.Context, s store.Scope, operation string) (a readexec.Attempt, err error) {
	if !s.Valid() {
		return a, store.ErrScope
	}
	if !identity.Identifier(operation) {
		return a, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		a, e = scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 ORDER BY attempt_number DESC LIMIT 1`, s.Tenant(), s.Actor(), operation))
		return e
	})
	if err != nil {
		return readexec.Attempt{}, err
	}
	return a, nil
}
