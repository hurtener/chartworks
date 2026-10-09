package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// TestReportAppAuthoring uses real PostgreSQL and the actual revision/composition
// domain with no source adapter or model gateway configured.
func TestReportAppAuthoring(t *testing.T) {
	ctx := t.Context()
	token := newTokenFixture(t)
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	policy(t, db, support.Scope(t, "tenant", "author"))
	documents, err := reporting.NewDocuments(db, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, db, nil, nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	service, err := reporting.NewAuthoring(documents, compositions)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.execute", "reporting.publish", "mcp.use", "cw.tenant.write:tenant"}
	for _, id := range []string{"a-private", "b-private", "c-private"} {
		for _, permission := range []string{"read", "write", "preview", "execute", "publish"} {
			scopes = append(scopes, "cw.report."+permission+":"+id)
		}
	}
	bearer := func(tenant, user string, scopes []string, mcp bool) string {
		claims := token.claims(tenant, user, scopes)
		if mcp {
			claims["aud"] = token.cfg.MCPAudience()
		}
		return token.sign(t, claims, nil)
	}
	actor := func(tenant, user string, scopes []string) identity.Envelope {
		e, err := token.verifier.Verify(ctx, bearer(tenant, user, scopes, false), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	author := actor("tenant", "author", scopes)
	definition := phase29Text("Manual report")
	definition.Widgets = append(definition.Widgets, reporting.Widget{ID: "second", Kind: "text", Grid: reporting.GridCell{Row: 1, Width: 12, Height: 1}, Text: &reporting.TextWidget{Format: "plain", Text: "Preserve second widget"}})
	registry, err := reportingapi.AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, reportingapi.AuthoringHandler(token.verifier, service, http.NotFoundHandler()))
	call := func(suffix string, in any, want int) []byte {
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		response := callProtected(t, handler, "POST", reportingapi.AuthoringPath+suffix, bearer("tenant", "author", scopes, false), string(raw), map[string]string{"Content-Type": "application/json"})
		if response.Code != want {
			t.Fatalf("%s returned %d, want %d: %s", suffix, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	var state reporting.DocumentState
	if err = json.Unmarshal(call("create", reporting.AuthoringCreateRequest{ID: "a-private", Definition: definition}, http.StatusOK), &state); err != nil {
		t.Fatal(err)
	}
	if state.DraftRevision != 1 || state.PublishedRevision != 0 {
		t.Fatal("new report was not private", state)
	}
	for _, id := range []string{"b-private", "c-private"} {
		if _, err := service.Create(ctx, author, reporting.AuthoringCreateRequest{ID: id, Definition: definition}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("draft-catalog-preprojection-and-public-catalog-unchanged", func(t *testing.T) {
		public, err := documents.List(ctx, author, "report", "", 10)
		if err != nil || len(public.Items) != 0 {
			t.Fatal("published catalog exposed drafts", public, err)
		}
		one := []string{"reporting.read", "reporting.write", "cw.report.read:c-private", "cw.report.write:c-private"}
		page, err := service.Drafts(ctx, actor("tenant", "author", one), reporting.DraftListRequest{Limit: 1})
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != "c-private" || page.Next != "" {
			t.Fatal("target selection after pagination", page, err)
		}
		page, err = service.Drafts(ctx, author, reporting.DraftListRequest{Limit: 1})
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != "a-private" || page.Next != "a-private" {
			t.Fatal(page, err)
		}
		page, err = service.Drafts(ctx, author, reporting.DraftListRequest{After: page.Next, Limit: 1})
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != "b-private" || page.Next != "b-private" {
			t.Fatal(page, err)
		}
		foreign, err := service.Drafts(ctx, actor("foreign", "author", one), reporting.DraftListRequest{Limit: 10})
		if err != nil || len(foreign.Items) != 0 {
			t.Fatal("cross-tenant catalog", foreign, err)
		}
		// A persisted dependency partitions metadata too, before pagination.
		raw := support.Raw(t, dsn)
		sql(t, raw, `INSERT INTO chartworks.document_references VALUES('tenant','report','b-private',1,'execution_context','use','restricted:v1')`)
		page, err = service.Drafts(ctx, author, reporting.DraftListRequest{After: "a-private", Limit: 1})
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != "c-private" || page.Next != "" {
			t.Fatal("dependency reach checked after projection", page, err)
		}
		page, err = service.Drafts(ctx, actor("tenant", "author", append(slices.Clone(scopes), "cw.execution_context.use:restricted:v1")), reporting.DraftListRequest{After: "a-private", Limit: 1})
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != "b-private" {
			t.Fatal("exact dependency reach unavailable", page, err)
		}
	})

	t.Run("reopen-private-authority-and-selected-widget-patch", func(t *testing.T) {
		restricted := actor("tenant", "author", []string{"reporting.read", "reporting.write", "cw.report.read:a-private", "cw.report.write:a-private"})
		if _, err := service.Read(ctx, restricted, reporting.AuthoringReadRequest{Report: state.ID}); err == nil {
			t.Fatal("write bypassed private preview")
		}
		before, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
		if err != nil {
			t.Fatal(err)
		}
		patch := reporting.AuthoringWidgetRequest{Report: state.ID, Widget: "intro", ExpectedVersion: state.Version, Revision: state.DraftRevision, Patch: reporting.WidgetPatch{Text: &reporting.TextWidget{Format: "plain", Text: "Only selected widget changed"}}}
		if err = json.Unmarshal(call("widget", patch, http.StatusOK), &state); err != nil {
			t.Fatal(err)
		}
		after, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
		if err != nil || after.Revision != 2 {
			t.Fatal(after, err)
		}
		expected := phase27Copy(t, before.Definition)
		expected.Widgets[0].Text = patch.Patch.Text
		if !reflect.DeepEqual(after.Definition, expected) {
			t.Fatal("patch modified an untargeted field")
		}
		if _, err := service.PatchWidget(ctx, author, patch); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale patch bypassed CAS", err)
		}
		patch.ExpectedVersion = state.Version
		if _, err := service.PatchWidget(ctx, author, patch); !errors.Is(err, store.ErrConflict) {
			t.Fatal("old baseline plus fresh CAS reverted untargeted fields", err)
		}
		patch.ExpectedVersion, patch.Revision, patch.Widget = state.Version, state.DraftRevision, "missing"
		if _, err := service.PatchWidget(ctx, author, patch); err == nil {
			t.Fatal("unknown widget accepted")
		}
		raw, _ := json.Marshal(patch)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		payload["definition"] = definition
		call("widget", payload, http.StatusBadRequest)
		payload = map[string]any{"report": state.ID, "widget": "intro", "expected_version": state.Version, "revision": state.DraftRevision, "patch": map[string]any{"grid": map[string]int{"row": 9}}}
		call("widget", payload, http.StatusBadRequest)
	})

	t.Run("save-concurrent-cas-and-reopen", func(t *testing.T) {
		current, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
		if err != nil {
			t.Fatal(err)
		}
		edit := reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: current.Definition}
		edit.Definition.Metadata[0].Title = "Saved through CAS"
		var wg sync.WaitGroup
		results := make(chan error, 4)
		for range 4 {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := service.Save(ctx, author, edit); results <- err }()
		}
		wg.Wait()
		close(results)
		won, conflicts := 0, 0
		for err := range results {
			if err == nil {
				won++
			} else if errors.Is(err, store.ErrConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if won != 1 || conflicts != 3 {
			t.Fatal("CAS admitted multiple saves", won, conflicts)
		}
		view, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
		if err != nil || view.Definition.Metadata[0].Title != "Saved through CAS" || view.State.PublishedRevision != 0 {
			t.Fatal(view, err)
		}
		state = view.State
	})

	t.Run("private-preview-without-model-or-source-and-publication-isolation", func(t *testing.T) {
		var preview reporting.CompositionView
		if err = json.Unmarshal(call("preview", reporting.AuthoringPreviewRequest{Report: state.ID, Key: "manual-preview", Revision: state.DraftRevision}, http.StatusOK), &preview); err != nil {
			t.Fatal(err)
		}
		if !preview.Private || preview.Complete {
			t.Fatal("admission executed or made preview public", preview)
		}
		if err = json.Unmarshal(call("execute", reporting.AuthoringExecuteRequest{Run: preview.ID}, http.StatusOK), &preview); err != nil {
			t.Fatal(err)
		}
		if !preview.Private || !preview.Complete || preview.State != "completed" {
			t.Fatal(preview)
		}
		if _, err := compositions.Widget(ctx, author, preview.ID, "main", "intro"); err == nil {
			t.Fatal("private artifact read bypassed exact run authority")
		}
		// The trusted host obtains a fresh exact Pengui run grant after admission.
		retainedReader := actor("tenant", "author", append(slices.Clone(scopes), "cw.run.read:"+preview.ID))
		payload, err := compositions.Widget(ctx, retainedReader, preview.ID, "main", "intro")
		if err != nil || payload.Text == nil || payload.Text.Text != "Only selected widget changed" {
			t.Fatal(payload, err)
		}
		state = phase29Publish(t, documents, author, state)
		if _, err := compositions.Get(ctx, actor("tenant", "reader", []string{"reporting.read", "cw.report.read:a-private"}), preview.ID); err == nil {
			t.Fatal("publication exposed retained private preview")
		}
		drafts, err := service.Drafts(ctx, author, reporting.DraftListRequest{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range drafts.Items {
			if item.ID == state.ID {
				t.Fatal("published pointer remained draft catalog")
			}
		}
	})

	t.Run("strict-mcp-shared-domain-and-authority-negatives", func(t *testing.T) {
		bindings, err := reportingapi.AuthoringMCPBindings(service)
		if err != nil {
			t.Fatal(err)
		}
		registry, err := mcpserver.NewRegistry(bindings)
		if err != nil {
			t.Fatal(err)
		}
		server, err := mcpserver.New(token.verifier, registry, config.DefaultMCP(), nil)
		if err != nil {
			t.Fatal(err)
		}
		current := bearer("tenant", "author", scopes, true)
		client, err := server.Client(func(context.Context) (string, error) { return current, nil })
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.CallTool(ctx, "reporting_authoring_read_v1", json.RawMessage(`{"report":"a-private","revision":3}`))
		if err != nil || result == nil || result.IsError {
			t.Fatal("MCP cannot reopen HTTP-authored report", result, err)
		}
		for _, raw := range []string{`{"report":"a-private","revision":3,"profile":"builder"}`, `{"report":"a-private","revision":3,"tenant":"foreign"}`} {
			result, err = client.CallTool(ctx, "reporting_authoring_read_v1", json.RawMessage(raw))
			if err != nil || result == nil || !result.IsError {
				t.Fatal("MCP accepted client authority", result, err)
			}
		}
		current = bearer("tenant", "author", []string{"mcp.use", "reporting.write", "reporting.read", "cw.report.read:a-private"}, true)
		result, err = client.CallTool(ctx, "reporting_authoring_read_v1", json.RawMessage(`{"report":"a-private","revision":3}`))
		if err != nil || result == nil || !result.IsError {
			t.Fatal("creator/read granted write", result, err)
		}
		broad := actor("tenant", "author", []string{"reporting.read", "reporting.write", "cw.report.write:*"})
		if _, err := service.Drafts(ctx, broad, reporting.DraftListRequest{Limit: 10}); err == nil {
			t.Fatal("wildcard editing session accepted")
		}
		bad := phase29Text("No live generation")
		bad.Widgets[0] = reporting.Widget{ID: "live", Kind: "query", Query: &reporting.QueryWidget{Durability: "replayable", Question: "Unbounded caller prompt"}}
		if _, err := service.Create(ctx, author, reporting.AuthoringCreateRequest{ID: "a-private", Definition: bad}); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("manual lane accepted query generation", err)
		}
		bad.Widgets[0] = reporting.Widget{ID: "narrative", Kind: "block", Block: &reporting.BlockWidget{Block: "block", Narrative: true}}
		if _, err := service.Save(ctx, author, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.PublishedRevision, Definition: bad}); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("manual lane accepted narrative", err)
		}
	})
}

func TestReportAppAuthoringBlockReach(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	f.block(t, "manual-block", f.base)
	snapshot, err := f.f.f.db.ReadBlock(ctx, f.author, "manual-block", reporting.Reference{}, reporting.Read)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "cw.tenant.write:" + f.author.Tenant(), "cw.report.write:manual-data", "cw.report.read:manual-data", "cw.report.preview:manual-data", "cw.block.read:manual-block"}
	for _, ref := range snapshot.References {
		scope := "cw." + ref.Kind + "." + ref.Permission + ":" + ref.ID
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	service, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	definition := phase29Text("Governed widget")
	definition.Widgets = []reporting.Widget{phase29BlockWidget("selected", "manual-block", 0, f.base.Outputs[0].ID)}
	definition.Widgets[0].Block.Revision = snapshot.Revision.Number
	beforeSource, beforeModel := f.f.f.lookups.Load(), f.f.model.requests.Load()
	author := phase27Actor(t, f.f, f.author.User(), scopes)
	state, err := service.Create(ctx, author, reporting.AuthoringCreateRequest{ID: "manual-data", Definition: definition})
	if err != nil {
		t.Fatal("exact resource closure cannot author approved block", err)
	}
	if _, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID}); err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"cw.block.read:manual-block", "cw.execution_context.use:" + f.base.Context} {
		denied := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(scope string) bool { return scope == missing }))
		if _, err := service.Save(ctx, denied, reporting.AuthoringSaveRequest{Report: state.ID, Revision: state.DraftRevision, ExpectedVersion: state.Version, Definition: definition}); err == nil {
			t.Fatal("selected dependency reach bypassed", missing)
		}
		page, err := service.Drafts(ctx, denied, reporting.DraftListRequest{Limit: 10})
		if err != nil || len(page.Items) != 0 {
			t.Fatal("draft dependency metadata leaked", page, err)
		}
	}
	if beforeSource != f.f.f.lookups.Load() || beforeModel != f.f.model.requests.Load() {
		t.Fatal("manual authoring or denial performed source/model work")
	}

	// An explicit preview uses the already approved block and its actual source
	// partition. No query-generation or narrative action is granted.
	executionScopes := append(slices.Clone(scopes), "reporting.execute", "sources.read", "sources.query", "topics.read",
		"cw.report.execute:manual-data", "cw.block.execute:manual-block", "cw.block.preview:manual-block", "cw.source.query:"+f.base.Source)
	executor := phase27Actor(t, f.f, author.User(), executionScopes)
	beforeAttempts := f.attemptCount(t)
	preview, err := service.Preview(ctx, executor, reporting.AuthoringPreviewRequest{Report: state.ID, Key: "manual-block-preview", Revision: state.DraftRevision})
	if err != nil || !preview.Private || preview.Complete || preview.QueryGroups != 1 {
		t.Fatal("explicit manual block preview admission", preview, err)
	}
	if f.attemptCount(t) != beforeAttempts {
		t.Fatal("preview admission executed a source query")
	}
	for _, missing := range []string{"cw.block.execute:manual-block", "cw.execution_context.use:" + f.base.Context} {
		denied := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(executionScopes), func(scope string) bool { return scope == missing }))
		beforeDeniedSource := f.f.f.lookups.Load()
		if _, err := service.Execute(ctx, denied, reporting.AuthoringExecuteRequest{Run: preview.ID}); err == nil {
			t.Fatal("execution accepted missing selected dependency authority", missing)
		}
		if f.f.f.lookups.Load() != beforeDeniedSource || f.attemptCount(t) != beforeAttempts || f.f.model.requests.Load() != beforeModel {
			t.Fatal("denied execution performed source/model work", missing)
		}
	}
	completed, err := service.Execute(ctx, executor, reporting.AuthoringExecuteRequest{Run: preview.ID})
	if err != nil || !completed.Private || !completed.Complete || completed.State != "completed" {
		t.Fatal("manual approved-block preview execution", completed, err)
	}
	if f.attemptCount(t) != beforeAttempts+1 || f.f.model.requests.Load() != beforeModel {
		t.Fatal("frozen preview did not use exactly one source attempt with zero model calls")
	}
	record, err := f.f.f.db.ReadComposition(ctx, executor, preview.ID)
	if err != nil || len(record.Results) != 1 || record.Results[0].Block == nil || !record.Results[0].Block.Private || record.Results[0].Block.Revision != snapshot.Revision.Number {
		t.Fatal("private preview lost the exact approved child revision", record, err)
	}

	// The host refreshes only the returned run and original private/context
	// authority. Retained reads need no authoring, execution or source-query grant.
	readerScopes := []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID,
		"cw.report.preview:" + state.ID, "cw.execution_context.use:" + f.base.Context}
	reader := phase27Actor(t, f.f, author.User(), readerScopes)
	beforeRetainedSource := f.f.f.lookups.Load()
	view, err := f.compositions.Get(ctx, reader, preview.ID)
	if err != nil || !view.Private || !view.Complete {
		t.Fatal("exact refreshed authority cannot reopen retained preview", view, err)
	}
	payload, err := f.compositions.Widget(ctx, reader, preview.ID, "main", "selected")
	if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].ID != f.base.Outputs[0].ID || payload.Outputs[0].Chart == nil {
		t.Fatal("retained manual block output selection", payload, err)
	}
	rows := payload.Outputs[0].Chart.Rows
	if len(rows) != 2 || len(rows[0]) != 2 || rows[0][0].Value != "1" || rows[0][1].Value != "9007199254740993.125" {
		t.Fatal("retained approved-block values lost exact source semantics", rows)
	}
	for _, missing := range []string{"cw.run.read:" + preview.ID, "cw.execution_context.use:" + f.base.Context} {
		denied := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(readerScopes), func(scope string) bool { return scope == missing }))
		if _, err := f.compositions.Widget(ctx, denied, preview.ID, "main", "selected"); err == nil {
			t.Fatal("retained output ignored exact run/context reach", missing)
		}
	}
	if f.f.f.lookups.Load() != beforeRetainedSource || f.attemptCount(t) != beforeAttempts+1 || f.f.model.requests.Load() != beforeModel {
		t.Fatal("retained reads or denials re-executed source/model work")
	}
}
