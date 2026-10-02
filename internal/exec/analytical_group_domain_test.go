package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func ordinaryGroupFixture(t *testing.T, sql, domain string, metrics ...AnalyticalExpression) (Plan, AnalyticalContract) {
	t.Helper()
	p, c := analyticalFixture(t, sql, analyticalMetrics(metrics...))
	c.Version = AnalyticalGroupedProgramsVersion
	c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
	c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
	c.Grain = &AnalyticalGrain{Policy: AnalyticalGroupingPolicy, Columns: []string{"active"}, Dimensions: []string{"active"}}
	if domain != "" {
		c.GroupDomain = &AnalyticalGroupDomain{Policy: AnalyticalGroupDomainPolicy, Domain: domain}
	}
	return p, c
}

func TestSQLRecoveryOrdinaryReviewedGroupDomains(t *testing.T) {
	metric := analyticalMeasure("sum", "amount", AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"paid"}})
	where := `SELECT active,sum(amount) FROM analytics.sales WHERE name='paid' GROUP BY active`
	filter := `SELECT active,sum(amount) FILTER (WHERE name='paid') FROM analytics.sales GROUP BY active`
	for _, domain := range []string{AnalyticalGroupDomainRaw, AnalyticalGroupDomainQualifying} {
		for _, sql := range []string{where, filter} {
			p, c := ordinaryGroupFixture(t, sql, domain, metric)
			before := Hash(c)
			proof, err := CheckAnalyticalPlan(context.Background(), p, c)
			want := (sql == where) == (domain == AnalyticalGroupDomainQualifying)
			if (err == nil) != want || Hash(c) != before || want && (proof == nil || proof.Contract != before) {
				t.Fatal("changed group existence admitted", domain, sql, err)
			}
		}
	}
	p, c := ordinaryGroupFixture(t, filter, "", metric)
	var detail *AnalyticalError
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.As(err, &detail) || detail.Code != AnalyticalGroupDomainReviewCode {
		t.Fatal("missing reviewed domain was inferred", err)
	}
	for _, version := range []string{AnalyticalIntentVersion, AnalyticalGroupedPopulationsVersion} {
		c.Version = version
		for _, sql := range []string{filter, where} {
			p.candidate.statement = sql
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
				t.Fatal("retained proof policy changed", version, err)
			}
		}
	}
	c.Version = AnalyticalGroupedProgramsVersion
	c.Grain = nil
	p.candidate.statement = `SELECT sum(amount) FILTER (WHERE name='paid') FROM analytics.sales`
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("scalar total semantics changed", err)
	}
}

func TestSQLRecoveryOrdinaryGroupDomainOwnedPredicates(t *testing.T) {
	one := analyticalMeasure("sum", "amount", AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"paid"}})
	two := analyticalMeasure("count", "amount", AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"cancelled"}})
	base := `SELECT active,sum(amount) FILTER(WHERE name='paid'),count(amount) FILTER(WHERE name='cancelled') FROM analytics.sales WHERE name='paid' OR name='cancelled' GROUP BY active`
	p, c := ordinaryGroupFixture(t, base, AnalyticalGroupDomainQualifying, one, two)
	p.candidate.binding.Relations[0].Columns = append(p.candidate.binding.Relations[0].Columns, Column{Name: "occurred", NativeType: "timestamptz", Category: "temporal", Safe: true})
	c.Binding = Hash(p.candidate.binding)
	number := businessFixtureConstraint()
	number.Operator, number.Value, number.Upper, number.Bounds = "range", "1.5", "9.75", "[)"
	constraints := []BusinessConstraint{
		number,
		{Resolution: Hash("private-active"), Dataset: "sales", Column: "active", SourceRevision: 1, Kind: "boolean", Operator: "eq", Nulls: "exclude", Value: "true"},
		{Resolution: Hash("private-time"), Dataset: "sales", Column: "occurred", SourceRevision: 1, Kind: "time_window", Operator: "range", Nulls: "exclude", Bounds: "[)", TemporalType: "timestamptz", Calendar: "gregorian", TimeZone: "UTC", Grain: "day", Value: "2026-01-02T00:00:00Z", Upper: "2026-01-03T00:00:00Z"},
	}
	having := businessFixtureConstraint()
	having.Resolution, having.Aggregation, having.Value = Hash("owned-having"), "sum", "2"
	constraints = append(constraints, having)
	var err error
	c.QueryPopulation, err = NewAnalyticalQueryPopulation(context.Background(), p.candidate.binding, c.Dataset, constraints)
	if err != nil {
		t.Fatal(err)
	}
	bind := func(sql string, owned []BusinessConstraint) Plan {
		t.Helper()
		bound, err := BindBusinessConstraints(context.Background(), p.candidate.binding, sql, nil, owned)
		if err != nil {
			t.Fatal(err)
		}
		next := p
		next.candidate.statement, next.candidate.parameters = bound.SQL, bound.Parameters
		return next
	}
	good := bind(base, constraints)
	if _, err := CheckAnalyticalPlan(context.Background(), good, c); err != nil {
		t.Fatal("owned scalar/range/time + qualifying union", err)
	}
	for _, bad := range []Plan{
		bind(base, constraints[:2]),
		bind(base, constraints[:3]),
		bind(strings.Replace(base, " WHERE name='paid' OR name='cancelled' GROUP", " GROUP", 1), constraints),
		bind(strings.Replace(base, " OR name='cancelled' GROUP", " AND name='cancelled' GROUP", 1), constraints),
		bind(base+" HAVING sum(amount)>0", constraints),
	} {
		if _, err := CheckAnalyticalPlan(context.Background(), bad, c); err == nil {
			t.Fatal("missing/changed domain or owned condition admitted")
		}
	}
	bad := good
	bad.candidate.parameters = append([]Parameter(nil), good.candidate.parameters...)
	bad.candidate.parameters[0].Value = "2.5"
	if _, err := CheckAnalyticalPlan(context.Background(), bad, c); err == nil {
		t.Fatal("owned value substitution admitted")
	}
	c.GroupDomain.Domain = AnalyticalGroupDomainRaw
	rawBase := strings.Replace(base, " WHERE name='paid' OR name='cancelled' GROUP", " GROUP", 1)
	if _, err := CheckAnalyticalPlan(context.Background(), bind(rawBase, constraints), c); err != nil {
		t.Fatal("raw groups lost independently owned WHERE/HAVING", err)
	}
}

func TestSQLRecoveryOrdinaryJoinedGroupDomain(t *testing.T) {
	base := `SELECT c.region,sum(s.amount) FILTER(WHERE s.name='paid') FROM analytics.sales s LEFT JOIN analytics.customers c ON s.id=c.id GROUP BY c.region`
	p, c := analyticalJoinFixture(t, base)
	c.Version = AnalyticalGroupedProgramsVersion
	c.Metrics[0].Expression.Filters = []AnalyticalFilter{{Column: "name", Kind: "eq", Values: []string{"paid"}}}
	c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
	c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
	c.GroupDomain = &AnalyticalGroupDomain{Policy: AnalyticalGroupDomainPolicy, Domain: AnalyticalGroupDomainRaw}
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("reviewed raw joined domain", err)
	}
	p.candidate.statement = strings.Replace(strings.Replace(base, " FILTER(WHERE s.name='paid')", "", 1), " GROUP BY", " WHERE s.name='paid' GROUP BY", 1)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("raw joined group domain narrowed")
	}
	c.GroupDomain.Domain = AnalyticalGroupDomainQualifying
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("reviewed qualifying joined domain", err)
	}
	p.candidate.binding.Relations[1].UniqueKeys = nil
	c.Binding = Hash(p.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("domain metadata authorized fanout")
	}
}

func TestSQLRecoveryOrdinaryCommonPredicateNormalization(t *testing.T) {
	common := AnalyticalFilter{Column: "name", Kind: "eq", Values: []string{"paid"}}
	yes := AnalyticalFilter{Column: "active", Kind: "eq", Values: []string{"true"}}
	no := AnalyticalFilter{Column: "active", Kind: "eq", Values: []string{"false"}}
	one := analyticalMeasure("sum", "amount", common, yes)
	two := analyticalMeasure("count", "amount", common, no)
	projection := `SELECT active,sum(amount) FILTER(WHERE active=true),count(amount) FILTER(WHERE active=false) FROM analytics.sales WHERE `
	group := ` GROUP BY active`
	distributed := `(name='paid' AND active=true) OR (name='paid' AND active=false)`
	for _, where := range []string{`name='paid' AND (active=true OR active=false)`, distributed} {
		p, c := ordinaryGroupFixture(t, projection+where+group, AnalyticalGroupDomainQualifying, one, two)
		before := Hash(c)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil || Hash(c) != before {
			t.Fatal("equivalent common metric population rejected", where, err)
		}
		// Independently bound private predicates must still be present and exact.
		constraint := businessFixtureConstraint()
		var err error
		c.QueryPopulation, err = NewAnalyticalQueryPopulation(context.Background(), p.candidate.binding, c.Dataset, []BusinessConstraint{constraint})
		if err != nil {
			t.Fatal(err)
		}
		bound, err := BindBusinessConstraints(context.Background(), p.candidate.binding, projection+where+group, nil, []BusinessConstraint{constraint})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("missing owned predicate accepted")
		}
		p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal("owned predicate lost common population equivalence", err)
		}
		p.candidate.parameters = append([]Parameter(nil), bound.Parameters...)
		p.candidate.parameters[0].Value = "999"
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("changed owned predicate accepted")
		}
	}
	for _, sql := range []string{
		projection + `(name='paid' AND active=true) OR active=false` + group,
		projection + `(name='paid' AND active=true) OR (name='cancelled' AND active=false)` + group,
		strings.Replace(projection, ` FILTER(WHERE active=true)`, "", 1) + distributed + group,
		strings.Replace(projection, ` FILTER(WHERE active=false)`, "", 1) + distributed + group,
	} {
		p, c := ordinaryGroupFixture(t, sql, AnalyticalGroupDomainQualifying, one, two)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("unproved or non-common population promoted", sql)
		}
	}
	p, c := ordinaryGroupFixture(t, projection+distributed+group, AnalyticalGroupDomainRaw, one, two)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("raw groups silently narrowed")
	}
	for _, version := range []string{AnalyticalIntentVersion, AnalyticalGroupedPopulationsVersion} {
		c.Version, c.GroupDomain = version, nil
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalMismatch) {
			t.Fatal("retained normalization policy changed", version, err)
		}
	}
}
