package acceptance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func (f *phase30Fixture) claim(t *testing.T, id, owner string) (jobs.Lease, auth.Execution, jobs.Invocation) {
	t.Helper()
	lease, err := f.domain.f.f.db.ClaimJob(t.Context(), owner, f.limits)
	if err != nil || lease.Job.ID != id {
		t.Fatal("claim exact reporting occurrence", lease, err)
	}
	proof, err := f.provider.Acquire(t.Context(), lease.Job)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.domain.f.f.db, f.limits)
	if err != nil {
		t.Fatal(err)
	}
	task, err := runner.Inspect(t.Context(), proof.Envelope(), id)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := jobs.ReportingInvocation(proof, lease, task)
	if err != nil {
		t.Fatal(err)
	}
	return lease, proof, inv
}

func testPhase30Operations(t *testing.T) {
	t.Run("shared-claim-and-nonrefundable-budget-fences", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-budget-fence", f.domain.base)
		target := phase30Target("saved_sql", "p30-budget-fence")
		target.Budget.QueryAttempts = 3
		first := f.submit(t, "budget-fence", target)
		second := f.submit(t, "second-root", target)
		third := f.submit(t, "third-root", target)
		lease, _, inv := f.claim(t, first.ID, "first-root-owner")
		f.claim(t, second.ID, "second-root-owner")
		if _, err := f.domain.f.f.db.ClaimJob(t.Context(), "over-tenant-concurrency", f.limits); !errors.Is(err, jobs.ErrEmpty) {
			t.Fatal("reporting bypassed shared tenant concurrency", err)
		}
		if _, err := f.queue.Cancel(t.Context(), f.controls(t), second.ID); err != nil {
			t.Fatal(err)
		}
		f.claim(t, third.ID, "released-root-owner")
		if _, err := f.queue.Cancel(t.Context(), f.controls(t), third.ID); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan error, 24)
		for range 24 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- f.domain.f.f.db.ReserveReportingUsage(t.Context(), inv, jobs.ReportingCharge{Queries: 1})
			}()
		}
		wg.Wait()
		close(results)
		accepted := 0
		for err := range results {
			if err == nil {
				accepted++
			} else if !errors.Is(err, jobs.ErrReportingBudget) {
				t.Fatal("concurrent reservation", err)
			}
		}
		if accepted != 3 {
			t.Fatal("budget race permitted overspend or lost capacity", accepted)
		}
		var reservations int
		raw := support.Raw(t, f.domain.f.f.dsn)
		if err := raw.QueryRow(t.Context(), `SELECT query_reservations FROM chartworks.reporting_occurrence_delivery WHERE tenant_id=$1 AND operation_id=$2`, f.actor.Tenant(), first.ID).Scan(&reservations); err != nil || reservations != 3 {
			t.Fatal("reservations not durable", reservations, err)
		}
		if _, err := raw.Exec(t.Context(), `UPDATE chartworks.reporting_occurrence_delivery SET query_reservations=0 WHERE tenant_id=$1 AND operation_id=$2`, f.actor.Tenant(), first.ID); err == nil {
			t.Fatal("budget counters could be refunded by retry metadata")
		}
		if _, err := raw.Exec(t.Context(), `UPDATE chartworks.operations SET lease_until=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2`, f.actor.Tenant(), first.ID); err != nil {
			t.Fatal(err)
		}
		reclaimed, proof, fresh := f.claim(t, first.ID, "reclaimed-root-owner")
		if reclaimed.Fence <= lease.Fence || reclaimed.Attempt != lease.Attempt+1 {
			t.Fatal("reclaim did not advance fence/attempt", reclaimed)
		}
		if err := f.domain.f.f.db.ReserveReportingUsage(t.Context(), inv, jobs.ReportingCharge{Queries: 1}); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale owner can reserve usage", err)
		}
		if err := f.domain.f.f.db.ReserveReportingUsage(t.Context(), fresh, jobs.ReportingCharge{Queries: 1}); !errors.Is(err, jobs.ErrReportingBudget) {
			t.Fatal("new owner reset exhausted budget", err)
		}
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		if err := f.scheduled.ExecuteScheduledReporting(t.Context(), reclaimed, proof); !errors.Is(err, jobs.ErrReportingBudget) {
			t.Fatal("real consumer bypassed durable physical-attempt budget", err)
		}
		if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
			t.Fatal("exhausted occurrence performed source/model I/O")
		}
	})
	t.Run("reporting-overlap-policy", func(t *testing.T) {
		for _, policy := range []string{"skip", "queue"} {
			t.Run(policy, func(t *testing.T) {
				f := newPhase30Fixture(t, false)
				f.domain.block(t, "p30-overlap", f.domain.base)
				request := phase30Manual(phase30Target("saved_sql", "p30-overlap"))
				request.Spec.Overlap = policy
				s := f.schedule(t, "overlap-policy", request)
				manager := f.manager(t)
				first, err := f.queue.Fire(t.Context(), manager, s.ID, "first")
				if err != nil {
					t.Fatal(err)
				}
				second, err := f.queue.Fire(t.Context(), manager, s.ID, "second")
				if policy == "skip" {
					if !errors.Is(err, store.ErrConflict) {
						t.Fatal("skip admitted overlapping occurrence", second, err)
					}
				} else if err != nil || second.ID == first.ID {
					t.Fatal("queue policy failed to retain separate occurrence", second, err)
				}
				before := f.domain.attemptCount(t)
				if err := f.queue.RunOnce(t.Context()); err != nil {
					t.Fatal(err)
				}
				if policy == "queue" {
					if err := f.queue.RunOnce(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				count := 1
				if policy == "queue" {
					count = 2
				}
				if f.domain.attemptCount(t) != before+count {
					t.Fatal("overlap policy duplicated or discarded execution")
				}
			})
		}
	})
	t.Run("cancellation-reaches-broker-and-joins-worker", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-cancel", f.domain.base)
		job := f.submit(t, "cancel-acquiring", phase30Target("saved_sql", "p30-cancel"))
		before := f.domain.attemptCount(t)
		f.mode.Store(9)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- f.queue.RunOnce(ctx) }()
		deadline := time.After(3 * time.Second)
		for f.calls.Load() == 0 {
			select {
			case <-deadline:
				cancel()
				t.Fatal("reporting worker never reached real broker")
			case <-time.After(5 * time.Millisecond):
			}
		}
		if _, err := f.queue.Cancel(t.Context(), f.controls(t), job.ID); err != nil {
			cancel()
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cancel()
			t.Fatal("cancelled worker did not join")
		}
		if state := f.get(t, job.ID); state.State != "cancelled" || state.Delivery.Catalog == "available" || f.domain.attemptCount(t) != before {
			t.Fatal("cancellation had source or delivery effects", state)
		}
	})
	t.Run("attempt-timeout-is-bounded-and-retryable", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-timeout", f.domain.base)
		job := f.submit(t, "timeout-acquiring", phase30Target("saved_sql", "p30-timeout"))
		limits := f.limits
		limits.AttemptTimeout = 200 * time.Millisecond
		queue, err := jobs.NewWithReporting(f.domain.f.f.db, f.provider, limits, nil, f.scheduled)
		if err != nil {
			t.Fatal(err)
		}
		f.mode.Store(9)
		before, started := f.domain.attemptCount(t), time.Now()
		if err := queue.RunOnce(t.Context()); err == nil {
			t.Fatal("broker timeout disappeared")
		}
		if time.Since(started) > 3*time.Second {
			t.Fatal("configured attempt deadline was not enforced")
		}
		if current := f.get(t, job.ID); current.State != "retry" || f.domain.attemptCount(t) != before {
			t.Fatal("timeout executed or permanently discarded the occurrence", current)
		}
		f.mode.Store(0)
		f.retryNow(t, job.ID)
		if err := f.queue.RunOnce(t.Context()); err != nil {
			t.Fatal("bounded timed-out occurrence could not retry", err)
		}
		if current := f.get(t, job.ID); current.State != "succeeded" || current.Attempts != 2 {
			t.Fatal("retry lost original occurrence", current)
		}
	})
	t.Run("physical-model-budget-is-not-a-hint", func(t *testing.T) {
		f := newPhase30Fixture(t, true)
		d := phase29Text("Model budget refusal")
		d.Widgets = append(d.Widgets, f.domain.queryWidget())
		f.domain.report(t, "p30-model-budget", d, true)
		target := phase30Target("saved_question", "p30-model-budget")
		target.Budget.ModelCalls, target.Budget.ModelTokens = 1, 64
		job := f.submit(t, "bounded-model", target)
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		_ = f.queue.RunOnce(t.Context())
		current := f.get(t, job.ID)
		if current.State != "blocked" || current.Delivery == nil || current.Delivery.Catalog == "available" || current.Delivery.Reserved.ModelCalls > 1 || current.Delivery.Reserved.ModelTokens > 64 {
			t.Fatal("model budget fabricated successful delivery", current)
		}
		if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load()-beforeModels > 1 {
			t.Fatal("model budget reached unbounded source/model execution")
		}
	})
	t.Run("actual-retained-row-bound", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-row-bound", f.domain.base)
		target := phase30Target("saved_sql", "p30-row-bound")
		target.Budget.MaxRows = 1
		job := f.submit(t, "one-row-ceiling", target)
		if err := f.queue.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
		view, err := f.delivery.View(t.Context(), f.domain.execute, reporting.ReportingViewRequest{Kind: "block", Run: job.ID, Output: "table-main", Limit: 10})
		if err != nil || view.Output == nil || view.Output.Table == nil || len(view.Output.Table.Rows) != 1 || view.PageBounds.Total != 1 {
			t.Fatal("scheduled row cap was only metadata", view, err)
		}
	})
	t.Run("source-accepted-before-result-checkpoint-remains-uncertain", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-uncertain", f.domain.base)
		job := f.submit(t, "lost-normalized-result", phase30Target("saved_sql", "p30-uncertain"))
		raw := support.Raw(t, f.domain.f.f.dsn)
		_, err := raw.Exec(t.Context(), `CREATE FUNCTION chartworks.p30_reject_result() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.result IS NOT NULL THEN RAISE EXCEPTION 'synthetic normalized checkpoint failure' USING ERRCODE='58000'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER p30_reject_result BEFORE UPDATE ON chartworks.frozen_run_payloads FOR EACH ROW EXECUTE FUNCTION chartworks.p30_reject_result();`)
		if err != nil {
			t.Fatal(err)
		}
		before := f.domain.attemptCount(t)
		if err := f.queue.RunOnce(t.Context()); err == nil {
			t.Fatal("failed result persistence was hidden")
		}
		if f.domain.attemptCount(t) != before+1 {
			t.Fatal("fixture did not cross the real source acceptance boundary")
		}
		if current := f.get(t, job.ID); current.State != "retry" || current.Delivery.Catalog == "available" {
			t.Fatal("uncheckpointed source result was delivered", current)
		}
		if _, err := raw.Exec(t.Context(), `DROP TRIGGER p30_reject_result ON chartworks.frozen_run_payloads; DROP FUNCTION chartworks.p30_reject_result();`); err != nil {
			t.Fatal(err)
		}
		f.retryNow(t, job.ID)
		if err := f.queue.RunOnce(t.Context()); err == nil {
			t.Fatal("retry invented lost source-result evidence")
		}
		if current := f.get(t, job.ID); current.State != "blocked" || current.Delivery.Catalog == "available" || f.domain.attemptCount(t) != before+1 {
			t.Fatal("uncertain source request was silently resubmitted", current)
		}
	})
}
