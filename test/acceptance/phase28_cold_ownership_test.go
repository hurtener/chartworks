package acceptance

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

type coldOwnershipHooks struct {
	reporting.RunRepository
	claim   func(context.Context, jobs.Invocation, string, config.ReportingExecution) (bool, error)
	reserve func(context.Context, jobs.Invocation, readexec.Options) error
}

func (r *coldOwnershipHooks) ClaimFrozenReuse(ctx context.Context, inv jobs.Invocation, id string, limits config.ReportingExecution) (bool, error) {
	if r.claim != nil {
		return r.claim(ctx, inv, id, limits)
	}
	return r.RunRepository.ClaimFrozenReuse(ctx, inv, id, limits)
}
func (r *coldOwnershipHooks) ReserveFrozenQuery(ctx context.Context, inv jobs.Invocation, o readexec.Options) error {
	if r.reserve != nil {
		return r.reserve(ctx, inv, o)
	}
	return r.RunRepository.ReserveFrozenQuery(ctx, inv, o)
}

// Inject process/reply-loss boundaries into the real operation/ownership store.
// Only normal runs below reach the genuine native PostgreSQL executor.
func TestFrozenColdOwnershipBoundariesPostgres(t *testing.T) {
	f := newReportingStoreFixture(t)
	db, ctx := f.f.f.db, t.Context()
	t.Run("unreserved_owner_replaced_and_old_invocation_fenced", func(t *testing.T) {
		block := f.create(t, "cold-unreserved", true).State.ID
		hooks := &coldOwnershipHooks{RunRepository: db}
		var old jobs.Invocation
		hooks.claim = func(ctx context.Context, inv jobs.Invocation, id string, limits config.ReportingExecution) (bool, error) {
			owner, err := db.ClaimFrozenReuse(ctx, inv, id, limits)
			if err != nil || !owner {
				return owner, err
			}
			old = inv
			return false, store.ErrUnavailable
		}
		crashing := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
		first := f.admit(t, crashing, block, "cold-unreserved-one", 60)
		if _, err := crashing.Run(ctx, f.execute, first.ID, false); !errors.Is(err, store.ErrUnavailable) {
			t.Fatal("claim fault was not exercised", err)
		}
		second := f.admit(t, f.runs, block, "cold-unreserved-two", 60)
		second, err := f.runs.Run(ctx, f.execute, second.ID, false)
		if err != nil || second.State != "succeeded" || len(second.QueryAttempts) != 1 {
			t.Fatal("dead unreserved owner prevented safe takeover", err, second.State)
		}
		if err = db.ReserveFrozenQuery(ctx, old, readexec.Options{Operation: first.ID, Number: 1}); !errors.Is(err, store.ErrConflict) {
			t.Fatal("old invocation acquired source after replacement", err)
		}
		var owner string
		if err = f.raw.QueryRow(ctx, `SELECT owner_operation FROM chartworks.frozen_reuse_owners WHERE tenant_id=$1 AND owner_operation=$2`, f.execute.Tenant(), second.ID).Scan(&owner); err != nil || owner != second.ID {
			t.Fatal("replacement not durable", err)
		}
	})
	t.Run("reservation_gap_blocks_retry_and_survives_erasure", func(t *testing.T) {
		block := f.create(t, "cold-reservation-gap", true).State.ID
		hooks := &coldOwnershipHooks{RunRepository: db}
		hooks.reserve = func(ctx context.Context, inv jobs.Invocation, o readexec.Options) error {
			if err := db.ReserveFrozenQuery(ctx, inv, o); err != nil {
				return err
			}
			return store.ErrUnavailable
		}
		crashing := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
		first := f.admit(t, crashing, block, "cold-reservation-gap-one", 60)
		if _, err := crashing.Run(ctx, f.execute, first.ID, false); !errors.Is(err, store.ErrUnavailable) {
			t.Fatal("reservation gap not exercised", err)
		}
		if _, err := f.runs.Run(ctx, f.execute, first.ID, true); !errors.Is(err, readexec.ErrUncertain) {
			t.Fatal("missing native journal refunded original reservation", err)
		}
		if _, err := f.raw.Exec(ctx, `UPDATE chartworks.frozen_runs SET payload_expires_at=created_at+interval '1 microsecond' WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), first.ID); err != nil {
			t.Fatal(err)
		}
		if n, err := db.ExpireFrozenArtifacts(ctx, f.execute, 100); err != nil || n != 1 {
			t.Fatal("expiry not exercised", n, err)
		}
		if _, err := f.raw.Exec(ctx, `DELETE FROM chartworks.frozen_run_attempts WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), first.ID); err != nil {
			t.Fatal(err)
		}
		second := f.admit(t, f.runs, block, "cold-reservation-gap-two", 60)
		if _, err := f.runs.Run(ctx, f.execute, second.ID, false); !errors.Is(err, readexec.ErrUncertain) {
			t.Fatal("expired/missing payload released unresolved source custody", err)
		}
		var number, journal, payload int
		var owner, settlement string
		if err := f.raw.QueryRow(ctx, `SELECT owner_operation,reservation_number,settlement,(SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND operation_id=ANY($3::text[])),(SELECT count(*) FROM chartworks.frozen_run_payloads WHERE tenant_id=$1 AND operation_id=$2) FROM chartworks.frozen_reuse_owners WHERE tenant_id=$1 AND owner_operation=$2`, f.execute.Tenant(), first.ID, []string{first.ID, second.ID}).Scan(&owner, &number, &settlement, &journal, &payload); err != nil || owner != first.ID || number != 1 || settlement != "unresolved" || journal != 0 || payload != 0 {
			t.Fatal("custody/evidence erasure mismatch", owner, number, settlement, journal, payload, err)
		}
		if _, err := f.raw.Exec(ctx, `DELETE FROM chartworks.frozen_reuse_owners WHERE tenant_id=$1 AND owner_operation=$2`, f.execute.Tenant(), first.ID); err == nil {
			t.Fatal("unresolved custody could be deleted")
		}
	})
	t.Run("successful_lost_values_are_not_replaced", func(t *testing.T) {
		block := f.create(t, "cold-success-loss", true).State.ID
		hooks := &storeFrozenHooks{RunRepository: db}
		hooks.checkpoint = func(ctx context.Context, inv jobs.Invocation, p reporting.PreparedRunWrite) (reporting.RunRecord, error) {
			w, err := p.Checked(inv)
			if err != nil {
				return reporting.RunRecord{}, err
			}
			if w.Kind == "attempt" {
				return reporting.RunRecord{}, store.ErrUnavailable
			}
			return db.CheckpointFrozenRun(ctx, inv, p)
		}
		crashing := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
		first := f.admit(t, crashing, block, "cold-success-loss-one", 60)
		if _, err := crashing.Run(ctx, f.execute, first.ID, false); !errors.Is(err, store.ErrUnavailable) {
			t.Fatal("native-success checkpoint loss not exercised", err)
		}
		second := f.admit(t, f.runs, block, "cold-success-loss-two", 60)
		if _, err := f.runs.Run(ctx, f.execute, second.ID, false); !errors.Is(err, reporting.ErrIncomplete) {
			t.Fatal("lost successful values rerun by follower", err)
		}
		if _, err := f.runs.Run(ctx, f.execute, first.ID, true); !errors.Is(err, reporting.ErrIncomplete) {
			t.Fatal("lost successful values rerun by owner", err)
		}
		var physical int
		if err := f.raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND operation_id=ANY($2::text[])`, f.execute.Tenant(), []string{first.ID, second.ID}).Scan(&physical); err != nil || physical != 1 {
			t.Fatal("successful lost values repeated native work", physical, err)
		}
	})
	t.Run("retained_owner_resume_does_not_query", func(t *testing.T) {
		block := f.create(t, "cold-retained-resume", true).State.ID
		lost := &phase28LostReply{RunRepository: db, kind: "result"}
		crashing := phase28RunService(t, f.f, f.blocks, lost, nil, config.DefaultReportingExecution())
		first := f.admit(t, crashing, block, "cold-retained-resume-one", 60)
		if _, err := crashing.Run(ctx, f.execute, first.ID, false); !errors.Is(err, store.ErrUnavailable) {
			t.Fatal("retained checkpoint loss not exercised", err)
		}
		first, err := f.runs.Run(ctx, f.execute, first.ID, true)
		if err != nil || first.State != "succeeded" || len(first.QueryAttempts) != 1 || first.Attempts != 2 {
			t.Fatal("retained owner failed resume or repeated query", err, first.State)
		}
		var ownerFence, reservationFence int64
		var completed bool
		if err = f.raw.QueryRow(ctx, `SELECT owner_fence,reservation_fence,completed FROM chartworks.frozen_reuse_owners WHERE tenant_id=$1 AND owner_operation=$2`, f.execute.Tenant(), first.ID).Scan(&ownerFence, &reservationFence, &completed); err != nil || ownerFence <= reservationFence || !completed {
			t.Fatal("resume lost original physical reservation fence", ownerFence, reservationFence, completed, err)
		}
	})
	t.Run("execute_only_follower_is_typed_and_bounded", func(t *testing.T) {
		block := f.create(t, "cold-execute-only", true).State.ID
		first := f.admit(t, f.runs, block, "cold-execute-only-one", 60)
		if _, err := f.runs.Run(ctx, f.execute, first.ID, false); err != nil {
			t.Fatal(err)
		}
		second := f.admit(t, f.runs, block, "cold-execute-only-two", 60)
		scopes := slices.DeleteFunc(phase28Scopes(f.execute.Tenant()), func(s string) bool { return s == "reporting.read" })
		e := phase27Actor(t, f.f, f.execute.User(), scopes)
		if _, err := f.runs.Run(ctx, e, second.ID, false); !errors.Is(err, reporting.ErrIncomplete) {
			t.Fatal("execute-only follower was unbounded or bypassed custody", err)
		}
	})
}

// The source really executes; only its reply is lost. Reconciliation observes
// the original PostgreSQL transaction before an explicitly requested retry.
func TestFrozenColdNativeUncertaintyPostgres(t *testing.T) {
	for _, resumeOwner := range []bool{true, false} {
		name := "settled_follower_takeover"
		if resumeOwner {
			name = "settled_owner_numbered_retry"
		}
		t.Run(name, func(t *testing.T) {
			f := newReportingStoreFixture(t)
			db, ctx := f.f.f.db, t.Context()
			block := f.create(t, "cold-native-uncertainty", true).State.ID
			native, err := readexec.NewExecutor(&lostReadReply{ExecutionAdapter: f.f.f.s, lose: true}, db, config.DefaultReadValidation())
			if err != nil {
				t.Fatal(err)
			}
			query, topics := newPhase18Service(t, f.f)
			blocks, err := reporting.New(db, topics, f.f.f.s, f.f.f.validator, native, reporting.CaptureFromQueries(query), config.DefaultReporting())
			if err != nil {
				t.Fatal(err)
			}
			runs := phase28RunService(t, f.f, blocks, db, nil, config.DefaultReportingExecution())
			first := f.admit(t, runs, block, "cold-native-uncertainty-one", 60)
			first, err = runs.Run(ctx, f.execute, first.ID, false)
			if !errors.Is(err, readexec.ErrUncertain) || len(first.QueryAttempts) != 1 || first.QueryAttempts[0].Status != "uncertain" || first.QueryAttempts[0].Remote == nil {
				t.Fatal("native reply loss not preserved", err, first.QueryAttempts)
			}
			second := f.admit(t, runs, block, "cold-native-uncertainty-two", 60)
			if _, err = runs.Run(ctx, f.execute, second.ID, false); !errors.Is(err, readexec.ErrUncertain) {
				t.Fatal("follower replaced uncertain native owner", err)
			}
			if _, err = runs.Run(ctx, f.execute, first.ID, true); !errors.Is(err, readexec.ErrUncertain) {
				t.Fatal("owner retried unresolved native work", err)
			}
			receipt, err := native.Control(ctx, f.execute, first.QueryAttempts[0].ID, false)
			if err != nil || receipt.RemoteState != "stopped" || receipt.Attempt.Status != "interrupted" {
				t.Fatal("native transaction was not definitely reconciled", err, receipt)
			}
			target := second.ID
			if resumeOwner {
				target = first.ID
			}
			result, err := runs.Run(ctx, f.execute, target, true)
			if err != nil || result.State != "succeeded" {
				t.Fatal("definitely reconciled source custody did not recover", err, result.State)
			}
			if resumeOwner && (len(result.QueryAttempts) != 2 || result.QueryAttempts[1].Number != 2) {
				t.Fatal("owner resume lost numbered physical history", result.QueryAttempts)
			}
			if !resumeOwner && (len(result.QueryAttempts) != 1 || result.QueryAttempts[0].Number != 1) {
				t.Fatal("new owner did not record its own physical attempt", result.QueryAttempts)
			}
			var physical int
			if err = f.raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND operation_id=ANY($2::text[]) AND remote_query IS NOT NULL`, f.execute.Tenant(), []string{first.ID, second.ID}).Scan(&physical); err != nil || physical != 2 {
				t.Fatal("settled recovery duplicated or lost physical attempts", physical, err)
			}
		})
	}
}

func TestFrozenColdMissingCustodyPostgres(t *testing.T) {
	f := newReportingStoreFixture(t)
	block := f.create(t, "cold-missing-custody", true).State.ID
	hooks := &coldOwnershipHooks{RunRepository: f.f.f.db}
	// A broken claimant cannot turn the source-start guard into optional fencing.
	hooks.claim = func(context.Context, jobs.Invocation, string, config.ReportingExecution) (bool, error) {
		return true, nil
	}
	runs := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
	run := f.admit(t, runs, block, "cold-missing-custody-key", 60)
	if _, err := runs.Run(t.Context(), f.execute, run.ID, false); !errors.Is(err, store.ErrConflict) {
		t.Fatal("missing custody allowed unowned source entry", err)
	}
	var physical int
	if err := f.raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), run.ID).Scan(&physical); err != nil || physical != 0 {
		t.Fatal("native journal started without custody", physical, err)
	}
	// An unexpected custody record for THIS immutable nonsharing operation is
	// different from deliberate fresh work beside a foreign matching owner.
	fresh := f.admit(t, f.runs, block, "cold-unexpected-own-custody", 0)
	if _, err := f.raw.Exec(t.Context(), `INSERT INTO chartworks.frozen_reuse_owners(tenant_id,reuse_key,private_session,owner_operation,owner_fence) VALUES($1,repeat('a',64),'',$2,1)`, f.execute.Tenant(), fresh.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runs.Run(t.Context(), f.execute, fresh.ID, false); !errors.Is(err, store.ErrConflict) {
		t.Fatal("nonsharing path abandoned its own custody", err)
	}
	if err := f.raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), fresh.ID).Scan(&physical); err != nil || physical != 0 {
		t.Fatal("unexpected own custody allowed source work", physical, err)
	}
}

func TestFrozenColdFollowerCancellationPostgres(t *testing.T) {
	f := newReportingStoreFixture(t)
	db := f.f.f.db
	block := f.create(t, "cold-follower-cancellation", true).State.ID
	hooks := &coldOwnershipHooks{RunRepository: db}
	runs := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
	first := f.admit(t, runs, block, "cold-follower-cancellation-one", 60)
	second := f.admit(t, runs, block, "cold-follower-cancellation-two", 60)
	ready, waiting, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var waitOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	followerCtx, cancelFollower := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancelFollower(); cancel(); unblock(); wg.Wait() }()
	hooks.reserve = func(ctx context.Context, inv jobs.Invocation, o readexec.Options) error {
		if err := db.ReserveFrozenQuery(ctx, inv, o); err != nil {
			return err
		}
		if o.Operation == first.ID {
			close(ready)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	hooks.claim = func(ctx context.Context, inv jobs.Invocation, id string, limits config.ReportingExecution) (bool, error) {
		owner, err := db.ClaimFrozenReuse(ctx, inv, id, limits)
		if id == second.ID && err == nil && !owner {
			waitOnce.Do(func() { close(waiting) })
		}
		return owner, err
	}
	type outcome struct {
		view reporting.RunView
		err  error
	}
	ownerDone, followerDone := make(chan outcome, 1), make(chan outcome, 1)
	wg.Add(1)
	go func() { defer wg.Done(); v, e := runs.Run(ctx, f.execute, first.ID, false); ownerDone <- outcome{v, e} }()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("owner did not reserve", ctx.Err())
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, e := runs.Run(followerCtx, f.execute, second.ID, false)
		followerDone <- outcome{v, e}
	}()
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal("follower did not wait", ctx.Err())
	}
	cancelFollower()
	select {
	case out := <-followerDone:
		if !errors.Is(out.err, context.Canceled) {
			t.Fatal("follower cancellation not observed", out.err)
		}
	case <-ctx.Done():
		t.Fatal("follower wait ignored cancellation", ctx.Err())
	}
	var live bool
	if err := f.raw.QueryRow(ctx, `SELECT status='running' FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), first.ID).Scan(&live); err != nil || !live {
		t.Fatal("follower cancelled owner operation", live, err)
	}
	unblock()
	select {
	case out := <-ownerDone:
		if out.err != nil || out.view.State != "succeeded" || len(out.view.QueryAttempts) != 1 || out.view.QueryAttempts[0].CancelRequested {
			t.Fatal("cancelled follower affected owner's native work", out.err, out.view.State)
		}
	case <-ctx.Done():
		t.Fatal("owner did not finish", ctx.Err())
	}
	var followerAttempts int
	if err := f.raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), second.ID).Scan(&followerAttempts); err != nil || followerAttempts != 0 {
		t.Fatal("cancelled follower started source work", followerAttempts, err)
	}
}

func TestFrozenColdPrivateSessionIsolationPostgres(t *testing.T) {
	f := newReportingStoreFixture(t)
	ctx := t.Context()
	block := f.create(t, "cold-private-session", false)
	if _, err := f.blocks.Validate(ctx, f.author, block.State.ID, reporting.ValidateRequest{ExpectedVersion: block.State.Version}); err != nil {
		t.Fatal(err)
	}
	request := reporting.RunRequest{Key: "cold-private-session-one", Policy: "private_preview", Reference: reporting.Reference{Revision: 1}, ReuseMaxAgeSeconds: 60}
	first, err := f.runs.Admit(ctx, f.execute, block.State.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	first, err = f.runs.Run(ctx, f.execute, first.ID, false)
	if err != nil || !first.Private || len(first.QueryAttempts) != 1 {
		t.Fatal("first private session failed", err)
	}
	claims := f.f.f.token.claims(f.execute.Tenant(), f.execute.User(), phase28Scopes(f.execute.Tenant()))
	claims["session"] = "cold-second-session"
	other, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	request.Key = "cold-private-session-two"
	second, err := f.runs.Admit(ctx, other, block.State.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err = f.runs.Run(ctx, other, second.ID, false)
	if err != nil || !second.Private || second.ReusedFrom != "" || len(second.QueryAttempts) != 1 {
		t.Fatal("private sessions shared custody or retained values", err, second.ReusedFrom)
	}
	var keys, partitions int
	if err = f.raw.QueryRow(ctx, `SELECT count(DISTINCT reuse_key),count(DISTINCT private_session) FROM chartworks.frozen_reuse_owners WHERE tenant_id=$1 AND owner_operation=ANY($2::text[])`, f.execute.Tenant(), []string{first.ID, second.ID}).Scan(&keys, &partitions); err != nil || keys != 1 || partitions != 2 {
		t.Fatal("private fixture did not exercise one canonical key in two sessions", keys, partitions, err)
	}
	if _, err = f.runs.Get(ctx, other, first.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("private origin became readable in other session", err)
	}
}

func TestFrozenColdExplicitNoReusePostgres(t *testing.T) {
	f := newReportingStoreFixture(t)
	db := f.f.f.db
	block := f.create(t, "cold-explicit-no-reuse", true).State.ID
	hooks := &coldOwnershipHooks{RunRepository: db}
	runs := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
	owner := f.admit(t, runs, block, "cold-explicit-no-reuse-owner", 60)
	ready, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	var wg sync.WaitGroup
	defer func() { cancel(); unblock(); wg.Wait() }()
	hooks.reserve = func(ctx context.Context, inv jobs.Invocation, o readexec.Options) error {
		if err := db.ReserveFrozenQuery(ctx, inv, o); err != nil {
			return err
		}
		if o.Operation == owner.ID {
			close(ready)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	done := make(chan error, 1)
	wg.Add(1)
	go func() { defer wg.Done(); _, err := runs.Run(ctx, f.execute, owner.ID, false); done <- err }()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("shared owner did not reserve", ctx.Err())
	}
	fresh := f.admit(t, f.runs, block, "cold-explicit-no-reuse-active", 0)
	fresh, err := f.runs.Run(ctx, f.execute, fresh.ID, false)
	if err != nil || fresh.State != "succeeded" || fresh.ReusedFrom != "" || len(fresh.QueryAttempts) != 1 {
		t.Fatal("active foreign custody blocked explicit fresh work", err, fresh.State)
	}
	unblock()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal("shared owner failed", err)
		}
	case <-ctx.Done():
		t.Fatal("shared owner did not finish", ctx.Err())
	}
	fresh = f.admit(t, f.runs, block, "cold-explicit-no-reuse-completed", 0)
	fresh, err = f.runs.Run(ctx, f.execute, fresh.ID, false)
	if err != nil || fresh.State != "succeeded" || fresh.ReusedFrom != "" || len(fresh.QueryAttempts) != 1 {
		t.Fatal("completed foreign custody blocked explicit fresh work", err, fresh.State)
	}
	var custodians int
	if err = f.raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.frozen_reuse_owners WHERE tenant_id=$1 AND owner_operation=$2`, f.execute.Tenant(), owner.ID).Scan(&custodians); err != nil || custodians != 1 {
		t.Fatal("nonsharing runs replaced the shared owner", custodians, err)
	}
}
