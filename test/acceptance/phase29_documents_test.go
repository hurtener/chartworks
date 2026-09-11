package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func phase29DocumentScopes() []string {
	return []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.publish",
		"cw.report.read:*", "cw.report.write:*", "cw.report.preview:*", "cw.report.publish:*",
		"cw.dashboard.read:*", "cw.dashboard.write:*", "cw.dashboard.preview:*", "cw.dashboard.publish:*"}
}

func phase29Text(title string) reporting.DocumentDefinition {
	return reporting.DocumentDefinition{SchemaVersion: reporting.DocumentVersion, Locale: "en-US", Timezone: "UTC", PartialFailure: "fail_closed",
		Metadata: []reporting.DocumentMetadata{{Locale: "en-US", Title: title}, {Locale: "es-AR", Title: "Informe sintético"}},
		Widgets: []reporting.Widget{{ID: "intro", Kind: "text", Grid: reporting.GridCell{Width: 12, Height: 1}, Text: &reporting.TextWidget{Format: "markdown", Text: "# Retained evidence\n\nNo warehouse execution is needed to read this."}}}}
}

func phase29Publish(t *testing.T, s *reporting.Documents, e identity.Envelope, state reporting.DocumentState) reporting.DocumentState {
	t.Helper()
	ctx := context.Background()
	revision := state.DraftRevision
	state, err := s.Transition(ctx, e, state.Kind, state.ID, state.Version, revision, "review", "Ready for review")
	if err != nil {
		t.Fatal("review document", err)
	}
	state, err = s.Transition(ctx, e, state.Kind, state.ID, state.Version, revision, "publish", "Reviewed definition")
	if err != nil {
		t.Fatal("publish document", err)
	}
	return state
}

func TestDocumentStorage(t *testing.T) {
	f := newPhase18Fixture(t)
	ctx := context.Background()
	author := phase27Actor(t, f, f.f.e.User(), phase29DocumentScopes())
	documents, err := reporting.NewDocuments(f.f.db, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("independent-pointers-private-preview-and-cas", func(t *testing.T) {
		state, err := documents.Create(ctx, author, "report", "doc-lifecycle", phase29Text("Public candidate"))
		if err != nil || state.Version != 1 || state.DraftRevision != 1 || state.PublishedRevision != 0 {
			t.Fatal("create private draft", state, err)
		}
		creatorWithoutPreview := phase27Actor(t, f, author.User(), []string{"reporting.read", "cw.report.read:*"})
		if _, err := documents.Read(ctx, creatorWithoutPreview, "report", state.ID, reporting.DocumentReference{Revision: 1}); err == nil {
			t.Fatal("creator metadata granted a private preview")
		}
		state, err = documents.Transition(ctx, author, "report", state.ID, 1, 1, "review", "Review first revision")
		if err != nil || state.DraftRevision != 0 || state.ReviewRevision != 1 {
			t.Fatal("review pointer", state, err)
		}
		state, err = documents.Edit(ctx, author, "report", state.ID, state.Version, reporting.DocumentReference{Stage: "review"}, phase29Text("Private amendment"))
		if err != nil || state.ReviewRevision != 1 || state.DraftRevision != 2 {
			t.Fatal("amendment replaced pending review", state, err)
		}
		state, err = documents.Transition(ctx, author, "report", state.ID, state.Version, 1, "publish", "Publish reviewed first revision")
		if err != nil || state.PublishedRevision != 1 || state.ReviewRevision != 0 || state.DraftRevision != 2 {
			t.Fatal("publication collapsed independent pointers", state, err)
		}
		public, err := documents.Read(ctx, creatorWithoutPreview, "report", state.ID, reporting.DocumentReference{})
		if err != nil || public.Private || public.Definition.Metadata[0].Title != "Public candidate" || public.State.DraftRevision != 0 || public.State.ReviewRevision != 0 {
			t.Fatal("public projection leaked private amendment", public, err)
		}
		if _, err := documents.Read(ctx, creatorWithoutPreview, "report", state.ID, reporting.DocumentReference{Revision: 2}); err == nil {
			t.Fatal("publication exposed a different private revision")
		}
		var wg sync.WaitGroup
		results := make(chan error, 6)
		for range 6 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := documents.Edit(ctx, author, "report", state.ID, state.Version, reporting.DocumentReference{Revision: 2}, phase29Text("Concurrent amendment"))
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		won, conflicted := 0, 0
		for err := range results {
			if err == nil {
				won++
			} else if errors.Is(err, store.ErrConflict) {
				conflicted++
			} else {
				t.Fatal("unexpected CAS failure", err)
			}
		}
		if won != 1 || conflicted != 5 {
			t.Fatal("CAS admitted multiple concurrent edits", won, conflicted)
		}
	})

	t.Run("reject-amend-archive", func(t *testing.T) {
		state, err := documents.Create(ctx, author, "report", "doc-rejected", phase29Text("Review candidate"))
		if err != nil {
			t.Fatal(err)
		}
		state, err = documents.Transition(ctx, author, "report", state.ID, state.Version, 1, "review", "Review")
		if err != nil {
			t.Fatal(err)
		}
		state, err = documents.Transition(ctx, author, "report", state.ID, state.Version, 1, "reject", "Needs amendment")
		if err != nil || state.ReviewRevision != 0 || state.DraftRevision != 1 {
			t.Fatal(state, err)
		}
		if _, err := documents.Transition(ctx, author, "report", state.ID, state.Version, 1, "review", "Unchanged resubmission"); !errors.Is(err, store.ErrConflict) {
			t.Fatal("rejected content bypassed amendment", err)
		}
		state, err = documents.Edit(ctx, author, "report", state.ID, state.Version, reporting.DocumentReference{Stage: "draft"}, phase29Text("Amended candidate"))
		if err != nil {
			t.Fatal(err)
		}
		state = phase29Publish(t, documents, author, state)
		state, err = documents.Transition(ctx, author, "report", state.ID, state.Version, state.PublishedRevision, "archive", "Retired report")
		if err != nil || !state.Archived {
			t.Fatal("archive", state, err)
		}
		if _, err := documents.Edit(ctx, author, "report", state.ID, state.Version, reporting.DocumentReference{Revision: state.PublishedRevision}, phase29Text("Forbidden edit")); !errors.Is(err, store.ErrConflict) {
			t.Fatal("archived document edited", err)
		}
	})

	t.Run("page-redaction-before-payload-and-zero-visible", func(t *testing.T) {
		visible, err := documents.Create(ctx, author, "report", "doc-visible", phase29Text("Visible report"))
		if err != nil {
			t.Fatal(err)
		}
		visible = phase29Publish(t, documents, author, visible)
		hidden, err := documents.Create(ctx, author, "report", "doc-hidden-canary", phase29Text("HIDDEN_REPORT_NAME_CANARY"))
		if err != nil {
			t.Fatal(err)
		}
		hidden = phase29Publish(t, documents, author, hidden)
		d := phase29Text("Dashboard")
		d.Widgets = nil
		d.Pages = []reporting.DocumentPage{{ID: "visible-page", Title: "Visible", Report: visible.ID, Revision: visible.PublishedRevision}, {ID: "hidden-page-canary", Title: "HIDDEN_PAGE_NAME_CANARY", Report: hidden.ID, Revision: hidden.PublishedRevision}}
		state, err := documents.Create(ctx, author, "dashboard", "doc-dashboard", d)
		if err != nil {
			t.Fatal(err)
		}
		state = phase29Publish(t, documents, author, state)
		limited := phase27Actor(t, f, "document-reader", []string{"reporting.read", "cw.dashboard.read:" + state.ID, "cw.report.read:" + visible.ID})
		beforeSource, beforeModel := f.f.lookups.Load(), f.model.requests.Load()
		view, err := documents.Read(ctx, limited, "dashboard", state.ID, reporting.DocumentReference{})
		if err != nil || len(view.Definition.Pages) != 1 || view.Definition.Pages[0].Report != visible.ID {
			t.Fatal("page redaction", view, err)
		}
		raw, _ := json.Marshal(view)
		if strings.Contains(string(raw), "CANARY") || strings.Contains(string(raw), "hidden") {
			t.Fatal("redaction disclosed hidden page coordinates or names")
		}
		if _, err := f.f.db.ReadDocument(ctx, limited, "dashboard", state.ID, reporting.DocumentReference{}, reporting.Read, false); err == nil {
			t.Fatal("full-reference repository read bypassed page restrictions")
		}
		none := phase27Actor(t, f, "no-pages-reader", []string{"reporting.read", "cw.dashboard.read:" + state.ID})
		view, err = documents.Read(ctx, none, "dashboard", state.ID, reporting.DocumentReference{})
		if err != nil || len(view.Definition.Pages) != 0 {
			t.Fatal("zero visible pages rejected", view, err)
		}
		if _, err := documents.List(ctx, limited, "dashboard", "", 10); err != nil {
			t.Fatal(err)
		}
		if beforeSource != f.f.lookups.Load() || beforeModel != f.model.requests.Load() {
			t.Fatal("metadata reads accessed a warehouse or model")
		}
	})

	t.Run("immutable-legacy-and-versioned-import-quarantine", func(t *testing.T) {
		d := phase29Text("Legacy report")
		w := d.Widgets[0]
		w.Grid = reporting.GridCell{}
		d.SchemaVersion, d.Widgets = 1, nil
		d.Sections = []reporting.LegacySection{{ID: "overview", Title: "Overview", Widgets: []reporting.Widget{w}}}
		raw, _ := json.Marshal(d)
		external := reporting.ExternalReference{System: "synthetic", ID: "legacy-report", Version: "v1"}
		result, err := documents.Import(ctx, author, "report", "doc-legacy", raw, external)
		if err != nil || result.State == nil || result.Quarantine != "" {
			t.Fatal("supported legacy import", result, err)
		}
		view, err := documents.Read(ctx, author, "report", "doc-legacy", reporting.DocumentReference{Stage: "draft"})
		if err != nil || view.Definition.SchemaVersion != 2 || len(view.Definition.Widgets) != 2 || view.Digest != reporting.DocumentDigest(raw) {
			t.Fatal("legacy projection changed retained meaning", view, err)
		}
		if again, err := documents.Import(ctx, author, "report", "doc-legacy", raw, external); err != nil || again.State == nil || again.State.Version != result.State.Version {
			t.Fatal("exact external version replay changed the draft", again, err)
		}
		d.Metadata[0].Title = "Changed same external version"
		changed, _ := json.Marshal(d)
		if _, err := documents.Import(ctx, author, "report", "doc-legacy", changed, external); !errors.Is(err, store.ErrConflict) {
			t.Fatal("external version silently replaced content", err)
		}
		quarantine, err := documents.Import(ctx, author, "report", "unsupported-document", json.RawMessage(`{"schema_version":99,"future_widget":{"kind":"script"}}`), reporting.ExternalReference{System: "synthetic", ID: "future", Version: "v99"})
		if err != nil || quarantine.State != nil || quarantine.Quarantine == "" {
			t.Fatal("unsupported record was not quarantined", quarantine, err)
		}
		connection, err := pgx.Connect(ctx, os.Getenv("CHARTWORKS_TEST_STORE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = connection.Close(ctx) }()
		var version int
		if err := connection.QueryRow(ctx, `SELECT (definition->>'schema_version')::integer FROM chartworks.document_revisions WHERE tenant_id=$1 AND kind='report' AND document_id='doc-legacy' AND revision=1`, author.Tenant()).Scan(&version); err != nil || version != 1 {
			t.Fatal("read projection mutated history", version, err)
		}
		if _, err := connection.Exec(ctx, `UPDATE chartworks.document_revisions SET digest=repeat('a',64) WHERE tenant_id=$1 AND kind='report' AND document_id='doc-legacy'`, author.Tenant()); err == nil {
			t.Fatal("immutable revision trigger missing")
		}
		var count int
		if err := connection.QueryRow(ctx, `SELECT count(*) FROM chartworks.document_quarantine WHERE tenant_id=$1 AND actor_id=$2 AND quarantine_id=$3`, author.Tenant(), author.User(), quarantine.Quarantine).Scan(&count); err != nil || count != 1 {
			t.Fatal("private quarantine not persisted", count, err)
		}
	})
}
