package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestJobStoreRejectsInvalidWork(t *testing.T) {
	q := newQueueFixture(t, nil)
	ctx := context.Background()
	scope := support.Scope(t, q.actor.Tenant(), q.actor.User())
	target := jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}
	scheduleRequest := jobs.ScheduleRequest{Target: target, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}
	if _, err := q.db.AdmitJob(ctx, store.Scope{}, "s", "key", target, q.limits); err == nil {
		t.Fatal("unscoped job")
	}
	if _, err := q.db.ReadJob(ctx, store.Scope{}, "x"); err == nil {
		t.Fatal("unscoped read")
	}
	if _, err := q.db.ListJobs(ctx, scope, access.Selection{}, 100); err == nil {
		t.Fatal("unconstrained list")
	}
	if _, err := q.db.CancelJob(ctx, store.Scope{}, "x"); err == nil {
		t.Fatal("unscoped cancellation")
	}
	if _, err := q.db.CancelJob(ctx, scope, "absent"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("absent cancel", err)
	}
	if _, err := q.db.CreateSchedule(ctx, scope, "session", "bad/", scheduleRequest, q.limits); err == nil {
		t.Fatal("invalid schedule key")
	}
	if _, err := q.db.ReadSchedule(ctx, store.Scope{}, "x"); err == nil {
		t.Fatal("unscoped schedule")
	}
	if _, err := q.db.SetSchedule(ctx, scope, "x", 0, false); err == nil {
		t.Fatal("unversioned schedule")
	}
	if _, err := q.db.SetSchedule(ctx, scope, "absent", 1, false); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("absent state", err)
	}
	if _, err := q.db.FireSchedule(ctx, scope, "s", "x", "bad/", q.limits); err == nil {
		t.Fatal("invalid occurrence key")
	}
	if _, err := q.db.FireSchedule(ctx, scope, "s", "absent", "key", q.limits); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("absent fire", err)
	}
	if _, err := q.db.ClaimJob(ctx, "invalid/", q.limits); err == nil {
		t.Fatal("invalid worker")
	}
	if _, err := q.db.TickSchedules(ctx, jobs.Limits{}); err == nil {
		t.Fatal("unbounded tick")
	}
	if err := q.db.ConfigureQueue(ctx, jobs.Limits{}); err == nil {
		t.Fatal("unbounded queue")
	}
	if err := q.db.HeartbeatJob(ctx, jobs.Lease{}, time.Second); err == nil {
		t.Fatal("invalid lease")
	}
	if err := q.db.FinishAttempt(ctx, jobs.Lease{}, "attempt_failed", false, 0); err == nil {
		t.Fatal("invalid attempt")
	}
	if _, err := q.db.CompleteJob(ctx, jobs.Lease{}, auth.Execution{}); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("unverified completion")
	}
	first, err := q.service.CreateSchedule(ctx, q.actor, "same", scheduleRequest)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := q.service.CreateSchedule(ctx, q.actor, "same", scheduleRequest)
	if err != nil || first.ID != replay.ID {
		t.Fatal("schedule replay")
	}
	j := q.submit(t, "lease-check")
	lease, err := q.db.ClaimJob(ctx, "owner", q.limits)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.db.HeartbeatJob(ctx, lease, time.Second); err != nil {
		t.Fatal("live heartbeat", err)
	}
	stolen := lease
	stolen.Owner = "intruder"
	if err := q.db.HeartbeatJob(ctx, stolen, time.Second); !errors.Is(err, store.ErrConflict) {
		t.Fatal("heartbeat stole lease", err)
	}
	if err := q.db.FinishAttempt(ctx, stolen, "attempt_failed", false, 0); !errors.Is(err, store.ErrConflict) {
		t.Fatal("finish stole lease", err)
	}
	if err := q.db.FinishAttempt(ctx, lease, "provider_secret_error", false, 0); err == nil {
		t.Fatal("arbitrary error persisted")
	}
	if err := q.db.FinishAttempt(ctx, lease, "attempt_failed", false, time.Minute+1); err == nil {
		t.Fatal("unbounded delay")
	}
	if err := q.db.FinishAttempt(ctx, lease, "attempt_failed", false, 0); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < q.limits.MaxAttempts; i++ {
		lease, err = q.db.ClaimJob(ctx, "owner", q.limits)
		if err != nil {
			t.Fatal(err)
		}
		if err = q.db.FinishAttempt(ctx, lease, "attempt_failed", false, 0); err != nil {
			t.Fatal(err)
		}
	}
	after, err := q.service.Get(ctx, q.actor, j.ID)
	if err != nil || after.State != "failed" || after.Attempts != q.limits.MaxAttempts {
		t.Fatal("retry exhaustion", err, after)
	}
	if _, err := q.service.Cancel(ctx, q.actor, j.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatal("terminal state changed", err)
	}
	// Queue admission pins cross-replica bounds even when an individual process is restarted.
	mismatch := q.limits
	mismatch.MaxPending++
	if _, err := q.db.AdmitJob(ctx, scope, "s", "new", target, mismatch); !errors.Is(err, store.ErrConflict) {
		t.Fatal("queue mismatch admitted", err)
	}
	if _, err := q.db.ClaimJob(ctx, "other", mismatch); !errors.Is(err, store.ErrConflict) {
		t.Fatal("queue mismatch dispatched", err)
	}
	if _, err := q.db.CreateSchedule(ctx, scope, "s", "new", scheduleRequest, mismatch); !errors.Is(err, store.ErrConflict) {
		t.Fatal("queue mismatch scheduled", err)
	}
	if _, err := q.db.FireSchedule(ctx, scope, "s", first.ID, "new", mismatch); !errors.Is(err, store.ErrConflict) {
		t.Fatal("queue mismatch fired", err)
	}
	if _, err := q.db.TickSchedules(ctx, mismatch); !errors.Is(err, store.ErrConflict) {
		t.Fatal("queue mismatch ticked", err)
	}
	unknown := support.Scope(t, "unconfigured-tenant", "operator")
	if _, err := q.db.AdmitJob(ctx, unknown, "s", "unknown", target, q.limits); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("job invented retention policy", err)
	}
}

func TestJobConcurrencyAndSkippedOccurrences(t *testing.T) {
	q := newQueueFixture(t, func(l *jobs.Limits) { l.Workers = 1; l.GlobalConcurrency = 1; l.TenantConcurrency = 1 })
	ctx := context.Background()
	raw := support.Raw(t, q.dsn)
	q.submit(t, "one")
	q.submit(t, "two")
	lease, err := q.db.ClaimJob(ctx, "one", q.limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.ClaimJob(ctx, "two", q.limits); !errors.Is(err, jobs.ErrEmpty) {
		t.Fatal("cross-replica concurrency exceeded", err)
	}
	if _, err := q.service.Cancel(ctx, q.actor, lease.Job.ID); err != nil {
		t.Fatal(err)
	}
	lease, err = q.db.ClaimJob(ctx, "two", q.limits)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.db.FinishAttempt(ctx, lease, "attempt_failed", false, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.ClaimJob(ctx, "early", q.limits); !errors.Is(err, jobs.ErrEmpty) {
		t.Fatal("backoff ignored", err)
	}
	spec := jobs.Spec{Type: "interval", IntervalSeconds: 60, Anchor: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Timezone: "UTC", Missed: "catch_up", MaxCatchUp: 2, Overlap: "skip"}
	schedule, err := q.service.CreateSchedule(ctx, q.actor, "overlap", jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.service.Fire(ctx, q.actor, schedule.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	if _, err := raw.Exec(ctx, `UPDATE chartworks.job_schedules SET next_due=$2::timestamptz,previous_due=$2::timestamptz-interval '1 minute' WHERE schedule_id=$1`, schedule.ID, due); err != nil {
		t.Fatal(err)
	}
	if n, err := q.db.TickSchedules(ctx, q.limits); err != nil || n != 0 {
		t.Fatal("overlap not skipped", n, err)
	}
	var n int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.job_occurrences WHERE schedule_id=$1 AND disposition='overlap_skipped'`, schedule.ID).Scan(&n); err != nil || n != 2 {
		t.Fatal("overlap outcome lost", n, err)
	}
}
