package acceptance

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
)

const scopedNetSQL = `WITH gross AS (SELECT sum(o.misleading_net_total_usd) AS total,count(o.order_id) AS events,count(o.misleading_net_total_usd) AS known FROM analytics.adv_orders o WHERE o.status_code='P'), refunds AS (SELECT sum(r.amount_usd) AS total,count(r.refund_id) AS events,count(r.amount_usd) AS known FROM analytics.adv_refunds r INNER JOIN analytics.adv_orders o ON r.division_id=o.division_id AND r.order_id=o.order_id WHERE r.status_code='P' AND o.status_code='P') SELECT g.total-r.total AS known_net,g.events-g.known AS unknown_orders,r.events-r.known AS unknown_refunds FROM gross g CROSS JOIN refunds r`

// Recorded SQL is an execution control, never authoring input or a live-model
// quality measurement. Definitions still travel through generated authoring,
// independent human review/publication, routed period custody and stored replay.
func scopedNetModelSQL(activity bool) string {
	if activity {
		return scopedNetSQL
	}
	sql := strings.Replace(scopedNetSQL, ",count(o.order_id) AS events,count(o.misleading_net_total_usd) AS known", "", 1)
	return strings.Replace(sql, ",g.events-g.known AS unknown_orders", "", 1)
}

func TestSQLRecoveryScalarCohortActivityAcceptance(t *testing.T) {
	h, model, current, _ := generateAdversarialPublished(t, true)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	questions := map[string]string{}
	for _, c := range cases {
		questions[c.ID] = c.Question
	}
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	oracle := adversarialOracle(t, warehouse)
	request := func(activity bool) nlqexec.PlanRequest {
		key, net, unknown := "cohort-known-net", "known_cohort_net", "unknown_cohort_refund_amounts"
		if activity {
			key, net, unknown = "activity-known-net", "known_activity_net", "unknown_activity_refund_amounts"
		}
		metrics := []string{net, unknown}
		if activity {
			metrics = append(metrics, "unknown_order_amounts")
		}
		return nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: questions[key], MetricIDs: metrics, Kinds: []string{"kpi", "measure", "dimension"}, LimitPerKind: 8}}
	}
	for _, activity := range []bool{false, true} {
		model.mode.Store(phase18RawResponse(t, scopedNetModelSQL(activity)))
		plan, err := h.query.Plan(t.Context(), h.queryActor, request(activity))
		if err != nil {
			var budget *nlq.BudgetError
			if errors.As(err, &budget) {
				t.Logf("mandatory budget tier=%s required_tokens=%d budget=%d", budget.Tier, budget.RequiredTokens, budget.Budget)
			}
			t.Fatal("scoped Plan", activity, err)
		}
		if plan.Analytical == nil || plan.Analytical.Version != readexec.AnalyticalScopedPopulationsVersion || plan.Bindings == nil || plan.Bindings.SchemaVersion != 2 {
			t.Fatal("missing scoped receipt")
		}
		out, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-scoped"})
		key := "known_cohort_net"
		if activity {
			key = "known_activity_net"
		}
		if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 1 || len(out.Execution.Result.Rows[0]) != len(request(activity).MetricIDs) {
			t.Fatal("scoped Run", activity, err)
		}
		row := out.Execution.Result.Rows[0]
		expected := []string{oracle[key].(string), "1"}
		if activity {
			expected = append(expected, "1")
		}
		for i, want := range expected {
			var value string
			if json.Unmarshal(row[i], &value) != nil {
				value = string(row[i])
			}
			if !liveNumberEquals(value, want) {
				t.Fatal("independent net/count oracle mismatch", activity, i, value, want)
			}
		}
		scope, _ := store.NewScope(h.queryActor.Tenant(), h.queryActor.User())
		stored, err := h.f.db.ReadQuery(t.Context(), scope, plan.QueryID)
		if err != nil || stored.AnalyticalVersion != 9 || stored.Clarification == nil || stored.Clarification.BaseSQL != scopedNetModelSQL(activity) || len(stored.Clarification.BaseParameters) != 0 || len(stored.Parameters) != 4 {
			t.Fatal("period custody/replay record", err)
		}
		assertScopedLearningBase(t, h.query, h.queryActor, current.Pack.Topic, plan.QueryID, scopedNetModelSQL(activity), nlqexec.ScopedScalarExamplePolicy)
		t.Logf("actual scoped activity=%t expected=%v receipt=%d binding=%d", activity, expected, stored.AnalyticalVersion, stored.Clarification.Binding.SchemaVersion)
		for _, b := range stored.Clarification.Binding.Bindings {
			if b.Population == "" || len(b.Parameters) != 2 {
				t.Fatal("missing lane placement evidence")
			}
		}
		if !activity {
			for i, mutate := range []func(*nlqexec.QueryRecord){
				func(q *nlqexec.QueryRecord) {
					q.Clarification.Binding.Bindings[0].Population = q.Clarification.Binding.Bindings[1].Population
				},
				func(q *nlqexec.QueryRecord) {
					q.Parameters[0].Value = "2025-01-01T05:00:00Z"
					q.Analytical.Query = readexec.AnalyticalQueryDigest(q.SQL, q.Parameters)
				},
				func(q *nlqexec.QueryRecord) {
					q.Analytical.Outputs[0].Column = (q.Analytical.Outputs[0].Column + 1) % len(q.Analytical.Outputs)
				},
			} {
				tampered := stored
				clarification := *stored.Clarification
				clarification.Binding.Bindings = append([]readexec.BusinessParameterBinding(nil), stored.Clarification.Binding.Bindings...)
				tampered.Clarification = &clarification
				analytical := *stored.Analytical
				analytical.Outputs = append([]readexec.AnalyticalOutput(nil), stored.Analytical.Outputs...)
				tampered.Analytical = &analytical
				tampered.Parameters = append([]readexec.Parameter(nil), stored.Parameters...)
				tampered.ID = readexec.Hash([]string{"scoped-custody-tamper", stored.ID, strconv.Itoa(i)})[:32]
				tampered.Operation, tampered.Status, tampered.Revision = "", "planned", 1
				tampered.Result = nil
				mutate(&tampered)
				operation := tampered.ID + "-rejected"
				if i == 2 {
					tampered.Status, tampered.Result, tampered.Operation = stored.Status, stored.Result, operation
				}
				if err := h.f.db.CreateQuery(t.Context(), scope, tampered); err != nil {
					t.Fatal("tamper fixture", err)
				}
				if _, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: tampered.ID, Operation: operation}); err == nil {
					t.Fatal("changed stored scoped custody executed")
				}
			}
		}
	}
	cohortSQL := scopedNetModelSQL(false)
	for _, bad := range []string{
		strings.Replace(cohortSQL, "r.division_id=o.division_id AND ", "", 1),
		strings.Replace(cohortSQL, " AND o.status_code='P'", "", 1),
		strings.Replace(cohortSQL, "r.status_code='P'", "r.status_code='V'", 1),
		strings.Replace(cohortSQL, "r.status_code='P'", "r.status_code='P' AND r.refunded_at >= TIMESTAMPTZ '2026-01-01 00:00 America/New_York'", 1),
		strings.Replace(cohortSQL, "g.total-r.total", "r.total-g.total", 1),
		strings.Replace(cohortSQL, "g.total-r.total", "coalesce(g.total,0)-coalesce(r.total,0)", 1),
		cohortSQL + " WHERE g.total>0",
		strings.Replace(cohortSQL, "g.total-r.total", "g.total-r.total+$1", 1),
	} {
		model.mode.Store(phase18RawResponse(t, bad))
		if _, err := h.query.Plan(t.Context(), h.queryActor, request(false)); err == nil {
			t.Fatal("changed scalar population accepted", bad)
		}
	}
	// All-unknown input is not an empty fact population and is never zero-filled.
	for i := range warehouse.Orders {
		warehouse.Orders[i].Amount = nil
	}
	for i := range warehouse.Refunds {
		warehouse.Refunds[i].Amount = nil
	}
	allUnknown := adversarialOracle(t, warehouse)["unknown_posted_refund_events_in_2026_paid_cohort"].(int)
	if allUnknown <= 1 {
		t.Fatal("fixture lacks all-NULL population witness")
	}
	if _, err := h.f.admin.Exec(t.Context(), `UPDATE analytics.adv_orders SET misleading_net_total_usd=NULL; UPDATE analytics.adv_refunds SET amount_usd=NULL`); err != nil {
		t.Fatal(err)
	}
	model.mode.Store(phase18RawResponse(t, cohortSQL))
	plan, err := h.query.Plan(t.Context(), h.queryActor, request(false))
	if err != nil {
		t.Fatal(err)
	}
	out, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-null"})
	if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 1 || string(out.Execution.Result.Rows[0][0]) != "null" {
		t.Fatal("all-unknown net was filled", err)
	}
	var unknown string
	if json.Unmarshal(out.Execution.Result.Rows[0][1], &unknown) != nil {
		unknown = string(out.Execution.Result.Rows[0][1])
	}
	if !liveNumberEquals(unknown, strconv.Itoa(allUnknown)) {
		t.Fatal("all-NULL rows collapsed to empty", unknown, allUnknown)
	}
	t.Logf("all-NULL net=NULL unknown_refunds=%d", allUnknown)
	if _, err := h.f.admin.Exec(t.Context(), `DELETE FROM analytics.adv_refunds; DELETE FROM analytics.adv_order_lines; DELETE FROM analytics.adv_orders`); err != nil {
		t.Fatal(err)
	}
	plan, err = h.query.Plan(t.Context(), h.queryActor, request(false))
	if err != nil {
		t.Fatal(err)
	}
	out, err = h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-empty"})
	if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 1 {
		t.Fatal("empty singleton disappeared", err)
	}
	row := out.Execution.Result.Rows[0]
	if len(row) != 2 || string(row[0]) != "null" || (string(row[1]) != "0" && string(row[1]) != `"0"`) {
		t.Fatal("empty population NULL/count-zero changed", row)
	}
}
