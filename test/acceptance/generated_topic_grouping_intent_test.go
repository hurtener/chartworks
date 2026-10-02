package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

// Recorded provider responses select opaque IDs from the actual request-local
// admitted catalog. Neither questions nor API inputs contain expected metric or
// grouping IDs. These are transport/lifecycle tests, not live language scores.
func recordedGroupingChoice(t *testing.T, mode string, input map[string]any) string {
	t.Helper()
	var material struct {
		Question   string `json:"question"`
		Candidates []struct {
			ID, Kind, Name string
			Key            *nlqroute.GroupingKey `json:"key"`
			Field          *semantics.Reference  `json:"field"`
		} `json:"candidates"`
	}
	for _, raw := range input["messages"].([]any) {
		m := raw.(map[string]any)
		if m["role"] == "user" {
			if err := json.Unmarshal([]byte(m["content"].(string)), &material); err != nil {
				t.Error(err)
			}
		}
	}
	want := strings.TrimPrefix(mode, "grounded_choice:")
	ids := []string{}
	selected := []any{}
	alternatives := []string{}
	for _, c := range material.Candidates {
		ids = append(ids, c.ID)
		matches := want == "metric" && c.Field != nil && c.Field.ID == "total_usd" || want == "scalar" && c.Kind == "scalar_total" || want == "status" && c.Key != nil && c.Name == "Order status" || c.Key != nil && string(c.Key.Grain) == want
		if matches {
			selected = append(selected, map[string]any{"id": c.ID, "quote": material.Question})
		}
	}
	decision := "select"
	if want == "clarify" {
		decision = "clarify"
		alternatives = ids[:min(2, len(ids))]
	} else if want == "no_match" {
		decision = "no_match"
	} else if len(selected) != 1 {
		t.Errorf("recorded choice %s expected one actual candidate, got %d", want, len(selected))
	}
	body, _ := json.Marshal(map[string]any{"decision": decision, "selected": selected, "alternatives": alternatives})
	wire, _ := json.Marshal(map[string]any{"id": "recorded-grouping-intent", "object": "chat.completion", "model": input["model"], "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(body)}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}})
	return "chat_raw:" + string(wire)
}

func TestGeneratedTopicGroundedGroupingIntentRecorded(t *testing.T) {
	model := newGatewayFixture(t, func(c *config.Gateway) {
		recordedLiveChatCaps(c)
		r := c.Roles["embedding"]
		r.MaxBatchItems = 64
		r.MaxBatchBytes = 65536
		c.Roles["embedding"] = r
	})
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedOrdersBusiness, report)
	current := h.generate(t, report, func(columns []semantics.Reference, last bool) {
		responses := []string{recordedGeneratedStep(t, columns)}
		if last {
			responses = append(responses, "topic_quality_echo")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	})
	h.publish(t, current, report)
	server := httptest.NewServer(nlqapi.ExecutionHandler(h.f.token.verifier, h.query, http.NotFoundHandler()))
	defer server.Close()
	token := h.f.token.sign(t, h.f.token.claims(h.f.e.Tenant(), h.f.e.User(), phase18Scopes(h.f.e.Tenant(), true)), nil)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	base := nlqexec.QuestionRequest{Topic: current.Pack.Topic, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, ConceptPolicy: sdk.NLQGroundedConceptPolicy, GroupingIntentPolicy: sdk.NLQGroundedGroupingIntentPolicy, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5, Rerank: true}
	gross, months := generatedIndependentTotals(t, h.f, false)
	cases := []struct {
		id, question, choice, sql string
		locale                    nlq.Language
	}{
		{"spanish-scalar", "¿Cuál es el valor bruto total en USD de todos los pedidos, incluidos los pagados y los cancelados?", "scalar", "SELECT SUM(total_usd) AS gross FROM analytics.orders", nlq.LanguageSpanish},
		{"spanish-month", "¿Cuál es el valor bruto de todos los pedidos por mes del calendario gregoriano en UTC durante 2026? Incluye pedidos pagados y cancelados.", "month", "SELECT date_trunc('month',CAST(ordered_at AS timestamp without time zone)) AS order_month, SUM(total_usd) AS gross FROM analytics.orders GROUP BY date_trunc('month',CAST(ordered_at AS timestamp without time zone)) ORDER BY order_month", nlq.LanguageSpanish},
		{"english-status", "What is all-order gross value in USD grouped by order status, including every paid and cancelled order?", "status", "SELECT status, SUM(total_usd) AS gross FROM analytics.orders GROUP BY status ORDER BY status", nlq.LanguageEnglish},
		{"english-quarter", "What is all-order gross value in USD by Gregorian calendar quarter in UTC during 2026, including paid and cancelled orders?", "quarter", "SELECT date_trunc('quarter',CAST(ordered_at AS timestamp without time zone)) AS order_quarter, SUM(total_usd) AS gross FROM analytics.orders GROUP BY date_trunc('quarter',CAST(ordered_at AS timestamp without time zone)) ORDER BY order_quarter", nlq.LanguageEnglish},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			in := base
			in.Question = tc.question
			in.Locale = tc.locale
			model.mu.Lock()
			model.chatSequence = []string{"grounded_choice:" + tc.choice, "grounded_choice:metric", phase18RawResponse(t, tc.sql)}
			model.mu.Unlock()
			planned, err := client.PlanNLQ(t.Context(), sdk.NLQPlanRequest{QuestionRequest: in})
			if err != nil {
				t.Fatalf("normal SDK plan failed: %v", err)
			}
			if planned.Route.GroupingIntent == nil || planned.Route.Concepts == nil || len(planned.Route.Request.MetricIDs) > 0 || len(planned.Route.Request.References) > 0 {
				t.Fatal("automatic intent did not reach normal Plan")
			}
			before := model.requests.Load()
			run, err := client.RunNLQ(t.Context(), sdk.NLQRunRequest{QueryID: planned.QueryID, Operation: "grounded-" + tc.id, Rows: 20, Bytes: 65536})
			if err != nil || run.Execution.Result == nil {
				t.Fatal("replayed SDK execution", err)
			}
			if model.requests.Load() != before {
				t.Fatal("retained run re-inferred intent")
			}
			rows := run.Execution.Result.Rows
			switch tc.choice {
			case "scalar":
				if !liveSingleNumericEquals(rows, gross) {
					t.Fatal("scalar oracle mismatch")
				}
			case "month":
				if !generatedCalendarCorrect(rows, months) {
					t.Fatal("month oracle mismatch")
				}
			case "quarter":
				if !generatedCalendarCorrect(rows, map[string]string{"2026-01": gross}) {
					t.Fatal("quarter oracle mismatch")
				}
			case "status":
				// Independent fixture facts, unrelated to generated SQL text or intent IDs.
				expected := map[string]string{"paid": "640.00", "cancelled": "60.00"}
				if len(rows) != len(expected) {
					t.Fatal("status row count")
				}
				for _, row := range rows {
					var key string
					if len(row) != 2 || json.Unmarshal(row[0], &key) != nil {
						t.Fatal("status shape")
					}
					var amount string
					if json.Unmarshal(row[1], &amount) != nil {
						amount = string(row[1])
					}
					if !liveNumberEquals(amount, expected[key]) {
						t.Fatal("status oracle mismatch")
					}
					delete(expected, key)
				}
				if len(expected) != 0 {
					t.Fatal("missing status")
				}
			}
		})
	}
	// Both unsupported requests must stop before model/source execution, even
	// though a malicious provider could have offered a scalar candidate.
	for _, tc := range []struct {
		question string
		locale   nlq.Language
	}{{"What is net revenue in USD after refunds?", nlq.LanguageEnglish}, {"¿Cuál es el valor bruto de todos los pedidos por hora en America/New_York, usando ordered_at?", nlq.LanguageSpanish}} {
		t.Run("refusal-"+string(tc.locale), func(t *testing.T) {
			in := base
			in.Question = tc.question
			in.Locale = tc.locale
			before := model.requests.Load()
			pre, err := client.PreflightNLQ(t.Context(), sdk.NLQPreflightRequest{QuestionRequest: in})
			if err != nil || pre.Route.Clarification == nil || model.requests.Load() != before {
				t.Fatal("unsupported request inferred or executable", err)
			}
			if tc.locale == nlq.LanguageSpanish && !strings.Contains(pre.Route.Clarification.Prompt, "Elegí") {
				t.Fatal("Spanish refusal not localized")
			}
		})
	}
	for _, mode := range []string{"clarify", "no_match"} {
		t.Run("pending-manual-"+mode, func(t *testing.T) {
			in := base
			in.Question = cases[2].question
			model.mu.Lock()
			model.chatSequence = []string{"grounded_choice:" + mode}
			model.mu.Unlock()
			pre, err := client.PreflightNLQ(t.Context(), sdk.NLQPreflightRequest{QuestionRequest: in})
			if err != nil || pre.QueryID == "" || pre.Route.Clarification == nil || pre.Route.GroupingIntent == nil {
				t.Fatal("pending grouping not retained", err)
			}
			// A reviewed manual choice starts a fresh explicit Plan. It is not a typed
			// business-answer continuation of the unresolved model proposal.
			dimension := ""
			for _, d := range current.Pack.Dimensions {
				if d.Field.ID == "status" {
					dimension = d.ID
				}
			}
			in.Grouping = &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: current.Pack.Topic, Dimension: dimension}}}
			model.mu.Lock()
			model.chatSequence = []string{"grounded_choice:metric", phase18RawResponse(t, cases[2].sql)}
			model.mu.Unlock()
			planned, err := client.PlanNLQ(t.Context(), sdk.NLQPlanRequest{QuestionRequest: in})
			if err != nil || planned.Route.GroupingIntent != nil || planned.Route.Request.GroupingIntentPolicy != "" {
				t.Fatal("manual choice invoked grouping inference", err)
			}
		})
	}
}
