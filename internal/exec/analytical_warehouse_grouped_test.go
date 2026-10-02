//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"strings"
	"testing"
)

func TestSQLRecoveryWarehouseGroupedSpineStructure(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			sql := `WITH s AS (SELECT id AS k,sum(amount) AS v FROM analytics.sales GROUP BY id), i AS (SELECT id AS k,sum(amount) AS v FROM analytics.sales GROUP BY id) SELECT keys.k FROM (SELECT k FROM s UNION SELECT k FROM i) keys`
			p, _ := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
			p.candidate.binding.Dialect = dialect
			root, err := warehouseAnalyticalAST(context.Background(), sql, p.candidate.binding)
			if err != nil {
				t.Fatal(err)
			}
			union := object(object(object(array(root["fromClause"])[0])["RangeSubselect"])["subquery"])
			u := object(union["SelectStmt"])
			if text(u["op"]) != "SETOP_UNION" || truth(u["all"]) || object(u["larg"]) == nil || object(u["rarg"]) == nil {
				t.Fatal("set identity lost")
			}
			all, err := warehouseAnalyticalAST(context.Background(), strings.Replace(sql, " UNION ", " UNION ALL ", 1), p.candidate.binding)
			if err != nil {
				t.Fatal(err)
			}
			au := object(object(object(object(array(all["fromClause"])[0])["RangeSubselect"])["subquery"])["SelectStmt"])
			if !truth(au["all"]) {
				t.Fatal("duplicate spine deduplicated by normalizer")
			}
			if _, err := warehouseAnalyticalAST(context.Background(), strings.Replace(sql, " UNION ", " INTERSECT ", 1), p.candidate.binding); err == nil {
				t.Fatal("unproved set operator normalized")
			}
			// The parser may attach trailing ORDER BY to the right SELECT. Keep
			// that clause visible for the grouped proof to reject; never strip it.
			ordered, err := warehouseAnalyticalAST(context.Background(), strings.Replace(sql, "SELECT k FROM i) keys", "SELECT k FROM i ORDER BY k) keys", 1), p.candidate.binding)
			if err == nil {
				u := object(object(object(object(array(ordered["fromClause"])[0])["RangeSubselect"])["subquery"])["SelectStmt"])
				if len(array(object(u["rarg"])["sortClause"])) == 0 {
					t.Fatal("spine ordering disappeared")
				}
			}

		})
	}
}

func TestSQLRecoverySnowflakeNullableRanking(t *testing.T) {
	for _, tc := range []struct {
		clause string
		pass   bool
	}{{"DESC NULLS LAST", true}, {"DESC", false}, {"DESC NULLS FIRST", false}, {"ASC NULLS LAST", false}} {
		p, c := analyticalFixture(t, "SELECT name,sum(amount) AS revenue FROM analytics.sales GROUP BY name ORDER BY revenue "+tc.clause+" LIMIT 2", analyticalMetrics(analyticalMeasure("sum", "amount")))
		p.candidate.binding.Dialect = "snowflake"
		c.Binding = Hash(p.candidate.binding)
		c.Version = AnalyticalIntentVersion
		c.Grain = &AnalyticalGrain{Policy: AnalyticalCalendarPolicy, Columns: []string{"name"}, Dimensions: []string{"category"}}
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Order: []AnalyticalOrder{{Metric: c.Metrics[0].ID, Descending: true, Nulls: "last"}}, Limit: 2}
		c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); (err == nil) != tc.pass {
			t.Fatal(tc.clause, err)
		}
	}
}
