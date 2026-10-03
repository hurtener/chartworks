package acceptance

import (
	"context"
	"errors"
	"slices"
	"testing"

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
