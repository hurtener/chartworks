package exec

import (
	"context"
	"errors"
	"testing"
)

func TestSQLRecoveryExplicitTotalIsNotUnknownGrain(t *testing.T) {
	for _, sql := range []string{`SELECT sum(amount) FROM analytics.sales`, `SELECT active,sum(amount) FROM analytics.sales GROUP BY active`, `SELECT sum(amount) FROM analytics.sales GROUP BY active`} {
		p, c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
		c.Version = AnalyticalGroupingVersion
		c.Grain = &AnalyticalGrain{Policy: AnalyticalGroupingPolicy, Columns: []string{}, Dimensions: []string{}}
		r, err := CheckAnalyticalPlan(context.Background(), p, c)
		if sql == `SELECT sum(amount) FROM analytics.sales` {
			if err != nil || r.Scope != AnalyticalTotalScope {
				t.Fatal("explicit total", err)
			}
		} else if r != nil || !errors.Is(err, ErrAnalyticalMismatch) {
			t.Fatal("hidden/visible grouping escaped total", err)
		}
		c.Grain = nil
		if r, err = CheckAnalyticalPlan(context.Background(), p, c); err != nil || r.Scope != AnalyticalMetricScope {
			t.Fatal("unknown scope changed", err)
		}
	}
	p, c := analyticalFixture(t, `SELECT sum(amount) FROM analytics.sales`, analyticalMetrics(analyticalMeasure("sum", "amount")))
	c.Grain = &AnalyticalGrain{Policy: AnalyticalGroupingPolicy}
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrBinding) {
		t.Fatal("v1 upgraded empty grouping", err)
	}
	c.Version = AnalyticalGroupingVersion
	c.Grain.Dimensions = []string{"fake"}
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrBinding) {
		t.Fatal("fake dimension allowed", err)
	}
}

func TestSQLRecoveryGroupingEditsKeepBoundParameterRoles(t *testing.T) {
	old := `SELECT id,sum(amount) FROM analytics.sales WHERE amount > $1 GROUP BY id ORDER BY id`
	for _, sql := range []string{`SELECT active,sum(amount) FROM analytics.sales WHERE amount > $1 GROUP BY active ORDER BY active`, `SELECT sum(amount) FROM analytics.sales WHERE amount > $1`} {
		if err := CheckParameterContinuity(context.Background(), "postgres", old, sql, 1); err != nil {
			t.Fatal("safe grouping edit blocked", err)
		}
	}
	for _, sql := range []string{`SELECT active,sum(amount) FROM analytics.sales GROUP BY active HAVING sum(amount)>$1`, `SELECT active,sum(amount) FROM analytics.sales WHERE id > $1 GROUP BY active`, `SELECT active,sum(amount) FROM analytics.sales WHERE NOT(amount > $1) GROUP BY active`, `SELECT active,sum(amount) FROM analytics.sales s WHERE amount > $1 GROUP BY active`} {
		if err := CheckParameterContinuity(context.Background(), "postgres", old, sql, 1); err == nil {
			t.Fatal("grouping edit changed protected roles")
		}
	}
}
