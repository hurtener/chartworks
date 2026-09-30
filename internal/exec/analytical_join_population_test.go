package exec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSQLRecoveryJoinedOwnedPredicateProof(t *testing.T) {
	const base = `SELECT c.region,sum(s.amount) FROM analytics.sales s JOIN analytics.customers c ON s.id=c.id GROUP BY c.region`
	p, c := analyticalJoinFixture(t, base)
	c.Joins[0].Type = "inner"
	constraints := []BusinessConstraint{{Resolution: Hash("private-region"), Dataset: "customers", Column: "region", SourceRevision: 1, Kind: "entity", Operator: "eq", Nulls: "exclude", Value: "PRIVATE-REGION-712"}}
	bound, err := BindBusinessConstraints(context.Background(), p.candidate.binding, base, nil, constraints)
	if err != nil {
		t.Fatal(err)
	}
	c.Version = AnalyticalGroupedPopulationsVersion
	c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
	c.QueryPopulation, err = NewAnalyticalQueryPopulationWithin(context.Background(), p.candidate.binding, []string{"sales", "customers"}, constraints)
	if err != nil {
		t.Fatal(err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	receipt, err := CheckAnalyticalPlan(context.Background(), p, c)
	if err != nil || receipt == nil {
		t.Fatal("secondary owned predicate", err)
	}
	raw, _ := json.Marshal(receipt)
	if strings.Contains(string(raw), "PRIVATE-REGION") {
		t.Fatal("private value in receipt")
	}
	for _, mutate := range []func(*Plan, *AnalyticalContract){
		func(p *Plan, c *AnalyticalContract) { p.candidate.statement = base; p.candidate.parameters = nil },
		func(p *Plan, c *AnalyticalContract) {
			p.candidate.parameters = append([]Parameter(nil), p.candidate.parameters...)
			p.candidate.parameters[0].Value = "PRIVATE-OTHER"
		},
		func(p *Plan, c *AnalyticalContract) { c.Joins = nil },
		func(p *Plan, c *AnalyticalContract) { c.Version = AnalyticalIntentVersion },
	} {
		next, contract := p, c
		mutate(&next, &contract)
		if proof, err := CheckAnalyticalPlan(context.Background(), next, contract); err == nil || proof != nil {
			t.Fatal("changed owned predicate obtained proof")
		}
	}
	// A secondary HAVING aggregate has its own grain obligation; a unique
	// dimension key alone cannot prove that facts did not duplicate its input.
	secondaryAggregate := c
	secondaryAggregate.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy, Constraints: []BusinessConstraint{{Dataset: "customers", Aggregation: "sum"}}}
	withoutFactKey := p.candidate.binding.Clone()
	withoutFactKey.Relations[0].UniqueKeys = nil
	if err := ValidateAnalyticalJoins(secondaryAggregate, withoutFactKey); err == nil {
		t.Fatal("owned aggregate borrowed primary metric grain")
	}
	if _, err := NewAnalyticalQueryPopulationWithin(context.Background(), p.candidate.binding, []string{"sales"}, constraints); err == nil {
		t.Fatal("constraint outside admitted coordinates accepted")
	}
	if _, err := NewAnalyticalQueryPopulationWithin(context.Background(), p.candidate.binding, []string{"sales", "sales", "customers"}, constraints); err == nil {
		t.Fatal("duplicate admitted coordinates")
	}
}
