//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"errors"
	"testing"
)

func TestSQLRecoveryWarehouseAnalyticalStructure(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			p, c := analyticalFixture(t, "SELECT name AS category,sum(amount) AS revenue FROM analytics.sales AS s GROUP BY name", analyticalMetrics(analyticalMeasure("sum", "amount")))
			p.candidate.binding.Dialect = dialect
			c.Binding = Hash(p.candidate.binding)
			c.Version = AnalyticalIntentVersion
			if _, err := warehouseAnalyticalAST(context.Background(), p.candidate.statement, p.candidate.binding); err != nil {
				t.Fatal(err)
			}
			for _, sql := range []string{"SELECT DISTINCT sum(amount) FROM analytics.sales", "SELECT sum(amount) FROM analytics.sales WHERE id IN (SELECT id FROM analytics.sales)", "SELECT sum(amount) OVER () FROM analytics.sales"} {
				if _, err := warehouseAnalyticalAST(context.Background(), sql, p.candidate.binding); err == nil {
					t.Fatal("unproved modifier admitted", sql)
				}
			}
		})
	}
}

func TestSQLRecoveryWarehouseAnalyticalMetrics(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			for _, tc := range []struct {
				sql  string
				pass bool
			}{
				{"SELECT sum(amount) FROM analytics.sales", true},
				{"SELECT avg(amount) FROM analytics.sales", false},
				{"SELECT sum(DISTINCT amount) FROM analytics.sales", false},
				{"SELECT sum(amount)*0 FROM analytics.sales", false},
			} {
				p, c := analyticalFixture(t, tc.sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
				p.candidate.binding.Dialect = dialect
				c.Binding = Hash(p.candidate.binding)
				c.Version = AnalyticalIntentVersion
				_, err := CheckAnalyticalPlan(context.Background(), p, c)
				if (err == nil) != tc.pass {
					t.Fatalf("%s pass=%v: %v", tc.sql, tc.pass, err)
				}
			}
		})
	}
}

func TestSQLRecoveryWarehouseAnalyticalPolicyIsolation(t *testing.T) {
	p, c := analyticalFixture(t, "SELECT sum(amount) FROM analytics.sales", analyticalMetrics(analyticalMeasure("sum", "amount")))
	p.candidate.binding.Dialect = "mysql"
	c.Binding = Hash(p.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalUnsupported) {
		t.Fatal("old dialect policy widened", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := warehouseAnalyticalAST(ctx, p.candidate.statement, p.candidate.binding); !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation", err)
	}
}

func TestSQLRecoveryWarehouseAnalyticalPopulationsAndRatios(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			filter := AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"A"}}
			for _, tc := range []struct {
				sql        string
				expression AnalyticalExpression
			}{
				{"SELECT sum(CASE WHEN name='A' THEN amount END) FROM analytics.sales", analyticalMeasure("sum", "amount", filter)},
				{"SELECT CASE WHEN count(id)=0 THEN NULL ELSE sum(amount)/count(id) END FROM analytics.sales", AnalyticalExpression{Op: "/", Args: []AnalyticalExpression{analyticalMeasure("sum", "amount"), analyticalMeasure("count", "id")}}},
			} {
				p, c := analyticalFixture(t, tc.sql, analyticalMetrics(tc.expression))
				p.candidate.binding.Dialect = dialect
				c.Binding = Hash(p.candidate.binding)
				c.Version = AnalyticalIntentVersion
				if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
					t.Fatal(tc.sql, err)
				}
			}
			p, c := analyticalFixture(t, "SELECT sum(amount) FROM analytics.sales", analyticalMetrics(analyticalMeasure("sum", "amount")))
			p.candidate.binding.Dialect = dialect
			c.Binding = Hash(p.candidate.binding)
			c.Version = AnalyticalIntentVersion
			if dialect == "snowflake" {
				p.candidate.statement = `SELECT sum("amount") FROM "analytics"."sales"`
			}
			constraint := BusinessConstraint{Resolution: Hash("entity"), Dataset: "sales", Column: "name", SourceRevision: 1, Kind: "entity", Operator: "eq", Nulls: "exclude", Value: "private-synthetic-value"}
			bound, err := BindBusinessConstraints(context.Background(), p.candidate.binding, p.candidate.statement, nil, []BusinessConstraint{constraint})
			if err != nil {
				t.Fatal(err)
			}
			c.QueryPopulation, err = NewAnalyticalQueryPopulation(context.Background(), p.candidate.binding, "sales", []BusinessConstraint{constraint})
			if err != nil {
				t.Fatal(err)
			}
			p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
				t.Fatal("owned population", err)
			}
			p.candidate.parameters[0].Value = "changed"
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalMismatch) {
				t.Fatal("changed private parameter", err)
			}
		})
	}
}

func TestSQLRecoveryWarehouseAnalyticalOrderLimit(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		tail := "ORDER BY sum(amount) DESC LIMIT 3"
		prefix := "SELECT "
		if dialect == "sqlserver" {
			prefix = "SELECT TOP (3) "
			tail = "ORDER BY sum(amount) DESC"
		}
		if dialect == "snowflake" {
			tail = "ORDER BY sum(amount) DESC NULLS LAST LIMIT 3"
		}
		p, c := analyticalFixture(t, prefix+"name,sum(amount) FROM analytics.sales GROUP BY name "+tail, analyticalMetrics(analyticalMeasure("sum", "amount")))
		p.candidate.binding.Dialect = dialect
		c.Binding = Hash(p.candidate.binding)
		c.Version = AnalyticalIntentVersion
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Order: []AnalyticalOrder{{Metric: "metric-0", Descending: true, Nulls: "last"}}, Limit: 3}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal(dialect, err)
		}
		c.Intent.Limit = 2
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalMismatch) {
			t.Fatal("wrong limit", dialect, err)
		}
	}
}

func TestSQLRecoveryWarehouseAnalyticalAverageTypes(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		p, c := analyticalFixture(t, "SELECT avg(id) FROM analytics.sales", analyticalMetrics(analyticalMeasure("avg", "id")))
		p.candidate.binding.Dialect = dialect
		c.Binding = Hash(p.candidate.binding)
		c.Version = AnalyticalIntentVersion
		_, err := CheckAnalyticalPlan(context.Background(), p, c)
		want := dialect == "mysql" || dialect == "snowflake"
		if (err == nil) != want {
			t.Fatal("integer AVG exactness", dialect, err)
		}
	}
}
