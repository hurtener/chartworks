package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
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

type factMatrixOrder struct {
	id                    int
	region, segment, date *string
	amount                *int
	paid                  bool
}
type factMatrixEvent struct {
	parent int
	amount *int
}
type factMatrixKey [2]string

// Row-only oracle: each fact establishes its own groups, including all-NULL
// inputs, before complete-key union. It deliberately shares no SQL, native proof,
// contract, binder or receipt reconstruction with the application under test.
func factMatrixOracle(orders []factMatrixOrder, events [][]factMatrixEvent, grain string, raw bool, maxima []int) map[factMatrixKey][]string {
	type aggregate struct{ sum, known int }
	key := func(o factMatrixOrder) factMatrixKey {
		out := factMatrixKey{"NULL", ""}
		if grain == "calendar" {
			if o.date != nil {
				out[0] = (*o.date)[:7]
			}
			return out
		}
		if o.region != nil {
			out[0] = *o.region
		}
		if grain == "composite" {
			out[1] = "NULL"
			if o.segment != nil {
				out[1] = *o.segment
			}
		}
		return out
	}
	parents := map[int]factMatrixOrder{}
	populations := make([]map[factMatrixKey]aggregate, 1+len(events))
	for i := range populations {
		populations[i] = map[factMatrixKey]aggregate{}
	}
	for _, o := range orders {
		parents[o.id] = o
		if !raw && !o.paid {
			continue
		}
		a := populations[0][key(o)]
		if o.paid && o.amount != nil {
			a.sum += *o.amount
			a.known++
		}
		populations[0][key(o)] = a
	}
	for i, rows := range events {
		for _, row := range rows {
			if row.amount != nil && *row.amount > maxima[i] {
				continue
			}
			k := key(parents[row.parent])
			a := populations[i+1][k]
			if row.amount != nil {
				a.sum += *row.amount
				a.known++
			}
			populations[i+1][k] = a
		}
	}
	result := map[factMatrixKey][]string{}
	for _, population := range populations {
		for k := range population {
			values := make([]string, 2*len(populations))
			for i, other := range populations {
				values[2*i], values[2*i+1] = "NULL", "NULL"
				if a, ok := other[k]; ok {
					values[2*i+1] = strconv.Itoa(a.known)
					if a.known > 0 {
						values[2*i] = strconv.Itoa(a.sum)
					}
				}
			}
			result[k] = values
		}
	}
	return result
}

func TestSQLRecoveryGroupedFactMatrixAcceptance(t *testing.T) {
	for _, lanes := range []int{3, 4} {
		for _, grain := range []string{"direct", "composite", "calendar"} {
			for _, raw := range []bool{false, true} {
				t.Run(fmt.Sprintf("lanes_%d_%s_raw_%t", lanes, grain, raw), func(t *testing.T) { runGroupedFactMatrix(t, lanes, grain, raw) })
			}
		}
	}
}

func runGroupedFactMatrix(t *testing.T, lanes int, grain string, raw bool) {
	t.Helper()
	f := liveCommerceSource(t)
	if _, err := f.admin.Exec(t.Context(), `DELETE FROM analytics.order_items; DELETE FROM analytics.refunds; DELETE FROM analytics.orders; DELETE FROM analytics.customers; DELETE FROM analytics.audit_totals;
 ALTER TABLE analytics.customers ALTER COLUMN region DROP NOT NULL; ALTER TABLE analytics.customers DROP CONSTRAINT customers_region_check;
 ALTER TABLE analytics.customers ALTER COLUMN segment DROP NOT NULL;
 ALTER TABLE analytics.orders ALTER COLUMN total_usd DROP NOT NULL; ALTER TABLE analytics.orders ALTER COLUMN ordered_at DROP NOT NULL;
 ALTER TABLE analytics.refunds ALTER COLUMN amount_usd DROP NOT NULL; ALTER TABLE analytics.order_items ALTER COLUMN amount_usd DROP NOT NULL;
 ALTER TABLE analytics.audit_totals ALTER COLUMN total_usd DROP NOT NULL;`); err != nil {
		t.Fatal(err)
	}
	text := func(s string) *string { return &s }
	number := func(n int) *int { return &n }
	orders := []factMatrixOrder{
		{1, text("north"), text("consumer"), text("2026-01-31"), number(100), true},
		{2, text("north"), text("business"), text("2026-01-31"), number(50), true},
		{3, nil, text("consumer"), text("2026-02-01"), number(30), true},
		{4, text("unknown"), nil, nil, nil, true},
		{5, text("zero"), text("consumer"), text("2026-04-01"), number(0), true},
		{6, text("gross-only"), text("consumer"), text("2026-05-01"), number(12), true},
		{7, text("cancelled"), text("consumer"), text("2026-06-01"), number(20), false},
		{8, text("north"), text("consumer"), text("2027-01-01"), number(60), true},
		{9, text("north"), text("consumer"), text("2026-12-31"), number(10), true},
		{10, nil, nil, nil, nil, true},
		{11, text("event-only"), text("consumer"), text("2026-07-01"), number(5), false},
		{12, text("filtered-only"), text("consumer"), text("2026-08-01"), number(8), false},
	}
	events := [][]factMatrixEvent{
		{{1, number(20)}, {1, number(99)}, {1, number(5)}, {2, nil}, {3, number(0)}, {4, nil}, {5, number(0)}, {7, number(10)}, {8, number(19)}, {9, number(3)}, {10, nil}, {11, number(13)}, {12, number(99)}},
		{{1, number(30)}, {1, number(99)}, {1, number(2)}, {2, nil}, {3, number(0)}, {4, nil}, {5, number(0)}, {7, number(14)}, {8, number(29)}, {9, number(4)}, {10, nil}, {11, number(15)}, {12, number(99)}},
		{{1, number(40)}, {2, nil}, {3, number(0)}, {4, nil}, {5, number(0)}, {7, number(18)}, {8, number(39)}, {9, number(5)}, {10, nil}, {11, number(17)}, {12, number(99)}},
	}[:lanes-1]
	for _, o := range orders {
		status := "cancelled"
		if o.paid {
			status = "paid"
		}
		if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.customers VALUES($1,$2,$3)`, o.id, o.segment, o.region); err != nil {
			t.Fatal(err)
		}
		if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES($1,$1,$2,$3,$4)`, o.id, o.date, o.amount, status); err != nil {
			t.Fatal(err)
		}
	}
	for lane, rows := range events {
		for i, row := range rows {
			var err error
			switch lane {
			case 0:
				_, err = f.admin.Exec(t.Context(), `INSERT INTO analytics.refunds VALUES($1,$2,'2028-09-01',$3)`, i+100, row.parent, row.amount)
			case 1:
				_, err = f.admin.Exec(t.Context(), `INSERT INTO analytics.order_items VALUES($1,$2,'home',1,$3)`, i+200, row.parent, row.amount)
			case 2:
				_, err = f.admin.Exec(t.Context(), `INSERT INTO analytics.audit_totals VALUES($1,$2)`, row.parent, row.amount)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	pack, _ := liveCommerceTopics(t, f)
	// Reorient only the explicitly reviewed child-to-unique-parent relationships.
	for i := range pack.Joins {
		j := &pack.Joins[i]
		if j.ID != "orders-customers" {
			j.Left, j.Right = j.Right, j.Left
			j.Type = semantics.JoinInner
			j.Cardinality = semantics.CardinalityManyToOne
			j.Evidence.LeftGrain, j.Evidence.RightGrain = j.Evidence.RightGrain, j.Evidence.LeftGrain
		}
	}
	facts := []semantics.Dataset{pack.Datasets[0], pack.Datasets[3], pack.Datasets[1]}
	if lanes == 4 {
		b, err := f.s.Binding(t.Context(), f.e, pack.Datasets[0].Source.Source, pack.Datasets[0].Source.Context)
		if err != nil {
			t.Fatal(err)
		}
		id := ""
		for _, relation := range b.Relations {
			if relation.Name == "audit_totals" {
				id = relation.ID
			}
		}
		if id == "" {
			t.Fatal("missing fourth source fact")
		}
		source := pack.Datasets[0].Source
		profile := f.profile(t, engineering.ProfileSpec{ID: "fact-matrix-adjustments", Source: source.Source, Context: source.Context, Dataset: id, Columns: []string{"id", "total_usd"}, SkipLLM: true}).Profile.Profile
		source.Dataset, source.ProfileVersion, source.ProfileDigest = id, profile.Version, profile.DeterministicHash()
		d := semantics.Dataset{ID: id, Name: "Adjustments", Source: source}
		for _, c := range profile.Schema {
			role := semantics.SemanticRoleMeasureInput
			if c.Name == "id" {
				role = semantics.SemanticRoleFactKey
			}
			d.Columns = append(d.Columns, semantics.Column{ID: c.Name, SourceName: c.Name, Name: c.Name, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable, Sensitivity: semantics.LiteralNonSensitive, SemanticRole: role})
		}
		pack.Datasets = append(pack.Datasets, d)
		facts = append(facts, d)
		pack.Joins = append(pack.Joins, semantics.Join{ID: "adjustments-orders", Name: "Adjustments to orders", Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: "id"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: facts[0].ID, ID: "order_id"}, Type: semantics.JoinInner, Cardinality: semantics.CardinalityManyToOne, Evidence: semantics.RelationshipEvidence{ID: "reviewed-synthetic-adjustment-key", LeftGrain: "adjustment", RightGrain: "order", Provenance: "reviewed_unique_key"}})
	}
	fields := []string{"total_usd", "amount_usd", "amount_usd", "total_usd"}
	names := []string{"Gross amount", "Refund amount", "Item amount", "Adjustment amount"}
	pack.Measures, pack.KPIs = nil, nil
	var metricIDs []string
	for i, d := range facts {
		for _, aggregation := range []semantics.Aggregation{semantics.AggregationSum, semantics.AggregationCount} {
			id := fmt.Sprintf("fact_%d_%s", i, aggregation)
			name := names[i]
			if aggregation == semantics.AggregationCount {
				name += " known count"
			}
			m := semantics.Measure{ID: id, Name: name, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: fields[i]}, Aggregation: aggregation}
			if i == 0 {
				m.Filters = []semantics.SemanticFilter{{ID: "paid", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: "status"}, Operator: "eq", Values: []string{"paid"}}}
			}
			pack.Measures = append(pack.Measures, m)
			metricIDs = append(metricIDs, id)
		}
	}
	domain := exec.AnalyticalGroupDomainQualifying
	if raw {
		domain = exec.AnalyticalGroupDomainRaw
	}
	pack.GroupedPopulation = &semantics.GroupedPopulationPolicy{Policy: semantics.GroupedPopulationUnionPolicy}
	for _, d := range facts {
		pack.GroupedPopulation.Datasets = append(pack.GroupedPopulation.Datasets, d.ID)
		pack.GroupedPopulation.GroupDomains = append(pack.GroupedPopulation.GroupDomains, semantics.GroupedPopulationDomain{Dataset: d.ID, Domain: domain})
	}
	sort.Strings(pack.GroupedPopulation.Datasets)
	sort.Slice(pack.GroupedPopulation.GroupDomains, func(i, j int) bool {
		return pack.GroupedPopulation.GroupDomains[i].Dataset < pack.GroupedPopulation.GroupDomains[j].Dataset
	})
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
	pattern := semantics.ClarificationPattern{ID: "matrix_facts", Version: "v1", Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "synthetic-exclusive-fact-review"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"reviewed fact maxima"}}, Why: "Apply each maximum only to its independently reviewed fact."}}
	for i, d := range facts[1:] {
		target := semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: fields[i+1]}
		pattern.Targets = append(pattern.Targets, target)
		pattern.Slots = append(pattern.Slots, semantics.ClarificationSlot{ID: fmt.Sprintf("maximum_%d", i), Prompt: "Which reviewed fact maximum?", Required: true, Kind: semantics.SlotNumber, Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Kind: "number", Target: target, Operator: "lte", Nulls: "include", Unit: "USD", Precision: 12, Scale: 2}})
	}
	definition := semantics.RuleSetDefinition{SchemaVersion: 1, ID: "matrix-fact-rules", Version: "v1", Topic: published.State.Topic, TopicVersion: published.State.Version, PackDigest: published.Digest, Patterns: []semantics.ClarificationPattern{pattern}}
	rd, err := rules.Save(t.Context(), actor, rulesets.SaveRequest{Definition: definition, Change: "Review exclusive fact maxima"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := rules.Review(t.Context(), actor, pack.Topic, rulesets.ReviewRequest{DraftRevision: rd.Revision, Digest: rd.Digest, Decision: "approve", Note: "Exact independent roots"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rules.Publish(t.Context(), actor, pack.Topic, rulesets.PublishRequest{Review: review.ID}); err != nil {
		t.Fatal(err)
	}
	sql, groupQuestion, keys := factMatrixSQL(lanes, grain, raw)
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	model.mode.Store(phase18RawResponse(t, sql))
	request := nlqexec.PlanRequest{Operation: "matrix-plan", QuestionRequest: nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: "Gross amount, Refund amount, Item amount and Adjustment amount with known counts for reviewed fact maxima by " + groupQuestion, Locale: nlq.LanguageEnglish, MetricIDs: metricIDs, Kinds: []string{"measure", "dimension"}, LimitPerKind: 10}}
	pending, err := query.Preflight(t.Context(), actor, nlqexec.PreflightRequest{QuestionRequest: request.QuestionRequest})
	if err != nil || pending.Route.Clarification == nil {
		t.Fatal("matrix preflight", err)
	}
	answers := func(maxima []int) []semantics.ClarificationAnswer {
		var out []semantics.ClarificationAnswer
		for i, max := range maxima {
			out = append(out, semantics.ClarificationAnswer{Topic: pack.Topic, TopicVersion: published.State.Version, RulesetVersion: definition.Version, Pattern: pattern.ID, PatternVersion: pattern.Version, Slot: fmt.Sprintf("maximum_%d", i), Value: &semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: strconv.Itoa(max), Unit: "USD"}}})
		}
		return out
	}
	maxima := []int{20, 30, 40}[:lanes-1]
	witnesses := map[factMatrixKey][]string{}
	zero, unknown, missing, event, filtered := factMatrixKey{"zero", ""}, factMatrixKey{"unknown", ""}, factMatrixKey{"gross-only", ""}, factMatrixKey{"event-only", ""}, factMatrixKey{"filtered-only", ""}
	north := factMatrixKey{"north", ""}
	northValues := []string{"220", "4", "47", "4", "65", "4", "84", "3"}
	if grain == "composite" {
		for _, key := range []*factMatrixKey{&zero, &missing, &event, &filtered, &north} {
			key[1] = "consumer"
		}
		unknown[1] = "NULL"
		northValues[0], northValues[1] = "170", "3"
		witnesses[factMatrixKey{"north", "business"}] = []string{"50", "1", "NULL", "0", "NULL", "0", "NULL", "0"}[:2*lanes]
	} else if grain == "calendar" {
		zero[0], unknown[0], missing[0], event[0], filtered[0], north[0] = "2026-04", "NULL", "2026-05", "2026-07", "2026-08", "2026-01"
		northValues = []string{"150", "2", "25", "2", "32", "2", "40", "1"}
	}
	witnesses[north] = northValues[:2*lanes]
	witnesses[zero] = []string{"0", "1", "0", "1", "0", "1", "0", "1"}[:2*lanes]
	witnesses[unknown] = []string{"NULL", "0", "NULL", "0", "NULL", "0", "NULL", "0"}[:2*lanes]
	witnesses[missing] = []string{"12", "1", "NULL", "NULL", "NULL", "NULL", "NULL", "NULL"}[:2*lanes]
	witnesses[event] = []string{"NULL", "NULL", "13", "1", "15", "1", "17", "1"}[:2*lanes]
	if raw {
		witnesses[event][1] = "0"
	}
	oracle := factMatrixOracle(orders, events, grain, raw, maxima)
	for key, want := range witnesses {
		if !reflect.DeepEqual(oracle[key], want) {
			t.Fatalf("hand-calculated witness %v got %v want %v", key, oracle[key], want)
		}
	}
	_, filteredExists := oracle[filtered]
	if filteredExists != raw {
		t.Fatal("fixture no longer distinguishes raw from qualifying group existence")
	}
	request.ClarificationQuery, request.AnswerContext, request.Answers = pending.QueryID, pending.Route.AnswerContext, answers(maxima)
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
		t.Fatal("matrix Plan", err)
	}
	if plan.Analytical == nil || plan.Analytical.Version != exec.AnalyticalGroupedFactsVersion || plan.Bindings == nil || plan.Bindings.SchemaVersion != 5 || len(plan.Bindings.Bindings) != lanes-1 || len(plan.Analytical.Outputs) != lanes*2 {
		t.Fatal("incomplete matrix receipt")
	}
	for _, effect := range plan.Bindings.Bindings {
		if effect.Population != effect.Dataset || effect.Population == facts[0].ID {
			t.Fatal("fact effect lost its exact root")
		}
	}
	scope, _ := store.NewScope(actor.Tenant(), actor.User())
	stored, err := f.db.ReadQuery(t.Context(), scope, plan.QueryID)
	if err != nil || stored.Clarification == nil || stored.Clarification.BaseSQL != sql {
		t.Fatal("matrix base custody", err)
	}
	assertRows := func(result nlqexec.RunResult, maxima []int) {
		t.Helper()
		if result.Execution.Result == nil {
			t.Fatal("missing matrix result")
		}
		want := factMatrixOracle(orders, events, grain, raw, maxima)
		if len(result.Execution.Result.Rows) != len(want) {
			t.Fatalf("matrix groups got %d want %d", len(result.Execution.Result.Rows), len(want))
		}
		for _, row := range result.Execution.Result.Rows {
			if len(row) != keys+2*lanes {
				t.Fatal("matrix output arity")
			}
			key := factMatrixKey{"NULL", ""}
			for i := 0; i < keys; i++ {
				key[i] = "NULL"
				if string(row[i]) != "null" {
					if json.Unmarshal(row[i], &key[i]) != nil {
						t.Fatal("invalid matrix key")
					}
					if grain == "calendar" {
						if len(key[i]) < 7 {
							t.Fatal("invalid calendar key")
						}
						key[i] = key[i][:7]
					}
				}
			}
			values, ok := want[key]
			if !ok {
				t.Fatal("unexpected or repeated matrix key", key)
			}
			delete(want, key)
			for i, expected := range values {
				got := "NULL"
				if string(row[keys+i]) != "null" && json.Unmarshal(row[keys+i], &got) != nil {
					got = string(row[keys+i])
				}
				if got != expected && (got == "NULL" || expected == "NULL" || !liveNumberEquals(got, expected)) {
					t.Fatalf("row-only oracle %v output %d got %s want %s", key, i, got, expected)
				}
			}
		}
	}
	run := nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "matrix-run"}
	result, err := client.RunNLQ(t.Context(), run)
	if err != nil {
		t.Fatal("matrix Run", err)
	}
	assertRows(result, maxima)
	metadata := support.Raw(t, f.dsn)
	calls, reads := model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := restarted.Run(t.Context(), actor, run)
	if err != nil {
		t.Fatal("matrix terminal replay", err)
	}
	assertRows(replay, maxima)
	repeated, err := restarted.Plan(t.Context(), actor, request)
	if err != nil || repeated.QueryID != plan.QueryID {
		t.Fatal("matrix Plan replay", err)
	}
	for _, fault := range []string{"schema", "policy", "population", "ordinal", "parameter", "base", "source", "context", "version", "query_digest", "constraint_digest"} {
		bad, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, groupedFactFault{Repository: f.db, fault: fault})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bad.Run(t.Context(), actor, run); err == nil {
			t.Fatal("matrix tamper accepted", fault)
		}
	}
	if calls != model.requests.Load() || reads != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("matrix replay or denial repeated model or source work")
	}
	replacement := []int{10, 15, 20}[:lanes-1]
	refined, err := query.Refine(t.Context(), actor, nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: answers(replacement)}})
	if err != nil {
		t.Fatal("matrix current replacement", err)
	}
	child, err := f.db.ReadQuery(t.Context(), scope, refined.QueryID)
	if err != nil || child.Clarification == nil || child.Clarification.BaseSQL != sql || len(child.Parameters) != lanes-1 {
		t.Fatal("matrix replacement custody", err)
	}
	changed, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: refined.QueryID, Operation: "matrix-current-run"})
	if err != nil {
		t.Fatal("matrix current Run", err)
	}
	assertRows(changed, replacement)
	t.Logf("independent numeric/NULL oracle and private custody passed: lanes=%d grain=%s raw=%t", lanes, grain, raw)
}

func factMatrixSQL(lanes int, grain string, raw bool) (string, string, int) {
	aliases := []string{"g", "r", "i", "a"}[:lanes]
	tables := []string{"analytics.orders o", "analytics.refunds r JOIN analytics.orders o ON r.order_id=o.order_id", "analytics.order_items i JOIN analytics.orders o ON i.order_id=o.order_id", "analytics.audit_totals a JOIN analytics.orders o ON a.id=o.order_id"}[:lanes]
	fields := []string{"o.total_usd", "r.amount_usd", "i.amount_usd", "a.total_usd"}[:lanes]
	keyProjection, group, question, keyNames, keyCount := "c.region AS region", "c.region", "Customer region", "region", 1
	if grain == "composite" {
		keyProjection += ",c.segment AS segment"
		group += ",c.segment"
		question += " and Customer segment"
		keyNames += ",segment"
		keyCount = 2
	}
	if grain == "calendar" {
		keyProjection = "date_trunc('month',o.ordered_at::timestamp) AS region"
		group = "date_trunc('month',o.ordered_at::timestamp)"
		question = "month of Order date"
	}
	var ctes, spine, outputs, joins []string
	for i, alias := range aliases {
		table := tables[i]
		if grain != "calendar" {
			table += " JOIN analytics.customers c ON o.customer_id=c.customer_id"
		}
		filter, where := "", ""
		if i == 0 {
			filter = " FILTER(WHERE o.status='paid')"
			if !raw {
				where = " WHERE o.status='paid'"
			}
		}
		ctes = append(ctes, alias+" AS (SELECT "+keyProjection+",SUM("+fields[i]+")"+filter+" AS value,COUNT("+fields[i]+")"+filter+" AS known FROM "+table+where+" GROUP BY "+group+")")
		spine = append(spine, "SELECT "+keyNames+" FROM "+alias)
		outputs = append(outputs, alias+".value AS value_"+alias, alias+".known AS known_"+alias)
		join := "LEFT JOIN " + alias + " ON keys.region IS NOT DISTINCT FROM " + alias + ".region"
		if keyCount == 2 {
			join += " AND keys.segment IS NOT DISTINCT FROM " + alias + ".segment"
		}
		joins = append(joins, join)
	}
	ctes = append(ctes, "keys AS ("+strings.Join(spine, " UNION ")+")")
	projectedKeys := "keys.region"
	if keyCount == 2 {
		projectedKeys += ",keys.segment"
	}
	return "WITH " + strings.Join(ctes, ", ") + " SELECT " + projectedKeys + "," + strings.Join(outputs, ",") + " FROM keys " + strings.Join(joins, " "), question, keyCount
}
