package acceptance

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

// TestReportAppManualPublicationLifecycle exercises actual PostgreSQL authority,
// immutable publication and CAS. Only explicit validation/preview execute the
// synthetic source; lifecycle operations do not execute queries or models.
func TestReportAppManualPublicationLifecycle(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	const blockID, reportID = "manual-private-chart", "manual-private-report"
	f.base.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
	f.base.Parameters = []reporting.Parameter{{Name: "minimum", Type: "integer", Default: &reporting.Value{Literal: "0"}}}
	chart, err := f.blocks.Create(ctx, f.blockAuthor, reporting.CreateRequest{ID: blockID, Definition: f.base})
	if err != nil {
		t.Fatal(err)
	}
	blockSnapshot, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, blockID, reporting.Reference{Revision: chart.Revision}, reporting.Read)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.execute", "reporting.publish", "sources.read", "sources.query", "topics.read", "cw.tenant.write:" + f.author.Tenant(), "cw.report.write:" + reportID, "cw.report.read:" + reportID, "cw.report.preview:" + reportID, "cw.report.execute:" + reportID, "cw.report.publish:" + reportID, "cw.block.read:" + blockID, "cw.block.execute:" + blockID, "cw.block.preview:" + blockID, "cw.block.publish:" + blockID, "cw.source.query:" + f.base.Source}
	for _, ref := range blockSnapshot.References {
		scope := "cw." + ref.Kind + "." + ref.Permission + ":" + ref.ID
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	author := phase27Actor(t, f.f, f.author.User(), scopes)
	actor := func(user string, scopes []string) identity.Envelope { return phase27Actor(t, f.f, user, scopes) }
	without := func(scope string) identity.Envelope {
		return actor(author.User(), slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == scope }))
	}
	service, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	w := phase29BlockWidget("selected", blockID, 0, "table-main")
	w.Block.Revision, w.Block.Digest, w.Block.Policy = chart.Revision, chart.Digest, "private_preview"
	w.Presentation = reporting.Presentation{Title: "Keep this presentation", Density: "compact"}
	w.Bindings = []reporting.FilterBinding{{Filter: "threshold", Parameter: "minimum"}}
	w.Overrides = []string{"minimum"}
	otherWidget := phase27Copy(t, w)
	otherWidget.ID = "unselected"
	otherWidget.Block.Outputs = []string{"table-second"}
	otherWidget.Bindings = nil
	otherWidget.Literals = []reporting.Argument{{Name: "minimum", Value: reporting.Value{Literal: "1"}}}
	d := phase29Text("Manual lifecycle")
	note := d.Widgets[0]
	note.ID, note.Grid.Row = "note", 2
	d.SchemaVersion, d.Widgets = reporting.PagedDocumentVersion, nil
	d.ReportPages = []reporting.ReportPage{
		{ID: "analysis", Title: "Analysis", Widgets: []reporting.Widget{w, note}, Filters: []reporting.ReportFilter{{Parameter: reporting.Parameter{Name: "threshold", Type: "integer", Default: &reporting.Value{Literal: "0"}}, Label: "Threshold"}}, Defaults: []reporting.Argument{{Name: "minimum", Value: reporting.Value{Literal: "0"}}}},
		{ID: "second", Title: "Untouched page", Widgets: []reporting.Widget{otherWidget}},
	}
	state, err := service.Create(ctx, author, reporting.AuthoringCreateRequest{ID: reportID, Definition: d})
	if err != nil {
		t.Fatal(err)
	}
	original, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: reportID})
	if err != nil {
		t.Fatal(err)
	}
	publish := reporting.AuthoringBlockPublishRequest{Block: blockID, ExpectedVersion: chart.State.Version, Revision: chart.Revision, Digest: chart.Digest, Evidence: "missing-evidence"}
	rebind := reporting.AuthoringRebindPublishedRequest{Report: reportID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Digest: original.Digest, Widgets: []reporting.AuthoringPublishedWidget{{Widget: "selected", Block: blockID, Revision: chart.Revision, Digest: chart.Digest}}}
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	inspect := func() reporting.AuthoringLifecycleView {
		t.Helper()
		out, err := service.InspectLifecycle(ctx, author, reporting.AuthoringLifecycleRequest{Report: reportID, Revision: original.Revision})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	initial := inspect()
	if initial.Report == nil || initial.Stage != "draft" || len(initial.Blocks) != 1 || len(initial.Blocks[0].Block.Outputs) != len(f.base.Outputs) || initial.Blocks[0].ValidationFresh || initial.Blocks[0].CanPublish || initial.Blocks[0].PublicationScope != "entire_revision" || initial.Blocks[0].AudienceEffect != "existing_authorized_readers" {
		t.Fatal("incomplete whole-revision disclosure", initial)
	}
	encoded, err := json.Marshal(initial)
	if err != nil || strings.Contains(string(encoded), f.base.SQL) || strings.Contains(string(encoded), `"sql"`) {
		t.Fatal("lifecycle inspection exposed SQL", err)
	}
	if _, err := service.PublishBlock(ctx, author, publish); err == nil {
		t.Fatal("unvalidated chart published")
	}
	if _, err := service.RebindPublished(ctx, author, rebind); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("unpublished revision rebound", err)
	}
	if f.attemptCount(t) != before || f.f.model.requests.Load() != models {
		t.Fatal("prepublication lifecycle executed source/model")
	}
	validation, err := f.blocks.Validate(ctx, f.blockAuthor, blockID, reporting.ValidateRequest{ExpectedVersion: chart.State.Version})
	if err != nil {
		t.Fatal(err)
	}
	publish.ExpectedVersion, publish.Evidence = validation.State.Version, validation.Evidence.ID
	preview, err := service.Preview(ctx, author, reporting.AuthoringPreviewRequest{Report: reportID, Revision: state.DraftRevision, Key: "manual-private-preview"})
	if err != nil || !preview.Private {
		t.Fatal(preview, err)
	}
	if _, err := service.Execute(ctx, author, reporting.AuthoringExecuteRequest{Run: preview.ID}); err != nil {
		t.Fatal(err)
	}
	before = f.attemptCount(t)
	if out := inspect(); !out.Blocks[0].ValidationFresh || !out.Blocks[0].CanPublish {
		t.Fatal("fresh native validation not disclosed", out)
	}

	t.Run("exact-authority-and-custody-before-publication", func(t *testing.T) {
		claims := f.f.f.token.claims("foreign-tenant", author.User(), scopes)
		foreign, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range []identity.Envelope{actor("different-author", scopes), foreign, without("cw.block.publish:" + blockID), without("reporting.publish"), without("cw.execution_context.use:" + f.base.Context)} {
			if _, err := service.PublishBlock(ctx, e, publish); err == nil {
				t.Fatal("foreign actor/tenant or revoked authority published")
			}
		}
		for _, e := range []identity.Envelope{actor("different-author", scopes), foreign, without("cw.block.preview:" + blockID), without("cw.execution_context.use:" + f.base.Context)} {
			if _, err := service.InspectLifecycle(ctx, e, reporting.AuthoringLifecycleRequest{Report: reportID, Revision: original.Revision}); err == nil {
				t.Fatal("private inspection ignored original actor/tenant/dependency")
			}
		}
		for _, mutate := range []func(*reporting.AuthoringBlockPublishRequest){
			func(p *reporting.AuthoringBlockPublishRequest) { p.ExpectedVersion++ },
			func(p *reporting.AuthoringBlockPublishRequest) { p.Revision++ },
			func(p *reporting.AuthoringBlockPublishRequest) { p.Digest = strings.Repeat("f", 64) },
			func(p *reporting.AuthoringBlockPublishRequest) { p.Evidence = "other-evidence" },
		} {
			bad := publish
			mutate(&bad)
			if _, err := service.PublishBlock(ctx, author, bad); err == nil {
				t.Fatal("stale publication pin admitted", bad)
			}
		}
	})

	published, err := service.PublishBlock(ctx, author, publish)
	if err != nil || published.PublishedRevision != chart.Revision {
		t.Fatal(published, err)
	}
	observed, err := service.InspectLifecycle(ctx, author, reporting.AuthoringLifecycleRequest{Block: blockID, Revision: chart.Revision})
	if err != nil || len(observed.Blocks) != 1 || observed.Blocks[0].PublishedAt == nil || observed.Blocks[0].Block.Private || observed.Blocks[0].CanPublish {
		t.Fatal("publication outcome was not inspectable", observed, err)
	}
	if _, err := service.PublishBlock(ctx, author, publish); !errors.Is(err, store.ErrConflict) {
		t.Fatal("original mutation silently repeated", err)
	}
	t.Run("published-chart-failed-report-cas-preserves-private-work", func(t *testing.T) {
		for _, mutate := range []func(*reporting.AuthoringRebindPublishedRequest){
			func(p *reporting.AuthoringRebindPublishedRequest) { p.ExpectedVersion++ },
			func(p *reporting.AuthoringRebindPublishedRequest) { p.Digest = strings.Repeat("e", 64) },
			func(p *reporting.AuthoringRebindPublishedRequest) { p.Widgets[0].Digest = strings.Repeat("e", 64) },
			func(p *reporting.AuthoringRebindPublishedRequest) { p.Widgets[0].Revision++ },
		} {
			bad := phase27Copy(t, rebind)
			mutate(&bad)
			if _, err := service.RebindPublished(ctx, author, bad); err == nil {
				t.Fatal("stale report/widget pin admitted")
			}
		}
		if _, err := service.RebindPublished(ctx, actor("different-author", scopes), rebind); err == nil {
			t.Fatal("published block erased private report actor custody")
		}
		if _, err := service.RebindPublished(ctx, without("cw.execution_context.use:"+f.base.Context), rebind); err == nil {
			t.Fatal("rebind ignored revoked dependency")
		}
		out := inspect()
		if !out.Report.Private || out.Report.State != original.State || !reflect.DeepEqual(out.Report.Definition, original.Definition) || out.Blocks[0].PublishedAt == nil {
			t.Fatal("failed rebind lost private work or falsely rolled back publication", out)
		}
	})
	state, err = service.RebindPublished(ctx, author, rebind)
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: reportID})
	want := phase27Copy(t, original.Definition)
	want.ReportPages[0].Widgets[0].Block.Policy, want.ReportPages[0].Widgets[0].Block.Digest = "published", ""
	if err != nil || !current.Private || state.DraftRevision != original.Revision+1 || !reflect.DeepEqual(current.Definition, want) {
		t.Fatal("rebind changed more than policy/private digest", current, err)
	}
	if _, err := service.Read(ctx, actor("different-author", scopes), reporting.AuthoringReadRequest{Report: reportID, Revision: original.Revision}); err == nil {
		t.Fatal("older private reference lost actor fence")
	}
	// The explicit selection must leave another private widget untouched, even
	// when it points to the same already-published block revision.
	if _, err := service.TransitionReport(ctx, author, reporting.AuthoringReportTransitionRequest{Report: reportID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Operation: "review"}); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("unselected private widget silently rebound", err)
	}
	secondRebind := rebind
	secondRebind.ExpectedVersion, secondRebind.Revision, secondRebind.Digest = state.Version, current.Revision, current.Digest
	secondRebind.Widgets = []reporting.AuthoringPublishedWidget{{Widget: "unselected", Block: blockID, Revision: chart.Revision, Digest: chart.Digest}}
	state, err = service.RebindPublished(ctx, author, secondRebind)
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.Read(ctx, author, reporting.AuthoringReadRequest{Report: reportID})
	want.ReportPages[1].Widgets[0].Block.Policy, want.ReportPages[1].Widgets[0].Block.Digest = "published", ""
	if err != nil || !reflect.DeepEqual(current.Definition, want) {
		t.Fatal("second explicit rebind changed unrelated content", current, err)
	}
	// A published dependency never acquires a block-preview requirement from
	// the still-private report. Report preview authority remains independent.
	if _, err := service.InspectLifecycle(ctx, without("cw.block.preview:"+blockID), reporting.AuthoringLifecycleRequest{Report: reportID, Revision: current.Revision}); err != nil {
		t.Fatal("published dependency borrowed parent private policy", err)
	}
	transition := reporting.AuthoringReportTransitionRequest{Report: reportID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Operation: "publish", Note: "Explicit publication"}
	if _, err := service.TransitionReport(ctx, author, transition); !errors.Is(err, store.ErrConflict) {
		t.Fatal("report skipped independent review", err)
	}
	transition.Operation = "review"
	state, err = service.TransitionReport(ctx, author, transition)
	if err != nil || state.DraftRevision != 0 || state.ReviewRevision != current.Revision || state.PublishedRevision != 0 {
		t.Fatal(state, err)
	}
	t.Run("review-stage-remains-discoverable-and-reopenable", func(t *testing.T) {
		list, err := service.Drafts(ctx, author, reporting.DraftListRequest{Limit: 10})
		if err != nil || len(list.Items) != 1 || list.Items[0].ID != reportID || list.Items[0].Stage != "review" || list.Items[0].ReviewRevision != state.ReviewRevision || list.Items[0].DraftRevision != 0 {
			t.Fatal("review vanished from manual catalog", list, err)
		}
		for _, request := range []reporting.AuthoringReadRequest{{Report: reportID}, {Report: reportID, Stage: "review"}, {Report: reportID, Revision: state.ReviewRevision}} {
			v, err := service.Read(ctx, author, request)
			if err != nil || v.Revision != state.ReviewRevision || !v.Private || v.State.ReviewRevision != state.ReviewRevision {
				t.Fatal("review could not reopen", request, v, err)
			}
		}
		if _, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: reportID, Stage: "draft"}); err == nil {
			t.Fatal("explicit missing draft silently selected review")
		}
		if _, err := service.Read(ctx, without("cw.report.preview:"+reportID), reporting.AuthoringReadRequest{Report: reportID, Stage: "review"}); err == nil {
			t.Fatal("review read bypassed private preview")
		}
		list, err = service.Drafts(ctx, without("cw.execution_context.use:"+f.base.Context), reporting.DraftListRequest{Limit: 10})
		if err != nil || len(list.Items) != 0 {
			t.Fatal("review catalog projected revoked dependency", list, err)
		}
		public, err := f.documents.List(ctx, author, "report", "", 10)
		if err != nil || len(public.Items) != 0 {
			t.Fatal("review became public catalog content", public, err)
		}
	})
	transition.ExpectedVersion, transition.Revision, transition.Operation = state.Version, state.ReviewRevision, "reject"
	if _, err := service.TransitionReport(ctx, without("cw.report.publish:"+reportID), transition); err == nil {
		t.Fatal("writer rejected without native publish authority")
	}
	withoutNote := transition
	withoutNote.Note = ""
	if _, err := service.TransitionReport(ctx, author, withoutNote); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("reject accepted no note", err)
	}
	transition.Operation = "publish"
	if _, err := service.TransitionReport(ctx, without("cw.report.publish:"+reportID), transition); err == nil {
		t.Fatal("writer published report without native publish authority")
	}
	if _, err := service.TransitionReport(ctx, without("cw.execution_context.use:"+f.base.Context), transition); err == nil {
		t.Fatal("report publication ignored revoked dependency")
	}
	state, err = service.TransitionReport(ctx, author, transition)
	if err != nil || state.PublishedRevision != current.Revision || state.ReviewRevision != 0 {
		t.Fatal(state, err)
	}
	out, err := service.InspectLifecycle(ctx, author, reporting.AuthoringLifecycleRequest{Report: reportID, Revision: state.PublishedRevision})
	if err != nil || out.Report.Private || out.Stage != "published" || out.Report.State != state {
		t.Fatal("exact report publication recovery failed", out, err)
	}
	consumerScopes := slices.DeleteFunc(slices.Clone(scopes), func(s string) bool {
		return s == "reporting.write" || s == "reporting.preview" || s == "reporting.publish" || strings.Contains(s, ".write:") || strings.Contains(s, ".preview:") || strings.Contains(s, ".publish:")
	})
	consumer := actor("existing-reader", consumerScopes)
	public, err := f.documents.Read(ctx, consumer, "report", reportID, reporting.DocumentReference{})
	if err != nil || public.Private || public.Revision != state.PublishedRevision {
		t.Fatal("existing authorized Consumer could not read publication", public, err)
	}
	readerScopes := []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + reportID, "cw.execution_context.use:" + f.base.Context}
	retained, err := f.compositions.Get(ctx, actor(author.User(), readerScopes), preview.ID)
	if err != nil || !retained.Private {
		t.Fatal("original retained preview lost privacy", retained, err)
	}
	if _, err := f.compositions.Get(ctx, actor("existing-reader", readerScopes), preview.ID); err == nil {
		t.Fatal("report publication exposed original private preview")
	}
	t.Run("imported-report-rebind-preserves-empty-defaults", func(t *testing.T) {
		const importedID = "imported-private-report"
		importScopes := slices.Clone(scopes)
		for i := range importScopes {
			importScopes[i] = strings.ReplaceAll(importScopes[i], reportID, importedID)
		}
		importer := actor(author.User(), importScopes)
		definition := phase27Copy(t, d)
		definition.PartialFailure = ""
		raw, err := json.Marshal(definition)
		if err != nil {
			t.Fatal(err)
		}
		imported, err := f.documents.Import(ctx, importer, "report", importedID, raw, reporting.ExternalReference{System: "synthetic", ID: "manual-import", Version: "v1"})
		if err != nil || imported.State == nil {
			t.Fatal(imported, err)
		}
		prior, err := service.Read(ctx, importer, reporting.AuthoringReadRequest{Report: importedID})
		if err != nil || prior.Definition.PartialFailure != "" {
			t.Fatal(prior, err)
		}
		pins := []reporting.AuthoringPublishedWidget{{Widget: "selected", Block: blockID, Revision: chart.Revision, Digest: chart.Digest}, {Widget: "unselected", Block: blockID, Revision: chart.Revision, Digest: chart.Digest}}
		if _, err := service.RebindPublished(ctx, importer, reporting.AuthoringRebindPublishedRequest{Report: importedID, ExpectedVersion: prior.State.Version, Revision: prior.Revision, Digest: prior.Digest, Widgets: pins}); err != nil {
			t.Fatal(err)
		}
		for p := range definition.ReportPages {
			for i := range definition.ReportPages[p].Widgets {
				w := &definition.ReportPages[p].Widgets[i]
				if w.Block != nil {
					w.Block.Policy, w.Block.Digest = "published", ""
				}
			}
		}
		after, err := service.Read(ctx, importer, reporting.AuthoringReadRequest{Report: importedID})
		if err != nil || !reflect.DeepEqual(after.Definition, definition) {
			t.Fatal("rebind normalized unrelated imported content", after, err)
		}
	})
	t.Run("newer-draft-and-review-remain-independent-on-reject", func(t *testing.T) {
		amended, err := service.Save(ctx, author, reporting.AuthoringSaveRequest{Report: reportID, ExpectedVersion: state.Version, Revision: current.Revision, Definition: current.Definition})
		if err != nil {
			t.Fatal(err)
		}
		review, err := service.TransitionReport(ctx, author, reporting.AuthoringReportTransitionRequest{Report: reportID, ExpectedVersion: amended.Version, Revision: amended.DraftRevision, Operation: "review"})
		if err != nil {
			t.Fatal(err)
		}
		newer, err := service.Save(ctx, author, reporting.AuthoringSaveRequest{Report: reportID, ExpectedVersion: review.Version, Revision: review.ReviewRevision, Definition: current.Definition})
		if err != nil || newer.ReviewRevision != review.ReviewRevision || newer.DraftRevision == review.ReviewRevision {
			t.Fatal(newer, err)
		}
		list, err := service.Drafts(ctx, author, reporting.DraftListRequest{Limit: 10})
		if err != nil || len(list.Items) != 1 || list.Items[0].Stage != "draft" || list.Items[0].ReviewRevision != review.ReviewRevision || list.Items[0].DraftRevision != newer.DraftRevision {
			t.Fatal(list, err)
		}
		opened, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: reportID, Stage: "review"})
		if err != nil || opened.Revision != review.ReviewRevision {
			t.Fatal(opened, err)
		}
		// Domain rejection preserves native publish authority; editor write is
		// an optional transport ceiling, never an invented rejection grant.
		rejected, err := service.TransitionReport(ctx, without("reporting.write"), reporting.AuthoringReportTransitionRequest{Report: reportID, ExpectedVersion: newer.Version, Revision: review.ReviewRevision, Operation: "reject", Note: "Revise this exact review"})
		if err != nil || rejected.DraftRevision != newer.DraftRevision || rejected.PublishedRevision != state.PublishedRevision || rejected.ReviewRevision != 0 {
			t.Fatal(rejected, err)
		}
		if _, err := service.TransitionReport(ctx, author, reporting.AuthoringReportTransitionRequest{Report: reportID, ExpectedVersion: rejected.Version, Revision: review.ReviewRevision, Operation: "review"}); !errors.Is(err, store.ErrConflict) {
			t.Fatal("rejected revision resubmitted without a new edit", err)
		}
		restarted, err := reporting.NewAuthoring(f.documents, f.compositions)
		if err != nil {
			t.Fatal(err)
		}
		oldReview, err := restarted.InspectLifecycle(ctx, author, reporting.AuthoringLifecycleRequest{Report: reportID, Revision: review.ReviewRevision})
		if err != nil || !oldReview.Rejected || oldReview.CanReview {
			t.Fatal("rejected history lost after service restart", oldReview, err)
		}
		nextReview, err := restarted.TransitionReport(ctx, author, reporting.AuthoringReportTransitionRequest{Report: reportID, ExpectedVersion: rejected.Version, Revision: rejected.DraftRevision, Operation: "review"})
		if err != nil {
			t.Fatal(err)
		}
		returned, err := restarted.TransitionReport(ctx, author, reporting.AuthoringReportTransitionRequest{Report: reportID, ExpectedVersion: nextReview.Version, Revision: nextReview.ReviewRevision, Operation: "reject", Note: "Create an amended revision"})
		if err != nil || returned.DraftRevision != nextReview.ReviewRevision {
			t.Fatal(returned, err)
		}
		reopened, err := restarted.InspectLifecycle(ctx, author, reporting.AuthoringLifecycleRequest{Report: reportID})
		if err != nil || !reopened.Rejected || reopened.CanReview || reopened.Stage != "draft" {
			t.Fatal("rejected current draft advertised review", reopened, err)
		}
		changed := phase27Copy(t, reopened.Report.Definition)
		changed.Metadata[0].Title += " amended"
		amendedAgain, err := restarted.Save(ctx, author, reporting.AuthoringSaveRequest{Report: reportID, ExpectedVersion: returned.Version, Revision: returned.DraftRevision, Definition: changed})
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := restarted.InspectLifecycle(ctx, author, reporting.AuthoringLifecycleRequest{Report: reportID, Revision: amendedAgain.DraftRevision})
		if err != nil || fresh.Rejected || !fresh.CanReview {
			t.Fatal("amendment did not restore native review eligibility", fresh, err)
		}

	})
	if f.attemptCount(t) != before || f.f.model.requests.Load() != models {
		t.Fatal("lifecycle or Consumer metadata read executed source/model")
	}
}
