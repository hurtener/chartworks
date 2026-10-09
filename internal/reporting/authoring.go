package reporting

import (
	"context"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// AuthoringVersion identifies the bounded manual report editing contract.
const AuthoringVersion = "report-authoring-v1"

// Authoring is a thin manual workflow over the existing revision and execution
// domain. It has no identity, grant, profile or alternate composition store.
type Authoring struct {
	documents *Documents
	runs      *Compositions
}

func NewAuthoring(documents *Documents, runs *Compositions) (*Authoring, error) {
	if documents == nil || runs == nil {
		return nil, ErrInvalid
	}
	return &Authoring{documents: documents, runs: runs}, nil
}

// requireAuthoringEnvelope keeps this optional first-slice editing lane exact.
// Broader signed authority remains valid for the existing APIs, but is not
// silently treated as a bounded manual editing session here.
func requireAuthoringEnvelope(e identity.Envelope) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	for _, reach := range e.Reach() {
		if reach.ID == "*" {
			return access.ErrForbidden
		}
	}
	return nil
}

func manualDocument(d DocumentDefinition) error {
	for _, canvas := range ReportCanvases(d) {
		for _, w := range canvas.Definition.Widgets {
			if w.Kind == "query" || w.Query != nil || w.Block != nil && w.Block.Narrative {
				return ErrInvalid
			}
		}
	}
	return nil
}

// AuthoringCapabilities are presentation hints from current signed authority.
// Target booleans do not promise dependency access, existence or valid content;
// every operation repeats its complete checks against authoritative metadata.
type AuthoringCapabilities struct {
	Version    string `json:"version"`
	Builder    bool   `json:"builder"`
	Consumer   bool   `json:"consumer"`
	CanCreate  bool   `json:"can_create"`
	CanOpen    bool   `json:"can_open"`
	CanSave    bool   `json:"can_save"`
	CanPreview bool   `json:"can_preview"`
	CanExecute bool   `json:"can_execute"`
	CanPublish bool   `json:"can_publish"`
}

type AuthoringCapabilitiesRequest struct {
	Report string `json:"report"`
}

func (s *Authoring) Capabilities(ctx context.Context, e identity.Envelope, in AuthoringCapabilitiesRequest) (AuthoringCapabilities, error) {
	if s == nil || ctx == nil || in.Report != "" && !identity.Identifier(in.Report) {
		return AuthoringCapabilities{}, ErrInvalid
	}
	if !e.Valid() {
		return AuthoringCapabilities{}, access.ErrUnauthenticated
	}
	if !e.Has("reporting.read") {
		return AuthoringCapabilities{}, access.ErrForbidden
	}
	if err := ctx.Err(); err != nil {
		return AuthoringCapabilities{}, err
	}
	out := AuthoringCapabilities{Version: AuthoringVersion}
	if requireAuthoringEnvelope(e) != nil {
		return out, nil
	}
	_, err := access.Constrain(e, "reporting.write", "report", "write")
	out.Builder = err == nil
	_, publishErr := access.Constrain(e, "reporting.publish", "report", "publish")
	out.CanPublish = publishErr == nil
	out.Builder = out.Builder || out.CanPublish
	for _, kind := range []string{"block", "report", "dashboard", "run"} {
		if _, err := access.Constrain(e, "reporting.read", kind, "read"); err == nil {
			out.Consumer = true
		}
	}
	if in.Report != "" {
		out.CanExecute = RequireDocument(e, "report", in.Report, Execute) == nil
		out.CanSave = RequireDocument(e, "report", in.Report, Write) == nil
		out.CanCreate = out.CanSave && access.Require(e, "reporting.write", access.Tenant(e, "write")) == nil
		out.CanPublish = RequireDocument(e, "report", in.Report, Publish) == nil
		out.CanOpen = (out.CanSave || out.CanPublish) && RequireDocument(e, "report", in.Report, Read) == nil && RequireDocument(e, "report", in.Report, Preview) == nil
		out.CanPreview = out.CanSave && out.CanOpen && out.CanExecute
	}
	return out, nil
}

// AuthoringReportIDs intersects exact read reach with independent write or
// publication reach for the private worklist. It constructs no new authority;
// stores apply these IDs and complete dependency/custody eligibility before
// reading metadata or paginating.
func AuthoringReportIDs(e identity.Envelope) ([]string, error) {
	if err := requireAuthoringEnvelope(e); err != nil {
		return nil, err
	}
	read, err := access.Constrain(e, "reporting.read", "report", "read")
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, permission := range []string{"write", "publish"} {
		selected, err := access.Constrain(e, "reporting."+permission, "report", permission)
		if err != nil {
			continue
		}
		for _, id := range selected.IDs() {
			if slices.Contains(read.IDs(), id) {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil, access.ErrForbidden
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

// DraftSummary exposes only bounded metadata and revision/CAS coordinates.
type DraftSummary struct {
	Stage          string             `json:"stage" jsonschema:"enum=draft,enum=review"`
	DraftRevision  int64              `json:"draft_revision"`
	ReviewRevision int64              `json:"review_revision"`
	ID             string             `json:"id"`
	Version        int64              `json:"version"`
	Revision       int64              `json:"revision"`
	Metadata       []DocumentMetadata `json:"metadata"`
	Updated        time.Time          `json:"updated_at"`
}

type DraftList struct {
	Items []DraftSummary `json:"items"`
	Next  string         `json:"next,omitempty"`
}

type DraftListRequest struct {
	After string `json:"after"`
	Limit int    `json:"limit"`
}

// DocumentDraftRepository is separate from the published catalog. Storage must
// apply tenant, read plus write/publish selection and dependency reach before metadata projection.
type DocumentDraftRepository interface {
	ListDocumentDrafts(context.Context, identity.Envelope, string, int) (DraftList, error)
}

func (s *Authoring) Drafts(ctx context.Context, e identity.Envelope, in DraftListRequest) (DraftList, error) {
	if s == nil || ctx == nil || in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) {
		return DraftList{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return DraftList{}, err
	}
	if _, err := AuthoringReportIDs(e); err != nil {
		return DraftList{}, err
	}
	repo, ok := s.documents.repo.(DocumentDraftRepository)
	if !ok {
		return DraftList{}, ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	out, err := repo.ListDocumentDrafts(ctx, e, in.After, in.Limit)
	if err != nil {
		return DraftList{}, err
	}
	if !e.Valid() {
		return DraftList{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

type AuthoringReadRequest struct {
	Stage    string `json:"stage,omitempty" jsonschema:"enum=draft,enum=review"`
	Report   string `json:"report"`
	Revision int64  `json:"revision"`
}

func (s *Authoring) Read(ctx context.Context, e identity.Envelope, in AuthoringReadRequest) (DocumentView, error) {
	if s == nil || ctx == nil {
		return DocumentView{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return DocumentView{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	snapshot, err := s.authoringDocumentSnapshot(ctx, e, in)
	if err != nil {
		return DocumentView{}, err
	}
	d, err := ProjectStoredDocument(snapshot.Revision.Raw, "report")
	if err != nil {
		return DocumentView{}, err
	}
	if !e.Valid() {
		return DocumentView{}, access.ErrUnauthenticated
	}
	return DocumentView{UnavailableQueries: snapshot.UnavailableQueries, State: snapshot.State, Revision: snapshot.Revision.Number, Digest: snapshot.Revision.Digest, Private: snapshot.PublishedAt == nil, Definition: d}, ctx.Err()
}

type AuthoringCreateRequest struct {
	ID         string             `json:"id"`
	Definition DocumentDefinition `json:"definition"`
}

func (s *Authoring) Create(ctx context.Context, e identity.Envelope, in AuthoringCreateRequest) (DocumentState, error) {
	if s == nil || ctx == nil {
		return DocumentState{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return DocumentState{}, err
	}
	if err := manualDocument(in.Definition); err != nil {
		return DocumentState{}, err
	}
	if err := access.Require(e, "reporting.write", access.Tenant(e, "write")); err != nil {
		return DocumentState{}, err
	}
	return s.documents.Create(ctx, e, "report", in.ID, in.Definition)
}

type AuthoringSaveRequest struct {
	Report          string             `json:"report"`
	ExpectedVersion int64              `json:"expected_version"`
	Revision        int64              `json:"revision"`
	Definition      DocumentDefinition `json:"definition"`
}

func (s *Authoring) Save(ctx context.Context, e identity.Envelope, in AuthoringSaveRequest) (DocumentState, error) {
	if s == nil || ctx == nil || in.Revision < 1 || in.Revision > 256 || in.ExpectedVersion < 1 {
		return DocumentState{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return DocumentState{}, err
	}
	if err := manualDocument(in.Definition); err != nil {
		return DocumentState{}, err
	}
	return s.documents.Edit(ctx, e, "report", in.Report, in.ExpectedVersion, DocumentReference{Revision: in.Revision}, in.Definition)
}

type AuthoringPreviewRequest struct {
	Report     string      `json:"report"`
	Key        string      `json:"key"`
	Revision   int64       `json:"revision"`
	Resolution Resolution  `json:"resolution"`
	Pages      []PageInput `json:"pages"`
}

// Preview reserves an exact, immutable, private composition. Execution remains
// explicit; this call never publishes or changes definition pointers.
func (s *Authoring) Preview(ctx context.Context, e identity.Envelope, in AuthoringPreviewRequest) (CompositionView, error) {
	if s == nil || ctx == nil || in.Revision < 1 || in.Revision > 256 {
		return CompositionView{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return CompositionView{}, err
	}
	if err := RequireDocument(e, "report", in.Report, Write); err != nil {
		return CompositionView{}, err
	}
	view, err := s.Read(ctx, e, AuthoringReadRequest{Report: in.Report, Revision: in.Revision})
	if err != nil {
		return CompositionView{}, err
	}
	if err := manualDocument(view.Definition); err != nil {
		return CompositionView{}, err
	}
	return s.runs.Admit(ctx, e, "report", in.Report, CompositionRequest{Key: in.Key, Reference: DocumentReference{Revision: in.Revision}, Preview: true, Resolution: in.Resolution, Pages: in.Pages})
}

type AuthoringExecuteRequest struct {
	Run    string `json:"run"`
	Resume bool   `json:"resume"`
}

// Execute accepts only an already admitted private report composition and
// retains the existing actor/session, dependency and current authority checks.
func (s *Authoring) Execute(ctx context.Context, e identity.Envelope, in AuthoringExecuteRequest) (CompositionView, error) {
	if s == nil || ctx == nil {
		return CompositionView{}, ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return CompositionView{}, err
	}
	view, err := s.runs.Inspect(ctx, e, in.Run)
	if err != nil {
		return CompositionView{}, err
	}
	if view.Kind != "report" || !view.Private {
		return CompositionView{}, ErrInvalid
	}
	if err := RequireDocument(e, "report", view.Document, Write); err != nil {
		return CompositionView{}, err
	}
	if err := RequireDocument(e, "report", view.Document, Preview); err != nil {
		return CompositionView{}, err
	}
	definition, err := s.Read(ctx, e, AuthoringReadRequest{Report: view.Document, Revision: view.Revision})
	if err != nil {
		return CompositionView{}, err
	}
	if err := manualDocument(definition.Definition); err != nil {
		return CompositionView{}, err
	}
	return s.runs.Run(ctx, e, in.Run, in.Resume)
}

// WidgetPatch is an allowlist of presentation-only changes. There is no layout,
// whole-definition, query, output-selection, binding or permission field.
type WidgetPatch struct {
	Text         *TextWidget   `json:"text,omitempty"`
	Presentation *Presentation `json:"presentation,omitempty"`
}

type AuthoringWidgetRequest struct {
	Page            string      `json:"page,omitempty"`
	Report          string      `json:"report"`
	Widget          string      `json:"widget"`
	ExpectedVersion int64       `json:"expected_version"`
	Revision        int64       `json:"revision"`
	Patch           WidgetPatch `json:"patch"`
}

// PatchWidget treats the addressed stable widget as edit intent, not a new ACL.
// The server constructs the amendment from the authorized immutable baseline;
// callers cannot replace unselected widgets, report filters or layout. Only
// the current private draft may be patched, preventing old-baseline rollback
// of unrelated fields under a newer head version.
func (s *Authoring) PatchWidget(ctx context.Context, e identity.Envelope, in AuthoringWidgetRequest) (DocumentState, error) {
	if s == nil || ctx == nil || !identity.Identifier(in.Widget) || in.Revision < 1 || in.Revision > 256 || in.ExpectedVersion < 1 || in.Patch.Text == nil && in.Patch.Presentation == nil {
		return DocumentState{}, ErrInvalid
	}
	view, err := s.Read(ctx, e, AuthoringReadRequest{Report: in.Report, Revision: in.Revision})
	if err != nil {
		return DocumentState{}, err
	}
	if view.State.Version != in.ExpectedVersion || !view.Private || view.State.DraftRevision != in.Revision {
		return DocumentState{}, store.ErrConflict
	}
	d := clone(view.Definition)
	if err := manualDocument(d); err != nil {
		return DocumentState{}, err
	}
	if _, err := SelectReportCanvas(d, in.Page); err != nil {
		return DocumentState{}, err
	}
	widgets := d.Widgets
	if d.SchemaVersion == PagedDocumentVersion {
		for i := range d.ReportPages {
			if d.ReportPages[i].ID == in.Page {
				widgets = d.ReportPages[i].Widgets
				break
			}
		}
	}
	found := false
	for i := range widgets {
		w := &widgets[i]
		if w.ID != in.Widget {
			continue
		}
		found = true
		if in.Patch.Text != nil {
			if w.Kind != "text" {
				return DocumentState{}, ErrInvalid
			}
			w.Text = clone(in.Patch.Text)
		}
		if in.Patch.Presentation != nil {
			w.Presentation = clone(*in.Patch.Presentation)
		}
	}
	if !found {
		return DocumentState{}, access.ErrNotFound
	}
	return s.Save(ctx, e, AuthoringSaveRequest{Report: in.Report, ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Definition: d})
}
