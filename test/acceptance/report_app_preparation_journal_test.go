package acceptance

import (
	"context"
	"fmt"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func preparationNativeAttempt(t *testing.T, original readexec.Attempt, r reporting.AuthoringPreparationRecord, n int) readexec.Attempt {
	t.Helper()
	a := phase27Copy(t, original)
	a.ID = fmt.Sprintf("%032x", n)
	a.Number = 1
	a.Manifest.Operation = r.SourceOperation
	a.Created = time.Now().UTC()
	a.Deadline = r.Deadline
	a.Status = "accepted"
	a.Remote = nil
	a.RemoteState = "not_issued"
	a.Finished = nil
	a.Rows = 0
	a.Bytes = 0
	a.SourceDurationNS = nil
	a.Code = ""
	return a
}

func TestReportAppPreparationJournalRetentionAndPositiveRecovery(t *testing.T) {
	f, s, e, _, base, raw, _ := preparationFixtureBase(t, "journal-preparation")
	ctx := t.Context()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	original, err := f.f.f.db.GetReadOperation(ctx, scope, base.SourceOperation)
	if err != nil {
		t.Fatal(err)
	}
	r := preparationCopy(t, base, 700)
	if _, fresh, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, r); err != nil || !fresh {
		t.Fatal(err)
	}
	if err := f.f.f.db.SealAuthoringPreparation(ctx, e, r, original.Manifest.Receipt); err != nil {
		t.Fatal(err)
	}
	a := preparationNativeAttempt(t, original, r, 701)
	if err := f.f.f.db.BeginRead(ctx, scope, a, 1); err != nil {
		t.Fatal(err)
	}
	finished := time.Now().UTC()
	a.Status = "failed"
	a.Finished = &finished
	if err := f.f.f.db.FinishRead(ctx, scope, a, false); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour).UTC()
	tx, err := raw.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `ALTER TABLE chartworks.read_attempts DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE chartworks.read_attempts SET created_at=$1::timestamptz,deadline=$1::timestamptz+interval '30 seconds',finished_at=$1::timestamptz+interval '1 second' WHERE attempt_id=$2`, old, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE; ALTER TABLE chartworks.read_attempts ENABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	agePreparation(t, raw, e.Tenant(), r.ID, old)
	// Ordinary native admission runs its own real journal-retention path. It
	// must retain the old exact receipt until preparation copies durable proof.
	next := preparationNativeAttempt(t, original, base, 702)
	next.Manifest.Operation = "independent-native-702"
	next.Deadline = next.Created.Add(30 * time.Second)
	if err := f.f.f.db.BeginRead(ctx, scope, next, 1); err != nil {
		t.Fatal(err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND attempt_id=$2`, e.Tenant(), a.ID) != 1 {
		t.Fatal("native journal erased before preparation witness")
	}
	recovered, err := s.PreparationControl(ctx, e, reporting.AuthoringPreparationControlRequest{NewBlock: r.Target, Preparation: r.ID, Action: "reconcile"})
	if err != nil || recovered.Status != "failed" || recovered.Code != "result_not_retained" {
		t.Fatal("positive original attempt did not settle", recovered, err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2 AND settled_at>clock_timestamp()-interval '5 seconds'`, e.Tenant(), r.ID) != 1 {
		t.Fatal("late recovery did not start new retention window")
	}
	next = preparationNativeAttempt(t, original, base, 703)
	next.Manifest.Operation = "independent-native-703"
	next.Deadline = next.Created.Add(30 * time.Second)
	if err := f.f.f.db.BeginRead(ctx, scope, next, 1); err != nil {
		t.Fatal(err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1 AND attempt_id=$2`, e.Tenant(), a.ID) != 0 {
		t.Fatal("verified native terminal evidence never became prunable")
	}
	// A retained witness remains positive evidence after native journal expiry.
	again, err := s.PreparationControl(ctx, e, reporting.AuthoringPreparationControlRequest{NewBlock: r.Target, Preparation: r.ID, Action: "reconcile"})
	if err != nil || again.Status != "failed" {
		t.Fatal("lost durable settled recovery", err)
	}
	agePreparation(t, raw, e.Tenant(), r.ID, old)
	fresh := preparationCopy(t, base, 704)
	if _, admitted, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, fresh); err != nil || !admitted {
		t.Fatal("settled expired target could not admit explicit fresh work", err)
	}
	if retained, err := f.f.f.db.ReadAuthoringPreparation(ctx, e, r.ID); err != nil || retained.Status != "failed" {
		t.Fatal("encoded key window must remain fenced despite aged fixture timestamps", err)
	}
	// Cancellation establishes guarded absence; a delayed original BeginRead
	// must fail even when the original operation is otherwise well-formed.
	stopped, err := f.f.f.db.SettleAuthoringPreparation(ctx, e, fresh.ID, true)
	if err != nil || !stopped.Settled {
		t.Fatal(err)
	}
	late := preparationNativeAttempt(t, original, fresh, 705)
	if err := f.f.f.db.BeginRead(ctx, scope, late, 1); err == nil {
		t.Fatal("late native admission after durable no-dispatch")
	}
}

func TestReportAppPreparationExistingWitnessSettlesUncertain(t *testing.T) {
	f, s, e, _, base, _, _ := preparationFixtureBase(t, "witnessed-uncertain")
	ctx := t.Context()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	original, err := f.f.f.db.GetReadOperation(ctx, scope, base.SourceOperation)
	if err != nil {
		t.Fatal(err)
	}
	r := preparationCopy(t, base, 800)
	if _, fresh, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, r); err != nil || !fresh {
		t.Fatal(err)
	}
	if err := f.f.f.db.SealAuthoringPreparation(ctx, e, r, original.Manifest.Receipt); err != nil {
		t.Fatal(err)
	}
	a := preparationNativeAttempt(t, original, r, 801)
	if err := f.f.f.db.BeginRead(ctx, scope, a, 1); err != nil {
		t.Fatal(err)
	}
	finished := time.Now().UTC()
	a.Status = "failed"
	a.Finished = &finished
	if err := f.f.f.db.FinishRead(ctx, scope, a, false); err != nil {
		t.Fatal(err)
	}
	r.Status = "uncertain"
	r.Code = "execution_outcome_unknown"
	r.Attempt = &a
	if err := f.f.f.db.FinishAuthoringPreparation(ctx, e, r); err != nil {
		t.Fatal(err)
	}
	v, err := s.PreparationControl(ctx, e, reporting.AuthoringPreparationControlRequest{NewBlock: r.Target, Preparation: r.ID, Action: "reconcile"})
	if err != nil || v.Status != "failed" || v.Code != "result_not_retained" {
		t.Fatal("existing witness left uncertainty stuck", v, err)
	}
}
