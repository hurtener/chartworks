package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

type frozenReuseOwner struct {
	operation        string
	fence            int64
	number           int
	reservationFence int64
	settlement       string
	attempt          *string
	completed        bool
}

func frozenReusePartition(m reporting.RunManifest) string {
	if m.Private {
		return m.Session
	}
	return ""
}

// Only complete current identities can acquire shared custody. This decision
// depends on sealed input, never mutable eligibility or an arbitrary caller key.
func frozenReuseEligible(m reporting.RunManifest) bool {
	if m.ReuseMaxAge <= 0 || m.ReuseKey != reporting.ReuseIdentity(m) {
		return false
	}
	if m.NarrativePack == nil {
		for _, output := range m.Outputs {
			if output.Kind == "narrative" {
				return false
			}
		}
	}
	return true
}

// An immutable nonsharing run may execute independently of foreign custody.
// It cannot silently abandon custody already recorded for its own operation.
func frozenReuseBypassTx(ctx context.Context, tx pgx.Tx, m reporting.RunManifest) error {
	var owned bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.frozen_reuse_owners WHERE tenant_id=$1 AND owner_operation=$2)`, m.Tenant, m.ID).Scan(&owned); err != nil {
		return err
	}
	if owned {
		return store.ErrConflict
	}
	return nil
}

func frozenReuseOwnerTx(ctx context.Context, tx pgx.Tx, m reporting.RunManifest) (o frozenReuseOwner, err error) {
	err = tx.QueryRow(ctx, `SELECT owner_operation,owner_fence,reservation_number,reservation_fence,settlement,settled_attempt,completed FROM chartworks.frozen_reuse_owners WHERE tenant_id=$1 AND reuse_key=$2 AND private_session=$3 FOR UPDATE`, m.Tenant, m.ReuseKey, frozenReusePartition(m)).Scan(&o.operation, &o.fence, &o.number, &o.reservationFence, &o.settlement, &o.attempt, &o.completed)
	return o, err
}

func frozenReuseAttemptSettlement(a readexec.Attempt) string {
	if a.Finished == nil {
		return "unresolved"
	}
	switch a.Status {
	case "succeeded", "empty", "truncated":
		if a.RemoteState == "stopped" {
			return "successful"
		}
	case "failed", "cancelled", "timed_out", "interrupted":
		if a.RemoteState == "stopped" || a.RemoteState == "not_issued" {
			return "retryable"
		}
	}
	return "unresolved"
}

// Refresh only from the exact owner's numbered physical receipt. No foreign
// operation/head locks are acquired after the custody lock: owner commits take
// their own operation/head first. Missing/erased journals remain unresolved.
func frozenReuseSettleTx(ctx context.Context, tx pgx.Tx, m reporting.RunManifest, o *frozenReuseOwner) error {
	if o.number == 0 || o.settlement != "unresolved" {
		return nil
	}
	var h frozenHead
	h.view.ID = o.operation
	if err := tx.QueryRow(ctx, `SELECT actor_id,session_id,source_id,context_id,private FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2`, m.Tenant, o.operation).Scan(&h.actor, &h.session, &h.view.Source, &h.view.Context, &h.view.Private); err != nil {
		return err
	}
	a, err := scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND attempt_number=$4`, m.Tenant, h.actor, o.operation, o.number))
	if errors.Is(err, pgx.ErrNoRows) {
		var raw []byte
		err = tx.QueryRow(ctx, `SELECT receipt FROM chartworks.frozen_run_attempts WHERE tenant_id=$1 AND operation_id=$2 AND attempt_number=$3`, m.Tenant, o.operation, o.number).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err == nil && json.Unmarshal(raw, &a) != nil {
			return store.ErrInvalid
		}
	}
	if err != nil {
		return err
	}
	if !frozenAttemptMatches(h, a) || a.Number != o.number {
		return store.ErrInvalid
	}
	settlement := frozenReuseAttemptSettlement(a)
	if settlement == "unresolved" {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE chartworks.frozen_reuse_owners SET settlement=$4,settled_attempt=$5 WHERE tenant_id=$1 AND reuse_key=$2 AND private_session=$3`, m.Tenant, m.ReuseKey, frozenReusePartition(m), settlement, a.ID)
	if err == nil {
		o.settlement = settlement
		o.attempt = &a.ID
	}
	return err
}

func frozenReuseRunTx(ctx context.Context, tx pgx.Tx, inv jobs.Invocation, e identity.Envelope, id string) (reporting.RunRecord, error) {
	if id != inv.Lease().Task.ID {
		return reporting.RunRecord{}, store.ErrInvalid
	}
	if _, err := requestFenceTx(ctx, tx, inv); err != nil {
		return reporting.RunRecord{}, err
	}
	h, err := frozenReadHeadTx(ctx, tx, e, id, true, true)
	if err != nil {
		return reporting.RunRecord{}, err
	}
	if !time.Now().Before(h.view.Expires) || h.view.State == "expired" {
		return reporting.RunRecord{}, reporting.ErrExpired
	}
	r, err := frozenReadTx(ctx, tx, e, id, true, true)
	if err != nil {
		return r, err
	}
	if r.Manifest == nil {
		return r, reporting.ErrExpired
	}
	if err = reporting.RequireRunManifest(e, *r.Manifest); err != nil {
		return r, err
	}
	if err = frozenCurrentTx(ctx, tx, e, *r.Manifest); err != nil {
		return r, err
	}
	return r, nil
}

// ClaimFrozenReuse elects a durable canonical-key source owner after an ordinary
// completed-artifact cache miss. A false result means wait and retry that cache;
// it conveys no foreign identity or permission to cancel the owner's operation.
func (d *DB) ClaimFrozenReuse(ctx context.Context, inv jobs.Invocation, id string, runtime config.ReportingExecution) (owner bool, err error) {
	if runtime.Validate() != nil {
		return false, reporting.ErrInvalid
	}
	l := inv.Lease()
	e, err := inv.Current("reporting.run", l.Task.Input.Target, l.Task.Input.InputHash)
	if err != nil {
		return false, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return false, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		r, readErr := frozenReuseRunTx(ctx, tx, inv, e, id)
		if readErr != nil {
			return readErr
		}
		m := *r.Manifest
		if !frozenReuseEligible(m) {
			if readErr = frozenReuseBypassTx(ctx, tx, m); readErr != nil {
				return readErr
			}
			owner = true
			return nil
		}
		number := 0
		if len(r.View.QueryAttempts) > 0 {
			number = r.View.QueryAttempts[len(r.View.QueryAttempts)-1].Number
		}
		reservationFence := int64(0)
		settlement := "none"
		if number > 0 {
			reservationFence = l.Fence
			settlement = "unresolved"
		}
		if _, readErr = tx.Exec(ctx, `INSERT INTO chartworks.frozen_reuse_owners(tenant_id,reuse_key,private_session,owner_operation,owner_fence,reservation_number,reservation_fence,settlement) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, m.Tenant, m.ReuseKey, frozenReusePartition(m), id, l.Fence, number, reservationFence, settlement); readErr != nil {
			return readErr
		}
		o, readErr := frozenReuseOwnerTx(ctx, tx, m)
		if readErr != nil {
			return readErr
		}
		if o.operation != id {
			if !e.Has("reporting.read") {
				// Execution authority alone cannot consume another operation's
				// retained artifact. Do not wait for a read that cannot succeed.
				return reporting.ErrIncomplete
			}
			// This deliberately does not lock the foreign operation. Any old or
			// newly resumed owner still has to pass the custody row before source I/O.
			var live bool
			if readErr = tx.QueryRow(ctx, `SELECT status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp() FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, m.Tenant, o.operation).Scan(&live); readErr != nil {
				return readErr
			}
			if live {
				return nil
			}
		}
		if readErr = frozenReuseSettleTx(ctx, tx, m, &o); readErr != nil {
			return readErr
		}
		if o.operation != id && o.completed {
			// Close the completed-cache-miss/election race. The origin can finish
			// after the caller's last lookup but before this row is acquired.
			age := min(time.Duration(m.ReuseMaxAge)*time.Second, time.Duration(m.Limits.MaxReuseAge), time.Duration(runtime.MaxReuseAge))
			var reusable bool
			if readErr = tx.QueryRow(ctx, `SELECT state='succeeded' AND payload_expires_at>clock_timestamp() AND observed_at>=$3 FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2`, m.Tenant, o.operation, time.Now().Add(-age)).Scan(&reusable); readErr != nil {
				return readErr
			}
			if reusable {
				previous, candidateErr := frozenReadTx(ctx, tx, e, o.operation, false, true)
				if candidateErr != nil {
					return candidateErr
				}
				if previous.Manifest == nil || previous.Result == nil {
					return reporting.ErrIncomplete
				}
				if previous.Manifest.ReuseKey == reporting.ReuseIdentity(*previous.Manifest) {
					return nil
				}
			}
		}
		if o.number > 0 && o.settlement != "retryable" && !o.completed {
			if o.settlement == "successful" || o.settlement == "retained" {
				return reporting.ErrIncomplete
			}
			return readexec.ErrUncertain
		}
		if o.operation == id {
			_, readErr = tx.Exec(ctx, `UPDATE chartworks.frozen_reuse_owners SET owner_fence=$4 WHERE tenant_id=$1 AND reuse_key=$2 AND private_session=$3`, m.Tenant, m.ReuseKey, frozenReusePartition(m), l.Fence)
		} else {
			// A finished retained artifact may age out of reusable freshness. Unlike
			// mere expiry, its durable completion+stopped proof closes source custody.
			_, readErr = tx.Exec(ctx, `UPDATE chartworks.frozen_reuse_owners SET owner_operation=$4,owner_fence=$5,reservation_number=0,reservation_fence=0,settlement='none',settled_attempt=NULL,completed=false WHERE tenant_id=$1 AND reuse_key=$2 AND private_session=$3`, m.Tenant, m.ReuseKey, frozenReusePartition(m), id, l.Fence)
		}
		owner = readErr == nil
		return readErr
	})
	return owner, err
}

// ReserveFrozenQuery runs inside exec.WithAttemptReservation before BeginRead.
// The durable reservation closes the crash gap before native journal insertion.
// It is never cleared by timeout, lost response, missing receipt or retention.
func (d *DB) ReserveFrozenQuery(ctx context.Context, inv jobs.Invocation, options readexec.Options) (err error) {
	l := inv.Lease()
	e, err := inv.Current("reporting.run", l.Task.Input.Target, l.Task.Input.InputHash)
	if err != nil {
		return err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return err
	}
	defer stop()
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		r, readErr := frozenReuseRunTx(ctx, tx, inv, e, options.Operation)
		if readErr != nil {
			return readErr
		}
		m := *r.Manifest
		if options.Number < 1 || options.Number > 3 || options.Preview != m.Private || r.Result != nil {
			return store.ErrConflict
		}
		if !frozenReuseEligible(m) {
			return frozenReuseBypassTx(ctx, tx, m)
		}
		o, readErr := frozenReuseOwnerTx(ctx, tx, m)
		if errors.Is(readErr, pgx.ErrNoRows) {
			return store.ErrConflict
		}
		if readErr != nil {
			return readErr
		}
		if o.operation != options.Operation || o.fence != l.Fence {
			return store.ErrConflict
		}
		if readErr = frozenReuseSettleTx(ctx, tx, m, &o); readErr != nil {
			return readErr
		}
		if o.number > 0 && o.settlement != "retryable" {
			if o.settlement == "successful" || o.settlement == "retained" {
				return reporting.ErrIncomplete
			}
			return readexec.ErrUncertain
		}
		if options.Number != o.number+1 {
			return store.ErrConflict
		}
		_, readErr = tx.Exec(ctx, `UPDATE chartworks.frozen_reuse_owners SET reservation_number=$4,reservation_fence=$5,settlement='unresolved',settled_attempt=NULL,completed=false WHERE tenant_id=$1 AND reuse_key=$2 AND private_session=$3`, m.Tenant, m.ReuseKey, frozenReusePartition(m), options.Number, l.Fence)
		return readErr
	})
}

// Copy only settlement already proved against the real native journal by the
// checkpoint consumer. This content-free evidence survives payload/journal GC.
func frozenReuseCheckpointTx(ctx context.Context, tx pgx.Tx, inv jobs.Invocation, w reporting.RunWrite) error {
	if !frozenReuseEligible(w.Manifest) {
		return nil
	}
	o, err := frozenReuseOwnerTx(ctx, tx, w.Manifest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} // Pre-migration retained continuation.
	if err != nil {
		return err
	}
	if o.operation != w.Manifest.ID {
		return store.ErrConflict
	}
	if o.fence != inv.Lease().Fence {
		// An explicitly resumed owner can continue retained outputs. No source
		// reservation is released or reconstructed, and the live operation fence
		// was checked before this row was acquired.
		if o.settlement != "retained" || w.Kind == "attempt" || w.Kind == "result" {
			return store.ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.frozen_reuse_owners SET owner_fence=$4 WHERE tenant_id=$1 AND reuse_key=$2 AND private_session=$3`, w.Manifest.Tenant, w.Manifest.ReuseKey, frozenReusePartition(w.Manifest), inv.Lease().Fence); err != nil {
			return err
		}
	}
	settlement := o.settlement
	attempt := o.attempt
	completed := o.completed
	switch w.Kind {
	case "attempt", "result":
		if w.Attempt == nil || w.Attempt.Number != o.number {
			return store.ErrConflict
		}
		settlement = frozenReuseAttemptSettlement(*w.Attempt)
		attempt = nil
		if settlement != "unresolved" {
			attempt = &w.Attempt.ID
		}
		if w.Kind == "result" {
			settlement = "retained"
		}
		// A repeated receipt cannot discard proof that values were retained.
		if o.settlement == "retained" && settlement == "successful" {
			settlement = o.settlement
		}
	case "complete":
		if settlement != "retained" {
			return readexec.ErrUncertain
		}
		completed = w.Outcome == "succeeded"
	default:
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE chartworks.frozen_reuse_owners SET settlement=$4,settled_attempt=$5,completed=$6 WHERE tenant_id=$1 AND reuse_key=$2 AND private_session=$3`, w.Manifest.Tenant, w.Manifest.ReuseKey, frozenReusePartition(w.Manifest), settlement, attempt, completed)
	return err
}
