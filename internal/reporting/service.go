package reporting

import (
	"context"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// Service is immutable except for bounded validation admission. All transports
// use these same lifecycle methods and the same PostgreSQL mutation proof.
type Service struct {
	repo      Repository
	topics    TopicReader
	sources   SourceReader
	validator Validator
	executor  Executor
	capture   QueryCapture
	limits    config.Reporting
	slots     chan struct{}
}

func nilValue(v any) bool {
	return v == nil || reflect.ValueOf(v).Kind() == reflect.Pointer && reflect.ValueOf(v).IsNil()
}

// New keeps metadata authoring/viewing independent of source/model availability.
// Missing execution dependencies disable only explicit validation and preview.
func New(repo Repository, topics TopicReader, sources SourceReader, validator Validator, executor Executor, capture QueryCapture, limits config.Reporting) (*Service, error) {
	if nilValue(repo) || nilValue(topics) || limits.Validate() != nil {
		return nil, ErrInvalid
	}
	if nilValue(sources) {
		sources = nil
	}
	if nilValue(validator) {
		validator = nil
	}
	if nilValue(executor) {
		executor = nil
	}
	if nilValue(capture) {
		capture = nil
	}
	return &Service{repo: repo, topics: topics, sources: sources, validator: validator, executor: executor, capture: capture, limits: limits, slots: make(chan struct{}, limits.MaxConcurrent)}, nil
}

// CanValidate reports whether the existing validator and executor are installed.
func (s *Service) CanValidate() bool {
	return s != nil && s.sources != nil && s.validator != nil && s.executor != nil
}

// CanCapture reports whether the owning query service supplies the private capture handoff.
func (s *Service) CanCapture() bool { return s != nil && s.capture != nil }

// Limits returns the configured reporting bounds by value.
func (s *Service) Limits() config.Reporting { return s.limits }

func (s *Service) begin(ctx context.Context, e identity.Envelope, id string, a Access) (context.Context, context.CancelFunc, error) {
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
	}
	if err := Require(e, id, a); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	if err := ctx.Err(); err != nil {
		cancel()
		return nil, nil, err
	}
	return ctx, cancel, nil
}

func (s *Service) commit(ctx context.Context, e identity.Envelope, m Mutation) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	p, err := prepare(e, m, s.limits)
	if err != nil {
		return State{}, err
	}
	state, err := s.repo.CommitBlock(ctx, e, p)
	if err == nil && ctx.Err() != nil {
		return State{}, ctx.Err()
	}
	return state, err
}

func expected(snapshot Snapshot, version int64) error {
	if version < 1 {
		return ErrInvalid
	}
	if snapshot.State.Version != version {
		return store.ErrConflict
	}
	return nil
}

func publicState(state State, private bool) State {
	if !private {
		state.DraftRevision, state.DraftState = 0, ""
	}
	return state
}

func project(snapshot Snapshot, now time.Time) View {
	r, d := snapshot.Revision, snapshot.Revision.Definition
	private := snapshot.PublishedAt == nil
	trust := Trust{Publication: "draft", Certification: "none", Health: snapshot.Health}
	if trust.Health.Status == "" {
		trust.Health = Health{Status: "unknown", Reason: "not_checked"}
	}
	if !private {
		trust.Publication = "published"
		if snapshot.State.PublishedRevision != r.Number {
			trust.Publication = "superseded"
		}
	}
	if snapshot.State.Archived {
		trust.Publication = "archived"
	}
	if snapshot.Attestation != nil {
		trust.HistoricalAttestation = clone(snapshot.Attestation)
		trust.Withdrawal = clone(snapshot.Withdrawal)
		trust.Certification = "valid"
		switch {
		case snapshot.Withdrawal != nil:
			trust.Certification = "withdrawn"
		case trust.Health.Status == "unavailable":
			trust.Certification = "unavailable"
		case snapshot.State.Archived || !snapshot.Current || !now.Before(snapshot.Attestation.EvidenceExpiresAt) || trust.Health.Status != "healthy" || trust.Health.DependencyDigest != snapshot.Attestation.DependencyDigest:
			trust.Certification = "stale"
		}
	}
	out := View{State: publicState(snapshot.State, private), Revision: r.Number, RevisionID: r.ID, Digest: r.Digest, ExecutionDigest: r.ExecutionDigest, Metadata: clone(d.Metadata), Source: d.Source, Context: d.Context, Topics: clone(d.Topics), Parameters: clone(d.Parameters), ExpectedSchema: clone(d.ExpectedSchema), Outputs: clone(d.Outputs), Actor: r.Actor, CreatedAt: r.CreatedAt, Private: private, Trust: trust}
	if snapshot.Validation != nil {
		out.Evidence = clone(&snapshot.Validation.Evidence)
	}
	return out
}

// Read never opens a warehouse or invokes a model, including after provider loss.
func (s *Service) Read(ctx context.Context, e identity.Envelope, id string, ref Reference) (View, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Read)
	if err != nil {
		return View{}, err
	}
	defer cancel()
	snapshot, err := s.repo.ReadBlock(ctx, e, id, ref, Read)
	if err != nil {
		return View{}, err
	}
	if err := ctx.Err(); err != nil {
		return View{}, err
	}
	return project(snapshot, time.Now()), nil
}

// SQL requires separate SQL-read authority as well as ordinary reach/private
// eligibility. It is the only metadata response that contains approved SQL.
func (s *Service) SQL(ctx context.Context, e identity.Envelope, id string, ref Reference) (SQLView, error) {
	ctx, cancel, err := s.begin(ctx, e, id, SQLRead)
	if err != nil {
		return SQLView{}, err
	}
	defer cancel()
	if err := Require(e, id, Read); err != nil {
		return SQLView{}, err
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, ref, SQLRead)
	if err != nil {
		return SQLView{}, err
	}
	if err := ctx.Err(); err != nil {
		return SQLView{}, err
	}
	return SQLView{ID: id, Revision: snapshot.Revision.Number, Digest: snapshot.Revision.Digest, SQL: snapshot.Revision.Definition.SQL, Provenance: clone(snapshot.Revision.Provenance)}, nil
}

func (s *Service) newRevision(e identity.Envelope, number int64, d Definition, provenance Provenance) (Revision, error) {
	id, err := newID()
	if err != nil {
		return Revision{}, err
	}
	return Revision{Number: number, ID: id, Definition: clone(d), Digest: DefinitionDigest(d), ExecutionDigest: ExecutionDigest(d), Actor: e.User(), CreatedAt: time.Now().UTC(), Provenance: clone(provenance)}, nil
}

// Create authors a private, unvalidated manual definition. It cannot supply a
// validation manifest, capture provenance, publication flag or certification.
func (s *Service) Create(ctx context.Context, e identity.Envelope, in CreateRequest) (View, error) {
	ctx, cancel, err := s.begin(ctx, e, in.ID, Write)
	if err != nil {
		return View{}, err
	}
	defer cancel()
	if err := validateDefinition(ctx, in.Definition, s.limits, false); err != nil {
		return View{}, err
	}
	if err := RequireParent(e, in.Definition.Topics[0].Topic, Write, true); err != nil {
		return View{}, err
	}
	_, refs, err := s.resolveDefinitions(ctx, e, in.Definition, false)
	if err != nil {
		return View{}, err
	}
	r, err := s.newRevision(e, 1, in.Definition, Provenance{Kind: "manual"})
	if err != nil {
		return View{}, err
	}
	state, err := s.commit(ctx, e, Mutation{ID: in.ID, Topic: in.Definition.Topics[0].Topic, Kind: "create", Revision: &r, References: refs})
	if err != nil {
		return View{}, err
	}
	return project(Snapshot{State: state, Revision: r}, time.Now()), nil
}

// Edit always appends a private revision, including when editing published SQL.
// Every edit invalidates validation, even when only presentation text changed.
func (s *Service) Edit(ctx context.Context, e identity.Envelope, id string, in EditRequest) (View, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Write)
	if err != nil {
		return View{}, err
	}
	defer cancel()
	base, err := s.repo.ReadBlock(ctx, e, id, Reference{Draft: true}, Write)
	if err != nil {
		return View{}, err
	}
	if err := expected(base, in.ExpectedVersion); err != nil {
		return View{}, err
	}
	if base.State.Archived {
		return View{}, store.ErrConflict
	}
	captured := in.Definition.Template != nil && base.Revision.Definition.Template != nil && digest(in.Definition.Template) == digest(base.Revision.Definition.Template) && in.Definition.SQL == base.Revision.Definition.SQL
	if err := validateDefinition(ctx, in.Definition, s.limits, captured); err != nil {
		return View{}, err
	}
	if in.Definition.Topics[0].Topic != base.State.Topic {
		return View{}, ErrInvalid
	}
	_, refs, err := s.resolveDefinitions(ctx, e, in.Definition, false)
	if err != nil {
		return View{}, err
	}
	provenance := clone(base.Revision.Provenance)
	provenance.Kind, provenance.ParentRevision = "amendment", base.Revision.Number
	r, err := s.newRevision(e, base.State.DraftRevision+1, in.Definition, provenance)
	if err != nil {
		return View{}, err
	}
	state, err := s.commit(ctx, e, Mutation{ID: id, Topic: base.State.Topic, Kind: "edit", ExpectedVersion: in.ExpectedVersion, TargetRevision: base.Revision.Number, TargetDigest: base.Revision.Digest, Revision: &r, References: refs})
	if err != nil {
		return View{}, err
	}
	return project(Snapshot{State: state, Revision: r}, time.Now()), nil
}

// CaptureQuery consumes only the existing query service's source-backed result.
// A capture is not validation or certification. Ambiguous text parameters require
// explicit typed declarations whose resolved defaults match the source exactly.
func (s *Service) CaptureQuery(ctx context.Context, e identity.Envelope, in CaptureRequest) (View, error) {
	ctx, cancel, err := s.begin(ctx, e, in.ID, Write)
	if err != nil {
		return View{}, err
	}
	defer cancel()
	if !identity.Identifier(in.Query) || !metadataValid(in.Metadata, s.limits) || len(in.Outputs) == 0 || len(in.Outputs) > s.limits.MaxOutputs {
		return View{}, ErrInvalid
	}
	if !s.CanCapture() {
		return View{}, ErrUnavailable
	}
	captured, err := s.capture.Capture(ctx, e, in.Query)
	if err != nil {
		return View{}, err
	}
	parameters := clone(in.Parameters)
	if len(parameters) == 0 {
		parameters, err = scalarDefaults(captured.Parameters)
		if err != nil {
			return View{}, err
		}
	}
	resolution := acceptedResolution(in.Resolution)
	resolved, err := ResolveParameters(parameters, nil, resolution)
	if err != nil || parameterDigest(resolved.Parameters) != parameterDigest(captured.Parameters) {
		return View{}, ErrInvalid
	}
	d := Definition{SchemaVersion: SchemaVersion, Metadata: clone(in.Metadata), Source: captured.Source, Context: captured.Context, Topics: clone(captured.Topics), Template: clone(captured.Template), SQL: captured.SQL, Parameters: parameters, ExpectedSchema: clone(captured.Schema), Outputs: clone(in.Outputs)}
	if err := validateDefinition(ctx, d, s.limits, true); err != nil {
		return View{}, err
	}
	if err := RequireParent(e, d.Topics[0].Topic, Write, true); err != nil {
		return View{}, err
	}
	_, refs, err := s.resolveDefinitions(ctx, e, d, false)
	if err != nil {
		return View{}, err
	}
	r, err := s.newRevision(e, 1, d, Provenance{Kind: "query_capture", Query: in.Query, OriginalQuestion: captured.Question, Template: clone(captured.Template)})
	if err != nil {
		return View{}, err
	}
	state, err := s.commit(ctx, e, Mutation{ID: in.ID, Topic: d.Topics[0].Topic, Kind: "capture", Revision: &r, References: refs})
	if err != nil {
		return View{}, err
	}
	return project(Snapshot{State: state, Revision: r}, time.Now()), nil
}

// List returns only blocks eligible under current signed reach and persisted privacy.
func (s *Service) List(ctx context.Context, e identity.Envelope, in ListRequest) (Page, error) {
	if s == nil || ctx == nil || in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) {
		return Page{}, ErrInvalid
	}
	if !e.Valid() {
		return Page{}, access.ErrUnauthenticated
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	out, err := s.repo.ListBlocks(ctx, e, in)
	if err != nil {
		return Page{}, err
	}
	for i := range out.Items {
		out.Items[i].State = publicState(out.Items[i].State, out.Items[i].Private)
	}
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	return out, nil
}

// History returns only lifecycle history eligible under current signed reach and persisted privacy.
func (s *Service) History(ctx context.Context, e identity.Envelope, id string) (History, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Read)
	if err != nil {
		return History{}, err
	}
	defer cancel()
	out, err := s.repo.BlockHistory(ctx, e, id)
	if err == nil && ctx.Err() != nil {
		return History{}, ctx.Err()
	}
	return out, err
}

// AssessQuestions is bounded, permission-filtered and advisory. Complete=false
// explicitly prevents a bounded candidate scan from claiming global uniqueness.
func (s *Service) AssessQuestions(ctx context.Context, e identity.Envelope, in QuestionRequest) (Assessment, error) {
	if !locale(in.Locale) || strings.TrimSpace(in.Question) == "" || !text(in.Question, 2048) || normalizeQuestion(in.Question) == "" {
		return Assessment{}, ErrInvalid
	}
	page, err := s.List(ctx, e, ListRequest{Limit: 100, IncludeDrafts: in.IncludeDrafts})
	if err != nil {
		return Assessment{}, err
	}
	out := Assessment{Matches: []QuestionMatch{}, Complete: page.Next == "", Threshold: s.limits.QuestionThreshold}
	for _, item := range page.Items {
		for _, localized := range item.Metadata {
			if localized.Locale != in.Locale {
				continue
			}
			for _, question := range append([]string{localized.Question}, localized.Aliases...) {
				score := questionScore(in.Question, question)
				if score < s.limits.QuestionThreshold {
					continue
				}
				kind := "similar"
				if score == 1 && normalizeQuestion(question) == normalizeQuestion(in.Question) {
					kind = "exact"
				}
				out.Matches = append(out.Matches, QuestionMatch{ID: item.State.ID, Revision: item.Revision, Question: question, Kind: kind, Score: score})
			}
		}
	}
	sort.SliceStable(out.Matches, func(i, j int) bool { return out.Matches[i].Score > out.Matches[j].Score })
	if len(out.Matches) > 100 {
		out.Matches, out.Complete = out.Matches[:100], false
	}
	return out, nil
}

func acceptedResolution(in Resolution) Resolution {
	out := clone(in)
	if out.At.IsZero() {
		out.At = time.Now().UTC()
	}
	if out.Timezone == "" {
		out.Timezone = "UTC"
	}
	return out
}

func revisionReference(revision int64) Reference {
	if revision == 0 {
		return Reference{Draft: true}
	}
	return Reference{Revision: revision}
}

func successful(status string) bool {
	return slices.Contains([]string{"succeeded", "empty", "truncated"}, status)
}
