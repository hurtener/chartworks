package exec

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func completenessFixture(t *testing.T, sql string, grouped bool) (Plan, AnalyticalContract) {
	t.Helper()
	f := AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"paid"}}
	sum := analyticalMeasure("sum", "amount", f)
	unknown := AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{analyticalMeasure("count", "id", f), analyticalMeasure("count", "amount", f)}}
	p, c := ordinaryGroupFixture(t, sql, AnalyticalGroupDomainQualifying, sum, unknown)
	c.Version = AnalyticalScopedPopulationsVersion
	if !grouped {
		c.Grain = nil
		c.GroupDomain = nil
	}
	c.Completeness = &AnalyticalCompleteness{Policy: AnalyticalCompletenessPolicy, Obligations: []AnalyticalCompletenessObligation{{Metric: c.Metrics[0].ID, UnknownCount: c.Metrics[1].ID}}}
	return p, c
}

func TestSQLRecoveryCompletenessProvedOutputOrdinals(t *testing.T) {
	for _, tc := range []struct {
		sql     string
		grouped bool
		want    []int
	}{
		{`SELECT count(id)-count(amount) AS revenue,sum(amount) AS unknown FROM analytics.sales WHERE name='paid'`, false, []int{1, 0}},
		{`SELECT active,count(id)-count(amount) AS revenue,sum(amount) AS unknown FROM analytics.sales WHERE name='paid' GROUP BY active`, true, []int{2, 1}},
		{`SELECT sum(amount) AS x,sum(amount) AS y,count(id)-count(amount) AS z FROM analytics.sales WHERE name='paid'`, false, []int{0, 2}},
	} {
		p, c := completenessFixture(t, tc.sql, tc.grouped)
		before := Hash(c)
		proof, err := CheckAnalyticalPlan(context.Background(), p, c)
		if err != nil {
			t.Fatal(err)
		}
		want := []AnalyticalOutput{{Metric: c.Metrics[0].ID, Column: tc.want[0]}, {Metric: c.Metrics[1].ID, Column: tc.want[1]}}
		if !reflect.DeepEqual(proof.Outputs, want) || !AnalyticalOutputsValid(proof) || !strings.HasSuffix(proof.Scope, AnalyticalCompletenessScope) || Hash(c) != before {
			t.Fatal("final ordinal proof", proof)
		}
	}
}

func TestSQLRecoveryCompletenessRetainsPopulationAndVersionProof(t *testing.T) {
	base := `SELECT active,sum(amount),count(id)-count(amount) FROM analytics.sales WHERE name='paid' GROUP BY active`
	for _, sql := range []string{
		strings.Replace(base, "count(id)-count(amount)", "count(amount)-count(id)", 1),
		strings.Replace(base, "count(id)-count(amount)", "count(id)-count(amount) FILTER(WHERE active)", 1),
		strings.Replace(strings.Replace(base, "sum(amount),count(id)-count(amount)", "sum(amount) FILTER(WHERE name='paid'),count(id) FILTER(WHERE name='paid')-count(amount) FILTER(WHERE name='paid')", 1), " WHERE name='paid'", "", 1),
		strings.Replace(base, " WHERE name='paid'", " WHERE name='paid' AND id>1", 1),
	} {
		p, c := completenessFixture(t, sql, true)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("altered completeness population admitted", sql)
		}
	}
	for _, mutate := range []func(*Plan, *AnalyticalContract){
		func(p *Plan, c *AnalyticalContract) { c.Version = AnalyticalGroupedProgramsVersion },
		func(p *Plan, c *AnalyticalContract) { c.Intent = nil },
		func(p *Plan, c *AnalyticalContract) { c.QueryPopulation = nil },
		func(p *Plan, c *AnalyticalContract) { c.Completeness = nil },
		func(p *Plan, c *AnalyticalContract) { c.Completeness.Obligations[0].UnknownCount = "missing" },
		func(p *Plan, c *AnalyticalContract) {
			for i := range p.candidate.binding.Relations[0].Columns {
				if p.candidate.binding.Relations[0].Columns[i].Name == "id" {
					p.candidate.binding.Relations[0].Columns[i].Nullable = true
				}
			}
			c.Binding = Hash(p.candidate.binding)
		},
	} {
		p, c := completenessFixture(t, base, true)
		mutate(&p, &c)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("weaker completeness custody accepted")
		}
	}
	p, c := completenessFixture(t, base, true)
	c.Version = AnalyticalGroupedProgramsVersion
	c.Completeness = nil
	proof, err := CheckAnalyticalPlan(context.Background(), p, c)
	if err != nil || len(proof.Outputs) != 0 {
		t.Fatal("retained v8 acquired v9 output meaning", err)
	}
	proof.Outputs = []AnalyticalOutput{{Metric: proof.Metrics[0], Column: 1}}
	if AnalyticalOutputsValid(proof) {
		t.Fatal("v8 accepted output evidence")
	}
}

func TestSQLRecoveryCompletenessRetainsOwnedPredicates(t *testing.T) {
	base := `SELECT active,sum(amount),count(id)-count(amount) FROM analytics.sales WHERE name='paid' GROUP BY active`
	p, c := completenessFixture(t, base, true)
	owned := businessFixtureConstraint()
	owned.Operator, owned.Value, owned.Upper, owned.Bounds = "range", "1.5", "9.75", "[)"
	var err error
	c.QueryPopulation, err = NewAnalyticalQueryPopulation(context.Background(), p.candidate.binding, c.Dataset, []BusinessConstraint{owned})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindBusinessConstraints(context.Background(), p.candidate.binding, base, nil, []BusinessConstraint{owned})
	if err != nil {
		t.Fatal(err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("owned predicate lost under completeness", err)
	}
	p.candidate.parameters = append([]Parameter(nil), bound.Parameters...)
	p.candidate.parameters[0].Value = "0"
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("changed owned population passed completeness")
	}
	p.candidate.statement, p.candidate.parameters = base, nil
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("missing owned predicate passed completeness")
	}
}

func TestSQLRecoveryCompletenessEffectiveJoinNullability(t *testing.T) {
	sql := `SELECT s.active,sum(s.amount),sum(c.amount),count(c.id)-count(c.amount) FROM analytics.sales s LEFT JOIN analytics.customers c ON s.id=c.id GROUP BY s.active`
	p, c := analyticalJoinFixture(t, sql)
	p.candidate.binding.Relations[1].Columns = append(p.candidate.binding.Relations[1].Columns, Column{Name: "amount", NativeType: "numeric", Category: "numeric", Nullable: true, Safe: true})
	c.Binding = Hash(p.candidate.binding)
	c.Version = AnalyticalScopedPopulationsVersion
	c.Grain = &AnalyticalGrain{Policy: AnalyticalGroupingPolicy, Columns: []string{"active"}, Dimensions: []string{"active"}}
	c.GroupDomain = &AnalyticalGroupDomain{Policy: AnalyticalGroupDomainPolicy, Domain: AnalyticalGroupDomainRaw}
	c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
	c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
	c.Metrics = analyticalMetrics(analyticalMeasure("sum", "amount"), analyticalMeasure("sum", "customers/amount"), AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{analyticalMeasure("count", "customers/id"), analyticalMeasure("count", "customers/amount")}})
	c.Completeness = &AnalyticalCompleteness{Policy: AnalyticalCompletenessPolicy, Obligations: []AnalyticalCompletenessObligation{{Metric: c.Metrics[1].ID, UnknownCount: c.Metrics[2].ID}}}
	for _, bindingOnly := range []bool{true, false} {
		var err error
		if bindingOnly {
			err = ValidateAnalyticalCompleteness(c, p.candidate.binding)
		} else {
			_, err = CheckAnalyticalPlan(context.Background(), p, c)
		}
		if err == nil {
			t.Fatal("raw non-null key certified a null-extended completeness population", bindingOnly)
		}
	}
	// INNER removes the null-extension ambiguity; both physical keys stay unique.
	c.Joins[0].Type = "inner"
	p.candidate.statement = strings.Replace(sql, "LEFT JOIN", "INNER JOIN", 1)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("INNER completeness control", err)
	}
	// LEFT is still permitted when the obligation belongs to the preserved side.
	c.Joins[0].Type = "left"
	c.Metrics[2].Expression = AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{analyticalMeasure("count", "id"), analyticalMeasure("count", "amount")}}
	c.Completeness.Obligations[0].Metric = c.Metrics[0].ID
	p.candidate.statement = strings.Replace(sql, "count(c.id)-count(c.amount)", "count(s.id)-count(s.amount)", 1)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("preserved LEFT-side completeness control", err)
	}
}
