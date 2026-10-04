package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestReportAppPreparationContradictoryLiabilityRemainsReserved(t *testing.T) {
	f, service, e, _, base, raw, _ := preparationFixtureBase(t, "contradictory-preparation")
	ctx := t.Context()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	original, err := f.f.f.db.GetReadOperation(ctx, scope, base.SourceOperation)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour).UTC()
	r := preparationCopy(t, base, 1200)
	r.Operation = preparationKey(1200, old)
	r.Request.Operation = r.Operation
	r.Status = "failed"
	r.CreatedAt = old
	r.Deadline = old.Add(30 * time.Second)
	r.ExpiresAt = old.Add(15 * time.Minute)
	resetPreparationIdentity(&r)
	proof := reporting.PreparationSettlement{Kind: "not_issued", Status: "failed", RemoteState: "not_issued", Finished: old}
	seedPreparation(t, raw, r, true, &proof, &old, nil)
	// Inject a contradictory legacy physical journal in the disposable database.
	// New admission cannot create this state: the shared preparation lock forbids
	// a native read after a no-dispatch proof. Cleanup must still fail closed.
	a := preparationNativeAttempt(t, original, r, 1201)
	a.Status = "uncertain"
	a.RemoteState = "unknown"
	a.Created = old
	a.Deadline = old.Add(30 * time.Second)
	manifest, _ := json.Marshal(a.Manifest)
	tx, err := raw.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `ALTER TABLE chartworks.read_attempts DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO chartworks.read_attempts(tenant_id,actor_id,attempt_id,operation_id,attempt_number,source_id,context_id,manifest,manifest_hash,status,remote_state,created_at,deadline)VALUES($1,$2,$3,$4,1,$5,$6,$7,$8,'uncertain','unknown',$9,$10)`, e.Tenant(), e.User(), a.ID, a.Manifest.Operation, a.Manifest.Receipt.Source, a.Manifest.Receipt.Context, manifest, readexec.Hash(a.Manifest), a.Created, a.Deadline); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE; ALTER TABLE chartworks.read_attempts ENABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	before := f.attemptCount(t)
	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, preparationCopy(t, base, 1202)); !errors.Is(err, readexec.ErrUncertain) {
		t.Fatal("contradiction freed target", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations p WHERE p.tenant_id=$1 AND p.preparation_id=$2 AND chartworks.preparation_has_liability(p) AND NOT chartworks.preparation_cleanup_eligible(p)`, e.Tenant(), r.ID) != 1 {
		t.Fatal("contradictory failed custody not retained")
	}
	for _, action := range []string{"cancel", "reconcile"} {
		if _, err := service.PreparationControl(ctx, e, reporting.AuthoringPreparationControlRequest{NewBlock: r.Target, Preparation: r.ID, Action: action}); !errors.Is(err, readexec.ErrUncertain) {
			t.Fatal("contradictory witness projected settled control", action, err)
		}
	}
	for n := 1210; n < 1216; n++ {
		unknown := preparationCopy(t, base, n)
		unknown.Status = "uncertain"
		unknown.CreatedAt = old
		unknown.Deadline = old.Add(30 * time.Second)
		unknown.ExpiresAt = old.Add(15 * time.Minute)
		seedPreparation(t, raw, unknown, true, nil, nil, nil)
	}
	// Six other unresolved rows plus this contradictory witness consume seven
	// complete reservations. Charging only its tiny JSON would wrongly pass the
	// byte gate and return target-uncertain instead of the expected capacity error.
	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, preparationCopy(t, base, 1220)); !errors.Is(err, readexec.ErrLimit) {
		t.Fatal("contradictory liability undercharged", err)
	}
	if f.attemptCount(t) != before || count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), r.ID) != 1 {
		t.Fatal("replacement or cleanup changed contradictory liability")
	}
}
