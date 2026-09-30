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
	_, err := f.admin.Exec(t.Context(), `ALTER TABLE analytics.customers ALTER COLUMN region DROP NOT NULL; ALTER TABLE analytics.customers DROP CONSTRAINT customers_region_check;
 INSERT INTO analytics.customers VALUES(5,'consumer','east'),(6,'consumer',NULL),(7,'business',NULL);
 INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES(107,5,'2026-03-01',40,'cancelled'),(108,6,'2026-03-01',120,'paid'),(109,7,'2026-03-01',30,'paid');
 INSERT INTO analytics.refunds VALUES(205,107,'2026-03-02',5),(206,108,'2026-03-02',10),(207,108,'2026-03-03',15);`)
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := liveCommerceTopics(t, f)
	facts := []string{pack.Datasets[0].ID, pack.Datasets[3].ID}
	sort.Strings(facts)
	pack.GroupedPopulation = &semantics.GroupedPopulationPolicy{Policy: semantics.GroupedPopulationUnionPolicy, Datasets: facts}
	for i := range pack.Joins {
		if pack.Joins[i].ID == "orders-refunds" {
			pack.Joins[i].Type = semantics.JoinInner
		}
	}
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
		if p.Analytical == nil || p.Analytical.Version != exec.AnalyticalGroupedPopulationsVersion || !strings.HasSuffix(p.Analytical.Scope, ";independent_grouped_populations") {
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
		for _, required := range []string{"Only the reviewed UNION DISTINCT of grouped lane keys", "all other set operations are unsupported", "IS NOT DISTINCT FROM", "Exact compiled lane contract:", exec.AnalyticalGroupedPopulationPolicy} {
			if !strings.Contains(system, required) {
				t.Fatal("grouped provider wire omitted required scoped instruction", required)
			}
		}
		for _, forbidden := range []string{"Use one qualified base relation, with no joins or nested SELECTs.", "Windows and set operations are not analytically supported by this contract."} {
			if strings.Contains(system, forbidden) {
				t.Fatal("grouped provider wire contradicted proved SQL shape", forbidden)
			}
		}
	}
	if !seenRoles["sqlgen"] || !seenRoles["sqlfix"] {
		t.Fatal("grouped provider-wire regression did not observe generation and correction", seenRoles)
	}
	// Empty refund input produces no refund groups. The existing gross groups
	// remain in the spine and subtraction stays NULL; no implicit count/zero fill.
	if _, err := f.admin.Exec(t.Context(), "DELETE FROM analytics.refunds"); err != nil {
		t.Fatal(err)
	}
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
