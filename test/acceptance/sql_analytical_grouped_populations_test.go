package acceptance

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
)

// Recorded provider SQL is executed through the real PostgreSQL/native and
// durable Plan/Run boundaries. It is not live model-quality evidence.
func TestSQLRecoveryGroupedCommerceAcceptance(t *testing.T) {
	f := liveCommerceSource(t)
	_, err := f.admin.Exec(t.Context(), `ALTER TABLE analytics.customers ALTER COLUMN region DROP NOT NULL; ALTER TABLE analytics.customers DROP CONSTRAINT customers_region_check; ALTER TABLE analytics.orders ALTER COLUMN total_usd DROP NOT NULL; ALTER TABLE analytics.refunds ALTER COLUMN amount_usd DROP NOT NULL;
 INSERT INTO analytics.customers VALUES(5,'consumer','east'),(6,'consumer',NULL),(7,'business',NULL),(8,'consumer','quiet');
 INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES(107,5,'2026-03-01',40,'cancelled'),(108,6,'2026-03-01',120,'paid'),(109,7,'2026-03-01',30,'paid'),(110,8,'2026-03-01',40,'cancelled');
 INSERT INTO analytics.refunds VALUES(205,107,'2026-03-02',5),(206,108,'2026-03-02',10),(207,108,'2026-03-03',15);`)
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := liveCommerceTopics(t, f)
	facts := []string{pack.Datasets[0].ID, pack.Datasets[3].ID}
	sort.Strings(facts)
	pack.GroupedPopulation = &semantics.GroupedPopulationPolicy{Policy: semantics.GroupedPopulationUnionPolicy, Datasets: facts}
	for _, id := range facts {
		pack.GroupedPopulation.GroupDomains = append(pack.GroupedPopulation.GroupDomains, semantics.GroupedPopulationDomain{Dataset: id, Domain: exec.AnalyticalGroupDomainQualifying})
	}
	for i := range pack.Joins {
		if pack.Joins[i].ID == "orders-refunds" {
			pack.Joins[i].Type = semantics.JoinInner
		}
	}
	pack.Measures = append(pack.Measures, semantics.Measure{ID: "paid_values", Name: "Paid order values", Field: pack.Measures[0].Field, Aggregation: semantics.AggregationCount, Filters: append([]semantics.SemanticFilter(nil), pack.Measures[0].Filters...)}, semantics.Measure{ID: "refund_values", Name: "Refund values", Field: pack.Measures[1].Field, Aggregation: semantics.AggregationCount})
	pack.Measures = append(pack.Measures, semantics.Measure{ID: "cancelled_values", Name: "Cancelled order values", Field: pack.Measures[0].Field, Aggregation: semantics.AggregationCount, Filters: []semantics.SemanticFilter{{ID: "cancelled-values", Field: pack.Measures[0].Filters[0].Field, Operator: "eq", Values: []string{"cancelled"}}}})
	pack.KPIs[0].Description = "Paid order revenue less all refund events, aligned by the reviewed customer grouping. Missing lane values remain NULL."
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
	phase17PublishTopic(t, draft, topic, author, pack)
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
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	statement := `WITH gross AS (SELECT c.region AS region,sum(o.total_usd) AS value FROM analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id WHERE o.status='paid' GROUP BY c.region), refunds AS (SELECT c.region AS region,sum(r.amount_usd) AS value FROM analytics.refunds r JOIN analytics.orders o ON r.order_id=o.order_id JOIN analytics.customers c ON o.customer_id=c.customer_id GROUP BY c.region), keys AS (SELECT region FROM gross UNION SELECT region FROM refunds) SELECT keys.region,gross.value-refunds.value AS net FROM keys LEFT JOIN gross ON keys.region IS NOT DISTINCT FROM gross.region LEFT JOIN refunds ON keys.region IS NOT DISTINCT FROM refunds.region`
	request := nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: "Net revenue by Customer region", Locale: nlq.LanguageEnglish, MetricIDs: []string{"net_revenue"}, Kinds: []string{"kpi", "dimension"}, LimitPerKind: 5}
	plan := func() nlqexec.PlanResult {
		t.Helper()
		model.mode.Store(phase18RawResponse(t, statement))
		p, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request})
		if err != nil {
			t.Fatal("grouped Plan", err)
		}
		if p.Analytical == nil || p.Analytical.Version != exec.AnalyticalGroupedProgramsVersion || !strings.HasSuffix(p.Analytical.Scope, ";independent_grouped_populations") {
			t.Fatal("missing grouped proof", p.Analytical)
		}
		return p
	}
	p := plan()
	run, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: "grouped-commerce"})
	if err != nil || run.Execution.Result == nil {
		t.Fatal("grouped Run", err)
	}
	got := map[string]string{}
	for _, row := range run.Execution.Result.Rows {
		region := "NULL"
		value := "NULL"
		if string(row[0]) != "null" && json.Unmarshal(row[0], &region) != nil {
			t.Fatal("region type")
		}
		if string(row[1]) != "null" && json.Unmarshal(row[1], &value) != nil {
			t.Fatal("amount type")
		}
		if _, duplicate := got[region]; duplicate {
			t.Fatal("group duplicated", region)
		}
		got[region] = value
	}
	expected := map[string]string{"north": "300.00", "south": "125.00", "west": "NULL", "east": "NULL", "NULL": "125.00"}
	if exec.Hash(got) != exec.Hash(expected) {
		t.Fatal("wrong grouped Commerce totals", got)
	}
	t.Run("group_domain_is_not_aggregate_filter_equivalence", func(t *testing.T) {
		badDomain := strings.Replace(statement, "sum(o.total_usd)", "sum(o.total_usd) FILTER (WHERE o.status='paid')", 1)
		badDomain = strings.Replace(badDomain, " WHERE o.status='paid' GROUP BY", " GROUP BY", 1)
		rows, err := f.admin.Query(t.Context(), badDomain)
		if err != nil {
			t.Fatal("domain witness SQL", err)
		}
		witness := false
		for rows.Next() {
			var region *string
			var amount *string
			if err := rows.Scan(&region, &amount); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if region != nil && *region == "quiet" && amount == nil {
				witness = true
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if !witness {
			t.Fatal("missing real-source canceled-only NULL group witness")
		}
		model.mode.Store(phase18RawResponse(t, badDomain))
		badPlan, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request})
		if err == nil {
			result, runErr := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: badPlan.QueryID, Operation: badPlan.QueryID + "-domain-witness"})
			if runErr != nil {
				t.Fatal("accepted bad domain failed execution", runErr)
			}
			t.Fatalf("accepted metric-filter relocation changed grouped population: executed %d rows, expected %d", len(result.Execution.Result.Rows), len(expected))
		}
	})
	baseStatement := statement
	derivedSpine := strings.Replace(statement, ", keys AS (SELECT region FROM gross UNION SELECT region FROM refunds)", "", 1)
	derivedSpine = strings.Replace(derivedSpine, "FROM keys LEFT JOIN", "FROM (SELECT region FROM gross UNION SELECT region FROM refunds) keys LEFT JOIN", 1)
	wrappedLane := strings.Replace(statement, "gross AS (SELECT c.region", "gross AS (SELECT w.region,w.value FROM (SELECT c.region", 1)
	wrappedLane = strings.Replace(wrappedLane, "GROUP BY c.region), refunds AS", "GROUP BY c.region) w), refunds AS", 1)
	for _, equivalent := range []string{"SELECT q.region,q.net FROM (" + baseStatement + ") q", derivedSpine, wrappedLane} {
		statement = equivalent
		eq := plan()
		result, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: eq.QueryID, Operation: eq.QueryID + "-equivalent"})
		if err != nil || result.Execution.Result == nil {
			t.Fatal("derived grouped execution", err)
		}
		actual := map[string]string{}
		for _, row := range result.Execution.Result.Rows {
			key, value := "NULL", "NULL"
			if string(row[0]) != "null" && json.Unmarshal(row[0], &key) != nil {
				t.Fatal("region type")
			}
			if string(row[1]) != "null" && json.Unmarshal(row[1], &value) != nil {
				t.Fatal("metric type")
			}
			if _, duplicate := actual[key]; duplicate {
				t.Fatal("derived duplicate group")
			}
			actual[key] = value
		}
		if exec.Hash(actual) != exec.Hash(expected) {
			t.Fatal("derived wrapper changed populations", actual)
		}
	}
	statement = "SELECT q.region,q.net FROM (" + baseStatement + ") q ORDER BY q.net DESC NULLS LAST LIMIT 1"
	request.Question = "Net revenue by Customer region ordered by Net revenue descending nulls last limit 1"
	ranked := plan()
	rankedRows, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: ranked.QueryID, Operation: ranked.QueryID + "-ranked"})
	if err != nil || rankedRows.Execution.Result == nil || len(rankedRows.Execution.Result.Rows) != 1 || string(rankedRows.Execution.Result.Rows[0][0]) != `"north"` || string(rankedRows.Execution.Result.Rows[0][1]) != `"300.00"` {
		t.Fatal("final derived ordering/limit changed result", err)
	}
	model.mode.Store(phase18RawResponse(t, "SELECT q.region,q.net FROM ("+baseStatement+" ORDER BY net DESC NULLS LAST LIMIT 1) q"))
	if _, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); err == nil {
		t.Fatal("buried order/limit proved final-layer intent")
	}
	request.Question = "Net revenue by Customer region"
	statement = baseStatement
	restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	metadata := support.Raw(t, f.dsn)
	calls := model.requests.Load()
	attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	replay, err := restarted.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: "grouped-commerce"})
	if err != nil || replay.Execution.Result == nil || exec.Hash(replay.Execution.Result.Rows) != exec.Hash(run.Execution.Result.Rows) {
		t.Fatal("durable grouped replay", err)
	}
	if calls != model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("grouped replay made provider/source call")
	}
	for _, mutation := range []string{`analytical_version=6`, `analytical=analytical-'grouping'`, `analytical=analytical-'intent'`, `analytical=analytical-'query_population'`, `analytical=jsonb_set(analytical,'{scope}','"selected_metric_expression_population_and_grouping;single_base_relation"')`} {
		if _, err := metadata.Exec(t.Context(), `UPDATE chartworks.nlq_queries SET `+mutation+` WHERE query_id=$1`, p.QueryID); err == nil {
			t.Fatal("grouped retained proof was mutable", mutation)
		}
	}
	for name, bad := range map[string]string{
		"duplicate_spine":    strings.Replace(statement, " UNION ", " UNION ALL ", 1),
		"null_loss":          strings.Replace(statement, "IS NOT DISTINCT FROM", "=", 1),
		"missing_group_loss": strings.Replace(statement, "keys LEFT JOIN gross", "keys JOIN gross", 1),
		"zero_fill":          strings.Replace(statement, "gross.value-refunds.value", "coalesce(gross.value,0)-coalesce(refunds.value,0)", 1),
		"raw_fanout":         strings.Replace(statement, "FROM analytics.orders o JOIN analytics.customers", "FROM analytics.orders o JOIN analytics.refunds extra ON o.order_id=extra.order_id JOIN analytics.customers", 1),
	} {
		t.Run(name, func(t *testing.T) {
			model.mode.Store(phase18RawResponse(t, bad))
			if _, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); err == nil {
				t.Fatal("unproved grouped SQL accepted")
			}
		})
	}
	// Inspect the effective provider requests, including correction requests.
	// A correct recorded response alone cannot prove coherent model instructions.
	model.mu.Lock()
	bodies := append([]string(nil), model.requestBodies...)
	model.mu.Unlock()
	seenRoles := map[string]bool{}
	for _, body := range bodies {
		var wire struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal([]byte(body), &wire) != nil {
			t.Fatal("invalid captured provider JSON")
		}
		role := ""
		for _, candidate := range []string{"sqlgen", "sqlfix"} {
			if wire.Model == model.cfg.Roles[candidate].Model {
				role = candidate
			}
		}
		if role == "" {
			continue
		}
		seenRoles[role] = true
		if len(wire.Messages) < 1 || wire.Messages[0].Role != "system" {
			t.Fatal("grouped provider request lost system instructions")
		}
		system := wire.Messages[0].Content
		for _, required := range []string{"Only the reviewed UNION DISTINCT of grouped lane keys", "all other set operations are unsupported", "IS NOT DISTINCT FROM", "Exact compiled lane contract:", "key-spine CTE or derived SELECT", "Keep reviewed ORDER/LIMIT at the final SELECT", exec.AnalyticalGroupedPopulationPolicy} {
			if !strings.Contains(system, required) {
				t.Fatal("grouped provider wire omitted required scoped instruction", required)
			}
		}
		for _, forbidden := range []string{"Use one qualified base relation, with no joins or nested SELECTs.", "Windows and set operations are not analytically supported by this contract.", "only a reviewed filter shared by every selected metric may be moved to WHERE"} {
			if strings.Contains(system, forbidden) {
				t.Fatal("grouped provider wire contradicted proved SQL shape", forbidden)
			}
		}
	}
	if !seenRoles["sqlgen"] || !seenRoles["sqlfix"] {
		t.Fatal("grouped provider-wire regression did not observe generation and correction", seenRoles)
	}
	t.Run("reviewed_raw_and_qualifying_count_domains", func(t *testing.T) {
		if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.customers VALUES(9,'consumer','all_null'); INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES(111,9,'2026-03-01',NULL,'paid'); INSERT INTO analytics.refunds VALUES(208,111,'2026-03-02',NULL)`); err != nil {
			t.Fatal(err)
		}
		rawPack := pack
		rawPack.Topic = pack.Topic + "-raw-domain"
		rawPack.GroupedPopulation = &semantics.GroupedPopulationPolicy{Policy: semantics.GroupedPopulationUnionPolicy, Datasets: append([]string(nil), facts...)}
		for _, id := range facts {
			rawPack.GroupedPopulation.GroupDomains = append(rawPack.GroupedPopulation.GroupDomains, semantics.GroupedPopulationDomain{Dataset: id, Domain: exec.AnalyticalGroupDomainRaw})
		}
		phase17PublishTopic(t, draft, topic, author, rawPack)
		countSQL := strings.ReplaceAll(strings.ReplaceAll(baseStatement, "sum(o.total_usd)", "count(o.total_usd)"), "sum(r.amount_usd)", "count(r.amount_usd)")
		countSQL = strings.Replace(countSQL, "gross.value-refunds.value AS net", "gross.value AS paid_count,refunds.value AS refund_count", 1)
		for _, rawDomain := range []bool{false, true} {
			sql := countSQL
			topicID := pack.Topic
			expectedCounts := map[string][]string{"north": {"3", "2"}, "south": {"1", "2"}, "west": {"1", "NULL"}, "east": {"NULL", "1"}, "NULL": {"2", "2"}, "all_null": {"0", "0"}}
			if rawDomain {
				topicID = rawPack.Topic
				sql = strings.Replace(sql, "count(o.total_usd)", "count(o.total_usd) FILTER (WHERE o.status='paid')", 1)
				sql = strings.Replace(sql, " WHERE o.status='paid' GROUP BY", " GROUP BY", 1)
				expectedCounts["east"] = []string{"0", "1"}
				expectedCounts["quiet"] = []string{"0", "NULL"}
			}
			countRequest := request
			countRequest.Topic = topicID
			countRequest.Question = "Paid order values and Refund values by Customer region"
			countRequest.MetricIDs = []string{"paid_values", "refund_values"}
			countRequest.Kinds = []string{"measure", "dimension"}
			model.mode.Store(phase18RawResponse(t, sql))
			planned, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: countRequest})
			if err != nil {
				t.Fatal("reviewed count-domain Plan", rawDomain, err)
			}
			result, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: planned.QueryID + "-counts"})
			if err != nil || result.Execution.Result == nil {
				t.Fatal("count-domain Run", err)
			}
			actual := map[string][]string{}
			for _, row := range result.Execution.Result.Rows {
				key := "NULL"
				if string(row[0]) != "null" && json.Unmarshal(row[0], &key) != nil {
					t.Fatal("count region type")
				}
				values := []string{"NULL", "NULL"}
				for i := 0; i < 2; i++ {
					if string(row[i+1]) != "null" && json.Unmarshal(row[i+1], &values[i]) != nil {
						t.Fatal("count value type")
					}
				}
				if _, duplicate := actual[key]; duplicate {
					t.Fatal("count group duplicate")
				}
				actual[key] = values
			}
			if exec.Hash(actual) != exec.Hash(expectedCounts) {
				t.Fatal("zero, NULL-input and missing-group semantics collapsed", rawDomain, actual)
			}
			model.mode.Store(phase18RawResponse(t, strings.Replace(sql, "count(o.total_usd)", "count(*)", 1)))
			if _, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: countRequest}); err == nil {
				t.Fatal("all-NULL COUNT(column) substituted by COUNT(*)")
			}
		}
		unionSQL := strings.Replace(countSQL, "count(o.total_usd) AS value", "count(o.total_usd) FILTER (WHERE o.status='paid') AS value,count(o.total_usd) FILTER (WHERE o.status='cancelled') AS cancelled_value", 1)
		unionSQL = strings.Replace(unionSQL, "WHERE o.status='paid' GROUP BY", "WHERE (o.status='paid' OR o.status='cancelled') GROUP BY", 1)
		unionSQL = strings.Replace(unionSQL, "gross.value AS paid_count,", "gross.value AS paid_count,gross.cancelled_value AS cancelled_count,", 1)
		unionRequest := request
		unionRequest.Question = "Paid order values and Cancelled order values and Refund values by Customer region"
		unionRequest.MetricIDs = []string{"paid_values", "cancelled_values", "refund_values"}
		unionRequest.Kinds = []string{"measure", "dimension"}
		model.mode.Store(phase18RawResponse(t, unionSQL))
		unionPlan, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: unionRequest})
		if err != nil {
			t.Fatal("qualifying union Plan", err)
		}
		unionRun, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: unionPlan.QueryID, Operation: unionPlan.QueryID + "-union"})
		if err != nil || unionRun.Execution.Result == nil {
			t.Fatal("qualifying union Run", err)
		}
		unionExpected := map[string][]string{"north": {"3", "0", "2"}, "south": {"1", "1", "2"}, "west": {"1", "0", "NULL"}, "east": {"0", "1", "1"}, "quiet": {"0", "1", "NULL"}, "NULL": {"2", "0", "2"}, "all_null": {"0", "0", "0"}}
		unionActual := map[string][]string{}
		for _, row := range unionRun.Execution.Result.Rows {
			key := "NULL"
			if string(row[0]) != "null" && json.Unmarshal(row[0], &key) != nil {
				t.Fatal("union region type")
			}
			values := []string{"NULL", "NULL", "NULL"}
			for i := 0; i < 3; i++ {
				if string(row[i+1]) != "null" && json.Unmarshal(row[i+1], &values[i]) != nil {
					t.Fatal("union count type")
				}
			}
			if _, duplicate := unionActual[key]; duplicate {
				t.Fatal("union group duplicate")
			}
			unionActual[key] = values
		}
		if exec.Hash(unionActual) != exec.Hash(unionExpected) {
			t.Fatal("qualifying population union changed counts", unionActual)
		}
		model.mode.Store(phase18RawResponse(t, strings.Replace(unionSQL, "o.status='paid' OR o.status='cancelled'", "o.status='paid' AND o.status='cancelled'", 1)))
		if _, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: unionRequest}); err == nil {
			t.Fatal("population intersection replaced reviewed union")
		}
		if _, err := f.admin.Exec(t.Context(), `DELETE FROM analytics.refunds WHERE refund_id=208; DELETE FROM analytics.orders WHERE order_id=111; DELETE FROM analytics.customers WHERE customer_id=9`); err != nil {
			t.Fatal(err)
		}
	})
	// Empty refund input produces no refund groups. The existing gross groups
	// remain in the spine and subtraction stays NULL; no implicit count/zero fill.
	if _, err := f.admin.Exec(t.Context(), "DELETE FROM analytics.refunds"); err != nil {
		t.Fatal(err)
	}
	statement = "SELECT q.region,q.net FROM (" + baseStatement + ") q"
	empty := plan()
	out, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: empty.QueryID, Operation: "grouped-empty-refunds"})
	if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 4 {
		t.Fatal("empty grouped lane", err)
	}
	for _, row := range out.Execution.Result.Rows {
		if string(row[1]) != "null" {
			t.Fatal("empty lane was zero-filled")
		}
	}
	if _, err := f.admin.Exec(t.Context(), "DELETE FROM analytics.order_items; DELETE FROM analytics.orders"); err != nil {
		t.Fatal(err)
	}
	bothEmpty := plan()
	zero, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: bothEmpty.QueryID, Operation: "grouped-empty-both"})
	if err != nil || zero.Execution.Result == nil || len(zero.Execution.Result.Rows) != 0 {
		t.Fatal("empty grouped inputs invented a row", err)
	}
	// A stale source cannot borrow the former dimension uniqueness evidence.
	if _, err := f.admin.Exec(t.Context(), `ALTER TABLE analytics.customers DROP CONSTRAINT customers_pkey CASCADE; INSERT INTO analytics.customers VALUES(1,'consumer','north')`); err != nil {
		t.Fatal(err)
	}
	model.mode.Store(phase18RawResponse(t, statement))
	if _, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); err == nil {
		t.Fatal("duplicate dimension with stale key evidence admitted")
	}
}
