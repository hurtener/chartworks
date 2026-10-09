package acceptance

import (
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
	"slices"
	"strings"
	"testing"
	"time"
)

// The bridge uses real private block definitions, native validation/execution,
// PostgreSQL reference custody and the ordinary immutable report lifecycle.
func TestReportPrivateBlockBridge(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	draft, err := f.blocks.Create(ctx, f.blockAuthor, reporting.CreateRequest{ID: "private-page-chart", Definition: f.base})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, draft.State.ID, reporting.Reference{Revision: draft.Revision}, reporting.Read)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.execute", "reporting.publish", "sources.read", "sources.query", "topics.read", "cw.tenant.write:" + f.author.Tenant(), "cw.report.write:private-page-report", "cw.report.read:private-page-report", "cw.report.preview:private-page-report", "cw.report.execute:private-page-report", "cw.report.publish:private-page-report", "cw.block.read:" + draft.State.ID, "cw.block.execute:" + draft.State.ID, "cw.block.preview:" + draft.State.ID, "cw.source.query:" + f.base.Source}
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
	w := phase29BlockWidget("local-chart", draft.State.ID, 0, "table-main")
	w.Block.Revision = draft.Revision
	w.Block.Digest = draft.Digest
	w.Block.Policy = "private_preview"
	d := phase29Text("Private chart canvas")
	d.SchemaVersion = reporting.PagedDocumentVersion
	d.Widgets = nil
	d.ReportPages = []reporting.ReportPage{{ID: "analysis", Title: "Analysis", Widgets: []reporting.Widget{w}}, {ID: "empty", Title: "Empty", Widgets: []reporting.Widget{}}}
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	state, err := service.Create(ctx, author, reporting.AuthoringCreateRequest{ID: "private-page-report", Definition: d})
	if err != nil {
		t.Fatal("save exact unvalidated private reference", err)
	}
	if _, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID}); err != nil {
		t.Fatal(err)
	}
	if f.attemptCount(t) != before || f.f.model.requests.Load() != models {
		t.Fatal("saving private reference queried source/model")
	}
	assertUnavailable := func(equivalentScopes []string, actor, key string) {
		t.Helper()
		e := phase27Actor(t, f.f, actor, equivalentScopes)
		before := f.attemptCount(t)
		view, err := service.Preview(ctx, e, reporting.AuthoringPreviewRequest{Report: state.ID, Revision: state.DraftRevision, Key: key})
		if err == nil {
			if view.QueryGroups != 0 {
				t.Fatal("invalid private dependency sealed executable work", view)
			}
			if _, err := service.Execute(ctx, e, reporting.AuthoringExecuteRequest{Run: view.ID}); err == nil {
				t.Fatal("unavailable private dependency completed")
			}
		}
		if f.attemptCount(t) != before {
			t.Fatal("private dependency denial executed source")
		}
	}
	assertUnavailable(scopes, author.User(), "unvalidated-private-preview")
	bad := phase27Copy(t, d)
	bad.ReportPages[0].Widgets[0].Block.Digest = strings.Repeat("0", 64)
	if _, err := service.Save(ctx, author, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: bad}); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("wrong private digest admitted", err)
	}
	for _, operation := range []string{"review", "publish"} {
		if _, err := f.documents.Transition(ctx, author, "report", state.ID, state.Version, state.DraftRevision, operation, "Explicit check"); !errors.Is(err, reporting.ErrStale) {
			t.Fatal("private reference entered public lifecycle", operation, err)
		}
	}
	raw := support.Raw(t, f.f.f.dsn)
	if _, err := raw.Exec(ctx, `INSERT INTO chartworks.document_publications(tenant_id,kind,document_id,revision,actor_id) VALUES($1,'report',$2,$3,$4)`, author.Tenant(), state.ID, state.DraftRevision, author.User()); err == nil {
		t.Fatal("storage publication guard bypassed")
	}
	other := phase27Actor(t, f.f, "other-author", scopes)
	if _, err := service.Read(ctx, other, reporting.AuthoringReadRequest{Report: state.ID}); err == nil {
		t.Fatal("same grants erased private block actor fence")
	}
	listed, err := service.Drafts(ctx, other, reporting.DraftListRequest{Limit: 10})
	if err != nil || len(listed.Items) != 0 {
		t.Fatal("private metadata escaped before paging", listed, err)
	}
	if _, err := service.Read(ctx, author, reporting.AuthoringReadRequest{Report: "unapproved-other-report"}); err == nil {
		t.Fatal("block reach became other-document authority")
	}
	claims := f.f.f.token.claims("other-tenant", author.User(), scopes)
	foreign, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Read(ctx, foreign, reporting.AuthoringReadRequest{Report: state.ID}); err == nil {
		t.Fatal("private reference crossed organization")
	}
	for _, missing := range []string{"cw.block.preview:" + draft.State.ID, "cw.execution_context.use:" + f.base.Context} {
		denied := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == missing }))
		if _, err := service.Read(ctx, denied, reporting.AuthoringReadRequest{Report: state.ID}); err == nil {
			t.Fatal("private metadata ignored withdrawn dependency", missing)
		}
	}
	validation, err := f.blocks.Validate(ctx, f.blockAuthor, draft.State.ID, reporting.ValidateRequest{ExpectedVersion: draft.State.Version})
	if err != nil {
		t.Fatal("explicit data validation", err)
	}
	before = f.attemptCount(t)
	assertUnavailable(slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == "cw.block.execute:"+draft.State.ID }), author.User(), "missing-private-execute")
	preview, err := service.Preview(ctx, author, reporting.AuthoringPreviewRequest{Report: state.ID, Revision: state.DraftRevision, Key: "validated-private-preview"})
	if err != nil || !preview.Private || preview.QueryGroups != 1 || len(preview.Pages) != 2 {
		t.Fatal(preview, err)
	}
	done, err := service.Execute(ctx, author, reporting.AuthoringExecuteRequest{Run: preview.ID})
	if err != nil || !done.Private || !done.Complete {
		t.Fatal(done, err)
	}
	if f.attemptCount(t) != before+1 || f.f.model.requests.Load() != models {
		t.Fatal("private preview did not use one frozen query with zero model calls")
	}
	readerScopes := []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + state.ID, "cw.execution_context.use:" + f.base.Context}
	reader := phase27Actor(t, f.f, author.User(), readerScopes)
	payload, err := f.compositions.Widget(ctx, reader, preview.ID, "analysis", "local-chart")
	if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].Chart == nil || len(payload.Outputs[0].Chart.Rows) != 2 {
		t.Fatal(payload, err)
	}
	if _, err := f.compositions.Widget(ctx, phase27Actor(t, f.f, "other-author", readerScopes), preview.ID, "analysis", "local-chart"); err == nil {
		t.Fatal("private result actor fence bypassed")
	}
	// Retain healthy observation, but insert later immutable expired evidence.
	// This isolates the bridge TTL gate from the existing health-status check.
	pending, err := service.Preview(ctx, author, reporting.AuthoringPreviewRequest{Report: state.ID, Revision: state.DraftRevision, Key: "expiry-after-private-seal"})
	if err != nil || pending.QueryGroups != 1 {
		t.Fatal(pending, err)
	}
	currentBlock, err := f.f.f.db.ReadBlock(ctx, author, draft.State.ID, reporting.Reference{Revision: draft.Revision}, reporting.Execute)
	if err != nil || currentBlock.Validation == nil {
		t.Fatal(err)
	}
	expired := phase27Copy(t, *currentBlock.Validation)
	expired.Evidence.ID = strings.Repeat("f", 32)
	expired.Evidence.CreatedAt = expired.Evidence.CreatedAt.Add(time.Microsecond)
	expired.Evidence.ExpiresAt = expired.Evidence.CreatedAt.Add(time.Microsecond)
	if !time.Now().After(expired.Evidence.ExpiresAt) {
		t.Fatal("expiry fixture is not historical")
	}
	expiredJSON, err := json.Marshal(expired)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(ctx, `INSERT INTO chartworks.block_validations(tenant_id,block_id,revision,evidence_id,actor_id,attempt_id,record,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, author.Tenant(), draft.State.ID, draft.Revision, expired.Evidence.ID, author.User(), expired.Evidence.Attempt.ID, expiredJSON, expired.Evidence.CreatedAt, expired.Evidence.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	currentBlock, err = f.f.f.db.ReadBlock(ctx, author, draft.State.ID, reporting.Reference{Revision: draft.Revision}, reporting.Execute)
	if err != nil || currentBlock.Validation == nil || currentBlock.Validation.Evidence.ID != expired.Evidence.ID || currentBlock.Health.Status != "healthy" {
		t.Fatal("fixture did not retain healthy observation with expired evidence", currentBlock.Health, err)
	}
	beforeExpiry := f.attemptCount(t)
	assertUnavailable(scopes, author.User(), "expired-private-admission")
	if _, err := service.Execute(ctx, author, reporting.AuthoringExecuteRequest{Run: pending.ID}); err == nil {
		t.Fatal("expiry after sealing allowed execution")
	}
	if f.attemptCount(t) != beforeExpiry {
		t.Fatal("expired evidence triggered revalidation or a source query")
	}
	if _, err := f.compositions.Widget(ctx, reader, preview.ID, "analysis", "local-chart"); err != nil {
		t.Fatal("evidence expiry erased retained result", err)
	}
	// New private revision cannot borrow the prior revision's data evidence.
	newer, err := f.blocks.Edit(ctx, f.blockAuthor, draft.State.ID, reporting.EditRequest{ExpectedVersion: validation.State.Version, Definition: f.base})
	if err != nil {
		t.Fatal(err)
	}
	unvalidated := phase27Copy(t, d)
	unvalidated.ReportPages[0].Widgets[0].Block.Revision = newer.Revision
	unvalidated.ReportPages[0].Widgets[0].Block.Digest = newer.Digest
	state, err = service.Save(ctx, author, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: unvalidated})
	if err != nil {
		t.Fatal(err)
	}
	assertUnavailable(scopes, author.User(), "new-revision-old-validation")
	nextValidation, err := f.blocks.Validate(ctx, f.blockAuthor, draft.State.ID, reporting.ValidateRequest{ExpectedVersion: newer.State.Version})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.blocks.Publish(ctx, f.blockAuthor, draft.State.ID, reporting.PublishRequest{ExpectedVersion: nextValidation.State.Version, Evidence: nextValidation.Evidence.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.documents.Transition(ctx, author, "report", state.ID, state.Version, state.DraftRevision, "review", "Block now published"); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("block publication silently rewrote report policy", err)
	}
	if _, err := service.Read(ctx, other, reporting.AuthoringReadRequest{Report: state.ID}); err == nil {
		t.Fatal("later block publication exposed older private reference")
	}
	public := phase27Copy(t, unvalidated)
	public.ReportPages[0].Widgets[0].Block.Policy = "published"
	public.ReportPages[0].Widgets[0].Block.Digest = ""
	state, err = service.Save(ctx, author, reporting.AuthoringSaveRequest{Report: state.ID, ExpectedVersion: state.Version, Revision: state.DraftRevision, Definition: public})
	if err != nil {
		t.Fatal(err)
	}
	state = phase29Publish(t, f.documents, author, state)
	retained, err := f.compositions.Get(ctx, reader, preview.ID)
	if err != nil || !retained.Private {
		t.Fatal("later report publication widened old preview", retained, err)
	}
	if _, err := f.documents.Delete(ctx, author, "report", state.ID, reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "erase-private-page-report", Reason: "Remove synthetic report"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.blocks.Read(ctx, author, draft.State.ID, reporting.Reference{Revision: newer.Revision}); err != nil {
		t.Fatal("document deletion inferred block ownership", err)
	}
	if _, err := f.compositions.Widget(ctx, reader, preview.ID, "analysis", "local-chart"); err == nil {
		t.Fatal("deleted report retained live private output")
	}
}
