package exec

import (
	"encoding/json"
	"strings"
	"testing"
)

func groupSelectionFixture(t *testing.T, domain string, periods bool) (Plan, AnalyticalContract, string) {
	t.Helper()
	p, c, sql := groupedOwnedFixture(t, domain, false)
	if !periods {
		for i := range c.GroupedPopulations.Lanes {
			c.GroupedPopulations.Lanes[i].QueryPopulation = nil
		}
	}
	c.Version = AnalyticalGroupedSelectionVersion
	selection, err := NewAnalyticalQueryPopulation(t.Context(), p.candidate.binding, "dimensions", []BusinessConstraint{{Resolution: Hash("private group choice"), Dataset: "dimensions", Column: "region", SourceRevision: 1, Kind: "text", Operator: "eq", Nulls: "exclude", Value: "PRIVATE_NORTH"}})
	if err != nil {
		t.Fatal(err)
	}
	c.GroupSelection = selection
	return p, c, sql
}

func TestSQLRecoveryGroupedSelectionProofAndCustody(t *testing.T) {
	for _, domain := range []string{AnalyticalGroupDomainRaw, AnalyticalGroupDomainQualifying} {
		for _, periods := range []bool{false, true} {
			name := domain + map[bool]string{false: "_unowned", true: "_owned_periods"}[periods]
			t.Run(name, func(t *testing.T) {
				p, c, sql := groupSelectionFixture(t, domain, periods)
				before := Hash(c)
				bound, err := BindGroupedSelectionConstraints(t.Context(), p.candidate.binding, sql, nil, c)
				if err != nil {
					t.Fatal("bind", err)
				}
				want := 1
				if periods {
					want = 5
				}
				if len(bound.Parameters) != want || bound.Receipt.SchemaVersion != 4 || bound.Receipt.PopulationPolicy != AnalyticalGroupedSelectionPolicy || bound.Receipt.Constraints != Hash([]any{c.GroupedPopulations, c.GroupSelection}) {
					t.Fatal("binding evidence incomplete")
				}
				if strings.Contains(bound.SQL, "PRIVATE_NORTH") {
					t.Fatal("private value in statement")
				}
				p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
				receipt, err := CheckAnalyticalPlan(t.Context(), p, c)
				if err != nil {
					t.Fatal("proof", err)
				}
				if receipt.Version != AnalyticalGroupedSelectionVersion || receipt.Contract != before || Hash(c) != before || !strings.HasSuffix(receipt.Scope, ";independent_selected_grouped_populations") || len(receipt.Outputs) != 1 || !AnalyticalOutputsValid(receipt) {
					t.Fatal("wrong proof or caller mutation")
				}
				raw, _ := json.Marshal(bound.Receipt)
				if strings.Contains(string(raw), "PRIVATE_NORTH") || strings.Contains(string(raw), "2026") {
					t.Fatal("private value in receipt")
				}
				groups := 0
				for _, effect := range bound.Receipt.Bindings {
					if effect.Population == "" {
						groups++
						if len(effect.Parameters) != 1 || effect.Parameters[0] != want {
							t.Fatal("group custody lost")
						}
					}
				}
				if groups != 1 {
					t.Fatal("group binding missing")
				}
				for _, version := range []string{AnalyticalGroupedProgramsVersion, AnalyticalGroupedOwnedPopulationsVersion, AnalyticalScopedPopulationsVersion} {
					old := c
					old.Version = version
					if _, err := CheckAnalyticalPlan(t.Context(), p, old); err == nil {
						t.Fatal("old policy borrowed group selection", version)
					}
				}
			})
		}
	}
}

func TestSQLRecoveryGroupedSelectionRejectsDriftAndRelocation(t *testing.T) {
	p, c, sql := groupSelectionFixture(t, AnalyticalGroupDomainQualifying, true)
	for name, statement := range map[string]string{
		"model_parameter": strings.Replace(sql, "s.value-i.value", "s.value-i.value+$1", 1),
		"outer_filter":    sql + " WHERE k.region='north'",
		"derived_wrapper": "SELECT w.region,w.net FROM (" + sql + ") w",
		"spine_filter":    strings.Replace(sql, "SELECT region FROM i)", "SELECT region FROM i WHERE region IS NOT NULL)", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BindGroupedSelectionConstraints(t.Context(), p.candidate.binding, statement, nil, c); err == nil {
				t.Fatal("unproved placement admitted")
			}
		})
	}
	bound, err := BindGroupedSelectionConstraints(t.Context(), p.candidate.binding, sql, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	for name, change := range map[string]func(*Plan){
		"changed_value": func(p *Plan) {
			p.candidate.parameters = append([]Parameter(nil), p.candidate.parameters...)
			p.candidate.parameters[len(p.candidate.parameters)-1].Value = "OTHER_PRIVATE_GROUP"
		},
		"nullable_lane_instead_of_spine": func(p *Plan) {
			p.candidate.statement = strings.ReplaceAll(p.candidate.statement, `"keys"."region"`, `"s"."region"`)
		},
		"missing_predicate": func(p *Plan) {
			position := strings.LastIndex(p.candidate.statement, " WHERE ")
			if position < 0 {
				t.Fatal("missing fixture predicate")
			}
			p.candidate.statement = p.candidate.statement[:position]
		},
		"broadened_predicate": func(p *Plan) { p.candidate.statement += " OR TRUE" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := p
			change(&bad)
			if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
				t.Fatal("private group selection drift accepted")
			}
		})
	}
	for name, change := range map[string]func(*AnalyticalContract){
		"non_grouping_fact_field": func(c *AnalyticalContract) {
			c.GroupSelection.Constraints[0].Dataset = "sales"
			c.GroupSelection.Constraints[0].Column = "status"
		},
		"aggregate_constraint": func(c *AnalyticalContract) { c.GroupSelection.Constraints[0].Aggregation = "count" },
		"empty_selection":      func(c *AnalyticalContract) { c.GroupSelection.Constraints = nil },
		"stale_revision":       func(c *AnalyticalContract) { c.GroupSelection.Constraints[0].SourceRevision++ },
	} {
		t.Run(name, func(t *testing.T) {
			bad := c
			copy := *c.GroupSelection
			copy.Constraints = append([]BusinessConstraint(nil), copy.Constraints...)
			bad.GroupSelection = &copy
			change(&bad)
			if _, err := BindGroupedSelectionConstraints(t.Context(), p.candidate.binding, sql, nil, bad); err == nil {
				t.Fatal("unowned group contract admitted")
			}
		})
	}
}

func TestSQLRecoveryGroupedSelectionNullAndIntent(t *testing.T) {
	p, c, sql := groupSelectionFixture(t, AnalyticalGroupDomainRaw, false)
	constraint := &c.GroupSelection.Constraints[0]
	constraint.Null = true
	constraint.Nulls = "only"
	constraint.Value = ""
	bound, err := BindGroupedSelectionConstraints(t.Context(), p.candidate.binding, sql, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.Parameters) != 0 || len(bound.Receipt.Bindings) != 1 || len(bound.Receipt.Bindings[0].Parameters) != 0 {
		t.Fatal("NULL group invented scalar")
	}
	p.candidate.statement = bound.SQL
	p.candidate.parameters = bound.Parameters
	if _, err := CheckAnalyticalPlan(t.Context(), p, c); err != nil {
		t.Fatal("NULL group proof", err)
	}
	// An unreviewed outer limit still cannot borrow the predicate proof.
	if _, err := BindGroupedSelectionConstraints(t.Context(), p.candidate.binding, sql+" LIMIT 1", nil, c); err == nil {
		t.Fatal("unreviewed limit admitted")
	}
}
