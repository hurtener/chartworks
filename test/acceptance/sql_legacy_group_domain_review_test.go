package acceptance

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
)

func TestSQLRecoveryLegacyGroupedDomainReviewAcceptance(t *testing.T) {
	f := liveCommerceSource(t)
	ctx := t.Context()
	if _, err := f.admin.Exec(ctx, `ALTER TABLE analytics.customers DROP CONSTRAINT customers_region_check; INSERT INTO analytics.customers VALUES(8,'consumer','quiet'); INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES(110,8,'2026-03-01',40,'cancelled')`); err != nil {
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
	question := nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: "Net revenue by Customer region", Locale: nlq.LanguageEnglish, MetricIDs: []string{"net_revenue"}, Kinds: []string{"kpi", "dimension"}, LimitPerKind: 5}
	origin, err := query.Preflight(ctx, actor, nlqexec.PreflightRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := store.NewScope(actor.Tenant(), actor.User())
	old, err := f.db.ReadQuery(ctx, sc, origin.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	old.ID = exec.Hash([]string{"historical-group-domain", origin.QueryID})[:32]
	old.Status = "planned"
	old.SQL = `WITH gross AS (SELECT c.region AS region,sum(o.total_usd) FILTER (WHERE o.status='paid') AS value FROM analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id GROUP BY c.region), refunds AS (SELECT c.region AS region,sum(r.amount_usd) AS value FROM analytics.refunds r JOIN analytics.orders o ON r.order_id=o.order_id JOIN analytics.customers c ON o.customer_id=c.customer_id GROUP BY c.region), keys AS (SELECT region FROM gross UNION SELECT region FROM refunds) SELECT keys.region,gross.value-refunds.value AS net FROM keys LEFT JOIN gross ON keys.region IS NOT DISTINCT FROM gross.region LEFT JOIN refunds ON keys.region IS NOT DISTINCT FROM refunds.region`
	for _, d := range pack.Datasets {
		scope := exec.RelationScope{Dataset: d.ID}
		for _, c := range d.Columns {
			scope.Columns = append(scope.Columns, c.SourceName)
		}
		sort.Strings(scope.Columns)
		old.RelationScope = append(old.RelationScope, scope)
	}
	sort.Slice(old.RelationScope, func(i, j int) bool { return old.RelationScope[i].Dataset < old.RelationScope[j].Dataset })
	binding, err := f.s.Binding(ctx, actor, pack.Datasets[0].Source.Source, question.Context)
	if err != nil {
		t.Fatal(err)
	}
	native, err := f.validator.ValidateWithin(ctx, actor, exec.Request{Source: binding.Source, Context: question.Context, SQL: old.SQL}, old.RelationScope)
	if err != nil || !native.Receipt().Validated {
		t.Fatal("historical native safety", err)
	}
	orders, customers, refunds := pack.Datasets[0].ID, pack.Datasets[2].ID, pack.Datasets[3].ID
	edge := func(left, right, lc, rc string) exec.AnalyticalJoin {
		if left > right {
			left, right, lc, rc = right, left, rc, lc
		}
		return exec.AnalyticalJoin{Left: left, Right: right, Type: "inner", LeftColumns: []string{lc}, RightColumns: []string{rc}}
	}
	orderJoin := edge(orders, customers, "customer_id", "customer_id")
	refundJoin := edge(refunds, orders, "order_id", "order_id")
	contract := exec.AnalyticalContract{Version: exec.AnalyticalGroupedPopulationsVersion, Binding: exec.Hash(binding), Semantics: old.Route.Selection.Digest, Dataset: orders, Metrics: []exec.AnalyticalMetric{{ID: pack.Topic + ":kpi:net_revenue", Expression: exec.AnalyticalExpression{Op: "-", Args: []exec.AnalyticalExpression{{Op: "sum", Column: "total_usd", Filters: []exec.AnalyticalFilter{{Column: "status", Kind: "eq", Values: []string{"paid"}}}}, {Op: "sum", Column: refunds + "/amount_usd"}}}}}, Grain: &exec.AnalyticalGrain{Policy: exec.AnalyticalCalendarPolicy, Columns: []string{customers + "/region"}, Dimensions: []string{pack.Topic + ":dimension:region"}}, Intent: &exec.AnalyticalIntent{Policy: exec.AnalyticalIntentPolicy}, QueryPopulation: &exec.AnalyticalQueryPopulation{Policy: exec.AnalyticalQueryPopulationPolicy}, GroupedPopulations: &exec.AnalyticalGroupedPopulations{Policy: exec.AnalyticalGroupedPopulationPolicy}}
	for _, fact := range facts {
		lane := exec.AnalyticalGroupedLane{Dataset: fact, Joins: []exec.AnalyticalJoin{orderJoin}}
		if fact == refunds {
			lane.Joins = []exec.AnalyticalJoin{refundJoin, orderJoin}
		}
		contract.GroupedPopulations.Lanes = append(contract.GroupedPopulations.Lanes, lane)
	}
	// Reconstruct retained evidence exactly as the historical version stored it;
	// the corrected checker deliberately does not certify this ambiguous SQL.
	old.AnalyticalVersion = 7
	old.Analytical = &exec.AnalyticalReceipt{Version: contract.Version, Scope: strings.ReplaceAll(exec.AnalyticalGrainScope, "single_base_relation", "independent_grouped_populations"), Contract: exec.Hash(contract), Query: exec.AnalyticalQueryDigest(old.SQL, nil), Metrics: []string{contract.Metrics[0].ID}, Grouping: contract.Grain.Dimensions, Intent: exec.AnalyticalIntentPolicy, QueryPopulation: exec.AnalyticalQueryPopulationPolicy}
	if err := f.db.CreateQuery(ctx, sc, old); err != nil {
		t.Fatal("historical receipt fixture", err)
	}
	calls := model.requests.Load()
	metadata := support.Raw(t, f.dsn)
	attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	if _, err := query.Refine(ctx, actor, nlqexec.RefineRequest{QueryID: old.ID}); nlqexec.GenerationProblem(err) == nil {
		t.Fatal("missing localized group review", err)
	}
	if model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
		t.Fatal("ambiguous origin generated or executed")
	}
	// Only a separately reviewed publication supplies the current domain choice.
	pack.Version = "v2"
	for _, id := range facts {
		pack.GroupedPopulation.GroupDomains = append(pack.GroupedPopulation.GroupDomains, semantics.GroupedPopulationDomain{Dataset: id, Domain: exec.AnalyticalGroupDomainQualifying})
	}
	next, err := draft.Save(ctx, author, drafts.SaveRequest{Expected: 1, Pack: pack, Change: "Review group existence"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := topic.Review(ctx, author, pack.Topic, topics.ReviewRequest{DraftRevision: next.Metadata.Revision, Digest: next.Metadata.Digest, Decision: "approve", Note: "Reviewed current group existence"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := topic.Publish(ctx, author, pack.Topic, topics.PublishRequest{Review: review.ID, Expected: 1}); err != nil {
		t.Fatal(err)
	}
	ready, err := query.Preflight(ctx, actor, nlqexec.PreflightRequest{QuestionRequest: question})
	if err != nil || ready.Route.Selection == nil || ready.Route.Clarification != nil {
		t.Fatal("current reviewed preflight", err)
	}
	input := nlqexec.RefineRequest{QueryID: old.ID, IntentReview: &nlqexec.LegacyIntentReview{QueryID: ready.QueryID, SelectionDigest: ready.Route.Selection.Digest}}
	calls = model.requests.Load()
	for _, mutate := range []func(*nlqexec.LegacyIntentReview){func(r *nlqexec.LegacyIntentReview) { r.SelectionDigest = origin.Route.Selection.Digest }, func(r *nlqexec.LegacyIntentReview) { r.AnswerContext = "mixed" }, func(r *nlqexec.LegacyIntentReview) { r.QueryID = origin.QueryID }} {
		bad := input
		copy := *input.IntentReview
		bad.IntentReview = &copy
		mutate(&copy)
		if _, err := query.Refine(ctx, actor, bad); err == nil {
			t.Fatal("foreign/stale/mixed selection accepted")
		}
		if model.requests.Load() != calls {
			t.Fatal("invalid domain review reached provider")
		}
	}

	for _, who := range []struct{ tenant, user, session string }{{actor.Tenant(), "other-review-user", actor.Session()}, {actor.Tenant(), actor.User(), "other-review-session"}, {"other-review-tenant", actor.User(), actor.Session()}} {
		foreign, err := identity.FromVerified(who.tenant, who.user, who.session, actor.Scopes(), actor.Deadline(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := query.Refine(ctx, foreign, input); err == nil {
			t.Fatal("foreign domain review admitted")
		}
		if model.requests.Load() != calls {
			t.Fatal("foreign review reached provider")
		}
	}
	currentSQL := strings.Replace(old.SQL, "sum(o.total_usd) FILTER (WHERE o.status='paid')", "sum(o.total_usd)", 1)
	currentSQL = strings.Replace(currentSQL, "o.customer_id=c.customer_id GROUP BY c.region", "o.customer_id=c.customer_id WHERE o.status='paid' GROUP BY c.region", 1)
	model.mode.Store(phase18RawResponse(t, currentSQL))
	model.mu.Lock()
	start := len(model.requestBodies)
	model.mu.Unlock()
	var results [2]nlqexec.PlanResult
	var errs [2]error
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = query.Refine(ctx, actor, input) }(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil || results[i].QueryID == "" || results[i].QueryID != results[0].QueryID {
			t.Fatal("reviewed v7 replacement", err)
		}
	}
	saved, err := f.db.ReadQuery(ctx, sc, results[0].QueryID)
	if err != nil || saved.IntentReview == nil || saved.IntentReview.SelectionDigest != ready.Route.Selection.Digest || saved.ParentDigest != nlqexec.QueryLineageDigest(old) || saved.TopicVersions[0] == old.TopicVersions[0] {
		t.Fatal("lost distinct publication pins", err)
	}
	restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	run, err := restarted.Run(ctx, actor, nlqexec.RunRequest{QueryID: saved.ID, Operation: saved.ID + "-run"})
	if err != nil || run.Execution.Result == nil {
		t.Fatal("reviewed v7 restart/run", err)
	}
	exact := map[string]string{}
	for _, row := range run.Execution.Result.Rows {
		exact[string(row[0])] = string(row[1])
		if string(row[0]) == `"quiet"` {
			t.Fatal("canceled-only phantom group survived reviewed replacement")
		}
	}
	if exec.Hash(exact) != exec.Hash(map[string]string{`"north"`: `"300.00"`, `"south"`: `"125.00"`, `"west"`: "null"}) {
		t.Fatal("review changed measured values", exact)
	}
	calls = model.requests.Load()
	again, err := restarted.Refine(ctx, actor, input)
	if err != nil || again.QueryID != saved.ID || calls != model.requests.Load() {
		t.Fatal("review replay generated", err)
	}
	model.mu.Lock()
	wire := strings.Join(model.requestBodies[start:], "\n")
	model.mu.Unlock()
	if strings.Contains(wire, "previous_sql") || strings.Contains(wire, old.SQL) {
		t.Fatal("old SQL became intent authority")
	}
	unchanged, err := f.db.ReadQuery(ctx, sc, old.ID)
	if err != nil || nlqexec.QueryLineageDigest(unchanged) != nlqexec.QueryLineageDigest(old) {
		t.Fatal("historical query changed", err)
	}
	calls = model.requests.Load()
	attempts = count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET status='failed',revision=revision+1 WHERE tenant_id=$1 AND query_id=$2`, actor.Tenant(), ready.QueryID); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Run(ctx, actor, nlqexec.RunRequest{QueryID: saved.ID, Operation: saved.ID + "-run"}); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("stale ready origin replay", err)
	}
	if model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
		t.Fatal("stale ready replay did model or row work")
	}
}
