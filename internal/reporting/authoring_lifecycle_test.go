package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestAuthoringLifecycleDenialBeforeIO(t *testing.T) {
	s := &Authoring{}
	e := authoringEnvelope(t, "reporting.read", "cw.report.read:report", "cw.block.read:block")
	if _, err := s.TransitionReport(t.Context(), e, AuthoringReportTransitionRequest{Report: "report", ExpectedVersion: 1, Revision: 1, Operation: "review"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	for _, operation := range []string{"publish", "reject"} {
		writer := authoringEnvelope(t, "reporting.write", "cw.report.write:report")
		if _, err := s.TransitionReport(t.Context(), writer, AuthoringReportTransitionRequest{Report: "report", ExpectedVersion: 1, Revision: 1, Operation: operation, Note: "Explicit note"}); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("write substituted for native publish/reject authority", err)
		}
	}
	for _, operation := range []string{"archive", "withdraw", "review-and-publish", ""} {
		if _, err := s.TransitionReport(t.Context(), e, AuthoringReportTransitionRequest{Report: "report", ExpectedVersion: 1, Revision: 1, Operation: operation}); !errors.Is(err, ErrInvalid) {
			t.Fatal("open manual transition enum", err)
		}
	}
	if _, err := s.TransitionReport(t.Context(), e, AuthoringReportTransitionRequest{Report: "report", ExpectedVersion: 1, Revision: 1, Operation: "reject"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("reject accepted absent note", err)
	}
	if _, err := s.PublishBlock(t.Context(), e, AuthoringBlockPublishRequest{Block: "block", Revision: 1, ExpectedVersion: 1, Digest: strings.Repeat("a", 64), Evidence: "evidence"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	for _, in := range []AuthoringLifecycleRequest{
		{}, {Report: "report", Block: "block", Revision: 1}, {Block: "block"}, {Block: "block", Revision: 257}, {Block: "block", Revision: 1, Stage: "draft"}, {Report: "report", Revision: 1, Stage: "review"}, {Report: "report", Stage: "published"},
	} {
		if _, err := s.InspectLifecycle(t.Context(), e, in); !errors.Is(err, ErrInvalid) {
			t.Fatal("ambiguous inspection target accepted", in, err)
		}
	}
	wildcard := authoringEnvelope(t, "reporting.read", "cw.block.read:*")
	if _, err := s.InspectLifecycle(t.Context(), wildcard, AuthoringLifecycleRequest{Block: "block", Revision: 1}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("wildcard lifecycle accepted", err)
	}
	for _, in := range []AuthoringReadRequest{{Report: "report", Stage: "review"}, {Report: "report", Stage: "draft"}, {Report: "report"}, {Report: "report", Revision: 1}} {
		if _, err := authoringDocumentReference(in); err != nil {
			t.Fatal(in, err)
		}
	}
}

func TestAuthoringLifecycleFreshEvidenceAndWholeRevisionDisclosure(t *testing.T) {
	s, repo, e, _ := authoringBlockFixture(t, false)
	key := authoringBlockKey(e.Tenant(), "block")
	base := repo.revisions[key][1]
	r := base.Revision
	v := &ValidationRecord{Rules: clone(r.Definition.Rules), Evidence: Evidence{ID: "evidence", Revision: r.Number, RevisionID: r.ID, DefinitionDigest: r.Digest, ExecutionDigest: r.ExecutionDigest, CanonicalizationVersion: CanonicalizationVersion, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Attempt: exec.Attempt{Status: "succeeded"}}}
	v.Evidence.DependencyDigest = DependencyDigest(v.Dependencies, r.Definition.Topics, v.Rules)
	base.Validation, base.Current, base.Health = v, true, Health{Status: "healthy", DependencyDigest: v.Evidence.DependencyDigest}
	repo.revisions[key][1] = clone(base)
	request := AuthoringLifecycleRequest{Block: "block", Revision: 1}
	out, err := s.InspectLifecycle(t.Context(), e, request)
	if err != nil || len(out.Blocks) != 1 || !out.Blocks[0].ValidationFresh || !out.Blocks[0].CanPublish || len(out.Blocks[0].Block.Outputs) != 2 || out.Blocks[0].Block.Actor != e.User() || out.Blocks[0].Block.Digest != r.Digest {
		t.Fatal("whole exact fresh revision not disclosed", out, err)
	}
	for name, mutate := range map[string]func(*Snapshot){
		"expired":      func(s *Snapshot) { s.Validation.Evidence.ExpiresAt = time.Now().Add(-time.Second) },
		"definition":   func(s *Snapshot) { s.Validation.Evidence.DefinitionDigest = strings.Repeat("f", 64) },
		"revision":     func(s *Snapshot) { s.Validation.Evidence.Revision++ },
		"execution":    func(s *Snapshot) { s.Validation.Evidence.ExecutionDigest = strings.Repeat("f", 64) },
		"dependencies": func(s *Snapshot) { s.Current = false },
		"health":       func(s *Snapshot) { s.Health.Status = "unavailable" },
		"missing":      func(s *Snapshot) { s.Validation = nil },
	} {
		t.Run(name, func(t *testing.T) {
			changed := clone(base)
			mutate(&changed)
			repo.revisions[key][1] = changed
			out, err := s.InspectLifecycle(t.Context(), e, request)
			if err != nil || out.Blocks[0].ValidationFresh || out.Blocks[0].CanPublish {
				t.Fatal("stale validation advertised for publication", out, err)
			}
			if _, err := s.PublishBlock(t.Context(), e, AuthoringBlockPublishRequest{Block: "block", ExpectedVersion: base.State.Version, Revision: 1, Digest: r.Digest, Evidence: "evidence"}); !errors.Is(err, ErrStale) {
				t.Fatal("stale validation published", err)
			}
		})
	}
	repo.revisions[key][1] = clone(base)
	noPublish := authoringBlockActor(t, e.Tenant(), e.User(), slices.DeleteFunc(authoringBlockScopes(), func(s string) bool { return s == "cw.block.publish:block" }))
	out, err = s.InspectLifecycle(t.Context(), noPublish, request)
	if err != nil || !out.Blocks[0].ValidationFresh || out.Blocks[0].CanPublish {
		t.Fatal("data evidence became publication authority", out, err)
	}
	for _, who := range []struct{ tenant, actor string }{{"foreign", e.User()}, {e.Tenant(), "different-author"}} {
		if _, err := s.InspectLifecycle(t.Context(), authoringBlockActor(t, who.tenant, who.actor, authoringBlockScopes()), request); err == nil {
			t.Fatal("private block crossed actor/tenant")
		}
	}
	if repo.commits != 0 {
		t.Fatal("metadata inspection or rejected publication mutated store")
	}
}

// This double exercises bounded projection, not persistence eligibility. The
// production PostgreSQL checks remain covered by the lifecycle acceptance test.
type lifecycleProjectionDocumentRepo struct {
	DocumentRepository
	snapshot DocumentSnapshot
}

func (r lifecycleProjectionDocumentRepo) ReadDocument(context.Context, identity.Envelope, string, string, DocumentReference, Access, bool) (DocumentSnapshot, error) {
	return clone(r.snapshot), nil
}

func TestAuthoringLifecycleProjectionBudget(t *testing.T) {
	size, err := accountLifecycleProjection(authoringLifecycleMaxBytes-2, "", 0)
	if err != nil || size != authoringLifecycleMaxBytes {
		t.Fatal("exact byte ceiling", size, err)
	}
	if next, err := accountLifecycleProjection(size, "", 1); !errors.Is(err, ErrBudget) || next != size {
		t.Fatal("framing escaped aggregate budget", next, err)
	}
	value := "escaped <>& text 🐧"
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if size, err := accountLifecycleProjection(7, value, 1); err != nil || size != 8+len(raw) {
		t.Fatal("counted characters instead of serialized bytes", size, err)
	}

	s, repo, _, definition := authoringBlockFixture(t, false)
	scopes := append(authoringBlockScopes(), "cw.report.read:report", "cw.report.write:report", "cw.report.preview:report")
	e := authoringBlockActor(t, "tenant", "author", scopes)
	key := authoringBlockKey(e.Tenant(), "block")
	baseline := repo.revisions[key][1]
	// Every individual revision is well below the aggregate limit. Distinct
	// immutable revisions of one authorized block can still exceed it together.
	for i := 0; i < 120; i++ {
		definition.ExpectedSchema = append(definition.ExpectedSchema, exec.Field{Name: "field_" + strconv.Itoa(i) + "_" + strings.Repeat("x", 100), Type: "text", NativeType: "text", Encoding: "string"})
	}
	d := documentFixture()
	d.SchemaVersion, d.Widgets = PagedDocumentVersion, nil
	d.ReportPages = []ReportPage{{ID: "main", Title: "Bounded lifecycle", Widgets: []Widget{}}}
	for revision := int64(1); revision <= 100; revision++ {
		pin := clone(baseline)
		pin.Revision.Number, pin.Revision.Definition, pin.Validation = revision, clone(definition), nil
		pin.Revision.Digest = DefinitionDigest(definition)
		repo.revisions[key][revision] = pin
		d.ReportPages[0].Widgets = append(d.ReportPages[0].Widgets, Widget{ID: "widget_" + strconv.FormatInt(revision, 10), Kind: "block", Grid: GridCell{Row: int(revision - 1), Width: 12, Height: 1}, Block: &BlockWidget{Block: "block", Revision: revision, Digest: pin.Revision.Digest, Policy: "private_preview", Outputs: []string{definition.Outputs[0].ID}}})
	}
	raw, err = json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectStoredDocument(raw, "report"); err != nil {
		t.Fatal("invalid bounded report fixture", err)
	}
	s.documents.repo = lifecycleProjectionDocumentRepo{snapshot: DocumentSnapshot{State: DocumentState{Kind: "report", ID: "report", Version: 1, DraftRevision: 1, LatestRevision: 1}, Revision: DocumentRevision{Number: 1, Raw: raw, Digest: DocumentDigest(raw)}}}
	one, err := s.InspectLifecycle(t.Context(), e, AuthoringLifecycleRequest{Block: "block", Revision: 1})
	if err != nil || len(one.Blocks) != 1 {
		t.Fatal("single revision unexpectedly exceeded budget", err)
	}
	oneJSON, err := json.Marshal(one.Blocks[0])
	if err != nil || len(oneJSON)*100 <= authoringLifecycleMaxBytes {
		t.Fatal("fixture cannot exercise aggregate overflow", len(oneJSON), err)
	}
	repo.reads = 0
	out, err := s.InspectLifecycle(t.Context(), e, AuthoringLifecycleRequest{Report: "report", Revision: 1})
	if !errors.Is(err, ErrBudget) || out.Version != "" || out.Report != nil || out.Blocks != nil || repo.reads >= 100 {
		t.Fatal("aggregate overflow returned partial output or loaded remaining fanout", repo.reads, out, err)
	}

	// Deduplication saves response bytes, never per-widget private custody.
	d.ReportPages[0].Widgets = d.ReportPages[0].Widgets[:2]
	d.ReportPages[0].Widgets[1].Block = clone(d.ReportPages[0].Widgets[0].Block)
	d.ReportPages[0].Widgets[1].Block.Digest = strings.Repeat("f", 64)
	raw, _ = json.Marshal(d)
	s.documents.repo = lifecycleProjectionDocumentRepo{snapshot: DocumentSnapshot{State: DocumentState{Kind: "report", ID: "report", Version: 1, DraftRevision: 1, LatestRevision: 1}, Revision: DocumentRevision{Number: 1, Raw: raw, Digest: DocumentDigest(raw)}}}
	out, err = s.InspectLifecycle(t.Context(), e, AuthoringLifecycleRequest{Report: "report", Revision: 1})
	if !errors.Is(err, ErrStale) || out.Report != nil || out.Blocks != nil {
		t.Fatal("deduplicated private pin skipped its own custody check", out, err)
	}
}

func TestAuthoringLifecycleNativeActionHints(t *testing.T) {
	writer := authoringEnvelope(t, "reporting.write", "cw.report.write:report")
	publisher := authoringEnvelope(t, "reporting.publish", "cw.report.publish:report")
	out := AuthoringLifecycleView{Stage: "draft", Report: &DocumentView{Revision: 1, State: DocumentState{ID: "report", DraftRevision: 1}, Definition: DocumentDefinition{SchemaVersion: 3, ReportPages: []ReportPage{{ID: "main", Title: "Summary"}}}}}
	setReportLifecycleHints(writer, &out)
	if !out.CanReview || out.CanPublish || out.CanReject {
		t.Fatal("draft writer hints", out.CanReview, out.CanPublish, out.CanReject)
	}
	out.Rejected = true
	setReportLifecycleHints(writer, &out)
	if out.CanReview {
		t.Fatal("rejected revision advertised resubmission")
	}
	out.Rejected = false
	out.Report.State.ReviewRevision = 2
	setReportLifecycleHints(writer, &out)
	if out.CanReview {
		t.Fatal("pending independent review would be replaced")
	}
	out.Report.State.ReviewRevision = 0
	out.Report.Definition.ReportPages[0].Widgets = []Widget{{Kind: "block", Block: &BlockWidget{Policy: "private_preview"}}}
	setReportLifecycleHints(writer, &out)
	if out.CanReview {
		t.Fatal("private reference advertised review before explicit rebind")
	}
	out.Stage = "review"
	out.Report.State.ReviewRevision = 1
	setReportLifecycleHints(writer, &out)
	if out.CanPublish || out.CanReject || out.CanReview {
		t.Fatal("write implied publish or reject")
	}
	setReportLifecycleHints(publisher, &out)
	if !out.CanPublish || !out.CanReject || out.CanReview {
		t.Fatal("native publication hint missing")
	}
	out.Report.State.Archived = true
	setReportLifecycleHints(publisher, &out)
	if out.CanPublish || out.CanReject || out.CanReview {
		t.Fatal("archived transition advertised")
	}
	out.Report.State.Archived = false
	out.Stage = "published"
	setReportLifecycleHints(publisher, &out)
	if out.CanPublish || out.CanReject || out.CanReview {
		t.Fatal("published view advertised retry")
	}
}
