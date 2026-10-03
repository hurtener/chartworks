package acceptance

import (
	"errors"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"reflect"
	"slices"
	"testing"
)

func TestReportPagesLifecycleAndSnapshots(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	d := phase29Text("Two private pages")
	first := d.Widgets[0]
	second := phase27Copy(t, first)
	second.ID = "other"
	second.Text.Text = "Independent"
	d.SchemaVersion = reporting.PagedDocumentVersion
	d.Widgets = nil
	d.ReportPages = []reporting.ReportPage{{ID: "overview", Title: "Overview", Widgets: []reporting.Widget{first}}, {ID: "detail", Title: "Detail", Widgets: []reporting.Widget{second}}}
	state := f.report(t, "paged-report", d, false)
	original, err := f.documents.Read(ctx, f.author, "report", state.ID, reporting.DocumentReference{Revision: 1})
	if err != nil || !original.Private || !reflect.DeepEqual(original.Definition, d) {
		t.Fatal(original, err)
	}
	before := f.attemptCount(t)
	models := f.f.model.requests.Load()
	preview, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "pages-preview", Reference: reporting.DocumentReference{Revision: 1}, Preview: true})
	if err != nil || len(preview.Pages) != 2 || preview.Pages[0].ID != "overview" || preview.Pages[1].ID != "detail" {
		t.Fatal(preview, err)
	}
	if _, err = f.compositions.Run(ctx, f.execute, preview.ID, false); err != nil {
		t.Fatal(err)
	}
	changed := phase27Copy(t, d)
	changed.ReportPages[1].Title = "Renamed"
	changed.ReportPages[1].Widgets[0].Text.Text = "Edited second page"
	amended, err := f.documents.Edit(ctx, f.author, "report", state.ID, state.Version, reporting.DocumentReference{Revision: 1}, changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.documents.Edit(ctx, f.author, "report", state.ID, state.Version, reporting.DocumentReference{Revision: 1}, d); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale report CAS accepted", err)
	}
	reopened, err := f.documents.Read(ctx, f.author, "report", state.ID, reporting.DocumentReference{Revision: amended.DraftRevision})
	if err != nil || !reflect.DeepEqual(reopened.Definition.ReportPages[0], d.ReportPages[0]) {
		t.Fatal("other page mutated", reopened, err)
	}
	historical, err := f.documents.Read(ctx, f.author, "report", state.ID, reporting.DocumentReference{Revision: 1})
	if err != nil || historical.Digest != original.Digest || !reflect.DeepEqual(historical.Definition, d) {
		t.Fatal("immutable snapshot changed", historical, err)
	}
	published := phase29Publish(t, f.documents, f.author, amended)
	public, err := f.documents.Read(ctx, f.author, "report", state.ID, reporting.DocumentReference{})
	if err != nil || public.Private || public.Revision != published.PublishedRevision || public.Definition.ReportPages[1].Title != "Renamed" {
		t.Fatal(public, err)
	}
	old, err := f.compositions.Get(ctx, f.execute, preview.ID)
	if err != nil || !old.Private || old.Pages[1].Title != "Detail" {
		t.Fatal("later publication rewrote private preview", old, err)
	}
	payload, err := f.compositions.Widget(ctx, f.execute, preview.ID, "detail", "other")
	if err != nil || payload.Text == nil || payload.Text.Text != "Independent" {
		t.Fatal(payload, err)
	}
	if _, err = f.compositions.Widget(ctx, f.execute, preview.ID, "overview", "other"); err == nil {
		t.Fatal("wrong page resolved a globally known widget")
	}
	dashboard := phase29Text("Nested pages")
	dashboard.Widgets = nil
	dashboard.Pages = []reporting.DocumentPage{{ID: "first-group", Title: "First", Report: state.ID, Revision: public.Revision}, {ID: "second-group", Title: "Second", Report: state.ID, Revision: public.Revision}}
	ds, err := f.documents.Create(ctx, f.author, "dashboard", "paged-dashboard", dashboard)
	if err != nil {
		t.Fatal(err)
	}
	ds = phase29Publish(t, f.documents, f.author, ds)
	nested, err := f.compositions.Admit(ctx, f.execute, "dashboard", ds.ID, reporting.CompositionRequest{Key: "nested-pages"})
	if err != nil || len(nested.Pages) != 4 {
		t.Fatal("dashboard dropped inline canvases", nested, err)
	}
	ids := map[string]bool{}
	for _, page := range nested.Pages {
		if ids[page.ID] {
			t.Fatal("duplicate nested coordinate")
		}
		ids[page.ID] = true
	}
	if _, err = f.compositions.Run(ctx, f.execute, nested.ID, false); err != nil {
		t.Fatal(err)
	}
	removed := phase27Copy(t, changed)
	removed.ReportPages = removed.ReportPages[:1]
	if _, err = f.documents.Edit(ctx, f.author, "report", state.ID, published.Version, reporting.DocumentReference{Revision: public.Revision}, removed); err != nil {
		t.Fatal(err)
	}
	saved, err := f.compositions.Widget(ctx, f.execute, preview.ID, "detail", "other")
	if err != nil || saved.Text.Text != "Independent" {
		t.Fatal("draft page removal erased historical output", saved, err)
	}
	if f.attemptCount(t) != before || f.f.model.requests.Load() != models {
		t.Fatal("metadata/text pages executed sources or models")
	}
}
func TestReportPagesSameDatasetIsolationAndAuthority(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	block := phase27Copy(t, f.base)
	block.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
	block.Parameters = []reporting.Parameter{{Name: "minimum", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}, Min: "1", Max: "2"}}
	f.block(t, "pages-source", block)
	snapshot, err := f.f.f.db.ReadBlock(ctx, f.author, "pages-source", reporting.Reference{}, reporting.Read)
	if err != nil {
		t.Fatal(err)
	}
	d := phase29Text("Same dataset independently")
	d.SchemaVersion = reporting.PagedDocumentVersion
	d.Widgets = nil
	for i, id := range []string{"north", "south"} {
		w := phase29BlockWidget("widget-"+id, "pages-source", 0, "table-main")
		w.Block.Revision = snapshot.Revision.Number
		w.Bindings = []reporting.FilterBinding{{Filter: "minimum", Parameter: "minimum"}}
		literal := []string{"1", "2"}[i]
		d.ReportPages = append(d.ReportPages, reporting.ReportPage{ID: id, Title: id, Widgets: []reporting.Widget{w}, Filters: []reporting.ReportFilter{{Label: "Minimum", Parameter: reporting.Parameter{Name: "minimum", Type: "integer", Default: &reporting.Value{Literal: literal}}}}})
	}
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.execute", "sources.read", "sources.query", "topics.read", "cw.tenant.write:" + f.author.Tenant(), "cw.report.write:isolated-pages", "cw.report.read:isolated-pages", "cw.report.preview:isolated-pages", "cw.report.execute:isolated-pages", "cw.block.read:pages-source", "cw.block.execute:pages-source", "cw.block.preview:pages-source", "cw.source.query:" + f.base.Source}
	for _, ref := range snapshot.References {
		scope := "cw." + ref.Kind + "." + ref.Permission + ":" + ref.ID
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	author := phase27Actor(t, f.f, f.author.User(), scopes)
	service, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	before := f.attemptCount(t)
	models := f.f.model.requests.Load()
	state, err := service.Create(ctx, author, reporting.AuthoringCreateRequest{ID: "isolated-pages", Definition: d})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID}); err != nil {
		t.Fatal(err)
	}
	patch := reporting.AuthoringWidgetRequest{Report: state.ID, Page: "north", Widget: "widget-south", ExpectedVersion: state.Version, Revision: state.DraftRevision, Patch: reporting.WidgetPatch{Presentation: &reporting.Presentation{Title: "Only selected chart"}}}
	if _, err := service.PatchWidget(ctx, author, patch); err == nil {
		t.Fatal("selected page escaped to sibling widget")
	}
	patch.Widget = "widget-north"
	patch.Page = ""
	if _, err := service.PatchWidget(ctx, author, patch); err == nil {
		t.Fatal("v3 widget patch guessed page")
	}
	patch.Page = "north"
	state, err = service.PatchWidget(ctx, author, patch)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil || !reflect.DeepEqual(reloaded.Definition.ReportPages[1], d.ReportPages[1]) {
		t.Fatal("patch changed independent page", reloaded, err)
	}
	for _, missing := range []string{"cw.block.read:pages-source", "cw.execution_context.use:" + f.base.Context, "cw.report.preview:isolated-pages"} {
		denied := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == missing }))
		if _, err := service.Read(ctx, denied, reporting.AuthoringReadRequest{Report: state.ID}); err == nil {
			t.Fatal("missing dependency exposed pages", missing)
		}
	}
	if f.attemptCount(t) != before || f.f.model.requests.Load() != models {
		t.Fatal("page editing executed source/model")
	}
	preview, err := service.Preview(ctx, author, reporting.AuthoringPreviewRequest{Report: state.ID, Revision: state.DraftRevision, Key: "isolated-page-preview"})
	if err != nil || preview.QueryGroups != 2 || len(preview.Pages) != 2 {
		t.Fatal(preview, err)
	}
	if _, err := service.Execute(ctx, author, reporting.AuthoringExecuteRequest{Run: preview.ID}); err != nil {
		t.Fatal(err)
	}
	reader := phase27Actor(t, f.f, author.User(), []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + state.ID, "cw.execution_context.use:" + f.base.Context})
	for _, tc := range []struct {
		page, widget string
		rows         int
	}{{"north", "widget-north", 2}, {"south", "widget-south", 1}} {
		payload, err := f.compositions.Widget(ctx, reader, preview.ID, tc.page, tc.widget)
		if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].Chart == nil || len(payload.Outputs[0].Chart.Rows) != tc.rows {
			t.Fatal("page-local filter leaked", tc, payload, err)
		}
	}
	if _, err := f.compositions.Widget(ctx, reader, preview.ID, "north", "widget-south"); err == nil {
		t.Fatal("wrong page returned same-dataset values")
	}
	other := phase27Actor(t, f.f, "other-actor", []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + state.ID, "cw.execution_context.use:" + f.base.Context})
	if _, err := f.compositions.Widget(ctx, other, preview.ID, "north", "widget-north"); err == nil {
		t.Fatal("another actor read private page result")
	}
	withdrawn := phase27Actor(t, f.f, author.User(), []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + state.ID})
	if _, err := f.compositions.Widget(ctx, withdrawn, preview.ID, "north", "widget-north"); err == nil {
		t.Fatal("revoked context still read page")
	}
	if f.attemptCount(t) != before+2 || f.f.model.requests.Load() != models {
		t.Fatal("independent page execution count", f.attemptCount(t), before)
	}
}
func TestReportPagesScheduledFullManifest(t *testing.T) {
	f := newPhase30Fixture(t, false)
	ctx := t.Context()
	f.domain.block(t, "scheduled-page-source", f.domain.base)
	d := phase29Text("Scheduled pages")
	d.SchemaVersion = reporting.PagedDocumentVersion
	d.Widgets = nil
	for _, id := range []string{"one", "two"} {
		d.ReportPages = append(d.ReportPages, reporting.ReportPage{ID: id, Title: id, Widgets: []reporting.Widget{phase29BlockWidget("chart-"+id, "scheduled-page-source", 0, "table-main")}})
	}
	d.ReportPages[1].Timezone = "America/Argentina/Buenos_Aires"
	f.domain.report(t, "scheduled-pages", d, true)
	target := phase30Target("report", "scheduled-pages")
	j := f.submit(t, "all-page-occurrence", target)
	before := f.domain.attemptCount(t)
	if err := f.queue.RunOnce(ctx); err != nil {
		t.Fatal("page-aware schedule failed", err, f.get(t, j.ID))
	}
	done := f.get(t, j.ID)
	if done.State != "succeeded" || done.Delivery == nil || done.Delivery.Catalog != "available" {
		t.Fatal(done)
	}
	view, err := f.delivery.View(ctx, f.domain.execute, reporting.DeliveryViewRequest{Kind: "report", Run: j.ID, Limit: 1})
	if err != nil || len(view.Pages) != 2 || view.Pages[0].ID != "one" || view.Pages[1].ID != "two" || view.Pages[1].Timezone != d.ReportPages[1].Timezone {
		t.Fatal("scheduled manifest lost page settings", view, err)
	}
	if f.domain.attemptCount(t) != before+2 {
		t.Fatal("page timezone partitions unexpectedly reused", before, f.domain.attemptCount(t))
	}
	target.Arguments = []jobs.ReportingArgument{{Name: "minimum", Value: jobs.ReportingValue{Literal: "1"}}}
	if _, err := f.queue.Submit(ctx, f.actor, "unqualified-page-arguments", jobs.Submission{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &target}); err == nil {
		t.Fatal("page-unqualified schedule arguments accepted")
	}
}
func TestReportPagesFilterOptionsCursor(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	block := phase27Copy(t, f.base)
	block.SQL = "SELECT id, amount FROM analytics.sales WHERE amount >= $1 ORDER BY id"
	block.Parameters = []reporting.Parameter{{Name: "minimum", Type: "number", Required: true, Default: &reporting.Value{Literal: "0"}}}
	f.block(t, "page-options-source", block)
	_, topics := newPhase18Service(t, f.f)
	published, err := topics.Read(ctx, f.blockAuthor, block.Topics[0].Topic, block.Topics[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	dataset, column := "", ""
	for _, d := range published.Definition.Datasets {
		for _, c := range d.Columns {
			if c.SourceName == "amount" {
				dataset, column = d.ID, c.ID
			}
		}
	}
	if column == "" {
		t.Fatal("missing reviewed amount")
	}
	filter := reporting.ReportFilter{Label: "Amount", Parameter: reporting.Parameter{Name: "amount", Type: "number", Default: &reporting.Value{Literal: "0"}}, Options: &reporting.FilterOptionSource{Version: 1, Block: "page-options-source", BlockRevision: 1, Topic: published.Definition.Topic, TopicVersion: published.Definition.Version, Dataset: dataset, Column: column}}
	d := phase29Text("Page choices")
	d.SchemaVersion = reporting.PagedDocumentVersion
	d.Widgets = nil
	for _, id := range []string{"north", "south"} {
		w := phase29BlockWidget("choices-"+id, "page-options-source", 0, "table-main")
		w.Block.Revision = 1
		w.Bindings = []reporting.FilterBinding{{Filter: "amount", Parameter: "minimum"}}
		d.ReportPages = append(d.ReportPages, reporting.ReportPage{ID: id, Title: id, Widgets: []reporting.Widget{w}, Filters: []reporting.ReportFilter{filter}})
	}
	state := f.report(t, "page-options", d, true)
	request := reporting.FilterOptionsRequest{Page: "north", Revision: state.PublishedRevision, Filter: "amount", Limit: 1, Locale: "en-US"}
	page, err := f.documents.FilterOptions(ctx, f.execute, state.ID, request)
	if err != nil || page.Page != "north" || page.Next == "" {
		t.Fatal(page, err)
	}
	before := f.attemptCount(t)
	request.Page = "south"
	request.Cursor = page.Next
	if _, err := f.documents.FilterOptions(ctx, f.execute, state.ID, request); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("cursor crossed inline page boundary", err)
	}
	request.Page = ""
	request.Cursor = ""
	if _, err := f.documents.FilterOptions(ctx, f.execute, state.ID, request); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("v3 options guessed first page", err)
	}
	if f.attemptCount(t) != before {
		t.Fatal("invalid page cursor executed a source")
	}
}
func TestReportPagesConcurrentCAS(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	d := phase29Text("Concurrent page edits")
	first := d.Widgets[0]
	second := phase27Copy(t, first)
	second.ID = "second"
	d.SchemaVersion = reporting.PagedDocumentVersion
	d.Widgets = nil
	d.ReportPages = []reporting.ReportPage{{ID: "first", Title: "First", Widgets: []reporting.Widget{first}}, {ID: "second", Title: "Second", Widgets: []reporting.Widget{second}}}
	state := f.report(t, "concurrent-page-report", d, false)
	results := make(chan error, 2)
	for i := range 2 {
		changed := phase27Copy(t, d)
		changed.ReportPages[i].Title = "Edited"
		go func() {
			_, err := f.documents.Edit(ctx, f.author, "report", state.ID, state.Version, reporting.DocumentReference{Revision: 1}, changed)
			results <- err
		}()
	}
	won, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			won++
		case errors.Is(err, store.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if won != 1 || conflicts != 1 {
		t.Fatal("page edits bypassed shared report CAS", won, conflicts)
	}
	current, err := f.documents.Read(ctx, f.author, "report", state.ID, reporting.DocumentReference{Stage: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	edited := 0
	for _, p := range current.Definition.ReportPages {
		if p.Title == "Edited" {
			edited++
		}
	}
	if edited != 1 || current.Revision != 2 || current.State.Version != 2 {
		t.Fatal("concurrent page edit merged or lost unrelated state", current)
	}
}
func TestReportPagesReadSurvivesLowerAuthoringLimits(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	d := phase29Text("Readable historical pages")
	first := d.Widgets[0]
	second := phase27Copy(t, first)
	second.ID = "second"
	d.SchemaVersion = reporting.PagedDocumentVersion
	d.Widgets = nil
	d.ReportPages = []reporting.ReportPage{{ID: "first", Title: "First", Widgets: []reporting.Widget{first}}, {ID: "second", Title: "Second", Widgets: []reporting.Widget{second}}}
	state := f.report(t, "lowered-page-limits", d, true)
	limits := config.DefaultReporting()
	limits.Composition.MaxPages = 1
	limits.Composition.MaxWidgets = 1
	documents, err := reporting.NewDocuments(f.f.f.db, f.blocks, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := reporting.NewDelivery(f.blocks, f.runs, documents, f.compositions, f.f.f.db, limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	before := f.attemptCount(t)
	description, err := delivery.Describe(ctx, f.execute, reporting.DeliveryDescribeRequest{Target: reporting.DeliveryTarget{Kind: "report", ID: state.ID, Revision: state.PublishedRevision}})
	if err != nil || len(description.Pages) != 2 {
		t.Fatal("lowered authoring ceilings invalidated immutable read", description, err)
	}
	if f.attemptCount(t) != before {
		t.Fatal("historical metadata read executed source")
	}
}
