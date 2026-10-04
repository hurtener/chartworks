package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// AuthoringLifecycleRequest selects one metadata-only inspection target. Block
// inspection always pins an exact revision; report selection also supports its
// independent draft/review pointers. Inspection never retries a mutation.
type AuthoringLifecycleRequest struct {
	Report   string `json:"report,omitempty"`
	Block    string `json:"block,omitempty"`
	Revision int64  `json:"revision"`
	Stage    string `json:"stage,omitempty" jsonschema:"enum=draft,enum=review"`
}

// AuthoringBlockLifecycle discloses every output in the entire revision, even
// when the containing report selected fewer outputs. Eligibility is a current
// hint, not a grant or a promise that a later CAS publication will succeed.
type AuthoringBlockLifecycle struct {
	Block            AuthoringBlockMetadata `json:"block"`
	PublishedAt      *time.Time             `json:"published_at,omitempty"`
	ValidationFresh  bool                   `json:"validation_fresh"`
	CanPublish       bool                   `json:"can_publish"`
	PublicationScope string                 `json:"publication_scope" jsonschema:"enum=entire_revision"`
	AudienceEffect   string                 `json:"audience_effect" jsonschema:"enum=existing_authorized_readers"`
}

type AuthoringLifecycleView struct {
	Rejected   bool                      `json:"rejected"`
	CanReview  bool                      `json:"can_review"`
	CanPublish bool                      `json:"can_publish"`
	CanReject  bool                      `json:"can_reject"`
	Version    string                    `json:"version"`
	Report     *DocumentView             `json:"report,omitempty"`
	Stage      string                    `json:"stage,omitempty"`
	Blocks     []AuthoringBlockLifecycle `json:"blocks"`
}

// Keep the complete lifecycle projection below the app's 4 MiB response
// ceiling. This is one aggregate budget, not a per-block allowance.
const authoringLifecycleMaxBytes = 3 << 20

func accountLifecycleProjection(used int, value any, framing int) (int, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return used, ErrInvalid
	}
	if used < 0 || framing < 0 || used > authoringLifecycleMaxBytes || framing > authoringLifecycleMaxBytes-used || len(raw) > authoringLifecycleMaxBytes-used-framing {
		return used, ErrBudget
	}
	return used + len(raw) + framing, nil
}

func authoringDocumentReference(in AuthoringReadRequest) (DocumentReference, error) {
	if !identity.Identifier(in.Report) || in.Revision < 0 || in.Revision > 256 || !slices.Contains([]string{"", "draft", "review"}, in.Stage) || in.Revision > 0 && in.Stage != "" {
		return DocumentReference{}, ErrInvalid
	}
	ref := DocumentReference{Revision: in.Revision, Stage: in.Stage}
	if ref.Revision == 0 && ref.Stage == "" {
		ref.Stage = "draft"
	}
	return ref, nil
}

// authoringDocumentSnapshot preserves full lifecycle pointers under exact write
// authority without allowing write to stand in for read or private preview.
func (s *Authoring) authoringDocumentSnapshot(ctx context.Context, e identity.Envelope, in AuthoringReadRequest) (DocumentSnapshot, error) {
	ref, err := authoringDocumentReference(in)
	if err != nil {
		return DocumentSnapshot{}, err
	}
	for _, a := range []Access{Write, Read} {
		if err := RequireDocument(e, "report", in.Report, a); err != nil {
			return DocumentSnapshot{}, err
		}
	}
	if s == nil || s.documents == nil {
		return DocumentSnapshot{}, ErrUnavailable
	}
	snapshot, err := s.documents.repo.ReadDocument(ctx, e, "report", in.Report, ref, Read, false)
	if in.Revision == 0 && in.Stage == "" && (errors.Is(err, access.ErrNotFound) || errors.Is(err, store.ErrNotFound)) {
		// Native review deliberately clears the draft pointer. Only a missing
		// default draft falls back; explicit stages and other errors are final.
		snapshot, err = s.documents.repo.ReadDocument(ctx, e, "report", in.Report, DocumentReference{Stage: "review"}, Read, false)
	}
	return snapshot, err
}

func documentLifecycleStage(snapshot DocumentSnapshot) string {
	switch {
	case snapshot.PublishedAt != nil:
		return "published"
	case snapshot.State.ReviewRevision == snapshot.Revision.Number:
		return "review"
	case snapshot.State.DraftRevision == snapshot.Revision.Number:
		return "draft"
	default:
		return "private_revision"
	}
}

func (s *Authoring) inspectBlockLifecycle(ctx context.Context, e identity.Envelope, id string, revision int64) (AuthoringBlockLifecycle, Snapshot, error) {
	blocks, err := s.blockService()
	if err != nil {
		return AuthoringBlockLifecycle{}, Snapshot{}, err
	}
	snapshot, err := blocks.repo.ReadBlock(ctx, e, id, Reference{Revision: revision}, Read)
	if err != nil {
		return AuthoringBlockLifecycle{}, Snapshot{}, err
	}
	out := AuthoringBlockLifecycle{Block: authoringBlockView(project(snapshot, time.Now()), blocks.limits).Block, PublishedAt: clone(snapshot.PublishedAt), PublicationScope: "entire_revision", AudienceEffect: "existing_authorized_readers"}
	if snapshot.Validation != nil {
		out.ValidationFresh = freshValidation(snapshot, snapshot.Validation.Evidence.ID, time.Now()) == nil
	}
	out.CanPublish = out.ValidationFresh && snapshot.Revision.Actor == e.User() && snapshot.PublishedAt == nil && !snapshot.State.Archived && snapshot.State.DraftRevision == snapshot.Revision.Number && snapshot.State.DraftState == "validated" && Require(e, id, Publish) == nil && RequireParent(e, snapshot.State.Topic, Publish, false) == nil && RequireReferences(e, Publish, snapshot.References) == nil
	return out, snapshot, nil
}

// InspectLifecycle reports authoritative current metadata only. No publication,
// rebind, validation, source query, model call or audience grant is implied.
func (s *Authoring) InspectLifecycle(ctx context.Context, e identity.Envelope, in AuthoringLifecycleRequest) (AuthoringLifecycleView, error) {
	if s == nil || ctx == nil || (in.Report == "") == (in.Block == "") {
		return AuthoringLifecycleView{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return AuthoringLifecycleView{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	out := AuthoringLifecycleView{Version: AuthoringVersion, Blocks: []AuthoringBlockLifecycle{}}
	used, err := accountLifecycleProjection(0, out, 0)
	if err != nil {
		return AuthoringLifecycleView{}, err
	}
	if in.Block != "" {
		if !identity.Identifier(in.Block) || in.Revision < 1 || in.Revision > 256 || in.Stage != "" {
			return AuthoringLifecycleView{}, ErrInvalid
		}
		block, _, err := s.inspectBlockLifecycle(ctx, e, in.Block, in.Revision)
		if err != nil {
			return AuthoringLifecycleView{}, err
		}
		used, err = accountLifecycleProjection(used, block, 0)
		if err != nil {
			return AuthoringLifecycleView{}, err
		}
		out.Blocks = append(out.Blocks, block)
	} else {
		snapshot, err := s.authoringDocumentSnapshot(ctx, e, AuthoringReadRequest{Report: in.Report, Revision: in.Revision, Stage: in.Stage})
		if err != nil {
			return AuthoringLifecycleView{}, err
		}
		d, err := ProjectStoredDocument(snapshot.Revision.Raw, "report")
		if err != nil {
			return AuthoringLifecycleView{}, err
		}
		if err := manualDocument(d); err != nil {
			return AuthoringLifecycleView{}, err
		}
		report := &DocumentView{UnavailableQueries: snapshot.UnavailableQueries, State: snapshot.State, Revision: snapshot.Revision.Number, Digest: snapshot.Revision.Digest, Private: snapshot.PublishedAt == nil, Definition: d}
		stage := documentLifecycleStage(snapshot)
		used, err = accountLifecycleProjection(used, report, len(`,"report":`))
		if err != nil {
			return AuthoringLifecycleView{}, err
		}
		used, err = accountLifecycleProjection(used, stage, len(`,"stage":`))
		if err != nil {
			return AuthoringLifecycleView{}, err
		}
		out.Report, out.Stage, out.Rejected = report, stage, snapshot.Rejected
		type key struct {
			block    string
			revision int64
		}
		seen := map[key]bool{}
		for _, canvas := range ReportCanvases(d) {
			for _, w := range canvas.Definition.Widgets {
				if w.Block == nil {
					continue
				}
				block, pin, err := s.inspectBlockLifecycle(ctx, e, w.Block.Block, w.Block.Revision)
				if err != nil {
					return AuthoringLifecycleView{}, err
				}
				if err := CheckDocumentBlockReference(e, *w.Block, pin); err != nil {
					return AuthoringLifecycleView{}, err
				}
				k := key{w.Block.Block, pin.Revision.Number}
				if !seen[k] {
					framing := 0
					if len(out.Blocks) > 0 {
						framing = 1 // Array separator; brackets are in the base envelope.
					}
					used, err = accountLifecycleProjection(used, block, framing)
					if err != nil {
						return AuthoringLifecycleView{}, err
					}
					seen[k] = true
					out.Blocks = append(out.Blocks, block)
				}
			}
		}
	}
	setReportLifecycleHints(e, &out)
	// Recheck the complete encoded response, including any future additive DTO
	// fields. Never return a partial disclosure when the aggregate is too big.
	if _, err := accountLifecycleProjection(0, out, 0); err != nil {
		return AuthoringLifecycleView{}, err
	}
	if !e.Valid() {
		return AuthoringLifecycleView{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

// These are current signed presentation hints, not publication admission.
// Tool registration or the Builder label cannot stand in for native reach.
func setReportLifecycleHints(e identity.Envelope, out *AuthoringLifecycleView) {
	out.CanReview, out.CanPublish, out.CanReject = false, false, false
	if out.Report == nil || out.Report.State.Archived || out.Report.State.Version >= 4096 {
		return
	}
	r := out.Report
	if !out.Rejected && out.Stage == "draft" && r.State.DraftRevision == r.Revision && r.State.ReviewRevision == 0 && RequireDocument(e, "report", r.State.ID, Write) == nil {
		out.CanReview = true
		for _, canvas := range ReportCanvases(r.Definition) {
			for _, widget := range canvas.Definition.Widgets {
				if widget.Block != nil && widget.Block.Policy == "private_preview" {
					out.CanReview = false
				}
			}
		}
	}
	if out.Stage == "review" && r.State.ReviewRevision == r.Revision && RequireDocument(e, "report", r.State.ID, Publish) == nil {
		out.CanPublish = true
		out.CanReject = true
	}
}

// AuthoringBlockPublishRequest confirms the whole immutable block revision,
// not one widget/output. Evidence must be its current fresh native receipt.
type AuthoringBlockPublishRequest struct {
	Block           string `json:"block"`
	ExpectedVersion int64  `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64  `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string `json:"digest"`
	Evidence        string `json:"evidence"`
}

// PublishBlock calls native publication once. It does not rebind or publish a
// report. An uncertain response must be inspected rather than automatically
// replayed; a successful exposure cannot be rolled back by a failed rebind.
func (s *Authoring) PublishBlock(ctx context.Context, e identity.Envelope, in AuthoringBlockPublishRequest) (State, error) {
	if s == nil || ctx == nil || !identity.Identifier(in.Block) || in.ExpectedVersion < 1 || in.Revision < 1 || in.Revision > 256 || !hashValid(in.Digest) || !identity.Identifier(in.Evidence) {
		return State{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return State{}, err
	}
	for _, a := range []Access{Read, Publish} {
		if err := Require(e, in.Block, a); err != nil {
			return State{}, err
		}
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	blocks, err := s.blockService()
	if err != nil {
		return State{}, err
	}
	base, err := blocks.repo.ReadBlock(ctx, e, in.Block, Reference{Revision: in.Revision}, Read)
	if err != nil {
		return State{}, err
	}
	if base.Revision.Actor != e.User() {
		return State{}, access.ErrNotFound
	}
	if base.State.Version != in.ExpectedVersion || base.Revision.Number != in.Revision || base.Revision.Digest != in.Digest || base.State.DraftRevision != in.Revision || base.PublishedAt != nil || base.State.Archived {
		return State{}, store.ErrConflict
	}
	return blocks.Publish(ctx, e, in.Block, PublishRequest{ExpectedVersion: in.ExpectedVersion, Evidence: in.Evidence})
}

// AuthoringPublishedWidget identifies an existing widget and its exact private
// block pin. It supplies no replacement output/filter/layout or other content.
type AuthoringPublishedWidget struct {
	Widget   string `json:"widget"`
	Block    string `json:"block"`
	Revision int64  `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest   string `json:"digest"`
}

type AuthoringRebindPublishedRequest struct {
	Report          string                     `json:"report"`
	ExpectedVersion int64                      `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64                      `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Digest          string                     `json:"digest"`
	Widgets         []AuthoringPublishedWidget `json:"widgets" jsonschema:"minItems=1,maxItems=100"`
}

// RebindPublished is an explicit CAS edit, separate from block publication.
// Only selected policies and their private digest markers change. Every selected
// revision must already be published; all retained private previews stay private.
func (s *Authoring) RebindPublished(ctx context.Context, e identity.Envelope, in AuthoringRebindPublishedRequest) (DocumentState, error) {
	if s == nil || ctx == nil || in.ExpectedVersion < 1 || in.Revision < 1 || in.Revision > 256 || !hashValid(in.Digest) || len(in.Widgets) < 1 || len(in.Widgets) > 100 {
		return DocumentState{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return DocumentState{}, err
	}
	selected := map[string]AuthoringPublishedWidget{}
	for _, w := range in.Widgets {
		if !identity.Identifier(w.Widget) || !identity.Identifier(w.Block) || w.Revision < 1 || w.Revision > 256 || !hashValid(w.Digest) {
			return DocumentState{}, ErrInvalid
		}
		if _, exists := selected[w.Widget]; exists {
			return DocumentState{}, ErrInvalid
		}
		selected[w.Widget] = w
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	base, err := s.authoringDocumentSnapshot(ctx, e, AuthoringReadRequest{Report: in.Report, Revision: in.Revision})
	if err != nil {
		return DocumentState{}, err
	}
	if base.State.Version != in.ExpectedVersion || base.Revision.Digest != in.Digest || base.State.DraftRevision != in.Revision || base.PublishedAt != nil || base.State.Archived {
		return DocumentState{}, store.ErrConflict
	}
	d, err := ProjectStoredDocument(base.Revision.Raw, "report")
	if err != nil {
		return DocumentState{}, err
	}
	if err := manualDocument(d); err != nil {
		return DocumentState{}, err
	}
	// Private block references exist only in v3. Refuse legacy projections
	// rather than silently rewriting other retained content during this edit.
	if d.SchemaVersion != PagedDocumentVersion {
		return DocumentState{}, ErrInvalid
	}
	blocks, err := s.blockService()
	if err != nil {
		return DocumentState{}, err
	}
	for p := range d.ReportPages {
		for i := range d.ReportPages[p].Widgets {
			w := &d.ReportPages[p].Widgets[i]
			pin, chosen := selected[w.ID]
			if !chosen {
				continue
			}
			if w.Kind != "block" || w.Block == nil || w.Block.Policy != "private_preview" || w.Block.Block != pin.Block || w.Block.Revision != pin.Revision || w.Block.Digest != pin.Digest {
				return DocumentState{}, store.ErrConflict
			}
			published, err := blocks.repo.ReadBlock(ctx, e, pin.Block, Reference{Revision: pin.Revision}, Read)
			if err != nil {
				return DocumentState{}, err
			}
			if err := CheckDocumentBlockReference(e, *w.Block, published); err != nil {
				return DocumentState{}, err
			}
			if published.PublishedAt == nil || published.Revision.Digest != pin.Digest {
				return DocumentState{}, ErrStale
			}
			w.Block.Policy, w.Block.Digest = "published", ""
			delete(selected, w.ID)
		}
	}
	if len(selected) != 0 {
		return DocumentState{}, access.ErrNotFound
	}
	// Reuse native revision validation and the ordinary prepared edit/CAS
	// commit, but preserve stored values exactly. Generic Edit normalizes empty
	// defaults, which could change otherwise valid imported v3 definitions.
	r, err := s.documents.revision(ctx, e, "report", d, nil, base.State.LatestRevision+1, nil)
	if err != nil {
		return DocumentState{}, err
	}
	return s.documents.write(ctx, e, DocumentMutation{Kind: "report", ID: in.Report, Operation: "edit", ExpectedVersion: in.ExpectedVersion, TargetRevision: in.Revision, Revision: &r})
}

// AuthoringReportTransitionRequest adds only the manual target and a closed
// operation to the existing native expected_version/revision/note transition.
type AuthoringReportTransitionRequest struct {
	Report          string `json:"report"`
	ExpectedVersion int64  `json:"expected_version" jsonschema:"minimum=1"`
	Revision        int64  `json:"revision" jsonschema:"minimum=1,maximum=256"`
	Operation       string `json:"operation" jsonschema:"enum=review,enum=publish,enum=reject"`
	Note            string `json:"note"`
}

// TransitionReport preserves the independent native transitions. In particular,
// rejection requires publication authority and a note; it is not withdrawal.
func (s *Authoring) TransitionReport(ctx context.Context, e identity.Envelope, in AuthoringReportTransitionRequest) (DocumentState, error) {
	if s == nil || ctx == nil || in.ExpectedVersion < 1 || in.Revision < 1 || in.Revision > 256 || !slices.Contains([]string{"review", "publish", "reject"}, in.Operation) || !text(in.Note, 2048) || in.Operation == "reject" && in.Note == "" {
		return DocumentState{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return DocumentState{}, err
	}
	a := documentMutationAccess(in.Operation)
	if err := RequireDocument(e, "report", in.Report, a); err != nil {
		return DocumentState{}, err
	}
	if s.documents == nil {
		return DocumentState{}, ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	base, err := s.documents.repo.ReadDocument(ctx, e, "report", in.Report, DocumentReference{Revision: in.Revision}, a, false)
	if err != nil {
		return DocumentState{}, err
	}
	if base.State.Version != in.ExpectedVersion || base.State.Archived {
		return DocumentState{}, store.ErrConflict
	}
	d, err := ProjectStoredDocument(base.Revision.Raw, "report")
	if err != nil {
		return DocumentState{}, err
	}
	if err := manualDocument(d); err != nil {
		return DocumentState{}, err
	}
	return s.documents.Transition(ctx, e, "report", in.Report, in.ExpectedVersion, in.Revision, in.Operation, in.Note)
}
