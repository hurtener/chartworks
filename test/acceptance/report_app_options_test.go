package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func optionKey(number int) string {
	return "option:" + strconv.FormatInt(time.Now().Unix(), 10) + ":" + fmt.Sprintf("%032x", number)
}

func TestReportAppAuthoringOptions(t *testing.T) {
	f, s, author, prepare, publication, scopes := filteredDatasetFixture(t)
	ctx := t.Context()
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	in := reporting.AuthoringOptionRequest{Target: reporting.AuthoringOptionTarget{Dataset: &reporting.AuthoringDatasetOptionTarget{NewBlock: prepare.NewBlock, Topic: prepare.Intent.Topic, Dataset: prepare.Intent.Dataset, Dimension: "region"}}, Operation: optionKey(1), Limit: 2, Locale: "en-US"}
	firstRequest := phase27Copy(t, in)
	first, err := s.DatasetOptions(ctx, author, in)
	if err != nil || first.Status != "completed" || !first.ValuesAvailable || first.Complete || len(first.Options) != 2 || first.Next == "" || first.Options[0].Label != "" || first.Options[1].Label != "East" {
		t.Fatal("first governed options", first, err)
	}
	if f.attemptCount(t) != before+1 {
		t.Fatal("one explicit lookup did not use one read")
	}
	replay, err := s.DatasetOptions(ctx, author, in)
	if err != nil || replay.ValuesAvailable || replay.Complete || !replay.NewOperationAllowed || replay.Code != "result_not_retained" || len(replay.Options) != 0 || f.attemptCount(t) != before+1 {
		t.Fatal("option replay reran or fabricated empty values", replay, err)
	}
	status, err := s.OptionStatus(ctx, author, reporting.AuthoringOptionReference{Target: in.Target, Operation: in.Operation})
	if err != nil || status.InputDigest != first.InputDigest || status.ValuesAvailable || f.attemptCount(t) != before+1 {
		t.Fatal("status source work", status, err)
	}
	otherSession, err := f.f.f.token.verifier.Verify(ctx, phase27Token(t, f.f, author.User(), "second-options-session", scopes), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := s.DatasetOptions(ctx, otherSession, in); err == nil || response.ValuesAvailable || f.attemptCount(t) != before+1 {
		t.Fatal("completed operation repeated across sessions", response, err)
	}
	changed := phase27Copy(t, in)
	changed.Search = "North"
	if _, err := s.DatasetOptions(ctx, author, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal("changed search reused operation", err)
	}
	changed.Operation = optionKey(2)
	changed.Cursor = first.Next
	if _, err := s.DatasetOptions(ctx, author, changed); err == nil {
		t.Fatal("cursor rebound to search")
	}
	in.Operation = optionKey(3)
	in.Cursor = first.Next
	second, err := s.DatasetOptions(ctx, author, in)
	if err != nil || !second.ValuesAvailable || len(second.Options) != 2 || second.Options[0].Label != "North" || second.Options[1].Label != "South" {
		t.Fatal("keyset second page", second, err)
	}
	in.Operation = optionKey(4)
	in.Cursor = second.Next
	last, err := s.DatasetOptions(ctx, author, in)
	if err != nil || !last.Complete || len(last.Options) != 1 || last.Options[0].Label != "x' OR true --" {
		t.Fatal("NULL leaked or keyset skipped values", last, err)
	}
	in.Operation = optionKey(5)
	in.Cursor = ""
	in.Search = "x' OR true --"
	literal, err := s.DatasetOptions(ctx, author, in)
	if err != nil || len(literal.Options) != 1 || literal.Options[0].Label != in.Search {
		t.Fatal("search was not literal", literal, err)
	}
	in.Operation = optionKey(6)
	in.Search = "%"
	empty, err := s.DatasetOptions(ctx, author, in)
	if err != nil || !empty.ValuesAvailable || !empty.Complete || len(empty.Options) != 0 {
		t.Fatal("escaped wildcard or genuine empty page", empty, err)
	}
	if f.attemptCount(t) != before+5 {
		t.Fatal("unexpected retry source work")
	}
	// Search runs over the complete reviewed relation, not an initial client page.
	if _, err := f.f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount,created_at,name) SELECT 1000+n,1,'2026-01-02','region-'||lpad(n::text,3,'0') FROM generate_series(1,205) n`); err != nil {
		t.Fatal(err)
	}
	in.Operation = optionKey(7)
	in.Search = "region-205"
	in.Limit = 199
	full, err := s.DatasetOptions(ctx, author, in)
	if err != nil || len(full.Options) != 1 || full.Options[0].Label != "region-205" || !full.Complete {
		t.Fatal("full governed population search", full, err)
	}
	in.Operation = optionKey(8)
	in.Search = ""
	page, err := s.DatasetOptions(ctx, author, in)
	if err != nil || len(page.Options) != 199 || page.Complete || page.Next == "" {
		t.Fatal("199+1 options bound", page, err)
	}
	continuation := phase27Copy(t, in)
	continuation.Operation = optionKey(13)
	continuation.Cursor = page.Next
	remaining, err := s.DatasetOptions(ctx, author, continuation)
	if err != nil || !remaining.Complete || len(remaining.Options) != 11 || !remaining.ValuesAvailable {
		t.Fatal("199 page omitted the remaining governed population", remaining, err)
	}
	for _, limit := range []int{200, 201} {
		invalid := phase27Copy(t, in)
		invalid.Operation = optionKey(100 + limit)
		invalid.Limit = limit
		if _, err := s.DatasetOptions(ctx, author, invalid); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("oversized page admitted", limit, err)
		}
	}
	nextSession := phase27Copy(t, in)
	nextSession.Operation = optionKey(12)
	nextSession.Search = "East"
	beforeSession := f.attemptCount(t)
	if response, err := s.DatasetOptions(ctx, otherSession, nextSession); err != nil || !response.ValuesAvailable || f.attemptCount(t) != beforeSession+1 {
		t.Fatal("explicit new session operation failed", response, err)
	}
	// Lost authority is checked before retained lookup payload projection.
	limited := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(x string) bool { return x == "cw.source.query:"+publication.Definition.Datasets[0].Source.Source }))
	denied, err := s.OptionStatus(ctx, limited, reporting.AuthoringOptionReference{Target: in.Target, Operation: in.Operation})
	if err == nil || !reflect.DeepEqual(denied, reporting.AuthoringOptionView{}) {
		t.Fatal("status leaked after source reach loss", denied, err)
	}
	// Expired operation IDs cannot dispatch after the metadata tombstone is gone.
	expired := phase27Copy(t, in)
	expired.Operation = "option:" + strconv.FormatInt(time.Now().Add(-25*time.Hour).Unix(), 10) + ":" + fmt.Sprintf("%032x", 9)
	if _, err := s.DatasetOptions(ctx, author, expired); !errors.Is(err, store.ErrExpired) {
		t.Fatal("expired operation became fresh", err)
	}

	if _, err := f.f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount,created_at,name) VALUES(2000,1,'2026-01-02',$1)`, "oversize-"+strings.Repeat("q", 600<<10)); err != nil {
		t.Fatal(err)
	}
	oversized := phase27Copy(t, in)
	oversized.Operation = optionKey(23)
	oversized.Search = "oversize-"
	oversized.Cursor = ""
	if response, err := s.DatasetOptions(ctx, author, oversized); err != nil || response.ValuesAvailable || len(response.Options) != 0 || response.Complete || response.Status != "failed" {
		t.Fatal("oversized option values released", response, err)
	}

	if _, err := f.f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount,created_at,name) SELECT 3000+n,1,'2026-01-02','serialization-'||lpad(n::text,3,'0')||repeat(chr(10),950) FROM generate_series(1,150) n`); err != nil {
		t.Fatal(err)
	}
	escaped := phase27Copy(t, in)
	escaped.Operation = optionKey(25)
	escaped.Search = "serialization-"
	escaped.Cursor = ""
	if response, err := s.DatasetOptions(ctx, author, escaped); err != nil || response.ValuesAvailable || len(response.Options) != 0 || response.Code != "option_result_budget" || response.Status != "failed" {
		t.Fatal("duplicated escaped options exceeded transport budget", response, err)
	}
	// The UI uses an explicit full bounded page, distinct from the small native
	// pagination cases above. Capture this exact request/response for conformance.
	wireRequest := phase27Copy(t, firstRequest)
	wireRequest.Operation = optionKey(26)
	wireRequest.Limit = 199
	wireRequest.Search = "East"
	wireResponse, err := s.DatasetOptions(ctx, author, wireRequest)
	if err != nil || !wireResponse.ValuesAvailable || len(wireResponse.Options) != 1 || wireResponse.Options[0].Label != "East" {
		t.Fatal("UI bounded lookup wire", wireResponse, err)
	}
	// The same source/options become report filters by exact native bindings.
	prepared, err := s.PrepareDatasetChart(ctx, author, prepare)
	if err != nil || prepared.Status != "prepared" {
		t.Fatal(prepared, err)
	}
	created, err := s.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: prepare.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil {
		t.Fatal(err)
	}
	widget := phase29BlockWidget("options-widget", prepare.NewBlock, 0, "chart")
	// This native capture also drives the browser journey: leave practical room
	// for the retained chart, exact amount and disclosure within the authored card.
	widget.Grid.Height = 4
	widget.Block.Policy = "private_preview"
	widget.Block.Revision = created.Block.Revision
	widget.Block.Digest = created.Block.Digest
	pageDefinition := reporting.ReportPage{ID: "analysis", Title: "Analysis", Widgets: []reporting.Widget{widget}}
	for _, p := range created.Block.Parameters {
		filter := phase27Copy(t, p)
		filter.Name = p.Dimension.Dimension
		pageDefinition.Filters = append(pageDefinition.Filters, reporting.ReportFilter{Parameter: filter, Label: map[string]string{"day": "Day", "region": "Region"}[filter.Name]})
		pageDefinition.Widgets[0].Bindings = append(pageDefinition.Widgets[0].Bindings, reporting.FilterBinding{Filter: filter.Name, Parameter: p.Name})
	}
	if len(pageDefinition.Filters) != 2 || pageDefinition.Filters[0].Parameter.Type != "date_range" || pageDefinition.Filters[1].Parameter.Type != "dimension_set" {
		t.Fatal("both native typed parameters must reach report filters", pageDefinition.Filters)
	}
	document := phase29Text("Option report")
	document.SchemaVersion = reporting.PagedDocumentVersion
	document.Widgets = nil
	document.ReportPages = []reporting.ReportPage{pageDefinition, {ID: "notes", Title: "Notes", Widgets: []reporting.Widget{{ID: "filter-notes", Kind: "text", Grid: reporting.GridCell{Width: 12, Height: 2}, Text: &reporting.TextWidget{Format: "plain", Text: "Synthetic notes stay unchanged while Analysis filters are edited."}}}}}
	metadataReads := f.attemptCount(t)
	state, err := s.Create(ctx, author, reporting.AuthoringCreateRequest{ID: "filtered-report", Definition: document})
	if err != nil {
		t.Fatal(err)
	}
	initialReport, err := s.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil {
		t.Fatal(err)
	}
	initialDrafts, err := s.Drafts(ctx, author, reporting.DraftListRequest{Limit: 40})
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := s.Capabilities(ctx, author, reporting.AuthoringCapabilitiesRequest{Report: state.ID})
	if err != nil || !capabilities.CanSave || !capabilities.CanPreview || f.attemptCount(t) != metadataReads {
		t.Fatal("authoring metadata executed source work", capabilities, err)
	}
	initialOptionRequest := reporting.AuthoringOptionRequest{Target: reporting.AuthoringOptionTarget{Report: &reporting.AuthoringReportOptionTarget{Report: state.ID, Revision: initialReport.Revision, Digest: initialReport.Digest, Page: "analysis", Filter: "region", Policy: "private_preview"}}, Operation: optionKey(30), Search: "East", Limit: 199, Locale: "en-US"}
	initialOptions, err := s.ReportOptions(ctx, author, initialOptionRequest)
	if err != nil || !initialOptions.ValuesAvailable || len(initialOptions.Options) != 1 || initialOptions.Options[0].Label != "East" || f.attemptCount(t) != metadataReads+1 {
		t.Fatal("initial explicit Search requires exactly one governed read", initialOptions, err)
	}
	// Persist different page defaults through the real authoring CAS seam. Local
	// editor changes and Save are metadata-only; block defaults stay independent.
	saveRequest := reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: phase27Copy(t, initialReport.Definition)}
	for i := range saveRequest.Definition.ReportPages[0].Filters {
		filter := &saveRequest.Definition.ReportPages[0].Filters[i].Parameter
		switch filter.Name {
		case "day":
			filter.Default = &reporting.Value{DateRange: &reporting.DateRange{Start: "2026-01-02", EndExclusive: "2026-02-01"}}
		case "region":
			filter.Default = &reporting.Value{Items: []string{"North", "East"}}
		}
	}
	state, err = s.Save(ctx, author, saveRequest)
	if err != nil {
		t.Fatal(err)
	}
	savedState := phase27Copy(t, state)
	saved, err := s.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil || saved.Revision != initialReport.Revision+1 || !reflect.DeepEqual(saved.Definition, saveRequest.Definition) {
		t.Fatal("saved typed defaults", saved, err)
	}
	savedDrafts, err := s.Drafts(ctx, author, reporting.DraftListRequest{Limit: 40})
	if err != nil || f.attemptCount(t) != metadataReads+1 {
		t.Fatal("Save or reopen ran a source query", err)
	}
	reportRequest := reporting.AuthoringOptionRequest{Target: reporting.AuthoringOptionTarget{Report: &reporting.AuthoringReportOptionTarget{Report: state.ID, Revision: state.DraftRevision, Digest: saved.Digest, Page: "analysis", Filter: "region", Policy: "private_preview"}}, Operation: optionKey(10), Search: "East", Limit: 199, Locale: "en-US"}
	privateRequest := phase27Copy(t, reportRequest)
	private, err := s.ReportOptions(ctx, author, reportRequest)
	if err != nil || !private.ValuesAvailable || len(private.Options) != 1 || private.Options[0].Label != "East" || f.attemptCount(t) != metadataReads+2 {
		t.Fatal("private exact options", private, err)
	}
	privateReads := f.attemptCount(t)
	for _, deniedActor := range []identity.Envelope{phase27Actor(t, f.f, "other-private-actor", scopes), phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(x string) bool { return x == "reporting.preview" || strings.Contains(x, ".preview:") }))} {
		blocked := phase27Copy(t, reportRequest)
		blocked.Operation = optionKey(14)
		if response, err := s.ReportOptions(ctx, deniedActor, blocked); err == nil || !reflect.DeepEqual(response, reporting.AuthoringOptionView{}) || f.attemptCount(t) != privateReads {
			t.Fatal("private actor/preview authority bypass", response, err)
		}
	}
	current, err := s.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil || !reflect.DeepEqual(current.Definition, saved.Definition) {
		t.Fatal("options changed defaults", err)
	}
	if f.f.model.requests.Load() != models {
		t.Fatal("option lookup invoked model")
	}
	// Execute the browser's default/temporary/clear inputs against the actual
	// bound source. Retained DTOs are captured directly, without relabeling pins.
	validationRequest := reporting.AuthoringBlockValidateRequest{Block: prepare.NewBlock, ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision, Digest: created.Block.Digest, Arguments: []reporting.Argument{}, Resolution: reporting.Resolution{At: time.Now().UTC().Truncate(time.Second).Add(123 * time.Millisecond), Timezone: "UTC"}}
	validation, err := s.ValidateBlock(ctx, author, validationRequest)
	if err != nil || f.attemptCount(t) != privateReads+1 {
		t.Fatal("explicit validation read", validation, err)
	}
	validatedBlock, err := s.ReadBlock(ctx, author, reporting.AuthoringBlockReadRequest{Block: prepare.NewBlock, Revision: created.Block.Revision})
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := reporting.NewDelivery(f.blocks, f.runs, f.documents, f.compositions, f.f.f.db, f.limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	temporaryPages := []reporting.PageInput{{Page: "analysis", Filters: []reporting.Argument{{Name: "day", Value: reporting.Value{DateRange: &reporting.DateRange{Start: "2026-01-01", EndExclusive: "2026-01-02"}}}, {Name: "region", Value: reporting.Value{Items: []string{"North"}}}}, Overrides: []reporting.WidgetOverride{}}}
	privateRuns := map[string]any{}
	for _, selection := range []struct {
		name  string
		pages []reporting.PageInput
		exact string
	}{{"defaults", []reporting.PageInput{}, "3.250"}, {"temporary", temporaryPages, "1.250"}, {"cleared", []reporting.PageInput{}, "3.250"}} {
		reads := f.attemptCount(t)
		previewRequest := reporting.AuthoringPreviewRequest{Report: saved.State.ID, Revision: saved.Revision, Key: "filter-browser-private-" + selection.name, Resolution: reporting.Resolution{At: time.Now().UTC().Truncate(time.Second).Add(123 * time.Millisecond), Timezone: "UTC"}, Pages: selection.pages}
		preview, err := s.Preview(ctx, author, previewRequest)
		if err != nil || !preview.Private || f.attemptCount(t) != reads {
			t.Fatal("private admission performed hidden work", preview, err)
		}
		executeRequest := reporting.AuthoringExecuteRequest{Run: preview.ID}
		complete, err := s.Execute(ctx, author, executeRequest)
		if err != nil || !complete.Complete || !complete.Private || f.attemptCount(t) != reads+1 {
			t.Fatal("one private explicit execution", complete, err)
		}
		reader := phase27Actor(t, f.f, author.User(), []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + saved.State.ID, "cw.execution_context.use:" + created.Block.Context})
		rootRequest := reporting.DeliveryViewRequest{Kind: "report", Run: preview.ID, Offset: 0, Limit: 100}
		root, err := delivery.View(ctx, reader, rootRequest)
		if err != nil || !root.Summary.Private {
			t.Fatal("private retained root", root, err)
		}
		outputRequest := reporting.DeliveryViewRequest{Kind: "report", Run: preview.ID, Page: "analysis", Widget: "options-widget", Output: "chart", Offset: 0, Limit: 100}
		output, err := delivery.View(ctx, reader, outputRequest)
		if err != nil || output.Output == nil || output.Output.Chart == nil || len(output.Output.Chart.Points) != 1 || output.Output.Chart.Points[0].Value.Exact != selection.exact || f.attemptCount(t) != reads+1 {
			t.Fatal("private retained exact filtered amount", selection.name, output, err)
		}
		notesRequest := reporting.DeliveryViewRequest{Kind: "report", Run: preview.ID, Page: "notes", Widget: "filter-notes", Offset: 0, Limit: 100}
		notes, err := delivery.View(ctx, reader, notesRequest)
		if err != nil || notes.Text == nil || notes.Text.Text != document.ReportPages[1].Widgets[0].Text.Text || f.attemptCount(t) != reads+1 {
			t.Fatal("native sibling notes retained view", notes, err)
		}
		privateRuns[selection.name] = map[string]any{"notes_request": notesRequest, "view_notes": notes, "preview_request": previewRequest, "preview_response": preview, "execute_request": executeRequest, "execute_response": complete, "root_request": rootRequest, "view_root": root, "output_request": outputRequest, "view_output": output, "source_reads": f.attemptCount(t) - reads}
	}
	privateAfterRuns, err := s.Read(ctx, author, reporting.AuthoringReadRequest{Report: saved.State.ID})
	blockAfterRuns, blockErr := s.ReadBlock(ctx, author, reporting.AuthoringBlockReadRequest{Block: prepare.NewBlock, Revision: created.Block.Revision})
	if err != nil || blockErr != nil || !reflect.DeepEqual(privateAfterRuns.Definition, saved.Definition) || privateAfterRuns.Digest != saved.Digest || !reflect.DeepEqual(blockAfterRuns.Block.Parameters, created.Block.Parameters) || f.f.model.requests.Load() != models {
		t.Fatal("temporary/clear changed persisted defaults or used a model", err, blockErr)
	}
	// Publication is explicit; saved defaults remain byte-identical on rebind.
	publisher := phase27Actor(t, f.f, author.User(), append(slices.Clone(scopes), "reporting.publish", "cw.block.publish:"+prepare.NewBlock, "cw.report.publish:"+state.ID))
	native, err := f.blocks.Read(ctx, publisher, prepare.NewBlock, reporting.Reference{Revision: created.Block.Revision})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, f.blocks, publisher, native)
	rebound := phase27Copy(t, saved.Definition)
	rebound.ReportPages[0].Widgets[0].Block.Policy = "published"
	rebound.ReportPages[0].Widgets[0].Block.Digest = ""
	state, err = s.Save(ctx, publisher, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: rebound})
	if err != nil {
		t.Fatal(err)
	}

	privatePublished, err := s.Read(ctx, publisher, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil {
		t.Fatal(err)
	}
	draftPublished := phase27Copy(t, reportRequest)
	draftPublished.Operation = optionKey(18)
	draftPublished.Target.Report.Revision = state.DraftRevision
	draftPublished.Target.Report.Digest = privatePublished.Digest
	withoutBlockPreview := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(x string) bool { return x == "cw.block.preview:"+prepare.NewBlock }))
	if choices, err := s.ReportOptions(ctx, withoutBlockPreview, draftPublished); err != nil || !choices.ValuesAvailable {
		t.Fatal("private parent imposed preview on published dependency", choices, err)
	}
	mixedPolicies := phase27Copy(t, saved.Definition)
	publishedWidget := phase27Copy(t, rebound.ReportPages[0].Widgets[0])
	publishedWidget.ID = "published-same-revision"
	publishedWidget.Grid.Row = widget.Grid.Height
	mixedPolicies.ReportPages[0].Widgets = append(mixedPolicies.ReportPages[0].Widgets, publishedWidget)
	state, err = s.Save(ctx, publisher, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: mixedPolicies})
	if err != nil {
		t.Fatal(err)
	}
	mixedPrivate, err := s.Read(ctx, publisher, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil {
		t.Fatal(err)
	}
	mixedRequest := phase27Copy(t, reportRequest)
	mixedRequest.Operation = optionKey(19)
	mixedRequest.Target.Report.Revision = state.DraftRevision
	mixedRequest.Target.Report.Digest = mixedPrivate.Digest
	if choices, err := s.ReportOptions(ctx, publisher, mixedRequest); err != nil || !choices.ValuesAvailable {
		t.Fatal("mixed private/public same revision", choices, err)
	}
	policies, err := f.f.f.db.ReadAuthoringOption(ctx, publisher, reporting.AuthoringOptionReference{Target: mixedRequest.Target, Operation: mixedRequest.Operation})
	if err != nil || len(policies.Blocks) != 2 || policies.Blocks[0].Policy == policies.Blocks[1].Policy {
		t.Fatal("policy custody collapsed", policies, err)
	}
	mixedRequest.Operation = optionKey(20)
	if choices, err := s.ReportOptions(ctx, withoutBlockPreview, mixedRequest); err == nil || choices.ValuesAvailable {
		t.Fatal("published reference laundered private preview requirement", choices, err)
	}
	// Explicitly rebind the remaining private reference before report publication.
	mixedPolicies.ReportPages[0].Widgets[0].Block.Policy = "published"
	mixedPolicies.ReportPages[0].Widgets[0].Block.Digest = ""
	state, err = s.Save(ctx, publisher, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: mixedPolicies})
	if err != nil {
		t.Fatal(err)
	}
	state = phase29Publish(t, f.documents, publisher, state)
	published, err := f.documents.Read(ctx, publisher, "report", state.ID, reporting.DocumentReference{Revision: state.PublishedRevision})
	if err != nil {
		t.Fatal(err)
	}
	consumerScopes := slices.DeleteFunc(slices.Clone(scopes), func(x string) bool {
		return x == "reporting.preview" || x == "reporting.write" || x == "reporting.validate" || x == "charts.bind" || strings.Contains(x, ".preview:")
	})
	consumer := phase27Actor(t, f.f, "option-consumer", consumerScopes)
	description, err := delivery.Describe(ctx, consumer, reporting.DeliveryDescribeRequest{Target: reporting.DeliveryTarget{Kind: "report", ID: state.ID, Revision: state.PublishedRevision}, Locale: "en-US"})
	if err != nil || description.DefinitionDigest != published.Digest {
		t.Fatal("published description omitted native digest", description, err)
	}
	reportRequest.Operation = optionKey(11)
	reportRequest.Target.Report.Policy = "published"
	reportRequest.Target.Report.Revision = state.PublishedRevision
	reportRequest.Target.Report.Digest = description.DefinitionDigest
	publicRequest := phase27Copy(t, reportRequest)
	public, err := s.ReportOptions(ctx, consumer, reportRequest)
	if err != nil || !public.ValuesAvailable || len(public.Options) != 1 || public.Options[0].Label != "East" {
		t.Fatal("Consumer options continuity", public, err)
	}
	if !reflect.DeepEqual(saved.Definition.ReportPages[0].Filters, published.Definition.ReportPages[0].Filters) {
		t.Fatal("publication changed canonical filters/defaults")
	}
	publishedReport := phase27Copy(t, published)
	publicCatalog, err := delivery.Search(ctx, consumer, reporting.DeliverySearchRequest{Kind: "report", Locale: "en-US", Limit: 40})
	if err != nil {
		t.Fatal(err)
	}
	consumerCapabilities, err := s.Capabilities(ctx, consumer, reporting.AuthoringCapabilitiesRequest{Report: state.ID})
	if err != nil || consumerCapabilities.Builder || !consumerCapabilities.CanExecute {
		t.Fatal("consumer capability projection", consumerCapabilities, err)
	}
	publicRuns := map[string]any{}
	for _, selection := range []struct {
		name  string
		pages []reporting.PageInput
		exact string
	}{{"defaults", []reporting.PageInput{}, "3.250"}, {"temporary", temporaryPages, "1.250"}, {"cleared", []reporting.PageInput{}, "3.250"}} {
		reads := f.attemptCount(t)
		runRequest := reporting.DeliveryRunRequest{Target: description.Resource.Target, Key: "filter-browser-public-" + selection.name, Arguments: []reporting.Argument{}, Pages: selection.pages, Outputs: []string{}, Locale: "en-US", Timezone: "UTC"}
		run, err := delivery.Run(ctx, consumer, runRequest)
		if err != nil || run.State != "completed" || f.attemptCount(t) != reads+1 {
			t.Fatal("one public explicit execution, deduplicating identical widgets", run, err)
		}
		reader := phase27Actor(t, f.f, consumer.User(), []string{"reporting.read", "cw.run.read:" + run.Run, "cw.report.read:" + state.ID, "cw.execution_context.use:" + created.Block.Context})
		rootRequest := reporting.DeliveryViewRequest{Kind: "report", Run: run.Run, Offset: 0, Limit: 100}
		root, err := delivery.View(ctx, reader, rootRequest)
		if err != nil || root.Summary.Private {
			t.Fatal("public retained root", root, err)
		}
		outputs := map[string]any{}
		for _, widget := range publishedReport.Definition.ReportPages[0].Widgets {
			outputRequest := reporting.DeliveryViewRequest{Kind: "report", Run: run.Run, Page: "analysis", Widget: widget.ID, Output: "chart", Offset: 0, Limit: 100}
			output, err := delivery.View(ctx, reader, outputRequest)
			if err != nil || output.Output == nil || output.Output.Chart == nil || len(output.Output.Chart.Points) != 1 || output.Output.Chart.Points[0].Value.Exact != selection.exact {
				t.Fatal("public retained exact filtered amount", selection.name, output, err)
			}
			outputs[widget.ID] = map[string]any{"request": outputRequest, "response": output}
		}
		if f.attemptCount(t) != reads+1 {
			t.Fatal("public retained views performed source work")
		}
		notesRequest := reporting.DeliveryViewRequest{Kind: "report", Run: run.Run, Page: "notes", Widget: "filter-notes", Offset: 0, Limit: 100}
		notes, err := delivery.View(ctx, reader, notesRequest)
		if err != nil || notes.Text == nil || notes.Text.Text != document.ReportPages[1].Widgets[0].Text.Text || f.attemptCount(t) != reads+1 {
			t.Fatal("public sibling notes retained view", notes, err)
		}
		publicRuns[selection.name] = map[string]any{"notes_request": notesRequest, "view_notes": notes, "run_request": runRequest, "run_response": run, "root_request": rootRequest, "view_root": root, "outputs": outputs, "source_reads": f.attemptCount(t) - reads}
	}
	publicAfterRuns, err := f.documents.Read(ctx, consumer, "report", publishedReport.State.ID, reporting.DocumentReference{Revision: publishedReport.Revision})
	if err != nil || !reflect.DeepEqual(publicAfterRuns.Definition, publishedReport.Definition) || publicAfterRuns.Digest != publishedReport.Digest || f.f.model.requests.Load() != models {
		t.Fatal("public temporary/clear changed immutable defaults or used a model", err)
	}
	// A newer publication does not substitute an old exact eligible block pin.
	rawBlock, err := f.f.f.db.ReadBlock(ctx, publisher, prepare.NewBlock, reporting.Reference{Revision: created.Block.Revision}, reporting.Write)
	if err != nil {
		t.Fatal(err)
	}
	amendment := phase27Copy(t, rawBlock.Revision.Definition)
	amendment.Metadata[0].Description = "Second synthetic reviewed display revision"
	updated, err := f.blocks.Edit(ctx, publisher, prepare.NewBlock, reporting.EditRequest{ExpectedVersion: rawBlock.State.Version, Definition: amendment})
	if err != nil {
		t.Fatal(err)
	}
	secondBlock, _ := phase27ValidatePublish(t, f.blocks, publisher, updated)
	reportRequest.Operation = optionKey(16)
	if old, err := s.ReportOptions(ctx, consumer, reportRequest); err != nil || !old.ValuesAvailable {
		t.Fatal("new block publication invalidated exact old pin", old, err)
	}
	mixed := phase27Copy(t, published.Definition)
	a := mixed.ReportPages[0].Widgets[0]
	b := phase27Copy(t, a)
	b.ID = "options-widget-two"
	b.Grid.Row = a.Grid.Height
	b.Block.Revision = secondBlock.PublishedRevision
	c := phase27Copy(t, a)
	c.ID = "options-widget-three"
	c.Grid.Row = 2 * a.Grid.Height
	mixed.ReportPages[0].Widgets = []reporting.Widget{a, b, c}
	state, err = s.Save(ctx, publisher, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.PublishedRevision, Definition: mixed})
	if err != nil {
		t.Fatal(err)
	}
	state = phase29Publish(t, f.documents, publisher, state)
	published, err = f.documents.Read(ctx, publisher, "report", state.ID, reporting.DocumentReference{Revision: state.PublishedRevision})
	if err != nil {
		t.Fatal(err)
	}
	reportRequest.Operation = optionKey(17)
	reportRequest.Target.Report.Revision = state.PublishedRevision
	reportRequest.Target.Report.Digest = published.Digest
	if mixedPage, err := s.ReportOptions(ctx, consumer, reportRequest); err != nil || !mixedPage.ValuesAvailable || len(mixedPage.Options) != 1 {
		t.Fatal("repeated mixed published revision options", mixedPage, err)
	}
	record, err := f.f.f.db.ReadAuthoringOption(ctx, consumer, reporting.AuthoringOptionReference{Target: reportRequest.Target, Operation: reportRequest.Operation})
	if err != nil || len(record.Blocks) != 2 {
		t.Fatal("exact block refs not deduplicated", record, err)
	}
	floating := phase27Copy(t, published.Definition)
	floating.ReportPages[0].Widgets[0].Block.Revision = 0
	state, err = s.Save(ctx, publisher, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.PublishedRevision, Definition: floating})
	if err != nil {
		t.Fatal(err)
	}
	floatingView, err := s.Read(ctx, publisher, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil {
		t.Fatal(err)
	}
	floatingRequest := phase27Copy(t, reportRequest)
	floatingRequest.Operation = optionKey(24)
	floatingRequest.Target.Report.Policy = "private_preview"
	floatingRequest.Target.Report.Revision = state.DraftRevision
	floatingRequest.Target.Report.Digest = floatingView.Digest
	beforeFloating := f.attemptCount(t)
	if response, err := s.ReportOptions(ctx, publisher, floatingRequest); err != nil || response.Status != "unsupported" || response.Code != "option_exact_block_revision_required" || response.ValuesAvailable || f.attemptCount(t) != beforeFloating {
		t.Fatal("floating option pin not rejected before source work", response, err)
	}

	// Local-only synthetic wire capture for UI/native DTO conformance. The exporter
	// deliberately excludes envelopes, SQL, source controls and operational records.
	if path := os.Getenv("CHARTWORKS_FILTER_DTO_PATH"); path != "" {
		metadata, err := s.Dataset(ctx, author, reporting.AuthoringDatasetRequest{Topic: prepare.Intent.Topic, Dataset: prepare.Intent.Dataset})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.MarshalIndent(map[string]any{
			"dataset": metadata, "prepare_request": prepare, "preparation": prepared, "option_request": wireRequest, "option_response": wireResponse, "created": created,
			"initial_report": initialReport, "initial_drafts": initialDrafts, "capabilities": capabilities, "initial_option_request": initialOptionRequest, "initial_option_response": initialOptions,
			"save_request": saveRequest, "saved_state": savedState, "private_report": saved, "saved_drafts": savedDrafts, "private_option_request": privateRequest, "private_option_response": private,
			"validation_request": validationRequest, "validation": validation, "validated_block": validatedBlock, "private_runs": privateRuns, "private_after_runs": privateAfterRuns,
			"published_report": publishedReport, "published_catalog": publicCatalog, "consumer_capabilities": consumerCapabilities, "published_description": description, "published_option_request": publicRequest, "published_option_response": public, "published_runs": publicRuns, "published_after_runs": publicAfterRuns,
			"read_counts": map[string]int{"create_open_metadata": 0, "save_reopen_metadata": 0, "explicit_option_search": 1, "preview_admission": 0, "explicit_validation": 1, "retained_view": 0, "model_calls": 0},
		}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		var inspect func(any)
		inspect = func(value any) {
			switch v := value.(type) {
			case map[string]any:
				for key, child := range v {
					switch key {
					case "sql", "statement", "attempt", "remote", "manifest", "session", "token", "credential", "authorization", "password", "dsn":
						t.Fatal("private custody in DTO capture", key)
					}
					inspect(child)
				}
			case []any:
				for _, child := range v {
					inspect(child)
				}
			}
		}
		inspect(decoded)
		if len(wire) > 512<<10 {
			t.Fatal("DTO capture bound")
		}
		if err := os.WriteFile(path, wire, 0600); err != nil {
			t.Fatal(err)
		}
	}

}
