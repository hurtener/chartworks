package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func recordedAdversarialNetStep(t *testing.T, columns []semantics.Reference, datasets map[string]string, last bool, confirmed ...bool) string {
	t.Helper()
	var envelope map[string]any
	if json.Unmarshal([]byte(strings.TrimPrefix(recordedAdversarialStep(t, columns, datasets, len(confirmed) == 0 || confirmed[0]), "chat_raw:")), &envelope) != nil {
		t.Fatal("net recorded envelope")
	}
	message := envelope["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	var body map[string]any
	if json.Unmarshal([]byte(message["content"].(string)), &body) != nil {
		t.Fatal("net recorded body")
	}
	gross := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["orders"], "misleading_net_total_usd")
	refunds := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["refunds"], "amount_usd")
	for _, field := range columns {
		if (len(confirmed) == 0 || confirmed[0]) && field.Dataset == datasets["refunds"] && field.ID == "amount_usd" {
			body["filter_proposals"] = append(body["filter_proposals"].([]any), map[string]any{"measure": refunds, "id": "paid_parent", "operator": "eq", "nulls": "exclude", "vocabulary_ids": []string{"orders_paid"}, "join_id": "confirmed_refunds_to_orders"})
		}
	}
	for _, field := range columns {
		if field.Dataset == datasets["refunds"] && (field.ID == "refund_id" || field.ID == "amount_usd") {
			name := "Posted refund event count"
			if field.ID == "amount_usd" {
				name = "Known posted refund amount count"
			}
			count := drafts.CountProposal{Dataset: field.Dataset, Column: field.ID, Name: name, Description: "Counts non-NULL values for the exact governed refund population, without deduplication", Aliases: []string{}, Unit: "refunds"}
			body["count_proposals"] = append(body["count_proposals"].([]any), count)
			id := drafts.GeneratedCountMeasureID(field.Dataset, field.ID)
			body["filter_proposals"] = append(body["filter_proposals"].([]any), map[string]any{"measure": id, "id": "posted_refund_count", "operator": "eq", "nulls": "exclude", "vocabulary_ids": []string{"refunds_posted"}})
			if len(confirmed) == 0 || confirmed[0] {
				body["filter_proposals"] = append(body["filter_proposals"].([]any), map[string]any{"measure": id, "id": "paid_parent_count", "operator": "eq", "nulls": "exclude", "vocabulary_ids": []string{"orders_paid"}, "join_id": "confirmed_refunds_to_orders"})
			}
		}
	}
	if len(confirmed) == 0 || confirmed[0] {
		for _, field := range columns {
			if field.Dataset == datasets["orders"] && field.ID == "misleading_net_total_usd" {
				rowCount := drafts.GeneratedCountMeasureID(field.Dataset, "order_id")
				knownCount := drafts.GeneratedCountMeasureID(field.Dataset, "misleading_net_total_usd")
				companion := semantics.KPI{ID: "unknown_order_amounts_in_scope", Name: "Unknown paid order amounts in the current gross scope", Description: "Scope-inheriting count of non-NULL order identities minus known amount count under the same paid population; no independent period", Expression: rowCount + " - " + knownCount, Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: rowCount}, {Kind: semantics.KindMeasure, ID: knownCount}}}
				body["kpis"] = []semantics.KPI{companion}
				for _, raw := range body["results"].([]any) {
					result := raw.(map[string]any)
					if result["dataset"] == field.Dataset && result["column"] == field.ID {
						result["completeness"] = semantics.KnownAmountCompleteness{Policy: semantics.KnownAmountCompletenessPolicy, UnknownCount: semantics.Reference{Kind: semantics.KindKPI, ID: companion.ID}}
					}
				}
			}
		}
	}
	if last {
		orderTime := semantics.GeneratedEntityID(semantics.EnhancementDimension, datasets["orders"], "ordered_at")
		refundTime := semantics.GeneratedEntityID(semantics.EnhancementDimension, datasets["refunds"], "refunded_at")
		kpis, _ := body["kpis"].([]semantics.KPI)
		if kpis == nil {
			kpis = []semantics.KPI{}
		}
		for _, meaning := range []struct{ id, name, axis string }{{"known_cohort_net", "Order-cohort net known value", orderTime}, {"known_activity_net", "Refund-activity net known value", refundTime}} {
			kpis = append(kpis, semantics.KPI{ID: meaning.id, Name: meaning.name, Description: "Known paid gross less known posted refunds on paid parent orders under this independently reviewed period mapping", Expression: gross + " - " + refunds, Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: gross}, {Kind: semantics.KindMeasure, ID: refunds}}, Periods: &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: []semantics.MetricPeriodBinding{{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: gross}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: orderTime}}, {Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: refunds}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: meaning.axis}}}}})
		}
		for _, companion := range []struct{ id, name, dataset, identity, amount, axis string }{
			{"unknown_order_amounts", "Unknown paid order amount count", datasets["orders"], "order_id", "misleading_net_total_usd", orderTime},
			{"unknown_cohort_refund_amounts", "Unknown cohort refund amount count", datasets["refunds"], "refund_id", "amount_usd", orderTime},
			{"unknown_activity_refund_amounts", "Unknown activity refund amount count", datasets["refunds"], "refund_id", "amount_usd", refundTime},
		} {
			all := drafts.GeneratedCountMeasureID(companion.dataset, companion.identity)
			known := drafts.GeneratedCountMeasureID(companion.dataset, companion.amount)
			bindings := []semantics.MetricPeriodBinding{}
			for _, id := range []string{all, known} {
				bindings = append(bindings, semantics.MetricPeriodBinding{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: id}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: companion.axis}})
			}
			kpis = append(kpis, semantics.KPI{ID: companion.id, Name: companion.name, Description: "Count non-NULL fact identifiers minus count known amounts under exactly the same reviewed population and period; preserve zero for no unknown events", Expression: all + " - " + known, Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: all}, {Kind: semantics.KindMeasure, ID: known}}, Periods: &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: bindings}})
		}
		body["kpis"] = kpis
	}
	raw, _ := json.Marshal(body)
	message["content"] = string(raw)
	raw, _ = json.Marshal(envelope)
	return "chat_raw:" + string(raw)
}

// This is the synthetic operator's review, separate from model proposals and
// advisory. It accepts exact reviewed definitions or stops; it repairs nothing.
func reviewGeneratedAdversarialNet(t *testing.T, pack semantics.TopicPack, datasets map[string]string) {
	t.Helper()
	gross := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["orders"], "misleading_net_total_usd")
	refunds := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["refunds"], "amount_usd")
	orderTime := semantics.GeneratedEntityID(semantics.EnhancementDimension, datasets["orders"], "ordered_at")
	refundTime := semantics.GeneratedEntityID(semantics.EnhancementDimension, datasets["refunds"], "refunded_at")
	if len(pack.KPIs) != 6 {
		t.Fatal("operator rejects missing/extra net or companion meanings")
	}
	for _, kpi := range pack.KPIs {
		left, right, axis := gross, refunds, orderTime
		switch kpi.ID {
		case "known_cohort_net":
		case "known_activity_net":
			axis = refundTime
		case "unknown_order_amounts_in_scope":
			left = drafts.GeneratedCountMeasureID(datasets["orders"], "order_id")
			right = drafts.GeneratedCountMeasureID(datasets["orders"], "misleading_net_total_usd")
			if kpi.Expression != left+" - "+right || kpi.Periods != nil {
				t.Fatal("operator rejects scope-inheriting completeness companion")
			}
			continue
		case "unknown_order_amounts":
			left = drafts.GeneratedCountMeasureID(datasets["orders"], "order_id")
			right = drafts.GeneratedCountMeasureID(datasets["orders"], "misleading_net_total_usd")
		case "unknown_cohort_refund_amounts":
			left = drafts.GeneratedCountMeasureID(datasets["refunds"], "refund_id")
			right = drafts.GeneratedCountMeasureID(datasets["refunds"], "amount_usd")
		case "unknown_activity_refund_amounts":
			left = drafts.GeneratedCountMeasureID(datasets["refunds"], "refund_id")
			right = drafts.GeneratedCountMeasureID(datasets["refunds"], "amount_usd")
			axis = refundTime
		default:
			t.Fatal("operator rejects unknown net/companion definition")
		}
		if kpi.Expression != left+" - "+right || kpi.Periods == nil || kpi.Periods.Policy != semantics.MetricPeriodBindingsPolicy || len(kpi.Periods.Bindings) != 2 {
			t.Fatal("operator rejects formula/period mismatch")
		}
		seen := map[string]string{}
		for _, binding := range kpi.Periods.Bindings {
			seen[binding.Measure.ID] = binding.Dimension.ID
		}
		leftAxis := axis
		if strings.HasPrefix(kpi.ID, "known_") {
			leftAxis = orderTime
		}
		if seen[left] != leftAxis || seen[right] != axis {
			t.Fatal("operator rejects swapped period basis")
		}
	}

	catalog := semantics.CompletenessCatalog{Measures: pack.Measures, KPIs: pack.KPIs, Columns: map[semantics.Reference]semantics.Column{}}
	for _, dataset := range pack.Datasets {
		for _, column := range dataset.Columns {
			catalog.Columns[semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: column.ID}] = column
		}
	}
	if binding, err := semantics.ResolveKnownAmountCompleteness(catalog, gross); err != nil || binding.UnknownCount.ID != "unknown_order_amounts_in_scope" {
		t.Fatal("operator rejects incomplete amount disclosure link", err)
	}
	found := false
	for _, measure := range pack.Measures {
		if measure.ID == refunds || measure.ID == drafts.GeneratedCountMeasureID(datasets["refunds"], "refund_id") || measure.ID == drafts.GeneratedCountMeasureID(datasets["refunds"], "amount_usd") {
			found = true
			own, parent := false, false
			for _, filter := range measure.Filters {
				if filter.Operator != "eq" || len(filter.Values) != 1 || filter.Values[0] != "P" {
					t.Fatal("operator rejects refund population")
				}
				own = own || filter.Field.Dataset == datasets["refunds"] && filter.Field.ID == "status_code" && filter.Relationship == ""
				parent = parent || filter.Field.Dataset == datasets["orders"] && filter.Field.ID == "status_code" && filter.Relationship == "confirmed_refunds_to_orders"
			}
			if !own || !parent || len(measure.Filters) != 2 {
				t.Fatal("operator rejects missing posted/paid-parent populations")
			}
		}
	}
	if !found {
		t.Fatal("operator missing refund measure")
	}
}

func TestGeneratedAdversarialNetAuthoringRecorded(t *testing.T) {
	h, _, current, datasets := generateAdversarialPublished(t, true)
	reviewGeneratedAdversarialNet(t, current.Pack, datasets)
	published, err := h.client.PublishedTopic(t.Context(), current.Pack.Topic)
	if err != nil || published.Digest != current.Metadata.Digest || len(published.Definition.KPIs) != 6 {
		t.Fatal("reviewed net meaning did not survive publication", err)
	}
	for _, kpi := range published.Definition.KPIs {
		if kpi.ID == "unknown_order_amounts_in_scope" {
			if kpi.Periods != nil {
				t.Fatal("scope companion gained independent period")
			}
			continue
		}
		if kpi.Periods == nil || len(kpi.Periods.Bindings) != 2 {
			t.Fatal("public net definition dropped exact period metadata")
		}
	}
	t.Log("Generated two independently reviewed net definitions and three unknown-count companions; SQL execution is qualified separately by the sealed period and v9 lane consumers")
}

// The two SQL programs have identical period-free populations. Only the sealed
// reviewed application may place the requested interval on order or refund time.
func adversarialNetSQL(activity bool) string {
	gross := "SUM(o.misleading_net_total_usd) AS gross_value"
	output := "g.gross_value-r.refund_value AS known_net, r.event_count-r.known_count AS unknown_refund_amounts"
	if activity {
		gross += ", COUNT(o.order_id) AS event_count, COUNT(o.misleading_net_total_usd) AS known_count"
		output = "g.gross_value-r.refund_value AS known_net, g.event_count-g.known_count AS unknown_order_amounts, r.event_count-r.known_count AS unknown_refund_amounts"
	}
	return "WITH g AS (SELECT " + gross + " FROM analytics.adv_orders AS o WHERE o.status_code='P'), r AS (SELECT SUM(r.amount_usd) AS refund_value, COUNT(r.refund_id) AS event_count, COUNT(r.amount_usd) AS known_count FROM analytics.adv_refunds AS r INNER JOIN analytics.adv_orders AS p ON r.division_id=p.division_id AND r.order_id=p.order_id WHERE r.status_code='P' AND p.status_code='P') SELECT " + output + " FROM g CROSS JOIN r"
}

func TestGeneratedAdversarialNetExecutionRecorded(t *testing.T) {
	h, model, current, _ := generateAdversarialPublished(t, true)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	oracle := adversarialOracle(t, warehouse)
	for _, tc := range cases {
		if tc.ID != "cohort-known-net" && tc.ID != "activity-known-net" {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			activity := tc.ID == "activity-known-net"
			metrics := []string{"known_cohort_net", "unknown_cohort_refund_amounts"}
			expected := []string{oracle["known_cohort_net"].(string), fmt.Sprint(oracle["unknown_posted_refund_events_in_2026_paid_cohort"])}
			if activity {
				metrics = []string{"known_activity_net", "unknown_order_amounts", "unknown_activity_refund_amounts"}
				expected = []string{oracle["known_activity_net"].(string), fmt.Sprint(oracle["unknown_amount_paid_orders"]), fmt.Sprint(oracle["unknown_posted_refund_events_on_paid_orders_by_2026_activity"])}
			}
			model.mode.Store(phase18RawResponse(t, adversarialNetSQL(activity)))
			request := nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: tc.Question, MetricIDs: metrics, Kinds: []string{"measure", "kpi", "dimension"}, LimitPerKind: 10, Rerank: true}}
			plan, err := h.query.Plan(t.Context(), h.queryActor, request)
			if err != nil {
				var clarification *nlqroute.Clarification
				if errors.As(err, &clarification) {
					raw, _ := json.Marshal(clarification)
					t.Fatalf("net clarification: %s", raw)
				}
				var budget *nlq.BudgetError
				if errors.As(err, &budget) {
					t.Fatalf("net context budget: tier=%s limit=%d required=%d", budget.Tier, budget.Budget, budget.RequiredTokens)
				}
				t.Fatal("net plan", err)
			}
			run, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "net-" + tc.ID, Rows: 10, Bytes: 65536})
			if err != nil || run.Execution.Result == nil {
				t.Fatal("net execution", err)
			}
			rows := run.Execution.Result.Rows
			if len(rows) != 1 || len(rows[0]) != len(expected) {
				t.Fatal("required net/unknown-count outputs omitted or multiplied", len(rows))
			}
			for i, want := range expected {
				var actual string
				if json.Unmarshal(rows[0][i], &actual) != nil {
					actual = string(rows[0][i])
				}
				if !liveNumberEquals(actual, want) {
					t.Fatalf("independent net/companion oracle mismatch output%d: got=%s want=%s", i, actual, want)
				}
			}
			t.Logf("exact held-out net and all requested companion outputs matched independent oracle: %v", expected)
		})
	}
}

func TestGeneratedAdversarialNetMeaningClarificationsRecorded(t *testing.T) {
	for _, reviewed := range []bool{false, true} {
		name := "missing"
		reason := "reviewed_metric_meaning_required"
		if reviewed {
			name = "competing"
			reason = "ambiguous_metric_meaning"
		}
		t.Run(name, func(t *testing.T) {
			h, model, current, datasets := generateAdversarialPublished(t, reviewed)
			gross := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["orders"], "misleading_net_total_usd")
			for _, control := range []struct {
				id, question, locale string
				pinned               bool
			}{{"en", "What is net revenue?", "en", false}, {"es", "¿Cuáles son los ingresos netos?", "es", false}, {"incompatible_selected_gross", "What is net revenue?", "en", true}} {
				t.Run(control.id, func(t *testing.T) {
					model.mode.Store(phase18RawResponse(t, "SELECT SUM(misleading_net_total_usd) AS gross FROM analytics.adv_orders WHERE status_code='P'"))
					metrics := []string(nil)
					if control.pinned {
						metrics = []string{gross}
					}
					_, err := h.query.Plan(t.Context(), h.queryActor, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.Language(control.locale), Question: control.question, MetricIDs: metrics, Kinds: []string{"measure", "kpi"}, LimitPerKind: 10, Rerank: true}})
					var clarification *nlqroute.Clarification
					if !errors.As(err, &clarification) || clarification.Reason != reason {
						t.Fatalf("expected exact %s boundary, got %T %v", reason, err, err)
					}
				})
			}
		})
	}
}

func TestGeneratedAdversarialCompletenessOutputIdentityRecorded(t *testing.T) {
	h, model, current, datasets := generateAdversarialPublished(t, true)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	question := ""
	for _, c := range cases {
		if c.ID == "gross-en" {
			question = c.Question
		}
	}
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	oracle := adversarialOracle(t, warehouse)
	metric := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["orders"], "misleading_net_total_usd")
	// Aliases deliberately suggest the opposite meaning. Only proof-issued
	// expression identity may locate the amount and its unknown-count output.
	model.mode.Store(phase18RawResponse(t, "SELECT COUNT(order_id)-COUNT(misleading_net_total_usd) AS looks_like_gross,SUM(misleading_net_total_usd) AS looks_like_unknown FROM analytics.adv_orders WHERE status_code='P'"))
	plan, err := h.query.Plan(t.Context(), h.queryActor, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: question, MetricIDs: []string{metric}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5, Rerank: true}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "proof-output-identity", Rows: 10, Bytes: 65536})
	if err != nil || run.Execution.Result == nil {
		t.Fatal(err)
	}
	evidence, ok := adversarialAmountEvidence(run, current.Pack.Topic+":measure:"+metric)
	if !ok || evidence.ValueColumn != 1 || evidence.UnknownCountColumn != 0 || evidence.Status != "incomplete" || len(evidence.Rows) != 1 || evidence.Rows[0].UnknownCount != fmt.Sprint(oracle["unknown_amount_paid_orders"]) {
		t.Fatal("result guessed meaning from alias/position", evidence)
	}
	if len(run.Execution.Result.Rows) != 1 || !liveSingleNumericEquals([][]json.RawMessage{{run.Execution.Result.Rows[0][evidence.ValueColumn]}}, oracle["known_gross_paid_local_2026"].(string)) {
		t.Fatal("proof-bound amount differs from independent oracle")
	}
	server := httptest.NewServer(nlqapi.ExecutionHandler(h.f.token.verifier, h.query, http.NotFoundHandler()))
	defer server.Close()
	claims := h.f.token.claims(h.queryActor.Tenant(), h.queryActor.User(), phase18Scopes(h.queryActor.Tenant(), true))
	claims["session"] = h.queryActor.Session()
	token := h.f.token.sign(t, claims, nil)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	before := model.requests.Load()
	replay, err := client.RunNLQ(t.Context(), sdk.NLQRunRequest{QueryID: plan.QueryID, Operation: "proof-output-identity", Rows: 10, Bytes: 65536})
	if err != nil {
		t.Fatal("HTTP/SDK completeness replay", err)
	}
	publicEvidence, ok := adversarialAmountEvidence(replay, current.Pack.Topic+":measure:"+metric)
	if !ok || publicEvidence.ValueColumn != 1 || publicEvidence.UnknownCountColumn != 0 || publicEvidence.Status != "incomplete" || model.requests.Load() != before {
		t.Fatal("public surface lost proof-bound evidence or replay called model", publicEvidence)
	}
	httpPlan, err := client.PlanNLQ(t.Context(), sdk.NLQPlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: question, MetricIDs: []string{metric}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5, Rerank: true}})
	if err != nil {
		t.Fatal("HTTP/SDK completeness plan", err)
	}
	httpRun, err := client.RunNLQ(t.Context(), sdk.NLQRunRequest{QueryID: httpPlan.QueryID, Operation: "http-proof-output-identity", Rows: 10, Bytes: 65536})
	if err != nil {
		t.Fatal("HTTP/SDK completeness run", err)
	}
	if metadata, ok := adversarialAmountEvidence(httpRun, current.Pack.Topic+":measure:"+metric); !ok || metadata.Status != "incomplete" || len(metadata.Rows) != 1 || metadata.Rows[0].UnknownCount != "1" {
		t.Fatal("HTTP/SDK completeness evidence missing", metadata)
	}
}

// The caller repeats its original request, not the router's normalized grouping.
// This exercises persisted operation identity across the HTTP/SDK boundary.
func TestGeneratedAdversarialMonthlyOriginalPlanReplayRecorded(t *testing.T) {
	h, model, current, datasets := generateAdversarialPublished(t, true)
	metadata := support.Raw(t, h.f.dsn)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	question := ""
	for _, c := range cases {
		if c.ID == "month-en" {
			question = c.Question
		}
	}
	if question == "" {
		t.Fatal("missing unchanged monthly held-out question")
	}
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	oracle := adversarialOracle(t, warehouse)
	metric := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["orders"], "misleading_net_total_usd")
	model.mode.Store(phase18RawResponse(t, "SELECT date_trunc('month',ordered_at,'America/New_York') AS order_month,SUM(misleading_net_total_usd) AS known_gross,COUNT(order_id)-COUNT(misleading_net_total_usd) AS unknown_amounts FROM analytics.adv_orders WHERE status_code='P' GROUP BY date_trunc('month',ordered_at,'America/New_York') ORDER BY order_month"))
	server := httptest.NewServer(nlqapi.ExecutionHandler(h.f.token.verifier, h.query, http.NotFoundHandler()))
	defer server.Close()
	claims := h.f.token.claims(h.queryActor.Tenant(), h.queryActor.User(), phase18Scopes(h.queryActor.Tenant(), true))
	claims["session"] = h.queryActor.Session()
	token := h.f.token.sign(t, claims, nil)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	original := sdk.NLQPlanRequest{Operation: "monthly-original-plan", QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: question, MetricIDs: []string{metric}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5, Rerank: true}}
	plan, err := client.PlanNLQ(t.Context(), original)
	if err != nil {
		t.Fatal("initial monthly plan", err)
	}
	before := model.requests.Load()
	assertPlanReplay := func(stage string) {
		t.Helper()
		if original.Grouping != nil {
			t.Fatal("original caller request acquired grouping")
		}
		replayed, err := client.PlanNLQ(t.Context(), original)
		if err != nil || replayed.QueryID != plan.QueryID || model.requests.Load() != before {
			t.Fatalf("%s: unchanged monthly Plan replay: query=%s want=%s calls=%d want=%d err=%v", stage, replayed.QueryID, plan.QueryID, model.requests.Load(), before, err)
		}
	}
	assertPlanReplay("before Run")
	changed := original
	changed.Question += " changed"
	if _, err := client.PlanNLQ(t.Context(), changed); err == nil || model.requests.Load() != before {
		t.Fatal("changed submission reused original")
	}
	if _, err := metadata.Exec(t.Context(), `UPDATE chartworks.nlq_queries SET plan_request_digest=repeat('a',64),revision=revision+1 WHERE query_id=$1`, plan.QueryID); err == nil {
		t.Fatal("immutable Plan digest changed")
	} else {
		var p *pgconn.PgError
		if !errors.As(err, &p) || p.Code != "55000" {
			t.Fatal("wrong immutable-origin denial", err)
		}
	}
	runRequest := sdk.NLQRunRequest{QueryID: plan.QueryID, Operation: "monthly-original-run", Rows: 20, Bytes: 65536}
	run, err := client.RunNLQ(t.Context(), runRequest)
	if err != nil || run.Execution.Result == nil {
		t.Fatal("monthly Run", err)
	}
	evidence, ok := adversarialAmountEvidence(run, current.Pack.Topic+":measure:"+metric)
	if !ok || evidence.Status != "incomplete" || len(evidence.Rows) != len(run.Execution.Result.Rows) {
		t.Fatal("missing monthly completeness", evidence)
	}
	projected := [][]json.RawMessage{}
	for i, row := range run.Execution.Result.Rows {
		if evidence.ValueColumn < 0 || evidence.ValueColumn >= len(row) || len(row) == 0 {
			t.Fatal("invalid proof output ordinal")
		}
		projected = append(projected, []json.RawMessage{row[0], row[evidence.ValueColumn]})
		var month string
		if json.Unmarshal(row[0], &month) != nil || len(month) < 7 {
			t.Fatal("invalid month")
		}
		want := oracle["monthly_unknown_paid_amounts_local_2026"].(map[string]int)[month[:7]]
		status := "complete"
		if want > 0 {
			status = "incomplete"
		}
		if evidence.Rows[i].Row != i || evidence.Rows[i].UnknownCount != fmt.Sprint(want) || evidence.Rows[i].Status != status {
			t.Fatal("monthly completeness differs from independent oracle", evidence.Rows[i])
		}
	}
	if !adversarialMonthsMatch(projected, oracle["monthly_qualifying_paid_local_2026"].(map[string]any)) {
		t.Fatal("monthly values differ from independent oracle")
	}
	assertPlanReplay("after Run")
	index, err := vindex.New(h.f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(h.f.db, h.f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(h.topics, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := nlqexec.New(router, h.topics, h.f.s, h.f.validator, h.f.executor, model.engine, h.f.db)
	if err != nil {
		t.Fatal(err)
	}
	restartedServer := httptest.NewServer(nlqapi.ExecutionHandler(h.f.token.verifier, restarted, http.NotFoundHandler()))
	defer restartedServer.Close()
	client, err = sdk.New(restartedServer.URL, restartedServer.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	assertPlanReplay("service restart")
	resolved, err := restarted.ResolveOperationQueryID(t.Context(), h.queryActor, original.Operation)
	if err != nil || resolved != plan.QueryID {
		t.Fatal("original operation resolution lost", err)
	}

	replayed, err := client.RunNLQ(t.Context(), runRequest)
	if err != nil || replayed.Execution.Result == nil {
		t.Fatal("monthly Run replay", err)
	}
	wantRows, _ := json.Marshal(run.Execution.Result.Rows)
	gotRows, _ := json.Marshal(replayed.Execution.Result.Rows)
	wantCompleteness, _ := json.Marshal(run.AmountCompleteness)
	gotCompleteness, _ := json.Marshal(replayed.AmountCompleteness)
	if string(wantRows) != string(gotRows) || string(wantCompleteness) != string(gotCompleteness) || model.requests.Load() != before {
		t.Fatal("monthly replay changed proved results or called model")
	}
	composedRun := nlqexec.RunRequest(runRequest)
	composedRun.Operation = original.Operation
	composed, _, err := restarted.PlanAndRun(t.Context(), h.queryActor, nlqexec.PlanRequest(original), composedRun)
	if err != nil || composed.QueryID != plan.QueryID || model.requests.Load() != before {
		t.Fatal("composed replay regenerated", err)
	}
	other := original
	other.Operation = "monthly-other-plan"
	second, err := client.PlanNLQ(t.Context(), other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: second.QueryID, Operation: original.Operation}); err == nil {
		t.Fatal("Run stole reserved Plan operation")
	}
	if _, err := metadata.Exec(t.Context(), `UPDATE chartworks.nlq_queries SET operation=$1,revision=revision+1 WHERE query_id=$2`, original.Operation, second.QueryID); err == nil {
		t.Fatal("store allowed cross-query Plan/Run collision")
	} else {
		var p *pgconn.PgError
		if !errors.As(err, &p) || p.Code != "23505" {
			t.Fatal("wrong operation collision denial", err)
		}
	}
	scope, _ := store.NewScope(h.queryActor.Tenant(), h.queryActor.User())
	candidate, err := h.f.db.ReadQuery(t.Context(), scope, second.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	candidate.ID = "monthly-concurrent-reservation"
	candidate.Operation = "monthly-distinct-insert-key"
	candidate.PlanOperation = "monthly-shared-reservation"
	tx, err := metadata.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `UPDATE chartworks.nlq_queries SET operation=$1,revision=revision+1 WHERE query_id=$2`, candidate.PlanOperation, second.QueryID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- h.f.db.CreateQuery(t.Context(), scope, candidate) }()
	select {
	case err := <-result:
		t.Fatal("cross-column reservation did not wait for writer", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, store.ErrConflict) {
			t.Fatal("concurrent Plan/Run reservation conflict lost", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reservation lock not released")
	}
}
