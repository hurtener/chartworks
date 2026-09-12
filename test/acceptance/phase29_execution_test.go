package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

type phase29ExecutionFixture struct {
	f            *phase17Fixture
	blocks       *reporting.Service
	runs         *reporting.Runs
	query        *nlqexec.Service
	documents    *reporting.Documents
	compositions *reporting.Compositions
	blockAuthor  identity.Envelope
	author       identity.Envelope
	execute      identity.Envelope
	base         reporting.Definition
	limits       config.Reporting
}

func phase29RuntimeScopes(tenant string) []string {
	base := slices.DeleteFunc(phase28Scopes(tenant), func(s string) bool {
		return strings.HasPrefix(s, "reporting.") || strings.HasPrefix(s, "cw.block.") || strings.HasPrefix(s, "cw.run.") || strings.HasPrefix(s, "cw.tenant.") || strings.HasPrefix(s, "jobs.") || strings.Contains(s, ".write") || strings.Contains(s, ".publish") || strings.Contains(s, ".certify")
	})
	return append(base, "reporting.read", "reporting.preview", "reporting.execute", "query.execute", "query.plan", "jobs.cancel",
		"cw.block.read:*", "cw.block.execute:*", "cw.block.preview:*",
		"cw.report.read:*", "cw.report.execute:*", "cw.report.preview:*",
		"cw.dashboard.read:*", "cw.dashboard.execute:*", "cw.dashboard.preview:*", "cw.run.read:*")
}

func phase29AuthorScopes(tenant string) []string {
	base := slices.DeleteFunc(phase29RuntimeScopes(tenant), func(s string) bool {
		return s == "reporting.execute" || s == "jobs.cancel" || strings.HasPrefix(s, "cw.") && strings.Contains(s, ".execute:")
	})
	return append(base, "reporting.write", "reporting.publish", "cw.report.write:*", "cw.report.publish:*", "cw.dashboard.write:*", "cw.dashboard.publish:*")
}

func newPhase29Execution(t *testing.T, live bool) *phase29ExecutionFixture {
	t.Helper()
	f := newPhase18Fixture(t)
	query, topics := newPhase18Service(t, f)
	limits := config.DefaultReporting()
	limits.Composition.LiveQueries, limits.Composition.SessionBound = live, live
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), limits)
	if err != nil {
		t.Fatal(err)
	}
	blockAuthor := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	runtimeScopes, authorScopes := phase29RuntimeScopes(f.f.e.Tenant()), phase29AuthorScopes(f.f.e.Tenant())
	if len(runtimeScopes) > 32 || len(authorScopes) > 32 {
		t.Fatalf("fixture must preserve the signed scope ceiling: runtime=%d author=%d", len(runtimeScopes), len(authorScopes))
	}
	author := phase27Actor(t, f, f.f.e.User(), authorScopes)
	execute := phase27Actor(t, f, f.f.e.User(), runtimeScopes)
	runs := phase28RunService(t, f, blocks, f.f.db, nil, limits.Execution)
	documents, err := reporting.NewDocuments(f.f.db, blocks, reporting.DocumentsFromQueries(query), limits)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, f.f.db, runs, reporting.DocumentsFromQueries(query), runner)
	if err != nil {
		t.Fatal(err)
	}
	base := phase27Definition(t, f, blockAuthor, "SELECT id, amount FROM analytics.sales ORDER BY id")
	base.Outputs = base.Outputs[:1]
	second := phase27Copy(t, base.Outputs[0])
	second.ID = "table-second"
	base.Outputs = append(base.Outputs, second)
	f.model.embeddingMode.Store("fixed")
	f.model.rerankMode.Store("fixed")
	f.model.mode.Store(phase18RawResponse(t, base.SQL))
	return &phase29ExecutionFixture{f: f, blocks: blocks, runs: runs, query: query, documents: documents, compositions: compositions, blockAuthor: blockAuthor, author: author, execute: execute, base: base, limits: limits}
}

func (f *phase29ExecutionFixture) block(t *testing.T, id string, definition reporting.Definition) {
	t.Helper()
	created, err := f.blocks.Create(context.Background(), f.blockAuthor, reporting.CreateRequest{ID: id, Definition: definition})
	if err != nil {
		t.Fatal("create governed fixture block", err)
	}
	phase27ValidatePublish(t, f.blocks, f.blockAuthor, created)
}

func (f *phase29ExecutionFixture) report(t *testing.T, id string, d reporting.DocumentDefinition, publish bool) reporting.DocumentState {
	t.Helper()
	state, err := f.documents.Create(context.Background(), f.author, "report", id, d)
	if err != nil {
		t.Fatal("create composed report", err)
	}
	if publish {
		state = phase29Publish(t, f.documents, f.author, state)
	}
	return state
}

func phase29BlockWidget(id, block string, row int, outputs ...string) reporting.Widget {
	return reporting.Widget{ID: id, Kind: "block", Grid: reporting.GridCell{Row: row, Width: 12, Height: 1}, Block: &reporting.BlockWidget{Block: block, Outputs: outputs}}
}

func (f *phase29ExecutionFixture) queryWidget() reporting.Widget {
	return reporting.Widget{ID: "dynamic", Kind: "query", Grid: reporting.GridCell{Row: 2, Width: 12, Height: 1}, Query: &reporting.QueryWidget{Durability: "replayable", Context: f.base.Context, Topics: phase27CopyNoTest(f.base.Topics), Question: "What is revenue?"}}
}

func phase27CopyNoTest[T any](value T) T {
	body, _ := json.Marshal(value)
	var out T
	_ = json.Unmarshal(body, &out)
	return out
}

func (f *phase29ExecutionFixture) attemptCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := support.Raw(t, f.f.f.dsn).QueryRow(context.Background(), `SELECT count(*) FROM chartworks.read_attempts WHERE tenant_id=$1`, f.execute.Tenant()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestReportingCompositionData(t *testing.T) {
	t.Run("single-query-union-and-exact-publication-seal", testPhase29SealedFanout)
	t.Run("dynamic-durability-and-independent-trust", testPhase29DynamicDurability)
	t.Run("block-preview-remains-private", testPhase29BlockPreview)
}

func testPhase29SealedFanout(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	block := phase27Copy(t, f.base)
	block.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
	block.Parameters = []reporting.Parameter{{Name: "minimum", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}, Min: "1", Max: "2"}}
	f.block(t, "shared-block", block)
	d := phase29Text("Stable shared-block composition")
	first := phase29BlockWidget("first", "shared-block", 1, "table-main")
	second := phase29BlockWidget("second", "shared-block", 2, "table-second")
	otherValues := phase29BlockWidget("filtered", "shared-block", 3, "table-main")
	otherValues.Literals = []reporting.Argument{{Name: "minimum", Value: reporting.Value{Literal: "2"}}}
	d.Widgets = append(d.Widgets, first, second, otherValues)
	state := f.report(t, "stable-report", d, true)
	before := f.attemptCount(t)
	admitted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "stable-fanout"})
	if err != nil || admitted.QueryGroups != 2 || admitted.Complete {
		t.Fatal("equivalent widgets were not grouped safely", admitted, err)
	}
	sealed, err := f.f.f.db.ReadComposition(ctx, f.execute, admitted.ID)
	if err != nil || len(sealed.Manifest.Groups) != 2 || len(sealed.Manifest.Groups[0].Outputs) != 2 || !reflect.DeepEqual(sealed.Manifest.Groups[0].Outputs, []string{"table-main", "table-second"}) {
		t.Fatal("selected output union was not sealed", sealed, err)
	}
	if f.attemptCount(t) != before {
		t.Fatal("composition admission executed a warehouse query")
	}
	// Publish a genuinely different result-producing block revision after the
	// report is sealed. The accepted run must use revision one, not follow it.
	old, err := f.blocks.Read(ctx, f.blockAuthor, "shared-block", reporting.Reference{})
	if err != nil {
		t.Fatal(err)
	}
	changed := phase27Copy(t, block)
	changed.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 AND id=2 ORDER BY id"
	amendment, err := f.blocks.Edit(ctx, f.blockAuthor, "shared-block", reporting.EditRequest{ExpectedVersion: old.State.Version, Definition: changed})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, f.blocks, f.blockAuthor, amendment)
	d.Widgets = d.Widgets[:1]
	state, err = f.documents.Edit(ctx, f.author, "report", state.ID, state.Version, reporting.DocumentReference{}, d)
	if err != nil {
		t.Fatal(err)
	}
	phase29Publish(t, f.documents, f.author, state)
	replay, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "stable-fanout"})
	if err != nil || replay.ID != admitted.ID || replay.Manifest != admitted.Manifest || replay.Revision != 1 {
		t.Fatal("replay re-resolved a new publication", replay, err)
	}
	before = f.attemptCount(t)
	completed, err := f.compositions.Run(ctx, f.execute, admitted.ID, false)
	if err != nil || !completed.Complete || completed.State != "completed" || !completed.MixedFreshness {
		t.Fatal("shared block execution", completed, err)
	}
	if f.attemptCount(t) != before+2 {
		t.Fatal("grouping either duplicated equivalent execution or shared unequal parameters")
	}
	retained, err := f.f.f.db.ReadComposition(ctx, f.execute, admitted.ID)
	if err != nil || len(retained.Results) != 2 {
		t.Fatal(err)
	}
	for _, result := range retained.Results {
		if result.Block == nil || result.Block.Revision != 1 || len(result.Block.QueryAttempts) != 1 {
			t.Fatal("child block did not use one exact retained revision/attempt", result)
		}
	}
	reader := phase27Actor(t, f.f, "read-only-composition", []string{"reporting.read", "cw.report.read:" + state.ID, "cw.execution_context.use:" + block.Context})
	beforeSource, beforeModel := f.f.f.lookups.Load(), f.f.model.requests.Load()
	for _, selected := range []struct{ widget, output string }{{"first", "table-main"}, {"second", "table-second"}, {"filtered", "table-main"}} {
		payload, err := f.compositions.Widget(ctx, reader, admitted.ID, "main", selected.widget)
		if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].ID != selected.output || payload.Outputs[0].Chart == nil {
			t.Fatal("widget received another selection from the shared union", payload, err)
		}
	}
	if _, err := f.compositions.Get(ctx, reader, admitted.ID); err != nil {
		t.Fatal("artifact read required execution/source-query authority", err)
	}
	if beforeSource != f.f.f.lookups.Load() || beforeModel != f.f.model.requests.Load() {
		t.Fatal("retained reads executed source/model work")
	}
}

func testPhase29BlockPreview(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "private-child", f.base)
	d := phase29Text("Private report preview")
	d.Widgets = append(d.Widgets, phase29BlockWidget("block", "private-child", 1, "table-main"))
	state := f.report(t, "private-block-report", d, false)
	preview, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "private-block-preview", Preview: true, Reference: reporting.DocumentReference{Revision: 1}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err = f.compositions.Run(ctx, f.execute, preview.ID, false)
	if err != nil || !preview.Private || !preview.Complete {
		t.Fatal("governed-block private preview", preview, err)
	}
	record, err := f.f.f.db.ReadComposition(ctx, f.execute, preview.ID)
	if err != nil || len(record.Results) != 1 || record.Results[0].Block == nil || !record.Results[0].Block.Private || !record.Results[0].Block.QueryAttempts[0].Manifest.Preview {
		t.Fatal("private parent produced a public child artifact", record, err)
	}
	phase29Publish(t, f.documents, f.author, state)
	withoutPreview := phase27Actor(t, f.f, f.execute.User(), []string{"reporting.read", "cw.report.read:*", "cw.run.read:*", "cw.execution_context.use:*"})
	if _, err := f.compositions.Get(ctx, withoutPreview, preview.ID); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("publication or creator metadata waived preview authority", err)
	}
	foreign := phase27Actor(t, f.f, "another-preview-reader", []string{"reporting.read", "reporting.preview", "cw.report.preview:*", "cw.run.read:*", "cw.execution_context.use:*"})
	if _, err := f.compositions.Widget(ctx, foreign, preview.ID, "main", "block"); err == nil {
		t.Fatal("private artifact crossed originating actor boundary")
	}
	owner := phase27Actor(t, f.f, f.execute.User(), []string{"reporting.read", "reporting.preview", "cw.report.preview:*", "cw.run.read:*", "cw.execution_context.use:*"})
	if payload, err := f.compositions.Widget(ctx, owner, preview.ID, "main", "block"); err != nil || len(payload.Outputs) != 1 {
		t.Fatal("authorized read-only preview lost retained values", payload, err)
	}
}

func testPhase29DynamicDurability(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := context.Background()
	f.block(t, "mixed-block", f.base)
	d := phase29Text("Mixed block, live query and text report")
	d.Widgets = append(d.Widgets, phase29BlockWidget("frozen", "mixed-block", 1, "table-main"), f.queryWidget())
	state := f.report(t, "mixed-report", d, true)
	beforeModels := f.f.model.requests.Load()
	admitted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "mixed-report-run"})
	if err != nil {
		t.Fatal("admit replayable dynamic query", err)
	}
	if f.f.model.requests.Load() != beforeModels {
		t.Fatal("manifest admission generated SQL before its durable marker")
	}
	completed, err := f.compositions.Run(ctx, f.execute, admitted.ID, false)
	if err != nil || !completed.Complete || len(completed.Pages) != 1 || len(completed.Pages[0].Widgets) != 3 {
		t.Fatal("execute mixed query/block/text report", completed, err)
	}
	dynamic := completed.Pages[0].Widgets[2]
	if dynamic.Kind != "query" || dynamic.Trust != nil || dynamic.Durability != "replayable" || dynamic.QueryDigest == "" || dynamic.SemanticDigest == "" || dynamic.Observed == nil {
		t.Fatal("dynamic query inherited block certification or lost provenance", dynamic)
	}
	record, err := f.f.f.db.ReadComposition(ctx, f.execute, admitted.ID)
	if err != nil || len(record.Results) != 2 || len(record.Plans) != 1 {
		t.Fatal("dynamic checkpoints missing", err)
	}
	queryResult := record.Results[1]
	if queryResult.Query == nil || queryResult.Query.Execution.Result == nil || len(queryResult.Query.Execution.Result.Rows) != 2 || queryResult.Query.Execution.Attempt.ID == "" || !record.Started[queryResult.Group] {
		t.Fatal("dynamic result was not executed and durably checkpointed", queryResult)
	}
	beforeSource, beforeModel := f.f.f.lookups.Load(), f.f.model.requests.Load()
	reader := phase27Actor(t, f.f, "query-artifact-reader", []string{"reporting.read", "cw.report.read:" + state.ID, "cw.execution_context.use:" + f.base.Context})
	payload, err := f.compositions.Widget(ctx, reader, admitted.ID, "main", "dynamic")
	if err != nil || payload.Query == nil || payload.Query.QueryDigest != dynamic.QueryDigest {
		t.Fatal("dynamic artifact read required generation authority", payload, err)
	}
	if _, err := f.compositions.Run(ctx, f.execute, admitted.ID, false); err != nil {
		t.Fatal("terminal composition replay", err)
	}
	if beforeSource != f.f.f.lookups.Load() || beforeModel != f.f.model.requests.Load() {
		t.Fatal("terminal replay or retained widget read regenerated data")
	}
	// A session-bound widget clones the original governed definition, rather
	// than impersonating an author or rerunning the original query operation.
	sessionDefinition := phase29Text("Session-bound private report")
	sessionWidget := f.queryWidget()
	sessionWidget.Query.Durability = "session_bound"
	sessionWidget.Query.Question = ""
	sessionWidget.Query.Query = queryResult.Query.Query
	sessionDefinition.Widgets = append(sessionDefinition.Widgets, sessionWidget)
	sessionState := f.report(t, "session-bound-report", sessionDefinition, false)
	sessionRun, err := f.compositions.Admit(ctx, f.execute, "report", sessionState.ID, reporting.CompositionRequest{Key: "session-bound-preview", Preview: true, Reference: reporting.DocumentReference{Revision: 1}})
	if err != nil {
		t.Fatal("admit exact originating-session query", err)
	}
	beforeModel = f.f.model.requests.Load()
	sessionRun, err = f.compositions.Run(ctx, f.execute, sessionRun.ID, false)
	if err != nil || !sessionRun.Complete || !sessionRun.Private || sessionRun.Pages[0].Widgets[1].Durability != "session_bound" {
		t.Fatal("session-bound preview execution", sessionRun, err)
	}
	if f.f.model.requests.Load() != beforeModel {
		t.Fatal("session-bound saved SQL was regenerated")
	}
	sc, _ := store.NewScope(f.execute.Tenant(), f.execute.User())
	original, err := f.f.f.db.ReadQuery(ctx, sc, queryResult.Query.Query)
	if err != nil || original.Operation != queryResult.QueryPlan.Operation || original.Result == nil {
		t.Fatal("session-bound execution mutated its origin", err)
	}
	foreign := phase27Actor(t, f.f, "different-query-owner", phase29RuntimeScopes(f.execute.Tenant()))
	if _, err := f.compositions.Run(ctx, foreign, sessionRun.ID, false); err == nil {
		t.Fatal("session-bound execution impersonated the original owner")
	}
}
