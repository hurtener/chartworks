package nlqexec

import (
	"context"
	"log/slog"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

const ExampleRequalificationPolicy = "current-semantic-example-review-v1"

// ExampleRequalification preserves the exact earlier review origin. It is
// metadata, not an execution capability or inherited activation approval.
type ExampleRequalification struct {
	Policy         string `json:"policy"`
	ExampleID      string `json:"example_id"`
	Version        int64  `json:"version"`
	Digest         string `json:"digest"`
	OriginDigest   string `json:"origin_digest"`
	ContractDigest string `json:"contract_digest"`
}

func (r *ExampleRequalification) Valid() bool {
	return r == nil || r.Policy == ExampleRequalificationPolicy && identity.Identifier(r.ExampleID) && r.Version > 0 && topics.DigestValid(r.Digest) && topics.DigestValid(r.OriginDigest) && topics.DigestValid(r.ContractDigest)
}

type ExampleRequalificationRequest struct {
	ExampleID       string          `json:"example_id"`
	ExpectedVersion int64           `json:"expected_version"`
	Anchor          QuestionRequest `json:"anchor"`
}

func (ExampleRequalificationRequest) String() string         { return "example-requalification(redacted)" }
func (r ExampleRequalificationRequest) GoString() string     { return r.String() }
func (r ExampleRequalificationRequest) LogValue() slog.Value { return slog.StringValue(r.String()) }

type ExampleRequalificationRepository interface {
	RequalifyExample(context.Context, store.Scope, ExampleRecord) (ExampleRecord, error)
}

// RequalifyExample creates a separately reviewed candidate after an explicit
// current intent anchor. The original row, counts and activation are untouched.
// Native executability alone is insufficient: current semantic proof must pass.
func (s *Service) RequalifyExample(ctx context.Context, e identity.Envelope, in ExampleRequalificationRequest) (ExampleRecord, error) {
	return s.requalifyExample(ctx, e, in, nil)
}

func (s *Service) requalifyExample(ctx context.Context, e identity.Envelope, in ExampleRequalificationRequest, want *PortableExample) (ExampleRecord, error) {
	canonicalizeQuestion(&in.Anchor)
	if ctx == nil || !e.Valid() || !identity.Identifier(in.ExampleID) || in.ExpectedVersion < 1 {
		return ExampleRecord{}, ErrInvalid
	}
	if !e.Has("feedback.write") || !canInspect(e) {
		return ExampleRecord{}, access.ErrForbidden
	}
	if err := requireQuestionAction(e, "query.plan", in.Anchor); err != nil {
		return ExampleRecord{}, err
	}
	if err := validateQuestion(in.Anchor); err != nil {
		return ExampleRecord{}, err
	}
	if _, err := s.observeParent(ctx, e, in.Anchor.ClarificationQuery, in.Anchor.Context); err != nil {
		return ExampleRecord{}, err
	}
	if err := s.validateClarificationOrigin(ctx, e, in.Anchor, "query.plan"); err != nil {
		return ExampleRecord{}, err
	}
	repo, ok := s.repo.(ExampleRequalificationRepository)
	if !ok {
		return ExampleRecord{}, store.ErrUnavailable
	}
	a, err := s.admit(ctx, e, in.Anchor, true)
	if err != nil {
		return ExampleRecord{}, err
	}
	if len(a.publications) != 1 || a.route.Context == nil {
		return ExampleRecord{}, exec.ErrBinding
	}
	if want != nil && !ExampleParametersValid(ExampleRecord{Topic: a.route.Topic, Question: want.Question, SQL: want.SQL, Digest: want.Digest, ParameterSchema: want.ParameterSchema, Origin: want.Origin}) {
		return ExampleRecord{}, exec.ErrBinding
	}
	old, err := s.repo.ReadExample(ctx, mustScope(e), in.ExampleID)
	if err != nil {
		return ExampleRecord{}, err
	}
	if old.Topic != a.route.Topic {
		return ExampleRecord{}, store.ErrNotFound
	}
	if old.Version != in.ExpectedVersion {
		return ExampleRecord{}, store.ErrConflict
	}
	if !ExampleParametersValid(old) || old.PositiveEvidence < 1 || old.NegativeEvidence >= old.PositiveEvidence {
		return ExampleRecord{}, store.ErrConflict
	}
	if old.Origin.Context != a.context || old.Origin.Locale != a.route.Context.Locale {
		return ExampleRecord{}, exec.ErrBinding
	}
	if old.Origin.TopicVersion == routeVersion(a.route, a.route.Topic) && old.Origin.SourceBindingDigest == exec.Hash(a.binding) && (len(old.Origin.RuleVersions)+len(a.route.RuleVersions) == 0 || exec.Hash(old.Origin.RuleVersions) == exec.Hash(a.route.RuleVersions)) && (len(old.Origin.Templates)+len(a.route.Templates) == 0 || exec.Hash(old.Origin.Templates) == exec.Hash(a.route.Templates)) {
		return ExampleRecord{}, store.ErrConflict
	}
	if !ownedExampleApplicable(old, a) {
		return ExampleRecord{}, exec.ErrBinding
	}
	parameters, err := exampleValidationParameters(old)
	if err != nil {
		return ExampleRecord{}, err
	}
	base, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: old.SQL, Parameters: parameters}, a.relationScope)
	if err != nil {
		return ExampleRecord{}, err
	}
	if err = verifyExampleParameterDomains(ctx, e, old.ParameterSchema, base, a.binding); err != nil {
		return ExampleRecord{}, err
	}
	contract, err := compileCurrentAnalytical(ctx, a)
	if err != nil {
		return ExampleRecord{}, err
	}
	if contract == nil {
		return ExampleRecord{}, exec.ErrAnalyticalUnsupported
	}
	candidate, err := bindClarificationCandidate(ctx, a, generatedCandidate{SQL: old.SQL, Parameters: parameters})
	if err != nil {
		return ExampleRecord{}, err
	}
	plan := base
	if candidate.SQL != old.SQL || exec.Hash(candidate.Parameters) != exec.Hash(parameters) {
		plan, err = s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: candidate.SQL, Parameters: candidate.Parameters}, a.relationScope)
		if err != nil {
			return ExampleRecord{}, err
		}
	}
	if _, err = exec.CheckAnalyticalPlan(ctx, plan, *contract); err != nil {
		return ExampleRecord{}, err
	}
	id, err := newID()
	if err != nil {
		return ExampleRecord{}, err
	}
	now := time.Now().UTC()
	origin := ExampleOrigin{SchemaVersion: 1, BindingPolicy: old.Origin.BindingPolicy, Context: a.context, Locale: a.route.Context.Locale, TopicVersion: routeVersion(a.route, a.route.Topic), SourceBindingDigest: exec.Hash(a.binding), RuleVersions: append([]string(nil), a.route.RuleVersions...), Templates: append([]rulesets.TemplateSelection(nil), a.route.Templates...), Requalification: &ExampleRequalification{Policy: ExampleRequalificationPolicy, ExampleID: old.ID, Version: old.Version, Digest: old.Digest, OriginDigest: exec.Hash(old.Origin), ContractDigest: exec.Hash(*contract)}}
	// Historical evidence is retained as such under its immutable parent origin;
	// the new row is always a candidate and needs a separate activation decision.
	out := ExampleRecord{ID: id, Topic: old.Topic, Question: old.Question, SQL: old.SQL, ParameterSchema: old.ParameterSchema.Clone(), Origin: origin, State: "candidate", Version: 1, PositiveEvidence: old.PositiveEvidence, NegativeEvidence: old.NegativeEvidence, EvidenceCount: old.EvidenceCount, Weight: old.Weight, Uncertainty: old.Uncertainty, EvidenceOutcome: "positive", Provenance: ExampleRequalificationPolicy, Created: now, Updated: now}
	out.Digest = originExampleDigest(out.Topic, out.Question, out.SQL, out.ParameterSchema, out.Origin)
	if want != nil && (want.Question != out.Question || want.SQL != out.SQL || exec.Hash(want.ParameterSchema) != exec.Hash(out.ParameterSchema) || want.Digest != out.Digest || exec.Hash(want.Origin) != exec.Hash(out.Origin) || want.PositiveEvidence != out.PositiveEvidence || want.NegativeEvidence != out.NegativeEvidence) {
		return ExampleRecord{}, exec.ErrBinding
	}
	stored, err := repo.RequalifyExample(ctx, mustScope(e), out)
	if err != nil {
		return ExampleRecord{}, err
	}
	return redactExample(stored, canInspect(e)), nil
}

func cloneExampleOrigin(in ExampleOrigin) ExampleOrigin {
	out := in
	out.RuleVersions = append([]string(nil), in.RuleVersions...)
	out.Templates = append([]rulesets.TemplateSelection(nil), in.Templates...)
	if in.Requalification != nil {
		copy := *in.Requalification
		out.Requalification = &copy
	}
	return out
}
