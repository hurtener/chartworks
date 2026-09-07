package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestPhase13PipelineStoreAtomicActivation(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	relation := readexec.Relation{ID: "analytics.sales", Schema: "analytics", Name: "sales", Columns: []readexec.Column{{Name: "id", NativeType: "int8", Category: "integer", Safe: true}}}
	binding := readexec.Binding{
		Tenant: f.e.Tenant(), Source: "atomic-input", Context: "atomic-input:v1", Revision: 1, Dialect: "postgres",
		Contract: "source-contract:atomic-input", Fingerprint: readexec.Hash(relation), Relations: []readexec.Relation{relation},
	}
	if !binding.Valid() {
		t.Fatal("synthetic pipeline input binding is invalid")
	}
	definition := engineering.PipelineDefinition{
		ID: "atomic-pipeline", Name: "Atomic activation pipeline", Connection: "workspace",
		Steps: []engineering.PipelineStep{
			{
				ID: "first", Source: binding.Source, Context: binding.Context,
				SQL: "SELECT id::bigint AS id FROM analytics.sales", Inputs: []string{relation.ID},
				DependsOn: []string{}, FromSteps: []string{}, Strategy: "replace",
				Columns: []engineering.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}}, Checks: []engineering.PipelineCheck{},
			},
			{
				ID: "second", Source: binding.Source, Context: binding.Context,
				SQL: "SELECT id::bigint AS id FROM analytics.sales", Inputs: []string{relation.ID},
				DependsOn: []string{}, FromSteps: []string{}, Strategy: "replace",
				Columns: []engineering.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}}, Checks: []engineering.PipelineCheck{},
			},
		},
	}
	record, err := f.db.SavePipeline(ctx, f.e, definition, 0)
	if err != nil {
		t.Fatal("save pipeline", err)
	}
	record, err = f.db.PublishPipeline(ctx, f.e, definition.ID, record.Version)
	if err != nil {
		t.Fatal("publish pipeline", err)
	}

	limits := jobs.Defaults()
	limits.Workers, limits.GlobalConcurrency, limits.TenantConcurrency = 1, 2, 1
	limits.Lease, limits.Heartbeat, limits.Poll = 2*time.Second, 50*time.Millisecond, 20*time.Millisecond
	limits.AttemptTimeout, limits.Backoff = time.Second, 20*time.Millisecond
	runner, err := jobs.NewRequestRunner(f.db, limits)
	if err != nil {
		t.Fatal("request runner", err)
	}
	task, err := runner.Admit(ctx, f.e, "atomic-pipeline-run", jobs.RequestInput{Kind: "pipeline.run", Target: definition.ID, InputHash: record.Digest})
	if err != nil {
		t.Fatal("admit pipeline run", err)
	}
	metadata := support.Raw(t, f.dsn)

	completed, err := runner.Run(ctx, f.e, task, time.Second, func(runCtx context.Context, invocation jobs.Invocation) error {
		execution, reserveErr := f.db.ReservePipelineExecution(runCtx, f.e, record, invocation.Lease().Task, "cw_stage")
		if reserveErr != nil || len(execution.Stages) != 2 {
			t.Fatalf("reserve two output stages: %#v %v", execution, reserveErr)
		}

		states := make(map[string]engineering.PipelineStageState, len(execution.Stages))
		for _, prepared := range execution.Stages {
			state := prepared
			state.State, state.Stage.State, state.Fence = "dispatching", "dispatching", invocation.Lease().Fence
			state.Application = "cw-pipeline-atomic-activation"
			state.Input = binding
			state.Dependencies = []string{relation.ID}
			if _, mutateErr := f.db.MutatePipelineStage(runCtx, invocation, record, state); mutateErr != nil {
				t.Fatal("dispatch stage", state.Stage.Step, mutateErr)
			}
			state.State, state.Stage.State, state.Stage.OID, state.Rows = "applied", "applied", 4100+int64(len(states)), 2
			state.RenderedHash = readexec.Hash([]string{"rendered", state.Stage.Step})
			state.ValidationHash = readexec.Hash([]string{"validated", state.Stage.Step})
			state.LineageHash = readexec.Hash([]string{"lineage", state.Stage.Step})
			if _, mutateErr := f.db.MutatePipelineStage(runCtx, invocation, record, state); mutateErr != nil {
				t.Fatal("apply stage", state.Stage.Step, mutateErr)
			}
			state.State, state.Stage.State = "checked", "checked"
			state.Stage = sources.SealPipelineStage(state.Stage)
			if _, mutateErr := f.db.MutatePipelineStage(runCtx, invocation, record, state); mutateErr != nil {
				t.Fatal("check stage", state.Stage.Step, mutateErr)
			}
			if replay, mutateErr := f.db.MutatePipelineStage(runCtx, invocation, record, state); mutateErr != nil || len(replay.Stages) != 2 {
				t.Fatalf("exact checked replay changed the stage: %#v %v", replay, mutateErr)
			}
			divergent := state
			divergent.Rows++
			if _, mutateErr := f.db.MutatePipelineStage(runCtx, invocation, record, divergent); !errors.Is(mutateErr, store.ErrConflict) {
				t.Fatalf("divergent checked replay accepted: %v", mutateErr)
			}
			states[state.Stage.Step] = state
		}

		records := make([]sources.Record, 0, len(execution.Stages))
		for _, stage := range execution.Stages {
			state := states[stage.Stage.Step]
			location := state.Stage.Location()
			outputBinding := readexec.Binding{
				Tenant: f.e.Tenant(), Source: state.Stage.Source, Context: state.Stage.Context, Revision: state.Stage.Revision, Dialect: "postgres",
				Contract: "source-contract:" + readexec.Hash(location)[:32], Fingerprint: readexec.Hash([]any{location, state.Stage.Columns}),
				Relations: []readexec.Relation{{ID: "ds:" + readexec.Hash(location)[:32], Schema: state.Stage.Schema, Name: state.Stage.Table, Columns: []readexec.Column{{Name: "id", NativeType: "int8", Category: "integer", Safe: true}}}},
			}
			records = append(records, sources.Record{
				Source:     sources.Source{ID: state.Stage.Source, Name: "Managed output " + state.Stage.Step, Dialect: "postgres", Revision: state.Stage.Revision, ContextID: state.Stage.Context, Status: "registered"},
				Connection: state.Stage.Alias, Binding: outputBinding, Pipeline: &location,
			})
		}

		invalid := append([]sources.Record(nil), records...)
		if len(invalid) != 2 || invalid[1].Source.ID != definition.ID+".second" {
			t.Fatalf("test setup lost deterministic stage order: %#v", invalid)
		}
		otherStage := states["second"].Stage
		otherStage.OID++
		otherStage = sources.SealPipelineStage(otherStage)
		otherLocation := otherStage.Location()
		invalid[1].Pipeline = &otherLocation
		if !invalid[1].Valid() {
			t.Fatal("test setup did not produce a structurally valid mismatched output")
		}
		if completeErr := f.db.CompletePipelineExecution(runCtx, invocation, record, invalid); !errors.Is(completeErr, store.ErrInvalid) {
			t.Fatalf("mismatched second output accepted: %v", completeErr)
		}

		var sourceRows, revisionRows, outputRows int
		var runState, requestState string
		if queryErr := metadata.QueryRow(runCtx, `
			SELECT
			 (SELECT count(*) FROM chartworks.sources WHERE tenant_id=$1 AND source_id IN ($2,$3)),
			 (SELECT count(*) FROM chartworks.source_revisions WHERE tenant_id=$1 AND source_id IN ($2,$3)),
			 (SELECT count(*) FROM chartworks.pipeline_outputs WHERE tenant_id=$1 AND pipeline_id=$4),
			 (SELECT state FROM chartworks.pipeline_runs WHERE tenant_id=$1 AND operation_id=$5),
			 (SELECT status FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$5)`,
			f.e.Tenant(), definition.ID+".first", definition.ID+".second", definition.ID, task.ID,
		).Scan(&sourceRows, &revisionRows, &outputRows, &runState, &requestState); queryErr != nil {
			t.Fatal("inspect rolled back activation", queryErr)
		}
		if sourceRows != 0 || revisionRows != 0 || outputRows != 0 || runState != "staged" || requestState != "running" {
			t.Fatalf("failed activation leaked partial state: sources=%d revisions=%d outputs=%d run=%s request=%s", sourceRows, revisionRows, outputRows, runState, requestState)
		}
		failedReceipt, readErr := f.db.ReadPipelineExecution(runCtx, f.e, task.ID)
		if readErr != nil || len(failedReceipt.Stages) != 2 || failedReceipt.Stages[0].State != "checked" || failedReceipt.Stages[1].State != "checked" {
			t.Fatalf("failed activation lost checked stage evidence: %#v %v", failedReceipt, readErr)
		}

		if completeErr := f.db.CompletePipelineExecution(runCtx, invocation, record, records); completeErr != nil {
			t.Fatal("corrected activation retry", completeErr)
		}
		replayCtx, cancelReplay := context.WithTimeout(context.WithoutCancel(runCtx), time.Second)
		defer cancelReplay()
		if completeErr := f.db.CompletePipelineExecution(replayCtx, invocation, record, records); !errors.Is(completeErr, store.ErrConflict) {
			t.Fatalf("completed activation replay was not fenced: %v", completeErr)
		}
		return nil
	})
	if err != nil || completed.State != "succeeded" {
		t.Fatalf("pipeline request did not complete: %#v %v", completed, err)
	}

	var sourceRows, revisionRows, outputRows, activatedAudits int
	if err = metadata.QueryRow(ctx, `
		SELECT
		 (SELECT count(*) FROM chartworks.sources WHERE tenant_id=$1 AND source_id IN ($2,$3)),
		 (SELECT count(*) FROM chartworks.source_revisions WHERE tenant_id=$1 AND source_id IN ($2,$3)),
		 (SELECT count(*) FROM chartworks.pipeline_outputs WHERE tenant_id=$1 AND pipeline_id=$4 AND operation_id=$5),
		 (SELECT count(*) FROM chartworks.audit_events WHERE tenant_id=$1 AND action='pipeline.activated' AND resource_id=$5)`,
		f.e.Tenant(), definition.ID+".first", definition.ID+".second", definition.ID, task.ID,
	).Scan(&sourceRows, &revisionRows, &outputRows, &activatedAudits); err != nil {
		t.Fatal("inspect committed activation", err)
	}
	if sourceRows != 2 || revisionRows != 2 || outputRows != 2 || activatedAudits != 1 {
		t.Fatalf("activation was not committed exactly once: sources=%d revisions=%d outputs=%d audits=%d", sourceRows, revisionRows, outputRows, activatedAudits)
	}
}
