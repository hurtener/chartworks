package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/test/support"
)

func TestPipelineCancellationAuditFailureRollsBackLedger(t *testing.T) {
	f := newPipelineFixture(t, nil, nil)
	ctx := context.Background()
	source := f.create(t, "cancel-rollback-source")
	definition := f.definition(t, source, "cancel-rollback-pipeline")
	draft, err := f.pipelines.Draft(ctx, f.e, definition, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pipelines.Publish(ctx, f.e, definition.ID, draft.Version); err != nil {
		t.Fatal(err)
	}
	record, err := f.db.ReadPipeline(ctx, f.e, definition.ID, draft.Version, "engineering.pipeline.run", "write")
	if err != nil {
		t.Fatal(err)
	}
	limits := jobs.Defaults()
	limits.Workers, limits.GlobalConcurrency, limits.TenantConcurrency = 1, 2, 1
	limits.Lease, limits.Heartbeat, limits.Poll, limits.AttemptTimeout, limits.Backoff = 2*time.Second, 50*time.Millisecond, 20*time.Millisecond, time.Second, 20*time.Millisecond
	runner, err := jobs.NewRequestRunner(f.db, limits)
	if err != nil {
		t.Fatal(err)
	}
	task, err := runner.Admit(ctx, f.e, "cancel-rollback", jobs.RequestInput{Kind: "pipeline.run", Target: definition.ID, InputHash: record.Digest})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := f.db.ReservePipelineExecution(ctx, f.e, record, task, "cw_stage")
	if err != nil || execution.State != "staged" || execution.Operation.State != "pending" || len(execution.Stages) != 1 {
		t.Fatal("pipeline reservation did not establish the cancellation fixture", err, execution)
	}

	readAuthority := f.token.envelope(t, f.e.Tenant(), f.e.User(), "jobs.read", "cw.source.write:"+definition.ID)
	if _, err = f.db.ReadPipelineExecutionControl(ctx, readAuthority, task.ID, "engineering.pipeline.run"); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("an unregistered control action reached the pipeline ledger", err)
	}
	if inspected, inspectErr := f.db.ReadPipelineExecutionControl(ctx, readAuthority, task.ID, "jobs.read"); inspectErr != nil || inspected.Operation.ID != task.ID || inspected.State != "staged" {
		t.Fatal("registered pipeline inspection lost the staged receipt", inspectErr, inspected)
	}

	metadata := support.Raw(t, f.dsn)
	if _, err = metadata.Exec(ctx, `CREATE FUNCTION chartworks.fail_pipeline_cancel_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='request.cancelled' THEN RAISE EXCEPTION 'synthetic pipeline cancellation audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_pipeline_cancel_audit BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.fail_pipeline_cancel_audit()`); err != nil {
		t.Fatal("install cancellation audit failure", err)
	}
	cancelAuthority := f.token.envelope(t, f.e.Tenant(), f.e.User(), "jobs.cancel", "cw.source.write:"+definition.ID)
	if _, err = f.db.CancelPipelineExecution(ctx, cancelAuthority, task.ID); err == nil {
		t.Fatal("pipeline cancellation committed without its required audit")
	}

	var requestState, runState string
	var stages, cancelledAudits int
	if err = metadata.QueryRow(ctx, `SELECT
		(SELECT status FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2),
		(SELECT state FROM chartworks.pipeline_runs WHERE tenant_id=$1 AND operation_id=$2),
		(SELECT count(*) FROM chartworks.pipeline_stages WHERE tenant_id=$1 AND operation_id=$2),
		(SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1 AND resource_id=$2 AND action='request.cancelled')`,
		f.e.Tenant(), task.ID).Scan(&requestState, &runState, &stages, &cancelledAudits); err != nil {
		t.Fatal("inspect cancellation rollback", err)
	}
	if requestState != "pending" || runState != "staged" || stages != 1 || cancelledAudits != 0 {
		t.Fatalf("failed cancellation leaked partial state: request=%s run=%s stages=%d audits=%d", requestState, runState, stages, cancelledAudits)
	}
	if retained, readErr := f.db.ReadPipelineExecution(ctx, f.e, task.ID); readErr != nil || retained.Operation.State != "pending" || retained.State != "staged" || len(retained.Stages) != 1 {
		t.Fatal("failed cancellation lost the retained execution", readErr, retained)
	}

	if _, err = metadata.Exec(ctx, `DROP TRIGGER fail_pipeline_cancel_audit ON chartworks.audit_events; DROP FUNCTION chartworks.fail_pipeline_cancel_audit()`); err != nil {
		t.Fatal("remove cancellation audit failure", err)
	}
	cancelled, err := f.db.CancelPipelineExecution(ctx, cancelAuthority, task.ID)
	if err != nil || cancelled.State != "cancelled" || cancelled.Code != "cancelled" {
		t.Fatal("rolled-back request could not be cancelled", err, cancelled)
	}
	replayed, err := f.db.CancelPipelineExecution(ctx, cancelAuthority, task.ID)
	if err != nil || replayed.State != "cancelled" || replayed.ManifestHash != task.ManifestHash {
		t.Fatal("completed cancellation replay changed the retained operation", err, replayed)
	}
	if retained, readErr := f.db.ReadPipelineExecution(ctx, f.e, task.ID); readErr != nil || retained.Operation.State != "cancelled" || retained.State != "staged" {
		t.Fatal("cancellation rewrote the staged pipeline outcome", readErr, retained)
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1 AND resource_id=$2 AND action='request.cancelled'`, f.e.Tenant(), task.ID) != 1 {
		t.Fatal("cancellation audit was not committed exactly once")
	}
	if _, err := f.db.ReadPipelineExecutionControl(ctx, readAuthority, task.ID, "jobs.cancel"); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("read authority unexpectedly substituted for cancellation authority")
	}
}
