package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
)

// TestCW06RulesReporting proves the real PostgreSQL linkage between immutable
// rule publications and reporting validation/certification. It deliberately
// uses synthetic metadata and the ordinary source validator/executor path.
func TestCW06RulesReporting(t *testing.T) {
	f := newReportingFixture(t)
	query, topicService := newPhase18Service(t, f)
	e := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	ctx := context.Background()
	publishedTopic, err := topicService.Read(ctx, e, f.pack.Topic, f.pack.Version)
	if err != nil {
		t.Fatal("read topic", err)
	}
	rules, err := rulesets.New(f.f.db, f.f.db, f.f.db)
	if err != nil {
		t.Fatal("rules service", err)
	}
	phase17PublishRules(t, rules, e, publishedTopic)
	publishedRules, err := rules.Read(ctx, e, f.pack.Topic, "")
	if err != nil {
		t.Fatal("read current rules", err)
	}

	blocks, err := reporting.New(f.f.db, topicService, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query, rules), config.DefaultReporting(), rules)
	if err != nil {
		t.Fatal("reporting service", err)
	}
	definition := phase27Definition(t, f, e, "SELECT id, amount FROM analytics.sales ORDER BY id /* CW06_RULE_SNAPSHOT */")
	definition, err = reporting.MigrateDefinition(definition)
	if err != nil {
		t.Fatal("migrate definition", err)
	}
	definition.Rules = []reporting.RulePin{{Topic: f.pack.Topic, TopicVersion: publishedTopic.State.Version, PackDigest: publishedTopic.Digest, RuleVersion: publishedRules.State.Version, RuleDigest: publishedRules.Digest}}
	draft, err := blocks.Create(ctx, e, reporting.CreateRequest{ID: "cw06-rule-snapshot", Definition: definition})
	if err != nil || len(draft.Rules) != 1 {
		t.Fatalf("create rule-bound block: %#v %v", draft, err)
	}
	state, evidence := phase27ValidatePublish(t, blocks, e, draft)
	attestation, err := blocks.Certify(ctx, e, draft.State.ID, reporting.CertifyRequest{ExpectedVersion: state.Version, Revision: state.PublishedRevision, Evidence: evidence.ID, Note: "Reviewed exact rule snapshot"})
	if err != nil || attestation.DependencyDigest != evidence.DependencyDigest {
		t.Fatalf("certify rule-bound block: %#v %v", attestation, err)
	}
	before, err := blocks.Read(ctx, e, draft.State.ID, reporting.Reference{})
	if err != nil || before.Trust.Certification != "valid" || before.Trust.Health.Status != "healthy" {
		t.Fatalf("initial certification not healthy: %#v %v", before.Trust, err)
	}

	next := phase16RuleDefinition(publishedTopic)
	next.Version = "rules-v2"
	next.Rules = append(next.Rules, semantics.RuleDefinition{ID: "cw06-v2-advisory", Version: "v1", Category: semantics.RuleSemantic, Class: semantics.RuleAdvisoryContext, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTemplate, Template: "monthly_sales"}, Priority: 10, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "cw06-review-v2"}, Guidance: &semantics.AdvisoryGuidance{Text: "Use the reviewed monthly template context.", Sensitivity: semantics.LiteralNonSensitive}})
	candidate, err := rules.Save(ctx, e, rulesets.SaveRequest{Expected: 1, Definition: next, Change: "CW06 replacement publication"})
	if err != nil {
		t.Fatal("save replacement rules", err)
	}
	review, err := rules.Review(ctx, e, f.pack.Topic, rulesets.ReviewRequest{DraftRevision: candidate.Revision, Digest: candidate.Digest, Decision: "approve", Note: "Review replacement rules"})
	if err != nil {
		t.Fatal("review replacement rules", err)
	}
	if _, err = rules.Publish(ctx, e, f.pack.Topic, rulesets.PublishRequest{Review: review.ID, Expected: 1}); err != nil {
		t.Fatal("publish replacement rules", err)
	}
	after, err := blocks.Read(ctx, e, draft.State.ID, reporting.Reference{})
	if err != nil || after.Trust.Certification != "stale" || after.Trust.Health.Status != "stale" || after.Trust.Health.Reason != "rule_publication_changed" {
		t.Fatalf("replacement did not stale approval: %#v %v", after.Trust, err)
	}
	if _, err = blocks.Validate(ctx, e, draft.State.ID, reporting.ValidateRequest{ExpectedVersion: after.State.Version, Revision: after.Revision}); !errors.Is(err, reporting.ErrStale) {
		t.Fatalf("stale exact rule dependency was revalidated silently: %v", err)
	}
	historical, err := rules.Read(ctx, e, f.pack.Topic, publishedRules.State.Version)
	if err != nil || historical.Digest != publishedRules.Digest {
		t.Fatalf("historical rule publication mutated: %#v %v", historical, err)
	}
}
