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

// Fault injection changes only the read projection of this synthetic repository;
// actual database immutability protections remain enabled.
type groupedOwnedOrdinalFault struct {
	nlqexec.Repository
	fault string
}

func (r groupedOwnedOrdinalFault) ReadQuery(ctx context.Context, scope store.Scope, id string) (nlqexec.QueryRecord, error) {
	q, err := r.Repository.ReadQuery(ctx, scope, id)
	if err == nil && q.Analytical != nil && q.Analytical.Version == exec.AnalyticalGroupedOwnedPopulationsVersion {
		if r.fault == "ordinal" {
			copy := *q.Analytical
			copy.Outputs = append([]exec.AnalyticalOutput(nil), copy.Outputs...)
			if len(copy.Outputs) > 0 {
				copy.Outputs[0].Column = 0
			}
			q.Analytical = &copy
		} else if q.Clarification != nil {
			evidence := *q.Clarification
			evidence.Binding.Bindings = append([]exec.BusinessParameterBinding(nil), evidence.Binding.Bindings...)
			if evidence.Binding.Validation != nil {
				v := *evidence.Binding.Validation
				evidence.Binding.Validation = &v
			}
			switch r.fault {
			case "schema":
				evidence.Binding.SchemaVersion = 2
			case "policy":
				evidence.Binding.PopulationPolicy = exec.AnalyticalScalarPopulationPolicy
			case "duplicate_lane":
				evidence.Binding.Bindings[1].Population = evidence.Binding.Bindings[0].Population
			case "constraint_digest":
				evidence.Binding.Constraints = exec.Hash("wrong-owned-constraints")
			case "source_digest":
				evidence.Binding.SourceBinding = exec.Hash("wrong-source-binding")
			case "validation_source":
				evidence.Binding.Validation.Source = "wrong-source"
			case "validation_context":
				evidence.Binding.Validation.Context = "wrong-context"
			case "indexes":
				evidence.Binding.Bindings[0].Parameters = []int{3, 4}
			}
			q.Clarification = &evidence
		}
	}
	return q, err
}

type groupedOwnedOrder struct {
	id     int
	region *string
	date   string
	amount *int
	paid   bool
}
type groupedOwnedRefund struct {
	order  int
	date   string
	amount *int
}

// Independent in-memory row oracle, with no SQL/compiler/proof reuse. Group
// existence precedes nullable SUM and COUNT behavior and is evaluated per fact.
func groupedOwnedOracle(orders []groupedOwnedOrder, refunds []groupedOwnedRefund, year string, activity, raw, calendar bool) map[string][2]string {
	type aggregate struct{ sum, known int }
	gross, returns := map[string]aggregate{}, map[string]aggregate{}
	byID := map[int]groupedOwnedOrder{}
	key := func(o groupedOwnedOrder) string {
		if calendar {
			return o.date[:7] + "-01"
		}
		if o.region == nil {
			return "NULL"
		}
		return *o.region
	}
	for _, o := range orders {
		byID[o.id] = o
		if !strings.HasPrefix(o.date, year) || !raw && !o.paid {
			continue
		}
		a := gross[key(o)]
		if o.paid && o.amount != nil {
			a.sum += *o.amount
			a.known++
		}
		gross[key(o)] = a
	}
	for _, r := range refunds {
		o := byID[r.order]
		period := o.date
		if activity {
			period = r.date
		}
		if !strings.HasPrefix(period, year) {
			continue
		}
		a := returns[key(o)]
		if r.amount != nil {
			a.sum += *r.amount
			a.known++
		}
		returns[key(o)] = a
	}
	out := map[string][2]string{}
	keys := map[string]bool{}
	for k := range gross {
		keys[k] = true
	}
	for k := range returns {
		keys[k] = true
	}
	for k := range keys {
		g, gok := gross[k]
		r, rok := returns[k]
		row := [2]string{"NULL", "NULL"}
		if gok && rok {
			row[1] = fmt.Sprint(g.known - r.known)
			if g.known > 0 && r.known > 0 {
				row[0] = fmt.Sprint(g.sum - r.sum)
			}
		}
		out[k] = row
	}
	return out
}

func TestSQLRecoveryGroupedOwnedPeriodsAcceptance(t *testing.T) {
	for _, calendar := range []bool{false, true} {
		for _, activity := range []bool{false, true} {
			for _, raw := range []bool{false, true} {
				name := map[bool]string{false: "region_", true: "month_"}[calendar] + map[bool]string{false: "shared_cohort", true: "fact_activity"}[activity] + map[bool]string{false: "_qualifying", true: "_raw"}[raw]
				t.Run(name, func(t *testing.T) {
					f := liveCommerceSource(t)
					if _, err := f.admin.Exec(t.Context(), `DELETE FROM analytics.order_items; DELETE FROM analytics.refunds; DELETE FROM analytics.orders; DELETE FROM analytics.customers;
ALTER TABLE analytics.customers ALTER COLUMN region DROP NOT NULL; ALTER TABLE analytics.customers DROP CONSTRAINT customers_region_check;
ALTER TABLE analytics.orders ALTER COLUMN total_usd DROP NOT NULL; ALTER TABLE analytics.refunds ALTER COLUMN amount_usd DROP NOT NULL;`); err != nil {
						t.Fatal(err)
					}
					text := func(s string) *string { return &s }
					amount := func(n int) *int { return &n }
					orders := []groupedOwnedOrder{
						{1, text("east"), "2026-01-01", amount(100), true}, {2, text("east"), "2025-01-01", amount(50), true},
						{3, nil, "2026-01-01", amount(30), true}, {4, text("unknown"), "2026-01-01", nil, true},
						{5, text("zero"), "2026-01-01", amount(0), true}, {6, text("missing"), "2026-01-01", amount(12), true},
						{7, text("cancelled-only"), "2026-01-01", amount(20), false}, {8, text("refund-only"), "2025-01-01", amount(60), true},
						{9, text("future-parent"), "2027-01-01", amount(10), true},
					}
					refunds := []groupedOwnedRefund{{1, "2027-01-01", amount(10)}, {2, "2026-01-01", amount(15)}, {3, "2026-01-01", amount(5)}, {4, "2026-01-01", nil}, {5, "2026-01-01", amount(0)}, {8, "2026-01-01", amount(7)}, {9, "2026-01-01", amount(2)}, {1, "2026-01-01", nil}}
					if calendar {
						orders = append(orders, groupedOwnedOrder{10, text("edge"), "2026-01-31", amount(13), true}, groupedOwnedOrder{11, text("edge"), "2026-02-01", amount(23), true}, groupedOwnedOrder{12, text("edge"), "2026-12-31", amount(9), true})
						refunds = append(refunds, groupedOwnedRefund{10, "2026-01-31", amount(3)}, groupedOwnedRefund{11, "2026-02-02", amount(4)}, groupedOwnedRefund{12, "2027-01-01", amount(1)})
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
					if activity {
						refundAxis = "refund_date"
					}
					periods := func(gross, refund string) *semantics.MetricPeriodBindings {
						bindings := []semantics.MetricPeriodBinding{{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: gross}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "order_month"}}, {Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: refund}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: refundAxis}}}
						sort.Slice(bindings, func(i, j int) bool { return bindings[i].Measure.ID < bindings[j].Measure.ID })
						return &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: bindings}
					}
					pack.KPIs[0].Periods = periods("gross_revenue", "refund_amount")
					pack.Measures = append(pack.Measures, semantics.Measure{ID: "gross_known", Name: "Known paid amount count", Field: pack.Measures[0].Field, Aggregation: semantics.AggregationCount, Filters: append([]semantics.SemanticFilter(nil), pack.Measures[0].Filters...)}, semantics.Measure{ID: "refund_known", Name: "Known refund amount count", Field: pack.Measures[1].Field, Aggregation: semantics.AggregationCount})
					pack.KPIs = append(pack.KPIs, semantics.KPI{ID: "known_difference", Name: "Known amount count difference", Expression: "gross_known - refund_known", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "gross_known"}, {Kind: semantics.KindMeasure, ID: "refund_known"}}, Periods: periods("gross_known", "refund_known")})
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
					rowDomain := " WHERE o.status='paid'"
					if raw {
						rowDomain = ""
					}
					sql := `WITH g AS (SELECT c.region AS region,SUM(o.total_usd) FILTER(WHERE o.status='paid') AS value,COUNT(o.total_usd) FILTER(WHERE o.status='paid') AS known FROM analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id` + rowDomain + ` GROUP BY c.region), r AS (SELECT c.region AS region,SUM(r.amount_usd) AS value,COUNT(r.amount_usd) AS known FROM analytics.refunds r JOIN analytics.orders o ON r.order_id=o.order_id JOIN analytics.customers c ON o.customer_id=c.customer_id GROUP BY c.region), keys AS (SELECT region FROM g UNION SELECT region FROM r) SELECT keys.region,g.value-r.value AS net,g.known-r.known AS known_difference FROM keys LEFT JOIN g ON keys.region IS NOT DISTINCT FROM g.region LEFT JOIN r ON keys.region IS NOT DISTINCT FROM r.region`
					groupQuestion := "Customer region"
					if calendar {
						sql = strings.ReplaceAll(sql, "c.region", "date_trunc('month',o.ordered_at::timestamp)")
						sql = strings.ReplaceAll(sql, "JOIN analytics.customers c ON o.customer_id=c.customer_id", "")
						groupQuestion = "month of Order date"
					}
					model.mode.Store(phase18RawResponse(t, sql))
					request := nlqexec.PlanRequest{Operation: "grouped-owned-plan", QuestionRequest: nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: "Net revenue and Known amount count difference in 2026 by " + groupQuestion, Locale: nlq.LanguageEnglish, MetricIDs: []string{"net_revenue", "known_difference"}, Kinds: []string{"kpi", "dimension"}, LimitPerKind: 10}}
					server := httptest.NewServer(nlqapi.ExecutionHandler(f.token.verifier, query, http.NotFoundHandler()))
					defer server.Close()
					claims := f.token.claims(actor.Tenant(), actor.User(), phase18Scopes(actor.Tenant(), true))
					claims["session"] = actor.Session()
					token := f.token.sign(t, claims, nil)
					client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
					if err != nil {
						t.Fatal(err)
					}
					plan, err := client.PlanNLQ(t.Context(), request)
					if err != nil {
						t.Fatal("owned grouped Plan", err)
					}
					if plan.Analytical == nil || plan.Analytical.Version != exec.AnalyticalGroupedOwnedPopulationsVersion || plan.Bindings == nil || plan.Bindings.SchemaVersion != 3 {
						t.Fatal("missing owned grouped receipt")
					}
					expectedScope := exec.AnalyticalGrainScope
					if calendar {
						expectedScope = exec.AnalyticalCalendarScope
					}
					if plan.Analytical.Scope != strings.ReplaceAll(expectedScope, "single_base_relation", "independent_owned_grouped_populations") {
						t.Fatal("wrong grouped period scope")
					}
					scope, _ := store.NewScope(actor.Tenant(), actor.User())
					stored, err := f.db.ReadQuery(t.Context(), scope, plan.QueryID)
					if err != nil {
						t.Fatal(err)
					}
					if stored.Clarification == nil || stored.Clarification.BaseSQL != sql || len(stored.Parameters) != 4 || len(stored.Clarification.BaseParameters) != 0 {
						t.Fatal("private base/parameter custody")
					}
					assertResult := func(result nlqexec.RunResult, year string) {
						t.Helper()
						if result.Execution.Result == nil {
							t.Fatal("missing result")
						}
						got := map[string][2]string{}
						for _, row := range result.Execution.Result.Rows {
							if len(row) != 3 {
								t.Fatal("output count")
							}
							key := "NULL"
							if string(row[0]) != "null" {
								if json.Unmarshal(row[0], &key) != nil {
									t.Fatal("region type")
								}
							}
							if calendar {
								if len(key) < 10 {
									t.Fatal("invalid civil calendar key", key)
								}
								key = key[:10]
							}
							values := [2]string{"NULL", "NULL"}
							for i := range values {
								if string(row[i+1]) != "null" && json.Unmarshal(row[i+1], &values[i]) != nil {
									values[i] = string(row[i+1])
								}
							}
							if _, ok := got[key]; ok {
								t.Fatal("duplicate group")
							}
							got[key] = values
						}
						want := groupedOwnedOracle(orders, refunds, year, activity, raw, calendar)
						if len(got) != len(want) {
							t.Fatalf("independent group oracle: got %v want %v", got, want)
						}
						for key, expected := range want {
							actual, ok := got[key]
							if !ok {
								t.Fatal("missing group", key)
							}
							for i := range expected {
								if actual[i] != expected[i] && (actual[i] == "NULL" || expected[i] == "NULL" || !liveNumberEquals(actual[i], expected[i])) {
									t.Fatalf("independent value oracle group %s output%d got %s want %s", key, i, actual[i], expected[i])
								}
							}
						}
					}
					runRequest := nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "grouped-owned-run"}
					result, err := query.Run(t.Context(), actor, runRequest)
					if err != nil {
						t.Fatal("owned grouped Run", err)
					}
					assertResult(result, "2026")
					learned := assertScopedLearningBase(t, query, actor, pack.Topic, plan.QueryID, sql, nlqexec.ScopedGroupedExamplePolicy)
					active := activateScopedLearningBase(t, query, actor, learned)
					fresh := request
					fresh.Operation = "current-scoped-population"
					fresh.Question = strings.Replace(fresh.Question, "2026", "2025", 1)
					freshPlan, freshErr := query.Plan(t.Context(), actor, fresh)
					if freshErr != nil {
						t.Fatal("new-period learned plan", freshErr)
					}
					freshStored, freshErr := f.db.ReadQuery(t.Context(), scope, freshPlan.QueryID)
					if freshErr != nil {
						t.Fatal(freshErr)
					}
					assertScopedExampleUsage(t, freshStored, active, 3)
					freshResult, freshErr := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: freshPlan.QueryID, Operation: "current-scoped-population-run"})
					if freshErr != nil {
						t.Fatal("new-period learned run", freshErr)
					}
					assertResult(freshResult, "2025")
					metadata := support.Raw(t, f.dsn)
					calls, attempts := model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
					restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
					if err != nil {
						t.Fatal(err)
					}
					replay, err := restarted.Run(t.Context(), actor, runRequest)
					if err != nil {
						t.Fatal("owned replay", err)
					}
					assertResult(replay, "2026")
					repeated, err := restarted.Plan(t.Context(), actor, request)
					if err != nil || repeated.QueryID != plan.QueryID {
						t.Fatal("owned original Plan replay", err)
					}
					if calls != model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
						t.Fatal("terminal replay repeated work")
					}
					for _, fault := range []string{"ordinal", "schema", "policy", "duplicate_lane", "constraint_digest", "source_digest", "validation_source", "validation_context", "indexes"} {
						faulted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, groupedOwnedOrdinalFault{Repository: f.db, fault: fault})
						if err != nil {
							t.Fatal(err)
						}
						if _, err = faulted.Run(t.Context(), actor, runRequest); !errors.Is(err, exec.ErrBinding) {
							t.Fatal("terminal replay did not reject corrupted owned evidence", fault, err)
						}
						if calls != model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
							t.Fatal("corrupt replay made execution/model call", fault)
						}
					}

					publicPlan, err := client.PlanNLQ(t.Context(), sdk.NLQPlanRequest{Operation: request.Operation, QuestionRequest: request.QuestionRequest})
					if err != nil || publicPlan.QueryID != plan.QueryID {
						t.Fatal("schema3 HTTP/SDK Plan replay", err)
					}
					publicRun, err := client.RunNLQ(t.Context(), runRequest)
					if err != nil || publicRun.Bindings == nil || publicRun.Bindings.SchemaVersion != 3 || publicRun.Analytical == nil || publicRun.Analytical.Version != exec.AnalyticalGroupedOwnedPopulationsVersion {
						t.Fatal("schema3 HTTP/SDK Run replay", err)
					}
					assertResult(publicRun, "2026")
					if calls != model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
						t.Fatal("HTTP/SDK replay repeated execution/model work")
					}
					next := request.QuestionRequest
					next.Question = strings.Replace(next.Question, "2026", "2025", 1)
					refined, err := query.Refine(t.Context(), actor, nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: next})
					if err != nil {
						t.Fatal("owned period refinement", err)
					}
					changed, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: refined.QueryID, Operation: "refined-owned-run"})
					if err != nil {
						t.Fatal("refined Run", err)
					}
					assertResult(changed, "2025")
					for _, mutation := range []string{`analytical_version=9`, `analytical=analytical-'grouping'`, `clarification=jsonb_set(clarification,'{binding,schema_version}','2')`} {
						if _, err := metadata.Exec(t.Context(), `UPDATE chartworks.nlq_queries SET `+mutation+` WHERE query_id=$1`, plan.QueryID); err == nil {
							t.Fatal("immutable owned receipt changed")
						}
					}
				})
			}
		}
	}
}
