package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

// Row arithmetic is independent of generated SQL, compiler expressions and
// receipts. A nil sum stays SQL NULL; unknown counts distinguish empty input.

func scalarPredicateOracle(t *testing.T, w adversarialWarehouse, activity, explicitPaid bool) []string {
	t.Helper()
	return scalarPredicateYearOracle(t, w, 2026, activity, explicitPaid)
}

func scalarPredicateYearOracle(t *testing.T, w adversarialWarehouse, year int, activity, explicitPaid bool) []string {
	t.Helper()
	zone, err := time.LoadLocation(w.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	inYear := func(at string) bool {
		v, err := time.Parse(time.RFC3339, at)
		if err != nil {
			t.Fatal(err)
		}
		return v.In(zone).Year() == year
	}
	paid := func(v *string) bool { return v != nil && *v == "P" }
	add := func(sum **big.Rat, amount *string) {
		if amount == nil {
			return
		}
		v, ok := new(big.Rat).SetString(*amount)
		if !ok {
			t.Fatal("invalid fixture amount")
		}
		if *sum == nil {
			*sum = new(big.Rat)
		}
		(*sum).Add(*sum, v)
	}
	type key struct {
		division string
		id       int
	}
	parents := map[key]adversarialOrder{}
	var gross, refunded *big.Rat
	unknownOrders, unknownRefunds := 0, 0
	for _, o := range w.Orders {
		parents[key{o.Division, o.ID}] = o
		if !paid(o.Status) || !inYear(o.At) || explicitPaid && !paid(o.Status) {
			continue
		}
		add(&gross, o.Amount)
		if o.Amount == nil {
			unknownOrders++
		}
	}
	for _, r := range w.Refunds {
		parent, ok := parents[key{r.Division, r.Order}]
		if !ok || !paid(r.Status) || !paid(parent.Status) || explicitPaid && !paid(parent.Status) {
			continue
		}
		at := parent.At
		if activity {
			at = r.At
		}
		if !inYear(at) {
			continue
		}
		add(&refunded, r.Amount)
		if r.Amount == nil {
			unknownRefunds++
		}
	}
	net := "NULL"
	if gross != nil && refunded != nil {
		net = new(big.Rat).Sub(gross, refunded).FloatString(2)
	}
	return []string{net, fmt.Sprint(unknownOrders), fmt.Sprint(unknownRefunds)}
}

func TestScalarPredicateEntailmentPublicBoundary(t *testing.T) {
	h, model, current, datasets := generateAdversarialPublished(t, true)
	var w adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &w)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	questions := map[string]string{}
	for _, c := range cases {
		questions[c.ID] = c.Question
	}
	var selection nlqroute.InterpretationSelection
	for _, d := range current.Pack.Dimensions {
		if d.Field.Dataset != datasets["orders"] || d.Field.ID != "status_code" {
			continue
		}
		for _, v := range d.Values {
			if v.ID == "orders_paid" && v.Value == "P" {
				selection = nlqroute.InterpretationSelection{Topic: current.Pack.Topic, Dimension: d.ID, Value: v.ID, Operator: "eq"}
			}
		}
	}
	if selection.Value == "" {
		t.Fatal("fixture lacks an explicit reviewed paid selection")
	}
	// Follow the exact selected transitive closure, including unknown-amount
	// COUNT leaves. An unrelated unselected line-total measure grants no proof.
	selected := map[string]bool{}
	var visit func(semantics.Reference)
	visit = func(ref semantics.Reference) {
		if ref.Kind == semantics.KindMeasure {
			selected[ref.ID] = true
			return
		}
		for _, k := range current.Pack.KPIs {
			if k.ID == ref.ID {
				for _, input := range k.Inputs {
					visit(input)
				}
			}
		}
	}
	for _, id := range []string{"known_cohort_net", "known_activity_net", "unknown_order_amounts", "unknown_cohort_refund_amounts", "unknown_activity_refund_amounts"} {
		visit(semantics.Reference{Kind: semantics.KindKPI, ID: id})
	}
	if len(selected) != 6 {
		t.Fatal("fixture lost selected SUM/COUNT closure", selected)
	}
	for _, m := range current.Pack.Measures {
		if !selected[m.ID] {
			continue
		}
		proved := false
		for _, f := range m.Filters {
			if f.Field.Dataset == datasets["orders"] && f.Field.ID == "status_code" && f.Operator == "eq" && reflect.DeepEqual(f.Values, []string{"P"}) && (m.Field.Dataset == datasets["orders"] && f.Relationship == "" || m.Field.Dataset == datasets["refunds"] && f.Relationship == "confirmed_refunds_to_orders") {
				proved = true
			}
		}
		if !proved {
			t.Fatal("a reviewed SUM/COUNT leaf does not entail paid orders", m.ID)
		}
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
	for _, state := range []string{"numeric", "all_null", "empty"} {
		switch state {
		case "all_null":
			for i := range w.Orders {
				w.Orders[i].Amount = nil
			}
			for i := range w.Refunds {
				w.Refunds[i].Amount = nil
			}
			if _, err := h.f.admin.Exec(t.Context(), `UPDATE analytics.adv_orders SET misleading_net_total_usd=NULL; UPDATE analytics.adv_refunds SET amount_usd=NULL`); err != nil {
				t.Fatal(err)
			}
		case "empty":
			w.Orders, w.Refunds = nil, nil
			if _, err := h.f.admin.Exec(t.Context(), `DELETE FROM analytics.adv_refunds; DELETE FROM analytics.adv_order_lines; DELETE FROM analytics.adv_orders`); err != nil {
				t.Fatal(err)
			}
		}
		for _, activity := range []bool{false, true} {
			name, net, unknown := "cohort-known-net", "known_cohort_net", "unknown_cohort_refund_amounts"
			if activity {
				name, net, unknown = "activity-known-net", "known_activity_net", "unknown_activity_refund_amounts"
			}
			want := scalarPredicateOracle(t, w, activity, false)
			if got := scalarPredicateOracle(t, w, activity, true); !reflect.DeepEqual(got, want) {
				t.Fatal("explicit paid selection changes the independent row oracle", got, want)
			}
			model.mode.Store(phase18RawResponse(t, scopedNetSQL))
			request := nlqexec.QuestionRequest{Topic: current.Pack.Topic, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: questions[name], MetricIDs: []string{net, "unknown_order_amounts", unknown}, Kinds: []string{"kpi", "measure", "dimension"}, LimitPerKind: 8}
			plan, err := h.query.Plan(t.Context(), h.queryActor, nlqexec.PlanRequest{QuestionRequest: request})
			if err != nil || plan.Analytical == nil || plan.Analytical.Version != exec.AnalyticalScopedPopulationsVersion {
				t.Fatal("ordinary scoped control", state, activity, err)
			}
			run, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-oracle"})
			if err != nil || run.Execution.Result == nil || len(run.Execution.Result.Rows) != 1 || len(run.Execution.Result.Rows[0]) != 3 {
				t.Fatal("ordinary source control", err)
			}
			for i, cell := range run.Execution.Result.Rows[0] {
				got := "NULL"
				if string(cell) != "null" && json.Unmarshal(cell, &got) != nil {
					got = string(cell)
				}
				if got != want[i] && (got == "NULL" || want[i] == "NULL" || !liveNumberEquals(got, want[i])) {
					t.Fatal("independent source oracle mismatch", state, activity, i, got, want[i])
				}
			}
			request.InterpretationSelections = []nlqroute.InterpretationSelection{selection}
			preflight, err := h.query.Preflight(t.Context(), h.queryActor, nlqexec.PreflightRequest{QuestionRequest: request})
			if err != nil || preflight.Route.Interpretation == nil || len(preflight.Route.Interpretation.Values) != 1 || preflight.Route.Interpretation.Values[0].GovernedValue != selection.Value {
				var budget *nlq.BudgetError
				if errors.As(err, &budget) {
					t.Logf("explicit paid preflight budget state=%s activity=%t tier=%s required_tokens=%d budget=%d", state, activity, budget.Tier, budget.RequiredTokens, budget.Budget)
				}
				t.Fatal("explicit paid intent did not reach public routing", err)
			}
			// Measure the unchanged mandatory input separately from optional
			// retrieval. This observes headroom without changing routing or caps.
			assembler, err := nlq.NewDefaultContextAssembler()
			if err != nil {
				t.Fatal(err)
			}
			view := preflight.Route.Context
			mandatory, err := assembler.Assemble(t.Context(), nlq.ContextInput{MetricFormat: view.MetricFormat, Locale: view.Locale, Strategy: view.Strategy, Topic: view.Topic, TopicVersion: view.TopicVersion, Topics: view.Topics, Question: view.Question, Relations: view.Relations, Constraints: view.Constraints, Metrics: view.Metrics}, view.Tier)
			if err != nil {
				t.Fatal("mandatory headroom observation", err)
			}
			t.Logf("state=%s activity=%t mandatory_tokens=%d budget=%d headroom=%d", state, activity, mandatory.Tokens, mandatory.Budget, mandatory.Budget-mandatory.Tokens)
			explicit, err := h.query.Plan(t.Context(), h.queryActor, nlqexec.PlanRequest{QuestionRequest: request})
			if err != nil || explicit.Analytical == nil || explicit.Analytical.Version != "analytical-metrics-v13" || explicit.Bindings == nil || explicit.Bindings.SchemaVersion != 6 {
				t.Fatal("exact reviewed predicate must be proved for every selected scalar leaf", state, activity, err)
			}
			p, err := client.PlanNLQ(t.Context(), sdk.NLQPlanRequest{QuestionRequest: request})
			if err != nil || p.QueryID == "" || p.Analytical == nil || p.Analytical.Version != "analytical-metrics-v13" {
				t.Fatal("SDK must admit the proved scalar predicate", state, activity, err)
			}
			r, err := client.RunNLQ(t.Context(), sdk.NLQRunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-entailed"})
			if err != nil || r.Execution.Result == nil || len(r.Execution.Result.Rows) != 1 || len(r.Execution.Result.Rows[0]) != 3 {
				t.Fatal("proved scalar source run", state, activity, err)
			}
			for i, cell := range r.Execution.Result.Rows[0] {
				got := "NULL"
				if string(cell) != "null" && json.Unmarshal(cell, &got) != nil {
					got = string(cell)
				}
				if got != want[i] && (got == "NULL" || want[i] == "NULL" || !liveNumberEquals(got, want[i])) {
					t.Fatal("explicit-predicate independent source oracle mismatch", state, activity, i, got, want[i])
				}
			}
			t.Logf("state=%s activity=%t source_oracle=%v explicit_paid=identical service=proved SDK=proved", state, activity, want)
		}
	}
}
