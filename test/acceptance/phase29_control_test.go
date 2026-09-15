package acceptance

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestReportingCompositionControlBoundaries(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "control-block", f.base)
	d := phase29Text("Cancellation requires current authority")
	d.Widgets = append(d.Widgets, phase29BlockWidget("table", "control-block", 1, "table-main"))
	state := f.report(t, "control-report", d, true)
	v, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "control-public"})
	if err != nil {
		t.Fatal(err)
	}
	private, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "control-private", Preview: true, Reference: reporting.DocumentReference{Revision: 1}})
	if err != nil {
		t.Fatal(err)
	}
	db := f.f.f.db
	beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
	for _, tc := range []struct {
		name, remove, id string
		want             error
	}{
		{"action", "jobs.cancel", v.ID, access.ErrForbidden},
		{"target", "cw.report.execute:*", v.ID, access.ErrNotFound},
		{"dependency", "cw.execution_context.use:*", v.ID, access.ErrNotFound},
		{"private-action", "reporting.preview", private.ID, access.ErrForbidden},
		{"private-target", "cw.report.preview:*", private.ID, access.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scopes := slices.Clone(f.execute.Scopes())
			if !slices.Contains(scopes, tc.remove) {
				t.Fatal("fixture did not exercise removed authority", tc.remove, scopes)
			}
			scopes = slices.DeleteFunc(scopes, func(s string) bool { return s == tc.remove })
			denied := phase27Actor(t, f.f, f.execute.User(), scopes)
			out, err := db.CancelComposition(ctx, denied, tc.id)
			if !errors.Is(err, tc.want) || out.ID != "" {
				t.Fatal("cancellation authority bypass", out, err)
			}
		})
	}
	if _, err := db.CancelComposition(ctx, f.execute, "missing-operation"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("missing cancellation", err)
	}
	if _, err := db.CompositionWidget(ctx, f.execute, v.ID, "main", "table"); !errors.Is(err, reporting.ErrIncomplete) {
		t.Fatal("sealed values became readable", err)
	}
	if _, err := db.CompositionWidget(ctx, f.execute, v.ID, "../main", "table"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid widget coordinates", err)
	}
	noRead := phase27Actor(t, f.f, f.execute.User(), []string{"cw.report.read:*", "cw.run.read:*"})
	if _, err := db.ViewComposition(ctx, noRead, v.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("metadata action bypass", err)
	}
	if _, err := db.CompositionWidget(ctx, noRead, v.ID, "main", "intro"); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("value action bypass", err)
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := db.ReadComposition(stopped, f.execute, v.ID); err == nil {
		t.Fatal("cancelled execution read succeeded")
	}
	if _, err := db.ViewComposition(stopped, f.execute, v.ID); err == nil {
		t.Fatal("cancelled metadata read succeeded")
	}
	if _, err := db.CompositionWidget(stopped, f.execute, v.ID, "main", "intro"); err == nil {
		t.Fatal("cancelled value read succeeded")
	}
	if _, err := db.CancelComposition(stopped, f.execute, v.ID); err == nil {
		t.Fatal("cancelled control request succeeded")
	}
	// An actual late audit failure must roll back both common operation intent and
	// the composition head. It must not become a partially cancelled artifact.
	raw := support.Raw(t, f.f.f.dsn)
	sql(t, raw, `CREATE FUNCTION chartworks.reject_cancel_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'SYNTHETIC_CANCEL_AUDIT_FAILURE'; END; $$`)
	sql(t, raw, `CREATE TRIGGER reject_cancel_audit BEFORE INSERT ON chartworks.audit_events FOR EACH ROW WHEN (NEW.action='composition.cancelled') EXECUTE FUNCTION chartworks.reject_cancel_audit()`)
	if got, err := db.CancelComposition(ctx, f.execute, v.ID); !errors.Is(err, store.ErrUnavailable) || got.ID != "" {
		t.Fatal("late cancellation audit failure", got, err)
	}
	record, err := db.ReadComposition(ctx, f.execute, v.ID)
	if err != nil || record.State != "sealed" {
		t.Fatal("failed cancellation changed artifact", record.State, err)
	}
	var taskState string
	if err := raw.QueryRow(ctx, `SELECT status FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), v.ID).Scan(&taskState); err != nil || taskState != "pending" {
		t.Fatal("failed cancellation changed common operation", taskState, err)
	}
	sql(t, raw, `DROP TRIGGER reject_cancel_audit ON chartworks.audit_events`)
	for range 2 {
		if out, err := db.CancelComposition(ctx, f.execute, v.ID); err != nil || out.State != "cancelled" {
			t.Fatal("idempotent cancellation", out, err)
		}
	}
	var audits int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1 AND resource_id=$2 AND action='composition.cancelled'`, f.execute.Tenant(), v.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("cancellation duplicated audit", audits, err)
	}
	if f.attemptCount(t) != beforeQueries || f.f.model.requests.Load() != beforeModels {
		t.Fatal("control/read paths invoked sources or models")
	}
}
