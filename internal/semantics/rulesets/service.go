package rulesets

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

type Service struct {
	repo   Repository
	topics TopicRepository
}

func New(repo Repository, topicRepo TopicRepository) (*Service, error) {
	if repo == nil || topicRepo == nil {
		return nil, store.ErrInvalid
	}
	return &Service{repo: repo, topics: topicRepo}, nil
}

func publishedSubject(p topics.Published) (semantics.RuleSubject, error) {
	pack := semantics.TopicPack{SchemaVersion: p.Definition.SchemaVersion, Topic: p.Definition.Topic, Version: p.Definition.Version, Name: p.Definition.Name, Description: p.Definition.Description, Measures: p.Definition.Measures, Dimensions: p.Definition.Dimensions, KPIs: p.Definition.KPIs, Joins: p.Definition.Joins}
	for _, dataset := range p.Definition.Datasets {
		pack.Datasets = append(pack.Datasets, semantics.Dataset{ID: dataset.ID, Name: dataset.Name, Columns: append([]semantics.Column(nil), dataset.Columns...)})
	}
	return semantics.NewRuleSubject(pack, p.Digest)
}

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

func (s *Service) Read(ctx context.Context, e identity.Envelope, topic, version string) (Published, error) {
	if ctx == nil || !identity.Identifier(topic) || version != "" && !identity.Identifier(version) {
		return Published{}, store.ErrInvalid
	}
	pin, err := s.repo.RuleVersionPin(ctx, e, topic, version, drafts.Read)
	if err != nil {
		return Published{}, err
	}
	if _, err = s.topics.ReadPublishedTopic(ctx, e, topic, pin.TopicVersion, drafts.Read); err != nil {
		return Published{}, err
	}
	return s.repo.ReadPublishedRules(ctx, e, topic, pin.RuleVersion, drafts.Read)
}

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
	return Evaluation{Topic: topic, TopicVersion: pin.TopicVersion, RuleVersion: pin.Version, RuleDigest: published.Digest, Result: result, EvaluatedAt: time.Now().UTC()}, nil
}
