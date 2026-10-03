package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func requestControlFixture(t *testing.T) (*DB, *requestControlAuthority, identity.Envelope, *jobs.RequestRunner, jobs.RequestTask) {
	t.Helper()
	db := requestControlTestDB(t)
	signer := newRequestControlAuthority(t)
	e := signer.envelope(t, "tenant", "actor", "session", "sources.upload", "cw.source.write:source", "cw.execution_context.use:context")
	limits := jobs.Defaults()
	limits.MaxAttempts = 2
	runner, err := jobs.NewRequestRunner(db, limits)
	if err != nil {
		t.Fatal(err)
	}
	task, err := runner.Admit(context.Background(), e, "control", jobs.RequestInput{Kind: "upload.load", Target: "source", Context: "context", InputHash: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	return db, signer, e, runner, task
}

func TestRequestControlCancellationUnderSaturation(t *testing.T) {
	db, _, e, runner, task := requestControlFixture(t)
	release := holdRequestOrdinaryPool(t, db)
	defer release()
	ctx, stop := context.WithTimeout(context.Background(), 4*time.Second)
	defer stop()
	entered := make(chan jobs.Invocation, 1)
	finished := make(chan error, 1)
	go func() {
		out, err := runner.Run(ctx, e, task, 3*time.Second, func(work context.Context, i jobs.Invocation) error {
			entered <- i
			<-work.Done() // The joined real observer must see durable cancellation.
			return work.Err()
		})
		if err == nil || out.State != "cancelled" {
			finished <- errors.New("runner did not return durable cancellation")
			return
		}
		finished <- nil
	}()
	var invocation jobs.Invocation
	select {
	case invocation = <-entered:
	case err := <-finished:
		t.Fatalf("claim failed before handler: %v", err)
	case <-ctx.Done():
		t.Fatal("claim could not enter handler")
	}
	out, err := runner.Cancel(ctx, e, task.ID) // Includes the public Inspect preflight.
	if err != nil || out.State != "cancelled" {
		t.Fatalf("public cancellation starved: state=%q err=%v", out.State, err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("request observer did not stop and join after cancellation")
	}
	if err := db.FailRequest(ctx, invocation, "attempt_failed", true, 0); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("late failure overwrote cancellation: %v", err)
	}
	err = db.requestControlTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error { _, err := requestFenceTx(ctx, tx, invocation); return err })
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("cancelled invocation passed publication fence: %v", err)
	}
	var attempt string
	err = db.requestControlTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM chartworks.operation_attempts WHERE tenant_id=$1 AND operation_id=$2 AND fence=$3`, e.Tenant(), task.ID, invocation.Lease().Fence).Scan(&attempt)
	})
	if err != nil || attempt != "cancelled" {
		t.Fatalf("cancellation did not seal matching attempt: %q %v", attempt, err)
	}
}

func TestRequestControlSignedAuthorityNegativesUnderSaturation(t *testing.T) {
	db, signer, e, runner, task := requestControlFixture(t)
	release := holdRequestOrdinaryPool(t, db)
	defer release()
	full := []string{"sources.upload", "cw.source.write:source", "cw.execution_context.use:context"}
	cases := []struct {
		name, tenant, actor, session string
		scopes                       []string
		want                         error
	}{
		{"cross_tenant", "foreign", "actor", "session", full, store.ErrNotFound},
		{"other_actor", "tenant", "other", "session", full, store.ErrNotFound},
		{"other_session", "tenant", "actor", "other", full, store.ErrNotFound},
		{"missing_action", "tenant", "actor", "session", full[1:], access.ErrForbidden},
		{"other_source", "tenant", "actor", "session", []string{"sources.upload", "cw.source.write:other", "cw.execution_context.use:context"}, access.ErrNotFound},
		{"other_context", "tenant", "actor", "session", []string{"sources.upload", "cw.source.write:source", "cw.execution_context.use:other"}, access.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			other := signer.envelope(t, tc.tenant, tc.actor, tc.session, tc.scopes...)
			if _, err := db.ClaimRequest(context.Background(), other, task.ID, "denied-owner", jobs.Defaults()); !errors.Is(err, tc.want) {
				t.Fatalf("claim widened signed authority: %v", err)
			}
			if _, err := runner.Cancel(context.Background(), other, task.ID); !errors.Is(err, tc.want) {
				t.Fatalf("cancel widened signed authority: %v", err)
			}
		})
	}
	if _, err := db.PulseRequest(context.Background(), jobs.Invocation{}, true, time.Second); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatalf("zero invocation pulse accepted: %v", err)
	}
	if err := db.FailRequest(context.Background(), jobs.Invocation{}, "attempt_failed", false, 0); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatalf("zero invocation cleanup accepted: %v", err)
	}
	current, err := runner.Inspect(context.Background(), e, task.ID)
	if err != nil || current.State != "pending" || current.Attempts != 0 {
		t.Fatalf("denied controls changed accepted work: %+v %v", current, err)
	}
}

func TestRequestControlExpiredAuthorityOnlySealsFailure(t *testing.T) {
	db, signer, e, runner, task := requestControlFixture(t)
	failure := errors.New("synthetic failure after authority expiry")
	var owned jobs.Invocation
	var release func()
	_, err := runner.Run(context.Background(), e, task, 3*time.Second, func(ctx context.Context, i jobs.Invocation) error {
		owned = i
		release = holdRequestOrdinaryPool(t, db)
		signer.now.Add(301) // Deterministic verifier clock, not elapsed sleep.
		if _, err := db.PulseRequest(ctx, i, true, time.Second); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("expired invocation renewed: %v", err)
		}
		err := db.requestControlTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error { _, err := requestFenceTx(ctx, tx, i); return err })
		if !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("expired invocation passed publication fence: %v", err)
		}
		if err := db.FailRequest(ctx, i, "authority_blocked", true, 0); err != nil {
			t.Errorf("owned failure cleanup starved: %v", err)
		}
		return failure
	})
	if release != nil {
		defer release()
	}
	if !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("expired caller retrieved terminal metadata: %v", err)
	}
	if !owned.Valid() {
		t.Fatal("expiry destroyed opaque ownership evidence")
	}
	fresh := signer.envelope(t, "tenant", "actor", "session", "sources.upload", "cw.source.write:source", "cw.execution_context.use:context")
	out, err := runner.Inspect(context.Background(), fresh, task.ID)
	if err != nil || out.State != "blocked" || out.Code != "authority_blocked" {
		t.Fatalf("fresh caller did not recover cleanup receipt: %+v %v", out, err)
	}
}

func TestRequestControlStaleFenceUnderSaturation(t *testing.T) {
	db, _, e, runner, task := requestControlFixture(t)
	failure := errors.New("synthetic abandoned handler")
	var release func()
	_, err := runner.Run(context.Background(), e, task, 4*time.Second, func(ctx context.Context, old jobs.Invocation) error {
		release = holdRequestOrdinaryPool(t, db)
		// Expire only the fixture's old lease, then reclaim through the real runner.
		// No token, actor, attempt, owner, manifest or fence is forged.
		if err := db.requestControlTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE chartworks.operations SET lease_until=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), task.ID)
			return err
		}); err != nil {
			return err
		}
		successor, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		out, err := runner.Run(successor, e, task, 2*time.Second, func(ctx context.Context, current jobs.Invocation) error {
			if current.Lease().Fence <= old.Lease().Fence {
				t.Error("reclaim did not advance fence")
			}
			if _, err := db.PulseRequest(ctx, old, true, time.Second); !errors.Is(err, store.ErrConflict) {
				t.Errorf("stale invocation renewed successor: %v", err)
			}
			if err := db.FailRequest(ctx, old, "attempt_failed", true, 0); !errors.Is(err, store.ErrConflict) {
				t.Errorf("stale failure changed successor: %v", err)
			}
			err := db.requestControlTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error { _, err := requestFenceTx(ctx, tx, old); return err })
			if !errors.Is(err, store.ErrConflict) {
				t.Errorf("stale publication fence passed: %v", err)
			}
			return failure
		})
		if !errors.Is(err, failure) || out.State != "failed" || out.Attempts != 2 {
			t.Errorf("successor final failure not retained: %+v %v", out, err)
		}
		return failure
	})
	if release != nil {
		defer release()
	}
	if err == nil {
		t.Fatal("abandoned owner reported success")
	}
	out, err := runner.Inspect(context.Background(), e, task.ID)
	if err != nil || out.State != "failed" || out.Attempts != 2 {
		t.Fatalf("stale finalizer corrupted successor: %+v %v", out, err)
	}
}

func TestRequestControlHonorsRowLockAndCallerDeadline(t *testing.T) {
	db, _, e, runner, task := requestControlFixture(t)
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2 FOR UPDATE`, e.Tenant(), task.ID); err != nil {
		t.Fatal(err)
	}
	limited, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	_, err = runner.Cancel(limited, e, task.ID)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("control bypassed row lock or caller deadline: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	out, err := runner.Inspect(ctx, e, task.ID)
	if err != nil || out.State != "pending" {
		t.Fatalf("failed cancellation did not rollback: %+v %v", out, err)
	}
	out, err = runner.Cancel(ctx, e, task.ID)
	if err != nil || out.State != "cancelled" {
		t.Fatalf("control connection did not recover after rollback: %+v %v", out, err)
	}
	if db.requestControlPool.Stat().AcquiredConns() != 0 {
		t.Fatal("control transaction leaked connection")
	}
}

func TestRequestControlNestedClaimKeepsParentFenceUnderSaturation(t *testing.T) {
	db := requestControlTestDB(t)
	signer := newRequestControlAuthority(t)
	e := signer.envelope(t, "tenant", "actor", "session", "reporting.execute", "cw.report.execute:report", "cw.block.execute:block")
	limits := jobs.Defaults()
	limits.MaxAttempts = 1
	runner, err := jobs.NewRequestRunner(db, limits)
	if err != nil {
		t.Fatal(err)
	}
	task, err := runner.Admit(context.Background(), e, "parent", jobs.RequestInput{Kind: "report.run", Target: "report", InputHash: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("synthetic nested failure")
	var release func()
	out, err := runner.Run(context.Background(), e, task, 4*time.Second, func(ctx context.Context, parent jobs.Invocation) error {
		child, err := runner.AdmitNested(ctx, parent, "child", jobs.RequestInput{Kind: "reporting.run", Target: "block", InputHash: strings.Repeat("c", 64)})
		if err != nil {
			return err
		}
		release = holdRequestOrdinaryPool(t, db)
		if _, err := db.ClaimRequest(ctx, e, child.ID, "root-owner", limits); !errors.Is(err, store.ErrConflict) {
			t.Errorf("nested request claimed as root: %v", err)
		}
		if _, err := db.ClaimNestedRequest(ctx, jobs.Invocation{}, child.ID, "child-owner", limits); !errors.Is(err, jobs.ErrAuthority) {
			t.Errorf("zero parent claimed child: %v", err)
		}
		result, err := runner.RunNested(ctx, parent, child, 2*time.Second, func(ctx context.Context, i jobs.Invocation) error {
			if state, err := db.PulseRequest(ctx, i, true, limits.Lease); err != nil || state != "running" {
				t.Errorf("nested pulse starved: %q %v", state, err)
			}
			if _, err := runner.Cancel(ctx, e, task.ID); err != nil {
				return err
			}
			if _, err := db.PulseRequest(ctx, i, true, limits.Lease); !errors.Is(err, store.ErrConflict) {
				t.Errorf("cancelled parent allowed child renewal: %v", err)
			}
			err := db.requestControlTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error { _, err := requestFenceTx(ctx, tx, i); return err })
			if !errors.Is(err, store.ErrConflict) {
				t.Errorf("cancelled parent allowed child publication: %v", err)
			}
			return failure
		})
		if err == nil || result.State != "failed" {
			t.Errorf("nested failure was not sealed: %+v %v", result, err)
		}
		return failure
	})
	if release != nil {
		defer release()
	}
	if err == nil || out.State != "cancelled" {
		t.Fatalf("parent cancellation was overwritten: %+v %v", out, err)
	}
}

func TestRequestControlReserveDoesNotAdmitOrdinaryWork(t *testing.T) {
	db, _, e, runner, task := requestControlFixture(t)
	release := holdRequestOrdinaryPool(t, db)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	_, err := runner.Admit(ctx, e, "new-admission", task.Input)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ordinary admission consumed control reserve: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 60*time.Millisecond)
	_, err = runner.Resume(ctx, e, task.ID)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ordinary re-admission consumed control reserve: %v", err)
	}
	current, err := runner.Inspect(context.Background(), e, task.ID)
	if err != nil || current.State != "pending" || current.Attempts != 0 {
		t.Fatalf("ordinary waits mutated request: %+v %v", current, err)
	}
}
