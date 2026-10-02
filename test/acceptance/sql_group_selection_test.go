package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
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

// The existing row oracle computes the full independent groups before this
// selection. It does not read generated SQL, private receipts, or compiler state.
func TestSQLRecoveryGroupedSelectionAcceptance(t *testing.T) {
	for _, withPeriods := range []bool{false, true} {
		for _, raw := range []bool{false, true} {
			t.Run(fmt.Sprintf("periods_%t_raw_%t", withPeriods, raw), func(t *testing.T) { runGroupedSelectionAcceptance(t, withPeriods, raw, false) })
		}
	}
	t.Run("null_group_without_periods", func(t *testing.T) { runGroupedSelectionAcceptance(t, false, true, true) })
	t.Run("null_group_with_periods", func(t *testing.T) { runGroupedSelectionAcceptance(t, true, false, true) })
}

func runGroupedSelectionAcceptance(t *testing.T, withPeriods, raw, nullOnly bool) {
	t.Helper()
	f := liveCommerceSource(t)
	if _, err := f.admin.Exec(t.Context(), `DELETE FROM analytics.order_items; DELETE FROM analytics.refunds; DELETE FROM analytics.orders; DELETE FROM analytics.customers;
ALTER TABLE analytics.customers ALTER COLUMN region DROP NOT NULL; ALTER TABLE analytics.customers DROP CONSTRAINT customers_region_check;
ALTER TABLE analytics.orders ALTER COLUMN total_usd DROP NOT NULL; ALTER TABLE analytics.refunds ALTER COLUMN amount_usd DROP NOT NULL;`); err != nil {
		t.Fatal(err)
	}
	text := func(s string) *string { return &s }
	amount := func(n int) *int { return &n }
	orders := []groupedOwnedOrder{
		{1, text("PRIVATE_GROUP_731"), "2026-01-01", amount(100), true}, {2, text("PRIVATE_GROUP_731"), "2025-01-01", amount(50), true},
		{3, nil, "2026-01-01", amount(30), true}, {4, text("unknown"), "2026-01-01", nil, true},
		{5, text("zero"), "2026-01-01", amount(0), true}, {6, text("missing"), "2026-01-01", amount(12), true},
		{7, text("cancelled-only"), "2026-01-01", amount(20), false}, {8, text("refund-only"), "2025-01-01", amount(60), true},
		{9, text("future-parent"), "2027-01-01", amount(10), true},
	}
	refunds := []groupedOwnedRefund{{1, "2027-01-01", amount(10)}, {2, "2026-01-01", amount(15)}, {3, "2026-01-01", amount(5)}, {4, "2026-01-01", nil}, {5, "2026-01-01", amount(0)}, {8, "2026-01-01", amount(7)}, {9, "2026-01-01", amount(2)}, {1, "2026-01-01", nil}}
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
	for i, r := range refunds {
		if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.refunds VALUES($1,$2,$3,$4)`, 200+i, r.order, r.date, r.amount); err != nil {
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
			pack.Joins[i].Type = semantics.JoinInner
			pack.Joins[i].Left, pack.Joins[i].Right = pack.Joins[i].Right, pack.Joins[i].Left
			pack.Joins[i].Cardinality = semantics.CardinalityManyToOne
			pack.Joins[i].Evidence.LeftGrain = "refund"
			pack.Joins[i].Evidence.RightGrain = "order"
		}
	}
	pack.Dimensions[0].Temporal.Grains = append(pack.Dimensions[0].Temporal.Grains, semantics.GrainYear)
	pack.Dimensions = append(pack.Dimensions, semantics.Dimension{ID: "refund_date", Name: "Refund date", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[3].ID, ID: "refunded_at"}, Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainYear}}})
	refundAxis := "order_month"
	if withPeriods {
		refundAxis = "refund_date"
	}
	periods := func(gross, refund string) *semantics.MetricPeriodBindings {
		bindings := []semantics.MetricPeriodBinding{{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: gross}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "order_month"}}, {Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: refund}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: refundAxis}}}
		sort.Slice(bindings, func(i, j int) bool { return bindings[i].Measure.ID < bindings[j].Measure.ID })
		return &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: bindings}
	}
	if withPeriods {
		pack.KPIs[0].Periods = periods("gross_revenue", "refund_amount")
	}
	pack.Measures = append(pack.Measures, semantics.Measure{ID: "gross_known", Name: "Known paid amount count", Field: pack.Measures[0].Field, Aggregation: semantics.AggregationCount, Filters: append([]semantics.SemanticFilter(nil), pack.Measures[0].Filters...)}, semantics.Measure{ID: "refund_known", Name: "Known refund amount count", Field: pack.Measures[1].Field, Aggregation: semantics.AggregationCount})
	pack.KPIs = append(pack.KPIs, semantics.KPI{ID: "known_difference", Name: "Known amount count difference", Expression: "gross_known - refund_known", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "gross_known"}, {Kind: semantics.KindMeasure, ID: "refund_known"}}})
	if withPeriods {
		pack.KPIs[len(pack.KPIs)-1].Periods = periods("gross_known", "refund_known")
	}
	model := newGatewayFixture(t, func(cfg *config.Gateway) {
		r := cfg.Roles["embedding"]
		r.MaxBatchItems = 64
		r.MaxBatchBytes = 4096
		cfg.Roles["embedding"] = r
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
	target := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[2].ID, ID: "region"}
	nulls := "exclude"
	if nullOnly {
		nulls = "only"
	}
	values := []semantics.GovernedClarificationValue{}
	for _, key := range []string{"PRIVATE_GROUP_731", "unknown", "zero", "missing", "cancelled-only", "refund-only", "future-parent", "absent"} {
		values = append(values, semantics.GovernedClarificationValue{Canonical: key, Label: "Reviewed group", Aliases: []string{"choice-" + fmt.Sprint(len(values))}})
	}
	effect := &semantics.ClarificationEffect{Kind: "entity", Target: target, Operator: "eq", Nulls: nulls, MaxLength: 64, Values: values}
	pattern := semantics.ClarificationPattern{ID: "group", Version: "v1", Targets: []semantics.Reference{target}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-reviewed-group-selection"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"selected groups"}}, Why: "Select complete aligned groups."}, Slots: []semantics.ClarificationSlot{{ID: "region", Prompt: "Which reviewed group?", Required: true, Kind: semantics.SlotText, Sensitivity: semantics.LiteralSensitive, Effect: effect}}}
	definition := semantics.RuleSetDefinition{SchemaVersion: 1, ID: "selected-group-rules", Version: "rules-v1", Topic: published.State.Topic, TopicVersion: published.State.Version, PackDigest: published.Digest, Patterns: []semantics.ClarificationPattern{pattern}}
	ruleDraft, err := rules.Save(t.Context(), actor, rulesets.SaveRequest{Definition: definition, Change: "Review final group selection"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := rules.Review(t.Context(), actor, pack.Topic, rulesets.ReviewRequest{DraftRevision: ruleDraft.Revision, Digest: ruleDraft.Digest, Decision: "approve", Note: "Synthetic group selection"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rules.Publish(t.Context(), actor, pack.Topic, rulesets.PublishRequest{Review: review.ID}); err != nil {
		t.Fatal(err)
	}
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	rowDomain := " WHERE o.status='paid'"
	if raw {
		rowDomain = ""
	}
	sql := `WITH g AS (SELECT c.region AS region,SUM(o.total_usd) FILTER(WHERE o.status='paid') AS value,COUNT(o.total_usd) FILTER(WHERE o.status='paid') AS known FROM analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id` + rowDomain + ` GROUP BY c.region), r AS (SELECT c.region AS region,SUM(r.amount_usd) AS value,COUNT(r.amount_usd) AS known FROM analytics.refunds r JOIN analytics.orders o ON r.order_id=o.order_id JOIN analytics.customers c ON o.customer_id=c.customer_id GROUP BY c.region), keys AS (SELECT region FROM g UNION SELECT region FROM r) SELECT keys.region,g.value-r.value AS net,g.known-r.known AS known_difference FROM keys LEFT JOIN g ON keys.region IS NOT DISTINCT FROM g.region LEFT JOIN r ON keys.region IS NOT DISTINCT FROM r.region`

	model.mode.Store(phase18RawResponse(t, sql))
	question := "Net revenue and Known amount count difference for selected groups"
	if withPeriods {
		question += " in 2026"
	}
	question += " by Customer region"
	request := nlqexec.PlanRequest{Operation: "selected-group-plan", QuestionRequest: nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: question, Locale: nlq.LanguageEnglish, MetricIDs: []string{"net_revenue", "known_difference"}, Kinds: []string{"kpi", "dimension"}, LimitPerKind: 10}}
	model.mu.Lock()
	providerStart := len(model.requestBodies)
	model.mu.Unlock()
	pending, err := query.Preflight(t.Context(), actor, nlqexec.PreflightRequest{QuestionRequest: request.QuestionRequest})
	if err != nil || pending.Route.Clarification == nil {
		t.Fatal("group preflight", err)
	}
	answer := func(key string) semantics.ClarificationAnswer {
		v := semantics.ClarificationValue{Text: &key}
		if nullOnly {
			v = semantics.ClarificationValue{Null: true}
		}
		return semantics.ClarificationAnswer{Topic: pack.Topic, TopicVersion: published.State.Version, RulesetVersion: definition.Version, Pattern: "group", PatternVersion: "v1", Slot: "region", Value: &v}
	}
	request.ClarificationQuery, request.AnswerContext = pending.QueryID, pending.Route.AnswerContext
	selected := "PRIVATE_GROUP_731"
	if nullOnly {
		selected = "NULL"
	}
	request.Answers = []semantics.ClarificationAnswer{answer(selected)}
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
		t.Fatal("selected-group Plan", err)
	}
	if plan.Analytical == nil || plan.Analytical.Version != exec.AnalyticalGroupedSelectionVersion || plan.Bindings == nil || plan.Bindings.SchemaVersion != 4 {
		t.Fatal("missing selected-group proof")
	}
	scope, _ := store.NewScope(actor.Tenant(), actor.User())
	stored, err := f.db.ReadQuery(t.Context(), scope, plan.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	wantParameters := 1
	if nullOnly {
		wantParameters = 0
	}
	if withPeriods {
		wantParameters += 4
	}
	if stored.AnalyticalVersion != 11 || stored.Clarification == nil || stored.Clarification.BaseSQL != sql || len(stored.Clarification.BaseParameters) != 0 || len(stored.Parameters) != wantParameters {
		t.Fatal("private group base custody")
	}
	assertResult := func(result nlqexec.RunResult, key string) {
		t.Helper()
		if result.Execution.Result == nil {
			t.Fatal("missing selected result")
		}
		year := ""
		if withPeriods {
			year = "2026"
		}
		full := groupedOwnedOracle(orders, refunds, year, withPeriods, raw, false)
		expected, exists := full[key]
		rows := result.Execution.Result.Rows
		if !exists {
			if len(rows) != 0 {
				t.Fatal("selection invented a group")
			}
			return
		}
		if len(rows) != 1 || len(rows[0]) != 3 {
			t.Fatal("selection dropped or duplicated a group")
		}
		gotKey := "NULL"
		if string(rows[0][0]) != "null" {
			if json.Unmarshal(rows[0][0], &gotKey) != nil {
				t.Fatal("invalid key")
			}
		}
		if gotKey != key {
			t.Fatal("wrong complete group")
		}
		for i, want := range expected {
			got := "NULL"
			if string(rows[0][i+1]) != "null" && json.Unmarshal(rows[0][i+1], &got) != nil {
				got = string(rows[0][i+1])
			}
			if got != want && (got == "NULL" || want == "NULL" || !liveNumberEquals(got, want)) {
				t.Fatalf("group oracle output %d got %s want %s", i, got, want)
			}
		}
	}
	runRequest := nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "selected-group-run"}
	result, err := query.Run(t.Context(), actor, runRequest)
	if err != nil {
		t.Fatal("selected-group Run", err)
	}
	assertResult(result, selected)
	metadata := support.Raw(t, f.dsn)
	calls, attempts := model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := client.RunNLQ(t.Context(), runRequest)
	if err != nil {
		t.Fatal("selected-group replay", err)
	}
	assertResult(replay, selected)
	repeated, err := restarted.Plan(t.Context(), actor, request)
	if err != nil || repeated.QueryID != plan.QueryID {
		t.Fatal("selected-group Plan replay", err)
	}
	if calls != model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("terminal replay repeated work")
	}
	for _, fault := range []string{"ordinal", "schema", "policy", "constraint_digest", "source_digest", "validation_source", "validation_context", "indexes", "sql", "parameter", "final_population", "route_digest", "foreign_context"} {
		faulted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, groupSelectionFault{Repository: f.db, fault: fault})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = faulted.Run(t.Context(), actor, runRequest); err == nil {
			t.Fatal("corrupted selected-group replay accepted", fault)
		}
		if calls != model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
			t.Fatal("corrupt selected-group replay did work", fault)
		}
	}
	for _, actor := range []identity.Envelope{f.token.envelope(t, "other-tenant", actor.User(), phase18Scopes("other-tenant", true)...), f.token.envelope(t, actor.Tenant(), "other-user", phase18Scopes(actor.Tenant(), true)...), f.token.envelope(t, actor.Tenant(), actor.User(), "query.execute")} {
		if _, err = query.Run(t.Context(), actor, runRequest); err == nil {
			t.Fatal("foreign or missing authority accepted")
		}
	}
	if calls != model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("authority denial did work")
	}
	// Successful feedback records the vote but cannot learn schema4 private SQL.
	if err = query.Feedback(t.Context(), actor, nlqexec.FeedbackRequest{QueryID: plan.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal("feedback", err)
	}
	examples, err := query.Examples(t.Context(), actor, pack.Topic, 8)
	if err != nil || len(examples) != 0 {
		t.Fatal("scoped binding became reusable learning", err)
	}
	saved := nlqexec.SavedQuestion{Durability: "session_bound", Context: request.Context, Topics: []nlqexec.SavedTopic{{Topic: pack.Topic, Version: published.State.Version, Digest: published.Digest}}, Query: plan.QueryID}
	evidence, err := query.InspectSaved(t.Context(), actor, saved)
	if err != nil {
		t.Fatal("saved inspection", err)
	}
	copy, err := query.PrepareSaved(t.Context(), actor, saved, evidence, "selected-group-copy", "en")
	if err != nil {
		t.Fatal("saved copy", err)
	}
	savedResult, err := query.RunSaved(t.Context(), actor, saved, evidence, copy, 100, 1<<20, false)
	if err != nil {
		t.Fatal("saved execution", err)
	}
	assertResult(nlqexec.RunResult{Execution: savedResult.Execution}, selected)
	if !nullOnly {
		for _, key := range []string{"unknown", "zero", "missing", "cancelled-only", "refund-only", "future-parent", "absent"} {
			refined, err := query.Refine(t.Context(), actor, nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{answer(key)}}})
			if err != nil {
				t.Fatal("selected-group refinement", key, err)
			}
			changed, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: refined.QueryID, Operation: "selected-group-" + key})
			if err != nil {
				t.Fatal("refined Run", key, err)
			}
			assertResult(changed, key)
		}
	}
	model.mu.Lock()
	bodies := append([]string(nil), model.requestBodies...)
	prompts := strings.Join(bodies, "\n")
	model.mu.Unlock()
	// Authorized Route JSON retains canonical answers for editable continuation.
	// Only binding and analytical receipts promise a value-free public projection.
	public, _ := json.Marshal([]any{plan.Analytical, plan.Bindings})
	if strings.Contains(prompts, "PRIVATE_GROUP_731") || strings.Contains(string(public), "PRIVATE_GROUP_731") || strings.Contains(fmt.Sprintf("%+v %#v", stored.Clarification, stored.Clarification), "PRIVATE_GROUP_731") {
		for i, body := range bodies {
			if strings.Contains(body, "PRIVATE_GROUP_731") {
				reportGroupCanaryPaths(t, fmt.Sprintf("provider_request[%d] before_nlq=%t", i, i < providerStart), body)
			}
		}
		reportGroupCanaryPaths(t, "public_proof", string(public))
		t.Fatalf("private group boundary: provider=%t public=%t log=%t", strings.Contains(prompts, "PRIVATE_GROUP_731"), strings.Contains(string(public), "PRIVATE_GROUP_731"), strings.Contains(fmt.Sprintf("%+v %#v", stored.Clarification, stored.Clarification), "PRIVATE_GROUP_731"))
	}
	// Native model parameters and predicates in any guessed lane/spine remain denied.
	lanePredicate := " WHERE c.region IS NOT NULL"
	if !raw {
		lanePredicate = " AND c.region IS NOT NULL"
	}
	invalid := []string{sql + " WHERE keys.region='PRIVATE_GROUP_731'", strings.Replace(sql, "SELECT region FROM r)", "SELECT region FROM r WHERE region IS NOT NULL)", 1), strings.Replace(sql, " GROUP BY c.region), r AS", lanePredicate+" GROUP BY c.region), r AS", 1), sql + " LIMIT 1", strings.Replace(sql, "g.value-r.value AS net", "g.value-r.value+$1 AS net", 1)}
	for i, badSQL := range invalid {
		model.mode.Store(phase18RawResponse(t, badSQL))
		bad := request
		bad.Operation = fmt.Sprint("selected-group-reject-", i)
		before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		if p, err := query.Plan(t.Context(), actor, bad); err == nil || p.QueryID != "" {
			t.Fatal("unowned generated placement admitted", i)
		}
		if before != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
			t.Fatal("invalid selection executed")
		}
	}
	for _, mutation := range []string{`analytical_version=10`, `analytical=analytical-'grouping'`, `clarification=jsonb_set(clarification,'{binding,schema_version}','3')`} {
		if _, err := metadata.Exec(t.Context(), `UPDATE chartworks.nlq_queries SET `+mutation+` WHERE query_id=$1`, plan.QueryID); err == nil {
			t.Fatal("immutable selected-group receipt changed")
		}
	}
}

// Corrupt only the repository read projection; persistent guards remain active.
type groupSelectionFault struct {
	nlqexec.Repository
	fault string
}

func (r groupSelectionFault) ReadQuery(ctx context.Context, scope store.Scope, id string) (nlqexec.QueryRecord, error) {
	q, err := r.Repository.ReadQuery(ctx, scope, id)
	if err != nil || q.Analytical == nil || q.Analytical.Version != exec.AnalyticalGroupedSelectionVersion {
		return q, err
	}
	copied := *q.Analytical
	copied.Outputs = append([]exec.AnalyticalOutput(nil), copied.Outputs...)
	q.Analytical = &copied
	if r.fault == "ordinal" {
		q.Analytical.Outputs[0].Column = 0
		return q, nil
	}
	if r.fault == "sql" {
		q.SQL += " WHERE FALSE"
		return q, nil
	}
	if r.fault == "parameter" {
		q.Parameters = append(q.Parameters, exec.Parameter{Kind: "text", Value: "forged"})
		return q, nil
	}
	if r.fault == "route_digest" {
		q.Route.SourceBindingDigest = exec.Hash("stale source")
		return q, nil
	}
	if r.fault == "foreign_context" {
		q.Context = "other-context"
		return q, nil
	}
	evidence := *q.Clarification
	evidence.Binding.Bindings = append([]exec.BusinessParameterBinding(nil), evidence.Binding.Bindings...)
	if evidence.Binding.Validation != nil {
		v := *evidence.Binding.Validation
		evidence.Binding.Validation = &v
	}
	switch r.fault {
	case "schema":
		evidence.Binding.SchemaVersion = 3
	case "policy":
		evidence.Binding.PopulationPolicy = exec.AnalyticalGroupedOwnedPopulationPolicy
	case "constraint_digest":
		evidence.Binding.Constraints = exec.Hash("wrong constraints")
	case "source_digest":
		evidence.Binding.SourceBinding = exec.Hash("wrong binding")
	case "validation_source":
		evidence.Binding.Validation.Source = "wrong-source"
	case "validation_context":
		evidence.Binding.Validation.Context = "wrong-context"
	case "indexes":
		evidence.Binding.Bindings[len(evidence.Binding.Bindings)-1].Parameters = []int{63}
	case "final_population":
		evidence.Binding.Bindings[len(evidence.Binding.Bindings)-1].Population = "guessed-fact"
	}
	q.Clarification = &evidence
	return q, nil
}

func reportGroupCanaryPaths(t *testing.T, label, raw string) {
	t.Helper()
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return
	}
	var visit func(string, any)
	visit = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, child := range x {
				visit(path+"."+key, child)
			}
		case []any:
			for i, child := range x {
				visit(fmt.Sprintf("%s[%d]", path, i), child)
			}
		case string:
			if at := strings.Index(x, "PRIVATE_GROUP_731"); at >= 0 {
				start, end := max(0, at-100), min(len(x), at+150)
				snippet := strings.ReplaceAll(x[start:end], "PRIVATE_GROUP_731", "[canary]")
				t.Logf("%s %s: %s", label, path, snippet)
			}
		}
	}
	visit("$", value)
}
