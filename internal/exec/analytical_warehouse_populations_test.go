//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"strings"
	"testing"
)

func TestSQLRecoveryWarehouseIndependentPopulations(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			for _, sql := range []string{
				`WITH s AS (SELECT sum(amount) AS total FROM analytics.sales), i AS (SELECT sum(quantity) AS total FROM analytics.items) SELECT s.total-i.total AS net FROM s CROSS JOIN i`,
				`SELECT s.total-i.total AS net FROM (SELECT sum(amount) AS total FROM analytics.sales) s, (SELECT sum(quantity) AS total FROM analytics.items) i`,
			} {
				p, c := independentFixture(t, sql)
				p.candidate.binding.Dialect = dialect
				c.Binding = Hash(p.candidate.binding)
				if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
					t.Fatal(err)
				}
				for _, bad := range []string{strings.Replace(sql, "sum(quantity)", "avg(quantity)", 1), strings.Replace(sql, "FROM analytics.items)", "FROM analytics.items GROUP BY sale_id)", 1), strings.Replace(sql, "s.total-i.total", "i.total-s.total", 1), strings.Replace(sql, "FROM analytics.items)", "FROM analytics.items WHERE quantity>0)", 1)} {
					p.candidate.statement = bad
					if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
						t.Fatal("unproved independent population", bad)
					}
				}
			}
		})
	}
}
