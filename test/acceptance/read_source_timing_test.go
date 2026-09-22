package acceptance

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
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

func TestReadSourceDurationExcludesPoolContention(t *testing.T) {
	f := newSourceFixture(t, func(c *config.Sources) { c.MaxConns = 1 })
	source := f.create(t, "sales")
	plan := f.plan(t, source, `SELECT id FROM analytics.sales ORDER BY id`)
	first := &heldReadObserver{entered: make(chan struct{}), release: make(chan struct{})}
	firstDone := make(chan error, 1)
	limits := readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 2 * time.Second, CancelGrace: time.Second, PlannerCost: 1e7}
	go func() {
		_, err := f.s.ExecuteRead(context.Background(), f.e, plan, limits, strings.Repeat("a", 32), first)
		firstDone <- err
	}()
	select {
	case <-first.entered:
	case <-time.After(time.Second):
		t.Fatal("first read did not hold the only source connection")
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(first.release) }) }
	defer release()
	x := readExecutor(t, f, nil)
	type completion struct {
		report readexec.ExecutionReport
		wall   time.Duration
		err    error
	}
	secondDone := make(chan completion, 1)
	before := f.lookups.Load()
	go func() {
		started := time.Now()
		report, err := x.Execute(context.Background(), f.e, plan, readOptions("pool-contention"))
		secondDone <- completion{report, time.Since(started), err}
	}()
	deadline := time.Now().Add(time.Second)
	for f.lookups.Load() == before && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if f.lookups.Load() == before {
		t.Fatal("second read did not reach the source pool")
	}
	// The second call has reached pool lookup, but the only physical connection
	// remains owned by the first read. This wait is service queue time.
	time.Sleep(250 * time.Millisecond)
	release()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal("first read failed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first read did not release the source connection")
	}
	select {
	case c := <-secondDone:
		if c.err != nil || c.report.Result == nil || c.report.Attempt.SourceDurationNS == nil {
			t.Fatal("contended read did not complete with physical timing", c.err, c.report.Attempt)
		}
		if c.wall-time.Duration(*c.report.Attempt.SourceDurationNS) < 200*time.Millisecond {
			t.Fatal("source timing included pool queue wait", c.wall, *c.report.Attempt.SourceDurationNS)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("contended read did not finish")
	}
}

type uncertainWatcherStore struct {
	*postgres.DB
	trip atomic.Bool
	seen chan struct{}
	once sync.Once
}

func (s *uncertainWatcherStore) GetRead(ctx context.Context, scope store.Scope, id string) (readexec.Attempt, error) {
	if s.trip.Swap(false) {
		s.once.Do(func() { close(s.seen) })
		return readexec.Attempt{Status: "uncertain"}, nil
	}
	return s.DB.GetRead(ctx, scope, id)
}

type nativeThenUncertainWatcher struct {
	readexec.ExecutionAdapter
	store *uncertainWatcherStore
}

func (a nativeThenUncertainWatcher) ExecuteRead(ctx context.Context, e identity.Envelope, p readexec.Plan, l readexec.Limits, id string, observer readexec.Observer) (readexec.NativeResult, error) {
	out, err := a.ExecutionAdapter.ExecuteRead(ctx, e, p, l, id, observer)
	if err != nil {
		return out, err
	}
	a.store.trip.Store(true)
	select {
	case <-a.store.seen:
		return out, nil
	case <-time.After(time.Second):
		return out, readexec.ErrUncertain
	}
}

func TestReadUncertainWatcherDiscardsTimingBeforeReconciliation(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "sales")
	plan := f.plan(t, source, `SELECT id FROM analytics.sales ORDER BY id`)
	repo := &uncertainWatcherStore{DB: f.db, seen: make(chan struct{})}
	x, err := readexec.NewExecutor(nativeThenUncertainWatcher{ExecutionAdapter: f.s, store: repo}, repo, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	report := executeFixture(t, x, f.e, plan, readOptions("watcher-uncertain"))
	if report.Result != nil || report.Attempt.Status != "uncertain" || report.Attempt.SourceDurationNS != nil {
		t.Fatal("unconfirmed physical read retained timing", report.Attempt)
	}
	meta := support.Raw(t, f.dsn)
	// A legacy uncertain row with stale timing must also remain reconcilable.
	if _, err := meta.Exec(t.Context(), `UPDATE chartworks.read_attempts SET source_duration_ns=123 WHERE tenant_id=$1 AND attempt_id=$2`, f.e.Tenant(), report.Attempt.ID); err != nil {
		t.Fatal(err)
	}
	control, err := x.Control(t.Context(), f.e, report.Attempt.ID, false)
	if err != nil || control.Attempt.Status != "interrupted" || control.Attempt.SourceDurationNS != nil {
		t.Fatal("uncertain read could not reconcile without stale timing", err, control)
	}
	var saved *int64
	if err := meta.QueryRow(t.Context(), `SELECT source_duration_ns FROM chartworks.read_attempts WHERE tenant_id=$1 AND attempt_id=$2`, f.e.Tenant(), report.Attempt.ID).Scan(&saved); err != nil || saved != nil {
		t.Fatal("reconciled read kept unconfirmed timing", err, saved)
	}
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
