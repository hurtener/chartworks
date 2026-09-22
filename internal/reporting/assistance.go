package reporting

import (
	"context"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// ParameterizeRequest selects a parsed, half-open date range and its new typed
// period declaration. The digest/CAS prevent applying a stale suggested edit.
// Column contains exact SQL identifier components, not a SQL fragment.
type ParameterizeRequest struct {
	ExpectedVersion       int64     `json:"expected_version"`
	DefinitionDigest      string    `json:"definition_digest"`
	Column                []string  `json:"column"`
	Parameter             Parameter `json:"parameter"`
	Note                  string    `json:"note"`
	ProposalDigest        string    `json:"proposal_digest,omitempty"`
	OriginalQuestion      string    `json:"original_question,omitempty"`
	QuestionDisposition   string    `json:"question_disposition,omitempty"`
	TemplateDisposition   string    `json:"template_disposition,omitempty"`
	ParaphraseDisposition string    `json:"paraphrase_disposition,omitempty"`
}

// ParameterizationProposal is read-only authoring evidence. Unsupported
// dialects return an explicit disposition without changing any revision.
type ParameterizationProposal struct {
	Dialect          string   `json:"dialect"`
	Source           string   `json:"source"`
	Context          string   `json:"context"`
	SourceRevision   int64    `json:"source_revision"`
	BindingDigest    string   `json:"binding_digest"`
	TopicsDigest     string   `json:"topics_digest"`
	Disposition      string   `json:"disposition"`
	Reason           string   `json:"reason,omitempty"`
	DefinitionDigest string   `json:"definition_digest"`
	Column           []string `json:"column"`
	ProposalDigest   string   `json:"proposal_digest"`
}

// ParameterizationProposalRequest selects an exact draft predicate for read-only inspection.
type ParameterizationProposalRequest struct {
	DefinitionDigest string    `json:"definition_digest"`
	Column           []string  `json:"column"`
	Parameter        Parameter `json:"parameter"`
}

func parameterizationDisposition(dialect string) (string, string) {
	if dialect == "postgres" {
		return "supported", ""
	}
	return "unsupported", "dialect_ast_edit_not_available"
}

func proposalDigest(definition string, binding exec.Binding, topics []TopicPin, column []string, parameter Parameter, disposition string) string {
	return digest(struct {
		Version, Definition, Dialect, Source, Context, Binding, Topics, Disposition string
		SourceRevision                                                              int64
		Column                                                                      []string
		Parameter                                                                   Parameter
	}{"block-parameterization-proposal-v2", definition, binding.Dialect, binding.Source, binding.Context, exec.Hash(binding), digest(topics), disposition, binding.Revision, column, parameter})
}

// ProposeParameterization verifies the selected immutable draft and source
// binding. It performs no write, publication, query execution or model call.
func (s *Service) ProposeParameterization(ctx context.Context, e identity.Envelope, id string, in ParameterizationProposalRequest) (ParameterizationProposal, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Write)
	if err != nil {
		return ParameterizationProposal{}, err
	}
	defer cancel()
	if !hashValid(in.DefinitionDigest) || !parameterizationColumnValid(in.Column) || in.Parameter.Type != "relative_period" || in.Parameter.Default == nil || in.Parameter.Default.Period == nil || validateDeclarations([]Parameter{in.Parameter}, 1) != nil {
		return ParameterizationProposal{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Draft: true}, Write)
	if err != nil {
		return ParameterizationProposal{}, err
	}
	if snapshot.Revision.Digest != in.DefinitionDigest {
		return ParameterizationProposal{}, store.ErrConflict
	}
	if s.sources == nil {
		return ParameterizationProposal{}, ErrUnavailable
	}
	binding, err := s.sources.ContextBinding(ctx, e, snapshot.Revision.Definition.Source, snapshot.Revision.Definition.Context)
	if err != nil {
		return ParameterizationProposal{}, err
	}
	if !binding.Valid() || binding.Tenant != e.Tenant() || binding.Source != snapshot.Revision.Definition.Source || binding.Context != snapshot.Revision.Definition.Context {
		return ParameterizationProposal{}, ErrInvalid
	}
	disposition, reason := parameterizationDisposition(binding.Dialect)
	out := ParameterizationProposal{Dialect: binding.Dialect, Source: binding.Source, Context: binding.Context, SourceRevision: binding.Revision, BindingDigest: exec.Hash(binding), TopicsDigest: digest(snapshot.Revision.Definition.Topics), Disposition: disposition, Reason: reason, DefinitionDigest: in.DefinitionDigest, Column: clone(in.Column)}
	out.ProposalDigest = proposalDigest(in.DefinitionDigest, binding, snapshot.Revision.Definition.Topics, in.Column, in.Parameter, disposition)
	if disposition == "supported" {
		if _, err := parameterizePeriod(ctx, snapshot.Revision.Definition.SQL, in.Column, scalarSlots(snapshot.Revision.Definition.Parameters)+1); err != nil {
			return ParameterizationProposal{}, err
		}
	}
	return out, nil
}

// Parameterize appends a private draft and nothing more. It never silently
// modifies published SQL or fabricates validation for a suggested period edit.
func (s *Service) Parameterize(ctx context.Context, e identity.Envelope, id string, in ParameterizeRequest) (View, error) {
	ctx, cancel, err := s.begin(ctx, e, id, Write)
	if err != nil {
		return View{}, err
	}
	defer cancel()
	if !note(in.Note) || !hashValid(in.DefinitionDigest) || !hashValid(in.ProposalDigest) || !parameterizationColumnValid(in.Column) || in.Parameter.Type != "relative_period" || in.Parameter.Default == nil || in.Parameter.Default.Period == nil || !parameterizationIntentValid(in) {
		return View{}, ErrInvalid
	}
	snapshot, err := s.repo.ReadBlock(ctx, e, id, Reference{Draft: true}, Write)
	if err != nil {
		return View{}, err
	}
	if err := expected(snapshot, in.ExpectedVersion); err != nil {
		return View{}, err
	}
	if snapshot.State.Archived || snapshot.Revision.Digest != in.DefinitionDigest {
		return View{}, store.ErrConflict
	}
	if s.sources == nil {
		return View{}, ErrUnavailable
	}
	d := clone(snapshot.Revision.Definition)
	binding, err := s.sources.ContextBinding(ctx, e, d.Source, d.Context)
	if err != nil {
		return View{}, err
	}
	if !binding.Valid() || binding.Tenant != e.Tenant() || binding.Source != d.Source || binding.Context != d.Context || binding.Dialect != "postgres" {
		return View{}, ErrInvalid
	}
	disposition, _ := parameterizationDisposition(binding.Dialect)
	proposal := proposalDigest(in.DefinitionDigest, binding, d.Topics, in.Column, in.Parameter, disposition)
	if in.ProposalDigest != proposal {
		return View{}, store.ErrConflict
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return View{}, ctx.Err()
	default:
		return View{}, ErrBusy
	}
	d.Parameters = append(d.Parameters, clone(in.Parameter))
	if validateDeclarations(d.Parameters, s.limits.MaxParameters) != nil || scalarSlots(d.Parameters) > 64 {
		return View{}, ErrInvalid
	}
	d.SQL, err = parameterizePeriod(ctx, d.SQL, in.Column, scalarSlots(snapshot.Revision.Definition.Parameters)+1)
	if err != nil {
		return View{}, err
	}
	d.Template = nil
	d.Templates = nil
	if err := validateDefinition(ctx, d, s.limits, false); err != nil {
		return View{}, err
	}
	_, refs, err := s.resolveDefinitions(ctx, e, d, false)
	if err != nil {
		return View{}, err
	}
	provenance := clone(snapshot.Revision.Provenance)
	provenance.Kind = "parameterize"
	provenance.ParentRevision = snapshot.Revision.Number
	provenance.ChangeDigest = digest(struct {
		Before, After string
		Column        []string
		Parameter     Parameter
	}{snapshot.Revision.Digest, DefinitionDigest(d), in.Column, in.Parameter})
	provenance.Parameterization = &ParameterizationEvidence{ProposalDigest: proposal, Dialect: binding.Dialect, Source: binding.Source, Context: binding.Context, SourceRevision: binding.Revision, BindingDigest: exec.Hash(binding), TopicsDigest: digest(d.Topics), OriginalQuestion: in.OriginalQuestion, QuestionDisposition: in.QuestionDisposition, TemplateDisposition: in.TemplateDisposition, ParaphraseDisposition: in.ParaphraseDisposition}
	r, err := s.newRevision(e, snapshot.State.DraftRevision+1, d, provenance)
	if err != nil {
		return View{}, err
	}
	state, err := s.commit(ctx, e, Mutation{ID: id, Topic: snapshot.State.Topic, Kind: "parameterize", ExpectedVersion: in.ExpectedVersion, TargetRevision: snapshot.Revision.Number, TargetDigest: snapshot.Revision.Digest, Note: in.Note, Revision: &r, References: refs})
	if err != nil {
		return View{}, err
	}
	return project(Snapshot{State: state, Revision: r}, time.Now()), nil
}

func parameterizationIntentValid(in ParameterizeRequest) bool {
	if !text(in.OriginalQuestion, 2048) {
		return false
	}
	allowed := func(value string) bool {
		return value == "" || value == "preserved" || value == "revised" || value == "not_applicable"
	}
	if !allowed(in.QuestionDisposition) || !allowed(in.TemplateDisposition) || !allowed(in.ParaphraseDisposition) {
		return false
	}
	return strings.TrimSpace(in.OriginalQuestion) != "" && in.QuestionDisposition != "" && in.TemplateDisposition != "" && in.ParaphraseDisposition != ""
}

func parameterizationColumnValid(column []string) bool {
	if len(column) < 1 || len(column) > 4 {
		return false
	}
	for _, name := range column {
		if strings.TrimSpace(name) == "" || !text(name, 128) {
			return false
		}
	}
	return true
}
