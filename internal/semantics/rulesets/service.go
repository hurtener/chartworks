package rulesets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// Service coordinates reviewed rule lifecycle with exact pinned evidence.
type Service struct {
	repo     Repository
	topics   TopicRepository
	evidence EvidenceRepository
}

// New constructs a rule lifecycle service with topic, rule and optional evidence repositories.
func New(repo Repository, topicRepo TopicRepository, evidence ...EvidenceRepository) (*Service, error) {
	if repo == nil || topicRepo == nil {
		return nil, store.ErrInvalid
	}
	var evidenceRepo EvidenceRepository
	if len(evidence) > 1 {
		return nil, store.ErrInvalid
	}
	if len(evidence) == 1 {
		evidenceRepo = evidence[0]
	}
	return &Service{repo: repo, topics: topicRepo, evidence: evidenceRepo}, nil
}

const (
	comparisonReplay = "replay"
	comparisonShadow = "shadow"
)

func newComparisonID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", store.ErrUnavailable
	}
	return hex.EncodeToString(raw[:]), nil
}

func validReferences(refs []semantics.Reference) error {
	if len(refs) < 1 || len(refs) > 256 {
		return store.ErrInvalid
	}
	seen := make(map[semantics.Reference]bool, len(refs))
	for _, ref := range refs {
		if !ref.Valid() || seen[ref] {
			return store.ErrInvalid
		}
		seen[ref] = true
	}
	return nil
}

func sameConstraintEvaluation(a, b semantics.ConstraintEvaluation) bool {
	if a.Allowed != b.Allowed || len(a.Required) != len(b.Required) || len(a.Excluded) != len(b.Excluded) || len(a.Violations) != len(b.Violations) {
		return false
	}
	for i := range a.Required {
		if a.Required[i] != b.Required[i] {
			return false
		}
	}
	for i := range a.Excluded {
		if a.Excluded[i] != b.Excluded[i] {
			return false
		}
	}
	for i := range a.Violations {
		if a.Violations[i].Rule != b.Violations[i].Rule || a.Violations[i].Kind != b.Violations[i].Kind || a.Violations[i].Target != b.Violations[i].Target {
			return false
		}
	}
	return true
}

func publishedSubject(p topics.Published) (semantics.RuleSubject, error) {
	pack := semantics.TopicPack{SchemaVersion: p.Definition.SchemaVersion, Topic: p.Definition.Topic, Version: p.Definition.Version, Name: p.Definition.Name, Description: p.Definition.Description, Measures: p.Definition.Measures, Dimensions: p.Definition.Dimensions, KPIs: p.Definition.KPIs, Joins: p.Definition.Joins, CanonicalEntities: p.Definition.CanonicalEntities}
	for _, dataset := range p.Definition.Datasets {
		pack.Datasets = append(pack.Datasets, semantics.Dataset{ID: dataset.ID, Name: dataset.Name, Columns: append([]semantics.Column(nil), dataset.Columns...)})
	}
	return semantics.NewRuleSubject(pack, p.Digest)
}

// Save validates and persists a rule draft against the currently published topic.
func (s *Service) Save(ctx context.Context, e identity.Envelope, in SaveRequest) (Draft, error) {
	if ctx == nil || in.Expected < 0 || in.Expected >= 1<<62 || !textValid(in.Change) {
		return Draft{}, store.ErrInvalid
	}
	published, err := s.topics.ReadPublishedTopic(ctx, e, in.Definition.Topic, in.Definition.TopicVersion, drafts.Write)
	if err != nil {
		return Draft{}, err
	}
	if !published.State.Active {
		return Draft{}, store.ErrConflict
	}
	subject, err := publishedSubject(published)
	if err != nil {
		return Draft{}, err
	}
	model, err := semantics.CompilePublishedRules(subject, in.Definition)
	if err != nil {
		return Draft{}, err
	}
	return s.repo.SaveRuleDraft(ctx, e, published, model, in.Expected, in.Change)
}

// Review validates and records a decision for a rule draft.
func (s *Service) Review(ctx context.Context, e identity.Envelope, topic string, in ReviewRequest) (Review, error) {
	if ctx == nil || !identity.Identifier(topic) || in.DraftRevision < 1 || in.DraftRevision >= 1<<62 || !digestValid(in.Digest) || (in.Decision != "approve" && in.Decision != "reject") || !textValid(in.Note) {
		return Review{}, store.ErrInvalid
	}
	current, err := s.topics.ReadPublishedTopic(ctx, e, topic, "", drafts.Review)
	if err != nil {
		return Review{}, err
	}
	return s.repo.ReviewRuleDraft(ctx, e, current, topic, in)
}

// Publish validates and publishes an approved rule draft.
func (s *Service) Publish(ctx context.Context, e identity.Envelope, topic string, in PublishRequest) (Published, error) {
	if ctx == nil || !identity.Identifier(topic) || !identity.Identifier(in.Review) || in.Expected < 0 || in.Expected >= 1<<62 {
		return Published{}, store.ErrInvalid
	}
	current, err := s.topics.ReadPublishedTopic(ctx, e, topic, "", drafts.Publish)
	if err != nil {
		return Published{}, err
	}
	return s.repo.PublishRules(ctx, e, current, in.Review, in.Expected)
}

// Read returns a current or explicitly retained published rule version.
func (s *Service) Read(ctx context.Context, e identity.Envelope, topic, version string) (Published, error) {
	if ctx == nil || !identity.Identifier(topic) || version != "" && !identity.Identifier(version) {
		return Published{}, store.ErrInvalid
	}
	pin, err := s.repo.RuleVersionPin(ctx, e, topic, version, drafts.Read)
	if err != nil {
		return Published{}, err
	}
	topicVersion, err := s.topics.ReadPublishedTopic(ctx, e, topic, pin.TopicVersion, drafts.Read)
	if err != nil {
		return Published{}, err
	}
	if version == "" && !topicVersion.State.Active {
		return Published{}, store.ErrConflict
	}
	return s.repo.ReadPublishedRules(ctx, e, topic, pin.RuleVersion, drafts.Read, version == "")
}

// Retire removes the active rule pointer after checking the current topic pin.
func (s *Service) Retire(ctx context.Context, e identity.Envelope, topic string, in RetireRequest) (State, error) {
	if ctx == nil || !identity.Identifier(topic) || in.Expected < 1 || in.Expected >= 1<<62 || !textValid(in.Note) {
		return State{}, store.ErrInvalid
	}
	pin, err := s.repo.RuleVersionPin(ctx, e, topic, "", drafts.Publish)
	if err != nil {
		return State{}, err
	}
	current, err := s.topics.ReadPublishedTopic(ctx, e, topic, pin.TopicVersion, drafts.Publish)
	if err != nil {
		return State{}, err
	}
	return s.repo.RetireRules(ctx, e, current, in.Note, in.Expected)
}

// Evaluate returns hard-constraint evidence for the current published rule set.
func (s *Service) Evaluate(ctx context.Context, e identity.Envelope, topic string, in EvaluateRequest) (Evaluation, error) {
	published, err := s.Read(ctx, e, topic, "")
	if err != nil {
		return Evaluation{}, err
	}
	pin := published.Definition
	topicVersion, err := s.topics.ReadPublishedTopic(ctx, e, topic, pin.TopicVersion, drafts.Read)
	if err != nil {
		return Evaluation{}, err
	}
	if !topicVersion.State.Active || topicVersion.State.Version != pin.TopicVersion {
		return Evaluation{}, store.ErrConflict
	}
	subject, err := publishedSubject(topicVersion)
	if err != nil {
		return Evaluation{}, err
	}
	model, err := semantics.CompilePublishedRules(subject, pin)
	if err != nil {
		return Evaluation{}, err
	}
	result, err := semantics.EvaluateConstraints(subject, model, in.References)
	if err != nil {
		return Evaluation{}, err
	}
	return Evaluation{Topic: topic, TopicVersion: pin.TopicVersion, PackDigest: topicVersion.Digest, RuleVersion: pin.Version, RuleDigest: published.Digest, Result: result, EvaluatedAt: time.Now().UTC()}, nil
}

// readPinned performs the shared non-payload pin, exact topic read, and exact
// ruleset read ordering. Historical reads remain available after replacement;
// current=true additionally requires both pointers and the topic to be active.
func (s *Service) readPinned(ctx context.Context, e identity.Envelope, topic, topicVersion, ruleVersion string, current bool) (Evaluation, semantics.RuleSubject, semantics.RuleModel, error) {
	if ctx == nil || !identity.Identifier(topic) || ruleVersion != "" && !identity.Identifier(ruleVersion) || topicVersion != "" && !identity.Identifier(topicVersion) {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, store.ErrInvalid
	}
	pin, err := s.repo.RuleVersionPin(ctx, e, topic, ruleVersion, drafts.Read)
	if err != nil {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, err
	}
	if topicVersion != "" && pin.TopicVersion != topicVersion {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, store.ErrConflict
	}
	publishedTopic, err := s.topics.ReadPublishedTopic(ctx, e, topic, pin.TopicVersion, drafts.Read)
	if err != nil {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, err
	}
	if publishedTopic.State.Version != pin.TopicVersion || publishedTopic.Digest != pin.PackDigest || current && !publishedTopic.State.Active {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, store.ErrConflict
	}
	publishedRules, err := s.repo.ReadPublishedRules(ctx, e, topic, pin.RuleVersion, drafts.Read, current)
	if err != nil {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, err
	}
	if publishedRules.State.Version != pin.RuleVersion || publishedRules.Definition.TopicVersion != publishedTopic.State.Version || publishedRules.Definition.PackDigest != publishedTopic.Digest {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, store.ErrConflict
	}
	subject, err := publishedSubject(publishedTopic)
	if err != nil {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, err
	}
	model, err := semantics.CompilePublishedRules(subject, publishedRules.Definition)
	if err != nil {
		return Evaluation{}, semantics.RuleSubject{}, semantics.RuleModel{}, err
	}
	return Evaluation{Topic: topic, TopicVersion: pin.TopicVersion, PackDigest: pin.PackDigest, RuleVersion: pin.RuleVersion, RuleDigest: publishedRules.Digest}, subject, model, nil
}

func (s *Service) recordComparison(ctx context.Context, e identity.Envelope, comparison Comparison) (Comparison, error) {
	if s.evidence == nil {
		return Comparison{}, store.ErrUnavailable
	}
	return s.evidence.RecordComparison(ctx, e, comparison)
}

// Replay evaluates an exact retained ruleset/topic pair and persists the
// result for later query/evidence replay. It does not execute a query.
func (s *Service) Replay(ctx context.Context, e identity.Envelope, topic string, in ReplayRequest) (Comparison, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(topic) || !identity.Identifier(in.RuleVersion) || in.TopicVersion != "" && !identity.Identifier(in.TopicVersion) {
		return Comparison{}, store.ErrInvalid
	}
	if err := validReferences(in.References); err != nil {
		return Comparison{}, err
	}
	base, subject, model, err := s.readPinned(ctx, e, topic, in.TopicVersion, in.RuleVersion, false)
	if err != nil {
		return Comparison{}, err
	}
	base.Result, err = semantics.EvaluateConstraints(subject, model, in.References)
	if err != nil {
		return Comparison{}, err
	}
	base.EvaluatedAt = time.Now().UTC()
	id, err := newComparisonID()
	if err != nil {
		return Comparison{}, err
	}
	return s.recordComparison(ctx, e, Comparison{ID: id, Mode: comparisonReplay, Topic: topic, References: append([]semantics.Reference(nil), in.References...), Baseline: base, Changed: false, CreatedAt: time.Now().UTC()})
}

// Shadow compares a retained baseline with either an exact retained candidate
// or the current active ruleset. Both sides must describe the same published
// topic version and pack digest; a comparison never changes active state.
func (s *Service) Shadow(ctx context.Context, e identity.Envelope, topic string, in ShadowRequest) (Comparison, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(topic) || !identity.Identifier(in.BaselineRuleVersion) || in.CandidateRuleVersion != "" && !identity.Identifier(in.CandidateRuleVersion) || in.TopicVersion != "" && !identity.Identifier(in.TopicVersion) {
		return Comparison{}, store.ErrInvalid
	}
	if err := validReferences(in.References); err != nil {
		return Comparison{}, err
	}
	baseline, baselineSubject, baselineModel, err := s.readPinned(ctx, e, topic, in.TopicVersion, in.BaselineRuleVersion, false)
	if err != nil {
		return Comparison{}, err
	}
	baseline.Result, err = semantics.EvaluateConstraints(baselineSubject, baselineModel, in.References)
	if err != nil {
		return Comparison{}, err
	}
	baseline.EvaluatedAt = time.Now().UTC()
	candidateTopicVersion := baseline.TopicVersion
	candidate, candidateSubject, candidateModel, err := s.readPinned(ctx, e, topic, candidateTopicVersion, in.CandidateRuleVersion, in.CandidateRuleVersion == "")
	if err != nil {
		return Comparison{}, err
	}
	candidate.Result, err = semantics.EvaluateConstraints(candidateSubject, candidateModel, in.References)
	if err != nil {
		return Comparison{}, err
	}
	candidate.EvaluatedAt = time.Now().UTC()
	if baseline.PackDigest != candidate.PackDigest || baseline.TopicVersion != candidate.TopicVersion {
		return Comparison{}, store.ErrConflict
	}
	id, err := newComparisonID()
	if err != nil {
		return Comparison{}, err
	}
	return s.recordComparison(ctx, e, Comparison{ID: id, Mode: comparisonShadow, Topic: topic, References: append([]semantics.Reference(nil), in.References...), Baseline: baseline, Candidate: &candidate, Changed: !sameConstraintEvaluation(baseline.Result, candidate.Result), CreatedAt: time.Now().UTC()})
}

// Patterns returns detached clarification patterns from the exact or current
// published ruleset. Matching and value validation remain query consumers.
func (s *Service) Patterns(ctx context.Context, e identity.Envelope, topic, version string) ([]semantics.ClarificationPattern, error) {
	published, err := s.Read(ctx, e, topic, version)
	if err != nil {
		return nil, err
	}
	out := append([]semantics.ClarificationPattern(nil), published.Definition.Patterns...)
	for i := range out {
		out[i].Targets = append([]semantics.Reference(nil), out[i].Targets...)
		out[i].Slots = append([]semantics.ClarificationSlot(nil), out[i].Slots...)
		for j := range out[i].Slots {
			out[i].Slots[j].Choices = append([]semantics.ClarificationChoice(nil), out[i].Slots[j].Choices...)
		}
	}
	return out, nil
}

// Invalidations reads atomic lifecycle fences for downstream query/evidence
// consumers. It is intentionally metadata-only and never rewrites artifacts.
func (s *Service) Invalidations(ctx context.Context, e identity.Envelope, topic string, after int64, limit int) ([]Invalidation, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(topic) || after < 0 || limit < 1 || limit > 128 {
		return nil, store.ErrInvalid
	}
	if s.evidence == nil {
		return nil, store.ErrUnavailable
	}
	return s.evidence.ReadInvalidations(ctx, e, topic, after, limit)
}
