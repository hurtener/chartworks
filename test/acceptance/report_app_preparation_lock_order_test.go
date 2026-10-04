package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestReportAppPreparationNativeGuardDoesNotWaitBehindRotation(t *testing.T) {
	f, _, e, _, base, raw, _ := preparationFixtureBase(t, "native-preparation-lock-order")
	ctx := t.Context()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	original, err := f.f.f.db.GetReadOperation(ctx, scope, base.SourceOperation)
	if err != nil {
		t.Fatal(err)
	}
	r := preparationCopy(t, base, 1300)
	if _, fresh, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, r); err != nil || !fresh {
		t.Fatal(err)
	}
	if err := f.f.f.db.SealAuthoringPreparation(ctx, e, r, original.Manifest.Receipt); err != nil {
		t.Fatal(err)
	}
	// A uses the same held source row SHARE as WithSource's execution callback.
	holder := support.Raw(t, f.f.f.dsn)
	a, err := holder.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Rollback(context.Background())
	if _, err := a.Exec(ctx, `SELECT 1 FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 FOR SHARE`, e.Tenant(), r.Binding.Source); err != nil {
		t.Fatal(err)
	}
	// C queues the exclusive source lock used by revision rotation behind A.
	rotator := support.Raw(t, f.f.f.dsn)
	c, err := rotator.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Rollback(context.Background())
	rotationCtx, cancelRotation := context.WithTimeout(ctx, 4*time.Second)
	defer cancelRotation()
	rotation := make(chan error, 1)
	go func() {
		_, err := c.Exec(rotationCtx, `/* preparation rotation fixture */ SELECT 1 FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 FOR UPDATE`, e.Tenant(), r.Binding.Source)
		rotation <- err
	}()
	deadline := time.Now().Add(time.Second)
	for count(t, raw, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '/* preparation rotation fixture */%'`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("rotation never queued")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// B has acquired the shared native retention/admission fences when its
	// preparation trigger reaches the queued source lock. It must complete
	// promptly, rather than retain fences while waiting for A's callback. PostgreSQL
	// may still grant the compatible SHARE, so success and typed Busy are valid.
	admissionCtx, cancelAdmission := context.WithTimeout(ctx, time.Second)
	attempt := preparationNativeAttempt(t, original, r, 1301)
	err = f.f.f.db.BeginRead(admissionCtx, scope, attempt, 1)
	cancelAdmission()
	if err != nil && !errors.Is(err, reporting.ErrBusy) {
		t.Fatal("native guard waited behind queued rotation", err)
	}
	admitted := err == nil
	// A's next DispatchRead starts with this exact advisory fence. Prove it can
	// acquire it while A still holds source SHARE and C still waits for A.
	var fenceFree bool
	if err := a.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,721415))`, e.Tenant()).Scan(&fenceFree); err != nil || !fenceFree {
		t.Fatal("failed admission retained native journal fence", err)
	}
	expected := int64(0)
	if admitted {
		expected = 1
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND attempt_id=$2`, e.Tenant(), attempt.ID) != expected {
		t.Fatal("native admission result disagreed with durable row")
	}
	if err := a.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-rotation; err != nil {
		t.Fatal("rotation did not complete after source callback", err)
	}
	if err := c.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if admitted {
		finished := time.Now().UTC()
		attempt.Status = "cancelled"
		attempt.Finished = &finished
		attempt.RemoteState = "not_issued"
		if err := f.f.f.db.FinishRead(ctx, scope, attempt, false); err != nil {
			t.Fatal(err)
		}
	}
	stopped, err := f.f.f.db.SettleAuthoringPreparation(ctx, e, r.ID, true)
	if err != nil || !stopped.Settled {
		t.Fatal("rejected native admission fabricated dispatch liability", err)
	}
}

func TestReportAppPreparationNativeGuardRejectsHeldConflictingLocks(t *testing.T) {
	f, _, e, _, base, raw, _ := preparationFixtureBase(t, "conflicting-preparation-locks")
	ctx := t.Context()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	original, err := f.f.f.db.GetReadOperation(ctx, scope, base.SourceOperation)
	if err != nil {
		t.Fatal(err)
	}
	for i, kind := range []string{"custody", "topic", "source"} {
		t.Run(kind, func(t *testing.T) {
			r := preparationCopy(t, base, 1320+i)
			if _, fresh, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, r); err != nil || !fresh {
				t.Fatal(err)
			}
			if err := f.f.f.db.SealAuthoringPreparation(ctx, e, r, original.Manifest.Receipt); err != nil {
				t.Fatal(err)
			}
			holder := support.Raw(t, f.f.f.dsn)
			tx, err := holder.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			query, id := `SELECT 1 FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2 FOR UPDATE`, r.ID
			if kind == "topic" {
				query, id = `SELECT 1 FROM chartworks.topic_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR UPDATE`, r.Topics[0].Topic
			}
			if kind == "source" {
				query, id = `SELECT 1 FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 FOR UPDATE`, r.Binding.Source
			}
			if _, err := tx.Exec(ctx, query, e.Tenant(), id); err != nil {
				t.Fatal(err)
			}
			limited, cancel := context.WithTimeout(ctx, time.Second)
			a := preparationNativeAttempt(t, original, r, 1330+i)
			err = f.f.f.db.BeginRead(limited, scope, a, 1)
			cancel()
			if !errors.Is(err, reporting.ErrBusy) {
				t.Fatal("conflicting native fence did not fail fast", err)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			if count(t, raw, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND attempt_id=$2`, e.Tenant(), a.ID) != 0 {
				t.Fatal("busy admission left a native attempt")
			}
			if stopped, err := f.f.f.db.SettleAuthoringPreparation(ctx, e, r.ID, true); err != nil || !stopped.Settled {
				t.Fatal("failed guard left dispatch custody", err)
			}
		})
	}
}
