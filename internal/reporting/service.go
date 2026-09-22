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
	rules     RuleReader
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
func New(repo Repository, topics TopicReader, sources SourceReader, validator Validator, executor Executor, capture QueryCapture, limits config.Reporting, ruleReaders ...RuleReader) (*Service, error) {
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
	var rules RuleReader
	if len(ruleReaders) > 1 || len(ruleReaders) == 1 && nilValue(ruleReaders[0]) {
		return nil, ErrInvalid
	}
	if len(ruleReaders) == 1 {
		rules = ruleReaders[0]
	}
	return &Service{repo: repo, topics: topics, rules: rules, sources: sources, validator: validator, executor: executor, capture: capture, limits: limits, slots: make(chan struct{}, limits.MaxConcurrent)}, nil
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
	out := View{State: publicState(snapshot.State, private), Revision: r.Number, RevisionID: r.ID, Digest: r.Digest, ExecutionDigest: r.ExecutionDigest, Metadata: clone(d.Metadata), Source: d.Source, Context: d.Context, Topics: clone(d.Topics), Rules: clone(d.Rules), Parameters: clone(d.Parameters), ExpectedSchema: clone(d.ExpectedSchema), Outputs: OutputDefinitions(d), Actor: r.Actor, CreatedAt: r.CreatedAt, Private: private, Trust: trust}
	out.SchemaVersion = d.SchemaVersion
	out.QueryLimits = clone(d.QueryLimits)
	out.ResultPolicy = ResolveResultPolicy(d, nil, nil)
	if snapshot.Validation != nil {
		out.Evidence = clone(&snapshot.Validation.Evidence)
		out.ResultPolicy = ResolveResultPolicy(d, snapshot.Validation.Dependencies, snapshot.Validation.Definitions)
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
	return SQLView{Definition: clone(&snapshot.Revision.Definition), ID: id, Revision: snapshot.Revision.Number, Digest: snapshot.Revision.Digest, SQL: snapshot.Revision.Definition.SQL, Provenance: clone(snapshot.Revision.Provenance)}, nil
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
	if _, err = s.resolveRules(ctx, e, in.Definition, false); err != nil {
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
	captured := (in.Definition.Template != nil || len(in.Definition.Templates) > 0) &&
		digest(in.Definition.Template) == digest(base.Revision.Definition.Template) &&
		digest(in.Definition.Templates) == digest(base.Revision.Definition.Templates) &&
		digest(in.Definition.Rules) == digest(base.Revision.Definition.Rules) &&
		in.Definition.SQL == base.Revision.Definition.SQL
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
	if _, err = s.resolveRules(ctx, e, in.Definition, false); err != nil {
		return View{}, err
	}
	provenance := clone(base.Revision.Provenance)
	provenance.Kind, provenance.ParentRevision = "amendment", base.Revision.Number
	if !captured {
		provenance.Template = nil
		provenance.Templates = nil
	}
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
	d := Definition{SchemaVersion: SchemaVersion, Metadata: clone(in.Metadata), Source: captured.Source, Context: captured.Context, Topics: clone(captured.Topics), Template: clone(captured.Template), Templates: clone(captured.Templates), SQL: captured.SQL, Parameters: parameters, ExpectedSchema: clone(captured.Schema), Outputs: clone(in.Outputs)}
	if len(captured.Rules) > 0 {
		d, err = MigrateDefinition(d)
		if err != nil {
			return View{}, err
		}
		d.Rules = clone(captured.Rules)
	}
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
	if _, err = s.resolveRules(ctx, e, d, false); err != nil {
		return View{}, err
	}
	r, err := s.newRevision(e, 1, d, Provenance{Kind: "query_capture", Query: in.Query, OriginalQuestion: captured.Question, Template: clone(captured.Template), Templates: clone(captured.Templates)})
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
	if !locale(in.Locale) || strings.TrimSpace(in.Question) == "" || !text(in.Question, 2048) || normalizeQuestion(in.Question) == "" || !validQuestionIntent(in.Intent) {
		return Assessment{}, ErrInvalid
	}
	page, err := s.List(ctx, e, ListRequest{Limit: 100, IncludeDrafts: in.IncludeDrafts})
	if err != nil {
		return Assessment{}, err
	}
	authorityDigest := assessmentAuthorityDigest(e)
	out := Assessment{Matches: []QuestionMatch{}, Complete: page.Next == "", Threshold: s.limits.QuestionThreshold, AuthorityDigest: authorityDigest, Method: "lexical_fallback"}
	scope := []any{}
	semantic, fallback := false, false
	for _, item := range page.Items {
		for _, localized := range item.Metadata {
			if localized.Locale != in.Locale {
				continue
			}
			scope = append(scope, []any{item.State.ID, item.Revision, localized.Locale, digest(localized)})
			for _, question := range append([]string{localized.Question}, localized.Aliases...) {
				if in.Intent != nil && localized.Intent != nil {
					kind, score, evidence := semanticQuestionMatch(*in.Intent, *localized.Intent)
					out.Matches = append(out.Matches, QuestionMatch{ID: item.State.ID, Revision: item.Revision, Question: question, Kind: kind, Score: score, Method: "reviewed_intent", Evidence: evidence})
					semantic = true
					continue
				}
				score := questionScore(in.Question, question)
				if score < s.limits.QuestionThreshold {
					continue
				}
				kind := "similar"
				if score == 1 && normalizeQuestion(question) == normalizeQuestion(in.Question) {
					kind = "exact"
				}
				evidence := digest(struct {
					Version, Left, Right, Kind string
					Score                      float64
				}{"question-lexical-fallback-v1", normalizeQuestion(in.Question), normalizeQuestion(question), kind, score})
				out.Matches = append(out.Matches, QuestionMatch{ID: item.State.ID, Revision: item.Revision, Question: question, Kind: kind, Score: score, Method: "lexical_fallback", Evidence: evidence})
				fallback = true
			}
		}
	}
	if semantic && fallback {
		out.Method = "reviewed_intent_with_fallback"
	} else if semantic {
		out.Method = "reviewed_intent"
	}
	out.CandidateScopeDigest = digest(struct {
		Version    string
		Candidates []any
	}{"question-candidate-scope-v1", scope})
	sort.SliceStable(out.Matches, func(i, j int) bool {
		if out.Matches[i].Score != out.Matches[j].Score {
			return out.Matches[i].Score > out.Matches[j].Score
		}
		if out.Matches[i].ID != out.Matches[j].ID {
			return out.Matches[i].ID < out.Matches[j].ID
		}
		return out.Matches[i].Question < out.Matches[j].Question
	})
	if len(out.Matches) > 100 {
		out.Matches, out.Complete = out.Matches[:100], false
	}
	out.EvidenceDigest = digest(struct {
		Version, Request, Scope, Authority, Method string
		Threshold                                  float64
		Complete                                   bool
		Matches                                    []QuestionMatch
	}{"question-assessment-v2", digest(in), out.CandidateScopeDigest, authorityDigest, out.Method, out.Threshold, out.Complete, out.Matches})
	out.ID = questionAssessmentID(e, authorityDigest, out.EvidenceDigest)
	if recorder, ok := s.repo.(QuestionAssessmentRepository); ok {
		record := QuestionAssessmentRecord{ID: out.ID, RequestDigest: digest(in), CandidateScopeDigest: out.CandidateScopeDigest, EvidenceDigest: out.EvidenceDigest, AuthorityDigest: authorityDigest, Threshold: out.Threshold, Method: out.Method, Matches: clone(out.Matches), Complete: out.Complete, CreatedAt: time.Now().UTC()}
		if err := recorder.RecordQuestionAssessment(ctx, e, record); err != nil {
			return Assessment{}, err
		}
	}
	return out, nil
}

func questionAssessmentID(e identity.Envelope, authorityDigest, evidenceDigest string) string {
	return digest(struct{ Version, Tenant, Actor, Session, Authority, Evidence string }{"question-assessment-id-v2", e.Tenant(), e.User(), e.Session(), authorityDigest, evidenceDigest})[:32]
}

func assessmentAuthorityDigest(e identity.Envelope) string {
	scopes := e.Scopes()
	sort.Strings(scopes)
	reach := e.Reach()
	sort.Slice(reach, func(i, j int) bool {
		if reach[i].Kind != reach[j].Kind {
			return reach[i].Kind < reach[j].Kind
		}
		if reach[i].Permission != reach[j].Permission {
			return reach[i].Permission < reach[j].Permission
		}
		return reach[i].ID < reach[j].ID
	})
	return digest(struct {
		Version string
		Scopes  []string
		Reach   []identity.Reach
	}{"question-assessment-authority-v1", scopes, reach})
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
