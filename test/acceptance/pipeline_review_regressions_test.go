package acceptance

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

func rebuildPipelineSources(t *testing.T, f *pipelineFixture) {
	t.Helper()
	f.pipelines.Close()
	f.s.Close()
	var err error
	f.s, err = sources.New(f.db, f.values.Sources, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.s.Close)
	f.validator, err = readexec.NewValidator(f.s, f.values.Exec)
	if err != nil {
		t.Fatal(err)
	}
	f.pipelines, err = engineering.NewPipelineService(f.db, f.s, f.validator, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.pipelines.Close)
}

func TestPipelineRejectsDifferentInputDatabase(t *testing.T) {
	model := newGatewayFixture(t, nil)
	f := newPipelineFixture(t, model.engine, nil)
	ctx := context.Background()
	// Separate read references permit the managed destination to move without
	// changing the registered source's accepted database or native binding.
	for i := range f.values.Sources.Connections {
		if f.values.Sources.Connections[i].ID == "workspace" {
			f.values.Sources.Connections[i].ReadDSN = "env:CHARTWORKS_PIPELINE_READ"
		}
	}
	f.mu.Lock()
	f.sourceFixture.values["CHARTWORKS_PIPELINE_READ"] = f.sourceFixture.values["CHARTWORKS_SOURCE_READ"]
	f.mu.Unlock()
	rebuildPipelineSources(t, f)
	source := f.create(t, "source-a")
	definition := f.definition(t, source, "same-name-different-db")
	draft, err := f.pipelines.Draft(ctx, f.e, definition, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pipelines.Publish(ctx, f.e, definition.ID, draft.Version); err != nil {
		t.Fatal(err)
	}
	pending := definition
	pending.ID = "pending-other-db"
	pendingDraft, err := f.pipelines.Draft(ctx, f.e, pending, 0)
	if err != nil {
		t.Fatal(err)
	}

	secondDSN := support.Database(t)
	second := support.Raw(t, secondDSN)
	secondURL, err := url.Parse(secondDSN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = second.Exec(ctx, `CREATE SCHEMA analytics; CREATE TABLE analytics.sales(id integer PRIMARY KEY); INSERT INTO analytics.sales VALUES(777);`); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{f.role, f.writer} {
		if _, err = second.Exec(ctx, "GRANT USAGE ON SCHEMA analytics TO "+pgx.Identifier{role}.Sanitize()+"; GRANT SELECT ON analytics.sales TO "+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = second.Exec(ctx, "GRANT CREATE ON DATABASE "+pgx.Identifier{strings.TrimPrefix(secondURL.Path, "/")}.Sanitize()+" TO "+pgx.Identifier{f.writer}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	var firstID, secondID int
	if err = f.admin.QueryRow(ctx, "SELECT min(id) FROM analytics.sales").Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err = second.QueryRow(ctx, "SELECT min(id) FROM analytics.sales").Scan(&secondID); err != nil || firstID == secondID {
		t.Fatal("fixture databases must contain distinct values", err)
	}
	moveReference := func(name string) {
		t.Helper()
		f.mu.Lock()
		defer f.mu.Unlock()
		value, parseErr := url.Parse(f.sourceFixture.values[name])
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		value.Path = secondURL.Path
		f.sourceFixture.values[name] = value.String()
	}
	moveReference("CHARTWORKS_SOURCE_WRITE")
	if _, err = f.pipelines.AdmitRun(ctx, f.e, definition.ID, draft.Version, "moved-writer-admit"); !errors.Is(err, engineering.ErrOwnership) {
		t.Fatal("admission accepted a writer outside the destination read database", err)
	}
	if _, err = f.pipelines.Run(ctx, f.e, definition.ID, draft.Version, "moved-writer-run", false); !errors.Is(err, engineering.ErrOwnership) {
		t.Fatal("execution accepted a writer outside the destination read database", err)
	}
	moveReference("CHARTWORKS_PIPELINE_READ")
	modelCalls := model.requests.Load()
	if _, err = f.pipelines.Propose(ctx, f.e, engineering.PipelineProposalRequest{ID: "proposal-other-db", Name: "Cross database proposal", Connection: definition.Connection, Source: source.ID, Context: source.ContextID, Instruction: "Select the identifier"}); !errors.Is(err, readexec.ErrBinding) || model.requests.Load() != modelCalls {
		t.Fatal("cross-database proposal reached the model", err, model.requests.Load()-modelCalls)
	}
	rejected := definition
	rejected.ID = "rejected-other-db"
	if _, err = f.pipelines.Draft(ctx, f.e, rejected, 0); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("cross-database draft accepted", err)
	}
	if _, err = f.pipelines.Get(ctx, f.e, rejected.ID, 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("rejected draft persisted", err)
	}
	if _, err = f.pipelines.Publish(ctx, f.e, pending.ID, pendingDraft.Version); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("cross-database publication accepted", err)
	}
	if _, err = f.pipelines.AdmitRun(ctx, f.e, definition.ID, draft.Version, "different-db-admit"); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("cross-database admission accepted", err)
	}
	if _, err = f.pipelines.Run(ctx, f.e, definition.ID, draft.Version, "different-db-run", false); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("same relation name ran against wrong database", err)
	}
	metadata := support.Raw(t, f.dsn)
	var runs, stages, outputs int
	if err = metadata.QueryRow(ctx, `SELECT (SELECT count(*) FROM chartworks.pipeline_runs),(SELECT count(*) FROM chartworks.pipeline_stages),(SELECT count(*) FROM chartworks.pipeline_outputs)`).Scan(&runs, &stages, &outputs); err != nil || runs != 0 || stages != 0 || outputs != 0 {
		t.Fatal("rejection left execution effects", err, runs, stages, outputs)
	}
	for _, conn := range []*pgx.Conn{f.admin, second} {
		var namespaces int
		if err = conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_namespace WHERE nspname LIKE 'cw_test%'`).Scan(&namespaces); err != nil || namespaces != 0 {
			t.Fatal("rejection created a managed namespace", err, namespaces)
		}
	}
}

func TestPipelineExactRunAuthorityCoversPrivateStagesAndJobControls(t *testing.T) {
	f := newPipelineFixture(t, nil, nil)
	ctx := context.Background()
	source := f.create(t, "exact-authority-source")
	definition := f.definition(t, source, "exact-authority-pipeline")
	second := definition.Steps[0]
	second.ID = "second"
	second.Source = ""
	second.Context = ""
	second.Inputs = nil
	second.SQL = "SELECT id FROM {{step.output}}"
	second.DependsOn = []string{"output"}
	second.FromSteps = []string{"output"}
	definition.Steps = append(definition.Steps, second)
	externalReach := []string{
		"sources.query",
		"cw.source.write:" + definition.ID,
		"cw.source.query:" + source.ID,
		"cw.execution_context.use:" + source.ContextID,
		"cw.dataset.query:" + definition.Steps[0].Inputs[0],
	}
	draftAuthority := f.token.envelope(t, f.e.Tenant(), f.e.User(), append([]string{"engineering.pipeline.write"}, externalReach...)...)
	draft, err := f.pipelines.Draft(ctx, draftAuthority, definition, 0)
	if err != nil {
		t.Fatal(err)
	}
	publishAuthority := f.token.envelope(t, f.e.Tenant(), f.e.User(), append([]string{"engineering.pipeline.publish"}, externalReach...)...)
	if _, err = f.pipelines.Publish(ctx, publishAuthority, definition.ID, draft.Version); err != nil {
		t.Fatal(err)
	}

	runAuthority := f.token.envelope(t, f.e.Tenant(), f.e.User(), append([]string{"engineering.pipeline.run"}, externalReach...)...)
	admitted, err := f.pipelines.AdmitRun(ctx, runAuthority, definition.ID, draft.Version, "exact-authority-control")
	if err != nil || admitted.Operation.State != "pending" {
		t.Fatal("exact run authority did not admit", err, admitted)
	}
	missingJobAction := f.token.envelope(t, f.e.Tenant(), f.e.User(),
		"engineering.pipeline.run", "cw.source.write:"+definition.ID,
		"cw.source.query:"+source.ID, "cw.execution_context.use:"+source.ContextID,
		"cw.dataset.query:"+definition.Steps[0].Inputs[0],
	)
	if _, err = f.pipelines.InspectRun(ctx, missingJobAction, admitted.Operation.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("pipeline run action substituted for jobs.read", err)
	}
	readAuthority := f.token.envelope(t, f.e.Tenant(), f.e.User(),
		"jobs.read", "cw.source.write:"+definition.ID,
		"cw.source.query:"+source.ID, "cw.execution_context.use:"+source.ContextID,
		"cw.dataset.query:"+definition.Steps[0].Inputs[0],
	)
	if inspected, inspectErr := f.pipelines.InspectRun(ctx, readAuthority, admitted.Operation.ID); inspectErr != nil || inspected.Operation.ID != admitted.Operation.ID {
		t.Fatal("jobs.read plus original reach rejected", inspectErr, inspected)
	}
	missingDependency := f.token.envelope(t, f.e.Tenant(), f.e.User(),
		"jobs.cancel", "cw.source.write:"+definition.ID, "cw.source.query:"+source.ID,
		"cw.dataset.query:"+definition.Steps[0].Inputs[0],
	)
	if _, err = f.pipelines.Cancel(ctx, missingDependency, admitted.Operation.ID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("jobs.cancel omitted original context reach", err)
	}
	cancelAuthority := f.token.envelope(t, f.e.Tenant(), f.e.User(),
		"jobs.cancel", "cw.source.write:"+definition.ID,
		"cw.source.query:"+source.ID, "cw.execution_context.use:"+source.ContextID,
		"cw.dataset.query:"+definition.Steps[0].Inputs[0],
	)
	cancelled, err := f.pipelines.Cancel(ctx, cancelAuthority, admitted.Operation.ID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatal("jobs.cancel plus original reach rejected", err, cancelled)
	}

	run, err := f.pipelines.Run(ctx, runAuthority, definition.ID, draft.Version, "exact-authority-execute", false)
	if err != nil || run.State != "published" || len(run.Effects) != 2 {
		t.Fatal("exact run authority could not execute private stages", err, run)
	}
	for _, effect := range run.Effects {
		if _, bindingErr := f.s.Binding(ctx, runAuthority, effect.Source, effect.Context); !errors.Is(bindingErr, access.ErrNotFound) {
			t.Fatal("pipeline execution authority became ordinary output-read authority", bindingErr)
		}
	}
}

func TestPipelineServiceVerifiesEnabledRunnerAtConstruction(t *testing.T) {
	f := newPipelineFixture(t, nil, nil)
	changed := f.values
	changed.Pipelines.RunnerSHA256 = strings.Repeat("0", 64)
	if _, err := engineering.NewPipelineService(f.db, f.s, f.validator, nil, changed, f.lookup); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("enabled service accepted changed runner digest", err)
	}
	changed = f.values
	changed.Pipelines.RunnerPath = "/missing/chartworks-pipeline-runner"
	if _, err := engineering.NewPipelineService(f.db, f.s, f.validator, nil, changed, f.lookup); !errors.Is(err, engineering.ErrUnavailable) {
		t.Fatal("enabled service accepted missing runner", err)
	}
	changed.Pipelines.Enabled = false
	changed.Pipelines.RunnerSHA256 = ""
	disabled, err := engineering.NewPipelineService(f.db, f.s, f.validator, nil, changed, f.lookup)
	if err != nil {
		t.Fatal("disabled retained service resolved runner", err)
	}
	disabled.Close()
}

func TestPipelinePublishesTwoOutputsWithOneReadConnection(t *testing.T) {
	f := newPipelineFixture(t, nil, nil)
	f.values.Sources.MaxConns = 1
	rebuildPipelineSources(t, f)
	ctx := context.Background()
	source := f.create(t, "pool-one-source")
	definition := f.definition(t, source, "pool-one-pipeline")
	second := definition.Steps[0]
	second.ID = "second"
	second.Source = ""
	second.Context = ""
	second.Inputs = nil
	second.SQL = "SELECT id FROM {{step.output}}"
	second.DependsOn = []string{"output"}
	second.FromSteps = []string{"output"}
	definition.Steps = append(definition.Steps, second)
	draft, err := f.pipelines.Draft(ctx, f.e, definition, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pipelines.Publish(ctx, f.e, definition.ID, draft.Version); err != nil {
		t.Fatal(err)
	}
	run, err := f.pipelines.Run(ctx, f.e, definition.ID, draft.Version, "one-connection-two-outputs", false)
	if err != nil || run.State != "published" || len(run.Effects) != 2 {
		t.Fatal("publication borrowed more than one native read connection", err, run)
	}
	// A single metadata snapshot must expose the complete accepted manifest with
	// both current source revisions pointing at the same successful operation.
	metadata := support.Raw(t, f.dsn)
	var complete int
	if err = metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.pipeline_outputs p JOIN chartworks.sources s ON (s.tenant_id,s.source_id)=(p.tenant_id,p.source_id) JOIN chartworks.source_revisions r ON (r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision) WHERE p.tenant_id=$1 AND p.pipeline_id=$2 AND p.operation_id=$3 AND r.pipeline->>'Operation'=$3`, f.e.Tenant(), definition.ID, run.Operation.ID).Scan(&complete); err != nil || complete != 2 {
		t.Fatal("partial or mismatched output manifest", err, complete)
	}
	for _, effect := range run.Effects {
		if effect.Source == "" || effect.Context == "" || effect.Digest == "" {
			t.Fatal("published output lacks activation evidence")
		}
		binding, err := f.s.Binding(ctx, f.e, effect.Source, effect.Context)
		if err != nil || len(binding.Relations) != 1 {
			t.Fatal("published output binding", err)
		}
		discovery, err := f.s.Discover(ctx, f.e, effect.Source)
		if err != nil || discovery.ContextID != effect.Context || readexec.Hash(discovery.Relations) != readexec.Hash(binding.Relations) {
			t.Fatal("published output native discovery", err)
		}
		relation := binding.Relations[0]
		plan := f.plan(t, sources.Source{ID: effect.Source, ContextID: effect.Context}, "SELECT id FROM "+pgx.Identifier{relation.Schema, relation.Name}.Sanitize()+" ORDER BY id")
		rows, err := f.s.Read(ctx, f.e, plan)
		if err != nil || len(rows.Values) != 2 || rows.Values[0][0] == nil || *rows.Values[0][0] != "1" || rows.Values[1][0] == nil || *rows.Values[1][0] != "2" {
			t.Fatal("published stage contents", err, rows)
		}
	}
}
