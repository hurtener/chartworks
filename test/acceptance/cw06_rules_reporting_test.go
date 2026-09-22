package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/test/support"
)

type cw06CaptureBarrier struct {
	delegate reporting.QueryCapture
	ready    chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (b *cw06CaptureBarrier) Capture(ctx context.Context, e identity.Envelope, id string) (reporting.Capture, error) {
	out, err := b.delegate.Capture(ctx, e, id)
	b.once.Do(func() { close(b.ready) })
	if err != nil {
		return out, err
	}
	select {
	case <-ctx.Done():
		return reporting.Capture{}, ctx.Err()
	case <-b.release:
		return out, nil
	}
}

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
	f.model.embeddingMode.Store("fixed")
	f.model.rerankMode.Store("fixed")
	f.model.mode.Store(phase18RawResponse(t, definition.SQL))
	oldPlan, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(f, nlq.LanguageEnglish, f.pack.Topic), Operation: "cw06-pre-replacement"})
	if err != nil {
		t.Fatal("plan pre-replacement query", err)
	}
	if _, err = query.Run(ctx, e, nlqexec.RunRequest{QueryID: oldPlan.QueryID, Operation: "cw06-pre-replacement", Rows: 10, Bytes: 65536}); err != nil {
		t.Fatal("run pre-replacement query", err)
	}
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
	next.Rules = append(next.Rules, semantics.RuleDefinition{ID: "cw06-v2-quarterly", Version: "v1", Category: semantics.RuleSemantic, Class: semantics.RuleAdvisoryContext, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTemplate, Template: "quarterly_sales"}, Priority: 9, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "cw06-review-v2"}, Guidance: &semantics.AdvisoryGuidance{Text: "Use the reviewed quarterly template context.", Sensitivity: semantics.LiteralNonSensitive}})
	candidate, err := rules.Save(ctx, e, rulesets.SaveRequest{Expected: 1, Definition: next, Change: "CW06 replacement publication"})
	if err != nil {
		t.Fatal("save replacement rules", err)
	}
	review, err := rules.Review(ctx, e, f.pack.Topic, rulesets.ReviewRequest{DraftRevision: candidate.Revision, Digest: candidate.Digest, Decision: "approve", Note: "Review replacement rules"})
	if err != nil {
		t.Fatal("review replacement rules", err)
	}
	barrier := &cw06CaptureBarrier{delegate: reporting.CaptureFromQueries(query, rules), ready: make(chan struct{}), release: make(chan struct{})}
	racingBlocks, err := reporting.New(f.f.db, topicService, f.f.s, f.f.validator, f.f.executor, barrier, config.DefaultReporting(), rules)
	if err != nil {
		t.Fatal("racing reporting service", err)
	}
	raceErr := make(chan error, 1)
	go func() {
		_, captureErr := racingBlocks.CaptureQuery(ctx, e, reporting.CaptureRequest{ID: "cw06-racing-capture", Query: oldPlan.QueryID, Metadata: definition.Metadata, Outputs: definition.Outputs})
		raceErr <- captureErr
	}()
	<-barrier.ready
	publishedReplacement, err := rules.Publish(ctx, e, f.pack.Topic, rulesets.PublishRequest{Review: review.ID, Expected: 1})
	if err != nil {
		t.Fatal("publish replacement rules", err)
	}
	close(barrier.release)
	if err = <-raceErr; !errors.Is(err, reporting.ErrStale) {
		t.Fatalf("replacement racing query capture returned %v", err)
	}
	raw := support.Raw(t, f.f.dsn)
	for _, table := range []string{"block_heads", "block_revisions", "block_rule_pins"} {
		if got := count(t, raw, "SELECT count(*) FROM chartworks."+table+" WHERE tenant_id=$1 AND block_id='cw06-racing-capture'", e.Tenant()); got != 0 {
			t.Fatalf("stale racing capture left %d rows in %s", got, table)
		}
	}
	template := rulesets.TemplateSelection{ID: "monthly_sales", Topic: f.pack.Topic, TopicVersion: publishedTopic.State.Version, PackDigest: publishedTopic.Digest, RuleVersion: publishedReplacement.State.Version, RuleDigest: publishedReplacement.Digest}
	replay, err := rules.Replay(ctx, e, f.pack.Topic, rulesets.ReplayRequest{RuleVersion: publishedReplacement.State.Version, TopicVersion: publishedTopic.State.Version, References: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "revenue"}}, Template: &template})
	if err != nil || replay.Template == nil || *replay.Template != template {
		t.Fatalf("replay lost canonical template input: %#v %v", replay, err)
	}
	var persistedTemplate []byte
	if err = raw.QueryRow(ctx, `SELECT template_selection FROM chartworks.topic_rule_comparison_evidence WHERE tenant_id=$1 AND comparison_id=$2`, e.Tenant(), replay.ID).Scan(&persistedTemplate); err != nil {
		t.Fatal("read persisted replay template", err)
	}
	var persisted rulesets.TemplateSelection
	if err = json.Unmarshal(persistedTemplate, &persisted); err != nil || persisted != template {
		t.Fatalf("persisted replay template differs: %#v %v", persisted, err)
	}
	after, err := blocks.Read(ctx, e, draft.State.ID, reporting.Reference{})
	if err != nil || after.Trust.Certification != "stale" || after.Trust.Health.Status != "stale" || after.Trust.Health.Reason != "rule_publication_changed" {
		t.Fatalf("replacement did not stale approval: %#v %v", after.Trust, err)
	}
	if _, err = blocks.Validate(ctx, e, draft.State.ID, reporting.ValidateRequest{ExpectedVersion: after.State.Version, Revision: after.Revision}); !errors.Is(err, reporting.ErrStale) {
		t.Fatalf("stale exact rule dependency was revalidated silently: %v", err)
	}
	templateQuestion := phase18Question(f, nlq.LanguageEnglish, f.pack.Topic)
	templateQuestion.Templates = []rulesets.TemplateSelection{template}
	preflight, err := query.Preflight(ctx, e, nlqexec.PreflightRequest{QuestionRequest: templateQuestion})
	if err != nil || len(preflight.Route.Templates) != 1 || preflight.Route.Templates[0] != template || len(preflight.Route.Request.Templates) != 1 || preflight.Route.Request.Templates[0] != template {
		t.Fatalf("preflight lost canonical template evidence: %#v %v", preflight.Route, err)
	}
	templatePlan, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: templateQuestion, Operation: "cw06-template-plan"})
	if err != nil || len(templatePlan.Route.Templates) != 1 || templatePlan.Route.Templates[0] != template {
		t.Fatalf("plan lost canonical template evidence: %#v %v", templatePlan.Route, err)
	}
	refined, err := query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: templatePlan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Show the same reviewed monthly revenue with more detail"}})
	if err != nil || len(refined.Route.Templates) != 1 || refined.Route.Templates[0] != template {
		t.Fatalf("refinement did not reconstruct the sealed template: %#v %v", refined.Route, err)
	}
	substituted := template
	substituted.ID = "quarterly_sales"
	if _, err = query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: templatePlan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Substitute the template", Templates: []rulesets.TemplateSelection{substituted}}}); !errors.Is(err, nlqexec.ErrInvalid) {
		t.Fatalf("refinement accepted a substituted reviewed template: %v", err)
	}
	if _, err = query.Run(ctx, e, nlqexec.RunRequest{QueryID: templatePlan.QueryID, Operation: "cw06-template-plan", Rows: 10, Bytes: 65536}); err != nil {
		t.Fatal("run template query", err)
	}
	saved := nlqexec.SavedQuestion{Durability: "replayable", Context: f.context, Topics: []nlqexec.SavedTopic{{Topic: f.pack.Topic, Version: publishedTopic.State.Version, Digest: publishedTopic.Digest}}, Question: templateQuestion.Question, Selections: &nlqexec.SavedSelections{Templates: []rulesets.TemplateSelection{template}, Kinds: []string{"measure"}, LimitPerKind: 1}}
	savedEvidence, err := query.InspectSaved(ctx, e, saved)
	if err != nil {
		t.Fatal("inspect template saved question", err)
	}
	if _, err = query.PrepareSaved(ctx, e, saved, savedEvidence, "cw06-saved-template", "en-US"); err != nil {
		t.Fatal("prepare template saved question", err)
	}
	captured, err := blocks.CaptureQuery(ctx, e, reporting.CaptureRequest{ID: "cw06-template-capture", Query: templatePlan.QueryID, Metadata: definition.Metadata, Outputs: definition.Outputs})
	if err != nil || len(captured.Rules) != 1 {
		t.Fatalf("capture template query: %#v %v", captured, err)
	}
	capturedSQL, err := blocks.SQL(ctx, e, captured.State.ID, reporting.Reference{Draft: true})
	if err != nil || capturedSQL.Definition == nil || capturedSQL.Definition.Template != nil || len(capturedSQL.Definition.Templates) != 1 || capturedSQL.Definition.Templates[0] != (reporting.TemplateSelection{ID: template.ID, Topic: template.Topic, TopicVersion: template.TopicVersion, PackDigest: template.PackDigest, RuleVersion: template.RuleVersion, RuleDigest: template.RuleDigest}) {
		t.Fatalf("capture lost reviewed template provenance: %#v %v", capturedSQL.Definition, err)
	}
	orphaned := *capturedSQL.Definition
	orphaned.Rules = append([]reporting.RulePin(nil), orphaned.Rules...)
	orphaned.Rules[0].RuleDigest = publishedRules.Digest
	if _, err = blocks.Edit(ctx, e, captured.State.ID, reporting.EditRequest{ExpectedVersion: captured.State.Version, Definition: orphaned}); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatalf("edit retained orphaned template provenance: %v", err)
	}
	capturedState, capturedEvidence := phase27ValidatePublish(t, blocks, e, captured)
	if _, err = blocks.Certify(ctx, e, captured.State.ID, reporting.CertifyRequest{ExpectedVersion: capturedState.Version, Revision: capturedState.PublishedRevision, Evidence: capturedEvidence.ID, Note: "Reviewed recaptured template coordinates"}); err != nil {
		t.Fatal("recaptured template could not validate and certify", err)
	}

	// Hold the publisher after it owns the rule-head row. Frozen admission must
	// wait on the same head, observe the replacement after commit, and leave no
	// frozen manifest behind.
	third := next
	third.Version = "rules-v3"
	thirdDraft, err := rules.Save(ctx, e, rulesets.SaveRequest{Expected: candidate.Revision, Definition: third, Change: "CW06 seal-race replacement"})
	if err != nil {
		t.Fatal("save seal-race replacement", err)
	}
	thirdReview, err := rules.Review(ctx, e, f.pack.Topic, rulesets.ReviewRequest{DraftRevision: thirdDraft.Revision, Digest: thirdDraft.Digest, Decision: "approve", Note: "Review seal-race replacement"})
	if err != nil {
		t.Fatal("review seal-race replacement", err)
	}
	sql(t, raw, `CREATE FUNCTION chartworks.cw06_delay_rule_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(1); RETURN NEW; END $$`)
	sql(t, raw, `CREATE TRIGGER cw06_delay_rule_publish BEFORE UPDATE ON chartworks.topic_rule_publication_heads FOR EACH ROW EXECUTE FUNCTION chartworks.cw06_delay_rule_publish()`)
	defer func() {
		_, _ = raw.Exec(ctx, `DROP TRIGGER IF EXISTS cw06_delay_rule_publish ON chartworks.topic_rule_publication_heads`)
		_, _ = raw.Exec(ctx, `DROP FUNCTION IF EXISTS chartworks.cw06_delay_rule_publish()`)
	}()
	publishErr := make(chan error, 1)
	go func() {
		_, publishError := rules.Publish(ctx, e, f.pack.Topic, rulesets.PublishRequest{Review: thirdReview.ID, Expected: publishedReplacement.State.Revision})
		publishErr <- publishError
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var active int
		if scanErr := raw.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND state='active' AND query LIKE '%UPDATE chartworks.topic_rule_publication_heads SET revision%'`).Scan(&active); scanErr != nil {
			t.Fatal("observe delayed publisher", scanErr)
		}
		if active > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("publisher never reached fenced rule head")
		}
		time.Sleep(10 * time.Millisecond)
	}
	executor := phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant()))
	runs := phase28RunService(t, f, blocks, f.f.db, nil, config.DefaultReportingExecution())
	if _, err = runs.Admit(ctx, executor, captured.State.ID, reporting.RunRequest{Key: "cw06-publish-during-seal"}); !errors.Is(err, reporting.ErrStale) {
		t.Fatalf("publish during frozen seal returned %v", err)
	}
	if err = <-publishErr; err != nil {
		t.Fatal("publish seal-race replacement", err)
	}
	if got := count(t, raw, `SELECT count(*) FROM chartworks.frozen_runs WHERE tenant_id=$1 AND block_id=$2`, e.Tenant(), captured.State.ID); got != 0 {
		t.Fatalf("stale frozen seal left %d manifests", got)
	}
	historical, err := rules.Read(ctx, e, f.pack.Topic, publishedRules.State.Version)
	if err != nil || historical.Digest != publishedRules.Digest {
		t.Fatalf("historical rule publication mutated: %#v %v", historical, err)
	}
}
