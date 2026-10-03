package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

type factOracleRefund struct {
	id, order int
	date      string
	amount    *int
}

// This oracle operates on fixture rows, not SQL, binder constraints, receipts,
// compiler expressions or the older grouped-period oracle. Each input population
// establishes its own groups before SUM/COUNT and final complete-key selection.
func groupedFactOracle(orders []groupedOwnedOrder, refunds []factOracleRefund, minID int, raw, onlyNull, periods, selected bool) map[string][2]string {
	type aggregate struct{ sum, known int }
	gross, returned := map[string]aggregate{}, map[string]aggregate{}
	parents := map[int]groupedOwnedOrder{}
	key := func(o groupedOwnedOrder) string {
		if o.region == nil {
			return "NULL"
		}
		return *o.region
	}
	for _, o := range orders {
		parents[o.id] = o
		if periods && !strings.HasPrefix(o.date, "2026") || !raw && !o.paid {
			continue
		}
		value := gross[key(o)]
		if o.paid && o.amount != nil {
			value.sum += *o.amount
			value.known++
		}
		gross[key(o)] = value
	}
	for _, r := range refunds {
		if r.id < minID || periods && !strings.HasPrefix(r.date, "2026") {
			continue
		}
		if onlyNull && r.amount != nil || !onlyNull && r.amount != nil && *r.amount < 17 {
			continue
		}
		value := returned[key(parents[r.order])]
		if r.amount != nil {
			value.sum += *r.amount
			value.known++
		}
		returned[key(parents[r.order])] = value
	}
	keys := map[string]bool{}
	for k := range gross {
		keys[k] = true
	}
	for k := range returned {
		keys[k] = true
	}
	result := map[string][2]string{}
	for k := range keys {
		if selected && k != "north" {
			continue
		}
		g, gok := gross[k]
		r, rok := returned[k]
		pair := [2]string{"NULL", "NULL"}
		if gok && rok {
			pair[1] = strconv.Itoa(g.known - r.known)
			if g.known > 0 && r.known > 0 {
				pair[0] = strconv.Itoa(g.sum - r.sum)
			}
		}
		result[k] = pair
	}
	return result
}

func TestSQLRecoveryGroupedFactAcceptance(t *testing.T) {
	for _, raw := range []bool{false, true} {
		for _, onlyNull := range []bool{false, true} {
			t.Run(fmt.Sprintf("raw_%t_null_only_%t", raw, onlyNull), func(t *testing.T) { runGroupedFactAcceptance(t, raw, onlyNull, false, false) })
		}
	}
	t.Run("periods_and_final_selection", func(t *testing.T) { runGroupedFactAcceptance(t, false, false, true, true) })
}

func runGroupedFactAcceptance(t *testing.T, raw, onlyNull, periods, selected bool) {
	t.Helper()
	f := liveCommerceSource(t)
	if _, err := f.admin.Exec(t.Context(), `DELETE FROM analytics.order_items; DELETE FROM analytics.refunds; DELETE FROM analytics.orders; DELETE FROM analytics.customers;
 ALTER TABLE analytics.customers ALTER COLUMN region DROP NOT NULL;
 ALTER TABLE analytics.customers DROP CONSTRAINT customers_region_check;
 ALTER TABLE analytics.orders ALTER COLUMN total_usd DROP NOT NULL;
 ALTER TABLE analytics.refunds ALTER COLUMN amount_usd DROP NOT NULL;`); err != nil {
		t.Fatal(err)
	}
	text := func(s string) *string { return &s }
	number := func(n int) *int { return &n }
	orders := []groupedOwnedOrder{
		{1, text("north"), "2026-01-01", number(100), true}, {2, text("north"), "2026-01-01", number(50), true},
		{3, nil, "2026-01-01", number(30), true}, {4, text("unknown"), "2026-01-01", nil, true},
		{5, text("zero"), "2026-01-01", number(0), true}, {6, text("no-refund"), "2026-01-01", number(12), true},
		{7, text("cancelled-only"), "2026-01-01", number(20), false}, {8, text("old-parent"), "2025-01-01", number(60), true},
	}
	refunds := []factOracleRefund{
		{731042619, 1, "2026-01-01", number(20)}, {731042620, 1, "2026-01-01", number(5)}, {731042621, 2, "2026-01-01", nil},
		{731042622, 3, "2026-01-01", number(25)}, {731042623, 4, "2026-01-01", nil}, {731042624, 5, "2026-01-01", number(0)},
		{731042625, 7, "2026-01-01", number(20)}, {731042626, 8, "2026-01-01", number(20)}, {731042627, 1, "2027-01-01", number(22)},
		{731042628, 5, "2026-01-01", nil},
	}
	for _, o := range orders {
		status := "cancelled"
		if o.paid {
			status = "paid"
		}
		if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.customers VALUES($1,'consumer',$2)`, o.id, o.region); err != nil {
			t.Fatal(err)
		}
		if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES($1,$1,$2,$3,$4)`, o.id, o.date, o.amount, status); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range refunds {
		if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.refunds VALUES($1,$2,$3,$4)`, r.id, r.order, r.date, r.amount); err != nil {
			t.Fatal(err)
		}
	}
	pack, _ := liveCommerceTopics(t, f)
	facts := []string{pack.Datasets[0].ID, pack.Datasets[3].ID}
	sort.Strings(facts)
	domain := exec.AnalyticalGroupDomainQualifying
	if raw {
		domain = exec.AnalyticalGroupDomainRaw
	}
	pack.GroupedPopulation = &semantics.GroupedPopulationPolicy{Policy: semantics.GroupedPopulationUnionPolicy, Datasets: facts}
	for _, id := range facts {
		pack.GroupedPopulation.GroupDomains = append(pack.GroupedPopulation.GroupDomains, semantics.GroupedPopulationDomain{Dataset: id, Domain: domain})
	}
	for i := range pack.Joins {
		if pack.Joins[i].ID == "orders-refunds" {
			j := &pack.Joins[i]
			j.Type = semantics.JoinInner
			j.Left, j.Right = j.Right, j.Left
			j.Cardinality = semantics.CardinalityManyToOne
			j.Evidence.LeftGrain = "refund"
			j.Evidence.RightGrain = "order"
		}
	}
	pack.Measures = append(pack.Measures,
		semantics.Measure{ID: "gross_known", Name: "Known paid amount count", Field: pack.Measures[0].Field, Aggregation: semantics.AggregationCount, Filters: append([]semantics.SemanticFilter(nil), pack.Measures[0].Filters...)},
		semantics.Measure{ID: "refund_known", Name: "Known refund amount count", Field: pack.Measures[1].Field, Aggregation: semantics.AggregationCount})
	pack.KPIs = append(pack.KPIs, semantics.KPI{ID: "known_difference", Name: "Known amount count difference", Expression: "gross_known - refund_known", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "gross_known"}, {Kind: semantics.KindMeasure, ID: "refund_known"}}})
	if periods {
		pack.Dimensions[0].Temporal.Grains = append(pack.Dimensions[0].Temporal.Grains, semantics.GrainYear)
		pack.Dimensions = append(pack.Dimensions, semantics.Dimension{ID: "refund_date", Name: "Refund date", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[3].ID, ID: "refunded_at"}, Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainYear}}})
		for i, pair := range [][2]string{{"gross_revenue", "refund_amount"}, {"gross_known", "refund_known"}} {
			bindings := []semantics.MetricPeriodBinding{{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: pair[0]}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "order_month"}}, {Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: pair[1]}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "refund_date"}}}
			sort.Slice(bindings, func(i, j int) bool { return bindings[i].Measure.ID < bindings[j].Measure.ID })
			pack.KPIs[i].Periods = &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: bindings}
		}
	}
	model := newGatewayFixture(t, func(c *config.Gateway) {
		r := c.Roles["embedding"]
		r.MaxBatchItems = 64
		r.MaxBatchBytes = 4096
		c.Roles["embedding"] = r
	})
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := topics.New(f.db, f.s, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	author := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	published := phase17PublishTopic(t, draft, topic, author, pack)
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(topic, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	query, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	actor := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase18Scopes(f.e.Tenant(), true)...)
	amountTarget := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[3].ID, ID: "amount_usd"}
	idTarget := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[3].ID, ID: "refund_id"}
	nulls := "include"
	if onlyNull {
		nulls = "only"
	}
	slots := []semantics.ClarificationSlot{
		{ID: "minimum_id", Prompt: "Which minimum refund identity?", Required: true, Kind: semantics.SlotNumber, Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Kind: "number", Target: idTarget, Operator: "gte", Nulls: "exclude", Unit: "count", Precision: 19}},
		{ID: "minimum_amount", Prompt: "Which refund amount population?", Required: true, Kind: semantics.SlotNumber, Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Kind: "number", Target: amountTarget, Operator: "gte", Nulls: nulls, Unit: "USD", Precision: 12, Scale: 2}},
	}
	targets := []semantics.Reference{idTarget, amountTarget}
	if selected {
		target := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[2].ID, ID: "region"}
		targets = append(targets, target)
		slots = append(slots, semantics.ClarificationSlot{ID: "region", Prompt: "Which final group?", Required: true, Kind: semantics.SlotText, Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Kind: "entity", Target: target, Operator: "eq", Nulls: "exclude", MaxLength: 32, Values: []semantics.GovernedClarificationValue{{Canonical: "north", Label: "North group"}}}})
	}
	pattern := semantics.ClarificationPattern{ID: "fact_population", Version: "v1", Targets: targets, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-fact-row-review"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"reviewed refunds"}}, Why: "Restrict only the refund fact before grouping."}, Slots: slots}
	definition := semantics.RuleSetDefinition{SchemaVersion: 1, ID: "fact-predicate-rules", Version: "rules-v1", Topic: published.State.Topic, TopicVersion: published.State.Version, PackDigest: published.Digest, Patterns: []semantics.ClarificationPattern{pattern}}
	rd, err := rules.Save(t.Context(), actor, rulesets.SaveRequest{Definition: definition, Change: "Review exact refund predicates"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := rules.Review(t.Context(), actor, pack.Topic, rulesets.ReviewRequest{DraftRevision: rd.Revision, Digest: rd.Digest, Decision: "approve", Note: "Synthetic exact fact ownership"})
	if err != nil {
		t.Fatal(err)
	}
	ruleState, err := rules.Publish(t.Context(), actor, pack.Topic, rulesets.PublishRequest{Review: review.ID})
	if err != nil {
		t.Fatal(err)
	}
	rowDomain := " WHERE o.status='paid'"
	if raw {
		rowDomain = ""
	}
	sql := `WITH g AS (SELECT c.region AS region,SUM(o.total_usd) FILTER(WHERE o.status='paid') AS value,COUNT(o.total_usd) FILTER(WHERE o.status='paid') AS known FROM analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id` + rowDomain + ` GROUP BY c.region), r AS (SELECT c.region AS region,SUM(r.amount_usd) AS value,COUNT(r.amount_usd) AS known FROM analytics.refunds r JOIN analytics.orders o ON r.order_id=o.order_id JOIN analytics.customers c ON o.customer_id=c.customer_id GROUP BY c.region), keys AS (SELECT region FROM g UNION SELECT region FROM r) SELECT keys.region,g.value-r.value AS net,g.known-r.known AS known_difference FROM keys LEFT JOIN g ON keys.region IS NOT DISTINCT FROM g.region LEFT JOIN r ON keys.region IS NOT DISTINCT FROM r.region`
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	model.mode.Store(phase18RawResponse(t, sql))
	question := "Net revenue and Known amount count difference for reviewed refunds"
	if periods {
		question += " in 2026"
	}
	question += " by Customer region"
	request := nlqexec.PlanRequest{Operation: "fact-plan", QuestionRequest: nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: question, Locale: nlq.LanguageEnglish, MetricIDs: []string{"net_revenue", "known_difference"}, Kinds: []string{"kpi", "dimension"}, LimitPerKind: 10}}
	pending, err := query.Preflight(t.Context(), actor, nlqexec.PreflightRequest{QuestionRequest: request.QuestionRequest})
	if err != nil || pending.Route.Clarification == nil {
		t.Fatal("fact preflight", err)
	}
	answer := func(slot string, v semantics.ClarificationValue) semantics.ClarificationAnswer {
		return semantics.ClarificationAnswer{Topic: pack.Topic, TopicVersion: published.State.Version, RulesetVersion: definition.Version, Pattern: pattern.ID, PatternVersion: "v1", Slot: slot, Value: &v}
	}
	answers := func(minID int) []semantics.ClarificationAnswer {
		amount := semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: "17", Unit: "USD"}}
		if onlyNull {
			amount = semantics.ClarificationValue{Null: true}
		}
		out := []semantics.ClarificationAnswer{answer("minimum_id", semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: strconv.Itoa(minID), Unit: "count"}}), answer("minimum_amount", amount)}
		if selected {
			region := "north"
			out = append(out, answer("region", semantics.ClarificationValue{Text: &region}))
		}
		return out
	}
	request.ClarificationQuery, request.AnswerContext = pending.QueryID, pending.Route.AnswerContext
	request.Answers = answers(731042619)
	server := httptest.NewServer(nlqapi.ExecutionHandler(f.token.verifier, query, http.NotFoundHandler()))
	defer server.Close()
	claims := f.token.claims(actor.Tenant(), actor.User(), phase18Scopes(actor.Tenant(), true))
	claims["session"] = actor.Session()
	bearer := f.token.sign(t, claims, nil)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	plan, err := client.PlanNLQ(t.Context(), request)
	if err != nil {
		t.Fatal("fact Plan", err)
	}
	if plan.Analytical == nil || plan.Analytical.Version != exec.AnalyticalGroupedFactsVersion || plan.Bindings == nil || plan.Bindings.SchemaVersion != 5 || plan.Bindings.PopulationPolicy != exec.AnalyticalGroupedFactPolicy {
		t.Fatal("missing distinct fact receipt")
	}
	scope, _ := store.NewScope(actor.Tenant(), actor.User())
	stored, err := f.db.ReadQuery(t.Context(), scope, plan.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AnalyticalVersion != 12 || stored.Clarification == nil || stored.Clarification.BaseSQL != sql || len(stored.Clarification.BaseParameters) != 0 {
		t.Fatal("fact base custody")
	}
	for _, effect := range plan.Bindings.Bindings {
		if effect.Column == "refund_id" || effect.Column == "amount_usd" {
			if effect.Population != pack.Datasets[3].ID {
				t.Fatal("fact effect lost ownership")
			}
		}
		if effect.Column == "region" && effect.Population != "" {
			t.Fatal("final group became a fact restriction")
		}
	}
	assertRows := func(result nlqexec.RunResult, minID int) {
		t.Helper()
		if result.Execution.Result == nil {
			t.Fatal("missing result")
		}
		want := groupedFactOracle(orders, refunds, minID, raw, onlyNull, periods, selected)
		if len(result.Execution.Result.Rows) != len(want) {
			t.Fatalf("group count got %d want %d", len(result.Execution.Result.Rows), len(want))
		}
		for _, row := range result.Execution.Result.Rows {
			k := "NULL"
			if string(row[0]) != "null" && json.Unmarshal(row[0], &k) != nil {
				t.Fatal("key")
			}
			expected, ok := want[k]
			if !ok {
				t.Fatal("unexpected group", k)
			}
			delete(want, k)
			for i, w := range expected {
				got := "NULL"
				if string(row[i+1]) != "null" && json.Unmarshal(row[i+1], &got) != nil {
					got = string(row[i+1])
				}
				if got != w && (got == "NULL" || w == "NULL" || !liveNumberEquals(got, w)) {
					t.Fatalf("independent oracle group %s output %d got %s want %s", k, i, got, w)
				}
			}
		}
	}
	run := nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "fact-run"}
	result, err := client.RunNLQ(t.Context(), run)
	if err != nil {
		t.Fatal("fact Run", err)
	}
	assertRows(result, 731042619)
	metadata := support.Raw(t, f.dsn)
	calls, reads := model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := restarted.Run(t.Context(), actor, run)
	if err != nil {
		t.Fatal("fact terminal replay", err)
	}
	assertRows(replay, 731042619)
	repeated, err := restarted.Plan(t.Context(), actor, request)
	if err != nil || repeated.QueryID != plan.QueryID {
		t.Fatal("fact Plan replay", err)
	}
	if calls != model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("terminal replay repeated model or source execution")
	}
	for _, fault := range []string{"schema", "policy", "population", "ordinal", "parameter", "base", "source", "context", "version", "query_digest", "constraint_digest"} {
		bad, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, groupedFactFault{Repository: f.db, fault: fault})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bad.Run(t.Context(), actor, run); err == nil {
			t.Fatal("tampered fact replay accepted", fault)
		}
		if err := bad.Feedback(t.Context(), actor, nlqexec.FeedbackRequest{QueryID: plan.QueryID, Verdict: "positive"}); err == nil {
			t.Fatal("tampered fact evidence learned a reusable base", fault)
		}
	}
	if calls != model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("tamper rejection executed work")
	}
	if err := query.Feedback(t.Context(), actor, nlqexec.FeedbackRequest{QueryID: plan.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal("fact feedback", err)
	}
	examples, err := query.Examples(t.Context(), actor, pack.Topic, 8)
	if err != nil || len(examples) != 1 || examples[0].Origin.BindingPolicy != nlqexec.ScopedGroupedFactExamplePolicy || examples[0].SQL != sql || examples[0].ParameterSchema != nil || examples[0].State != "candidate" {
		t.Fatal("schema5 lost distinct value-free owned learning base", err)
	}
	// Establish a genuinely current, reviewed schema-1 owned example, then
	// prove a schema-5 request excludes it before prompt selection. This is a
	// consumption negative, not merely the absence of a learning producer.
	if !raw && !onlyNull && !periods && !selected {
		ordinary := request
		ordinary.Operation = "fact-learning-control"
		ordinary.Question = "Refund amount for reviewed refunds"
		ordinary.MetricIDs = []string{"refund_amount"}
		ordinary.Kinds = []string{"measure"}
		ordinary.ClarificationQuery, ordinary.AnswerContext = "", ""
		ordinary.Answers = nil
		pending, err := query.Preflight(t.Context(), actor, nlqexec.PreflightRequest{QuestionRequest: ordinary.QuestionRequest})
		if err != nil {
			t.Fatal("ordinary learning preflight", err)
		}
		ordinary.ClarificationQuery, ordinary.AnswerContext = pending.QueryID, pending.Route.AnswerContext
		ordinary.Answers = answers(731042619)
		model.mode.Store(phase18RawResponse(t, "SELECT SUM(r.amount_usd) AS amount FROM analytics.refunds r"))
		control, err := query.Plan(t.Context(), actor, ordinary)
		if err != nil || control.Bindings == nil || control.Bindings.SchemaVersion != 1 {
			t.Fatal("ordinary learning control", err)
		}
		if _, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: control.QueryID, Operation: "fact-learning-control-run"}); err != nil {
			t.Fatal(err)
		}
		if err := query.Feedback(t.Context(), actor, nlqexec.FeedbackRequest{QueryID: control.QueryID, Verdict: "positive"}); err != nil {
			t.Fatal(err)
		}
		controls, err := query.Examples(t.Context(), actor, pack.Topic, 8)
		if err != nil || len(controls) != 2 {
			t.Fatal("missing authentic owned example control", err)
		}
		var ordinaryBase nlqexec.ExampleRecord
		for _, example := range controls {
			if example.Origin.BindingPolicy == nlqexec.OwnedExamplePolicy {
				ordinaryBase = example
			}
		}
		if ordinaryBase.ID == "" {
			t.Fatal("missing authentic ordinary owned control")
		}
		active, err := query.ExampleState(t.Context(), actor, nlqexec.ExampleStateRequest{ExampleID: ordinaryBase.ID, ExpectedVersion: ordinaryBase.Version, State: "active", ReviewNote: "Reviewed current ordinary base"})
		if err != nil {
			t.Fatal(err)
		}
		model.mode.Store(phase18RawResponse(t, sql))
		current := request
		current.Operation = "fact-excludes-owned-example"
		planned, err := query.Plan(t.Context(), actor, current)
		if err != nil {
			t.Fatal("fact consumption negative", err)
		}
		evidence, err := f.db.ReadQuery(t.Context(), scope, planned.QueryID)
		if err != nil || evidence.ExampleSelection.Eligibility == nil || evidence.ExampleSelection.Eligibility.CurrentOwnedPredicates || evidence.ExampleSelection.Eligibility.CurrentScopedPolicy != nlqexec.ScopedGroupedFactExamplePolicy {
			t.Fatal("schema5 advertised the wrong owned-example eligibility", err)
		}
		for _, pick := range evidence.ExampleSelection.Selected {
			if pick.ExampleID == active.ID {
				t.Fatal("schema5 consumed an old owned binder family")
			}
		}
	}
	refined, err := query.Refine(t.Context(), actor, nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: answers(731042625)}})
	if err != nil {
		t.Fatal("current fact replacement", err)
	}
	child, err := f.db.ReadQuery(t.Context(), scope, refined.QueryID)
	if err != nil || child.Clarification == nil || child.Clarification.BaseSQL != sql {
		t.Fatal("replacement lost original unbound base", err)
	}
	for _, p := range child.Parameters {
		if p.Value == "731042619" {
			t.Fatal("historical scalar survived replacement")
		}
	}
	changedRun := nlqexec.RunRequest{QueryID: refined.QueryID, Operation: "fact-current-run"}
	changed, err := query.Run(t.Context(), actor, changedRun)
	if err != nil {
		t.Fatal("current fact Run", err)
	}
	assertRows(changed, 731042625)
	calls, reads = model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	replay, err = restarted.Run(t.Context(), actor, changedRun)
	if err != nil {
		t.Fatal("current fact terminal replay", err)
	}
	assertRows(replay, 731042625)
	if calls != model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("current replay repeated work")
	}
	for _, mutation := range []string{`analytical_version=11`, `analytical=analytical-'outputs'`, `clarification=jsonb_set(clarification,'{binding,schema_version}','4')`} {
		if _, err := metadata.Exec(t.Context(), `UPDATE chartworks.nlq_queries SET `+mutation+` WHERE query_id=$1`, plan.QueryID); err == nil {
			t.Fatal("immutable fact evidence mutated")
		}
	}
	// Neither exact fact placement nor a valid receipt grants source/actor reach.
	for _, foreign := range []string{"other-tenant", ""} {
		tenant := foreign
		if tenant == "" {
			tenant = actor.Tenant()
		}
		denied := f.token.envelope(t, tenant, "other-user", phase18Scopes(tenant, true)...)
		if _, err := query.Run(t.Context(), denied, run); err == nil {
			t.Fatal("foreign fact replay accepted")
		}
	}
	model.mu.Lock()
	prompts := strings.Join(model.requestBodies, "\n")
	model.mu.Unlock()
	public, _ := json.Marshal([]any{plan.Analytical, plan.Bindings})
	if strings.Contains(prompts, "731042619") || strings.Contains(string(public), "731042619") {
		t.Fatal("private fact scalar entered provider or receipt")
	}
	// Authenticated business-predicate replay re-admits current rules even
	// when the outer query has historical pins. Preserve that narrower existing
	// policy: retired answers cannot authorize fresh execution or retained values.
	if _, err := rules.Retire(t.Context(), actor, pack.Topic, rulesets.RetireRequest{Expected: ruleState.State.Revision, Note: "test current predicate authority"}); err != nil {
		t.Fatal("rule retirement", err)
	}
	calls, reads = model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	if _, err := restarted.Run(t.Context(), actor, run); err == nil {
		t.Fatal("retired business rules accepted terminal fact replay")
	}
	fresh := request
	fresh.Operation = "fact-retired-rules"
	if _, err := query.Plan(t.Context(), actor, fresh); err == nil {
		t.Fatal("retired answers acquired fresh current-rule admission")
	}
	if calls != model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("retired rule denial did model or source work")
	}

}

type groupedFactFault struct {
	nlqexec.Repository
	fault string
}

func (r groupedFactFault) ReadQuery(ctx context.Context, scope store.Scope, id string) (nlqexec.QueryRecord, error) {
	q, err := r.Repository.ReadQuery(ctx, scope, id)
	if err != nil || q.AnalyticalVersion != 12 {
		return q, err
	}
	proof := *q.Analytical
	proof.Outputs = append([]exec.AnalyticalOutput(nil), proof.Outputs...)
	q.Analytical = &proof
	evidence := *q.Clarification
	evidence.Binding.Bindings = append([]exec.BusinessParameterBinding(nil), evidence.Binding.Bindings...)
	q.Clarification = &evidence
	switch r.fault {
	case "schema":
		evidence.Binding.SchemaVersion = 4
	case "policy":
		evidence.Binding.PopulationPolicy = exec.AnalyticalGroupedSelectionPolicy
	case "population":
		evidence.Binding.Bindings[0].Population = ""
	case "ordinal":
		proof.Outputs[0].Column = 0
	case "parameter":
		q.Parameters = append([]exec.Parameter(nil), q.Parameters...)
		q.Parameters[0].Value = "991"
	case "base":
		evidence.BaseSQL += " WHERE FALSE"
	case "source":
		q.Route.SourceBindingDigest = exec.Hash("changed source")
	case "context":
		q.Context = "foreign-context"
	case "version":
		q.AnalyticalVersion = 11
	case "query_digest":
		proof.Query = exec.Hash("changed query")
	case "constraint_digest":
		evidence.Binding.Constraints = exec.Hash("changed constraints")
	}
	return q, nil
}
