package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

func TestReportingCompositionCheckpointRollback(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "atomic-checkpoint-block", f.base)
	d := phase29Text("Atomic checkpoints and resume")
	d.Widgets = append(d.Widgets, phase29BlockWidget("table", "atomic-checkpoint-block", 1, "table-main"))
	state := f.report(t, "atomic-checkpoint-report", d, true)
	raw := support.Raw(t, f.f.f.dsn)
	sql(t, raw, `CREATE FUNCTION chartworks.reject_checkpoint_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'SYNTHETIC_CHECKPOINT_FAILURE'; END; $$`)
	for _, tc := range []struct {
		name, table, event, predicate string
		groups                        int
	}{
		{"group-value", "composition_run_groups", "UPDATE", "NEW.result IS NOT NULL", 0},
		{"page-summary", "composition_run_pages", "UPDATE", "", 0},
		{"group-audit", "audit_events", "INSERT", "NEW.action='composition.group_checkpoint'", 0},
		{"completion-head", "composition_runs", "UPDATE", "NEW.state='completed'", 1},
		{"completion-audit", "audit_events", "INSERT", "NEW.action='composition.completed'", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "atomic-checkpoint-" + tc.name})
			if err != nil {
				t.Fatal(err)
			}
			target := pgx.Identifier{"chartworks", tc.table}.Sanitize()
			predicate := ""
			if tc.predicate != "" {
				predicate = " WHEN (" + tc.predicate + ")"
			}
			sql(t, raw, "CREATE TRIGGER reject_checkpoint_write BEFORE "+tc.event+" ON "+target+" FOR EACH ROW"+predicate+" EXECUTE FUNCTION chartworks.reject_checkpoint_write()")
			t.Cleanup(func() { sql(t, raw, "DROP TRIGGER IF EXISTS reject_checkpoint_write ON "+target) })
			beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
			interrupted, err := f.compositions.Run(ctx, f.execute, v.ID, false)
			if !errors.Is(err, store.ErrUnavailable) || interrupted.Complete || interrupted.State != "sealed" {
				t.Fatal("checkpoint fault misreported as terminal result", interrupted, err)
			}
			record, err := f.f.f.db.ReadComposition(ctx, f.execute, v.ID)
			if err != nil || len(record.Results) != tc.groups || record.State != "sealed" {
				t.Fatal("checkpoint rollback lost group/byte consistency", len(record.Results), record.State, err)
			}
			var reserved int64
			if err := raw.QueryRow(ctx, `SELECT reserved_bytes FROM chartworks.composition_runs WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), v.ID).Scan(&reserved); err != nil || reserved == 0 {
				t.Fatal("failed completion released reservation", reserved, err)
			}
			if f.attemptCount(t) != beforeQueries+1 || f.f.model.requests.Load() != beforeModels {
				t.Fatal("fault boundary did not retain one real child execution")
			}
			sql(t, raw, "DROP TRIGGER reject_checkpoint_write ON "+target)
			recovered, err := f.compositions.Run(ctx, f.execute, v.ID, true)
			if err != nil || !recovered.Complete || recovered.Manifest != v.Manifest || f.attemptCount(t) != beforeQueries+1 || f.f.model.requests.Load() != beforeModels {
				t.Fatal("resume repeated work or replaced manifest", recovered, err)
			}
			payload, err := f.compositions.Widget(ctx, f.execute, v.ID, "main", "table")
			if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].ID != "table-main" {
				t.Fatal("recovery discarded child values", payload, err)
			}
		})
	}
}
