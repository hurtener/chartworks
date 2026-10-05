package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

func preparationKey(n int, at time.Time) string {
	return "prepare:" + strconv.FormatInt(at.Unix(), 10) + ":" + fmt.Sprintf("%032x", n)
}
func preparationCopy(t *testing.T, base reporting.AuthoringPreparationRecord, n int) reporting.AuthoringPreparationRecord {
	t.Helper()
	r := phase27Copy(t, base)
	r.ID = fmt.Sprintf("%032x", n)
	r.Operation = preparationKey(n, time.Now())
	r.Request.Operation = r.Operation
	r.Request.OperationVersion = reporting.AuthoringPreparationOperationVersion
	r.InputDigest = readexec.Hash(r.Request)
	r.SourceOperation = "chart-prepare:" + readexec.Hash([]string{r.Binding.Tenant, r.Actor, r.Session, r.Target, r.Operation})
	r.CreatedAt = time.Now().UTC()
	r.Deadline = r.CreatedAt.Add(30 * time.Second)
	r.ExpiresAt = r.CreatedAt.Add(15 * time.Minute)
	r.Status = "accepted"
	r.Code = ""
	r.Digest = ""
	r.Revision = nil
	r.Attempt = nil
	return r
}

// These synthetic old rows exercise retention boundaries only. They are not
// claimed as physical source receipts. Only the disposable fixture can bypass
// lifecycle triggers to set the clock or represent pre-migration custody.
func seedPreparation(t *testing.T, raw *pgx.Conn, r reporting.AuthoringPreparationRecord, guarded bool, settlement *reporting.PreparationSettlement, settled, consumed *time.Time) {
	t.Helper()
	ctx := t.Context()
	wire, _ := json.Marshal(r)
	proof, _ := json.Marshal(settlement)
	if settlement == nil {
		proof = nil
	}
	tx, err := raw.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `ALTER TABLE chartworks.authoring_preparations DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.authoring_preparations(tenant_id,preparation_id,actor_id,session_id,target_id,operation_id,input_digest,source_operation,status,record,created_at,deadline,expires_at,admission_guarded,settlement,settled_at,consumed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, r.Binding.Tenant, r.ID, r.Actor, r.Session, r.Target, r.Operation, r.InputDigest, r.SourceOperation, r.Status, wire, r.CreatedAt, r.Deadline, r.ExpiresAt, guarded, proof, settled, consumed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE chartworks.authoring_preparations ENABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
func agePreparation(t *testing.T, raw *pgx.Conn, tenant, id string, at time.Time) {
	t.Helper()
	tx, err := raw.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(t.Context(), `ALTER TABLE chartworks.authoring_preparations DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(t.Context(), `UPDATE chartworks.authoring_preparations SET created_at=$3::timestamptz,deadline=$3::timestamptz+interval '30 seconds',expires_at=$3::timestamptz+interval '15 minutes',settled_at=CASE WHEN settlement IS NOT NULL THEN $3 END,consumed_at=CASE WHEN status='consumed' THEN $3 END WHERE tenant_id=$1 AND preparation_id=$2`, tenant, id, at); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(t.Context(), `ALTER TABLE chartworks.authoring_preparations ENABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}
func preparationFixtureBase(t *testing.T, target string) (*phase29ExecutionFixture, *reporting.Authoring, identity.Envelope, reporting.AuthoringPrepareRequest, reporting.AuthoringPreparationRecord, *pgx.Conn, []string) {
	t.Helper()
	f, s, e, in, _, _, scopes := reportDatasetFixture(t, target)
	v, err := s.PrepareDatasetChart(t.Context(), e, in)
	if err != nil || v.Status != "prepared" {
		t.Fatal("prepare fixture", v, err)
	}
	base, err := f.f.f.db.ReadAuthoringPreparation(t.Context(), e, v.Preparation)
	if err != nil {
		t.Fatal(err)
	}
	return f, s, e, in, base, support.Raw(t, f.f.f.dsn), scopes
}

func TestReportAppPreparationLiabilityAndNativeGuard(t *testing.T) {
	f, s, e, _, base, raw, _ := preparationFixtureBase(t, "preparation-liability")
	ctx := t.Context()
	old := time.Now().Add(-25 * time.Hour).UTC()
	legacy := preparationCopy(t, base, 101)
	legacy.Operation = "unrestricted-legacy"
	legacy.Request.Operation = legacy.Operation
	legacy.Request.OperationVersion = ""
	legacy.InputDigest = readexec.Hash(legacy.Request)
	legacy.SourceOperation = "chart-prepare:" + readexec.Hash([]string{e.Tenant(), e.User(), e.Session(), legacy.Target, legacy.Operation})
	legacy.CreatedAt = old
	legacy.Deadline = old.Add(30 * time.Second)
	legacy.ExpiresAt = old.Add(15 * time.Minute)
	seedPreparation(t, raw, legacy, false, nil, nil, nil)
	next := preparationCopy(t, base, 102)
	before := f.attemptCount(t)
	if replay, err := s.PrepareDatasetChart(ctx, e, legacy.Request); err != nil || replay.Status != "uncertain" {
		t.Fatal("legacy exact retained replay rejected", err)
	}
	changed := phase27Copy(t, legacy.Request)
	changed.Metadata[0].Title = "Changed legacy input"
	if _, err := s.PrepareDatasetChart(ctx, e, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal("legacy changed input not fenced", err)
	}
	if _, err := raw.Exec(ctx, `DELETE FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), legacy.ID); err == nil {
		t.Fatal("database allowed deleting unproved liability")
	}

	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, next); !errors.Is(err, readexec.ErrUncertain) {
		t.Fatal("missing journal freed aged liability", err)
	}
	if _, err := s.PreparationControl(ctx, e, reporting.AuthoringPreparationControlRequest{NewBlock: legacy.Target, Preparation: legacy.ID, Action: "reconcile"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("legacy absence became not-issued", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2 AND settlement IS NULL`, e.Tenant(), legacy.ID) != 1 || f.attemptCount(t) != before {
		t.Fatal("ambiguous legacy liability changed")
	}
	// Guarded no-dispatch is tested on a fresh owned target in a second fixture.
	t.Run("guarded-no-dispatch-and-race", func(t *testing.T) {
		g, _, owner, _, original, metadata, _ := preparationFixtureBase(t, "guarded-chart")
		scope, _ := store.NewScope(owner.Tenant(), owner.User())
		native, err := g.f.f.db.GetReadOperation(t.Context(), scope, original.SourceOperation)
		if err != nil {
			t.Fatal(err)
		}
		for n := 200; n < 210; n++ {
			candidate := preparationCopy(t, original, n)
			reserved, fresh, err := g.f.f.db.ReserveAuthoringPreparation(t.Context(), owner, candidate)
			if err != nil || !fresh {
				t.Fatal(err)
			}
			if err = g.f.f.db.SealAuthoringPreparation(t.Context(), owner, reserved, native.Manifest.Receipt); err != nil {
				t.Fatal(err)
			}
			a := phase27Copy(t, native)
			a.ID = fmt.Sprintf("%032x", n+1000)
			a.Manifest.Operation = reserved.SourceOperation
			a.Created = time.Now().UTC()
			a.Deadline = reserved.Deadline
			a.Status = "accepted"
			a.Remote = nil
			a.RemoteState = "not_issued"
			a.Finished = nil
			a.Rows = 0
			a.Bytes = 0
			a.SourceDurationNS = nil
			a.Code = ""
			var beginErr, settleErr error
			var settled reporting.AuthoringPreparationRecord
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); beginErr = g.f.f.db.BeginRead(t.Context(), scope, a, 1) }()
			go func() {
				defer wg.Done()
				settled, settleErr = g.f.f.db.SettleAuthoringPreparation(t.Context(), owner, reserved.ID, true)
			}()
			wg.Wait()
			if settleErr != nil {
				t.Fatal(settleErr)
			}
			if settled.Settled {
				if beginErr == nil {
					t.Fatal("native admission and no-dispatch both won")
				}
				if err := g.f.f.db.BeginRead(t.Context(), scope, a, 1); err == nil {
					t.Fatal("late original native read admitted")
				}
			} else {
				if beginErr != nil {
					t.Fatal("neither control nor native admission won", beginErr)
				}
				finished := time.Now().UTC()
				a.Status = "cancelled"
				a.Finished = &finished
				a.RemoteState = "not_issued"
				if err := g.f.f.db.FinishRead(t.Context(), scope, a, false); err != nil {
					t.Fatal(err)
				}
				out, err := g.f.f.db.SettleAuthoringPreparation(t.Context(), owner, reserved.ID, true)
				if err != nil || !out.Settled || out.Status != "failed" {
					t.Fatal("native terminal settlement", err, out.Status)
				}
			}
			if count(t, metadata, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2 AND settlement IS NOT NULL`, owner.Tenant(), reserved.ID) != 1 {
				t.Fatal("no durable winner")
			}
		}
	})
}

func TestReportAppPreparationBoundedRetentionAndQuota(t *testing.T) {
	f, _, e, _, base, raw, _ := preparationFixtureBase(t, "preparation-retention")
	ctx := t.Context()
	old := time.Now().Add(-25 * time.Hour).UTC()
	proof := reporting.PreparationSettlement{Kind: "not_issued", Status: "failed", RemoteState: "not_issued", Finished: old}
	seed := func(n int, status string, terminal bool, actor string, operation string) reporting.AuthoringPreparationRecord {
		r := preparationCopy(t, base, n)
		r.Actor = actor
		r.Operation = operation
		r.Request.Operation = operation
		r.InputDigest = readexec.Hash(r.Request)
		r.SourceOperation = "chart-prepare:" + readexec.Hash([]string{e.Tenant(), r.Actor, r.Session, r.Target, r.Operation})
		r.Status = status
		r.CreatedAt = old
		r.Deadline = old.Add(30 * time.Second)
		r.ExpiresAt = old.Add(15 * time.Minute)
		var p *reporting.PreparationSettlement
		var at *time.Time
		if terminal {
			p = &proof
			at = &old
		}
		seedPreparation(t, raw, r, true, p, at, nil)
		return r
	}
	for n := 300; n < 401; n++ {
		seed(n, "failed", true, e.User(), preparationKey(n, old))
	}
	foreign := seed(402, "failed", true, "other-actor", preparationKey(402, old))
	future := seed(403, "failed", true, e.User(), preparationKey(403, time.Now().Add(365*24*time.Hour)))
	legacy := seed(404, "failed", true, e.User(), "legacy-spent-key")
	uncertain := seed(405, "uncertain", false, e.User(), preparationKey(405, old))
	unprovedFailure := seed(413, "failed", false, e.User(), preparationKey(413, old))
	// Eight tiny unresolved records must consume at least the full 16MiB actor
	// payload budget. Pruning must commit even though fresh admission still fails.
	for n := 406; n < 413; n++ {
		seed(n, "uncertain", false, e.User(), preparationKey(n, old))
	}
	next := preparationCopy(t, base, 450)
	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, next); !errors.Is(err, readexec.ErrLimit) {
		t.Fatal("uncertainty did not reserve worst-case bytes", err)
	}
	if n := count(t, raw, `SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1 AND action='authoring.preparation_pruned'`, e.Tenant()); n != 100 {
		t.Fatal("batch or rollback starvation", n)
	}
	for _, r := range []reporting.AuthoringPreparationRecord{foreign, future, uncertain, unprovedFailure} {
		if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), r.ID) != 1 {
			t.Fatal("ineligible custody pruned", r.ID)
		}
	}
	next.Operation = preparationKey(451, time.Now())
	next.Request.Operation = next.Operation
	next.InputDigest = readexec.Hash(next.Request)
	next.SourceOperation = "chart-prepare:" + readexec.Hash([]string{e.Tenant(), e.User(), e.Session(), next.Target, next.Operation})
	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, next); !errors.Is(err, readexec.ErrLimit) {
		t.Fatal(err)
	}
	if n := count(t, raw, `SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1 AND action='authoring.preparation_pruned'`, e.Tenant()); n != 102 {
		t.Fatal("second bounded pass failed", n)
	}
	if _, err := f.f.f.db.ReadAuthoringPreparation(ctx, e, legacy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("terminal legacy not evicted", err)
	}
	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, func() reporting.AuthoringPreparationRecord {
		r := preparationCopy(t, legacy, 499)
		r.Operation = legacy.Operation
		r.Request.Operation = legacy.Operation
		r.Request.OperationVersion = ""
		r.InputDigest = readexec.Hash(r.Request)
		r.SourceOperation = legacy.SourceOperation
		return r
	}()); !errors.Is(err, reporting.ErrPreparationContract) {
		t.Fatal("evicted arbitrary key became fresh", err)
	}
}

func TestReportAppPreparationConsumedReplayAfterCleanup(t *testing.T) {
	f, s, e, in, base, raw, scopes := preparationFixtureBase(t, "preparation-consumed")
	ctx := t.Context()
	create := reporting.AuthoringCreatePreparedRequest{NewBlock: base.Target, Preparation: base.ID, Digest: base.Digest}
	created, err := s.CreatePreparedChart(ctx, e, create)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.f.f.db.ReadBlock(ctx, e, base.Target, reporting.Reference{Revision: 1}, reporting.Read)
	if err != nil {
		t.Fatal(err)
	}
	// Later revisions must not change which immutable revision Create recovers.
	amended := phase27Copy(t, base.Revision.Definition)
	amended.Metadata[0].Title = "Later title"
	if _, err := f.blocks.Edit(ctx, e, base.Target, reporting.EditRequest{ExpectedVersion: created.Block.State.Version, Definition: amended}); err != nil {
		t.Fatal("later native edit", err)
	}
	agePreparation(t, raw, e.Tenant(), base.ID, time.Now().Add(-25*time.Hour).UTC())
	before := f.attemptCount(t)
	next := preparationCopy(t, base, 600)
	sql(t, raw, `CREATE FUNCTION chartworks.reject_preparation_cleanup_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='authoring.preparation_pruned' THEN RAISE EXCEPTION 'fixture cleanup audit rejection'; END IF; RETURN NEW; END $$; CREATE TRIGGER preparation_cleanup_audit_failure BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_preparation_cleanup_audit()`)
	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, next); err == nil {
		t.Fatal("injected cleanup audit failure ignored")
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), base.ID) != 1 || count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparation_consumed WHERE tenant_id=$1`, e.Tenant()) != 0 {
		t.Fatal("receipt/delete/audit did not roll back atomically")
	}
	sql(t, raw, `DROP TRIGGER preparation_cleanup_audit_failure ON chartworks.audit_events; DROP FUNCTION chartworks.reject_preparation_cleanup_audit()`)

	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, e, next); !errors.Is(err, store.ErrConflict) {
		t.Fatal("existing target", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), base.ID) != 0 || count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparation_consumed WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), base.ID) != 1 {
		t.Fatal("consumed payload not compacted")
	}
	discovery := phase27Actor(t, f.f, e.User(), []string{"reporting.discover", "reporting.preview", "cw.block.read:" + base.Target, "cw.block.write:" + base.Target, "cw.block.preview:" + base.Target})
	requirements, err := s.DataDependencies(ctx, discovery, reporting.DataDependencyRequest{NewBlock: base.Target, Preparation: base.ID})
	if err != nil || requirements.Preparation != base.ID || requirements.Operation != in.Operation || requirements.Dataset != in.Intent.Dataset || requirements.Topic != in.Intent.Topic || len(requirements.QueryReferences) != 3 {
		t.Fatal("compacted discovery lost original custody", requirements, err)
	}

	replay, err := s.CreatePreparedChart(ctx, e, create)
	if err != nil || replay.Block.Revision != 1 || replay.Block.Digest != created.Block.Digest {
		t.Fatal("original Create replay", err)
	}
	for _, action := range []string{"inspect", "cancel", "reconcile"} {
		v, err := s.PreparationControl(ctx, e, reporting.AuthoringPreparationControlRequest{NewBlock: base.Target, Preparation: base.ID, Action: action})
		if err != nil || v.Status != "consumed" {
			t.Fatal("compacted control replay", action, err)
		}
	}
	retained, err := s.PrepareDatasetChart(ctx, e, in)
	if err != nil || retained.Status != "consumed" || len(retained.Schema) != 1 {
		t.Fatal("original Prepare replay", err)
	}
	after, err := f.f.f.db.ReadBlock(ctx, e, base.Target, reporting.Reference{Revision: 1}, reporting.Read)
	if err != nil || !reflect.DeepEqual(after.Revision, snapshot.Revision) || f.attemptCount(t) != before {
		t.Fatal("native revision/provenance changed or replay executed", err)
	}
	for _, missing := range []string{"sources.query", "cw.block.preview:" + base.Target, "cw.dataset.query:" + in.Intent.Dataset, "cw.execution_context.use:" + base.Binding.Context} {
		denied := phase27Actor(t, f.f, e.User(), slices.DeleteFunc(slices.Clone(scopes), func(v string) bool { return v == missing }))
		if v, err := s.CreatePreparedChart(ctx, denied, create); err == nil || !reflect.DeepEqual(v, reporting.AuthoringBlockView{}) {
			t.Fatal("receipt widened authority", missing)
		}
	}
	for _, who := range []identity.Envelope{phase27Actor(t, f.f, "other-actor", scopes), identity.Envelope{}} {
		if _, err := s.CreatePreparedChart(ctx, who, create); err == nil {
			t.Fatal("foreign receipt replay")
		}
	}
	otherSession, err := f.f.f.token.verifier.Verify(ctx, phase27Token(t, f.f, e.User(), "other-preparation-session", scopes), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePreparedChart(ctx, otherSession, create); err == nil {
		t.Fatal("cross-session receipt replay")
	}
	foreignScopes := slices.Clone(scopes)
	for i, v := range foreignScopes {
		foreignScopes[i] = strings.ReplaceAll(v, e.Tenant(), "foreign-preparation-tenant")
	}
	claims := f.f.f.token.claims("foreign-preparation-tenant", e.User(), foreignScopes)
	claims["session"] = e.Session()
	otherTenant, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePreparedChart(ctx, otherTenant, create); err == nil {
		t.Fatal("cross-tenant receipt replay")
	}
	wrong := create
	wrong.Digest = strings.Repeat("0", 64)
	if _, err := s.CreatePreparedChart(ctx, e, wrong); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("wrong digest", err)
	}
	if _, err := raw.Exec(ctx, `UPDATE chartworks.authoring_preparation_consumed SET record=record||'{"digest":"forged"}'::jsonb WHERE tenant_id=$1`, e.Tenant()); err == nil {
		t.Fatal("mutable receipt")
	}
	if _, err := raw.Exec(ctx, `DELETE FROM chartworks.authoring_preparation_consumed WHERE tenant_id=$1`, e.Tenant()); err == nil {
		t.Fatal("deleted live receipt")
	}
}
