package acceptance

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
)

// Count calls while forwarding every execution to the real native reader.
type coldReuseExecutor struct {
	reporting.Executor
	calls atomic.Int32
}

func (x *coldReuseExecutor) Execute(ctx context.Context, e identity.Envelope, p readexec.Plan, o readexec.Options) (readexec.ExecutionReport, error) {
	x.calls.Add(1)
	return x.Executor.Execute(ctx, e, p, o)
}

// Unlike warm concurrent reuse, both operations cross a real cache miss before
// either may enter the source executor. No successful origin is seeded.
func TestFrozenConcurrentColdReusePostgres(t *testing.T) {
	f := newReportingStoreFixture(t)
	query, topics := newPhase18Service(t, f.f)
	x := &coldReuseExecutor{Executor: f.f.f.executor}
	var err error
	f.blocks, err = reporting.New(f.f.f.db, topics, f.f.f.s, f.f.f.validator, x, reporting.CaptureFromQueries(query), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	block := f.create(t, "cold-reuse-block", true).State.ID
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	const peers = 2
	misses, release := make(chan string, peers), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	repo := &storeFrozenHooks{RunRepository: f.f.f.db}
	var gated sync.Map
	repo.reuse = func(ctx context.Context, inv jobs.Invocation, id string) (reporting.RunRecord, bool, error) {
		record, reused, reuseErr := f.f.f.db.ReuseFrozenRun(ctx, inv, id, config.DefaultReportingExecution())
		if _, seen := gated.LoadOrStore(id, true); !seen && reuseErr == nil && !reused {
			misses <- id
			select {
			case <-release:
			case <-ctx.Done():
				return reporting.RunRecord{}, false, ctx.Err()
			}
		}
		return record, reused, reuseErr
	}
	runs := phase28RunService(t, f.f, f.blocks, repo, nil, config.DefaultReportingExecution())
	views := []reporting.RunView{
		f.admit(t, runs, block, "cold-reuse-one", 60),
		f.admit(t, runs, block, "cold-reuse-two", 60),
	}
	var key string
	for _, view := range views {
		record, readErr := f.f.f.db.ReadFrozenRun(ctx, f.execute, view.ID, true)
		if readErr != nil || record.Manifest == nil || record.Result != nil || len(record.View.QueryAttempts) != 0 || record.View.State != "pending" {
			t.Fatal("cohort must begin cold with sealed distinct runs", readErr, view.ID)
		}
		if record.Manifest.ReuseKey != reporting.ReuseIdentity(*record.Manifest) || key != "" && key != record.Manifest.ReuseKey {
			t.Fatal("cold peers do not share one canonical reuse identity")
		}
		key = record.Manifest.ReuseKey
	}
	if views[0].ID == views[1].ID {
		t.Fatal("cold peers collapsed into request replay")
	}
	beforeSource, beforeModels := x.calls.Load(), f.f.model.requests.Load()
	errs := make([]error, peers)
	var wg sync.WaitGroup
	for i := range views {
		wg.Add(1)
		go func() {
			defer wg.Done()
			views[i], errs[i] = runs.Run(ctx, f.execute, views[i].ID, false)
		}()
	}
	defer func() { cancel(); unblock(); wg.Wait() }()
	seenMisses := map[string]bool{}
	for range peers {
		select {
		case id := <-misses:
			seenMisses[id] = true
		case <-ctx.Done():
			t.Fatal("both cold operations did not reach the real reuse miss", ctx.Err())
		}
	}
	if len(seenMisses) != peers || x.calls.Load() != beforeSource {
		t.Fatal("source work started before both cold misses were established")
	}
	unblock()
	wg.Wait()
	var physical, owners int
	var origin reporting.RunView
	for i, view := range views {
		if errs[i] != nil || view.State != "succeeded" {
			t.Fatalf("cold peer %d did not complete: state=%s err=%v", i, view.State, errs[i])
		}
		physical += len(view.QueryAttempts)
		if view.ReusedFrom == "" {
			owners++
			origin = view
		}
	}
	var journaled int
	if err = f.raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND operation_id=ANY($2::text[]) AND remote_query IS NOT NULL`, f.execute.Tenant(), []string{views[0].ID, views[1].ID}).Scan(&journaled); err != nil {
		t.Fatal(err)
	}
	if calls := x.calls.Load() - beforeSource; calls != 1 || physical != 1 || journaled != 1 || owners != 1 {
		t.Fatalf("concurrent cold reuse duplicated native work: executor_calls=%d physical_receipts=%d native_journal=%d origins=%d; want one each", calls, physical, journaled, owners)
	}
	for _, view := range views {
		if view.ID != origin.ID && (view.ReusedFrom != origin.ID || len(view.QueryAttempts) != 0 || view.Observed == nil || origin.Observed == nil || !view.Observed.Equal(*origin.Observed) || view.Expires.After(origin.Expires)) {
			t.Fatal("cold follower lost origin, observation, expiry or physical-attempt truth", view.ID)
		}
	}
	if f.f.model.requests.Load() != beforeModels {
		t.Fatal("non-narrative frozen execution called a model")
	}
}
