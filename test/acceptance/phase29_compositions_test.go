package acceptance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

func TestReportingCompositionStorage(t *testing.T) {
	f := newPhase18Fixture(t)
	ctx := context.Background()
	scopes := append(phase29DocumentScopes(), "reporting.execute", "cw.report.execute:*", "cw.dashboard.execute:*", "cw.run.read:*", "jobs.read", "jobs.cancel")
	author := phase27Actor(t, f, f.f.e.User(), scopes)
	documents, err := reporting.NewDocuments(f.f.db, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, f.f.db, nil, nil, runner)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("private-artifacts-stay-private-after-publication", func(t *testing.T) {
		state, err := documents.Create(ctx, author, "report", "composed-private", phase29Text("PRIVATE_PREVIEW_CANARY"))
		if err != nil {
			t.Fatal(err)
		}
		beforeSource, beforeModel := f.f.lookups.Load(), f.model.requests.Load()
		preview, err := compositions.Admit(ctx, author, "report", state.ID, reporting.CompositionRequest{Key: "private-composition", Preview: true, Reference: reporting.DocumentReference{Revision: 1}})
		if err != nil || !preview.Private || preview.Revision != 1 || preview.Complete {
			t.Fatal("seal exact private preview", preview, err)
		}
		preview, err = compositions.Run(ctx, author, preview.ID, false)
		if err != nil || preview.State != "completed" || !preview.Complete || !preview.Private {
			t.Fatal("execute text-only private preview", preview, err)
		}
		state = phase29Publish(t, documents, author, state)
		reader := phase27Actor(t, f, "public-report-reader", []string{"reporting.read", "cw.report.read:" + state.ID})
		if _, err := compositions.Get(ctx, reader, preview.ID); err == nil {
			t.Fatal("later publication exposed a previously private artifact")
		}
		stranger := phase27Actor(t, f, "foreign-preview-owner", []string{"reporting.read", "reporting.preview", "cw.run.read:*", "cw.report.preview:*"})
		if _, err := compositions.Get(ctx, stranger, preview.ID); err == nil {
			t.Fatal("preview reach impersonated the originating actor")
		}
		owner := phase27Actor(t, f, author.User(), []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + state.ID})
		view, err := compositions.Get(ctx, owner, preview.ID)
		if err != nil || !view.Private || !view.Complete {
			t.Fatal("read-only owner could not read retained preview", view, err)
		}
		if _, err := compositions.Run(ctx, owner, preview.ID, false); err == nil {
			t.Fatal("artifact-read reach became execution authority")
		}
		payload, err := compositions.Widget(ctx, owner, preview.ID, "main", "intro")
		if err != nil || payload.Text == nil || payload.State != "completed" {
			t.Fatal("retained private text", payload, err)
		}
		if beforeSource != f.f.lookups.Load() || beforeModel != f.model.requests.Load() {
			t.Fatal("text-only composition reached an unavailable source/model lane")
		}
	})

	t.Run("first-seal-replay-and-independent-retained-read", func(t *testing.T) {
		definition := phase29Text("Original public report")
		definition.Widgets[0].Text.Text = "ORIGINAL_TEXT_CANARY"
		state, err := documents.Create(ctx, author, "report", "composed-replay", definition)
		if err != nil {
			t.Fatal(err)
		}
		state = phase29Publish(t, documents, author, state)
		request := reporting.CompositionRequest{Key: "stable-composition-key"}
		admitted, err := compositions.Admit(ctx, author, "report", state.ID, request)
		if err != nil {
			t.Fatal(err)
		}
		definition.Widgets[0].Text.Text = "NEW_PRIVATE_TEXT_CANARY"
		state, err = documents.Edit(ctx, author, "report", state.ID, state.Version, reporting.DocumentReference{}, definition)
		if err != nil {
			t.Fatal(err)
		}
		state = phase29Publish(t, documents, author, state)
		replay, err := compositions.Admit(ctx, author, "report", state.ID, request)
		if err != nil || replay.ID != admitted.ID || replay.Manifest != admitted.Manifest || replay.Revision != 1 {
			t.Fatal("idempotent replay followed the new publication", replay, err)
		}
		completed, err := compositions.Run(ctx, author, admitted.ID, false)
		if err != nil || !completed.Complete || completed.Revision != 1 {
			t.Fatal("execute the original seal", completed, err)
		}
		reader := phase27Actor(t, f, "retained-only-reader", []string{"reporting.read", "cw.run.read:" + admitted.ID})
		payload, err := compositions.Widget(ctx, reader, admitted.ID, "main", "intro")
		if err != nil || payload.Text == nil || payload.Text.Text != "ORIGINAL_TEXT_CANARY" {
			t.Fatal("retained values were reconstructed from a newer definition", payload, err)
		}
		view, err := compositions.Get(ctx, reader, admitted.ID)
		if err != nil || !view.Complete {
			t.Fatal(view, err)
		}
		raw, _ := json.Marshal(view)
		if strings.Contains(string(raw), "TEXT_CANARY") {
			t.Fatal("metadata summary fetched/projected widget text")
		}
		db := support.Raw(t, f.f.dsn)
		var ledgerState string
		if err := db.QueryRow(ctx, `SELECT status FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, author.Tenant(), admitted.ID).Scan(&ledgerState); err != nil || ledgerState != "succeeded" {
			t.Fatal("composition and common ledger did not complete atomically", ledgerState, err)
		}
		if _, err := db.Exec(ctx, `UPDATE chartworks.composition_runs SET private=true WHERE tenant_id=$1 AND operation_id=$2`, author.Tenant(), admitted.ID); err == nil {
			t.Fatal("artifact privacy was mutable")
		}
	})

	t.Run("dashboard-retained-page-redaction-and-cancellation", func(t *testing.T) {
		first, err := documents.Create(ctx, author, "report", "composed-page-one", phase29Text("Public page"))
		if err != nil {
			t.Fatal(err)
		}
		first = phase29Publish(t, documents, author, first)
		second, err := documents.Create(ctx, author, "report", "composed-page-hidden", phase29Text("HIDDEN_REPORT_CANARY"))
		if err != nil {
			t.Fatal(err)
		}
		second = phase29Publish(t, documents, author, second)
		definition := phase29Text("Dashboard container")
		definition.Widgets = nil
		definition.Pages = []reporting.DocumentPage{{ID: "first", Title: "Visible page", Report: first.ID, Revision: 1}, {ID: "hidden", Title: "HIDDEN_PAGE_CANARY", Report: second.ID, Revision: 1}}
		state, err := documents.Create(ctx, author, "dashboard", "composed-dashboard", definition)
		if err != nil {
			t.Fatal(err)
		}
		state = phase29Publish(t, documents, author, state)
		run, err := compositions.Admit(ctx, author, "dashboard", state.ID, reporting.CompositionRequest{Key: "dashboard-composition"})
		if err != nil {
			t.Fatal(err)
		}
		run, err = compositions.Run(ctx, author, run.ID, false)
		if err != nil || !run.Complete || len(run.Pages) != 2 {
			t.Fatal("execute dashboard exact pages", run, err)
		}
		reader := phase27Actor(t, f, "page-limited", []string{"reporting.read", "cw.dashboard.read:" + state.ID, "cw.report.read:" + first.ID})
		view, err := compositions.Get(ctx, reader, run.ID)
		if err != nil || len(view.Pages) != 1 || view.Pages[0].ID != "first" || !view.Redacted || view.Complete {
			t.Fatal("retained page filtering", view, err)
		}
		raw, _ := json.Marshal(view)
		if strings.Contains(string(raw), "HIDDEN") || strings.Contains(string(raw), "composed-page-hidden") {
			t.Fatal("hidden retained page name/coordinate disclosed")
		}
		if _, err := compositions.Widget(ctx, reader, run.ID, "hidden", "intro"); err == nil {
			t.Fatal("direct widget address bypassed page reach")
		}
		none := phase27Actor(t, f, "no-retained-pages", []string{"reporting.read", "cw.dashboard.read:" + state.ID})
		view, err = compositions.Get(ctx, none, run.ID)
		if err != nil || len(view.Pages) != 0 || !view.Redacted {
			t.Fatal("zero-visible-page retained view", view, err)
		}
		pending, err := compositions.Admit(ctx, author, "dashboard", state.ID, reporting.CompositionRequest{Key: "cancelled-dashboard-composition"})
		if err != nil {
			t.Fatal(err)
		}
		cancelled, err := compositions.Cancel(ctx, author, pending.ID)
		if err != nil || cancelled.State != "cancelled" {
			t.Fatal("durable cancellation", cancelled, err)
		}
		if _, err := compositions.Run(ctx, author, pending.ID, false); err == nil {
			t.Fatal("cancelled composition executed")
		}
	})
}
