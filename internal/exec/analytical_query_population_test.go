package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func queryPopulationFixture(t *testing.T, sql string, constraints []BusinessConstraint, metrics []AnalyticalMetric) (Plan, AnalyticalContract) {
	t.Helper()
	p, c := analyticalFixture(t, sql, metrics)
	bound, err := BindBusinessConstraints(context.Background(), p.candidate.binding, sql, nil, constraints)
	if err != nil {
		t.Fatal(err)
	}
	population, err := NewAnalyticalQueryPopulation(context.Background(), p.candidate.binding, c.Dataset, constraints)
	if err != nil {
		t.Fatal(err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	c.Version, c.QueryPopulation = AnalyticalQueryPopulationVersion, population
	return p, c
}

func TestSQLRecoveryQueryPopulationRejectsAdditionalRestrictions(t *testing.T) {
	for _, suffix := range []string{"", " WHERE active=true", " WHERE 1=0", " WHERE name='private-narrowing'", " HAVING sum(amount)>0", " WHERE id IN (1,2)"} {
		t.Run(suffix, func(t *testing.T) {
			p, c := queryPopulationFixture(t, "SELECT sum(amount) FROM analytics.sales"+suffix, []BusinessConstraint{businessFixtureConstraint()}, analyticalMetrics(analyticalMeasure("sum", "amount")))
			proof, err := CheckAnalyticalPlan(context.Background(), p, c)
			if suffix == "" {
				if err != nil || proof == nil || proof.QueryPopulation != AnalyticalQueryPopulationPolicy {
					t.Fatal("owned population not proven", err)
				}
			} else if !errors.Is(err, ErrAnalyticalMismatch) || proof != nil {
				t.Fatal("invented narrowing acquired proof", err)
			}
		})
	}
}

func TestSQLRecoveryQueryPopulationTypedNullRangeAndAggregate(t *testing.T) {
	base := businessFixtureConstraint()
	cases := []BusinessConstraint{base}
	for _, nulls := range []string{"include", "only"} {
		c := base
		c.Nulls = nulls
		if nulls == "only" {
			c.Null, c.Value = true, ""
		}
		cases = append(cases, c)
	}
	c := base
	c.Operator, c.Value, c.Upper, c.Bounds = "range", "1.5", "9.75", "[)"
	cases = append(cases, c)
	c = base
	c.Aggregation = "sum"
	cases = append(cases, c)
	cases = append(cases, BusinessConstraint{Resolution: Hash("private-entity"), Dataset: "sales", Column: "name", SourceRevision: 1, Kind: "entity", Operator: "eq", Nulls: "exclude", Value: "private-value-731"})
	for i, c := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			p, contract := queryPopulationFixture(t, "SELECT sum(s.amount) FROM analytics.sales AS s", []BusinessConstraint{c}, analyticalMetrics(analyticalMeasure("sum", "amount")))
			before := Hash(contract)
			proof, err := CheckAnalyticalPlan(context.Background(), p, contract)
			if err != nil || proof == nil || before != Hash(contract) {
				t.Fatal("typed population failed or mutated", err)
			}
			wire, _ := json.Marshal(proof)
			if strings.Contains(string(wire), c.Value) && len(c.Value) > 3 || strings.Contains(fmt.Sprintf("%v %#v", contract.QueryPopulation, contract.QueryPopulation), c.Value) && len(c.Value) > 3 {
				t.Fatal("population values leaked")
			}
		})
	}
}

func TestSQLRecoveryQueryPopulationCompletenessAndParameterIdentity(t *testing.T) {
	constraints := []BusinessConstraint{businessFixtureConstraint(), {Resolution: Hash("flag"), Dataset: "sales", Column: "active", SourceRevision: 1, Kind: "boolean", Operator: "eq", Nulls: "exclude", Value: "true"}}
	p, c := queryPopulationFixture(t, "SELECT sum(amount) FROM analytics.sales", constraints, analyticalMetrics(analyticalMeasure("sum", "amount")))
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	bad := p
	bad.candidate.parameters = append([]Parameter(nil), p.candidate.parameters...)
	bad.candidate.parameters[0].Value = "2"
	if proof, err := CheckAnalyticalPlan(context.Background(), bad, c); err == nil || proof != nil {
		t.Fatal("changed binding acquired proof")
	}
	bound, err := BindBusinessConstraints(context.Background(), p.candidate.binding, "SELECT sum(amount) FROM analytics.sales", nil, constraints[:1])
	if err != nil {
		t.Fatal(err)
	}
	bad = p
	bad.candidate.statement, bad.candidate.parameters = bound.SQL, bound.Parameters
	if proof, err := CheckAnalyticalPlan(context.Background(), bad, c); !errors.Is(err, ErrAnalyticalMismatch) || proof != nil {
		t.Fatal("missing predicate acquired proof", err)
	}
	bad = p
	bad.candidate.statement = strings.Replace(p.candidate.statement, " >= ", " > ", 1)
	if proof, err := CheckAnalyticalPlan(context.Background(), bad, c); !errors.Is(err, ErrAnalyticalMismatch) || proof != nil {
		t.Fatal("changed bound acquired proof", err)
	}
}

func TestSQLRecoveryQueryPopulationKeepsMetricPopulationsIndependent(t *testing.T) {
	f := AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"A"}}
	one := analyticalMetrics(analyticalMeasure("sum", "amount", f))
	for _, sql := range []string{"SELECT sum(amount) FROM analytics.sales WHERE name='A'", "SELECT sum(amount) FILTER (WHERE name='A') FROM analytics.sales"} {
		p, c := queryPopulationFixture(t, sql, []BusinessConstraint{businessFixtureConstraint()}, one)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal("common metric filter rejected", err)
		}
	}
	other := AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"B"}}
	p, c := queryPopulationFixture(t, "SELECT sum(amount) FILTER (WHERE name='A'),sum(amount) FILTER (WHERE name='B') FROM analytics.sales", []BusinessConstraint{businessFixtureConstraint()}, analyticalMetrics(one[0].Expression, analyticalMeasure("sum", "amount", other)))
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("separate metric populations rejected", err)
	}
	p.candidate.statement = strings.Replace(p.candidate.statement, " WHERE ", " WHERE name='A' AND ", 1)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("independent metric population collapsed")
	}
}

func TestSQLRecoveryQueryPopulationProofBoundaries(t *testing.T) {
	p, c := queryPopulationFixture(t, "SELECT sum(amount) FROM analytics.sales", []BusinessConstraint{businessFixtureConstraint()}, analyticalMetrics(analyticalMeasure("sum", "amount")))
	original := c.QueryPopulation.Constraints[0]
	for _, mutate := range []func(*AnalyticalContract){
		func(c *AnalyticalContract) { c.Version = AnalyticalCalendarVersion },
		func(c *AnalyticalContract) { c.QueryPopulation.Policy = "unknown" },
		func(c *AnalyticalContract) { c.QueryPopulation.Constraints = nil },
		func(c *AnalyticalContract) { c.QueryPopulation.Constraints[0].SourceRevision++ },
		func(c *AnalyticalContract) { c.QueryPopulation.Constraints[0].Dataset = "items" },
	} {
		bad := c
		copy := *c.QueryPopulation
		copy.Constraints = []BusinessConstraint{original}
		bad.QueryPopulation = &copy
		mutate(&bad)
		if proof, err := CheckAnalyticalPlan(context.Background(), p, bad); err == nil || proof != nil {
			t.Fatal("invalid population acquired proof")
		}
	}
	if !reflect.DeepEqual(c.QueryPopulation.Constraints, []BusinessConstraint{original}) {
		t.Fatal("shared population changed")
	}
	bad := p
	bad.nativeChecked = false
	if _, err := CheckAnalyticalPlan(context.Background(), bad, c); !errors.Is(err, ErrBinding) {
		t.Fatal("bypassed native gate", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckAnalyticalPlan(ctx, p, c); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
	if _, err := NewAnalyticalQueryPopulation(nil, p.candidate.binding, c.Dataset, []BusinessConstraint{original}); !errors.Is(err, ErrBinding) {
		t.Fatal("nil context", err)
	}
}
