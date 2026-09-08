package rulesets

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

func TestServiceRejectsInvalidBoundariesBeforeDependencies(t *testing.T) {
	if _, err := New(nil, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("nil dependencies", err)
	}
	s := &Service{}
	ctx := context.Background()
	var nilContext context.Context
	e := identity.Envelope{}
	definition := semantics.RuleSetDefinition{Topic: "commerce"}
	if _, err := s.Save(nilContext, e, SaveRequest{Definition: definition, Change: "change"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("nil save context", err)
	}
	if _, err := s.Review(ctx, e, "bad/topic", ReviewRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid review", err)
	}
	if _, err := s.Publish(ctx, e, "bad/topic", PublishRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid publication", err)
	}
	if _, err := s.Read(ctx, e, "bad/topic", ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid read", err)
	}
	if _, err := s.Retire(ctx, e, "bad/topic", RetireRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid retirement", err)
	}
	if _, err := s.Evaluate(ctx, e, "bad/topic", EvaluateRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid evaluation", err)
	}
}

type evidenceBoundaryRepository struct {
	pin   Pin
	rules Published
}

func (r *evidenceBoundaryRepository) SaveRuleDraft(context.Context, identity.Envelope, topics.Published, semantics.RuleModel, int64, string) (Draft, error) {
	return Draft{}, store.ErrUnavailable
}

func (r *evidenceBoundaryRepository) ReviewRuleDraft(context.Context, identity.Envelope, topics.Published, string, ReviewRequest) (Review, error) {
	return Review{}, store.ErrUnavailable
}

func (r *evidenceBoundaryRepository) PublishRules(context.Context, identity.Envelope, topics.Published, string, int64) (Published, error) {
	return Published{}, store.ErrUnavailable
}

func (r *evidenceBoundaryRepository) RuleVersionPin(context.Context, identity.Envelope, string, string, drafts.Access) (Pin, error) {
	return r.pin, nil
}

func (r *evidenceBoundaryRepository) ReadPublishedRules(context.Context, identity.Envelope, string, string, drafts.Access, bool) (Published, error) {
	return r.rules, nil
}

func (r *evidenceBoundaryRepository) RetireRules(context.Context, identity.Envelope, topics.Published, string, int64) (State, error) {
	return State{}, store.ErrUnavailable
}

type evidenceBoundaryTopics struct{ published topics.Published }

func (r *evidenceBoundaryTopics) ReadPublishedTopic(context.Context, identity.Envelope, string, string, drafts.Access) (topics.Published, error) {
	return r.published, nil
}

type evidenceBoundaryStore struct {
	comparisons   []Comparison
	invalidations []Invalidation
}

func (s *evidenceBoundaryStore) RecordComparison(_ context.Context, _ identity.Envelope, comparison Comparison) (Comparison, error) {
	s.comparisons = append(s.comparisons, comparison)
	return comparison, nil
}

func (s *evidenceBoundaryStore) ReadInvalidations(_ context.Context, _ identity.Envelope, _ string, _ int64, _ int) ([]Invalidation, error) {
	return append([]Invalidation(nil), s.invalidations...), nil
}

func evidenceBoundaryFixture(t *testing.T) (*Service, *evidenceBoundaryRepository, *evidenceBoundaryStore, identity.Envelope) {
	t.Helper()
	digest := strings.Repeat("a", 64)
	column := semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "amount"}
	published := topics.Published{
		State: topics.State{Topic: "commerce", Revision: 1, Version: "topic-v1", Active: true},
		Definition: topics.Definition{
			SchemaVersion: 1, Topic: "commerce", Version: "topic-v1", Name: "Commerce",
			Datasets: []topics.Dataset{{ID: "sales", Name: "Sales", Source: topics.Binding{Source: "warehouse", Context: "ctx", Dataset: "sales"}, Columns: []semantics.Column{{ID: "amount", Name: "Amount"}}}},
			Measures: []semantics.Measure{{ID: "revenue", Name: "Revenue", Field: column, Aggregation: semantics.AggregationSum}},
		},
		Digest: digest,
	}
	rules := semantics.RuleSetDefinition{
		SchemaVersion: semantics.SchemaVersion, ID: "commerce-rules", Version: "rules-v1", Topic: "commerce", TopicVersion: "topic-v1", PackDigest: digest,
		Rules: []semantics.RuleDefinition{{
			ID: "require-revenue", Version: "v1", Category: semantics.RuleComputation, Class: semantics.RuleExecutionConstraint,
			Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 100,
			Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "review-1"},
			Constraint: &semantics.Constraint{Kind: semantics.ConstraintRequireReference, Target: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}},
		}},
	}
	repo := &evidenceBoundaryRepository{pin: Pin{RuleVersion: rules.Version, TopicVersion: published.State.Version, PackDigest: digest}, rules: Published{State: State{Topic: "commerce", Revision: 1, Version: rules.Version, Active: true}, Definition: rules, Digest: digest}}
	evidence := &evidenceBoundaryStore{invalidations: []Invalidation{{ID: "inv-1", Topic: "commerce", Revision: 2, Kind: "publish", OldRuleVersion: "rules-v0", NewRuleVersion: rules.Version, TopicVersion: published.State.Version, PackDigest: digest}}}
	topicsRepo := &evidenceBoundaryTopics{published: published}
	service, err := New(repo, topicsRepo, evidence)
	if err != nil {
		t.Fatal(err)
	}
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"topics.read", "cw.topic.read:commerce"}, time.Now().Add(time.Minute), nil)
	if err != nil {
		t.Fatal(err)
	}
	return service, repo, evidence, e
}

func TestEvidenceServiceBoundariesPreservePins(t *testing.T) {
	service, repo, evidence, e := evidenceBoundaryFixture(t)
	ctx := context.Background()
	if _, err := New(repo, &evidenceBoundaryTopics{}, evidence, evidence); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("multiple evidence repositories accepted", err)
	}
	for _, tc := range []struct {
		name  string
		topic string
		after int64
		limit int
	}{
		{"empty topic", "", 0, 8},
		{"negative cursor", "commerce", -1, 8},
		{"zero limit", "commerce", 0, 0},
		{"oversized limit", "commerce", 0, 129},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Invalidations(ctx, e, tc.topic, tc.after, tc.limit); !errors.Is(err, store.ErrInvalid) {
				t.Fatalf("invalid invalidation boundary returned %v", err)
			}
		})
	}
	withoutEvidence := &Service{repo: repo, topics: service.topics}
	if _, err := withoutEvidence.Invalidations(ctx, e, "commerce", 0, 8); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal("missing evidence store accepted", err)
	}
	got, err := service.Invalidations(ctx, e, "commerce", 1, 8)
	if err != nil || len(got) != 1 || got[0].Revision != 2 || got[0].OldRuleVersion != "rules-v0" || got[0].NewRuleVersion != "rules-v1" {
		t.Fatalf("invalidation pins changed: %#v err=%v", got, err)
	}
	if len(evidence.comparisons) != 0 {
		t.Fatal("invalidation read wrote comparison evidence")
	}
	refs := []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}
	if _, err := withoutEvidence.Replay(ctx, e, "commerce", ReplayRequest{RuleVersion: "rules-v1", TopicVersion: "topic-v1", References: refs}); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal("replay without evidence store did not fail closed", err)
	}
	replay, err := service.Replay(ctx, e, "commerce", ReplayRequest{RuleVersion: "rules-v1", TopicVersion: "topic-v1", References: refs})
	if err != nil || replay.Mode != comparisonReplay || replay.Candidate != nil || replay.Changed || replay.Baseline.RuleVersion != "rules-v1" || !replay.Baseline.Result.Allowed {
		t.Fatalf("replay evidence changed: %#v err=%v", replay, err)
	}
	if len(evidence.comparisons) != 1 || evidence.comparisons[0].Baseline.PackDigest != repo.pin.PackDigest {
		t.Fatalf("comparison store received incomplete pins: %#v", evidence.comparisons)
	}
	for _, tc := range []ReplayRequest{
		{RuleVersion: "rules-v1", TopicVersion: "topic-v1"},
		{RuleVersion: "rules-v1", TopicVersion: "topic-v1", References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}, {Kind: semantics.KindMeasure, ID: "revenue"}}},
	} {
		if _, err := service.Replay(ctx, e, "commerce", tc); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("invalid replay references accepted: %v", err)
		}
	}
	if _, err := service.Replay(nil, e, "commerce", ReplayRequest{RuleVersion: "rules-v1", References: refs}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("nil replay context accepted", err)
	}
	if _, err := service.Shadow(ctx, e, "bad/topic", ShadowRequest{BaselineRuleVersion: "rules-v1", References: refs}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid shadow topic accepted", err)
	}
	shadow, err := service.Shadow(ctx, e, "commerce", ShadowRequest{BaselineRuleVersion: "rules-v1", TopicVersion: "topic-v1", References: refs})
	if err != nil || shadow.Candidate == nil || shadow.Changed || len(shadow.Baseline.Result.Required) != 1 || len(shadow.Candidate.Result.Required) != 1 {
		t.Fatalf("same-pinned shadow changed evidence: %#v err=%v", shadow, err)
	}
}
