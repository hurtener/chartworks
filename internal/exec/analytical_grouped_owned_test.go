package exec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func groupedOwnedFixture(t *testing.T, domain string, shared bool) (Plan, AnalyticalContract, string) {
	t.Helper()
	base := strings.ReplaceAll(groupedSQL, "LEFT JOIN analytics.dimensions", "JOIN analytics.dimensions")
	base = strings.Replace(base, "sum(s.amount)", "sum(s.amount) FILTER (WHERE s.status='paid')", 1)
	base = strings.Replace(base, "sum(i.quantity)", "sum(i.quantity) FILTER (WHERE i.status='paid')", 1)
	if domain == AnalyticalGroupDomainQualifying {
		base = strings.Replace(base, "s.id=d.id GROUP BY", "s.id=d.id WHERE s.status='paid' GROUP BY", 1)
		base = strings.Replace(base, "i.sale_id=d.id GROUP BY", "i.sale_id=d.id WHERE i.status='paid' GROUP BY", 1)
	}
	p, c := groupedFixture(t, base)
	for i := range p.candidate.binding.Relations {
		r := &p.candidate.binding.Relations[i]
		r.Columns = append(r.Columns, Column{Name: "occurred", NativeType: "timestamptz", Category: "temporal", Nullable: true, Safe: true}, Column{Name: "status", NativeType: "text", Category: "text", Safe: true})
	}
	c.Version = AnalyticalGroupedOwnedPopulationsVersion
	c.Binding = Hash(p.candidate.binding)
	c.Metrics = analyticalMetrics(AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{
		analyticalMeasure("sum", "amount", AnalyticalFilter{Column: "status", Kind: "eq", Values: []string{"paid"}}),
		analyticalMeasure("sum", "items/quantity", AnalyticalFilter{Column: "items/status", Kind: "eq", Values: []string{"paid"}}),
	}})
	for i := range c.GroupedPopulations.Lanes {
		lane := &c.GroupedPopulations.Lanes[i]
		lane.Domain = domain
		lane.Joins[0].Type = "inner"
		target := lane.Dataset
		if shared {
			target = "dimensions"
		}
		constraint := BusinessConstraint{Resolution: Hash("period-" + lane.Dataset), Dataset: target, Column: "occurred", SourceRevision: 1, Kind: "time_window", Operator: "range", Nulls: "exclude", Bounds: "[)", TemporalType: "timestamptz", Calendar: "gregorian", TimeZone: "America/New_York", Grain: "year", Value: "2026-01-01T05:00:00Z", Upper: "2027-01-01T05:00:00Z"}
		population, err := NewAnalyticalQueryPopulation(context.Background(), p.candidate.binding, target, []BusinessConstraint{constraint})
		if err != nil {
			t.Fatal(err)
		}
		lane.QueryPopulation = population
	}
	return p, c, base
}

func TestSQLRecoveryGroupedOwnedPopulationProof(t *testing.T) {
	for _, domain := range []string{AnalyticalGroupDomainRaw, AnalyticalGroupDomainQualifying} {
		for _, shared := range []bool{false, true} {
			t.Run(domain+map[bool]string{false: "_fact", true: "_shared"}[shared], func(t *testing.T) {
				p, c, base := groupedOwnedFixture(t, domain, shared)
				before := Hash(c)
				bound, err := BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, base, nil, c)
				if err != nil {
					t.Fatal("bind", err)
				}
				if bound.Receipt.SchemaVersion != 3 || bound.Receipt.PopulationPolicy != AnalyticalGroupedOwnedPopulationPolicy || bound.Receipt.Constraints != Hash(c.GroupedPopulations) || len(bound.Parameters) != 4 || len(bound.Receipt.Bindings) != 2 {
					t.Fatal("lost lane evidence")
				}
				p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
				receipt, err := CheckAnalyticalPlan(t.Context(), p, c)
				if err != nil {
					t.Fatal("proof", err)
				}
				if receipt.Version != AnalyticalGroupedOwnedPopulationsVersion || receipt.Contract != before || Hash(c) != before || !strings.HasSuffix(receipt.Scope, ";independent_owned_grouped_populations") || len(receipt.Outputs) != 1 || receipt.Outputs[0].Column != 1 {
					t.Fatal("wrong owned receipt")
				}
				raw, _ := json.Marshal(bound.Receipt)
				if strings.Contains(string(raw), "2026") || strings.Contains(string(raw), "05:00") {
					t.Fatal("receipt leaked private period")
				}
				for _, version := range []string{AnalyticalGroupedPopulationsVersion, AnalyticalGroupedProgramsVersion, AnalyticalScopedPopulationsVersion} {
					old := c
					old.Version = version
					if _, err := CheckAnalyticalPlan(t.Context(), p, old); err == nil {
						t.Fatal("old policy borrowed owned lanes", version)
					}
				}
			})
		}
	}
}

func TestSQLRecoveryGroupedOwnedBindingCustody(t *testing.T) {
	p, c, base := groupedOwnedFixture(t, AnalyticalGroupDomainQualifying, false)
	for name, sql := range map[string]string{
		"capture":        strings.Replace(base, "s.value-i.value", "s.value-i.value+$1", 1),
		"outer_where":    base + " WHERE s.value>0",
		"duplicate_fact": strings.Replace(base, "FROM analytics.items i", "FROM analytics.sales i", 1),
		"wrapper":        "SELECT w.region,w.net FROM (" + base + ") w",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, sql, nil, c); err == nil {
				t.Fatal("unowned placement bound")
			}
		})
	}
	if _, err := BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, base, []Parameter{{Kind: "text", Value: "private"}}, c); err == nil {
		t.Fatal("model parameter admitted")
	}
	bound, err := BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, base, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	for name, change := range map[string]func(*Plan){
		"swapped_values": func(p *Plan) {
			p.candidate.parameters = append([]Parameter(nil), p.candidate.parameters...)
			p.candidate.parameters[0].Value = "2025-01-01T05:00:00Z"
		},
		"domain_relocated": func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, "WHERE (s.status='paid') AND", "WHERE", 1)
		},
		"spine_filter": func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, "SELECT region FROM i)", "SELECT region FROM i WHERE region IS NOT NULL)", 1)
		},
		"outer_substitute": func(p *Plan) { p.candidate.statement += " WHERE s.value>0" },
		"wrong_lane_period": func(p *Plan) {
			p.candidate.statement = strings.ReplaceAll(p.candidate.statement, `"i"."occurred"`, `"d"."occurred"`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := p
			change(&bad)
			if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
				t.Fatal("changed custody accepted")
			}
		})
	}
	for name, change := range map[string]func(*AnalyticalContract){
		"missing_lane":      func(c *AnalyticalContract) { c.GroupedPopulations.Lanes = c.GroupedPopulations.Lanes[:1] },
		"duplicate_lane":    func(c *AnalyticalContract) { c.GroupedPopulations.Lanes[1] = c.GroupedPopulations.Lanes[0] },
		"missing_period":    func(c *AnalyticalContract) { c.GroupedPopulations.Lanes[0].QueryPopulation = nil },
		"outer_population":  func(c *AnalyticalContract) { c.QueryPopulation = c.GroupedPopulations.Lanes[0].QueryPopulation },
		"unreviewed_domain": func(c *AnalyticalContract) { c.GroupedPopulations.Lanes[0].Domain = "" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := c
			g := *c.GroupedPopulations
			g.Lanes = append([]AnalyticalGroupedLane(nil), g.Lanes...)
			bad.GroupedPopulations = &g
			change(&bad)
			if _, err := CheckAnalyticalPlan(t.Context(), p, bad); err == nil {
				t.Fatal("incomplete owned contract accepted")
			}
		})
	}
}

func TestSQLRecoveryGroupedOwnedDistinctIntervals(t *testing.T) {
	p, c, base := groupedOwnedFixture(t, AnalyticalGroupDomainRaw, false)
	for i := range c.GroupedPopulations.Lanes {
		lane := &c.GroupedPopulations.Lanes[i]
		if lane.Dataset == "items" {
			detached := *lane.QueryPopulation
			detached.Constraints = append([]BusinessConstraint(nil), detached.Constraints...)
			detached.Constraints[0].Value = "2025-01-01T05:00:00Z"
			detached.Constraints[0].Upper = "2026-01-01T05:00:00Z"
			lane.QueryPopulation = &detached
		}
	}
	bound, err := BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, base, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.Parameters) != 4 || bound.Parameters[0].Value != "2026-01-01T05:00:00Z" || bound.Parameters[2].Value != "2025-01-01T05:00:00Z" {
		t.Fatal("lane interval/index identity lost")
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	if _, err := CheckAnalyticalPlan(t.Context(), p, c); err != nil {
		t.Fatal("distinct per-lane intervals", err)
	}
	p.candidate.parameters = append([]Parameter(nil), bound.Parameters...)
	p.candidate.parameters[0], p.candidate.parameters[2] = p.candidate.parameters[2], p.candidate.parameters[0]
	p.candidate.parameters[1], p.candidate.parameters[3] = p.candidate.parameters[3], p.candidate.parameters[1]
	if _, err := CheckAnalyticalPlan(t.Context(), p, c); err == nil {
		t.Fatal("swapped valid lane intervals accepted")
	}
}

func TestSQLRecoveryGroupedOwnedCalendar(t *testing.T) {
	p, c, base := groupedOwnedFixture(t, AnalyticalGroupDomainQualifying, true)
	c.Grain.Columns = nil
	c.Grain.Buckets = []AnalyticalBucket{{Column: "dimensions/occurred", Grain: "month", Calendar: "gregorian", Timezone: "America/New_York"}}
	c.Grain.Dimensions = []string{"owned_calendar"}
	base = strings.ReplaceAll(strings.ReplaceAll(base, "d.region AS region", "date_trunc('month',d.occurred,'America/New_York') AS region"), "GROUP BY d.region", "GROUP BY 1")
	bound, err := BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, base, nil, c)
	if err != nil {
		t.Fatal("calendar bind", err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	receipt, err := CheckAnalyticalPlan(t.Context(), p, c)
	if err != nil || receipt.Scope != strings.ReplaceAll(AnalyticalCalendarScope, "single_base_relation", "independent_owned_grouped_populations") {
		t.Fatal("calendar population proof", err)
	}
	p.candidate.statement = strings.Replace(p.candidate.statement, "America/New_York", "UTC", 1)
	if _, err := CheckAnalyticalPlan(t.Context(), p, c); err == nil {
		t.Fatal("wrong shared calendar zone accepted")
	}
}

func TestSQLRecoveryGroupedOwnedQualifyingUnion(t *testing.T) {
	p, c, base := groupedOwnedFixture(t, AnalyticalGroupDomainQualifying, false)
	c.Metrics = append(c.Metrics, AnalyticalMetric{ID: "cancelled", Expression: analyticalMeasure("count", "id", AnalyticalFilter{Column: "status", Kind: "eq", Values: []string{"cancelled"}})})
	base = strings.Replace(base, "AS value FROM analytics.sales", "AS value, COUNT(s.id) FILTER (WHERE s.status='cancelled') AS cancelled FROM analytics.sales", 1)
	base = strings.Replace(base, "WHERE s.status='paid' GROUP BY", "WHERE (s.status='paid' OR s.status='cancelled') GROUP BY", 1)
	base = strings.Replace(base, "s.value-i.value AS net FROM keys", "s.value-i.value AS net, s.cancelled FROM keys", 1)
	bound, err := BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, base, nil, c)
	if err != nil {
		t.Fatal("union bind", err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	if _, err := CheckAnalyticalPlan(t.Context(), p, c); err != nil {
		t.Fatal("owned qualifying population union", err)
	}
	p.candidate.statement = strings.Replace(p.candidate.statement, "s.status='paid' OR s.status='cancelled'", "s.status='paid' AND s.status='cancelled'", 1)
	if _, err := CheckAnalyticalPlan(t.Context(), p, c); err == nil {
		t.Fatal("owned union changed to intersection")
	}
}
