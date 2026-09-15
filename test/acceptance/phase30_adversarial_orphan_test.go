package acceptance

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// Constructors do not configure the queue. Pin deliberately small real limits
// before admitting any work; do not rewrite the shared fingerprint or emulate
// the production queue with a counter fixture.
func phase30OrphanFixture(t *testing.T, global, tenant int) *phase30Fixture {
	t.Helper()
	f := newPhase30Fixture(t, false)
	f.limits.MaxPending, f.limits.MaxPendingPerTenant = global, tenant
	var err error
	f.queue, err = jobs.NewWithReporting(f.domain.f.f.db, f.provider, f.limits, nil, f.scheduled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.domain.f.f.db.ConfigureQueue(t.Context(), f.limits); err != nil {
		t.Fatal("pin real bounded fixture queue", err)
	}
	f.domain.report(t, "p30-orphan-parent", phase29Text("Owned report"), true)
	return f
}

func phase30OrphanChild(t *testing.T, f *phase30Fixture, key string) (jobs.Job, auth.Execution, jobs.Invocation, jobs.RequestTask) {
	t.Helper()
	job := f.submit(t, key, phase30Target("report", "p30-orphan-parent"))
	_, proof, parent := f.claim(t, job.ID, key+"-owner")
	input := jobs.RequestInput{Kind: "reporting.run", Target: "child-block", InputHash: strings.Repeat("a", 64)}
	child, err := f.domain.f.f.db.AdmitNestedRequest(t.Context(), parent, key+"-child", input, f.limits)
	if err != nil {
		t.Fatal("admit real child under the current parent fence", err)
	}
	return job, proof, parent, child
}

func phase30AssertRoots(t *testing.T, f *phase30Fixture, wantPending, wantActive int) {
	t.Helper()
	raw := support.Raw(t, f.domain.f.f.dsn)
	var pending, active int
	if err := raw.QueryRow(t.Context(), `SELECT
 (SELECT count(*) FROM chartworks.pending_execution_roots WHERE tenant_id=$1),
 (SELECT count(*) FROM chartworks.active_execution_roots WHERE tenant_id=$1)`, f.actor.Tenant()).Scan(&pending, &active); err != nil {
		t.Fatal(err)
	}
	if pending != wantPending || active != wantActive {
		t.Fatalf("capacity roots: pending=%d active=%d, want pending=%d active=%d", pending, active, wantPending, wantActive)
	}
}

func phase30ExpectCapacity(t *testing.T, f *phase30Fixture, key string) {
	t.Helper()
	target := phase30Target("report", "p30-orphan-parent")
	out, err := f.queue.Submit(t.Context(), f.actor, key, jobs.Submission{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &target})
	if !errors.Is(err, jobs.ErrBusy) || out.ID != "" {
		t.Fatal("a counted root failed to hold admission capacity", out, err)
	}
}

func TestPhase30AdversarialOrphanCapacity(t *testing.T) {
	for _, limits := range []struct {
		name   string
		global int
		tenant int
	}{{"tenant-bound", 2, 1}, {"global-bound", 1, 1}} {
		t.Run(limits.name, func(t *testing.T) {
			f := phase30OrphanFixture(t, limits.global, limits.tenant)
			db := f.domain.f.f.db
			beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			for n := range 3 {
				key := fmt.Sprintf("orphan-%d", n)
				job, proof, parent, child := phase30OrphanChild(t, f, key)
				phase30AssertRoots(t, f, 1, 1)
				phase30ExpectCapacity(t, f, key+"-full")
				if _, err := f.queue.Cancel(t.Context(), f.controls(t), job.ID); err != nil {
					t.Fatal(err)
				}
				phase30AssertRoots(t, f, 0, 0)
				retained, err := db.ReadRequest(t.Context(), proof.Envelope(), child.ID)
				if err != nil || retained.State != "pending" || retained.Attempts != 0 || retained.Digest() != child.Digest() {
					t.Fatal("capacity repair rewrote accepted child evidence", retained, err)
				}
				if _, err := db.ClaimRequest(t.Context(), proof.Envelope(), child.ID, "independent", f.limits); !errors.Is(err, store.ErrConflict) {
					t.Fatal("orphan became independently claimable", err)
				}
				if _, err := db.ClaimNestedRequest(t.Context(), parent, child.ID, "stale-parent", f.limits); !errors.Is(err, store.ErrConflict) {
					t.Fatal("cancelled parent reclaimed the child", err)
				}
				if err := f.queue.RunOnce(t.Context()); !errors.Is(err, jobs.ErrEmpty) {
					t.Fatal("orphan reached the queued dispatcher", err)
				}
			}

			// All replicas still use the same admission lock: clearing three dead
			// children must release exactly the configured slot, not waive limits.
			type result struct {
				job jobs.Job
				err error
			}
			results := make(chan result, 8)
			for n := range 8 {
				go func() {
					target := phase30Target("report", "p30-orphan-parent")
					job, err := f.queue.Submit(t.Context(), f.actor, fmt.Sprintf("concurrent-%d", n), jobs.Submission{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &target})
					results <- result{job: job, err: err}
				}()
			}
			accepted := []jobs.Job{}
			failures := []error{}
			for range 8 {
				r := <-results
				if r.err == nil {
					accepted = append(accepted, r.job)
				} else if !errors.Is(r.err, jobs.ErrBusy) {
					failures = append(failures, r.err)
				}
			}
			if len(accepted) != 1 || len(failures) != 0 {
				t.Fatal("released capacity was lost or oversubscribed", len(accepted), failures)
			}
			phase30AssertRoots(t, f, 1, 0)
			if _, err := f.queue.Cancel(t.Context(), f.controls(t), accepted[0].ID); err != nil {
				t.Fatal(err)
			}
			phase30AssertRoots(t, f, 0, 0)
			if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
				t.Fatal("capacity or denied ownership checks executed warehouse/model work")
			}
		})
	}
}

func TestPhase30AdversarialOrphanLiveLease(t *testing.T) {
	f := phase30OrphanFixture(t, 1, 1)
	job, proof, parent, child := phase30OrphanChild(t, f, "live-orphan")
	db := f.domain.f.f.db
	lease, err := db.ClaimNestedRequest(t.Context(), parent, child.ID, "live-child", f.limits)
	if err != nil {
		t.Fatal(err)
	}
	phase30AssertRoots(t, f, 1, 1)
	if _, err := f.queue.Cancel(t.Context(), f.controls(t), job.ID); err != nil {
		t.Fatal(err)
	}
	// A cancellation receipt is not proof that the child's external work stopped.
	phase30AssertRoots(t, f, 1, 1)
	phase30ExpectCapacity(t, f, "still-running")
	retained, err := db.ReadRequest(t.Context(), proof.Envelope(), child.ID)
	if err != nil || retained.State != "running" || retained.Attempts != lease.Attempt || retained.Digest() != child.Digest() {
		t.Fatal("live child was prematurely released or rewritten", retained, err)
	}
	raw := support.Raw(t, f.domain.f.f.dsn)
	if _, err := raw.Exec(t.Context(), `UPDATE chartworks.operations SET lease_until=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2`, job.Tenant, child.ID); err != nil {
		t.Fatal(err)
	}
	phase30AssertRoots(t, f, 0, 0)
	if _, err := db.ClaimNestedRequest(t.Context(), parent, child.ID, "dead-owner", f.limits); !errors.Is(err, store.ErrConflict) {
		t.Fatal("lease expiry restored cancelled parent authority", err)
	}
	f.submit(t, "after-live-expiry", phase30Target("report", "p30-orphan-parent"))
	phase30AssertRoots(t, f, 1, 0)
}

func TestPhase30AdversarialOrphanOldFence(t *testing.T) {
	f := phase30OrphanFixture(t, 2, 2)
	job, _, parent, child := phase30OrphanChild(t, f, "old-fence")
	db := f.domain.f.f.db
	first, err := db.ClaimNestedRequest(t.Context(), parent, child.ID, "old-child", f.limits)
	if err != nil {
		t.Fatal(err)
	}
	phase30AssertRoots(t, f, 1, 1)
	raw := support.Raw(t, f.domain.f.f.dsn)
	if _, err := raw.Exec(t.Context(), `UPDATE chartworks.operations SET lease_until=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2`, job.Tenant, job.ID); err != nil {
		t.Fatal(err)
	}
	// Pending parent recovery and its orphaned live child are separate roots.
	phase30AssertRoots(t, f, 2, 1)
	_, _, fresh := f.claim(t, job.ID, "new-parent")
	phase30AssertRoots(t, f, 2, 2)
	phase30ExpectCapacity(t, f, "old-child-still-running")
	if _, err := db.ClaimNestedRequest(t.Context(), fresh, child.ID, "too-early", f.limits); !errors.Is(err, store.ErrConflict) {
		t.Fatal("new parent stole a still-live child lease", err)
	}
	if _, err := raw.Exec(t.Context(), `UPDATE chartworks.operations SET lease_until=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2`, job.Tenant, child.ID); err != nil {
		t.Fatal(err)
	}
	phase30AssertRoots(t, f, 1, 1)
	if _, err := db.ClaimNestedRequest(t.Context(), parent, child.ID, "stale-owner", f.limits); !errors.Is(err, store.ErrConflict) {
		t.Fatal("old parent fence regained authority", err)
	}
	reclaimed, err := db.ClaimNestedRequest(t.Context(), fresh, child.ID, "new-child", f.limits)
	if err != nil || reclaimed.Fence <= first.Fence || reclaimed.Attempt != first.Attempt+1 || reclaimed.Task.Digest() != child.Digest() {
		t.Fatal("valid parent could not recover the exact accepted child", reclaimed, err)
	}
	phase30AssertRoots(t, f, 1, 1)
}

func TestPhase30AdversarialOrphanExistingRows(t *testing.T) {
	f := phase30OrphanFixture(t, 1, 1)
	migrations, err := postgres.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	var original, repair string
	for _, m := range migrations {
		switch m.Version {
		case 31:
			_, definition, found := strings.Cut(m.SQL, "CREATE VIEW chartworks.pending_execution_roots AS")
			if !found {
				t.Fatal("original applied migration identity changed")
			}
			original = "CREATE OR REPLACE VIEW chartworks.pending_execution_roots AS" + definition
		case 34:
			repair = m.SQL
		}
	}
	if original == "" || repair == "" {
		t.Fatal("missing original or forward-only repair migration")
	}
	raw := support.Raw(t, f.domain.f.f.dsn)
	// Restore only the pre-fix view in this isolated database. The actual queue,
	// authority adapter, row/attempt guards and every other migration stay real.
	if _, err := raw.Exec(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	job, proof, _, child := phase30OrphanChild(t, f, "preexisting-orphan")
	if _, err := f.queue.Cancel(t.Context(), f.controls(t), job.ID); err != nil {
		t.Fatal(err)
	}
	phase30AssertRoots(t, f, 1, 0)
	phase30ExpectCapacity(t, f, "blocked-before-repair")
	t.Log("REPRODUCED: pre-fix view retains an unclaimable child and rejects new admission")
	if _, err := raw.Exec(t.Context(), repair); err != nil {
		t.Fatal("apply forward-only repair to existing orphan rows", err)
	}
	phase30AssertRoots(t, f, 0, 0)
	retained, err := f.domain.f.f.db.ReadRequest(t.Context(), proof.Envelope(), child.ID)
	if err != nil || retained.State != "pending" || retained.Attempts != 0 || retained.Digest() != child.Digest() {
		t.Fatal("migration changed accepted child identity or fabricated completion", retained, err)
	}
	f.submit(t, "blocked-before-repair", phase30Target("report", "p30-orphan-parent"))
	phase30AssertRoots(t, f, 1, 0)
	t.Log("REPAIRED: the same admission succeeds without deleting or rewriting retained work")
}
