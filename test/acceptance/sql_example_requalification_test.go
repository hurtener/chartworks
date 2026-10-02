package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func TestSQLRecoveryExampleRequalificationAcceptance(t *testing.T) {
	f := newPhase18FixtureReviewed(t, true)
	query, published := newPhase18Service(t, f)
	e := phase18Envelope(t, f, f.f.e.User(), f.f.e.Session(), true)
	ctx := t.Context()
	metadata := support.Raw(t, f.f.dsn)
	sql(t, f.f.admin, `UPDATE analytics.sales SET amount=CASE id WHEN 1 THEN 100 ELSE 50 END`)
	statement := "SELECT sum(amount) AS revenue FROM analytics.sales"
	f.model.mode.Store(phase18RawResponse(t, statement))
	question := phase18Question(f, nlq.LanguageEnglish, f.pack.Topic)
	question.MetricIDs = []string{"revenue"}
	planned, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal(err)
	}
	if err = query.Feedback(ctx, e, nlqexec.FeedbackRequest{QueryID: planned.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal(err)
	}
	rows, err := query.Examples(ctx, e, f.pack.Topic, 8)
	if err != nil || len(rows) != 1 {
		t.Fatal("initial example", err)
	}
	original, err := query.ExampleState(ctx, e, nlqexec.ExampleStateRequest{ExampleID: rows[0].ID, ExpectedVersion: rows[0].Version, State: "active", ReviewNote: "Reviewed initial SUM semantics"})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := store.NewScope(e.Tenant(), e.User())
	original, err = f.f.db.ReadExample(ctx, scope, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest := readexec.Hash(original)
	draftService, err := drafts.New(f.f.db, f.f.s, f.f.service)
	if err != nil {
		t.Fatal(err)
	}
	republish := func(version string, aggregation semantics.Aggregation) {
		current, err := draftService.Read(ctx, e, f.pack.Topic, 0)
		if err != nil {
			t.Fatal(err)
		}
		pack := cloneTopic(t, current.Pack)
		pack.Version = version
		pack.Description = "Explicit current semantic review " + version
		pack.Measures[0].Aggregation = aggregation
		next, err := draftService.Save(ctx, e, drafts.SaveRequest{Expected: current.Metadata.Revision, Pack: pack, Change: "Explicit semantic publication for requalification"})
		if err != nil {
			t.Fatal(err)
		}
		review, err := published.Review(ctx, e, pack.Topic, topics.ReviewRequest{DraftRevision: next.Metadata.Revision, Digest: next.Metadata.Digest, Decision: "approve", Note: "Independent current semantic review"})
		if err != nil {
			t.Fatal(err)
		}
		old, err := published.Read(ctx, e, pack.Topic, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = published.Publish(ctx, e, pack.Topic, topics.PublishRequest{Expected: old.State.Revision, Review: review.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := query.RequalifyExample(ctx, e, nlqexec.ExampleRequalificationRequest{ExampleID: original.ID, ExpectedVersion: original.Version, Anchor: question}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("current example requalified", err)
	}
	republish("requal-v2", semantics.AggregationSum)
	handler := nlqapi.ExecutionHandler(f.model.token.verifier, query, http.NotFoundHandler())
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return phase18Token(t, f, e.User(), e.Session(), true), nil })
	if err != nil {
		t.Fatal(err)
	}
	request := nlqexec.ExampleRequalificationRequest{ExampleID: original.ID, ExpectedVersion: original.Version, Anchor: question}
	candidate, err := client.RequalifyExampleNLQ(ctx, request)
	if err != nil {
		t.Fatal("requalify through SDK/HTTP", err)
	}
	if candidate.ID == original.ID || candidate.State != "candidate" || candidate.ReviewedAt != nil || candidate.Origin.Requalification == nil || candidate.Origin.TopicVersion != "requal-v2" || candidate.Origin.Requalification.Digest != original.Digest {
		t.Fatal("qualification lost separate origin/review")
	}
	if _, err := query.RequalifyExample(ctx, e, nlqexec.ExampleRequalificationRequest{ExampleID: candidate.ID, ExpectedVersion: candidate.Version, Anchor: question}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("current candidate spawned qualification chain", err)
	}
	again, err := client.RequalifyExampleNLQ(ctx, request)
	if err != nil || again.ID != candidate.ID {
		t.Fatal("qualification replay", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := client.RequalifyExampleNLQ(ctx, request)
			if err == nil && v.ID != candidate.ID {
				err = store.ErrConflict
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent qualification", err)
		}
	}
	bindings, err := nlqapi.ExecutionMCPBindings(query)
	if err != nil {
		t.Fatal("MCP registration", err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	mcpServer, err := mcpserver.New(f.model.token.verifier, mcpRegistry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	claims := f.model.token.claims(e.Tenant(), e.User(), append(e.Scopes(), "mcp.use"))
	claims["session"] = e.Session()
	claims["aud"] = f.model.token.cfg.MCPAudience()
	bearer := f.model.token.sign(t, claims, nil)
	mcpClient, err := mcpServer.Client(func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(request)
	response, err := mcpClient.CallTool(ctx, "requalify_example", raw)
	if err != nil || response == nil || response.IsError {
		t.Fatal("MCP qualified candidate", err, response)
	}
	old, err := f.f.db.ReadExample(ctx, scope, original.ID)
	if err != nil || readexec.Hash(old) != beforeDigest {
		t.Fatal("old example mutated", err)
	}
	if _, err = query.ExampleState(ctx, e, nlqexec.ExampleStateRequest{ExampleID: candidate.ID, ExpectedVersion: candidate.Version, State: "active", ReviewNote: "Explicitly reviewed fresh SUM qualification"}); err != nil {
		t.Fatal("separate activation", err)
	}
	later, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal(err)
	}
	record, err := f.f.db.ReadQuery(ctx, scope, later.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	used := false
	if record.ExampleSelection.Usage != nil {
		for _, item := range record.ExampleSelection.Usage.Used {
			used = used || item.ExampleID == candidate.ID
			if item.ExampleID == original.ID {
				t.Fatal("stale example still selected")
			}
		}
	}
	if !used {
		t.Fatal("fresh qualification did not reach prompt")
	}
	result, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: later.QueryID, Operation: later.QueryID + "-run"})
	if err != nil || result.Execution.Result == nil || !liveSingleNumericEquals(result.Execution.Result.Rows, "150") {
		t.Fatal("requalified generation result", err)
	}
	bundle, err := query.ExportExamples(ctx, e, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 8})
	if err != nil || bundle.SchemaVersion != 5 {
		t.Fatal("versioned qualified export", err)
	}
	for _, row := range bundle.Examples {
		if row.Origin.Requalification != nil {
			for _, mutate := range []func(*nlqexec.PortableExample){func(x *nlqexec.PortableExample) { x.SQL = "SELECT 1" }, func(x *nlqexec.PortableExample) { x.Question = "Different meaning" }} {
				bad := row
				mutate(&bad)
				if _, err := query.ImportExample(ctx, e, nlqexec.ExampleImportRequest{Anchor: question, Example: bad}); !errors.Is(err, readexec.ErrBinding) {
					t.Fatal("changed qualified export accepted", err)
				}
			}
			imported, err := query.ImportExample(ctx, e, nlqexec.ExampleImportRequest{Anchor: question, Example: row})
			if err != nil || imported.ID != candidate.ID {
				t.Fatal("portable qualification fresh revalidation", err)
			}
		}
	}

	// Synchronize after the empty rule-head read, before candidate insertion.
	// First rule publication must wait on the existing topic-head fence.
	rules, err := rulesets.New(f.f.db, f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := published.Read(ctx, e, f.pack.Topic, "")
	if err != nil {
		t.Fatal(err)
	}
	definition := semantics.RuleSetDefinition{SchemaVersion: 1, ID: "qualification-rules", Version: "rules-v1", Topic: f.pack.Topic, TopicVersion: publication.State.Version, PackDigest: publication.Digest, Rules: []semantics.RuleDefinition{{ID: "current-advice", Version: "v1", Category: semantics.RuleSemantic, Class: semantics.RuleAdvisoryContext, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 1, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-current-rule-review"}, Guidance: &semantics.AdvisoryGuidance{Text: "Use the current reviewed metric", Sensitivity: semantics.LiteralNonSensitive}}}}
	ruleDraft, err := rules.Save(ctx, e, rulesets.SaveRequest{Definition: definition, Change: "First reviewed rule publication"})
	if err != nil {
		t.Fatal(err)
	}
	ruleReview, err := rules.Review(ctx, e, f.pack.Topic, rulesets.ReviewRequest{DraftRevision: ruleDraft.Revision, Digest: ruleDraft.Digest, Decision: "approve", Note: "Reviewed first rule"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = metadata.Exec(ctx, `CREATE FUNCTION chartworks.test_qualification_barrier() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.origin ? 'requalification' THEN PERFORM pg_advisory_xact_lock(738291); END IF; RETURN NEW; END $$; CREATE TRIGGER test_qualification_barrier BEFORE INSERT ON chartworks.nlq_examples FOR EACH ROW EXECUTE FUNCTION chartworks.test_qualification_barrier(); SELECT pg_advisory_lock(738291)`)
	if err != nil {
		t.Fatal(err)
	}
	defer metadata.Exec(context.Background(), `SELECT pg_advisory_unlock(738291)`)
	qualified := make(chan error, 1)
	go func() { _, err := query.RequalifyExample(ctx, e, request); qualified <- err }()
	waitLocked := func(pattern string) bool {
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			var found bool
			if err := metadata.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1)`, pattern).Scan(&found); err != nil {
				t.Fatal(err)
			}
			if found {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}
	if !waitLocked("INSERT INTO chartworks.nlq_examples%") {
		t.Fatal("qualification never reached commit barrier")
	}
	publishedRule := make(chan error, 1)
	go func() {
		_, err := rules.Publish(ctx, e, f.pack.Topic, rulesets.PublishRequest{Review: ruleReview.ID})
		publishedRule <- err
	}()
	if !waitLocked("%FOR SHARE OF h%") {
		t.Fatal("first rule publication escaped qualification fence")
	}
	if _, err = metadata.Exec(ctx, `SELECT pg_advisory_unlock(738291)`); err != nil {
		t.Fatal(err)
	}
	if err = <-qualified; err != nil {
		t.Fatal("qualified before rule publication", err)
	}
	if err = <-publishedRule; err != nil {
		t.Fatal("serialized first rule publication", err)
	}
	if _, err = metadata.Exec(ctx, `DROP TRIGGER test_qualification_barrier ON chartworks.nlq_examples; DROP FUNCTION chartworks.test_qualification_barrier()`); err != nil {
		t.Fatal(err)
	}
	republish("requal-v3", semantics.AggregationAverage)
	publication, err = published.Read(ctx, e, f.pack.Topic, "")
	if err != nil {
		t.Fatal(err)
	}
	definition.Version = "rules-v2"
	definition.TopicVersion = publication.State.Version
	definition.PackDigest = publication.Digest
	ruleDraft, err = rules.Save(ctx, e, rulesets.SaveRequest{Expected: 1, Definition: definition, Change: "Review rules for changed aggregate"})
	if err != nil {
		t.Fatal(err)
	}
	ruleReview, err = rules.Review(ctx, e, f.pack.Topic, rulesets.ReviewRequest{DraftRevision: ruleDraft.Revision, Digest: ruleDraft.Digest, Decision: "approve", Note: "Reviewed changed topic rules"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rules.Publish(ctx, e, f.pack.Topic, rulesets.PublishRequest{Expected: 1, Review: ruleReview.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = query.RequalifyExample(ctx, e, request)
	if !errors.Is(err, readexec.ErrAnalyticalMismatch) {
		t.Fatal("executable SUM accepted under reviewed AVG", err)
	}
	stale := request
	stale.ExpectedVersion++
	if _, err = query.RequalifyExample(ctx, e, stale); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale original version accepted", err)
	}
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.nlq_examples SET origin=origin-'requalification' WHERE example_id=$1`, candidate.ID); err == nil {
		t.Fatal("immutable qualification origin removed")
	}
}

func TestSQLRecoveryOwnedExampleRequalificationAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := t.Context()
	base := `SELECT sum(amount) AS revenue FROM analytics.sales`
	f.model.mode.Store(phase18RawResponse(t, base))
	oldQuestion := ownedExampleQuestion(t, f, "Revenue named sales", nlq.LanguageEnglish, f.answer(t, "customer", cw01Text("primero")))
	oldPlan, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: oldQuestion})
	if err != nil {
		t.Fatal(err)
	}
	old := reviewOwnedExample(t, f, oldPlan)
	draftService, err := drafts.New(f.f.db, f.f.s, f.f.service)
	if err != nil {
		t.Fatal(err)
	}
	_, pubService := newPhase18Service(t, f.phase17Fixture)
	pack := cloneTopic(t, f.pack)
	pack.Version = "owned-requal-v2"
	pack.Description = "Re-reviewed unchanged owned metric"
	author := phase18Envelope(t, f.phase17Fixture, f.e.User(), f.f.e.Session(), true)
	previous, err := draftService.Read(ctx, author, pack.Topic, 0)
	if err != nil {
		t.Fatal("original author draft", err)
	}
	current, err := draftService.Save(ctx, author, drafts.SaveRequest{Expected: previous.Metadata.Revision, Pack: pack, Change: "Re-reviewed publication for owned template"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := pubService.Review(ctx, author, pack.Topic, topics.ReviewRequest{DraftRevision: current.Metadata.Revision, Digest: current.Metadata.Digest, Decision: "approve", Note: "Reviewed same value-free metric"})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := pubService.Publish(ctx, author, pack.Topic, topics.PublishRequest{Expected: f.published.State.Revision, Review: review.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.pack, f.published = pack, publication
	f.definition.Version = "owned-rules-v2"
	f.definition.TopicVersion = publication.State.Version
	f.definition.PackDigest = publication.Digest
	f.publishRules(t, f.definition, 1)
	currentQuestion := ownedExampleQuestion(t, f, "Revenue named sales", nlq.LanguageEnglish, f.answer(t, "customer", cw01Text("segundo")))
	request := nlqexec.ExampleRequalificationRequest{ExampleID: old.ID, ExpectedVersion: old.Version, Anchor: currentQuestion}
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.mu.Unlock()
	fresh, err := f.query.RequalifyExample(ctx, f.e, request)
	if err != nil {
		t.Fatal("current owned qualification", err)
	}
	if fresh.Origin.BindingPolicy != nlqexec.OwnedExamplePolicy || fresh.SQL != base || fresh.ParameterSchema != nil || fresh.State != "candidate" {
		t.Fatal("historical values entered template")
	}
	encoded, _ := json.Marshal(fresh)
	for _, private := range []string{"primero", "segundo", "cw-alpha-731", "cw-beta-731"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private qualification value persisted")
		}
	}
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if strings.Contains(wire, "cw-alpha-731") || strings.Contains(wire, "cw-beta-731") {
		t.Fatal("private current values entered provider")
	}
	if _, err = f.query.ExampleState(ctx, f.e, nlqexec.ExampleStateRequest{ExampleID: fresh.ID, ExpectedVersion: fresh.Version, State: "active", ReviewNote: "Reviewed current-only owned bindings"}); err != nil {
		t.Fatal(err)
	}
	plan, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: currentQuestion})
	if err != nil {
		t.Fatal(err)
	}
	requireOwnedExampleUse(t, f, plan, fresh.ID)
	requireExplanationSum(t, f.run(t, plan, 1, false), "5.5")
	wrong := request
	wrong.Anchor.Context = "foreign-context"
	if _, err = f.query.RequalifyExample(ctx, f.e, wrong); err == nil {
		t.Fatal("foreign context qualified")
	}
	other := phase18Envelope(t, f.phase17Fixture, f.e.User(), "foreign-session", true)
	if _, err = f.query.RequalifyExample(ctx, other, request); err == nil {
		t.Fatal("foreign pending answer origin qualified")
	}
	foreignTenant := f.model.token.envelope(t, "foreign-tenant", f.e.User(), phase18Scopes("foreign-tenant", true)...)
	if _, err = f.query.RequalifyExample(ctx, foreignTenant, request); err == nil {
		t.Fatal("foreign tenant qualified retained example")
	}
	withoutInspection := phase18Envelope(t, f.phase17Fixture, f.e.User(), f.e.Session(), false)
	if _, err = f.query.RequalifyExample(ctx, withoutInspection, request); err == nil {
		t.Fatal("missing inspection action qualified")
	}
}
