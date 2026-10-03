package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
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

// These are new native lifecycle witnesses for the reconstructed scoped policies,
// not evidence inherited from a prior implementation or a live-model quality run.
func TestSQLRecoveryScopedExampleRequalificationAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name              string
		schema            int
		periods, nullOnly bool
	}{
		{"scalar_periods", 2, true, false},
		{"grouped_periods", 3, true, false},
		{"group_selection_periods", 4, true, false},
		{"null_group_selection", 4, false, true},
		{"grouped_fact_periods_selection", 5, true, false},
		{"grouped_fact_null_only", 5, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newScopedRequalificationFixture(t, tc.schema, tc.periods, tc.nullOnly)
			f.qualify(t)
		})
	}
}

type scopedRequalificationFixture struct {
	f                 *engineeringFixture
	model             *gatewayFixture
	query             *nlqexec.Service
	draft             *drafts.Service
	topic             *topics.Service
	rules             *rulesets.Service
	actor, author     identity.Envelope
	pack              semantics.TopicPack
	published         topics.Published
	definition        semantics.RuleSetDefinition
	orders            []groupedOwnedOrder
	refunds           []groupedOwnedRefund
	sql               string
	schema            int
	periods, nullOnly bool
}

func newScopedRequalificationFixture(t *testing.T, schema int, withPeriods, nullOnly bool) *scopedRequalificationFixture {
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
		{1, text("PRIVATE_REQUAL_731"), "2026-01-01", amount(100), true}, {2, text("PRIVATE_REQUAL_731"), "2025-01-01", amount(50), true},
		{3, nil, "2026-01-01", amount(30), true}, {4, text("unknown"), "2026-01-01", nil, true},
		{5, text("zero"), "2026-01-01", amount(0), true}, {6, text("missing"), "2026-01-01", amount(12), true},
		{7, text("cancelled-only"), "2026-01-01", amount(20), false}, {8, text("refund-only"), "2025-01-01", amount(60), true},
		{9, text("future-parent"), "2027-01-01", amount(10), true},
		{10, text("PRIVATE_CURRENT_732"), "2025-01-01", amount(70), true},
		{11, nil, "2025-01-01", amount(25), true},
	}
	refunds := []groupedOwnedRefund{{1, "2027-01-01", amount(10)}, {2, "2026-01-01", amount(15)}, {3, "2026-01-01", amount(5)}, {4, "2026-01-01", nil}, {5, "2026-01-01", amount(0)}, {8, "2026-01-01", amount(7)}, {9, "2026-01-01", amount(2)}, {1, "2026-01-01", nil}, {10, "2025-01-01", amount(20)}, {11, "2025-01-01", amount(10)}}
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
		if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.refunds VALUES($1,$2,$3,$4)`, 731042619+i, r.order, r.date, r.amount); err != nil {
			t.Fatal(err)
		}
	}
	pack, _ := liveCommerceTopics(t, f)
	facts := []string{pack.Datasets[0].ID, pack.Datasets[3].ID}
	sort.Strings(facts)
	domain := exec.AnalyticalGroupDomainQualifying
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
	for _, key := range []string{"PRIVATE_REQUAL_731", "PRIVATE_CURRENT_732", "unknown", "zero", "missing", "cancelled-only", "refund-only", "future-parent", "absent"} {
		values = append(values, semantics.GovernedClarificationValue{Canonical: key, Label: "Reviewed group", Aliases: []string{"choice-" + fmt.Sprint(len(values))}})
	}
	effect := &semantics.ClarificationEffect{Kind: "entity", Target: target, Operator: "eq", Nulls: nulls, MaxLength: 64, Values: values}
	pattern := semantics.ClarificationPattern{ID: "group", Version: "v1", Targets: []semantics.Reference{target}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-reviewed-group-selection"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"selected groups"}}, Why: "Select complete aligned groups."}, Slots: []semantics.ClarificationSlot{{ID: "region", Prompt: "Which reviewed group?", Required: true, Kind: semantics.SlotText, Sensitivity: semantics.LiteralSensitive, Effect: effect}}}
	if schema == 5 {
		idTarget := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[3].ID, ID: "refund_id"}
		amountTarget := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[3].ID, ID: "amount_usd"}
		pattern.Targets = append(pattern.Targets, idTarget, amountTarget)
		factNulls := "include"
		if nullOnly {
			factNulls = "only"
		}
		pattern.Slots = append(pattern.Slots,
			semantics.ClarificationSlot{ID: "minimum_id", Prompt: "Which minimum refund identity?", Required: true, Kind: semantics.SlotNumber, Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Kind: "number", Target: idTarget, Operator: "gte", Nulls: "exclude", Unit: "count", Precision: 19}},
			semantics.ClarificationSlot{ID: "minimum_amount", Prompt: "Which refund amount population?", Required: true, Kind: semantics.SlotNumber, Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Kind: "number", Target: amountTarget, Operator: "gte", Nulls: factNulls, Unit: "USD", Precision: 12, Scale: 2}})
	}
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
	sql := `WITH g AS (SELECT c.region AS region,SUM(o.total_usd) FILTER(WHERE o.status='paid') AS value,COUNT(o.total_usd) FILTER(WHERE o.status='paid') AS known FROM analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id` + rowDomain + ` GROUP BY c.region), r AS (SELECT c.region AS region,SUM(r.amount_usd) AS value,COUNT(r.amount_usd) AS known FROM analytics.refunds r JOIN analytics.orders o ON r.order_id=o.order_id JOIN analytics.customers c ON o.customer_id=c.customer_id GROUP BY c.region), keys AS (SELECT region FROM g UNION SELECT region FROM r) SELECT keys.region,g.value-r.value AS net,g.known-r.known AS known_difference FROM keys LEFT JOIN g ON keys.region IS NOT DISTINCT FROM g.region LEFT JOIN r ON keys.region IS NOT DISTINCT FROM r.region`

	if schema == 2 {
		sql = `WITH g AS (SELECT SUM(o.total_usd) AS value,COUNT(o.total_usd) AS known FROM analytics.orders o WHERE o.status='paid'), r AS (SELECT SUM(r.amount_usd) AS value,COUNT(r.amount_usd) AS known FROM analytics.refunds r) SELECT g.value-r.value AS net,g.known-r.known AS known_difference FROM g CROSS JOIN r`
	}
	model.mode.Store(phase18RawResponse(t, sql))
	return &scopedRequalificationFixture{f: f, model: model, query: query, draft: draft, topic: topic, rules: rules, actor: actor, author: author, pack: pack, published: published, definition: definition, orders: orders, refunds: refunds, sql: sql, schema: schema, periods: withPeriods, nullOnly: nullOnly}
}

func (f *scopedRequalificationFixture) anchor(t *testing.T, year, selected string) nlqexec.QuestionRequest {
	t.Helper()
	question := "Net revenue and Known amount count difference"
	if f.schema == 4 || f.schema == 5 {
		question += " for selected groups"
	}
	if f.periods {
		question += " in " + year
	}
	if f.schema != 2 {
		question += " by Customer region"
	}
	request := nlqexec.QuestionRequest{Topic: f.pack.Topic, Context: f.pack.Datasets[0].Source.Context, Question: question, Locale: nlq.LanguageEnglish, MetricIDs: []string{"net_revenue", "known_difference"}, Kinds: []string{"kpi", "dimension"}, LimitPerKind: 10}
	if f.schema == 4 || f.schema == 5 {
		pending, err := f.query.Preflight(t.Context(), f.actor, nlqexec.PreflightRequest{QuestionRequest: request})
		if err != nil || pending.Route.Clarification == nil {
			t.Fatal("current group preflight", err)
		}
		value := semantics.ClarificationValue{Text: &selected}
		if f.nullOnly {
			value = semantics.ClarificationValue{Null: true}
		}
		request.ClarificationQuery, request.AnswerContext = pending.QueryID, pending.Route.AnswerContext
		request.Answers = []semantics.ClarificationAnswer{{Topic: f.pack.Topic, TopicVersion: f.published.State.Version, RulesetVersion: f.definition.Version, Pattern: "group", PatternVersion: "v1", Slot: "region", Value: &value}}
		if f.schema == 5 {
			minimum := "731042619"
			if year == "2025" {
				minimum = "731042627"
			}
			amount := semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: "17", Unit: "USD"}}
			if f.nullOnly {
				amount = semantics.ClarificationValue{Null: true}
			}
			for _, answer := range []struct {
				slot  string
				value semantics.ClarificationValue
			}{
				{"minimum_id", semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: minimum, Unit: "count"}}},
				{"minimum_amount", amount},
			} {
				value := answer.value
				request.Answers = append(request.Answers, semantics.ClarificationAnswer{Topic: f.pack.Topic, TopicVersion: f.published.State.Version, RulesetVersion: f.definition.Version, Pattern: "group", PatternVersion: "v1", Slot: answer.slot, Value: &value})
			}
		}

	}
	return request
}

func (f *scopedRequalificationFixture) republish(t *testing.T, version string, changedAggregate bool) {
	t.Helper()
	f.republishEdited(t, version, func(pack *semantics.TopicPack) {
		if changedAggregate {
			pack.Measures[0].Aggregation = semantics.AggregationAverage
		}
	})
}

func (f *scopedRequalificationFixture) republishEdited(t *testing.T, version string, edit func(*semantics.TopicPack)) {
	t.Helper()
	previous, err := f.rules.Read(t.Context(), f.actor, f.pack.Topic, "")
	if err != nil {
		t.Fatal("read prior rules", err)
	}
	current, err := f.draft.Read(t.Context(), f.author, f.pack.Topic, 0)
	if err != nil {
		t.Fatal("read author draft", err)
	}
	pack := cloneTopic(t, f.pack)
	pack.Version, pack.Description = version, "Explicitly reviewed scoped example qualification "+version
	edit(&pack)
	saved, err := f.draft.Save(t.Context(), f.author, drafts.SaveRequest{Expected: current.Metadata.Revision, Pack: pack, Change: "Current scoped example semantic review"})
	if err != nil {
		t.Fatal("save current topic", err)
	}
	review, err := f.topic.Review(t.Context(), f.author, pack.Topic, topics.ReviewRequest{DraftRevision: saved.Metadata.Revision, Digest: saved.Metadata.Digest, Decision: "approve", Note: "Independently reviewed current metric and owned scope"})
	if err != nil {
		t.Fatal("review current topic", err)
	}
	published, err := f.topic.Publish(t.Context(), f.author, pack.Topic, topics.PublishRequest{Expected: f.published.State.Revision, Review: review.ID})
	if err != nil {
		t.Fatal("publish current topic", err)
	}
	f.pack, f.published = pack, published
	f.definition.Version, f.definition.TopicVersion, f.definition.PackDigest = "rules-"+version, published.State.Version, published.Digest
	rd, err := f.rules.Save(t.Context(), f.actor, rulesets.SaveRequest{Expected: previous.State.Revision, Definition: f.definition, Change: "Current scoped rule review"})
	if err != nil {
		t.Fatal("save current rules", err)
	}
	rr, err := f.rules.Review(t.Context(), f.actor, pack.Topic, rulesets.ReviewRequest{DraftRevision: rd.Revision, Digest: rd.Digest, Decision: "approve", Note: "Reviewed current scoped value policy"})
	if err != nil {
		t.Fatal("review current rules", err)
	}
	if _, err = f.rules.Publish(t.Context(), f.actor, pack.Topic, rulesets.PublishRequest{Expected: previous.State.Revision, Review: rr.ID}); err != nil {
		t.Fatal("publish current rules", err)
	}
}

func scopedRequalificationExampleHash(x nlqexec.ExampleRecord) string {
	// SQL and the private review note are omitted from ExampleRecord's JSON.
	return exec.Hash([]any{x, x.SQL, x.ReviewNote, x.EvidenceOutcome})
}

func scopedRequalificationChatCalls(model *gatewayFixture) int {
	model.mu.Lock()
	defer model.mu.Unlock()
	calls := 0
	for _, path := range model.paths {
		if !strings.Contains(path, "embedding") && !strings.Contains(path, "rerank") {
			calls++
		}
	}
	return calls
}

func (f *scopedRequalificationFixture) assertResult(t *testing.T, result nlqexec.RunResult, year, selected string) {
	t.Helper()
	if result.Execution.Result == nil {
		t.Fatal("missing scoped result")
	}
	rows := result.Execution.Result.Rows
	decode := func(cell json.RawMessage) string {
		if string(cell) == "null" {
			return "NULL"
		}
		var value string
		if json.Unmarshal(cell, &value) != nil {
			value = string(cell)
		}
		return value
	}
	equal := func(got, want string) bool {
		return got == want || got != "NULL" && want != "NULL" && liveNumberEquals(got, want)
	}
	if f.schema == 2 {
		// Independent row arithmetic; it never reads generated SQL or a receipt.
		gross, refunded, grossKnown, refundKnown := 0, 0, 0, 0
		for _, o := range f.orders {
			if o.paid && strings.HasPrefix(o.date, year) && o.amount != nil {
				gross += *o.amount
				grossKnown++
			}
		}
		for _, r := range f.refunds {
			if strings.HasPrefix(r.date, year) && r.amount != nil {
				refunded += *r.amount
				refundKnown++
			}
		}
		want := []string{fmt.Sprint(gross - refunded), fmt.Sprint(grossKnown - refundKnown)}
		if len(rows) != 1 || len(rows[0]) != 2 {
			t.Fatal("scalar shape", rows)
		}
		for i, v := range want {
			if !equal(decode(rows[0][i]), v) {
				t.Fatal("independent scalar oracle", rows, want)
			}
		}
		return
	}
	refunds := f.refunds
	if f.schema == 5 {
		refunds = nil
		minimum := 731042619
		if year == "2025" {
			minimum = 731042627
		}
		for i, r := range f.refunds {
			if 731042619+i < minimum || f.nullOnly && r.amount != nil || !f.nullOnly && r.amount != nil && *r.amount < 17 {
				continue
			}
			refunds = append(refunds, r)
		}
	}
	if !f.periods {
		year = ""
	}
	expected := groupedOwnedOracle(f.orders, refunds, year, f.periods, false, false)
	if f.schema == 4 || f.schema == 5 {
		if f.nullOnly {
			selected = "NULL"
		}
		value, exists := expected[selected]
		expected = map[string][2]string{}
		if exists {
			expected[selected] = value
		}
	}
	if len(rows) != len(expected) {
		t.Fatalf("independent group oracle: rows=%v expected=%v", rows, expected)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if len(row) != 3 {
			t.Fatal("group output shape")
		}
		key := decode(row[0])
		want, ok := expected[key]
		if !ok || seen[key] {
			t.Fatal("missing or duplicate complete group", key)
		}
		seen[key] = true
		for i, v := range want {
			if !equal(decode(row[i+1]), v) {
				t.Fatal("independent grouped oracle", key, row, want)
			}
		}
	}
}

func (f *scopedRequalificationFixture) qualify(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	wantExamples := int64(2)
	scope, _ := store.NewScope(f.actor.Tenant(), f.actor.User())
	metadata := support.Raw(t, f.f.dsn)
	policy := map[int]string{2: nlqexec.ScopedScalarExamplePolicy, 3: nlqexec.ScopedGroupedExamplePolicy, 4: nlqexec.ScopedSelectionExamplePolicy, 5: nlqexec.ScopedGroupedFactExamplePolicy}[f.schema]
	analyticalVersion := map[int]string{2: exec.AnalyticalScopedPopulationsVersion, 3: exec.AnalyticalGroupedOwnedPopulationsVersion, 4: exec.AnalyticalGroupedSelectionVersion, 5: exec.AnalyticalGroupedFactsVersion}[f.schema]
	oldAnchor := f.anchor(t, "2026", "PRIVATE_REQUAL_731")
	oldPlan, err := f.query.Plan(ctx, f.actor, nlqexec.PlanRequest{QuestionRequest: oldAnchor})
	if err != nil {
		t.Fatal("original scoped plan", err)
	}
	if oldPlan.Bindings == nil || oldPlan.Bindings.SchemaVersion != f.schema || oldPlan.Analytical == nil || oldPlan.Analytical.Version != analyticalVersion {
		t.Fatal("original binder/analytical family")
	}
	oldRun, err := f.query.Run(ctx, f.actor, nlqexec.RunRequest{QueryID: oldPlan.QueryID, Operation: oldPlan.QueryID + "-old"})
	if err != nil {
		t.Fatal("original scoped run", err)
	}
	f.assertResult(t, oldRun, "2026", "PRIVATE_REQUAL_731")
	if f.schema == 5 {
		calls, reads := f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		for _, who := range []struct{ user, session string }{{"other-user", f.actor.Session()}, {f.actor.User(), "other-session"}} {
			claims := f.f.token.claims(f.actor.Tenant(), who.user, phase18Scopes(f.actor.Tenant(), true))
			claims["session"] = who.session
			other, err := f.f.token.verifier.Verify(ctx, f.f.token.sign(t, claims, nil), auth.HTTP)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.query.Feedback(ctx, other, nlqexec.FeedbackRequest{QueryID: oldPlan.QueryID, Verdict: "positive"}); err == nil {
				t.Fatal("foreign actor/session learned fact base")
			}
		}
		if calls != f.model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) || count(t, metadata, `SELECT count(*) FROM chartworks.nlq_examples`) != 0 {
			t.Fatal("denied feedback performed work")
		}
	}
	if err = f.query.Feedback(ctx, f.actor, nlqexec.FeedbackRequest{QueryID: oldPlan.QueryID, Verdict: "positive"}); err != nil {
		t.Fatal("scoped feedback", err)
	}
	examples, err := f.query.Examples(ctx, f.actor, f.pack.Topic, 8)
	if err != nil || len(examples) != 1 {
		t.Fatal("one scoped original", len(examples), err)
	}
	calls, reads := f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	old, err := f.query.ExampleState(ctx, f.actor, nlqexec.ExampleStateRequest{ExampleID: examples[0].ID, ExpectedVersion: examples[0].Version, State: "active", ReviewNote: "Reviewed original unbound scoped base"})
	if err != nil {
		t.Fatal("review original", err)
	}
	if calls != f.model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("activation performed model or source row execution")
	}
	old, err = f.f.db.ReadExample(ctx, scope, old.ID)
	if err != nil || old.SQL != f.sql || old.Origin.BindingPolicy != policy || old.ParameterSchema != nil {
		t.Fatal("original value-free custody", err)
	}
	originalHash := scopedRequalificationExampleHash(old)
	if f.schema == 5 {
		bundle, err := f.query.ExportExamples(ctx, f.actor, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 8})
		if err != nil || bundle.SchemaVersion != 3 || len(bundle.Examples) != 1 {
			t.Fatal("original grouped-fact portable v3", err)
		}
		portable := bundle.Examples[0]
		if portable.SchemaVersion != 3 || portable.SQL != f.sql || portable.Origin.BindingPolicy != policy {
			t.Fatal("original portable base custody")
		}
		portable.Question += " Imported review copy."
		portable.Digest = exec.Hash([]any{policy, f.pack.Topic, portable.Question, portable.SQL, portable.ParameterSchema})
		imported, err := f.query.ImportExample(ctx, f.actor, nlqexec.ExampleImportRequest{Anchor: oldAnchor, Example: portable})
		if err != nil || imported.ID == old.ID || imported.State != "candidate" || imported.ReviewedAt != nil || imported.ParameterSchema != nil {
			t.Fatal("import did not require separate candidate review", err)
		}
		repeated, err := f.query.ImportExample(ctx, f.actor, nlqexec.ExampleImportRequest{Anchor: oldAnchor, Example: portable})
		if err != nil || repeated.ID != imported.ID || repeated.Version != imported.Version {
			t.Fatal("portable replay duplicated evidence", err)
		}
		wantExamples++
	}
	if _, err = f.query.RequalifyExample(ctx, f.actor, nlqexec.ExampleRequalificationRequest{ExampleID: old.ID, ExpectedVersion: old.Version, Anchor: oldAnchor}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("unchanged original requalified", err)
	}

	f.republish(t, "scoped-requal-v2", false)
	currentAnchor := f.anchor(t, "2025", "PRIVATE_CURRENT_732")
	if f.schema == 5 {
		stalePlan, err := f.query.Plan(ctx, f.actor, nlqexec.PlanRequest{QuestionRequest: currentAnchor})
		if err != nil {
			t.Fatal("current plan before requalification", err)
		}
		staleQuery, err := f.f.db.ReadQuery(ctx, scope, stalePlan.QueryID)
		if err != nil || len(staleQuery.ExampleSelection.Selected) != 0 {
			t.Fatal("publication promoted stale/unchecked examples", err)
		}
	}
	server := httptest.NewServer(nlqapi.ExecutionHandler(f.f.token.verifier, f.query, http.NotFoundHandler()))
	defer server.Close()
	claims := f.f.token.claims(f.actor.Tenant(), f.actor.User(), phase18Scopes(f.actor.Tenant(), true))
	claims["session"] = f.actor.Session()
	bearer := f.f.token.sign(t, claims, nil)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	request := nlqexec.ExampleRequalificationRequest{ExampleID: old.ID, ExpectedVersion: old.Version, Anchor: currentAnchor}
	chat, reads := scopedRequalificationChatCalls(f.model), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	calls = f.model.requests.Load()
	response, err := client.RequalifyExampleNLQ(ctx, request)
	if err != nil {
		t.Fatal("SDK current scoped qualification", err)
	}
	if chat != scopedRequalificationChatCalls(f.model) || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("qualification generated replacement SQL or executed source rows")
	}
	// The same question can reuse a cached embedding; the route itself is
	// freshly authorized and its exact contract is checked below.
	if delta := f.model.requests.Load() - calls; delta < 0 || delta > 4 {
		t.Fatal("unbounded current routing calls", delta)
	} else {
		t.Logf("schema%d qualification routing provider calls=%d; SQL-generation/source-execution delta=0", f.schema, delta)
	}
	fresh, err := f.f.db.ReadExample(ctx, scope, response.ID)
	if err != nil {
		t.Fatal(err)
	}
	proof := fresh.Origin.Requalification
	if fresh.ID == old.ID || fresh.State != "candidate" || fresh.Version != 1 || fresh.ReviewedAt != nil || fresh.ReviewedBy != "" || fresh.ReviewNote != "" || proof == nil || !proof.Valid() {
		t.Fatal("separate candidate review/provenance missing")
	}
	if fresh.SQL != f.sql || fresh.Question != old.Question || fresh.ParameterSchema != nil || fresh.Origin.BindingPolicy != policy || fresh.Origin.TopicVersion != f.pack.Version || exec.Hash(fresh.Origin.RuleVersions) != exec.Hash([]string{f.definition.Version}) || fresh.Origin.Context != old.Origin.Context || fresh.Origin.SourceBindingDigest != old.Origin.SourceBindingDigest || fresh.Digest == old.Digest {
		t.Fatal("current scoped origin or value-free base changed incorrectly")
	}
	if proof.Policy != nlqexec.ExampleRequalificationPolicy || proof.ExampleID != old.ID || proof.Version != old.Version || proof.Digest != old.Digest || proof.OriginDigest != exec.Hash(old.Origin) || fresh.PositiveEvidence != old.PositiveEvidence || fresh.NegativeEvidence != old.NegativeEvidence || fresh.EvidenceCount != old.EvidenceCount {
		t.Fatal("original evidence lineage was rewritten")
	}
	encoded, _ := json.Marshal([]any{fresh.Question, fresh.SQL, fresh.ParameterSchema, fresh.Origin})
	for _, value := range []string{"PRIVATE_REQUAL_731", "PRIVATE_CURRENT_732", "2026-01-01", "2025-01-01", "731042619", "731042627"} {
		if strings.Contains(string(encoded), value) {
			t.Fatal("private current or original owned value entered reusable example")
		}
	}
	assertOriginal := func() {
		t.Helper()
		unchanged, err := f.f.db.ReadExample(ctx, scope, old.ID)
		if err != nil || scopedRequalificationExampleHash(unchanged) != originalHash {
			t.Fatal("immutable original changed", err)
		}
	}
	assertOriginal()
	for i := 0; i < 2; i++ {
		replayed, err := client.RequalifyExampleNLQ(ctx, request)
		if err != nil || replayed.ID != fresh.ID || replayed.Version != fresh.Version {
			t.Fatal("exact qualification replay", err)
		}
	}
	if _, err = f.query.RequalifyExample(ctx, f.actor, nlqexec.ExampleRequalificationRequest{ExampleID: fresh.ID, ExpectedVersion: fresh.Version, Anchor: currentAnchor}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("current candidate qualification chain", err)
	}
	if chat != scopedRequalificationChatCalls(f.model) || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("qualification retry generated SQL or executed source rows")
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.nlq_examples`) != wantExamples {
		t.Fatal("qualification retry duplicated evidence")
	}

	// Current generation may select only the separately activated candidate.
	calls, reads = f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	active, err := client.ExampleStateNLQ(ctx, nlqexec.ExampleStateRequest{ExampleID: fresh.ID, ExpectedVersion: fresh.Version, State: "active", ReviewNote: "Separately reviewed current scoped proof"})
	if err != nil || active.State != "active" {
		t.Fatal("separate activation", err)
	}
	if calls != f.model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("fresh activation performed model/source row execution")
	}
	currentPlan, err := f.query.Plan(ctx, f.actor, nlqexec.PlanRequest{QuestionRequest: currentAnchor})
	if err != nil {
		t.Fatal("generation with requalified base", err)
	}
	stored, err := f.f.db.ReadQuery(ctx, scope, currentPlan.QueryID)
	if err != nil || stored.Clarification == nil || stored.Clarification.Binding.SchemaVersion != f.schema || stored.Clarification.BaseSQL != f.sql || len(stored.Clarification.BaseParameters) != 0 || stored.Analytical == nil || stored.Analytical.Version != analyticalVersion {
		t.Fatal("current binder/native analytical witness", err)
	}
	if stored.Analytical.Contract != proof.ContractDigest {
		t.Fatal("qualification contract differs from exact current native proof")
	}
	wantParameters := 0
	if f.periods {
		wantParameters = 4
	}
	if (f.schema == 4 || f.schema == 5) && !f.nullOnly {
		wantParameters++
	}
	if f.schema == 5 {
		wantParameters++
		if !f.nullOnly {
			wantParameters++
		}
	}
	if len(stored.Parameters) != wantParameters {
		t.Fatal("owned values became model slots or disappeared")
	}
	used := false
	if stored.ExampleSelection.Usage != nil {
		for _, item := range stored.ExampleSelection.Usage.Used {
			used = used || item.ExampleID == fresh.ID
			if item.ExampleID == old.ID {
				t.Fatal("stale origin reached current prompt")
			}
		}
	}
	if !used {
		t.Fatal("fresh candidate did not actually reach generation")
	}
	if f.schema == 5 {
		f.model.mu.Lock()
		wire := strings.Join(f.model.requestBodies, "\n")
		f.model.mu.Unlock()
		if !strings.Contains(wire, policy) {
			t.Fatal("used receipt lacks actual grouped-fact demonstration")
		}
		for _, private := range []string{"PRIVATE_REQUAL_731", "PRIVATE_CURRENT_732", "731042619", "731042627"} {
			if strings.Contains(wire, private) {
				t.Fatal("historical/current owned value reached model")
			}
		}
	}
	currentRun := nlqexec.RunRequest{QueryID: currentPlan.QueryID, Operation: currentPlan.QueryID + "-current"}
	result, err := f.query.Run(ctx, f.actor, currentRun)
	if err != nil {
		t.Fatal("current scoped run", err)
	}
	f.assertResult(t, result, "2025", "PRIVATE_CURRENT_732")
	calls, reads = f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	replay, err := client.RunNLQ(ctx, currentRun)
	if err != nil {
		t.Fatal("current run replay", err)
	}
	f.assertResult(t, replay, "2025", "PRIVATE_CURRENT_732")
	if calls != f.model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("current terminal replay repeated model/source row execution")
	}

	if f.schema == 5 && !f.nullOnly {
		emptyAnchor := f.anchor(t, "2025", "absent")
		emptyPlan, err := f.query.Plan(ctx, f.actor, nlqexec.PlanRequest{QuestionRequest: emptyAnchor})
		if err != nil {
			t.Fatal("learned empty-group plan", err)
		}
		emptyQuery, err := f.f.db.ReadQuery(ctx, scope, emptyPlan.QueryID)
		if err != nil {
			t.Fatal(err)
		}
		assertScopedExampleUsage(t, emptyQuery, fresh, 5)
		emptyResult, err := f.query.Run(ctx, f.actor, nlqexec.RunRequest{QueryID: emptyPlan.QueryID, Operation: emptyPlan.QueryID + "-empty"})
		if err != nil {
			t.Fatal("learned empty-group execution", err)
		}
		f.assertResult(t, emptyResult, "2025", "absent")
	}

	bundle, err := client.ExportExamplesNLQ(ctx, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 8})
	if err != nil || bundle.SchemaVersion != 5 {
		t.Fatal("portable-v5 export", err)
	}
	var portable nlqexec.PortableExample
	found := 0
	for _, row := range bundle.Examples {
		if row.Origin.Requalification != nil {
			portable = row
			found++
		}
	}
	if found != 1 || portable.SchemaVersion != 5 || portable.SQL != f.sql || portable.Digest != fresh.Digest || exec.Hash(portable.Origin) != exec.Hash(fresh.Origin) {
		t.Fatal("scoped version5 lost exact base and origin")
	}
	wire, err := json.Marshal(portable)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip nlqexec.PortableExample
	if err = json.Unmarshal(wire, &roundTrip); err != nil {
		t.Fatal(err)
	}
	chat, reads = scopedRequalificationChatCalls(f.model), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	for i := 0; i < 2; i++ {
		imported, err := client.ImportExampleNLQ(ctx, nlqexec.ExampleImportRequest{Anchor: currentAnchor, Example: roundTrip})
		if err != nil || imported.ID != fresh.ID || imported.State != "active" || imported.Version != active.Version {
			t.Fatal("v5 authenticated revalidation/replay", err)
		}
	}
	if chat != scopedRequalificationChatCalls(f.model) || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("v5 revalidation generated SQL or executed source rows")
	}
	// A historical policy cannot establish a present-day owned scope.
	unowned := currentAnchor
	unowned.ClarificationQuery, unowned.AnswerContext, unowned.Answers = "", "", nil
	unowned.Question = strings.ReplaceAll(unowned.Question, " for selected groups", "")
	if f.schema != 4 {
		unowned.Question = strings.ReplaceAll(unowned.Question, " in 2025", "")
	}
	if _, err := client.ImportExampleNLQ(ctx, nlqexec.ExampleImportRequest{Anchor: unowned, Example: roundTrip}); err == nil {
		t.Fatal("portable origin authorized an absent current owned scope")
	}
	stale := request
	stale.ExpectedVersion++
	if _, err := f.query.RequalifyExample(ctx, f.actor, stale); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale parent version qualified", err)
	}
	wrongContext := request
	wrongContext.Anchor.Context = "foreign-context"
	if _, err := f.query.RequalifyExample(ctx, f.actor, wrongContext); err == nil {
		t.Fatal("foreign source context qualified")
	}
	foreign := f.f.token.envelope(t, "foreign-tenant", f.actor.User(), phase18Scopes("foreign-tenant", true)...)
	if _, err := f.query.RequalifyExample(ctx, foreign, request); err == nil {
		t.Fatal("foreign tenant qualified scoped origin")
	}
	// A forged digest is public metadata, never a substitute for fresh binder proof.
	mutations := map[string]func(*nlqexec.PortableExample){
		"binder_policy":  func(x *nlqexec.PortableExample) { x.Origin.BindingPolicy = nlqexec.OwnedExamplePolicy },
		"unknown_policy": func(x *nlqexec.PortableExample) { x.Origin.BindingPolicy = "unreviewed-owned-policy" },
		"sql": func(x *nlqexec.PortableExample) {
			x.SQL = strings.Replace(x.SQL, "g.value-r.value", "g.value+r.value", 1)
		},
		"count_population": func(x *nlqexec.PortableExample) { x.SQL = strings.Replace(x.SQL, "COUNT(r.amount_usd)", "COUNT(*)", 1) },
		"join": func(x *nlqexec.PortableExample) {
			x.SQL = strings.Replace(x.SQL, "r.order_id=o.order_id", "r.order_id=o.customer_id", 1)
		},
		"contract": func(x *nlqexec.PortableExample) {
			x.Origin.Requalification.ContractDigest = exec.Hash("forged-current-contract")
		},
		"parent": func(x *nlqexec.PortableExample) { x.Origin.Requalification.ExampleID = strings.Repeat("f", 32) },
		"parent_origin": func(x *nlqexec.PortableExample) {
			x.Origin.Requalification.OriginDigest = exec.Hash("forged-parent-origin")
		},
		"evidence":  func(x *nlqexec.PortableExample) { x.PositiveEvidence++ },
		"downgrade": func(x *nlqexec.PortableExample) { x.SchemaVersion = 3 },
	}
	if f.schema == 2 {
		delete(mutations, "join")
	}
	for name, mutate := range mutations {
		t.Run("reject_"+name, func(t *testing.T) {
			var bad nlqexec.PortableExample
			if err := json.Unmarshal(wire, &bad); err != nil {
				t.Fatal(err)
			}
			mutate(&bad)
			bad.Digest = exec.Hash([]any{nlqexec.ExampleRequalificationPolicy, f.pack.Topic, bad.Question, bad.SQL, bad.ParameterSchema, bad.Origin})
			if _, err := client.ImportExampleNLQ(ctx, nlqexec.ExampleImportRequest{Anchor: currentAnchor, Example: bad}); err == nil {
				t.Fatal("hostile qualified portable row admitted")
			}
		})
	}
	if chat != scopedRequalificationChatCalls(f.model) || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("hostile v5 replay generated SQL or executed source rows")
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.nlq_examples`) != wantExamples {
		t.Fatal("hostile import changed candidate population")
	}
	for _, mutation := range []string{`origin=origin-'requalification'`, `origin=jsonb_set(origin,'{binding_policy}','"current-owned-predicates-v1"')`, `sql_text=sql_text||' '`} {
		if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_examples SET `+mutation+` WHERE example_id=$1`, fresh.ID); err == nil {
			t.Fatal("immutable qualified origin/base mutated")
		}
	}
	assertOriginal()

	// Changing reviewed SUM to AVG leaves SQL native-executable, but must fail
	// the current analytical proof after exact schema-specific owned rebinding.
	f.republish(t, "scoped-requal-v3", true)
	changedAnchor := f.anchor(t, "2025", "PRIVATE_CURRENT_732")
	chat, reads = scopedRequalificationChatCalls(f.model), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	if _, err = f.query.RequalifyExample(ctx, f.actor, nlqexec.ExampleRequalificationRequest{ExampleID: old.ID, ExpectedVersion: old.Version, Anchor: changedAnchor}); !errors.Is(err, exec.ErrAnalyticalMismatch) {
		t.Fatal("native-executable SUM qualified under current AVG", err)
	}
	if chat != scopedRequalificationChatCalls(f.model) || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("semantic rejection generated SQL or executed source rows")
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.nlq_examples`) != wantExamples {
		t.Fatal("failed semantic proof persisted a candidate")
	}
	assertOriginal()
	if f.schema == 5 {
		f.republishEdited(t, "scoped-requal-v4", func(pack *semantics.TopicPack) {
			pack.Measures[0].Aggregation = semantics.AggregationSum
			for i := range pack.Measures {
				if pack.Measures[i].ID == "refund_known" {
					pack.Measures[i].Field.ID = "refund_id"
				}
			}
		})
		countAnchor := f.anchor(t, "2025", "PRIVATE_CURRENT_732")
		chat, reads = scopedRequalificationChatCalls(f.model), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		if _, err = f.query.RequalifyExample(ctx, f.actor, nlqexec.ExampleRequalificationRequest{ExampleID: old.ID, ExpectedVersion: old.Version, Anchor: countAnchor}); !errors.Is(err, exec.ErrAnalyticalMismatch) {
			t.Fatal("changed COUNT companion requalified old fact base", err)
		}
		if chat != scopedRequalificationChatCalls(f.model) || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) || count(t, metadata, `SELECT count(*) FROM chartworks.nlq_examples`) != wantExamples {
			t.Fatal("COUNT rejection performed replacement work")
		}
		assertOriginal()
		f.republishEdited(t, "scoped-requal-v5", func(pack *semantics.TopicPack) {
			for i := range pack.Measures {
				if pack.Measures[i].ID == "refund_known" {
					pack.Measures[i].Field.ID = "amount_usd"
				}
			}
			for i := range pack.Joins {
				if pack.Joins[i].ID == "orders-refunds" {
					pack.Joins[i].Left.ID = "refund_id"
				}
			}
		})
		joinAnchor := f.anchor(t, "2025", "PRIVATE_CURRENT_732")
		chat, reads = scopedRequalificationChatCalls(f.model), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		if _, err = f.query.RequalifyExample(ctx, f.actor, nlqexec.ExampleRequalificationRequest{ExampleID: old.ID, ExpectedVersion: old.Version, Anchor: joinAnchor}); err == nil {
			t.Fatal("changed reviewed join requalified old fact base")
		}
		if chat != scopedRequalificationChatCalls(f.model) || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) || count(t, metadata, `SELECT count(*) FROM chartworks.nlq_examples`) != wantExamples {
			t.Fatal("join rejection performed replacement work")
		}
		assertOriginal()
	}
}
