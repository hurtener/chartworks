package exec

import (
	"context"
	"strings"
	"testing"
)

func independentFixture(t *testing.T, sql string) (Plan, AnalyticalContract) {
	p, c := analyticalFixture(t, sql, analyticalMetrics(AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{analyticalMeasure("sum", "amount"), analyticalMeasure("sum", "items/quantity")}}))
	for i := range p.candidate.binding.Relations[1].Columns {
		p.candidate.binding.Relations[1].Columns[i].Category = "numeric"
	}
	c.Version = AnalyticalIntentVersion
	c.Populations = []string{"items", "sales"}
	c.Binding = Hash(p.candidate.binding)
	return p, c
}

func TestSQLRecoveryIndependentScalarPopulations(t *testing.T) {
	for _, sql := range []string{
		`WITH s AS (SELECT sum(amount) AS total FROM analytics.sales), i AS (SELECT sum(quantity) AS total FROM analytics.items) SELECT s.total-i.total AS net FROM s CROSS JOIN i`,
		`SELECT s.total-i.total AS net FROM (SELECT sum(amount) AS total FROM analytics.sales) s, (SELECT sum(quantity) AS total FROM analytics.items) i`,
	} {
		p, c := independentFixture(t, sql)
		receipt, err := CheckAnalyticalPlan(context.Background(), p, c)
		if err != nil {
			t.Fatal(sql, err)
		}
		if !strings.Contains(receipt.Scope, "independent_singleton_populations") {
			t.Fatal("missing scope")
		}
	}
}

func TestSQLRecoveryIndependentPopulationAdversaries(t *testing.T) {
	base := `WITH s AS (SELECT sum(amount) AS total FROM analytics.sales), i AS (SELECT sum(quantity) AS total FROM analytics.items) SELECT s.total-i.total AS net FROM s CROSS JOIN i`
	for _, sql := range []string{
		strings.Replace(base, "sum(quantity)", "avg(quantity)", 1),
		strings.Replace(base, "FROM analytics.items)", "FROM analytics.items WHERE quantity>0)", 1),
		strings.Replace(base, "FROM analytics.items)", "FROM analytics.items GROUP BY sale_id)", 1),
		strings.Replace(base, "FROM analytics.items)", "FROM analytics.items HAVING sum(quantity)>0)", 1),
		strings.Replace(base, "FROM analytics.items)", "FROM analytics.items LIMIT 0)", 1),
		strings.Replace(base, "CROSS JOIN i", "JOIN i ON s.total=i.total", 1),
		strings.Replace(base, "s.total-i.total", "s.total+i.total", 1),
		strings.Replace(base, "FROM analytics.items)", "FROM analytics.items i JOIN analytics.sales s ON i.sale_id=s.id)", 1),
		base + " WHERE s.total>0",
	} {
		p, c := independentFixture(t, sql)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("unsafe population accepted", sql)
		}
	}
}

func TestSQLRecoveryIndependentCountsRemainSourceBound(t *testing.T) {
	sql := `WITH s AS (SELECT count(*) AS total FROM analytics.sales), i AS (SELECT count(*) AS total FROM analytics.items) SELECT s.total-i.total AS net FROM s CROSS JOIN i`
	p, c := independentFixture(t, sql)
	c.Metrics = analyticalMetrics(AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{analyticalMeasure("count", "id"), analyticalMeasure("count", "items/sale_id")}})
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	p.candidate.statement = strings.Replace(sql, "s.total-i.total", "i.total-s.total", 1)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("counts from different sources collapsed")
	}
}
