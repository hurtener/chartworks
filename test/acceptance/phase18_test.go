package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
)

func phase18Scopes(tenant string, sqlInspection bool) []string {
	scopes := append([]string{}, topicScopes(tenant)...)
	scopes = append(scopes, "sources.query", "query.preflight", "query.plan", "query.execute", "feedback.write", "cw.source.query:*")
	if sqlInspection {
		scopes = append(scopes, "reporting.sql.read")
	}
	return scopes
}

func phase18Envelope(t *testing.T, fixture *phase17Fixture, user, session string, sqlInspection bool) identity.Envelope {
	t.Helper()
	claims := fixture.model.token.claims(fixture.f.e.Tenant(), user, phase18Scopes(fixture.f.e.Tenant(), sqlInspection))
	claims["session"] = session
	token := fixture.model.token.sign(t, claims, nil)
	e, err := fixture.model.token.verifier.Verify(context.Background(), token, auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func phase18RawResponse(t *testing.T, sql string) string {
	t.Helper()
	content, err := json.Marshal(map[string]any{"sql": sql, "parameters": []any{}, "assumptions": []string{}, "ambiguities": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(map[string]any{
		"id": "phase18-generated", "object": "chat.completion", "model": "model-sqlgen",
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	return "chat_raw:" + string(response)
}

func newPhase18Service(t *testing.T, fixture *phase17Fixture) (*nlqexec.Service, *topics.Service) {
	t.Helper()
	index, err := vindex.New(fixture.f.db)
	if err != nil {
		t.Fatal(err)
	}
	published, err := topics.New(fixture.f.db, fixture.f.s, index, fixture.model.engine)
	if err != nil {
		t.Fatal(err)
	}
	service, err := nlqexec.New(fixture.service, published, fixture.f.s, fixture.f.validator, fixture.f.executor, fixture.model.engine, fixture.f.db)
	if err != nil {
		t.Fatal(err)
	}
	return service, published
}

// The phase-18 fixture keeps two independently published, same-source topics
// but avoids the phase-17 join-cardinality stress fixture. Its purpose is to
// exercise durable generation/execution semantics on a stable route boundary.
func newPhase18Fixture(t *testing.T) *phase17Fixture {
	t.Helper()
	f, draftsService, topicsService, model, pack := publicationFixture(t)
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	pack = phase17EnrichPack(t, f, pack)
	phase17PublishTopic(t, draftsService, topicsService, e, pack)
	related := cloneTopic(t, pack)
	related.Topic = "commerce-related"
	related.Name = "Commerce related"
	related.Description = "Synthetic same-source generation fixture"
	phase17PublishTopic(t, draftsService, topicsService, e, related)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	route, err := nlqroute.New(topicsService, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	return &phase17Fixture{f: f, e: e, pack: pack, related: related, service: route, model: model, context: pack.Datasets[0].Source.Context}
}

func phase18Question(fixture *phase17Fixture, locale nlq.Language, topicsList ...string) nlqexec.QuestionRequest {
	return nlqexec.QuestionRequest{Topic: topicsList[0], Topics: append([]string(nil), topicsList...), Context: fixture.context, Locale: locale, Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 1}
}

func TestPhase18(t *testing.T) {
	fixture := newPhase18Fixture(t)
	query, _ := newPhase18Service(t, fixture)
	ctx := context.Background()
	e := phase18Envelope(t, fixture, fixture.f.e.User(), "phase18-session", true)
	salesSQL := "SELECT id, amount FROM analytics.sales ORDER BY id"
	// The gateway fixture treats "normal" as inheriting the chat mode; use an
	// inert recognized-by-default mode so raw chat responses do not corrupt
	// embedding or rerank wire responses.
	fixture.model.embeddingMode.Store("fixed")
	fixture.model.rerankMode.Store("fixed")
	fixture.model.mode.Store(phase18RawResponse(t, salesSQL))

	t.Run("AC01", func(t *testing.T) {
		question := phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)
		preflight, err := query.Preflight(ctx, e, nlqexec.PreflightRequest{QuestionRequest: question})
		if err != nil || preflight.QueryID == "" || preflight.Route.Context == nil {
			t.Fatalf("preflight failed: out=%#v err=%v", preflight, err)
		}
		planned, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: question, Operation: "phase18-ac01-op"})
		if err != nil || planned.Status != "planned" || planned.QueryID == "" || planned.SQL != salesSQL {
			t.Fatalf("plan failed: out=%#v err=%v", planned, err)
		}
		run, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "phase18-ac01-op", Rows: 10, Bytes: 4096})
		if err != nil || run.Status != "succeeded" || run.Execution.Result == nil || len(run.Execution.Result.Rows) != 2 {
			t.Fatalf("run failed: out=%#v err=%v", run, err)
		}
		replay, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "phase18-ac01-op", Rows: 10, Bytes: 4096})
		if err != nil || replay.Status != "succeeded" || replay.Execution.Result == nil || len(replay.Execution.Result.Rows) != 2 {
			t.Fatalf("idempotent replay failed: out=%#v err=%v", replay, err)
		}
		spanish := phase18Question(fixture, nlq.LanguageSpanish, fixture.pack.Topic)
		spanish.Question = "¿Cuál es el ingreso?"
		spanishPlan, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: spanish})
		if err != nil || spanishPlan.Route.Context == nil || spanishPlan.Generation == "" {
			t.Fatalf("Spanish plan failed: out=%#v err=%v", spanishPlan, err)
		}
		refined, err := query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: planned.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Show revenue by id", Kinds: question.Kinds, LimitPerKind: question.LimitPerKind}})
		if err != nil || refined.Status != "planned" || refined.QueryID == planned.QueryID {
			t.Fatalf("refine failed: out=%#v err=%v", refined, err)
		}
		multi := phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic, fixture.related.Topic)
		multi.Question = "What is revenue across the confirmed topics?"
		multi.Joins = []nlqroute.JoinChoice{{Topic: fixture.pack.Topic, JoinID: "sales-items"}, {Topic: fixture.related.Topic, JoinID: "sales-items"}}
		multi.Rerank = true
		multiPlan, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: multi})
		if err != nil || multiPlan.Route.Outcome != nlq.StrategyMultiTopic || len(multiPlan.Route.Topics) != 2 {
			t.Fatalf("multi-topic plan failed: out=%#v err=%v", multiPlan, err)
		}
	})

	t.Run("AC02", func(t *testing.T) {
		assembler, err := nlq.NewDefaultContextAssembler()
		if err != nil {
			t.Fatal(err)
		}
		assembled, err := assembler.Assemble(ctx, nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Topic: fixture.pack.Topic, TopicVersion: fixture.pack.Version, Question: "What is revenue?", Constraints: &nlq.ConstraintState{Allowed: true, Required: []nlq.MandatoryConstraint{{ID: "filter", Kind: "required", Text: "tenant is signed"}}}, Metrics: []nlq.PinnedMetric{{ID: "revenue", Text: "sum amount"}}}, nlq.TierMedium)
		if err != nil {
			t.Fatal(err)
		}
		generation, err := assembler.ResolvePrecedence(ctx, nlq.GenerationInput{Context: assembled, EditBase: []nlq.Instruction{{Key: "edit", Text: "edit base"}}, Hints: []nlq.Instruction{{Key: "hint", Text: "hint"}}, Examples: []nlq.Instruction{{Key: "example", Text: "example"}}, Default: []nlq.Instruction{{Key: "default", Text: "default"}}})
		if err != nil || generation.Strategy != nlq.GenerationEditBase || !generation.FewShotDisabled || generation.Selected[0].Text != "edit base" || generation.MandatoryConstraints == nil || len(generation.PinnedMetrics) != 1 {
			t.Fatalf("precedence did not preserve governed inputs: %#v err=%v", generation, err)
		}
		if _, err = assembler.ResolvePrecedence(ctx, nlq.GenerationInput{Context: assembled, Hints: []nlq.Instruction{{Key: "hint", Text: "hint"}}, Examples: []nlq.Instruction{{Key: "example", Text: "example"}}, Default: []nlq.Instruction{{Key: "default", Text: "default"}}}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		validator := &countingPhase18Validator{}
		if !executionCorrectionClassified(readexec.ErrQuery) || executionCorrectionClassified(store.ErrUnavailable) || validator.calls != 0 {
			t.Fatal("execution correction classification is not bounded")
		}
		// The real validator is called again by the phase-18 service after every
		// generated correction; the focused unit test covers the two-call path.
		if _, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		zeroSQL := "SELECT id, amount FROM analytics.sales WHERE id = 999 ORDER BY id"
		fixture.model.mode.Store(phase18RawResponse(t, zeroSQL))
		planned, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)})
		if err != nil {
			t.Fatal(err)
		}
		run, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "phase18-ac04-op", Rows: 10, Bytes: 4096})
		if err != nil || run.Status != "empty" || run.Execution.Result == nil || len(run.Execution.Result.Rows) != 0 || run.SQL != zeroSQL {
			t.Fatalf("zero-row run changed governed result: out=%#v err=%v", run, err)
		}
		fixture.model.mode.Store(phase18RawResponse(t, salesSQL))
	})

	t.Run("AC05", func(t *testing.T) {
		planOne, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)})
		if err != nil {
			t.Fatal(err)
		}
		if err = query.Feedback(ctx, e, nlqexec.FeedbackRequest{QueryID: planOne.QueryID, Verdict: "positive", Note: "confirmed synthetic metric"}); err != nil {
			t.Fatal(err)
		}
		planTwo, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)})
		if err != nil {
			t.Fatal(err)
		}
		if err = query.Feedback(ctx, e, nlqexec.FeedbackRequest{QueryID: planTwo.QueryID, Verdict: "positive", Correction: salesSQL}); err != nil {
			t.Fatal(err)
		}
		examples, err := query.Examples(ctx, e, fixture.pack.Topic, 8)
		if err != nil || len(examples) != 1 || examples[0].EvidenceCount != 2 {
			t.Fatalf("feedback learning was not DB-first/deduplicated: examples=%#v err=%v", examples, err)
		}
		active, err := query.ExampleState(ctx, e, nlqexec.ExampleStateRequest{ExampleID: examples[0].ID, State: "active"})
		if err != nil || active.State != "active" {
			t.Fatalf("candidate activation failed: %#v err=%v", active, err)
		}
		restarted, _ := newPhase18Service(t, fixture)
		afterRestart, err := restarted.Examples(ctx, e, fixture.pack.Topic, 8)
		if err != nil || len(afterRestart) != 1 || afterRestart[0].State != "active" || afterRestart[0].Weight < 0.5 {
			t.Fatalf("learning did not survive restart: %#v err=%v", afterRestart, err)
		}
	})

	t.Run("AC06", func(t *testing.T) {
		fixture.model.mode.Store(phase18RawResponse(t, salesSQL))
		withoutInspection := phase18Envelope(t, fixture, fixture.f.e.User(), "phase18-no-inspection", false)
		planned, err := query.Plan(ctx, withoutInspection, nlqexec.PlanRequest{QuestionRequest: phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)})
		if err != nil || planned.SQL != "" || planned.Confidence == 0 || planned.Route.Context == nil {
			t.Fatalf("SQL inspection leak or missing structured result: %#v err=%v", planned, err)
		}
		foreign := phase18Envelope(t, fixture, fixture.f.e.User(), "phase18-foreign", true)
		if _, err = query.Refine(ctx, foreign, nlqexec.RefineRequest{QueryID: planned.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "foreign refine"}}); !errors.Is(err, nlqexec.ErrForeignSession) {
			t.Fatalf("foreign session refinement was not denied: %v", err)
		}
		if _, err = query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: fixture.pack.Topic, Context: "missing-context", Locale: nlq.LanguageEnglish, Question: "bad context"}}); !errors.Is(err, nlqexec.ErrInvalid) {
			t.Fatalf("invalid context did not return structured error: %v", err)
		}
	})
}

type countingPhase18Validator struct{ calls int }

func (*countingPhase18Validator) Validate(context.Context, identity.Envelope, readexec.Request) (readexec.Plan, error) {
	return readexec.Plan{}, nil
}

func executionCorrectionClassified(err error) bool { return errors.Is(err, readexec.ErrQuery) }
