package acceptance

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestJobsResumeRetentionAndIsolation(t *testing.T) {
	q := newQueueFixture(t, nil)
	ctx := context.Background()
	raw := support.Raw(t, q.dsn)
	scope := support.Scope(t, q.actor.Tenant(), q.actor.User())
	if _, err := q.db.SetPolicy(ctx, scope, 1, store.Policy{AuditDays: 7, OperationHours: 2}); err != nil {
		t.Fatal(err)
	}
	j := q.submit(t, "retention-hours")
	var hours float64
	if err := raw.QueryRow(ctx, `SELECT EXTRACT(EPOCH FROM(expires_at-created_at))/3600 FROM chartworks.operations WHERE operation_id=$1`, j.ID).Scan(&hours); err != nil || hours < 1.99 || hours > 2.01 {
		t.Fatal("queue ignored operation retention policy", hours, err)
	}
	spec := jobs.Spec{Type: "interval", Timezone: "UTC", IntervalSeconds: 60, Anchor: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Missed: "catch_up", MaxCatchUp: 2, Overlap: "queue"}
	request := jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}, Spec: spec}
	schedule, err := q.service.CreateSchedule(ctx, q.actor, "resume", request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.service.SetSchedule(ctx, q.actor, schedule.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Truncate(time.Minute).Add(-3 * time.Minute)
	if _, err := raw.Exec(ctx, `UPDATE chartworks.job_schedules SET next_due=$2::timestamptz,previous_due=$2::timestamptz-interval '1 minute' WHERE schedule_id=$1`, schedule.ID, due); err != nil {
		t.Fatal(err)
	}
	if n, err := q.db.TickSchedules(ctx, q.limits); err != nil || n != 0 {
		t.Fatal("paused scheduler advanced", err)
	}
	resumed, err := q.service.SetSchedule(ctx, q.actor, schedule.ID, 2, true)
	if err != nil || !resumed.NextDue.Equal(due) {
		t.Fatal("resume lost durable cursor", err)
	}
	if n, err := q.db.TickSchedules(ctx, q.limits); err != nil || n != 2 {
		t.Fatal("resume ignored catchup bound", n, err)
	}
	if n, err := q.db.TickSchedules(ctx, q.limits); err != nil || n != 0 {
		t.Fatal("repeated tick duplicated occurrence", n, err)
	}
	narrowed := q.token.envelope(t, q.actor.Tenant(), "reader", "scheduling.read", "cw.run.read:"+j.ID)
	if list, err := q.service.List(ctx, narrowed, 100); err != nil || len(list) != 1 || list[0].ID != j.ID {
		t.Fatal("list widened scope", err)
	}
	foreign := q.token.envelope(t, "foreign", "reader", "scheduling.read", "cw.run.read:*")
	if _, err := q.service.Get(ctx, foreign, j.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("foreign job visible", err)
	}
	if _, err := q.service.CreateSchedule(ctx, q.actor, "resume", jobs.ScheduleRequest{Target: request.Target, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("definition replay overwritten", err)
	}
	// A changed retention revision blocks the old immutable manifest before any erasure.
	q.oldAudit(t)
	if _, err := q.db.SetPolicy(ctx, scope, 2, store.Policy{AuditDays: 20, OperationHours: 2}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := q.service.RunOnce(ctx); err == nil {
			t.Fatal("outdated policy executed")
		}
	}
	if q.oldAuditCount(t) != 1 {
		t.Fatal("policy conflict erased audit")
	}
}
func TestJobsDSTAndTriggerBounds(t *testing.T) {
	spec := jobs.Spec{Type: "cron", Cron: "30 2 * * *", Timezone: "America/New_York", Missed: "skip", Overlap: "skip"}
	after := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	next, err := spec.Next(after)
	expected := time.Date(2026, 3, 9, 6, 30, 0, 0, time.UTC)
	if err != nil || !next.Equal(expected) {
		t.Fatal("nonexistent DST local time executed", next, err)
	}
	spec.Cron = "30 1 * * *"
	a, err := spec.Next(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	b, err := spec.Next(a)
	if err != nil || !b.Equal(a.Add(time.Hour)) {
		t.Fatal("repeated local time lost UTC occurrence", a, b, err)
	}
	p, err := spec.Previous(b.Add(20 * time.Minute))
	if err != nil || !p.Equal(b) {
		t.Fatal("previous DST occurrence", p, err)
	}
	for _, change := range []func(*jobs.Spec){func(s *jobs.Spec) { s.Timezone = "not/a-zone" }, func(s *jobs.Spec) { s.Type = "custom" }, func(s *jobs.Spec) { s.Cron = "@every 1s" }, func(s *jobs.Spec) { s.Cron = "61 * * * *" }, func(s *jobs.Spec) { s.MaxCatchUp = 33 }, func(s *jobs.Spec) { s.Missed = "catch_up"; s.MaxCatchUp = 0 }, func(s *jobs.Spec) { s.Overlap = "allow" }, func(s *jobs.Spec) { s.IntervalSeconds = 60 }} {
		bad := spec
		change(&bad)
		if bad.Validate() == nil {
			t.Fatal("bad recurrence accepted")
		}
	}
	manual := jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}
	if next, err := manual.Next(time.Now()); err != nil || !next.IsZero() {
		t.Fatal("manual invented clock trigger")
	}
	if _, err := manual.Previous(time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("unbounded occurrence history")
	}
}
func TestJobsObserverAndMetadataMode(t *testing.T) {
	q := newQueueFixture(t, nil)
	q.submit(t, "observed")
	q.mode.Store(1)
	var observed atomic.Int64
	s, err := jobs.New(q.db, q.provider, q.limits, func(stage string) {
		if stage != "worker_attempt" && stage != "schedule_tick" {
			t.Error("unbounded error text")
		}
		observed.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	until := time.Now().Add(2 * time.Second)
	for observed.Load() == 0 && time.Now().Before(until) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil || observed.Load() == 0 {
		t.Fatal("worker failure disappeared", err)
	}
	if _, err := jobs.New(nil, q.provider, q.limits); err == nil {
		t.Fatal("nil repository")
	}
	if _, err := jobs.New(q.db, nil, q.limits); err == nil {
		t.Fatal("nil authority")
	}
	if _, err := jobs.NewMetadata(nil, q.limits); err == nil {
		t.Fatal("nil metadata repository")
	}
	m, err := jobs.NewMetadata(q.db, q.limits)
	if err != nil {
		t.Fatal(err)
	}
	if m.DispatchEnabled() || m.Run(context.Background()) == nil {
		t.Fatal("metadata minted authority")
	}
	if _, err := m.Submit(context.Background(), q.actor, "disabled", jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}); err == nil {
		t.Fatal("metadata admitted work")
	}
}
