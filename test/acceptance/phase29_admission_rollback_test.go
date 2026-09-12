package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

// Capture an actual admitted task without constructing or altering its proof.
type phase29SealObserver struct {
	reporting.CompositionRepository
	id string
}

func (r *phase29SealObserver) SealComposition(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedComposition) (reporting.CompositionRecord, error) {
	if _, err := proof.Checked(e); err != nil {
		return reporting.CompositionRecord{}, err
	}
	r.id = task.ID
	return r.CompositionRepository.SealComposition(ctx, e, task, proof)
}

func TestReportingCompositionAdmissionRollback(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "atomic-seal-block", f.base)
	d := phase29Text("Atomic manifest and indexes")
	d.Widgets = append(d.Widgets, phase29BlockWidget("table", "atomic-seal-block", 1, "table-main"))
	state := f.report(t, "atomic-seal-report", d, true)
	observed := &phase29SealObserver{CompositionRepository: f.f.f.db}
	runner, err := jobs.NewRequestRunner(f.f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	service, err := reporting.NewCompositions(f.documents, observed, f.runs, reporting.DocumentsFromQueries(f.query), runner)
	if err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, f.f.f.dsn)
	sql(t, raw, `CREATE FUNCTION chartworks.reject_composition_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'SYNTHETIC_SEAL_FAILURE_CANARY'; END; $$`)
	beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
	for _, table := range []string{"composition_runs", "composition_run_payloads", "composition_run_groups", "composition_run_pages", "composition_run_widgets", "composition_run_references", "audit_events"} {
		t.Run(table, func(t *testing.T) {
			target := pgx.Identifier{"chartworks", table}.Sanitize()
			predicate := ""
			if table == "audit_events" {
				predicate = " WHEN (NEW.action='composition.sealed')"
			}
			sql(t, raw, "CREATE TRIGGER reject_composition_insert BEFORE INSERT ON "+target+" FOR EACH ROW"+predicate+" EXECUTE FUNCTION chartworks.reject_composition_insert()")
			t.Cleanup(func() { sql(t, raw, "DROP TRIGGER IF EXISTS reject_composition_insert ON "+target) })
			request := reporting.CompositionRequest{Key: "atomic-seal-" + table}
			observed.id = ""
			v, err := service.Admit(ctx, f.execute, "report", state.ID, request)
			if !errors.Is(err, store.ErrUnavailable) || strings.Contains(err.Error(), "CANARY") || v.ID != "" || observed.id == "" {
				t.Fatal("seal did not fail at its actual insert boundary", table, v, err)
			}
			id := observed.id
			var rows int
			if err := raw.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM chartworks.composition_runs WHERE tenant_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM chartworks.composition_run_groups WHERE tenant_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM chartworks.composition_run_pages WHERE tenant_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM chartworks.composition_run_widgets WHERE tenant_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM chartworks.composition_run_references WHERE tenant_id=$1 AND operation_id=$2)+
 (SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1 AND resource_id=$2 AND action='composition.sealed')`, f.execute.Tenant(), id).Scan(&rows); err != nil || rows != 0 {
				t.Fatal("failed seal retained partial values, indexes, quota or audit", rows, err)
			}
			pending, err := runner.Inspect(ctx, f.execute, id)
			if err != nil || pending.State != "pending" {
				t.Fatal("failed seal lost the reusable request key", pending, err)
			}
			sql(t, raw, "DROP TRIGGER reject_composition_insert ON "+target)
			retried, err := service.Admit(ctx, f.execute, "report", state.ID, request)
			if err != nil || retried.ID != id || retried.State != "sealed" || retried.QueryGroups != 1 {
				t.Fatal("retry did not reuse admission after full rollback", retried, err)
			}
		})
	}
	if f.attemptCount(t) != beforeQueries || f.f.model.requests.Load() != beforeModels {
		t.Fatal("failed or successful admission executed a source/model")
	}
}
