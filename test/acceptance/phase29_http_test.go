package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

func TestDocumentHTTPContracts(t *testing.T) {
	f := newPhase18Fixture(t)
	ctx := context.Background()
	scopes := append(phase29DocumentScopes(), "reporting.execute", "cw.report.execute:*", "cw.dashboard.execute:*", "cw.run.read:*", "jobs.read", "jobs.cancel", "reporting.retention", "cw.tenant.erase:"+f.f.e.Tenant())
	author := phase27Actor(t, f, f.f.e.User(), scopes)
	documents, err := reporting.NewDocuments(f.f.db, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reporting.NewCompositions(documents, f.f.db, nil, nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := reportingapi.DocumentsRegistry()
	if err != nil || len(registry.Definitions()) != 26 {
		t.Fatal("closed report/dashboard transport inventory", err)
	}
	handler := reportingapi.DocumentsHandler(f.f.token.verifier, documents, runs, http.NotFoundHandler())
	server := httptest.NewServer(assertRegisteredWireSchemas(t, registry, handler))
	t.Cleanup(server.Close)
	bearer := phase27Token(t, f, author.User(), author.Session(), scopes)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	beforeSource, beforeModel := f.f.lookups.Load(), f.model.requests.Load()
	publish := func(kind sdk.DocumentKind, state sdk.DocumentState) sdk.DocumentState {
		t.Helper()
		revision := state.DraftRevision
		state, err := client.TransitionDocument(ctx, kind, state.ID, "review", sdk.DocumentTransitionRequest{ExpectedVersion: state.Version, Revision: revision, Note: "Review via HTTP"})
		if err != nil {
			t.Fatal("HTTP review", err)
		}
		if view, err := client.ReadDocument(ctx, kind, state.ID, sdk.DocumentReference{Stage: "review"}); err != nil || !view.Private || view.Revision != revision {
			t.Fatal("authorized pending-review projection", view, err)
		}
		state, err = client.TransitionDocument(ctx, kind, state.ID, "publish", sdk.DocumentTransitionRequest{ExpectedVersion: state.Version, Revision: revision, Note: "Publish via HTTP"})
		if err != nil {
			t.Fatal("HTTP publication", err)
		}
		return state
	}
	create := func(kind sdk.DocumentKind, id string, definition reporting.DocumentDefinition) sdk.DocumentState {
		t.Helper()
		state, err := client.CreateDocument(ctx, kind, sdk.DocumentCreateRequest{ID: id, Definition: definition})
		if err != nil {
			t.Fatal("HTTP create", kind, err)
		}
		if view, err := client.ReadDocument(ctx, kind, id, sdk.DocumentReference{Stage: "draft"}); err != nil || !view.Private || view.Revision != 1 {
			t.Fatal("HTTP private read", view, err)
		}
		return state
	}
	report := publish(sdk.ReportDocument, create(sdk.ReportDocument, "http-report", phase29Text("Wire-contract report")))
	dashboardDefinition := phase29Text("Wire-contract dashboard")
	dashboardDefinition.Widgets = nil
	dashboardDefinition.Pages = []reporting.DocumentPage{{ID: "report-page", Title: "Report page", Report: report.ID, Revision: report.PublishedRevision}}
	dashboard := publish(sdk.DashboardDocument, create(sdk.DashboardDocument, "http-dashboard", dashboardDefinition))
	for _, target := range []struct {
		kind       sdk.DocumentKind
		state      sdk.DocumentState
		definition reporting.DocumentDefinition
		page       string
	}{{sdk.ReportDocument, report, phase29Text("Report amendment"), "main"}, {sdk.DashboardDocument, dashboard, dashboardDefinition, "report-page"}} {
		kind, state := target.kind, target.state
		if listed, err := client.ListDocuments(ctx, kind, "", 20); err != nil || len(listed.Items) != 1 || listed.Items[0].ID != state.ID {
			t.Fatal("HTTP published listing", listed, err)
		}
		admitted, err := client.AdmitComposition(ctx, kind, state.ID, sdk.CompositionRequest{Key: "http-" + string(kind) + "-run"})
		if err != nil {
			t.Fatal("HTTP composition admission", err)
		}
		if receipt, err := client.InspectComposition(ctx, admitted.ID); err != nil || receipt.State != "sealed" || receipt.Complete {
			t.Fatal("HTTP sealed receipt", receipt, err)
		}
		completed, err := client.ExecuteComposition(ctx, admitted.ID, false)
		if err != nil || !completed.Complete || completed.State != "completed" {
			t.Fatal("HTTP composition execution", completed, err)
		}
		if view, err := client.ReadComposition(ctx, admitted.ID); err != nil || !view.Complete {
			t.Fatal("HTTP retained metadata", view, err)
		}
		if payload, err := client.ReadCompositionWidget(ctx, admitted.ID, target.page, "intro"); err != nil || payload.Text == nil || payload.State != "completed" {
			t.Fatal("HTTP retained text", payload, err)
		}
		pending, err := client.AdmitComposition(ctx, kind, state.ID, sdk.CompositionRequest{Key: "http-" + string(kind) + "-cancel"})
		if err != nil {
			t.Fatal(err)
		}
		if cancelled, err := client.CancelComposition(ctx, pending.ID); err != nil || cancelled.State != "cancelled" {
			t.Fatal("HTTP cancellation", cancelled, err)
		}
		state, err = client.EditDocument(ctx, kind, state.ID, sdk.DocumentEditRequest{ExpectedVersion: state.Version, From: sdk.DocumentReference{}, Definition: target.definition})
		if err != nil {
			t.Fatal("HTTP amendment", err)
		}
		revision := state.DraftRevision
		state, err = client.TransitionDocument(ctx, kind, state.ID, "review", sdk.DocumentTransitionRequest{ExpectedVersion: state.Version, Revision: revision, Note: "Amendment review"})
		if err != nil {
			t.Fatal(err)
		}
		state, err = client.TransitionDocument(ctx, kind, state.ID, "reject", sdk.DocumentTransitionRequest{ExpectedVersion: state.Version, Revision: revision, Note: "Retain original publication"})
		if err != nil || state.PublishedRevision != 1 {
			t.Fatal("HTTP rejection changed publication", state, err)
		}
		if _, err := client.TransitionDocument(ctx, kind, state.ID, "archive", sdk.DocumentTransitionRequest{ExpectedVersion: state.Version, Revision: 1, Note: "Archive original publication"}); err != nil {
			t.Fatal("HTTP archive", err)
		}
		unsupported, err := client.ImportDocument(ctx, kind, sdk.DocumentImportRequest{ID: "http-import-" + string(kind), DefinitionJSON: `{"schema_version":99}`, External: sdk.DocumentExternalReference{System: "fixture", ID: string(kind), Version: "v99"}})
		if err != nil || unsupported.Quarantine == "" || unsupported.State != nil {
			t.Fatal("HTTP private import quarantine", unsupported, err)
		}
	}
	if removed, err := client.ExpireCompositions(ctx, 10); err != nil || removed != 0 {
		t.Fatal("bounded retention endpoint", removed, err)
	}
	for _, bad := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/v1/reports?limit=01", "", 400},
		{"GET", "/v1/reports?limit=1&limit=2", "", 400},
		{"GET", "/v1/reports/http-report?revision=1&stage=draft", "", 400},
		{"GET", "/v1/reports/http-report", `{}`, 400},
		{"GET", "/v1/reports/http-report?actor=other", "", 400},
		{"POST", "/v1/reports/http-report/review", `{"expected_version":1,"revision":1,"note":"review","tenant":"other"}`, 400},
		{"POST", "/v1/composition-runs/nonexistent/execute", `{"resume":false,"actor":"other"}`, 400},
		{"GET", "/v1/composition-runs/nonexistent/widget?page=main&widget=../secret", "", 400},
	} {
		response := callProtected(t, handler, bad.method, bad.path, bearer, bad.body, map[string]string{"Content-Type": "application/json"})
		if response.Code != bad.status {
			t.Fatal("closed document boundary", bad.path, response.Code, response.Body.String())
		}
	}
	if response := callProtected(t, handler, "POST", "/v1/reports", "", `{}`, map[string]string{"Content-Type": "application/json"}); response.Code != 401 {
		t.Fatal("unauthenticated document route", response.Code)
	}
	encoded, _ := json.Marshal(report)
	if len(encoded) == 0 || beforeSource != f.f.lookups.Load() || beforeModel != f.model.requests.Load() {
		t.Fatal("text-only transport reached source/model work")
	}
}
