package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

func resetPreparationIdentity(r *reporting.AuthoringPreparationRecord) {
	r.InputDigest = readexec.Hash(r.Request)
	r.SourceOperation = "chart-prepare:" + readexec.Hash([]string{r.Binding.Tenant, r.Actor, r.Session, r.Target, r.Operation})
}

func TestReportAppPreparationRetentionBoundariesAndLockedRows(t *testing.T) {
	f, _, e, _, base, raw, _ := preparationFixtureBase(t, "retention-boundaries")
	ctx := t.Context()
	old := time.Now().Add(-25 * time.Hour).UTC()
	proof := reporting.PreparationSettlement{Kind: "not_issued", Status: "failed", RemoteState: "not_issued", Finished: old}
	for n := 1000; n < 1003; n++ {
		r := preparationCopy(t, base, n)
		r.Operation = preparationKey(n, old)
		r.Request.Operation = r.Operation
		r.Status = "failed"
		r.CreatedAt = old
		r.Deadline = old.Add(30 * time.Second)
		r.ExpiresAt = old.Add(15 * time.Minute)
		resetPreparationIdentity(&r)
		seedPreparation(t, raw, r, true, &proof, &old, nil)
	}
	for _, tc := range []struct {
		delta string
		want  bool
	}{{"1 microsecond", false}, {"0 microseconds", true}, {"-1 microsecond", true}} {
		var eligible bool
		err := raw.QueryRow(ctx, `SELECT chartworks.preparation_cleanup_eligible(jsonb_populate_record(NULL::chartworks.authoring_preparations,to_jsonb(p)||jsonb_build_object('settled_at',statement_timestamp()-interval '24 hours'+$2::interval))) FROM chartworks.authoring_preparations p WHERE p.tenant_id=$1 AND p.preparation_id=$3`, e.Tenant(), tc.delta, "000000000000000000000000000003e8").Scan(&eligible)
		if err != nil || eligible != tc.want {
			t.Fatal("terminal retention boundary", tc.delta, eligible, err)
		}
		err = raw.QueryRow(ctx, `SELECT chartworks.preparation_cleanup_eligible(jsonb_populate_record(NULL::chartworks.authoring_preparations,to_jsonb(p)||jsonb_build_object('expires_at',statement_timestamp()+$2::interval))) FROM chartworks.authoring_preparations p WHERE p.tenant_id=$1 AND p.preparation_id=$3`, e.Tenant(), tc.delta, "000000000000000000000000000003e8").Scan(&eligible)
		if err != nil || eligible != tc.want {
			t.Fatal("consumption expiry boundary", tc.delta, eligible, err)
		}
	}
	locker := support.Raw(t, f.f.f.dsn)
	tx, err := locker.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SELECT 1 FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2 FOR UPDATE`, e.Tenant(), "000000000000000000000000000003e8"); err != nil {
		t.Fatal(err)
	}
	next := preparationCopy(t, base, 1030)
	if _, fresh, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, next); err != nil || !fresh {
		t.Fatal("locked old row blocked bounded cleanup", err)
	}
	if n := count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id IN('000000000000000000000000000003e8','000000000000000000000000000003e9','000000000000000000000000000003ea')`, e.Tenant()); n != 1 {
		t.Fatal("SKIP LOCKED did not leave exactly locked row", n)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.f.f.db.SettleAuthoringPreparation(ctx, e, next.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, fresh, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, preparationCopy(t, base, 1031)); err != nil || !fresh {
		t.Fatal(err)
	}
	if n := count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id='000000000000000000000000000003e8'`, e.Tenant()); n != 0 {
		t.Fatal("unlocked eligible row never pruned")
	}
}

func TestReportAppPreparationExpiryAtLockedAdmissionRemainsTyped(t *testing.T) {
	f, _, e, _, base, raw, _ := preparationFixtureBase(t, "expiry-admission")
	ctx := t.Context()
	locker := support.Raw(t, f.f.f.dsn)
	tx, err := locker.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SELECT 1 FROM chartworks.topic_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR UPDATE`, e.Tenant(), base.Topics[0].Topic); err != nil {
		t.Fatal(err)
	}
	r := preparationCopy(t, base, 1100)
	r.Operation = preparationKey(1100, time.Now().Add(-298*time.Second))
	r.Request.Operation = r.Operation
	// Keep immutable request/source-operation coordinates consistent after the
	// synthetic caller chooses a key near the allowed admission boundary.
	resetPreparationIdentity(&r)
	done := make(chan error, 1)
	go func() { _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, r); done <- err }()
	deadline := time.Now().Add(time.Second)
	for count(t, raw, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%topic_publication_heads%'`) == 0 {
		select {
		case err := <-done:
			t.Fatal("admission failed before held fence", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("admission did not reach held topic fence")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(2100 * time.Millisecond)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, reporting.ErrPreparationOperationExpired) {
		t.Fatal("database expiry lost typed disposition", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), r.ID) != 0 {
		t.Fatal("expired key admitted after lock wait")
	}
}
