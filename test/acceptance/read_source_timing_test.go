package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// The deliberately slow journal makes it impossible for an attempt's wall
// duration to masquerade as the source-only receipt.
type slowReadJournal struct{ *postgres.DB }

func (s slowReadJournal) BeginRead(ctx context.Context, scope store.Scope, a readexec.Attempt, maximum int) error {
	time.Sleep(40 * time.Millisecond)
	return s.DB.BeginRead(ctx, scope, a, maximum)
}

func (s slowReadJournal) FinishRead(ctx context.Context, scope store.Scope, a readexec.Attempt, reconcile bool) error {
	time.Sleep(40 * time.Millisecond)
	return s.DB.FinishRead(ctx, scope, a, reconcile)
}

func (s slowReadJournal) DispatchRead(ctx context.Context, scope store.Scope, id string, q readexec.RemoteQuery, accepted bool) error {
	time.Sleep(40 * time.Millisecond)
	return s.DB.DispatchRead(ctx, scope, id, q, accepted)
}

func TestReadSourceDurationIsPhysicalAndDurable(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	plan := f.plan(t, source, `SELECT id FROM analytics.sales ORDER BY id`)
	x, err := readexec.NewExecutor(f.s, slowReadJournal{f.db}, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	started := time.Now()
	report, err := x.Execute(ctx, f.e, plan, readOptions("source-duration"))
	wall := time.Since(started)
	if err != nil || report.Result == nil || report.Attempt.Status != "succeeded" || report.Attempt.SourceDurationNS == nil || *report.Attempt.SourceDurationNS <= 0 {
		t.Fatal("native read did not produce a positive source receipt", err, report.Attempt)
	}
	if wall-time.Duration(*report.Attempt.SourceDurationNS) < 130*time.Millisecond {
		t.Fatal("journal admission, dispatch or finalization was included in native source timing", wall, *report.Attempt.SourceDurationNS)
	}
	meta := support.Raw(t, f.dsn)
	var saved *int64
	if err := meta.QueryRow(ctx, `SELECT source_duration_ns FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3`, f.e.Tenant(), f.e.User(), report.Attempt.ID).Scan(&saved); err != nil || saved == nil || *saved != *report.Attempt.SourceDurationNS {
		t.Fatal("exact physical attempt did not retain source timing", err, saved)
	}
	inspected, err := x.Inspect(ctx, f.e, report.Attempt.ID)
	if err != nil || inspected.SourceDurationNS == nil || *inspected.SourceDurationNS != *saved {
		t.Fatal("durable receipt lost source timing", err, inspected)
	}
	physical := f.lookups.Load()
	if _, err := x.Execute(ctx, f.e, plan, readOptions("source-duration")); !errors.Is(err, readexec.ErrReplay) || f.lookups.Load() != physical {
		t.Fatal("replay reached physical source work", err)
	}
	var attempts int
	if err := meta.QueryRow(ctx, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND operation_id='source-duration'`, f.e.Tenant()).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("replay created another physical attempt", err, attempts)
	}
	if _, err := f.s.Rotate(ctx, f.e, source.ID, source.Revision); err != nil {
		t.Fatal(err)
	}
	stale, err := x.Execute(ctx, f.e, plan, readOptions("source-duration-stale"))
	if err != nil || stale.Result != nil || stale.Attempt.SourceDurationNS != nil || stale.Attempt.Remote != nil {
		t.Fatal("pre-source rejection fabricated timing or physical call", err, stale.Attempt)
	}
	var unknown *int64
	if err := meta.QueryRow(ctx, `SELECT source_duration_ns FROM chartworks.read_attempts WHERE tenant_id=$1 AND attempt_id=$2`, f.e.Tenant(), stale.Attempt.ID).Scan(&unknown); err != nil || unknown != nil {
		t.Fatal("unknown source duration became zero or non-null", err, unknown)
	}
}
