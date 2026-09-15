package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestStoreFrozenMetadataOutages(t *testing.T) {
	f := newReportingStoreFixture(t)
	db, ctx := f.f.f.db, context.Background()
	b := f.create(t, "store-outages", true)
	hooks := &storeFrozenHooks{RunRepository: db}
	hooks.seal = func(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedRun) (reporting.RunRecord, error) {
		for _, table := range []string{"operations", "frozen_runs", "source_revisions", "topic_publication_heads"} {
			t.Run("seal/"+table, func(t *testing.T) {
				before := f.frozenSnapshot(t)
				restore := storeHideTable(t, f.raw, table)
				out, err := db.SealFrozenRun(ctx, e, task, proof)
				restore()
				if !errors.Is(err, store.ErrUnavailable) || out.Manifest != nil || f.frozenSnapshot(t) != before {
					t.Fatal("seal accepted unavailable prerequisite", err)
				}
			})
		}
		return db.SealFrozenRun(ctx, e, task, proof)
	}
	hooks.checkpoint = func(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedRunWrite) (reporting.RunRecord, error) {
		w, err := proof.Checked(inv)
		if err != nil {
			return reporting.RunRecord{}, err
		}
		if w.Kind == "result" {
			for _, table := range []string{"frozen_runs", "source_revisions", "topic_publication_heads", "read_attempts", "frozen_run_payloads"} {
				t.Run("checkpoint/"+table, func(t *testing.T) {
					before := f.frozenSnapshot(t)
					restore := storeHideTable(t, f.raw, table)
					out, err := db.CheckpointFrozenRun(ctx, inv, proof)
					restore()
					if !errors.Is(err, store.ErrUnavailable) || out.Result != nil || f.frozenSnapshot(t) != before {
						t.Fatal("unavailable metadata produced a partial checkpoint", err)
					}
				})
			}
		}
		return db.CheckpointFrozenRun(ctx, inv, proof)
	}
	service := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
	v := f.admit(t, service, b.State.ID, "store-outages-key", 0)
	v, err := service.Run(ctx, f.execute, v.ID, false)
	if err != nil || v.State != "succeeded" {
		t.Fatal("valid source after metadata outages", err)
	}
	for _, table := range []string{"frozen_run_attempts", "read_attempts", "frozen_run_payloads", "frozen_run_outputs"} {
		t.Run("read/"+table, func(t *testing.T) {
			before := f.frozenSnapshot(t)
			restore := storeHideTable(t, f.raw, table)
			out, err := db.ReadFrozenRun(ctx, f.execute, v.ID, false)
			restore()
			if !errors.Is(err, store.ErrUnavailable) || out.View.ID != "" || out.Result != nil || f.frozenSnapshot(t) != before {
				t.Fatal("artifact read returned a partial response on missing metadata", err)
			}
		})
	}
	for _, table := range []string{"frozen_runs", "frozen_run_attempts", "read_attempts"} {
		t.Run("catalog/"+table, func(t *testing.T) {
			restore := storeHideTable(t, f.raw, table)
			out, err := db.ListFrozenArtifacts(ctx, f.execute, "", 10)
			restore()
			if !errors.Is(err, store.ErrUnavailable) || len(out.Items) != 0 || out.Next != "" {
				t.Fatal("catalog exposed partial metadata", err)
			}
		})
	}
	// Failure to append cancellation audit must roll back cancellation intent, not
	// leave a durable operation cancelled while reporting the write as failed.
	pending := f.admit(t, f.runs, b.State.ID, "store-cancel-rollback", 0)
	for _, table := range []string{"operations", "audit_events"} {
		t.Run("cancel/"+table, func(t *testing.T) {
			before := storeTableSnapshot(t, f.raw, "operations", "audit_events")
			operation, condition := "UPDATE", ""
			if table == "audit_events" {
				operation, condition = "INSERT", "NEW.action='reporting.run_cancelled'"
			}
			remove := storeWriteFault(t, f.raw, table, operation, condition)
			_, err := db.CancelFrozenRun(ctx, f.execute, pending.ID)
			remove()
			if !errors.Is(err, store.ErrUnavailable) || before != storeTableSnapshot(t, f.raw, "operations", "audit_events") {
				t.Fatal("cancellation failure committed partial intent", err)
			}
		})
	}
}

func TestStoreFrozenExpiryWinsOverCheckpoint(t *testing.T) {
	f := newReportingStoreFixture(t)
	db, ctx := f.f.f.db, context.Background()
	b := f.create(t, "store-expiry-fence", true)
	hooks := &storeFrozenHooks{RunRepository: db}
	expired := false
	hooks.checkpoint = func(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedRunWrite) (reporting.RunRecord, error) {
		w, err := proof.Checked(inv)
		if err != nil {
			return reporting.RunRecord{}, err
		}
		if w.Kind != "result" {
			return db.CheckpointFrozenRun(ctx, inv, proof)
		}
		if _, err = f.raw.Exec(ctx, `UPDATE chartworks.frozen_runs SET payload_expires_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), w.Manifest.ID); err != nil {
			t.Fatal(err)
		}
		n, err := db.ExpireFrozenArtifacts(ctx, f.execute, 10)
		if err != nil || n != 1 {
			t.Fatal("expire actual payload", n, err)
		}
		expired = true
		before := f.frozenSnapshot(t)
		out, err := db.CheckpointFrozenRun(ctx, inv, proof)
		if !errors.Is(err, reporting.ErrExpired) || out.Result != nil || f.frozenSnapshot(t) != before {
			t.Fatal("late source result resurrected expired values", err)
		}
		return out, err
	}
	service := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
	v := f.admit(t, service, b.State.ID, "store-expiry-fence-key", 0)
	out, err := service.Run(ctx, f.execute, v.ID, false)
	if !expired || !errors.Is(err, reporting.ErrExpired) || out.State != "expired" {
		t.Fatal("late completion defeated retention", out.State, err)
	}
	stored, err := db.ReadFrozenRun(ctx, f.execute, v.ID, false)
	if err != nil || stored.Manifest != nil || stored.Result != nil || len(stored.Outputs) != 0 || stored.View.RetainedBytes != 0 {
		t.Fatal("expired artifact retained payload", err)
	}
}
