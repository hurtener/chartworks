package exec

import (
	"context"
	"strings"
	"testing"
)

func scopedPopulationFixture(t *testing.T) (Plan, AnalyticalContract, string) {
	t.Helper()
	base := `WITH gross AS (SELECT sum(s.amount) AS total FROM analytics.sales s WHERE s.name='paid'), refunds AS (SELECT sum(i.quantity) AS total FROM analytics.items i JOIN analytics.sales o ON i.division=o.division AND i.sale_id=o.id WHERE i.status='posted' AND o.name='paid') SELECT gross.total-refunds.total AS net FROM gross CROSS JOIN refunds`
	p, c := independentFixture(t, base)
	for i := range p.candidate.binding.Relations {
		r := &p.candidate.binding.Relations[i]
		r.Columns = append(r.Columns, Column{Name: "division", NativeType: "integer", Category: "numeric", Safe: true}, Column{Name: "occurred", NativeType: "timestamptz", Category: "temporal", Safe: true, Nullable: true})
		if r.ID == "sales" {
			r.UniqueKeys = [][]string{{"division", "id"}}
		} else {
			r.Columns = append(r.Columns, Column{Name: "status", NativeType: "text", Category: "text", Safe: true})
		}
	}
	c.Version = AnalyticalScopedPopulationsVersion
	c.Binding = Hash(p.candidate.binding)
	c.Populations = nil
	c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
	c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
	c.Metrics = analyticalMetrics(AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{
		analyticalMeasure("sum", "amount", AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"paid"}}),
		analyticalMeasure("sum", "items/quantity", AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"paid"}}, AnalyticalFilter{Column: "items/status", Kind: "eq", Values: []string{"posted"}}),
	}})
	period := BusinessConstraint{Resolution: Hash("reviewed-current-year"), Dataset: "sales", Column: "occurred", SourceRevision: 1, Kind: "time_window", Operator: "range", Nulls: "exclude", Bounds: "[)", TemporalType: "timestamptz", Calendar: "gregorian", TimeZone: "America/New_York", Grain: "year", Value: "2026-01-01T05:00:00Z", Upper: "2027-01-01T05:00:00Z"}
	population, err := NewAnalyticalQueryPopulation(context.Background(), p.candidate.binding, "sales", []BusinessConstraint{period})
	if err != nil {
		t.Fatal(err)
	}
	c.ScalarPopulations = &AnalyticalScalarPopulations{Policy: AnalyticalScalarPopulationPolicy, Lanes: []AnalyticalScalarLane{
		{Dataset: "items", Joins: []AnalyticalJoin{{Left: "items", Right: "sales", Type: "inner", LeftColumns: []string{"division", "sale_id"}, RightColumns: []string{"division", "id"}}}, QueryPopulation: population},
		{Dataset: "sales", QueryPopulation: population},
	}}
	return p, c, base
}

func TestSQLRecoveryScopedSingletonPopulationProof(t *testing.T) {
	p, c, base := scopedPopulationFixture(t)
	before := Hash(c)
	bound, err := BindScalarPopulationConstraints(context.Background(), p.candidate.binding, base, nil, c)
	if err != nil {
		t.Fatal("bind exact repeated parent period", err)
	}
	if bound.Receipt.SchemaVersion != 2 || bound.Receipt.PopulationPolicy != AnalyticalScalarPopulationPolicy || len(bound.Parameters) != 4 || len(bound.Receipt.Bindings) != 2 || bound.Receipt.Bindings[0].Population != "sales" || bound.Receipt.Bindings[1].Population != "items" {
		t.Fatal("lost owned lane placement")
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	proof, err := CheckAnalyticalPlan(context.Background(), p, c)
	if err != nil || proof.Version != AnalyticalScopedPopulationsVersion || proof.Contract != before || Hash(c) != before || !strings.Contains(proof.Scope, "independent_scoped_singleton_populations") {
		t.Fatal("scoped singleton proof", err)
	}
	for _, change := range []func(*Plan){
		func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, "i.division=o.division AND ", "", 1)
		},
		func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, "i.status='posted' AND ", "", 1)
		},
		func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, "gross.total-refunds.total", "refunds.total-gross.total", 1)
		},
		func(p *Plan) {
			p.candidate.parameters = append([]Parameter(nil), p.candidate.parameters...)
			p.candidate.parameters[2].Value = "2025-01-01T05:00:00Z"
		},
		func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, `"o"."occurred"`, `"i"."occurred"`, -1)
		},
	} {
		bad := p
		change(&bad)
		if _, err := CheckAnalyticalPlan(context.Background(), bad, c); err == nil {
			t.Fatal("changed lane semantics admitted")
		}
	}
	legacy := c
	legacy.Version = AnalyticalGroupedProgramsVersion
	if _, err := CheckAnalyticalPlan(context.Background(), p, legacy); err == nil {
		t.Fatal("v9 lane fields accepted under v8")
	}
	p.candidate.binding.Relations[0].UniqueKeys = nil
	c.Binding = Hash(p.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("unproved parent uniqueness admitted")
	}
}

func TestSQLRecoveryScopedPopulationBindingCustody(t *testing.T) {
	p, c, base := scopedPopulationFixture(t)
	if _, err := BindBusinessConstraints(context.Background(), p.candidate.binding, base, nil, c.ScalarPopulations.Lanes[0].QueryPopulation.Constraints); err == nil {
		t.Fatal("legacy ambiguous CTE placement widened")
	}
	for _, sql := range []string{strings.Replace(base, "gross.total-refunds.total", "gross.total-refunds.total+$1", 1), strings.Replace(base, "gross AS (", "gross AS (SELECT 1), dead AS (", 1), strings.Replace(base, "FROM analytics.items i", "FROM analytics.sales i", 1)} {
		if _, err := BindScalarPopulationConstraints(context.Background(), p.candidate.binding, sql, nil, c); err == nil {
			t.Fatal("unproved scope/marker admitted", sql)
		}
	}
	if _, err := BindScalarPopulationConstraints(context.Background(), p.candidate.binding, base, []Parameter{{Kind: "number", Value: "42"}}, c); err == nil {
		t.Fatal("model parameters entered scoped binding")
	}
}
