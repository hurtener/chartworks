package exec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func grainPlan(t *testing.T, sql string) (Plan, AnalyticalContract) {
	t.Helper()
	p, c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
	c.Version = AnalyticalGrainVersion
	c.Grain = &AnalyticalGrain{Policy: AnalyticalGrainPolicy, Columns: []string{"active"}, Dimensions: []string{"topic:dimension:active"}}
	return p, c
}
func TestSQLRecoveryAnalyticalGrainPositive(t *testing.T) {
	for _, sql := range []string{
		`SELECT active, sum(amount) AS revenue FROM analytics.sales GROUP BY active`,
		`SELECT s.active AS category, sum(s.amount) AS revenue FROM analytics.sales AS s GROUP BY 1 ORDER BY category`,
		`SELECT active AS category, sum(amount) AS revenue FROM analytics.sales GROUP BY category`,
		`SELECT sum(amount) AS revenue,active AS category FROM analytics.sales GROUP BY 2`,
		`SELECT active,sum(amount) AS revenue FROM analytics.sales WHERE name='A' GROUP BY active,active`,
	} {
		t.Run(sql, func(t *testing.T) {
			p, c := grainPlan(t, sql)
			before, _ := json.Marshal(c)
			proof, err := CheckAnalyticalPlan(context.Background(), p, c)
			if err != nil || proof == nil || proof.Scope != AnalyticalGrainScope || proof.Version != AnalyticalGrainVersion || !reflect.DeepEqual(proof.Grouping, c.Grain.Dimensions) {
				t.Fatal("grouping proof", err)
			}
			proof.Grouping[0] = "mutated"
			after, _ := json.Marshal(c)
			if string(before) != string(after) {
				t.Fatal("proof aliases contract or checker mutated it")
			}
		})
	}
}
func TestSQLRecoveryAnalyticalGrainRejectsChangedPartition(t *testing.T) {
	for _, sql := range []string{
		`SELECT sum(amount) AS revenue FROM analytics.sales`,
		`SELECT name,sum(amount) AS revenue FROM analytics.sales GROUP BY name`,
		`SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active,id`,
		`SELECT sum(amount) AS revenue FROM analytics.sales GROUP BY active`,
		`SELECT active,id,sum(amount) AS revenue FROM analytics.sales GROUP BY active,id`,
		`SELECT active AS name,sum(amount) AS revenue FROM analytics.sales GROUP BY name`,
	} {
		t.Run(sql, func(t *testing.T) {
			p, c := grainPlan(t, sql)
			if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof != nil || !errors.Is(err, ErrAnalyticalMismatch) {
				t.Fatal("changed grain accepted", err)
			}
		})
	}
}
func TestSQLRecoveryAnalyticalGrainPreservesMetricChecks(t *testing.T) {
	for _, sql := range []string{
		`SELECT active,avg(amount) AS revenue FROM analytics.sales GROUP BY active`,
		`SELECT active,sum(amount) FILTER(WHERE name='B') AS revenue FROM analytics.sales GROUP BY active`,
		`SELECT active,sum(NULLIF(amount,0)) AS revenue FROM analytics.sales GROUP BY active`,
		`SELECT NULLIF(id,0),sum(amount) AS revenue FROM analytics.sales GROUP BY NULLIF(id,0)`,
		`SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY ROLLUP(active)`,
	} {
		t.Run(sql, func(t *testing.T) {
			p, c := grainPlan(t, sql)
			if proof, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil || proof != nil {
				t.Fatal("unsupported/incorrect metric passed with grouping")
			}
		})
	}
}
func TestSQLRecoveryAnalyticalGrainContractAndLegacy(t *testing.T) {
	for _, change := range []func(*AnalyticalContract){
		func(c *AnalyticalContract) { c.Version = AnalyticalVersion },
		func(c *AnalyticalContract) { c.Grain.Policy = "unknown" },
		func(c *AnalyticalContract) { c.Grain.Columns = nil },
		func(c *AnalyticalContract) { c.Grain.Columns = []string{"foreign"} },
		func(c *AnalyticalContract) { c.Grain.Columns = []string{"active", "active"} },
		func(c *AnalyticalContract) { c.Grain.Dimensions = nil },
		func(c *AnalyticalContract) { c.Grain.Dimensions = []string{"z", "a"} },
	} {
		p, c := grainPlan(t, `SELECT active,sum(amount) FROM analytics.sales GROUP BY active`)
		change(&c)
		if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof != nil || !errors.Is(err, ErrBinding) {
			t.Fatal("invalid grain contract", err)
		}
	}
	for _, version := range []string{AnalyticalVersion, AnalyticalGrainVersion} {
		p, c := grainPlan(t, `SELECT sum(amount) FROM analytics.sales`)
		c.Version, c.Grain = version, nil
		proof, err := CheckAnalyticalPlan(context.Background(), p, c)
		if err != nil || proof.Scope != AnalyticalMetricScope || len(proof.Grouping) != 0 || proof.Version != version {
			t.Fatal("unknown grain acquired stronger proof", err)
		}
	}
}
func TestSQLRecoveryAnalyticalGrainCannotAuthorizeOrAdmitJoins(t *testing.T) {
	for _, sql := range []string{
		`SELECT s.active,sum(s.amount) FROM analytics.sales s JOIN analytics.sales t ON s.id=t.id GROUP BY s.active`,
		`WITH q AS (SELECT active,amount FROM analytics.sales) SELECT active,sum(amount) FROM q GROUP BY active`,
	} {
		p, c := grainPlan(t, sql)
		if proof, err := CheckAnalyticalPlan(context.Background(), p, c); proof != nil || !errors.Is(err, ErrAnalyticalUnsupported) {
			t.Fatal("grain bypassed relation proof", err)
		}
	}
	p, c := grainPlan(t, `SELECT active,sum(amount) FROM analytics.sales GROUP BY active`)
	p.nativeChecked = false
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrBinding) {
		t.Fatal("grain issued native authority", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.nativeChecked = true
	if _, err := CheckAnalyticalPlan(ctx, p, c); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
}
