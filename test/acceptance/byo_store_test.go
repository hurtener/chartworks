package acceptance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Exercise the real repository independently of the service's permission checks:
// rejected commits must not spend quota, and accepted evidence is write-once.
func TestBYOStoreFencesAndAudit(t *testing.T) {
	ctx := context.Background()
	f := newPhase18Fixture(t)
	f.model.embeddingMode.Store("fixed")
	f.model.rerankMode.Store("fixed")
	limits := config.DefaultQueryBundles()
	limits.MaxSteps = 2
	s, _, _ := phase19Service(t, f, limits, nil, f.service, nil)
	e, _ := phase19Authority(t, f, f.e.Tenant(), f.e.User(), "byo-store", phase19Scopes())
	bundle := phase19Create(t, s, f, e)
	scope := support.Scope(t, e.Tenant(), e.User())
	now := time.Now().UTC().Truncate(time.Microsecond)
	record, err := f.f.db.ReadBYOBundle(ctx, scope, bundle.Reference, e.Session(), now)
	if err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, f.f.dsn)
	step := nlqbyo.Step{Operation: "first", InputDigest: readexec.Hash("input"), BundleDigest: record.Digest, Semantics: bundle.Semantics, Status: "accepted", Code: "accepted", CreatedAt: now, Deadline: now.Add(time.Second)}

	t.Run("invalid_coordinates_and_receipts", func(t *testing.T) {
		if err := f.f.db.CreateBYOBundle(ctx, scope, record, config.QueryBundles{}); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("invalid configuration admitted", err)
		}
		if _, err := f.f.db.ReadBYOBundle(ctx, store.Scope{}, bundle.Reference, e.Session(), now); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("invalid scope admitted", err)
		}
		if _, _, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), nlqbyo.Step{}, now); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("invalid step admitted", err)
		}
		if err := f.f.db.FinishBYOStep(ctx, scope, bundle.Reference, e.Session(), step); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("nonterminal finish admitted", err)
		}
		if _, err := f.f.db.ReadBYOSteps(ctx, scope, bundle.Reference, ""); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("invalid session admitted", err)
		}
		bad := step
		bad.BundleDigest = readexec.Hash("other bundle")
		if _, _, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), bad, now); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("cross-bundle receipt admitted", err)
		}
		bad = step
		bad.CreatedAt, bad.Deadline = bundle.ExpiresAt.Add(-time.Millisecond), bundle.ExpiresAt.Add(time.Millisecond)
		if _, _, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), bad, now); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("deadline exceeded bundle lifetime", err)
		}
		if _, _, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, "other-session", step, now); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("foreign session disclosed", err)
		}
		if _, _, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), step, bundle.ExpiresAt); !errors.Is(err, nlqbyo.ErrReplan) {
			t.Fatal("expired bundle reserved a step", err)
		}
	})

	t.Run("audit_failure_rolls_back_admission", func(t *testing.T) {
		sql(t, raw, `CREATE FUNCTION chartworks.reject_byo_audit_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'byo.%' THEN RAISE EXCEPTION 'synthetic BYO audit failure'; END IF; RETURN NEW; END; $$;
CREATE TRIGGER reject_byo_audit_test BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_byo_audit_test()`)
		defer sql(t, raw, `DROP TRIGGER reject_byo_audit_test ON chartworks.audit_events; DROP FUNCTION chartworks.reject_byo_audit_test()`)
		copyRecord := record
		copyRecord.Bundle.ID = readexec.Hash("audit-failure")
		copyRecord.Digest = readexec.Hash(copyRecord.Bundle)
		if err := f.f.db.CreateBYOBundle(ctx, scope, copyRecord, limits); err == nil {
			t.Fatal("unaudited context committed")
		}
		if _, err := f.f.db.ReadBYOBundle(ctx, scope, copyRecord.Bundle.Reference, e.Session(), now); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("failed admission left a bundle", err)
		}
		if _, fresh, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), step, now); err == nil || fresh {
			t.Fatal("unaudited step committed", err)
		}
		steps, err := f.f.db.ReadBYOSteps(ctx, scope, bundle.Reference, e.Session())
		if err != nil || len(steps) != 0 {
			t.Fatal("failed reservation consumed step budget", err)
		}
	})

	reserved, fresh, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), step, now)
	if err != nil || !fresh || reserved.Number != 1 {
		t.Fatalf("reservation after rollback: %+v %t %v", reserved, fresh, err)
	}
	finishedAt := now.Add(time.Millisecond)
	terminal := reserved
	terminal.Status, terminal.Code, terminal.FinishedAt = "rejected", "sql_unsafe", &finishedAt
	t.Run("terminal_fence_and_audit_rollback", func(t *testing.T) {
		if _, err := raw.Exec(ctx, `UPDATE chartworks.byo_context_bundles SET digest=$1 WHERE bundle_id=$2`, readexec.Hash("rewrite"), bundle.ID); err == nil {
			t.Fatal("context snapshot was mutable")
		}
		bad := terminal
		bad.InputDigest = readexec.Hash("replacement input")
		if err := f.f.db.FinishBYOStep(ctx, scope, bundle.Reference, e.Session(), bad); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("request evidence rewrite was not fenced", err)
		}
		sql(t, raw, `CREATE FUNCTION chartworks.reject_byo_finish_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action = 'byo.step_finished' THEN RAISE EXCEPTION 'synthetic BYO finish audit failure'; END IF; RETURN NEW; END; $$;
CREATE TRIGGER reject_byo_finish_test BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_byo_finish_test()`)
		if err := f.f.db.FinishBYOStep(ctx, scope, bundle.Reference, e.Session(), terminal); err == nil {
			t.Fatal("unaudited terminal receipt committed")
		}
		sql(t, raw, `DROP TRIGGER reject_byo_finish_test ON chartworks.audit_events; DROP FUNCTION chartworks.reject_byo_finish_test()`)
		steps, err := f.f.db.ReadBYOSteps(ctx, scope, bundle.Reference, e.Session())
		if err != nil || len(steps) != 1 || steps[0].Status != "accepted" {
			t.Fatal("finish audit failure did not roll back", err)
		}
		if err := f.f.db.FinishBYOStep(ctx, scope, bundle.Reference, e.Session(), terminal); err != nil {
			t.Fatal("finish retry after rollback", err)
		}
		if err := f.f.db.FinishBYOStep(ctx, scope, bundle.Reference, e.Session(), terminal); !errors.Is(err, store.ErrConflict) {
			t.Fatal("terminal evidence could be finished twice", err)
		}
		if _, err := raw.Exec(ctx, `UPDATE chartworks.byo_steps SET receipt=jsonb_set(receipt,'{code}','"changed"') WHERE bundle_id=$1`, bundle.ID); err == nil {
			t.Fatal("sealed receipt could be rewritten")
		}
	})

	t.Run("replay_and_budget", func(t *testing.T) {
		replayed, fresh, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), step, now)
		if err != nil || fresh || replayed.Status != "rejected" || replayed.Number != 1 {
			t.Fatal("terminal replay changed outcome", err)
		}
		bad := step
		bad.InputDigest = readexec.Hash("changed SQL")
		if _, _, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), bad, now); !errors.Is(err, store.ErrConflict) {
			t.Fatal("operation identity was reusable for different input", err)
		}
		second := step
		second.Operation = "second"
		if _, fresh, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), second, now); err != nil || !fresh {
			t.Fatal("second explicit step", err)
		}
		second.Operation = "third"
		if _, _, err := f.f.db.ReserveBYOStep(ctx, scope, bundle.Reference, e.Session(), second, now); !errors.Is(err, nlqbyo.ErrBudget) {
			t.Fatal("step ceiling bypassed", err)
		}
		other := bundle.Reference
		other.Context = "different-context"
		steps, err := f.f.db.ReadBYOSteps(ctx, scope, other, e.Session())
		if err != nil || len(steps) != 0 {
			t.Fatal("foreign context disclosed receipts", err)
		}
	})

	t.Run("atomic_tenant_and_session_quotas", func(t *testing.T) {
		// Repository fixtures intentionally use storage coordinates, not forged
		// envelopes. AC02/AC04 exercise caller authorization through the service.
		quotaScope := support.Scope(t, "byo-quota", "actor")
		quotaLimits := limits
		quotaLimits.PerSession, quotaLimits.PerTenant = 1, 2
		results := make(chan error, 2)
		var group sync.WaitGroup
		for _, id := range []string{"quota-a", "quota-b"} {
			candidate := record
			candidate.Binding.Tenant = quotaScope.Tenant()
			candidate.Bundle.ID = readexec.Hash(id)
			candidate.Digest = readexec.Hash(candidate.Bundle)
			group.Add(1)
			go func() {
				defer group.Done()
				results <- f.f.db.CreateBYOBundle(ctx, quotaScope, candidate, quotaLimits)
			}()
		}
		group.Wait()
		close(results)
		admitted, denied := 0, 0
		for err := range results {
			switch {
			case err == nil:
				admitted++
			case errors.Is(err, nlqbyo.ErrBudget):
				denied++
			default:
				t.Fatal(err)
			}
		}
		if admitted != 1 || denied != 1 {
			t.Fatal("concurrent session quota bypass", admitted, denied)
		}
		candidate := record
		candidate.Binding.Tenant = quotaScope.Tenant()
		candidate.Session, candidate.Bundle.ID = "second-session", readexec.Hash("quota-c")
		candidate.Digest = readexec.Hash(candidate.Bundle)
		if err := f.f.db.CreateBYOBundle(ctx, quotaScope, candidate, quotaLimits); err != nil {
			t.Fatal("separate session incorrectly denied", err)
		}
		candidate.Session, candidate.Bundle.ID = "third-session", readexec.Hash("quota-d")
		candidate.Digest = readexec.Hash(candidate.Bundle)
		if err := f.f.db.CreateBYOBundle(ctx, quotaScope, candidate, quotaLimits); !errors.Is(err, nlqbyo.ErrBudget) {
			t.Fatal("tenant quota bypassed", err)
		}
	})

	t.Run("retention_cleanup_is_tenant_scoped_and_cascades", func(t *testing.T) {
		candidate := record
		candidate.Bundle.ID = readexec.Hash("after-retention")
		candidate.Bundle.CreatedAt = record.RetainUntil
		candidate.Bundle.ExpiresAt = candidate.Bundle.CreatedAt.Add(time.Duration(limits.TTL))
		candidate.RetainUntil = candidate.Bundle.CreatedAt.Add(time.Duration(limits.Retention))
		candidate.Digest = readexec.Hash(candidate.Bundle)
		if err := f.f.db.CreateBYOBundle(ctx, scope, candidate, limits); err != nil {
			t.Fatal(err)
		}
		// A pre-expiry lookup proves deletion rather than merely expiry filtering.
		if _, err := f.f.db.ReadBYOBundle(ctx, scope, bundle.Reference, e.Session(), now); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("retained bundle survived cleanup", err)
		}
		if n := count(t, raw, `SELECT count(*) FROM chartworks.byo_steps`); n != 0 {
			t.Fatal("orphaned step evidence", n)
		}
		if n := count(t, raw, `SELECT count(*) FROM chartworks.byo_context_bundles WHERE tenant_id='byo-quota'`); n != 2 {
			t.Fatal("cleanup crossed tenant boundary", n)
		}
	})
}
