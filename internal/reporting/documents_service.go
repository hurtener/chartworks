package reporting

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// DocumentQueryCatalog checks dynamic references without executing a question.
// The production adapter belongs to the existing NLQ service, not a second
// query engine or a source-session impersonation mechanism.
type DocumentQueryCatalog interface {
	InspectDocumentQuery(context.Context, identity.Envelope, QueryWidget) (QueryOrigin, error)
}

// Documents owns report/dashboard authoring. Execution is a separate consumer
// of these exact revisions and the existing block/query services.
type Documents struct {
	repo    DocumentRepository
	blocks  *Service
	queries DocumentQueryCatalog
	limits  config.Reporting
}

// NewDocuments performs no metadata, warehouse or model work. Text-only
// documents remain available when optional query execution is disabled.
func NewDocuments(repo DocumentRepository, blocks *Service, queries DocumentQueryCatalog, limits config.Reporting) (*Documents, error) {
	if nilValue(repo) || limits.Validate() != nil {
		return nil, ErrInvalid
	}
	if nilValue(queries) {
		queries = nil
	}
	return &Documents{repo: repo, blocks: blocks, queries: queries, limits: limits}, nil
}

// ProjectStoredDocument applies hard format bounds, not mutable operator limits.
// Lowering an authoring ceiling cannot invalidate already retained metadata.
func ProjectStoredDocument(raw json.RawMessage, kind string) (DocumentDefinition, error) {
	limits := config.DefaultReportingComposition()
	limits.MaxTextBytes, limits.MaxDefinitionBytes = 128<<10, 2<<20
	return ProjectDocument(raw, kind, limits)
}

func normalizeDocument(d DocumentDefinition, limits config.ReportingComposition) DocumentDefinition {
	d = clone(d)
	if d.Locale == "" && len(d.Metadata) > 0 {
		d.Locale = d.Metadata[0].Locale
	}
	if d.Timezone == "" {
		d.Timezone = "UTC"
	}
	if d.PartialFailure == "" {
		d.PartialFailure = limits.PartialFailure
	}
	return d
}

func (s *Documents) begin(ctx context.Context, e identity.Envelope, kind, id string, a Access) (context.Context, context.CancelFunc, error) {
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
	}
	if err := RequireDocument(e, kind, id, a); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	if err := ctx.Err(); err != nil {
		cancel()
		return nil, nil, err
	}
	return ctx, cancel, nil
}

func (s *Documents) checkReferences(ctx context.Context, e identity.Envelope, kind string, d DocumentDefinition) ([]QueryOrigin, error) {
	origins := []QueryOrigin{}
	parameters := map[string]bool{}
	for _, page := range d.Pages {
		view, err := s.repo.ReadDocument(ctx, e, "report", page.Report, DocumentReference{Revision: page.Revision}, Read, false)
		if err != nil {
			return nil, err
		}
		if kind != "dashboard" || view.PublishedAt == nil || view.State.Archived {
			return nil, ErrStale
		}
	}
	for _, w := range d.Widgets {
		switch w.Kind {
		case "block":
			if s.blocks == nil {
				return nil, ErrUnavailable
			}
			snapshot, err := s.blocks.repo.ReadBlock(ctx, e, w.Block.Block, Reference{Revision: w.Block.Revision}, Read)
			if err != nil {
				return nil, err
			}
			if snapshot.PublishedAt == nil || snapshot.State.Archived {
				return nil, ErrStale
			}
			if _, err := SelectOutputs(snapshot.Revision.Definition.Outputs, w.Block.Outputs); err != nil {
				return nil, err
			}
			if err := ValidateWidgetBindings(snapshot.Revision.Definition.Parameters, d, w); err != nil {
				return nil, err
			}
			for _, p := range snapshot.Revision.Definition.Parameters {
				parameters[p.Name] = true
			}
		case "query":
			if s.queries == nil {
				return nil, ErrUnavailable
			}
			origin, err := s.queries.InspectDocumentQuery(ctx, e, *w.Query)
			if err != nil {
				return nil, err
			}
			origin.Widget = w.ID
			if !validQueryOrigin(origin, *w.Query) {
				return nil, ErrInvalid
			}
			origins = append(origins, clone(origin))
		}
	}
	for _, a := range d.Defaults {
		if !parameters[a.Name] {
			return nil, ErrInvalid
		}
	}
	return origins, nil
}

func validQueryOrigin(origin QueryOrigin, q QueryWidget) bool {
	if !identity.Identifier(origin.Source) || origin.Context != q.Context || digest(origin.Topics) != digest(q.Topics) || !hashValid(origin.SemanticDigest) || !hashValid(origin.QueryDigest) {
		return false
	}
	if q.Durability == "replayable" {
		return origin.Query == "" && origin.Actor == "" && origin.Session == ""
	}
	return origin.Query == q.Query && identity.Identifier(origin.Actor) && identity.Identifier(origin.Session)
}

func (s *Documents) write(ctx context.Context, e identity.Envelope, m DocumentMutation) (DocumentState, error) {
	proof, err := prepareDocument(e, m, s.limits)
	if err != nil {
		return DocumentState{}, err
	}
	state, err := s.repo.CommitDocument(ctx, e, proof)
	if err != nil {
		return DocumentState{}, err
	}
	if !e.Valid() {
		return DocumentState{}, access.ErrUnauthenticated
	}
	return state, ctx.Err()
}

func (s *Documents) revision(ctx context.Context, e identity.Envelope, kind string, d DocumentDefinition, raw json.RawMessage, number int64, external *ExternalReference) (DocumentRevision, error) {
	if ValidateDocument(kind, d, s.limits.Composition, false) != nil || !validExternal(external) {
		return DocumentRevision{}, ErrInvalid
	}
	origins, err := s.checkReferences(ctx, e, kind, d)
	if err != nil {
		return DocumentRevision{}, err
	}
	if len(raw) == 0 {
		raw, err = json.Marshal(d)
		if err != nil {
			return DocumentRevision{}, ErrInvalid
		}
	}
	return DocumentRevision{Number: number, Raw: append(json.RawMessage(nil), raw...), Digest: DocumentDigest(raw), Actor: e.User(), Session: e.Session(), Created: time.Now().UTC(), Origins: origins, External: clone(external)}, nil
}

// Create creates a private draft under explicit resource-write authority.
func (s *Documents) Create(ctx context.Context, e identity.Envelope, kind, id string, definition DocumentDefinition) (DocumentState, error) {
	ctx, cancel, err := s.begin(ctx, e, kind, id, Write)
	if err != nil {
		return DocumentState{}, err
	}
	defer cancel()
	d := normalizeDocument(definition, s.limits.Composition)
	r, err := s.revision(ctx, e, kind, d, nil, 1, nil)
	if err != nil {
		return DocumentState{}, err
	}
	return s.write(ctx, e, DocumentMutation{Kind: kind, ID: id, Operation: "create", Revision: &r})
}

// Edit creates a new immutable draft. Editing while a revision is in review
// does not replace that review pointer or alter the published revision.
func (s *Documents) Edit(ctx context.Context, e identity.Envelope, kind, id string, expected int64, from DocumentReference, definition DocumentDefinition) (DocumentState, error) {
	ctx, cancel, err := s.begin(ctx, e, kind, id, Write)
	if err != nil {
		return DocumentState{}, err
	}
	defer cancel()
	base, err := s.repo.ReadDocument(ctx, e, kind, id, from, Write, false)
	if err != nil {
		return DocumentState{}, err
	}
	if expected < 1 || base.State.Version != expected || base.State.Archived {
		return DocumentState{}, store.ErrConflict
	}
	d := normalizeDocument(definition, s.limits.Composition)
	r, err := s.revision(ctx, e, kind, d, nil, base.State.LatestRevision+1, nil)
	if err != nil {
		return DocumentState{}, err
	}
	return s.write(ctx, e, DocumentMutation{Kind: kind, ID: id, Operation: "edit", ExpectedVersion: expected, TargetRevision: base.Revision.Number, Revision: &r})
}

// Transition supports review, publish, reject and archive with exact CAS.
// Publication is a lifecycle event, not certification of any dynamic SQL.
func (s *Documents) Transition(ctx context.Context, e identity.Envelope, kind, id string, expected, revision int64, operation, note string) (DocumentState, error) {
	if !slices.Contains([]string{"review", "publish", "reject", "archive"}, operation) || expected < 1 || revision < 1 || !text(note, 2048) || (operation == "reject" || operation == "archive") && note == "" {
		return DocumentState{}, ErrInvalid
	}
	ctx, cancel, err := s.begin(ctx, e, kind, id, documentMutationAccess(operation))
	if err != nil {
		return DocumentState{}, err
	}
	defer cancel()
	return s.write(ctx, e, DocumentMutation{Kind: kind, ID: id, Operation: operation, ExpectedVersion: expected, TargetRevision: revision, Note: note})
}

// Read is metadata-only. Dashboard pages are redacted inside the database before
// names or definitions leave storage, including the zero-visible-page case.
func (s *Documents) Read(ctx context.Context, e identity.Envelope, kind, id string, ref DocumentReference) (DocumentView, error) {
	ctx, cancel, err := s.begin(ctx, e, kind, id, Read)
	if err != nil {
		return DocumentView{}, err
	}
	defer cancel()
	snapshot, err := s.repo.ReadDocument(ctx, e, kind, id, ref, Read, true)
	if err != nil {
		return DocumentView{}, err
	}
	d, err := ProjectStoredDocument(snapshot.Revision.Raw, kind)
	if err != nil {
		return DocumentView{}, err
	}
	state := snapshot.State
	private := snapshot.PublishedAt == nil
	if !private {
		state.DraftRevision, state.ReviewRevision = 0, 0
		state.LatestRevision = state.PublishedRevision
	}
	if !e.Valid() {
		return DocumentView{}, access.ErrUnauthenticated
	}
	return DocumentView{State: state, Revision: snapshot.Revision.Number, Digest: snapshot.Revision.Digest, Private: private, Definition: d}, ctx.Err()
}

// List reads a bounded public metadata index, never raw results or SQL.
func (s *Documents) List(ctx context.Context, e identity.Envelope, kind, after string, limit int) (DocumentList, error) {
	if s == nil || ctx == nil || !documentKind(kind) || limit < 1 || limit > 100 || after != "" && !identity.Identifier(after) {
		return DocumentList{}, ErrInvalid
	}
	return s.repo.ListDocuments(ctx, e, kind, after, limit)
}

// DocumentImportResult distinguishes accepted drafts from private quarantine.
type DocumentImportResult struct {
	State      *DocumentState `json:"state,omitempty"`
	Quarantine string         `json:"quarantine,omitempty"`
}

// Import preserves supported version-one bytes and exact external versions.
// Unknown or unsafe layouts are quarantined, never silently approximated.
func (s *Documents) Import(ctx context.Context, e identity.Envelope, kind, id string, raw json.RawMessage, external ExternalReference) (DocumentImportResult, error) {
	ctx, cancel, err := s.begin(ctx, e, kind, id, Write)
	if err != nil {
		return DocumentImportResult{}, err
	}
	defer cancel()
	if !validExternal(&external) || len(raw) == 0 || len(raw) > s.limits.Composition.MaxDefinitionBytes || !json.Valid(raw) {
		return DocumentImportResult{}, ErrInvalid
	}
	d, err := ProjectDocument(raw, kind, s.limits.Composition)
	if err != nil || ValidateDocument(kind, d, s.limits.Composition, false) != nil {
		qid, idErr := newID()
		if idErr != nil {
			return DocumentImportResult{}, idErr
		}
		proof, proofErr := prepareQuarantine(e, QuarantineRecord{ID: qid, Kind: kind, Target: id, Raw: raw, Digest: DocumentDigest(raw), Reason: "unsupported_definition", External: &external}, s.limits.Composition)
		if proofErr != nil {
			return DocumentImportResult{}, proofErr
		}
		qid, err = s.repo.QuarantineDocument(ctx, e, proof)
		return DocumentImportResult{Quarantine: qid}, err
	}
	r, err := s.revision(ctx, e, kind, d, raw, 1, &external)
	if err != nil {
		return DocumentImportResult{}, err
	}
	state, err := s.write(ctx, e, DocumentMutation{Kind: kind, ID: id, Operation: "import", Revision: &r})
	if err != nil {
		return DocumentImportResult{}, err
	}
	return DocumentImportResult{State: &state}, nil
}
