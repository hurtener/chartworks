package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/config"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
)

func TestSQLRecoveryAnalyticalJoinedCommerceAcceptance(t *testing.T) {
	f := liveCommerceSource(t)
	pack, _ := liveCommerceTopics(t, f)
	model := newGatewayFixture(t, func(cfg *config.Gateway) {
		role := cfg.Roles["embedding"]
		role.MaxBatchItems = 64
		role.MaxBatchBytes = 4096
		cfg.Roles["embedding"] = role
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
	sql := `SELECT c.region, sum(o.total_usd) AS revenue FROM analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id WHERE o.status='paid' GROUP BY c.region ORDER BY c.region`
	model.mode.Store(phase18RawResponse(t, sql))
	request := nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: "Gross revenue by Customer region", Locale: nlq.LanguageEnglish, MetricIDs: []string{"gross_revenue"}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5}
	p, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request})
	if err != nil {
		t.Fatal("joined plan", err)
	}
	if p.Analytical == nil || !strings.Contains(p.Analytical.Scope, "physically_unique_reviewed_joins") {
		t.Fatal("missing joined proof", p.Analytical)
	}
	run, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: "joined-commerce"})
	if err != nil || run.Execution.Result == nil {
		t.Fatal("joined run", err)
	}
	got := map[string]string{}
	for _, row := range run.Execution.Result.Rows {
		var region, amount string
		if json.Unmarshal(row[0], &region) != nil || json.Unmarshal(row[1], &amount) != nil {
			t.Fatal("typed row")
		}
		got[region] = amount
	}
	if len(got) != 3 || got["north"] != "350.00" || got["south"] != "200.00" || got["west"] != "90.00" {
		t.Fatal("wrong independent regional totals", got)
	}
	restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: "joined-commerce"}); err != nil {
		t.Fatal("joined replay", err)
	}
	// A raw one-to-many item join cannot borrow the customer uniqueness proof.
	model.mode.Store(phase18RawResponse(t, strings.ReplaceAll(sql, "JOIN analytics.customers c ON o.customer_id=c.customer_id", "JOIN analytics.order_items i ON o.order_id=i.order_id JOIN analytics.customers c ON o.customer_id=c.customer_id")))
	if _, err = query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); err == nil {
		t.Fatal("fan-out candidate accepted")
	}
	// Independent scalar aggregates cannot multiply the two fact populations.
	request.Question = "Net revenue"
	request.MetricIDs = []string{"net_revenue"}
	request.Kinds = []string{"kpi"}
	for _, statement := range []string{
		`WITH gross AS (SELECT sum(total_usd) AS value FROM analytics.orders WHERE status='paid'), refunds AS (SELECT sum(amount_usd) AS value FROM analytics.refunds) SELECT gross.value-refunds.value AS net FROM gross CROSS JOIN refunds`,
		`SELECT gross.value-refunds.value AS net FROM (SELECT sum(total_usd) AS value FROM analytics.orders WHERE status='paid') gross CROSS JOIN (SELECT sum(amount_usd) AS value FROM analytics.refunds) refunds`,
	} {
		model.mode.Store(phase18RawResponse(t, statement))
		p, err = query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request})
		if err != nil {
			t.Fatal("independent plan", err)
		}
		if p.Analytical == nil || !strings.Contains(p.Analytical.Scope, "independent_singleton_populations") {
			t.Fatal("missing singleton proof")
		}
		out, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 1 {
			t.Fatal("independent execution", err)
		}
		var net string
		if json.Unmarshal(out.Execution.Result.Rows[0][0], &net) != nil || net != "515.00" {
			t.Fatal("independent expected net", net)
		}
	}
}

func TestSQLRecoveryPhysicalUniqueCatalogAcceptance(t *testing.T) {
	f := newSourceFixture(t, nil)
	ctx := context.Background()
	if _, err := f.admin.Exec(ctx, `CREATE UNIQUE INDEX items_partial ON analytics.items(sale_id) WHERE quantity>0; CREATE UNIQUE INDEX items_expression ON analytics.items((sale_id+0)); CREATE UNIQUE INDEX items_composite ON analytics.items(sale_id,quantity)`); err != nil {
		t.Fatal(err)
	}
	source := f.create(t, "physical-keys")
	b, err := f.s.Binding(ctx, f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	var items readexec.Relation
	for _, r := range b.Relations {
		if r.Name == "items" {
			items = r
		}
	}
	if items.HasUniqueKey([]string{"sale_id"}) || !items.HasUniqueKey([]string{"sale_id", "quantity"}) {
		t.Fatal("partial/expression/composite evidence", items.UniqueKeys)
	}
	if _, err = f.admin.Exec(ctx, `DROP INDEX analytics.items_composite; DROP INDEX analytics.items_partial; DROP INDEX analytics.items_expression; INSERT INTO analytics.items VALUES(1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.validator.Validate(ctx, f.e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: "SELECT sale_id FROM analytics.items"}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("removed uniqueness not fenced at native boundary", err)
	}
	var duplicates int
	if err = f.admin.QueryRow(ctx, "SELECT count(*) FROM analytics.items WHERE sale_id=1").Scan(&duplicates); err != nil || duplicates != 2 {
		t.Fatal("duplicate key fixture", duplicates, err)
	}
}
