package acceptance

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

// Use the minimum supported real retention period. Do not disable triggers,
// rewrite an immutable manifest/expiry, or manufacture a validator-issued proof.
func TestReportingCompositionRetention(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	f.block(t, "retention-block", f.base)
	d := phase29Text("Retained composition")
	d.Widgets[0].Text.Text = "SYNTHETIC_RETENTION_TEXT"
	d.Widgets = append(d.Widgets, phase29BlockWidget("table", "retention-block", 1, "table-main"))
	state := f.report(t, "retention-report", d, true)
	limits := f.limits
	limits.Execution.Retention, limits.Execution.PreviewRetention = config.Duration(time.Minute), config.Duration(time.Minute)
	limits.Execution.MaxReuseAge = config.Duration(time.Second)
	compositions := f.withLimits(t, limits)
	expiring := []reporting.CompositionView{}
	var lastExpiry time.Time
	for _, kind := range []string{"completed", "private", "sealed", "cancelled"} {
		request := reporting.CompositionRequest{Key: "retention-" + kind}
		if kind == "private" {
			request.Preview, request.Reference.Revision = true, 1
		}
		v, err := compositions.Admit(ctx, f.execute, "report", state.ID, request)
		if err != nil {
			t.Fatal("seal short-lived composition", kind, err)
		}
		switch kind {
		case "completed", "private":
			v, err = compositions.Run(ctx, f.execute, v.ID, false)
			if err != nil || !v.Complete || v.State != "completed" {
				t.Fatal("complete short-lived values", kind, v, err)
			}
		case "cancelled":
			if cancelled, err := compositions.Cancel(ctx, f.execute, v.ID); err != nil || cancelled.State != "cancelled" {
				t.Fatal("cancel short-lived composition", cancelled, err)
			}
		}
		expiring = append(expiring, v)
		if v.Expires.After(lastExpiry) {
			lastExpiry = v.Expires
		}
	}
	// A second run using normal retention must survive every sweep, even
	// though it shares the same report, block and source execution context.
	live, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "retention-live"})
	if err != nil {
		t.Fatal(err)
	}
	if live, err = f.compositions.Run(ctx, f.execute, live.ID, false); err != nil || !live.Complete {
		t.Fatal("live comparison artifact", live, err)
	}
	raw := support.Raw(t, f.f.f.dsn)
	tenant := f.execute.Tenant()
	before := make(map[string]phase29RetentionSnapshot)
	for _, v := range append(expiring, live) {
		snapshot := phase29RetainedSnapshot(t, raw, tenant, v.ID)
		if snapshot.payloads == 0 || snapshot.groups == 0 || snapshot.retained == 0 ||
			(snapshot.complete && snapshot.widgets == 0) ||
			(snapshot.state == "sealed" && snapshot.reserved == 0) {
			t.Fatal("expiry fixture must contain real owned values and reservations", snapshot)
		}
		before[v.ID] = snapshot
	}
	for _, query := range []string{
		`DELETE FROM chartworks.composition_run_widgets WHERE tenant_id=$1 AND operation_id=$2`,
		`DELETE FROM chartworks.composition_run_groups WHERE tenant_id=$1 AND operation_id=$2`,
		`DELETE FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`,
		`UPDATE chartworks.composition_runs SET expires_at=created_at WHERE tenant_id=$1 AND operation_id=$2`,
	} {
		if _, err := raw.Exec(ctx, query, tenant, live.ID); err == nil {
			t.Fatal("live artifact immutability was relaxed", query)
		}
	}
	retention := phase27Actor(t, f.f, f.execute.User(), []string{"reporting.retention", "cw.tenant.erase:" + tenant})
	for _, limit := range []int{0, 101} {
		if count, err := compositions.Expire(ctx, retention, limit); !errors.Is(err, store.ErrInvalid) || count != 0 {
			t.Fatal("retention limit was not enforced", limit, count, err)
		}
	}
	if count, err := compositions.Expire(ctx, retention, 100); err != nil || count != 0 {
		t.Fatal("retention erased unexpired values", count, err)
	}
	// Await the original sealed expiry rather than mutating protected rows.
	timer := time.NewTimer(time.Until(lastExpiry) + 25*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("bounded wait for real expiry", ctx.Err())
	}
	beforeQueries, beforeModels, beforeLookups := f.attemptCount(t), f.f.model.requests.Load(), f.f.f.lookups.Load()
	for _, denied := range []struct {
		caller identity.Envelope
		want   error
	}{
		{f.execute, access.ErrForbidden},
		{phase27Actor(t, f.f, "retention-action-only", []string{"reporting.retention"}), access.ErrNotFound},
		{phase27Actor(t, f.f, "retention-resource-only", []string{"cw.tenant.erase:" + tenant}), access.ErrForbidden},
	} {
		if count, err := compositions.Expire(ctx, denied.caller, 100); !errors.Is(err, denied.want) || count != 0 {
			t.Fatal("retention widened signed authority", count, err)
		}
	}
	other := f.f.f.token.envelope(t, "foreign-retention-tenant", "foreign-retention-actor", "reporting.retention", "cw.tenant.erase:foreign-retention-tenant")
	if count, err := compositions.Expire(ctx, other, 100); err != nil || count != 0 {
		t.Fatal("retention crossed a tenant boundary", count, err)
	}
	for _, v := range expiring {
		tombstone, err := compositions.Get(ctx, f.execute, v.ID)
		if err != nil || tombstone.State != "expired" || tombstone.Complete || tombstone.Private != v.Private {
			t.Fatal("expired read lost tombstone/privacy", tombstone, err)
		}
		if _, err := compositions.Widget(ctx, f.execute, v.ID, "main", "table"); !errors.Is(err, reporting.ErrExpired) {
			t.Fatal("expired widget silently re-executed", err)
		}
		if got := phase29RetainedSnapshot(t, raw, tenant, v.ID); !reflect.DeepEqual(got, before[v.ID]) {
			t.Fatal("metadata GET mutated retention state", got, before[v.ID])
		}
	}
	sql(t, raw, `CREATE FUNCTION chartworks.reject_composition_expiry() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='composition.expired' THEN RAISE EXCEPTION 'synthetic expiry audit failure'; END IF; RETURN NEW; END; $$; CREATE TRIGGER reject_composition_expiry BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_composition_expiry()`)
	t.Cleanup(func() { sql(t, raw, `DROP TRIGGER IF EXISTS reject_composition_expiry ON chartworks.audit_events`) })
	if count, err := compositions.Expire(ctx, retention, 100); err == nil || count != 0 {
		t.Fatal("erasure succeeded without its atomic audit", count, err)
	}
	for _, v := range expiring {
		if got := phase29RetainedSnapshot(t, raw, tenant, v.ID); !reflect.DeepEqual(got, before[v.ID]) {
			t.Fatal("audit failure partially erased values or released reservations", got, before[v.ID])
		}
	}
	sql(t, raw, `DROP TRIGGER reject_composition_expiry ON chartworks.audit_events`)
	if count, err := compositions.Expire(ctx, retention, 1); err != nil || count != 1 {
		t.Fatal("bounded expiry did not erase exactly one artifact", count, err)
	}
	if count, err := compositions.Expire(ctx, retention, 100); err != nil || count != int64(len(expiring)-1) {
		t.Fatal("expiry retry did not erase remaining artifacts", count, err)
	}
	if count, err := compositions.Expire(ctx, retention, 100); err != nil || count != 0 {
		t.Fatal("expiry was not idempotent", count, err)
	}
	for _, v := range expiring {
		got, expected := phase29RetainedSnapshot(t, raw, tenant, v.ID), before[v.ID]
		expected.state, expected.code, expected.complete = "expired", "retention_expired", false
		expected.widgets, expected.groups, expected.payloads, expected.retained, expected.reserved = 0, 0, 0, 0, 0
		if !reflect.DeepEqual(got, expected) {
			t.Fatal("expiry left values or destroyed immutable metadata/reach", got, expected)
		}
		var audits int
		if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1 AND action='composition.expired' AND resource_id=$2`, tenant, v.ID).Scan(&audits); err != nil || audits != 1 {
			t.Fatal("expiry audit missing or duplicated", audits, err)
		}
	}
	if got := phase29RetainedSnapshot(t, raw, tenant, live.ID); !reflect.DeepEqual(got, before[live.ID]) {
		t.Fatal("expiry modified a live artifact", got, before[live.ID])
	}
	if widget, err := f.compositions.Widget(ctx, f.execute, live.ID, "main", "table"); err != nil || widget.State != "completed" || len(widget.Outputs) != 1 {
		t.Fatal("live retained output was lost", widget, err)
	}
	if f.attemptCount(t) != beforeQueries || f.f.model.requests.Load() != beforeModels || f.f.f.lookups.Load() != beforeLookups {
		t.Fatal("retention or retained reads called a source/model")
	}
}

type phase29RetentionSnapshot struct {
	state, code, manifest                        string
	private, complete                            bool
	retained, reserved                           int64
	widgets, groups, payloads, pages, references int
}

func phase29RetainedSnapshot(t *testing.T, raw *pgx.Conn, tenant, id string) phase29RetentionSnapshot {
	t.Helper()
	var out phase29RetentionSnapshot
	err := raw.QueryRow(context.Background(), `SELECT state,code,manifest_digest,private,complete,retained_bytes,reserved_bytes,
 (SELECT count(*) FROM chartworks.composition_run_widgets w WHERE w.tenant_id=h.tenant_id AND w.operation_id=h.operation_id),
 (SELECT count(*) FROM chartworks.composition_run_groups g WHERE g.tenant_id=h.tenant_id AND g.operation_id=h.operation_id),
 (SELECT count(*) FROM chartworks.composition_run_payloads p WHERE p.tenant_id=h.tenant_id AND p.operation_id=h.operation_id),
 (SELECT count(*) FROM chartworks.composition_run_pages p WHERE p.tenant_id=h.tenant_id AND p.operation_id=h.operation_id),
 (SELECT count(*) FROM chartworks.composition_run_references r WHERE r.tenant_id=h.tenant_id AND r.operation_id=h.operation_id)
 FROM chartworks.composition_runs h WHERE tenant_id=$1 AND operation_id=$2`, tenant, id).Scan(&out.state, &out.code, &out.manifest, &out.private, &out.complete, &out.retained, &out.reserved, &out.widgets, &out.groups, &out.payloads, &out.pages, &out.references)
	if err != nil || out.pages == 0 || out.references == 0 {
		t.Fatal("read retained composition evidence", out, err)
	}
	return out
}
