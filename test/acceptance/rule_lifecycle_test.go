package acceptance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/topicapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func TestRuleLifecycleAndDeterministicEvaluation(t *testing.T) {
	f, draftsService, topicService, gatewayFixture, pack := publicationFixture(t)
	ruleService, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := topicapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, topicapi.Handler(f.token.verifier, draftsService, topicService, ruleService, http.NotFoundHandler()))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	scopes := topicScopes(f.e.Tenant())
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		return f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), scopes), nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	draft, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Pack: pack, Change: "Rule subject"})
	if err != nil {
		t.Fatal("topic draft", err)
	}
	review, err := client.ReviewTopic(ctx, pack.Topic, sdk.TopicReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Approve rule subject"})
	if err != nil {
		t.Fatal("topic review", err)
	}
	publishedTopic, err := client.PublishTopic(ctx, pack.Topic, sdk.PublishTopicRequest{Review: review.ID})
	if err != nil {
		t.Fatal("topic publish", err)
	}
	fullEnvelope := f.token.envelope(t, f.e.Tenant(), f.e.User(), scopes...)
	for _, access := range []drafts.Access{drafts.Write, drafts.Review} {
		if _, err = f.db.ReadPublishedTopic(ctx, fullEnvelope, pack.Topic, pack.Version, access); err != nil {
			t.Fatal("published topic dependency read", access, err)
		}
	}
	gatewayRequests := gatewayFixture.requests.Load()

	rules := semantics.RuleSetDefinition{
		SchemaVersion: semantics.SchemaVersion,
		ID:            "commerce-rules",
		Version:       "rules-v1",
		Topic:         pack.Topic,
		TopicVersion:  pack.Version,
		PackDigest:    publishedTopic.Digest,
		Rules: []semantics.RuleDefinition{{
			ID: "require-revenue", Version: "v1", Category: semantics.RuleComputation,
			Class: semantics.RuleExecutionConstraint, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic},
			Priority: 100, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceFeedback, Evidence: "feedback-1"},
			Constraint: &semantics.Constraint{Kind: semantics.ConstraintRequireReference, Target: semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}},
		}},
	}
	publicPack := semantics.TopicPack{SchemaVersion: publishedTopic.Definition.SchemaVersion, Topic: publishedTopic.Definition.Topic, Version: publishedTopic.Definition.Version, Name: publishedTopic.Definition.Name, Description: publishedTopic.Definition.Description, Measures: publishedTopic.Definition.Measures, Dimensions: publishedTopic.Definition.Dimensions, KPIs: publishedTopic.Definition.KPIs, Joins: publishedTopic.Definition.Joins, CanonicalEntities: publishedTopic.Definition.CanonicalEntities}
	for _, dataset := range publishedTopic.Definition.Datasets {
		publicPack.Datasets = append(publicPack.Datasets, semantics.Dataset{ID: dataset.ID, Name: dataset.Name, Columns: dataset.Columns})
	}
	subject, err := semantics.NewRuleSubject(publicPack, publishedTopic.Digest)
	if err != nil {
		t.Fatal("public rule subject", err)
	}
	if _, err = semantics.CompilePublishedRules(subject, rules); err != nil {
		t.Fatal("public rule compile", err)
	}
	narrowScopes := []string{"topics.write", "cw.topic.write:" + pack.Topic, "cw.source.read:*", "cw.execution_context.use:*"}
	narrowEnvelope := f.token.envelope(t, f.e.Tenant(), f.e.User(), narrowScopes...)
	if _, err = f.db.ReadPublishedTopic(ctx, narrowEnvelope, pack.Topic, pack.Version, drafts.Write); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("published topic dependency reach widened", err)
	}
	narrowClient, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		return f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), narrowScopes), nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = narrowClient.SaveRuleDraft(ctx, pack.Topic, sdk.SaveRuleDraftRequest{Definition: rules, Change: "Missing dataset reach"}); err == nil {
		t.Fatal("rule draft ignored topic dependency reach")
	}
	ruleDraft, err := client.SaveRuleDraft(ctx, pack.Topic, sdk.SaveRuleDraftRequest{Definition: rules, Change: "Propose required revenue"})
	if err != nil || ruleDraft.Revision != 1 || ruleDraft.Digest == "" {
		t.Fatal("rule draft", ruleDraft, err)
	}
	if _, err = client.SaveRuleDraft(ctx, pack.Topic, sdk.SaveRuleDraftRequest{Expected: 7, Definition: rules, Change: "Wrong revision"}); err == nil {
		t.Fatal("rule draft CAS not enforced")
	}
	if _, err = client.PublishRules(ctx, pack.Topic, sdk.PublishRulesRequest{Review: ruleDraft.Digest}); err == nil {
		t.Fatal("draft self-published")
	}
	rejected, err := client.ReviewRules(ctx, pack.Topic, sdk.RuleReviewRequest{DraftRevision: 1, Digest: ruleDraft.Digest, Decision: "reject", Note: "Reject proposal"})
	if err != nil {
		t.Fatal("reject", err)
	}
	if _, err = client.PublishRules(ctx, pack.Topic, sdk.PublishRulesRequest{Review: rejected.ID}); !sdkConflict(err) {
		t.Fatal("rejected rules published", err)
	}
	approved, err := client.ReviewRules(ctx, pack.Topic, sdk.RuleReviewRequest{DraftRevision: 1, Digest: ruleDraft.Digest, Decision: "approve", Note: "Approve hard constraint"})
	if err != nil {
		t.Fatal("approve", err)
	}
	type publishResult struct {
		published sdk.PublishedRules
		err       error
	}
	results := make(chan publishResult, 2)
	for range 2 {
		go func() {
			published, publishErr := client.PublishRules(ctx, pack.Topic, sdk.PublishRulesRequest{Review: approved.ID})
			results <- publishResult{published: published, err: publishErr}
		}()
	}
	var active sdk.PublishedRules
	successes, conflicts := 0, 0
	for range 2 {
		result := <-results
		if result.err == nil {
			active = result.published
			successes++
		} else if sdkConflict(result.err) {
			conflicts++
		} else {
			t.Fatal("concurrent publish rules", result.err)
		}
	}
	if successes != 1 || conflicts != 1 || active.State.Revision != 1 || !active.State.Active || active.State.Retired {
		t.Fatal("publication CAS race", successes, conflicts, active.State)
	}

	missing, err := client.EvaluateRules(ctx, pack.Topic, sdk.RuleEvaluationRequest{References: []semantics.Reference{{Kind: semantics.KindDataset, ID: pack.Datasets[0].ID}}})
	if err != nil || missing.Result.Allowed || len(missing.Result.Violations) != 1 || missing.Result.Violations[0].Kind != semantics.ViolationMissingRequired {
		t.Fatal("missing hard constraint", missing, err)
	}
	allowed, err := client.EvaluateRules(ctx, pack.Topic, sdk.RuleEvaluationRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}})
	if err != nil || !allowed.Result.Allowed || len(allowed.Result.Required) != 1 || allowed.TopicVersion != pack.Version || allowed.RuleVersion != rules.Version {
		t.Fatal("allowed hard constraint", allowed, err)
	}
	if _, err = client.EvaluateRules(ctx, pack.Topic, sdk.RuleEvaluationRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "unknown"}}}); err == nil {
		t.Fatal("unknown evaluation reference accepted")
	}
	retained, err := client.PublishedRules(ctx, pack.Topic)
	if err != nil || !reflect.DeepEqual(retained, active) {
		t.Fatal("active retained rules", err)
	}
	metadata := support.Raw(t, f.dsn)
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.topic_rule_published_versions SET digest=$1 WHERE tenant_id=$2 AND topic_id=$3 AND version_id=$4`, ruleDraft.Digest, f.e.Tenant(), pack.Topic, rules.Version); err == nil {
		t.Fatal("published rules mutated")
	}

	rules.Version = "rules-v2"
	rules.Rules[0].Priority = 200
	secondDraft, err := client.SaveRuleDraft(ctx, pack.Topic, sdk.SaveRuleDraftRequest{Expected: 1, Definition: rules, Change: "Raise explicit priority"})
	if err != nil || secondDraft.Revision != 2 {
		t.Fatal("second draft", secondDraft, err)
	}
	secondReview, err := client.ReviewRules(ctx, pack.Topic, sdk.RuleReviewRequest{DraftRevision: 2, Digest: secondDraft.Digest, Decision: "approve", Note: "Approve second version"})
	if err != nil {
		t.Fatal("second review", err)
	}
	second, err := client.PublishRules(ctx, pack.Topic, sdk.PublishRulesRequest{Review: secondReview.ID, Expected: 1})
	if err != nil || second.State.Revision != 2 || second.State.Version != rules.Version {
		t.Fatal("second publish", second.State, err)
	}
	firstExact, err := client.PublishedRuleVersion(ctx, pack.Topic, "rules-v1")
	if err != nil || firstExact.State.Active || !firstExact.State.Retired || firstExact.State.Revision != 2 {
		t.Fatal("prior version was not retired", firstExact.State, err)
	}
	if _, err = client.RetireRules(ctx, pack.Topic, sdk.RetireRulesRequest{Expected: 1, Note: "Wrong revision"}); !sdkConflict(err) {
		t.Fatal("retirement CAS not enforced", err)
	}
	if gatewayFixture.requests.Load() != gatewayRequests {
		t.Fatal("rule lifecycle or evaluation called model gateway")
	}

	pack.Version = "v2"
	pack.Description = "Synthetic rule subject replacement"
	topicDraftV2, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Expected: 1, Pack: pack, Change: "Replace the published rule subject"})
	if err != nil || topicDraftV2.Metadata.Revision != 2 {
		t.Fatal("replacement topic draft", topicDraftV2.Metadata, err)
	}
	topicReviewV2, err := client.ReviewTopic(ctx, pack.Topic, sdk.TopicReviewRequest{DraftRevision: 2, Digest: topicDraftV2.Metadata.Digest, Decision: "approve", Note: "Approve replacement topic"})
	if err != nil {
		t.Fatal("replacement topic review", err)
	}
	topicV2, err := client.PublishTopic(ctx, pack.Topic, sdk.PublishTopicRequest{Review: topicReviewV2.ID, Expected: 1})
	if err != nil || topicV2.State.Revision != 2 || topicV2.State.Version != "v2" || !topicV2.State.Active {
		t.Fatal("replacement topic publish", topicV2.State, err)
	}
	topicTransitionRequests := gatewayFixture.requests.Load()
	if _, err = client.PublishedRules(ctx, pack.Topic); !sdkConflict(err) {
		t.Fatal("stale current rules remained readable", err)
	}
	if _, err = client.EvaluateRules(ctx, pack.Topic, sdk.RuleEvaluationRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}}); !sdkConflict(err) {
		t.Fatal("stale current rules remained evaluable", err)
	}
	secondExact, err := client.PublishedRuleVersion(ctx, pack.Topic, "rules-v2")
	if err != nil || !secondExact.State.Active || secondExact.State.Retired || secondExact.Definition.TopicVersion != "v1" {
		t.Fatal("retained rules unavailable after topic replacement", secondExact.State, err)
	}
	restored, err := client.RollbackTopic(ctx, pack.Topic, sdk.TopicTransitionRequest{Version: "v1", Expected: 2, Note: "Restore the pinned rule subject"})
	if err != nil || restored.State.Revision != 3 || restored.State.Version != "v1" || !restored.State.Active {
		t.Fatal("restore pinned topic", restored.State, err)
	}
	allowed, err = client.EvaluateRules(ctx, pack.Topic, sdk.RuleEvaluationRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}})
	if err != nil || !allowed.Result.Allowed || allowed.TopicVersion != "v1" || allowed.RuleVersion != "rules-v2" {
		t.Fatal("restored topic did not reactivate evaluation", allowed, err)
	}
	archived, err := client.ArchiveTopic(ctx, pack.Topic, sdk.ArchiveTopicRequest{Expected: 3, Note: "Archive the rule subject"})
	if err != nil || archived.Revision != 4 || !archived.Archived || archived.Active {
		t.Fatal("archive rule subject", archived, err)
	}
	if _, err = client.PublishedRules(ctx, pack.Topic); !sdkConflict(err) {
		t.Fatal("archived current rules remained readable", err)
	}
	if _, err = client.EvaluateRules(ctx, pack.Topic, sdk.RuleEvaluationRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}}); !sdkConflict(err) {
		t.Fatal("archived current rules remained evaluable", err)
	}
	if _, err = client.PublishedRuleVersion(ctx, pack.Topic, "rules-v2"); err != nil {
		t.Fatal("retained rules unavailable after archive", err)
	}

	type retireResult struct {
		state sdk.RuleState
		err   error
	}
	type topicResult struct {
		published sdk.PublishedTopic
		err       error
	}
	retireResults := make(chan retireResult, 1)
	topicResults := make(chan topicResult, 1)
	go func() {
		state, retireErr := client.RetireRules(ctx, pack.Topic, sdk.RetireRulesRequest{Expected: 2, Note: "Retire rules after archive"})
		retireResults <- retireResult{state: state, err: retireErr}
	}()
	go func() {
		published, rollbackErr := client.RollbackTopic(ctx, pack.Topic, sdk.TopicTransitionRequest{Version: "v1", Expected: 4, Note: "Restore archived topic"})
		topicResults <- topicResult{published: published, err: rollbackErr}
	}()
	retired := <-retireResults
	rolledBack := <-topicResults
	if retired.err != nil || retired.state.Revision != 3 || retired.state.Version != "rules-v2" || !retired.state.Retired || retired.state.Active {
		t.Fatal("retire after archive", retired.state, retired.err)
	}
	if rolledBack.err != nil || rolledBack.published.State.Revision != 5 || rolledBack.published.State.Version != "v1" || !rolledBack.published.State.Active || rolledBack.published.State.Archived {
		t.Fatal("concurrent topic rollback", rolledBack.published.State, rolledBack.err)
	}
	if _, err = client.PublishedRules(ctx, pack.Topic); err == nil {
		t.Fatal("retired rules remained active")
	}
	secondExact, err = client.PublishedRuleVersion(ctx, pack.Topic, "rules-v2")
	if err != nil || secondExact.State.Active || !secondExact.State.Retired || secondExact.State.Revision != 3 {
		t.Fatal("retired exact read", secondExact.State, err)
	}
	if _, err = client.EvaluateRules(ctx, pack.Topic, sdk.RuleEvaluationRequest{References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}}); err == nil {
		t.Fatal("retired rules evaluated as active")
	}
	if gatewayFixture.requests.Load() != topicTransitionRequests {
		t.Fatal("rule reads, evaluation, retirement, or topic transitions called model gateway")
	}
}

func sdkConflict(err error) bool {
	var status *sdk.StatusError
	return errors.As(err, &status) && status.Status == 409
}
