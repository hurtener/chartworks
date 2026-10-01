package acceptance

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

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
)

func TestSQLRecoveryGroupedCalendarCommerceAcceptance(t *testing.T) {
	for _, instant := range []bool{false, true} {
		name := "civil_month"
		if instant {
			name = "instant_dst_day"
		}
		t.Run(name, func(t *testing.T) {
			f := liveCommerceSource(t)
			if _, err := f.admin.Exec(t.Context(), `ALTER TABLE analytics.orders ALTER COLUMN ordered_at DROP NOT NULL`); err != nil {
				t.Fatal(err)
			}
			if instant {
				if _, err := f.admin.Exec(t.Context(), `DELETE FROM analytics.order_items; DELETE FROM analytics.refunds; DELETE FROM analytics.orders;
ALTER TABLE analytics.orders ALTER COLUMN ordered_at TYPE timestamptz USING ordered_at::timestamp AT TIME ZONE 'UTC';
INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES
(201,1,'2026-03-08T06:30:00Z',100,'paid'),(202,1,'2026-03-08T07:30:00Z',50,'paid'),
(203,2,'2026-11-01T05:30:00Z',80,'paid'),(204,2,'2026-11-01T06:30:00Z',20,'paid'),
(205,3,NULL,40,'paid'),(206,3,'2026-03-09T05:00:00Z',30,'paid'),(207,3,'2026-03-10T05:00:00Z',80,'cancelled');
INSERT INTO analytics.refunds VALUES(501,201,'2026-03-09',20),(502,203,'2026-11-02',5),(503,204,'2026-11-02',10),(504,205,'2026-03-11',5),(505,207,'2026-03-11',7)`); err != nil {
					t.Fatal(err)
				}
			} else if _, err := f.admin.Exec(t.Context(), `INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES(107,1,NULL,40,'paid'),(108,1,'2027-01-10',70,'paid'); INSERT INTO analytics.refunds VALUES(205,107,'2026-03-01',5),(206,108,'2027-01-20',10)`); err != nil {
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
			for i := range pack.Dimensions {
				if pack.Dimensions[i].ID == "order_month" && instant {
					pack.Dimensions[i].Temporal = &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "America/New_York", Grains: []semantics.TimeGrain{semantics.GrainDay}}
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
			expression, unit := "date_trunc('month',o.ordered_at::timestamp)", "month"
			if instant {
				expression, unit = "date_trunc('day',o.ordered_at,'America/New_York')", "day"
			}
			statement := `WITH gross AS (SELECT ` + expression + ` AS period,sum(o.total_usd) AS value FROM analytics.orders o WHERE o.status='paid' GROUP BY 1), refunds AS (SELECT ` + expression + ` AS period,sum(r.amount_usd) AS value FROM analytics.refunds r JOIN analytics.orders o ON r.order_id=o.order_id GROUP BY 1), keys AS (SELECT period FROM gross UNION SELECT period FROM refunds) SELECT keys.period,gross.value-refunds.value AS net FROM keys LEFT JOIN gross ON keys.period IS NOT DISTINCT FROM gross.period LEFT JOIN refunds ON keys.period IS NOT DISTINCT FROM refunds.period`
			request := nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: "Net revenue by " + unit + " of Order date", Locale: nlq.LanguageEnglish, MetricIDs: []string{"net_revenue"}, Kinds: []string{"kpi", "dimension"}, LimitPerKind: 5}
			expected := map[string]string{"2026-01-01": "225.00", "2026-02-01": "210.00", "2026-03-01": "NULL", "NULL": "35.00", "2027-01-01": "60.00"}
			if instant {
				expected = map[string]string{"2026-03-08T05:00:00Z": "130.00", "2026-11-01T04:00:00Z": "85.00", "2026-03-09T04:00:00Z": "NULL", "2026-03-10T04:00:00Z": "NULL", "NULL": "35.00"}
			}
			for _, sql := range []string{statement, "SELECT q.period,q.net FROM (" + statement + ") q"} {
				model.mode.Store(phase18RawResponse(t, sql))
				p, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request})
				if err != nil || p.Analytical == nil || p.Analytical.Version != exec.AnalyticalGroupedProgramsVersion || p.Analytical.Scope != "selected_metric_expression_population_and_calendar_grouping;independent_grouped_populations" {
					t.Fatal("calendar grouped Plan", err)
				}
				result, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
				if err != nil || result.Execution.Result == nil {
					t.Fatal("calendar grouped Run", err)
				}
				actual := map[string]string{}
				for _, row := range result.Execution.Result.Rows {
					key, value := "NULL", "NULL"
					if string(row[0]) != "null" {
						var raw string
						if json.Unmarshal(row[0], &raw) != nil {
							t.Fatal("calendar key type")
						}
						if instant {
							at, err := calendarResultInstant(raw)
							if err != nil {
								t.Fatal("calendar instant", err)
							}
							key = at.UTC().Format(time.RFC3339)
						} else {
							civil, err := time.Parse("2006-01-02 15:04:05.999999999", strings.Replace(raw, "T", " ", 1))
							if err != nil || civil.Hour() != 0 || civil.Minute() != 0 || civil.Second() != 0 {
								t.Fatal("calendar civil encoding", raw, err)
							}
							key = civil.Format("2006-01-02")
						}
					}
					if string(row[1]) != "null" && json.Unmarshal(row[1], &value) != nil {
						t.Fatal("calendar amount type")
					}
					if _, duplicate := actual[key]; duplicate {
						t.Fatal("duplicate calendar group")
					}
					actual[key] = value
				}
				if exec.Hash(actual) != exec.Hash(expected) {
					t.Fatal("incorrect independent calendar amounts", actual)
				}
				restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
				if err != nil {
					t.Fatal(err)
				}
				calls := model.requests.Load()
				replay, err := restarted.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
				if err != nil || replay.Execution.Result == nil || exec.Hash(replay.Execution.Result.Rows) != exec.Hash(result.Execution.Result.Rows) || calls != model.requests.Load() {
					t.Fatal("calendar grouped restart replay", err)
				}
			}
			bad := strings.Replace(statement, "r.order_id=o.order_id", "r.order_id=o.order_id AND r.amount_usd>0", 1)
			model.mode.Store(phase18RawResponse(t, bad))
			if _, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); err == nil {
				t.Fatal("calendar lane altered population")
			}
			if instant {
				bad = strings.Replace(statement, "America/New_York", "UTC", 1)
			} else {
				bad = strings.Replace(statement, "refunds AS (SELECT "+expression, "refunds AS (SELECT date_trunc('month',r.refunded_at::timestamp)", 1)
			}
			model.mode.Store(phase18RawResponse(t, bad))
			if _, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); err == nil {
				t.Fatal("calendar origin or timezone substitution")
			}
		})
	}
}
