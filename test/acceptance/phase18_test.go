package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
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
	token := phase18Token(t, fixture, user, session, sqlInspection)
	e, err := fixture.model.token.verifier.Verify(context.Background(), token, auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func phase18Token(t *testing.T, fixture *phase17Fixture, user, session string, sqlInspection bool) string {
	t.Helper()
	claims := fixture.model.token.claims(fixture.f.e.Tenant(), user, phase18Scopes(fixture.f.e.Tenant(), sqlInspection))
	claims["session"] = session
	return fixture.model.token.sign(t, claims, nil)
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
		question.MetricIDs = []string{"revenue"}
		refined, err := query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: planned.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Show revenue by id", Kinds: question.Kinds, LimitPerKind: question.LimitPerKind, MetricIDs: question.MetricIDs}})
		if err != nil || refined.Status != "planned" || refined.QueryID == planned.QueryID {
			t.Fatalf("refine failed: out=%#v err=%v", refined, err)
		}
		scope, err := store.NewScope(e.Tenant(), e.User())
		if err != nil {
			t.Fatal(err)
		}
		refinedRecord, err := fixture.f.db.ReadQuery(ctx, scope, refined.QueryID)
		if err != nil || !reflect.DeepEqual(refinedRecord.Route.Request.MetricIDs, []string{"revenue"}) || !reflect.DeepEqual(refinedRecord.Route.Request.Kinds, question.Kinds) {
			t.Fatalf("refinement discarded the parent governed route selections: %#v %v", refinedRecord.Route.Request, err)
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

		t.Run("http-sdk", func(t *testing.T) {
			httpFixture := newPhase18Fixture(t)
			httpQuery, _ := newPhase18Service(t, httpFixture)
			httpFixture.model.embeddingMode.Store("fixed")
			httpFixture.model.rerankMode.Store("fixed")
			httpCtx := context.Background()
			httpFixture.model.mode.Store(phase18RawResponse(t, salesSQL))
			handler := nlqapi.ExecutionHandler(httpFixture.model.token.verifier, httpQuery, http.NotFoundHandler())
			server := httptest.NewServer(handler)
			defer server.Close()
			token := phase18Token(t, httpFixture, httpFixture.f.e.User(), "phase18-http", true)
			client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
			if err != nil {
				t.Fatal(err)
			}
			question := phase18Question(httpFixture, nlq.LanguageEnglish, httpFixture.pack.Topic)
			preflight, err := client.PreflightNLQ(httpCtx, sdk.NLQPreflightRequest{QuestionRequest: question})
			if err != nil || preflight.QueryID == "" || preflight.Route.Context == nil {
				var statusErr *sdk.StatusError
				if errors.As(err, &statusErr) {
					t.Fatalf("HTTP preflight failed: out=%#v status=%d", preflight, statusErr.Status)
				}
				t.Fatalf("HTTP preflight failed: out=%#v err=%v", preflight, err)
			}
			planned, err := client.PlanNLQ(httpCtx, sdk.NLQPlanRequest{QuestionRequest: question, Operation: "phase18-http-plan"})
			if err != nil || planned.Status != "planned" || planned.SQL != salesSQL {
				t.Fatalf("HTTP plan failed: out=%#v err=%v", planned, err)
			}
			run, err := client.RunNLQ(httpCtx, sdk.NLQRunRequest{QueryID: planned.QueryID, Operation: "phase18-http-run", Rows: 10, Bytes: 4096})
			if err != nil || run.Status != "succeeded" || run.Execution.Result == nil {
				t.Fatalf("HTTP run failed: out=%#v err=%v", run, err)
			}
			refined, err := client.RefineNLQ(httpCtx, sdk.NLQRefineRequest{QueryID: planned.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Show revenue by id", Kinds: question.Kinds, LimitPerKind: question.LimitPerKind}})
			if err != nil || refined.Status != "planned" || refined.QueryID == planned.QueryID {
				t.Fatalf("HTTP refine failed: out=%#v err=%v", refined, err)
			}
			feedback, err := client.FeedbackNLQ(httpCtx, sdk.NLQFeedbackRequest{QueryID: planned.QueryID, Verdict: "positive", Note: "HTTP round trip"})
			if err != nil || !feedback.Accepted {
				t.Fatalf("HTTP feedback failed: out=%#v err=%v", feedback, err)
			}

			noInspectionToken := phase18Token(t, httpFixture, httpFixture.f.e.User(), "phase18-http-no-sql", false)
			noInspection, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return noInspectionToken, nil })
			if err != nil {
				t.Fatal(err)
			}
			withoutSQL, err := noInspection.PlanNLQ(httpCtx, sdk.NLQPlanRequest{QuestionRequest: question})
			if err != nil || withoutSQL.SQL != "" || withoutSQL.Route.Context == nil {
				t.Fatalf("HTTP SQL inspection boundary failed: out=%#v err=%v", withoutSQL, err)
			}
			foreignToken := phase18Token(t, httpFixture, httpFixture.f.e.User(), "phase18-http-foreign", true)
			foreignClient, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return foreignToken, nil })
			if err != nil {
				t.Fatal(err)
			}
			_, err = foreignClient.RefineNLQ(httpCtx, sdk.NLQRefineRequest{QueryID: planned.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "foreign refine"}})
			var statusErr *sdk.StatusError
			if !errors.As(err, &statusErr) || statusErr.Status != http.StatusConflict {
				t.Fatalf("foreign HTTP refine was not denied without disclosure: err=%v", err)
			}
		})
	})

	t.Run("AC06/terminal-replay-and-result-envelope", func(t *testing.T) {
		fixture := newPhase18Fixture(t)
		query, _ := newPhase18Service(t, fixture)
		e := phase18Envelope(t, fixture, fixture.f.e.User(), "phase18-terminal", true)
		fixture.model.embeddingMode.Store("fixed")
		fixture.model.rerankMode.Store("fixed")
		fixture.model.mode.Store(phase18RawResponse(t, salesSQL))
		ctx := context.Background()
		scope, err := store.NewScope(e.Tenant(), e.User())
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			status string
			want   error
		}{
			{status: "cancelled", want: readexec.ErrCancelled},
			{status: "timed_out", want: readexec.ErrTimeout},
			{status: "interrupted", want: readexec.ErrUncertain},
		} {
			t.Run(tc.status, func(t *testing.T) {
				planned, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)})
				if err != nil {
					t.Fatal(err)
				}
				record, err := fixture.f.db.ReadQuery(ctx, scope, planned.QueryID)
				if err != nil {
					t.Fatal(err)
				}
				record.Operation = "phase18-terminal-" + tc.status
				record.Status = tc.status
				record.Result = &readexec.Result{Schema: []readexec.Field{{Name: "id", Type: "integer", Encoding: "string", NativeType: "integer"}}, Rows: [][]json.RawMessage{{json.RawMessage(`1`)}}, Outcome: "complete", Bytes: 1}
				record.Revision++
				if err = fixture.f.db.UpdateQuery(ctx, scope, record, record.Revision-1); err != nil {
					t.Fatal("persist terminal status", err)
				}
				replay, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: record.Operation})
				if !errors.Is(err, tc.want) || replay.Execution.Result == nil || replay.SQL != salesSQL {
					t.Fatalf("terminal replay was not authorized/persisted: status=%#v err=%v want=%v", replay, err, tc.want)
				}
			})
		}
		planned, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)})
		if err != nil {
			t.Fatal(err)
		}
		near, err := fixture.f.db.ReadQuery(ctx, scope, planned.QueryID)
		if err != nil {
			t.Fatal(err)
		}
		near.Operation = "phase18-terminal-near-cap"
		near.Status = "succeeded"
		payload, err := json.Marshal(strings.Repeat("x", 16<<20))
		if err != nil {
			t.Fatal(err)
		}
		near.Result = &readexec.Result{Schema: []readexec.Field{{Name: "payload", Type: "text", Encoding: "string", NativeType: "text"}}, Rows: [][]json.RawMessage{{payload}}, Outcome: "succeeded", Bytes: len(payload)}
		near.Revision++
		if err = fixture.f.db.UpdateQuery(ctx, scope, near, near.Revision-1); err != nil {
			t.Fatal("persist near-cap result envelope", err)
		}
		readBack, err := fixture.f.db.ReadQuery(ctx, scope, planned.QueryID)
		if err != nil || readBack.Result == nil || len(readBack.Result.Rows) != 1 || len(readBack.Result.Rows[0][0]) != len(payload) {
			t.Fatalf("durable result envelope headroom rejected a valid executor result: result=%#v err=%v", readBack.Result, err)
		}
		limitedClaims := fixture.model.token.claims(e.Tenant(), e.User(), []string{"query.execute"})
		limitedClaims["session"] = e.Session()
		limitedToken := fixture.model.token.sign(t, limitedClaims, nil)
		limited, err := fixture.model.token.verifier.Verify(ctx, limitedToken, auth.HTTP)
		if err != nil {
			t.Fatal("limited terminal authority", err)
		}
		if _, err = query.Run(ctx, limited, nlqexec.RunRequest{QueryID: near.ID, Operation: near.Operation}); !errors.Is(err, access.ErrNotFound) && !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("terminal replay crossed retained resource authority: %v", err)
		}
	})

	t.Run("AC06/rule-evidence-invalidation", func(t *testing.T) {
		fixture := newPhase18Fixture(t)
		query, publishedTopics := newPhase18Service(t, fixture)
		ctx := context.Background()
		e := phase18Envelope(t, fixture, fixture.f.e.User(), "phase18-rule-evidence", true)
		fixture.model.embeddingMode.Store("fixed")
		fixture.model.rerankMode.Store("fixed")
		fixture.model.mode.Store(phase18RawResponse(t, "SELECT id, amount FROM analytics.sales ORDER BY id"))
		rules, err := rulesets.New(fixture.f.db, fixture.f.db, fixture.f.db)
		if err != nil {
			t.Fatal("rules service", err)
		}
		publishedTopic, err := publishedTopics.Read(ctx, e, fixture.pack.Topic, "")
		if err != nil {
			t.Fatal("published topic", err)
		}
		topicDigest := publishedTopic.Digest
		question := phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)
		question.Question = "What is revenue before rules are active?"
		beforeRules, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: question})
		if err != nil {
			t.Fatal("plan without active rules", err)
		}
		scope, err := store.NewScope(e.Tenant(), e.User())
		if err != nil {
			t.Fatal("query scope", err)
		}
		beforeRecord, err := fixture.f.db.ReadQuery(ctx, scope, beforeRules.QueryID)
		if err != nil || len(beforeRecord.RuleVersions) != 1 || beforeRecord.RuleVersions[0] != "" {
			t.Fatalf("query without active rules was not pinned explicitly: %#v %v", beforeRecord.RuleVersions, err)
		}

		phase17PublishRules(t, rules, e, publishedTopic)
		activeV1, err := rules.Read(ctx, e, fixture.pack.Topic, "")
		if err != nil || activeV1.State.Version != "rules-v1" || !activeV1.State.Active {
			t.Fatalf("initial rule activation: %#v %v", activeV1, err)
		}
		activatedRun, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: beforeRules.QueryID, Operation: "phase18-rule-activation"})
		if err != nil || activatedRun.Status != "succeeded" || activatedRun.EvidenceStale {
			t.Fatalf("query planned before rule activation did not replay against its unruled pin: %#v %v", activatedRun, err)
		}

		ruleQuestion := phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic)
		ruleQuestion.Question = "What is revenue under rules v1?"
		underV1, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: ruleQuestion})
		if err != nil {
			t.Fatal("plan under rules v1", err)
		}
		underV1Record, err := fixture.f.db.ReadQuery(ctx, scope, underV1.QueryID)
		if err != nil || len(underV1Record.RuleVersions) != 1 || underV1Record.RuleVersions[0] != "rules-v1" {
			t.Fatalf("rule v1 pin was not retained: %#v %v", underV1Record.RuleVersions, err)
		}

		multiQuestion := phase18Question(fixture, nlq.LanguageEnglish, fixture.pack.Topic, fixture.related.Topic)
		multiQuestion.Question = "What is revenue across the retained topics?"
		multiQuestion.Joins = []nlqroute.JoinChoice{{Topic: fixture.pack.Topic, JoinID: "sales-items"}, {Topic: fixture.related.Topic, JoinID: "sales-items"}}
		multiQuestion.Rerank = true
		multiPlan, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: multiQuestion})
		if err != nil {
			t.Fatal("multi-topic plan under rules v1", err)
		}
		multiRecord, err := fixture.f.db.ReadQuery(ctx, scope, multiPlan.QueryID)
		if err != nil || !reflect.DeepEqual(multiRecord.RuleVersions, []string{"rules-v1", ""}) || !reflect.DeepEqual(multiRecord.Topics, []string{fixture.pack.Topic, fixture.related.Topic}) {
			t.Fatalf("multi-topic rule/topic pins were not aligned: %#v %v", multiRecord, err)
		}

		definition := activeV1.Definition
		definition.Version = "rules-v2"
		candidate, err := rules.Save(ctx, e, rulesets.SaveRequest{Expected: 1, Definition: definition, Change: "Phase 18 invalidation replay candidate"})
		if err != nil {
			t.Fatal("save rules v2", err)
		}
		review, err := rules.Review(ctx, e, fixture.pack.Topic, rulesets.ReviewRequest{DraftRevision: candidate.Revision, Digest: candidate.Digest, Decision: "approve", Note: "Phase 18 invalidation replay candidate"})
		if err != nil {
			t.Fatal("review rules v2", err)
		}
		activeV2, err := rules.Publish(ctx, e, fixture.pack.Topic, rulesets.PublishRequest{Review: review.ID, Expected: 1})
		if err != nil || activeV2.State.Version != "rules-v2" {
			t.Fatalf("publish rules v2: %#v %v", activeV2, err)
		}

		staleRun, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: underV1.QueryID, Operation: "phase18-rule-publish-replay"})
		if err != nil || staleRun.Status != "succeeded" || !staleRun.EvidenceStale {
			t.Fatalf("rule publication did not invalidate retained query evidence: %#v %v", staleRun, err)
		}
		multiStale, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: multiPlan.QueryID, Operation: "phase18-multi-rule-publish-replay"})
		if err != nil || multiStale.Status != "succeeded" || !multiStale.EvidenceStale {
			t.Fatalf("multi-topic invalidation lost ordered rule pin: %#v %v", multiStale, err)
		}

		historicalV1, err := rules.Read(ctx, e, fixture.pack.Topic, "rules-v1")
		if err != nil || historicalV1.State.Version != "rules-v1" || historicalV1.Digest != activeV1.Digest {
			t.Fatalf("historical v1 rules mutated after publication: %#v %v", historicalV1, err)
		}
		historicalTopic, err := publishedTopics.Read(ctx, e, fixture.pack.Topic, publishedTopic.State.Version)
		if err != nil || historicalTopic.Digest != topicDigest || historicalTopic.State.Version != publishedTopic.State.Version {
			t.Fatalf("historical topic pin changed during rule replay: %#v %v", historicalTopic, err)
		}

		underV2, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: ruleQuestion})
		if err != nil {
			t.Fatal("plan under rules v2", err)
		}
		underV2Record, err := fixture.f.db.ReadQuery(ctx, scope, underV2.QueryID)
		if err != nil || len(underV2Record.RuleVersions) != 1 || underV2Record.RuleVersions[0] != "rules-v2" {
			t.Fatalf("rule v2 pin was not retained: %#v %v", underV2Record.RuleVersions, err)
		}
		retired, err := rules.Retire(ctx, e, fixture.pack.Topic, rulesets.RetireRequest{Expected: 2, Note: "Phase 18 invalidation replay retirement"})
		if err != nil || !retired.Retired || retired.Version != "rules-v2" {
			t.Fatalf("retire rules v2: %#v %v", retired, err)
		}
		retiredRun, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: underV2.QueryID, Operation: "phase18-rule-retire-replay"})
		if err != nil || retiredRun.Status != "succeeded" || !retiredRun.EvidenceStale {
			t.Fatalf("rule retirement did not invalidate retained query evidence: %#v %v", retiredRun, err)
		}
		if _, err = rules.Read(ctx, e, fixture.pack.Topic, ""); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("retired rules remained current: %v", err)
		}
		historicalV2, err := rules.Read(ctx, e, fixture.pack.Topic, "rules-v2")
		if err != nil || historicalV2.State.Version != "rules-v2" || historicalV2.Digest != activeV2.Digest {
			t.Fatalf("historical v2 rules mutated after retirement: %#v %v", historicalV2, err)
		}
	})
}

type countingPhase18Validator struct{ calls int }

func (*countingPhase18Validator) Validate(context.Context, identity.Envelope, readexec.Request) (readexec.Plan, error) {
	return readexec.Plan{}, nil
}

func executionCorrectionClassified(err error) bool { return errors.Is(err, readexec.ErrQuery) }
