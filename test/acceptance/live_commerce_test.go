package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/chartdata"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/gateway/bifrost"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/jackc/pgx/v5"
)

func TestCommerceSyntheticFixture(t *testing.T) {
	f := liveCommerceSource(t)
	main, distractor := liveCommerceTopics(t, f)
	if main.Topic == distractor.Topic || len(main.Datasets) != 4 || len(main.Joins) != 3 || len(main.KPIs) != 1 {
		t.Fatal("reviewed commerce fixture lost its grain or distractor")
	}
	var gross, refunded, naiveJoined string
	err := f.admin.QueryRow(t.Context(), `SELECT
  (SELECT sum(total_usd) FROM analytics.orders WHERE status='paid')::text,
  (SELECT sum(r.amount_usd) FROM analytics.refunds r JOIN analytics.orders o USING(order_id) WHERE o.status='paid')::text,
  (SELECT sum(o.total_usd) FROM analytics.orders o JOIN analytics.order_items i USING(order_id) JOIN analytics.refunds r USING(order_id) WHERE o.status='paid')::text`).Scan(&gross, &refunded, &naiveJoined)
	if err != nil || gross != "640.00" || refunded != "125.00" || naiveJoined == gross {
		t.Fatalf("synthetic grain baseline: gross=%s refunded=%s naive=%s err=%v", gross, refunded, naiveJoined, err)
	}
}

func TestCommerceReportingRecorded(t *testing.T) {
	f := liveCommerceSource(t)
	main, distractor := liveCommerceTopics(t, f)
	model := newGatewayFixture(t, func(cfg *config.Gateway) {
		embedding := cfg.Roles["embedding"]
		embedding.MaxBatchItems = 64
		embedding.MaxBatchBytes = 4096
		cfg.Roles["embedding"] = embedding
	})
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	draftService, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	topicService, err := topics.New(f.db, f.s, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	author := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	phase17PublishTopic(t, draftService, topicService, author, main)
	phase17PublishTopic(t, draftService, topicService, author, distractor)
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(topicService, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	query, err := nlqexec.New(router, topicService, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	model.mode.Store(phase18RawResponse(t, "SELECT SUM(total_usd) AS gross_revenue FROM analytics.orders WHERE status='paid'"))
	queryActor := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase18Scopes(f.e.Tenant(), true)...)
	for _, tc := range liveCommerceQuestions() {
		request := nlqroute.RouteRequest{Topic: main.Topic, Topics: []string{main.Topic}, Context: main.Datasets[0].Source.Context, Locale: tc.locale, Question: tc.text, Kinds: []string{"measure", "dimension", "kpi"}, LimitPerKind: 5, Rerank: true}
		if tc.metric != "" {
			request.MetricIDs = []string{tc.metric}
		}
		routeActor := author
		if strings.HasPrefix(tc.id, "observed-") || tc.id == "gross-es" {
			routeActor = queryActor
		}
		route, routeErr := router.Route(t.Context(), routeActor, request)
		if routeErr != nil {
			t.Fatalf("recorded commerce route %s: %v", tc.id, routeErr)
		}
		if tc.id == "underspecified" {
			if route.Clarification == nil || route.Clarification.Reason != "ambiguous_temporal_span" {
				t.Fatal("underspecified commerce question did not expose a typed period clarification")
			}
		} else if route.Outcome != nlq.StrategySingleTopic || route.Clarification != nil {
			t.Fatalf("commerce question %s did not reach the reviewed topic", tc.id)
		} else if route.Context == nil {
			t.Fatalf("commerce question %s lost current physical relation names from sealed context: has_context=%t", tc.id, route.Context != nil)
		} else if !strings.Contains(route.Context.Prompt, "analytics.orders") || !strings.Contains(route.Context.Prompt, "analytics.refunds") || !strings.Contains(route.Context.Prompt, "relation[") {
			t.Fatalf("commerce question %s physical context missing names: relations=%d orders=%t refunds=%t", tc.id, len(route.Context.Relations), strings.Contains(route.Context.Prompt, "analytics.orders"), strings.Contains(route.Context.Prompt, "analytics.refunds"))
		}
		if strings.HasPrefix(tc.id, "observed-") || tc.id == "gross-es" {
			end := "2027-01-01"
			if tc.id == "observed-range-net" {
				end = "2026-04-01"
			}
			if route.Interpretation == nil || len(route.Interpretation.Temporal) != 1 || route.Interpretation.Temporal[0].Start != "2026-01-01" || route.Interpretation.Temporal[0].End != end {
				t.Fatalf("natural commerce period %s lost reviewed half-open window: %#v", tc.id, route.Interpretation)
			}
		}
	}
	question := nlqexec.QuestionRequest{Topic: main.Topic, Topics: []string{main.Topic}, Context: main.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: "Gross revenue from paid orders?", MetricIDs: []string{"gross_revenue"}, Kinds: []string{"measure"}, LimitPerKind: 1, Rerank: true}
	planned, err := query.Plan(t.Context(), queryActor, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil {
		t.Fatalf("recorded Plan on real commerce source: %v", err)
	}
	scope, err := store.NewScope(queryActor.Tenant(), queryActor.User())
	if err != nil {
		t.Fatal(err)
	}
	retainedPlan, err := f.db.ReadQuery(t.Context(), scope, planned.QueryID)
	if err != nil || len(retainedPlan.RelationScope) != len(main.Datasets) {
		t.Fatalf("reviewed relation scope did not round trip durably: %v", err)
	}
	for _, relation := range retainedPlan.RelationScope {
		if relation.Dataset == "audit_totals" || slices.Contains(relation.Columns, "internal_code") {
			t.Fatal("unreviewed relation or column entered durable plan scope")
		}
	}
	run, err := query.Run(t.Context(), queryActor, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "commerce-recorded-run", Rows: 10, Bytes: 65536})
	if err != nil || run.Execution.Result == nil || !liveSingleNumericEquals(run.Execution.Result.Rows, "640") {
		t.Fatalf("recorded Run on real commerce source: %v status=%s", err, run.Status)
	}
	// The actor's wildcard dataset reach permits this registered same-source
	// relation at the read core, but the commerce topic never reviewed it.
	for _, extraSQL := range []string{"SELECT total_usd FROM analytics.audit_totals", "SELECT internal_code FROM analytics.orders"} {
		if _, err := f.validator.Validate(t.Context(), queryActor, readexec.Request{Source: main.Datasets[0].Source.Source, Context: main.Datasets[0].Source.Context, SQL: extraSQL}); err != nil {
			t.Fatalf("same-source scope negative needs an otherwise valid read: %v", err)
		}
		model.mode.Store(phase18RawResponse(t, extraSQL))
		if _, err := query.Plan(t.Context(), queryActor, nlqexec.PlanRequest{QuestionRequest: question}); err == nil {
			t.Fatal("NLQ planned an unreviewed same-source relation or column")
		}
	}
	model.mode.Store(phase18RawResponse(t, "SELECT SUM(total_usd) AS gross_revenue FROM analytics.orders WHERE status='paid'"))
	wrong := question
	wrong.Context = "wrong-context"
	if _, err := query.Plan(t.Context(), queryActor, nlqexec.PlanRequest{QuestionRequest: wrong}); err == nil {
		t.Fatal("wrong source context planned")
	}
	liveCommerceReporting(t, t.Context(), t.TempDir(), f, query, topicService, main, rendering.LocalProcessor{MaxBytes: 4 << 20})
}

func TestLiveCommerceGatewayConfig(t *testing.T) {
	g := liveGatewayConfig(t)
	if err := config.ValidateGateway(g, true); err != nil {
		t.Fatalf("live gateway role configuration: %v", err)
	}
	if g.Roles["embedding"].Model != "perplexity/pplx-embed-v1-0.6b" || g.Roles["embedding"].Dimensions != 1024 || g.Roles["sqlgen"].Model != "openai/gpt-6-luna" || g.Roles["rerank"].Provider != "openrouter-rerank" || g.Roles["rerank"].Model != "cohere/rerank-4-fast" || g.Roles["rerank"].OnFailure != "fail" {
		t.Fatal("live gateway roles drifted from the required paid gate")
	}
}

func TestLiveCommerceArtifactDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHARTWORKS_LIVE_ARTIFACT_DIR", dir)
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := liveArtifactDir(t); got != want {
		t.Fatalf("live artifact directory = %q, want %q", got, want)
	}
	checkout, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := filepath.Join(checkout, ".live-commerce-artifacts-forbidden")
	if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
		t.Fatal("forbidden artifact regression path already exists")
	}
	if _, err := checkedLiveArtifactDir(forbidden); err == nil {
		t.Fatal("checkout artifact path was accepted")
	}
	if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
		t.Fatal("rejected checkout artifact path was created")
	}
}

// This gate is deliberately absent from TestPhase25. It spends provider credits
// only when the operator opts in, and its receipt never claims release acceptance.
func TestLiveCommerceGatewayE2E(t *testing.T) {
	if os.Getenv("CHARTWORKS_LIVE_E2E") != "1" {
		t.Skip("set CHARTWORKS_LIVE_E2E=1 for the paid provider gate")
	}
	artifactDir := liveArtifactDir(t)
	engine := liveGateway(t)
	f := liveCommerceSource(t)
	main, distractor := liveCommerceTopics(t, f)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	draftService, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	topicService, err := topics.New(f.db, f.s, index, engine)
	if err != nil {
		t.Fatal(err)
	}
	author := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	mainPublication := phase17PublishTopic(t, draftService, topicService, author, main)
	distractorPublication := phase17PublishTopic(t, draftService, topicService, author, distractor)
	requireLiveModelCall(t, mainPublication.Receipt.Calls, "embedding", "openrouter", "perplexity/pplx-embed-v1-0.6b", "pplx-embed-v1-0.6b", "perplexity/pplx-embed-v1-0.6b")
	requireLiveModelCall(t, distractorPublication.Receipt.Calls, "embedding", "openrouter", "perplexity/pplx-embed-v1-0.6b", "pplx-embed-v1-0.6b", "perplexity/pplx-embed-v1-0.6b")
	writeLiveJSON(t, artifactDir, "embedding-receipt.json", map[string]any{"main": mainPublication.Receipt.Calls, "distractor": distractorPublication.Receipt.Calls, "space": engine.EmbeddingSpace()})
	writeLiveJSON(t, artifactDir, "rerank-receipt.json", liveRerankProbe(t, engine, author, main, distractor))
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(topicService, rules, index, engine)
	if err != nil {
		t.Fatal(err)
	}
	query, err := nlqexec.New(router, topicService, f.s, f.validator, f.executor, engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	queryActor := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase18Scopes(f.e.Tenant(), true)...)
	questions := liveCommerceQuestions()
	var receipts []liveReceipt
	defer func() { writeLiveJSON(t, artifactDir, "receipts.json", receipts) }()
	for _, tc := range questions {
		index := len(receipts)
		receipts = append(receipts, liveReceipt{Case: tc.id, Status: "not_completed"})
		t.Run(tc.id, func(t *testing.T) {
			request := nlqexec.QuestionRequest{Topic: main.Topic, Topics: []string{main.Topic}, Context: main.Datasets[0].Source.Context, Locale: tc.locale, Question: tc.text, Kinds: []string{"measure", "dimension", "kpi"}, LimitPerKind: 5, Rerank: true}
			if tc.metric != "" {
				request.MetricIDs = []string{tc.metric}
			}
			planned, err := query.Plan(ctx, queryActor, nlqexec.PlanRequest{QuestionRequest: request})
			if err != nil {
				var clarification *nlqroute.Clarification
				if tc.id == "underspecified" && errors.As(err, &clarification) {
					if clarification.Reason != "ambiguous_temporal_span" {
						t.Fatalf("unexpected underspecified clarification: %s", clarification.Reason)
					}
					receipts[index].Status = "clarification"
					receipts[index].Reason = clarification.Reason
					return
				}
				var binding *readexec.BusinessConstraintError
				if errors.As(err, &binding) {
					if tc.id == "observed-range-net" && binding.Code == "unsupported_select_shape" && binding.Field == "sql" {
						receipts[index].Status = "unsupported"
						receipts[index].Reason = binding.Code
						return
					}
					receipts[index].Status = "failed"
					receipts[index].Reason = binding.Code
					t.Fatalf("live plan (%s): constraint code=%s field=%s", tc.id, binding.Code, binding.Field)
				}
				receipts[index].Status = "failed"
				receipts[index].Reason = livePlanFailureReason(err)
				t.Fatalf("live plan (%s): %s", tc.id, receipts[index].Reason)
			}
			if tc.id == "observed-range-net" {
				t.Fatal("natural range net unexpectedly planned without reviewed aggregate binding")
			}
			if planned.Status != "planned" || planned.QueryID == "" || len(planned.Receipt.Calls) == 0 {
				t.Fatalf("missing validated live plan receipt: %s", tc.id)
			}
			requireLiveModelCall(t, planned.Receipt.Calls, "sqlgen", "openrouter", "openai/gpt-6-luna", "openai/gpt-6-luna", "gpt-6-luna")
			if tc.id == "underspecified" {
				t.Fatal("underspecified question planned without its required typed clarification")
			}
			run, err := query.Run(ctx, queryActor, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "live-" + tc.id, Rows: 100, Bytes: 1 << 20})
			if err != nil || run.Execution.Result == nil {
				t.Fatalf("live run (%s): %v status=%s", tc.id, err, run.Status)
			}
			if tc.wantNet && !liveSingleNumericEquals(run.Execution.Result.Rows, "515") {
				t.Fatal("net revenue did not preserve order/refund grain; expected 515 USD")
			}
			if (tc.id == "gross-en" || tc.id == "gross-es" || tc.id == "observed-year-en" || tc.id == "observed-year-es") && !liveRowsContainNumbers(run.Execution.Result.Rows, "80", "240", "320") {
				t.Fatal("monthly paid gross totals did not match the synthetic January-March fixture")
			}
			receipts[index] = liveReceipt{Case: tc.id, Status: run.Status, QueryID: planned.QueryID, RowCount: len(run.Execution.Result.Rows), ModelCalls: len(planned.Receipt.Calls), ModelUsage: planned.Receipt.Calls, RouteOutcome: string(planned.Route.Outcome), SourceStatus: run.Execution.Attempt.Status}
		})
	}
	// These requests must be denied; an error alone does not measure whether
	// an upstream model or source call was attempted.
	for _, negative := range []struct {
		name    string
		actor   identity.Envelope
		context string
	}{
		{"other-tenant", f.token.envelope(t, "other-tenant", "operator", phase18Scopes("other-tenant", true)...), main.Datasets[0].Source.Context},
		{"wrong-context", queryActor, "wrong-context"},
		{"missing-source-reach", f.token.envelope(t, f.e.Tenant(), f.e.User(), "query.plan", "topics.read", "cw.topic.read:"+main.Topic), main.Datasets[0].Source.Context},
	} {
		index := len(receipts)
		receipts = append(receipts, liveReceipt{Case: negative.name, Status: "not_completed"})
		t.Run(negative.name, func(t *testing.T) {
			_, err := query.Plan(ctx, negative.actor, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: main.Topic, Topics: []string{main.Topic}, Context: negative.context, Locale: nlq.LanguageEnglish, Question: "Gross revenue?", Kinds: []string{"measure"}, LimitPerKind: 1}})
			if err == nil {
				t.Fatal("authority or context negative unexpectedly planned")
			}
			receipts[index].Status = "denied"
		})
	}
	worker := os.Getenv("CHARTWORKS_LIVE_RENDERER")
	if !filepath.IsAbs(worker) {
		t.Fatal("CHARTWORKS_LIVE_RENDERER must name an absolute built chartworks-renderer executable")
	}
	processor, err := rendering.NewProcess(worker, liveCommerceRenderOptions())
	if err != nil {
		t.Fatalf("isolated renderer: %v", err)
	}
	liveCommerceReporting(t, ctx, artifactDir, f, query, topicService, main, processor)
}

func liveSingleNumericEquals(rows [][]json.RawMessage, want string) bool {
	if len(rows) != 1 || len(rows[0]) != 1 {
		return false
	}
	var value string
	if json.Unmarshal(rows[0][0], &value) != nil {
		value = string(rows[0][0])
	}
	actual, ok := new(big.Rat).SetString(value)
	if !ok {
		return false
	}
	expected, ok := new(big.Rat).SetString(want)
	return ok && actual.Cmp(expected) == 0
}

func liveRowsContainNumbers(rows [][]json.RawMessage, wants ...string) bool {
	if len(rows) != len(wants) {
		return false
	}
	remaining := map[string]bool{}
	for _, want := range wants {
		remaining[want] = true
	}
	for _, row := range rows {
		matched := ""
		for _, cell := range row {
			var value string
			if json.Unmarshal(cell, &value) != nil {
				value = string(cell)
			}
			n, ok := new(big.Rat).SetString(value)
			if !ok {
				continue
			}
			for want := range remaining {
				expected, _ := new(big.Rat).SetString(want)
				if n.Cmp(expected) == 0 {
					matched = want
					break
				}
			}
			if matched != "" {
				break
			}
		}
		if matched == "" {
			return false
		}
		delete(remaining, matched)
	}
	return len(remaining) == 0
}

func liveRerankProbe(t *testing.T, engine *bifrost.Engine, e identity.Envelope, main, distractor semantics.TopicPack) map[string]any {
	t.Helper()
	resources := []access.Resource{{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: main.Topic}, {Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: distractor.Topic}}
	call, err := gateway.Authorize(e, "topics.read", main.Datasets[0].Source.Context, resources...)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 1, Tokens: 32768, Duration: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// Put the distractor first so input-order preservation cannot pass the gate.
	candidates, err := gateway.AdmitCandidates(call, "topics.read", []gateway.Candidate{{ID: distractor.Topic, Text: distractor.Name + " " + distractor.Description, Resource: resources[1]}, {ID: main.Topic, Text: main.Name + " " + main.Description, Resource: resources[0]}})
	if err != nil {
		t.Fatal(err)
	}
	ranked, err := engine.Rerank(t.Context(), call, budget, "paid order net revenue after refunds", candidates)
	if err != nil || len(ranked.Items) != 2 || len(ranked.Receipt.Calls) == 0 || ranked.Receipt.Warning != "" {
		t.Fatalf("live SDK rerank receipt: %v", err)
	}
	seen := map[string]bool{}
	for _, item := range ranked.Items {
		if item.Score == nil {
			t.Fatal("rerank did not return provider scores")
		}
		seen[item.ID] = true
	}
	if !seen[main.Topic] || !seen[distractor.Topic] {
		t.Fatal("rerank lost an authorized candidate")
	}
	if ranked.Items[0].ID != main.Topic {
		t.Fatal("revenue question did not rank the commerce topic ahead of the retention distractor")
	}
	// A 0.01 relevance gap excludes ties and near-ties on the provider's 0..1 scale.
	if *ranked.Items[0].Score-*ranked.Items[1].Score < 0.01 {
		t.Fatal("commerce rerank margin below 0.01")
	}
	requireLiveModelCall(t, ranked.Receipt.Calls, "rerank", "openrouter-rerank", "cohere/rerank-4-fast", "rerank-v4.0-fast", "cohere/rerank-4-fast")
	return map[string]any{"usage": ranked.Receipt.Calls, "ranked": ranked.Items}
}

func requireLiveModelCall(t *testing.T, calls []gateway.Usage, role, provider, requested string, actual ...string) {
	t.Helper()
	observed := false
	for _, call := range calls {
		if call.Role != role {
			continue
		}
		if call.Provider != provider || call.RequestedModel != requested {
			t.Fatalf("%s receipt route/model drifted: provider=%q requested=%q", role, call.Provider, call.RequestedModel)
		}
		if !call.Cached && slices.Contains(actual, call.ActualModel) {
			observed = true
		}
	}
	if !observed {
		t.Fatalf("missing observed live %s provider/model receipt", role)
	}
}

type liveReceipt struct {
	Case         string          `json:"case"`
	Status       string          `json:"status"`
	Reason       string          `json:"reason,omitempty"`
	QueryID      string          `json:"query_id"`
	RowCount     int             `json:"row_count"`
	ModelCalls   int             `json:"model_calls"`
	ModelUsage   []gateway.Usage `json:"model_usage,omitempty"`
	RouteOutcome string          `json:"route_outcome"`
	SourceStatus string          `json:"source_status"`
}

type liveCommerceQuestion struct {
	id, text string
	locale   nlq.Language
	metric   string
	wantNet  bool
}

func liveCommerceQuestions() []liveCommerceQuestion {
	return []liveCommerceQuestion{
		{"gross-en", "What was gross revenue from paid orders by month? Return three month rows.", nlq.LanguageEnglish, "gross_revenue", false},
		{"gross-es", "¿Cuáles fueron los ingresos brutos de pedidos pagados por mes en 2026? Devuelve tres filas, una por mes.", nlq.LanguageSpanish, "gross_revenue", false},
		{"net-grain", "What is total net revenue in USD for all paid orders after subtracting every refund on those paid orders? Return one number.", nlq.LanguageEnglish, "net_revenue", true},
		{"observed-year-en", "What was gross revenue from paid orders by month in 2026?", nlq.LanguageEnglish, "gross_revenue", false},
		{"observed-year-es", "¿Cuáles fueron los ingresos brutos de pedidos pagados por mes en 2026?", nlq.LanguageSpanish, "gross_revenue", false},
		{"observed-range-net", "What is total net revenue in USD for all paid orders from January through March 2026, after subtracting every refund on those paid orders? Return one number.", nlq.LanguageEnglish, "net_revenue", true},
		{"underspecified", "How did sales do in January and February", nlq.LanguageEnglish, "", false},
	}
}

func livePlanFailureReason(err error) string {
	switch {
	case errors.Is(err, nlqexec.ErrValidationBudget):
		return "validation_budget"
	case errors.Is(err, readexec.ErrUnsafe):
		return "sql_safety"
	default:
		return "plan_error"
	}
}

func liveCommerceRenderOptions() rendering.Options {
	options := phase32Options()
	options.MaxTime = 30 * time.Second
	return options
}

func liveArtifactDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("CHARTWORKS_LIVE_ARTIFACT_DIR")
	realDir, err := checkedLiveArtifactDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return realDir
}

func checkedLiveArtifactDir(dir string) (string, error) {
	if !filepath.IsAbs(dir) || dir == "" {
		return "", errors.New("CHARTWORKS_LIVE_ARTIFACT_DIR must be an absolute external directory")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(dir))
	if err != nil {
		return "", errors.New("live artifact parent directory must exist")
	}
	candidate := filepath.Join(parent, filepath.Base(dir))
	if _, err := os.Lstat(dir); err == nil {
		candidate, err = filepath.EvalSymlinks(dir)
		if err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if !outsideCheckout(root, candidate) {
		return "", errors.New("live artifacts must be outside the Git checkout")
	}
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if !outsideCheckout(root, realDir) {
		return "", errors.New("live artifacts must be outside the Git checkout")
	}
	entries, err := os.ReadDir(realDir)
	if err != nil {
		return "", err
	}
	if len(entries) != 0 {
		return "", errors.New("live artifact directory must be empty to prevent stale evidence")
	}
	return realDir, nil
}

func outsideCheckout(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	return err == nil && rel != "." && (rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func writeLiveJSON(t *testing.T, dir, name string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func liveGateway(t *testing.T) *bifrost.Engine {
	t.Helper()
	g := liveGatewayConfig(t)
	engine, err := bifrost.New(t.Context(), g, os.LookupEnv, bifrost.TransportOptions{})
	if err != nil {
		t.Fatalf("live gateway construction: %v", err)
	}
	t.Cleanup(engine.Close)
	return engine
}

func liveGatewayConfig(t *testing.T) config.Gateway {
	t.Helper()
	raw, err := os.ReadFile("../../examples/chartworks.gateway.json")
	if err != nil {
		t.Fatal(err)
	}
	var excerpt struct {
		Gateway config.Gateway `json:"gateway"`
	}
	if err := json.Unmarshal(raw, &excerpt); err != nil {
		t.Fatal(err)
	}
	g := excerpt.Gateway
	g.Limits = config.DefaultGatewayLimits()
	g.Limits.Concurrency, g.Limits.TenantConcurrency = 2, 2
	g.Bifrost.Providers = []config.Provider{
		{Name: "openrouter", APIKey: "env:CHARTWORKS_OPENROUTER_API_KEY"},
		{Name: "openrouter-rerank", Type: "openrouter_rerank", APIKey: "env:CHARTWORKS_OPENROUTER_API_KEY"},
	}
	for _, role := range []string{"sqlgen", "sqlfix", "clarify"} {
		r := g.Roles[role]
		r.Model = "openai/gpt-6-luna"
		g.Roles[role] = r
	}
	r := g.Roles["rerank"]
	r.Provider = "openrouter-rerank"
	r.Model = "cohere/rerank-4-fast"
	r.OnFailure = "fail"
	g.Roles["rerank"] = r
	return g
}

func liveCommerceSource(t *testing.T) *engineeringFixture {
	t.Helper()
	f := newEngineeringFixture(t, nil, nil)
	raw, err := os.ReadFile("testdata/live_commerce.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(t.Context(), string(raw)); err != nil {
		t.Fatal("seed synthetic commerce", err)
	}
	if _, err := f.admin.Exec(t.Context(), "GRANT SELECT ON ALL TABLES IN SCHEMA analytics TO "+pgx.Identifier{f.role}.Sanitize()); err != nil {
		t.Fatal("grant synthetic reader", err)
	}
	cfg := f.cfg.Clone()
	cfg.Connections[0].Relations = []config.SourceRelation{
		config.SourceRelation{Schema: "analytics", Name: "customers", Columns: []string{"customer_id", "segment", "region"}},
		config.SourceRelation{Schema: "analytics", Name: "orders", Columns: []string{"order_id", "customer_id", "ordered_at", "total_usd", "status", "internal_code"}},
		config.SourceRelation{Schema: "analytics", Name: "order_items", Columns: []string{"item_id", "order_id", "category", "quantity", "amount_usd"}},
		config.SourceRelation{Schema: "analytics", Name: "refunds", Columns: []string{"refund_id", "order_id", "refunded_at", "amount_usd"}},
		config.SourceRelation{Schema: "analytics", Name: "audit_totals", Columns: []string{"id", "total_usd"}},
	}
	s, err := sources.New(f.db, cfg, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	validator, err := readexec.NewValidator(s, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	executor, err := readexec.NewExecutor(s, f.db, f.values.Exec)
	if err != nil {
		t.Fatal(err)
	}
	values := f.values
	values.Sources = cfg
	profiles, err := engineering.New(f.db, s, validator, executor, nil, values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(profiles.Close)
	f.s, f.validator, f.executor, f.service, f.cfg = s, validator, executor, profiles, cfg
	return f
}

func liveCommerceTopics(t *testing.T, f *engineeringFixture) (semantics.TopicPack, semantics.TopicPack) {
	t.Helper()
	source := f.create(t, "live-commerce")
	makeDataset := func(table, profileID string, fields []string) semantics.Dataset {
		binding, err := f.s.Binding(t.Context(), f.e, source.ID, source.ContextID)
		if err != nil {
			t.Fatal(err)
		}
		var datasetID string
		for _, relation := range binding.Relations {
			if relation.Name == table {
				datasetID = relation.ID
			}
		}
		if datasetID == "" {
			t.Fatalf("missing live relation %s", table)
		}
		profile := f.profile(t, engineering.ProfileSpec{ID: profileID, Source: source.ID, Context: source.ContextID, Dataset: datasetID, Columns: fields, SkipLLM: true}).Profile.Profile
		d := semantics.Dataset{ID: datasetID, Name: strings.ReplaceAll(table, "_", " "), Source: semantics.SourceReference{Source: source.ID, Context: source.ContextID, Dataset: datasetID, SourceRevision: source.Revision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
		for _, column := range profile.Schema {
			if !slices.Contains(fields, column.Name) {
				continue
			}
			c := semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable, Sensitivity: semantics.LiteralNonSensitive}
			switch column.Name {
			case "order_id", "item_id", "refund_id":
				c.SemanticRole = semantics.SemanticRoleFactKey
			case "customer_id":
				c.SemanticRole = semantics.SemanticRoleDimensionKey
			case "ordered_at", "refunded_at":
				c.SemanticRole = semantics.SemanticRoleEventTime
			case "total_usd", "amount_usd", "quantity":
				c.SemanticRole = semantics.SemanticRoleMeasureInput
			default:
				c.SemanticRole = semantics.SemanticRoleAttribute
			}
			d.Columns = append(d.Columns, c)
		}
		return d
	}
	orders := makeDataset("orders", "live-orders-profile", []string{"order_id", "customer_id", "ordered_at", "total_usd", "status"})
	items := makeDataset("order_items", "live-items-profile", []string{"item_id", "order_id", "category", "quantity", "amount_usd"})
	customers := makeDataset("customers", "live-customers-profile", []string{"customer_id", "segment", "region"})
	refunds := makeDataset("refunds", "live-refunds-profile", []string{"refund_id", "order_id", "refunded_at", "amount_usd"})
	col := func(d semantics.Dataset, id string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: id}
	}
	value := func(id, alias string) semantics.GovernedValue {
		return semantics.GovernedValue{ID: id, Value: id, Aliases: []string{alias}, Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "reviewed_profile", Evidence: "synthetic-commerce-v1", Policy: "low_cardinality"}}
	}
	main := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: "commerce-performance", Version: "v1", Name: "Commerce performance", Description: "Paid order revenue, refunds, products and customer segments at their reviewed grains.", Datasets: []semantics.Dataset{orders, items, customers, refunds},
		Measures: []semantics.Measure{
			{ID: "gross_revenue", Name: "Gross revenue", Description: "Sum paid order totals at order grain; exclude cancelled orders. Never sum after joining raw items or refunds.", Field: col(orders, "total_usd"), Aggregation: semantics.AggregationSum, Unit: "USD", Aliases: []string{"sales revenue", "ingresos brutos"}, Filters: []semantics.SemanticFilter{{ID: "paid-orders", Field: col(orders, "status"), Operator: "eq", Values: []string{"paid"}}}},
			{ID: "refund_amount", Name: "Refund amount", Description: "Sum refund events at refund grain; aggregate by order before combining with items.", Field: col(refunds, "amount_usd"), Aggregation: semantics.AggregationSum, Unit: "USD", Aliases: []string{"returns", "reembolsos"}},
			{ID: "units_sold", Name: "Units sold", Description: "Sum item quantities on paid orders at item grain.", Field: col(items, "quantity"), Aggregation: semantics.AggregationSum, Unit: "count", Aliases: []string{"unit volume", "unidades vendidas"}},
		},
		Dimensions: []semantics.Dimension{
			{ID: "order_month", Name: "Order date", Description: "Calendar month of the order", Field: col(orders, "ordered_at"), Role: semantics.DimensionTemporal, Aliases: []string{"sales month", "mes de pedido"}, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainQuarter}}},
			{ID: "region", Name: "Customer region", Field: col(customers, "region"), Role: semantics.DimensionCategorical, Geography: true, Aliases: []string{"sales territory", "región"}, Values: []semantics.GovernedValue{value("north", "norte"), value("south", "sur"), value("west", "oeste")}},
			{ID: "segment", Name: "Customer segment", Field: col(customers, "segment"), Role: semantics.DimensionCategorical, Aliases: []string{"client type", "segmento"}, Values: []semantics.GovernedValue{value("consumer", "consumidor"), value("business", "empresa")}},
			{ID: "category", Name: "Product category", Field: col(items, "category"), Role: semantics.DimensionCategorical, Aliases: []string{"product family", "categoría"}, Values: []semantics.GovernedValue{value("apparel", "ropa"), value("home", "hogar"), value("electronics", "electrónica")}},
		},
		KPIs: []semantics.KPI{{ID: "net_revenue", Name: "Net revenue", Description: "Paid gross revenue less refunds against those paid orders; aggregate each child at its own grain first.", Expression: "gross_revenue minus refund_amount", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "gross_revenue"}, {Kind: semantics.KindMeasure, ID: "refund_amount"}}, Unit: "USD", Aliases: []string{"net sales", "ingresos netos"}}},
		Joins: []semantics.Join{
			{ID: "orders-customers", Name: "Orders to customers", Left: col(orders, "customer_id"), Right: col(customers, "customer_id"), Type: semantics.JoinInner, Cardinality: semantics.CardinalityManyToOne, Evidence: semantics.RelationshipEvidence{ID: "synthetic-fk-1", LeftGrain: "order", RightGrain: "customer", Provenance: "declared_fk"}},
			{ID: "orders-items", Name: "Orders to items", Left: col(orders, "order_id"), Right: col(items, "order_id"), Type: semantics.JoinLeft, Cardinality: semantics.CardinalityOneToMany, Evidence: semantics.RelationshipEvidence{ID: "synthetic-fk-2", LeftGrain: "order", RightGrain: "item", Provenance: "declared_fk"}},
			{ID: "orders-refunds", Name: "Orders to refunds", Left: col(orders, "order_id"), Right: col(refunds, "order_id"), Type: semantics.JoinLeft, Cardinality: semantics.CardinalityOneToMany, Evidence: semantics.RelationshipEvidence{ID: "synthetic-fk-3", LeftGrain: "order", RightGrain: "refund", Provenance: "declared_fk"}},
		},
	}
	distractor := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: "customer-retention", Version: "v1", Name: "Customer retention", Description: "Customer cohorts and repeat ordering; no revenue metric is defined here.", Datasets: []semantics.Dataset{orders, customers},
		Measures:   []semantics.Measure{{ID: "order_count", Name: "Order count", Description: "Number of orders at order grain", Field: col(orders, "order_id"), Aggregation: semantics.AggregationCount, Unit: "count"}},
		Dimensions: []semantics.Dimension{{ID: "customer_segment", Name: "Customer segment", Field: col(customers, "segment"), Role: semantics.DimensionCategorical}},
		Joins:      []semantics.Join{{ID: "retention-orders-customers", Name: "Orders to customers", Left: col(orders, "customer_id"), Right: col(customers, "customer_id"), Type: semantics.JoinInner, Cardinality: semantics.CardinalityManyToOne}},
	}
	for _, p := range []semantics.TopicPack{main, distractor} {
		if _, err := semantics.Compile(p); err != nil {
			t.Fatalf("synthetic reviewed pack %s: %v", p.Topic, err)
		}
	}
	return main, distractor
}

func liveCommerceReporting(t *testing.T, ctx context.Context, artifactDir string, f *engineeringFixture, query *nlqexec.Service, topicService *topics.Service, topic semantics.TopicPack, processor rendering.Processor) {
	t.Helper()
	limits := config.DefaultReporting()
	blocks, err := reporting.New(f.db, topicService, f.s, f.validator, f.executor, reporting.CaptureFromQueries(query), limits)
	if err != nil {
		t.Fatal(err)
	}
	blockAuthor := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase27Scopes(f.e.Tenant())...)
	SQL := "SELECT status, SUM(total_usd) AS amount_usd FROM analytics.orders GROUP BY status ORDER BY status"
	binding := topic.Datasets[0].Source
	published, err := topicService.Read(ctx, blockAuthor, topic.Topic, topic.Version)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := f.validator.Validate(ctx, blockAuthor, readexec.Request{Source: binding.Source, Context: binding.Context, SQL: SQL})
	if err != nil {
		t.Fatalf("reviewed block validation: %v", err)
	}
	observed, err := f.executor.Execute(ctx, blockAuthor, plan, readexec.Options{Operation: "live-block-schema", Number: 1, Preview: true, Rows: 10, Bytes: 65536})
	if err != nil || observed.Result == nil {
		t.Fatalf("reviewed block schema: %v", err)
	}
	data, err := chartdata.FromReadResult(ctx, *observed.Result, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Columns) != 2 {
		t.Fatal("unexpected reviewed block columns")
	}
	table := charts.Mapping{Version: charts.Version, Kind: charts.Table, Columns: data.Columns, Bindings: charts.Bindings{Columns: []string{data.Columns[0].ID, data.Columns[1].ID}}, Options: charts.DefaultOptions()}
	bar := charts.Mapping{Version: charts.Version, Kind: charts.Bar, Columns: data.Columns, Bindings: charts.Bindings{Category: data.Columns[0].ID, Value: data.Columns[1].ID}, Options: charts.DefaultOptions()}
	for _, mapping := range []charts.Mapping{table, bar} {
		if err := charts.ValidateMapping(ctx, data, mapping, charts.Defaults()); err != nil {
			t.Fatalf("reviewed output mapping: %v", err)
		}
	}
	definition := reporting.Definition{SchemaVersion: reporting.SchemaVersion,
		Metadata: []reporting.Localized{{Locale: "en-US", Title: "Commerce by order state", Question: "How much did paid and cancelled orders total?", Description: "Synthetic retained evidence"}, {Locale: "es-AR", Title: "Comercio por estado", Question: "¿Cuánto sumaron los pedidos?"}},
		Source:   binding.Source, Context: binding.Context, Topics: []reporting.TopicPin{{Topic: topic.Topic, Version: topic.Version, Digest: published.Digest}}, SQL: SQL, ExpectedSchema: observed.Result.Schema,
		Outputs: []reporting.Output{{ID: "table-main", Kind: "table", Mapping: &table}, {ID: "bar-main", Kind: "chart", Mapping: &bar}},
	}
	created, err := blocks.Create(ctx, blockAuthor, reporting.CreateRequest{ID: "live-commerce-block", Definition: definition})
	if err != nil {
		t.Fatalf("create reviewed block: %v", err)
	}
	state, _ := phase27ValidatePublish(t, blocks, blockAuthor, created)
	runner, err := jobs.NewRequestRunner(f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reporting.NewRuns(blocks, f.db, runner, nil, "none", limits.Execution)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := reporting.NewDocuments(f.db, blocks, reporting.DocumentsFromQueries(query), limits)
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, f.db, runs, reporting.DocumentsFromQueries(query), runner)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := reporting.NewDelivery(blocks, runs, documents, compositions, f.db, limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	runActor := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase29RuntimeScopes(f.e.Tenant())...)
	blockRun, err := delivery.Run(ctx, runActor, reporting.DeliveryRunRequest{Target: reporting.DeliveryTarget{Kind: "block", ID: created.State.ID, Revision: state.PublishedRevision}, Key: "live-commerce-block-run", Outputs: []string{"table-main", "bar-main"}, Locale: "en-US", Timezone: "UTC"})
	if err != nil || blockRun.State != "succeeded" {
		t.Fatalf("retained block run: %s %v", blockRun.State, err)
	}
	reportAuthor := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase29AuthorScopes(f.e.Tenant())...)
	report := phase29Text("Synthetic commerce report")
	report.Widgets = append(report.Widgets, reporting.Widget{ID: "commerce", Kind: "block", Grid: reporting.GridCell{Row: 2, Width: 12, Height: 1}, Block: &reporting.BlockWidget{Block: created.State.ID, Outputs: []string{"table-main", "bar-main"}}})
	reportState, err := documents.Create(ctx, reportAuthor, "report", "live-commerce-report", report)
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	reportState = phase29Publish(t, documents, reportAuthor, reportState)
	reportRun, err := delivery.Run(ctx, runActor, reporting.DeliveryRunRequest{Target: reporting.DeliveryTarget{Kind: "report", ID: reportState.ID, Revision: reportState.PublishedRevision}, Key: "live-commerce-report-run", Locale: "en-US", Timezone: "UTC"})
	if err != nil || reportRun.State != "completed" {
		t.Fatalf("retained report run: %s %v", reportRun.State, err)
	}
	readActor := f.token.envelope(t, f.e.Tenant(), f.e.User(), "reporting.read", "reporting.export", "cw.block.read:*", "cw.report.read:*", "cw.run.read:*", "cw.run.export:*", "cw.execution_context.use:*")
	beforeRetainedRead := f.lookups.Load()
	tableRequest := reporting.DeliveryViewRequest{Kind: "report", Run: reportRun.Run, Page: "main", Widget: "commerce", Output: "table-main", Limit: 10}
	barRequest := reporting.DeliveryViewRequest{Kind: "report", Run: reportRun.Run, Page: "main", Widget: "commerce", Output: "bar-main", Limit: 10}
	tableView, err := delivery.View(ctx, readActor, tableRequest)
	if err != nil || tableView.Output == nil || tableView.Output.Table == nil || len(tableView.Output.Table.Rows) != 2 {
		t.Fatalf("retained report table view: %v", err)
	}
	barView, err := delivery.View(ctx, readActor, barRequest)
	if err != nil || barView.Output == nil || barView.Output.Chart == nil || len(barView.Output.Chart.Points) == 0 {
		t.Fatalf("retained report chart view: %v", err)
	}
	requireCommerceRetainedValues(t, tableView, barView)
	writeLiveJSON(t, artifactDir, "viewer-table.json", tableView)
	writeLiveJSON(t, artifactDir, "viewer-chart.json", barView)
	renderer, err := rendering.NewManaged(delivery, f.db, processor, 4<<20, liveCommerceRenderOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		format, filename string
		view             reporting.DeliveryViewRequest
	}{{"html", "report.html", tableRequest}, {"svg", "chart.svg", barRequest}} {
		request := rendering.Request{View: item.view, Format: item.format, Theme: "light", Width: 960, Height: 560}
		output, err := renderer.Generate(ctx, readActor, request)
		if err != nil || output.State != "succeeded" || output.Content == "" {
			t.Fatalf("static %s rendition: %v", item.format, err)
		}
		if strings.Contains(output.Content, "<script") {
			t.Fatal("static rendition contains script")
		}
		requireCommerceStaticValues(t, item.format, output.Content)
		if err := os.WriteFile(filepath.Join(artifactDir, item.filename), []byte(output.Content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if f.lookups.Load() != beforeRetainedRead {
		t.Fatal("retained viewer or renderer reopened the warehouse credential")
	}
	writeLiveJSON(t, artifactDir, "report-receipt.json", map[string]any{"block_run": blockRun.Run, "report_run": reportRun.Run, "block_state": blockRun.State, "report_state": reportRun.State, "table_rows": len(tableView.Output.Table.Rows), "chart_points": len(barView.Output.Chart.Points)})
}

func requireCommerceRetainedValues(t *testing.T, table, chart reporting.DeliveryViewResult) {
	t.Helper()
	if table.Output == nil || table.Output.Table == nil || chart.Output == nil || chart.Output.Chart == nil {
		t.Fatal("retained commerce outputs are missing")
	}
	view := table.Output.Table
	if len(view.Columns) != 2 || view.Columns[0].Name != "status" || view.Columns[1].Name != "amount_usd" || len(view.Rows) != 2 {
		t.Fatal("retained commerce table has the wrong schema or row count")
	}
	want := map[string]string{"paid": "640", "cancelled": "60"}
	for _, row := range view.Rows {
		if len(row) != 2 || row[0].Null || row[1].Null || !liveNumberEquals(row[1].Value, want[row[0].Value]) {
			t.Fatal("retained commerce table has an unexpected status or amount")
		}
		delete(want, row[0].Value)
	}
	if len(want) != 0 || len(chart.Output.Chart.Points) != 2 {
		t.Fatal("retained commerce table or chart omitted a status")
	}
	want = map[string]string{"paid": "640", "cancelled": "60"}
	for _, point := range chart.Output.Chart.Points {
		if point.Category.Null || point.Value.Null || !liveNumberEquals(point.Value.Exact, want[point.Category.Value]) {
			t.Fatal("retained commerce chart has an unexpected category or value")
		}
		delete(want, point.Category.Value)
	}
	if len(want) != 0 {
		t.Fatal("retained commerce chart omitted a category")
	}
}

func liveNumberEquals(value, want string) bool {
	if want == "" {
		return false
	}
	a, ok := new(big.Rat).SetString(value)
	if !ok {
		return false
	}
	b, ok := new(big.Rat).SetString(want)
	return ok && a.Cmp(b) == 0
}

func requireCommerceStaticValues(t *testing.T, format, content string) {
	t.Helper()
	trimmed := strings.TrimSpace(content)
	switch format {
	case "html":
		if !strings.HasPrefix(strings.ToLower(trimmed), "<!doctype html>") || !strings.Contains(trimmed, "<table>") {
			t.Fatal("retained HTML is not a populated static table")
		}
	case "svg":
		if !strings.HasPrefix(trimmed, "<svg ") || !strings.HasSuffix(trimmed, "</svg>") || !strings.Contains(trimmed, "data-kind=\"bar\"") {
			t.Fatal("retained SVG is not a populated static bar chart")
		}
	default:
		t.Fatal("unexpected static format")
	}
	for _, value := range []string{"paid", "cancelled", "640", "60"} {
		if !strings.Contains(trimmed, value) {
			t.Fatalf("static %s omitted synthetic commerce label or value %q", format, value)
		}
	}
}
