package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ jobs.NestedRequestRepository = (*DB)(nil)

func nestedRequestAuthority(parent jobs.Invocation, input jobs.RequestInput) (identity.Envelope, error) {
	p := parent.Lease().Task
	if _, nested := parent.Parent(); nested || (p.Input.Kind != "report.run" && p.Input.Kind != "dashboard.run") || input.Kind != "reporting.run" || input.Context != "" {
		return identity.Envelope{}, jobs.ErrAuthority
	}
	e, err := parent.Current(p.Input.Kind, p.Input.Target, p.Input.InputHash)
	if err != nil {
		return identity.Envelope{}, err
	}
	if err = input.Require(e); err != nil {
		return identity.Envelope{}, err
	}
	return e, nil
}

// AdmitNestedRequest requires the opaque live report owner; request input alone
// can never turn an ordinary task into a capacity-exempt child.
func (d *DB) AdmitNestedRequest(ctx context.Context, parent jobs.Invocation, key string, input jobs.RequestInput, l jobs.Limits) (jobs.RequestTask, error) {
	e, err := nestedRequestAuthority(parent, input)
	if err != nil {
		return jobs.RequestTask{}, err
	}
	return d.admitRequest(ctx, e, key, input, l, &parent)
}

// requireRequestParentTx locks parents before children, consistently with parent
// cancellation and publication. Only a matching current parent fence is accepted.
func requireRequestParentTx(ctx context.Context, tx pgx.Tx, i jobs.Invocation) error {
	l := i.Lease()
	parent, nested := i.Parent()
	expected := ""
	var fence int64
	if nested {
		if _, err := nestedRequestAuthority(parent, l.Task.Input); err != nil {
			return err
		}
		if _, err := requestFenceTx(ctx, tx, parent); err != nil {
			return err
		}
		expected = parent.Lease().Task.ID
		fence = parent.Lease().Fence
	}
	var actual string
	var actualFence int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(nested_parent,''),COALESCE(nested_fence,0) FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, l.Task.Tenant, l.Task.ID).Scan(&actual, &actualFence); err != nil {
		return err
	}
	if actual != expected || actualFence != fence {
		return jobs.ErrAuthority
	}
	return nil
}

func checkNestedReplayTx(ctx context.Context, tx pgx.Tx, task jobs.RequestTask, parent *jobs.Invocation) error {
	expected := ""
	if parent != nil {
		expected = parent.Lease().Task.ID
	}
	var actual string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(nested_parent,'') FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, task.Tenant, task.ID).Scan(&actual); err != nil {
		return err
	}
	if actual != expected {
		return store.ErrConflict
	}
	return nil
}

func prepareNestedAdmissionTx(ctx context.Context, tx pgx.Tx, parent *jobs.Invocation, input jobs.RequestInput) error {
	if parent == nil {
		return nil
	}
	if _, err := nestedRequestAuthority(*parent, input); err != nil {
		return err
	}
	task, err := requestFenceTx(ctx, tx, *parent)
	if err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.operations WHERE tenant_id=$1 AND nested_parent=$2`, task.Tenant, task.ID).Scan(&count); err != nil {
		return err
	}
	if count >= 100 {
		return jobs.ErrBusy
	}
	return nil
}

// ClaimNestedRequest transfers no authority. The parent's existing root slot
// covers exactly one sequential child, enforced by a database unique index.
func (d *DB) ClaimNestedRequest(ctx context.Context, parent jobs.Invocation, id, owner string, l jobs.Limits) (out jobs.RequestLease, err error) {
	if !identity.Identifier(id) || !identity.Identifier(owner) || l.Validate() != nil {
		return out, jobs.ErrInvalid
	}
	p := parent.Lease().Task
	e, err := parent.Current(p.Input.Kind, p.Input.Target, p.Input.InputHash)
	if err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := queueLock(ctx, tx, l); err != nil {
			return err
		}
		if _, err := requestFenceTx(ctx, tx, parent); err != nil {
			return err
		}
		task, err := readRequestTx(ctx, tx, e, id, true)
		if err != nil {
			return err
		}
		if _, err = nestedRequestAuthority(parent, task.Input); err != nil {
			return err
		}
		if task.Dispatch != nil {
			return jobs.ErrAuthority
		}
		if err = checkNestedReplayTx(ctx, tx, task, &parent); err != nil {
			return err
		}
		if !time.Now().Before(task.Expires) {
			return store.ErrExpired
		}
		var eligible bool
		if err = tx.QueryRow(ctx, `SELECT status IN('pending','retry','running') AND next_attempt_at<=clock_timestamp() AND (lease_until IS NULL OR lease_until<=clock_timestamp()) AND attempt_count<max_attempts FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), id).Scan(&eligible); err != nil {
			return err
		}
		if !eligible {
			return store.ErrConflict
		}
		fence, attempt, until, err := claimOperationLease(ctx, tx, e.Tenant(), id, owner, l.Lease)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.operations SET nested_fence=$3 WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), id, parent.Lease().Fence); err != nil {
			return err
		}
		task.State = "running"
		task.Attempts = attempt
		out = jobs.RequestLease{Task: task, Owner: owner, Fence: fence, Attempt: attempt, Until: until}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = store.ErrConflict
	}
	if err != nil {
		return jobs.RequestLease{}, err
	}
	return out, nil
}
