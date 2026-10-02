package exec

import (
	"context"
	"testing"
)

func TestSQLRecoveryReviewedNullPolicy(t *testing.T) {
	sum := analyticalMeasure("sum", "amount")
	zero := AnalyticalExpression{Op: "number", Value: "0"}
	reviewed := AnalyticalExpression{Op: "coalesce", Args: []AnalyticalExpression{sum, zero}}
	for _, tc := range []struct {
		sql  string
		expr AnalyticalExpression
		pass bool
	}{
		{"SELECT coalesce(sum(amount),0) FROM analytics.sales", reviewed, true},
		{"SELECT sum(amount) FROM analytics.sales", reviewed, false},
		{"SELECT coalesce(sum(amount),0) FROM analytics.sales", sum, false},
		{"SELECT coalesce(sum(amount),1) FROM analytics.sales", reviewed, false},
		{"SELECT coalesce(0,sum(amount)) FROM analytics.sales", reviewed, false},
		{"SELECT sum(coalesce(amount,0)) FROM analytics.sales", reviewed, false},
	} {
		p, c := analyticalFixture(t, tc.sql, analyticalMetrics(tc.expr))
		c.Version = AnalyticalGroupedPopulationsVersion
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
		c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); (err == nil) != tc.pass {
			t.Fatal(tc.sql, err)
		}
	}
	p, c := analyticalFixture(t, "SELECT coalesce(sum(amount),0) FROM analytics.sales", analyticalMetrics(reviewed))
	c.Version = AnalyticalIntentVersion
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("v6 null policy widened")
	}
}
func TestSQLRecoveryExplicitAbsenceOfFiltersAndLimits(t *testing.T) {
	for _, suffix := range []string{"", " WHERE id>0", " HAVING sum(amount)>0", " LIMIT 1", " LIMIT 0", " OFFSET 1"} {
		p, c := analyticalFixture(t, "SELECT sum(amount) FROM analytics.sales"+suffix, analyticalMetrics(analyticalMeasure("sum", "amount")))
		c.Version = AnalyticalGroupedPopulationsVersion
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
		c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
		c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
		_, err := CheckAnalyticalPlan(context.Background(), p, c)
		if (err == nil) != (suffix == "") {
			t.Fatal(suffix, err)
		}
	}
}

func TestSQLRecoveryV7CannotDropIntent(t *testing.T) {
	p, c := analyticalFixture(t, "SELECT sum(amount) FROM analytics.sales", analyticalMetrics(analyticalMeasure("sum", "amount")))
	c.Version = AnalyticalGroupedPopulationsVersion
	for _, which := range []string{"both", "intent", "population"} {
		c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
		c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
		if which != "population" {
			c.Intent = nil
		}
		if which != "intent" {
			c.QueryPopulation = nil
		}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("v7 proof without mandatory intent", which)
		}
	}
}
