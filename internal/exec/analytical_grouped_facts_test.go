package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func groupedFactConstraint(dataset, column, value string) BusinessConstraint {
	return BusinessConstraint{Resolution: Hash("fact-" + dataset + "/" + column), Dataset: dataset, Column: column, SourceRevision: 1, Kind: "text", Operator: "eq", Nulls: "exclude", Value: value}
}

func groupedFactsFixture(t *testing.T, domain string, periods, selection bool) (Plan, AnalyticalContract, string) {
	t.Helper()
	p, c, sql := groupedOwnedFixture(t, domain, false)
	for i := range p.candidate.binding.Relations {
		r := &p.candidate.binding.Relations[i]
		r.Columns = append(r.Columns, Column{Name: "tag", NativeType: "text", Category: "text", Nullable: true, Safe: true}, Column{Name: "channel", NativeType: "text", Category: "text", Nullable: true, Safe: true})
	}
	c.Binding = Hash(p.candidate.binding)
	c.Version = AnalyticalGroupedFactsVersion
	if !periods {
		for i := range c.GroupedPopulations.Lanes {
			c.GroupedPopulations.Lanes[i].QueryPopulation = nil
		}
	}
	if selection {
		var err error
		c.GroupSelection, err = NewAnalyticalQueryPopulation(t.Context(), p.candidate.binding, "dimensions", []BusinessConstraint{groupedFactConstraint("dimensions", "region", "PRIVATE_SELECTED_REGION")})
		if err != nil {
			t.Fatal("selection fixture", err)
		}
	}
	return p, c, sql
}

func groupedFactsSet(t *testing.T, p Plan, c *AnalyticalContract, dataset string, constraints ...BusinessConstraint) {
	t.Helper()
	population, err := NewAnalyticalQueryPopulation(t.Context(), p.candidate.binding, dataset, constraints)
	if err != nil {
		t.Fatal("fact fixture", err)
	}
	for i := range c.GroupedPopulations.Lanes {
		if c.GroupedPopulations.Lanes[i].Dataset == dataset {
			c.GroupedPopulations.Lanes[i].FactPopulation = population
			return
		}
	}
	t.Fatal("missing fact fixture lane", dataset)
}

func groupedFactsClone(t *testing.T, c AnalyticalContract) AnalyticalContract {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var out AnalyticalContract
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func groupedFactsReplace(t *testing.T, sql, old, replacement string) string {
	t.Helper()
	if strings.Count(sql, old) != 1 {
		t.Fatalf("mutation must name one fixture location: %q", old)
	}
	return strings.Replace(sql, old, replacement, 1)
}

// Inspect the flat fixture text independently of the binder's body extractor.
func groupedFactsBody(t *testing.T, sql, alias string) (string, int, int) {
	t.Helper()
	needle := alias + " AS ("
	start := strings.Index(sql, needle)
	if start < 0 {
		t.Fatal("missing fixture CTE", alias)
	}
	start += len(needle)
	depth := 1
	quoted := false
	for i := start; i < len(sql); i++ {
		if sql[i] == '\'' {
			if quoted && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
		}
		if quoted {
			continue
		}
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return sql[start:i], start, i
			}
		}
	}
	t.Fatal("unterminated fixture CTE", alias)
	return "", 0, 0
}

func groupedFactsReceipt(t *testing.T, p Plan, c AnalyticalContract, bound BusinessBoundQuery) {
	t.Helper()
	if bound.Receipt.SchemaVersion != 5 || bound.Receipt.PopulationPolicy != AnalyticalGroupedFactPolicy || bound.Receipt.SourceBinding != Hash(p.candidate.binding) || bound.Receipt.Constraints != Hash([]any{c.GroupedPopulations, c.GroupSelection}) || bound.Receipt.Statement != Hash([]any{bound.SQL, bound.Parameters}) || bound.Receipt.Validation != nil {
		t.Fatal("incomplete private binding evidence")
	}
	type expectedEffect struct {
		owner      string
		constraint BusinessConstraint
	}
	want := map[string]expectedEffect{}
	for _, lane := range c.GroupedPopulations.Lanes {
		for _, population := range []*AnalyticalQueryPopulation{lane.QueryPopulation, lane.FactPopulation} {
			if population == nil {
				continue
			}
			for _, constraint := range population.Constraints {
				want[constraint.Resolution] = expectedEffect{lane.Dataset, constraint}
			}
		}
	}
	if c.GroupSelection != nil {
		for _, constraint := range c.GroupSelection.Constraints {
			want[constraint.Resolution] = expectedEffect{"", constraint}
		}
	}
	if len(bound.Receipt.Bindings) != len(want) {
		t.Fatal("missing or duplicated effects")
	}
	seen := map[string]bool{}
	used := make([]bool, len(bound.Parameters))
	for _, effect := range bound.Receipt.Bindings {
		expected, ok := want[effect.Resolution]
		constraint := expected.constraint
		if !ok || seen[effect.Resolution] || effect.Population != expected.owner || effect.Dataset != constraint.Dataset || effect.Column != constraint.Column || effect.Aggregation != constraint.Aggregation || effect.Kind != constraint.Kind || effect.Operator != constraint.Operator || effect.Nulls != constraint.Nulls {
			t.Fatal("binding lost exact effect ownership")
		}
		seen[effect.Resolution] = true
		var values []string
		if !constraint.Null {
			values = append(values, constraint.Value)
			if constraint.Operator == "range" {
				values = append(values, constraint.Upper)
			}
		}
		if len(effect.Parameters) != len(values) {
			t.Fatal("wrong effect parameter arity")
		}
		for i, position := range effect.Parameters {
			if position < 1 || position > len(used) || used[position-1] || bound.Parameters[position-1].Value != values[i] || !bound.Parameters[position-1].Valid() {
				t.Fatal("parameter custody mismatch")
			}
			used[position-1] = true
		}
	}
	for _, present := range used {
		if !present {
			t.Fatal("unowned parameter")
		}
	}
	raw, err := json.Marshal(bound.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range want {
		for _, value := range []string{expected.constraint.Value, expected.constraint.Upper} {
			if len(value) > 3 && (strings.Contains(string(raw), value) || strings.Contains(bound.SQL, value) || strings.Contains(fmt.Sprintf("%v %#v", bound, bound), value)) {
				t.Fatal("private value exposed outside parameters")
			}
		}
	}
}

func TestGroupedFactsProofAndCustody(t *testing.T) {
	for _, domain := range []string{AnalyticalGroupDomainRaw, AnalyticalGroupDomainQualifying} {
		for _, tc := range []struct {
			name                               string
			periods, selection, both, multiple bool
		}{
			{name: "fact_only"},
			{name: "two_predicates_same_fact", multiple: true},
			{name: "both_facts", both: true},
			{name: "periods", periods: true},
			{name: "final_selection", selection: true},
			{name: "periods_facts_selection", periods: true, selection: true, both: true, multiple: true},
		} {
			t.Run(domain+"/"+tc.name, func(t *testing.T) {
				p, c, sql := groupedFactsFixture(t, domain, tc.periods, tc.selection)
				constraints := []BusinessConstraint{groupedFactConstraint("sales", "tag", "PRIVATE_SALES_TAG")}
				if tc.multiple {
					constraints = append(constraints, groupedFactConstraint("sales", "channel", "PRIVATE_SALES_CHANNEL"))
				}
				groupedFactsSet(t, p, &c, "sales", constraints...)
				if tc.both {
					groupedFactsSet(t, p, &c, "items", groupedFactConstraint("items", "tag", "PRIVATE_ITEMS_TAG"))
				}
				before := Hash(c)
				bindingBefore := Hash(p.candidate.binding)
				bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
				if err != nil {
					t.Fatal("bind", err)
				}
				groupedFactsReceipt(t, p, c, bound)
				p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
				proof, err := CheckAnalyticalPlan(t.Context(), p, c)
				if err != nil {
					t.Fatal("proof", err)
				}
				if proof.Version != AnalyticalGroupedFactsVersion || proof.Contract != before || !strings.HasSuffix(proof.Scope, ";independent_filtered_grouped_populations") || len(proof.Outputs) != 1 || proof.Outputs[0].Column != 1 || !AnalyticalOutputsValid(proof) || Hash(c) != before || Hash(p.candidate.binding) != bindingBefore {
					t.Fatal("wrong proof, output ordinal, or caller mutation")
				}
				sales, _, _ := groupedFactsBody(t, bound.SQL, "s")
				items, _, _ := groupedFactsBody(t, bound.SQL, "i")
				if !strings.Contains(sales, `"s"."tag"`) || !tc.both && strings.Contains(items, `"i"."tag"`) || strings.Contains(sales, `"i"."tag"`) || strings.Contains(items, `"s"."tag"`) {
					t.Fatal("fact restriction moved across lanes")
				}
				if !tc.periods && !tc.both {
					originalItems, _, _ := groupedFactsBody(t, sql, "i")
					if items != originalItems {
						t.Fatal("unrestricted lane changed")
					}
				}
				keys, _, _ := groupedFactsBody(t, bound.SQL, "keys")
				originalKeys, _, _ := groupedFactsBody(t, sql, "keys")
				if keys != originalKeys {
					t.Fatal("group spine rewritten")
				}
				again, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
				if err != nil || again.SQL != bound.SQL || !reflect.DeepEqual(again.Parameters, bound.Parameters) || Hash(again.Receipt) != Hash(bound.Receipt) {
					t.Fatal("nondeterministic private binding", err)
				}
			})
		}
	}
}

func TestGroupedFactsNullPredicates(t *testing.T) {
	for _, domain := range []string{AnalyticalGroupDomainRaw, AnalyticalGroupDomainQualifying} {
		for _, nulls := range []string{"include", "only"} {
			t.Run(domain+"/"+nulls, func(t *testing.T) {
				p, c, sql := groupedFactsFixture(t, domain, false, false)
				constraint := groupedFactConstraint("sales", "tag", "PRIVATE_NULLABLE_TAG")
				constraint.Nulls = nulls
				if nulls == "only" {
					constraint.Null, constraint.Value = true, ""
				}
				groupedFactsSet(t, p, &c, "sales", constraint)
				bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
				if err != nil {
					t.Fatal("bind NULL policy", err)
				}
				groupedFactsReceipt(t, p, c, bound)
				p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
				if _, err := CheckAnalyticalPlan(t.Context(), p, c); err != nil {
					t.Fatal("prove NULL policy", err)
				}
				if nulls == "only" && len(bound.Parameters) != 0 || nulls == "include" && len(bound.Parameters) != 1 {
					t.Fatal("NULL policy invented or omitted scalar")
				}
				p.candidate.statement = groupedFactsReplace(t, bound.SQL, `"s"."tag" IS NULL`, `"s"."tag" IS NOT NULL`)
				if _, err := CheckAnalyticalPlan(t.Context(), p, c); err == nil {
					t.Fatal("changed NULL population acquired proof")
				}
			})
		}
	}
}

func TestGroupedFactsRejectRetainedVersionsAndIncompleteContracts(t *testing.T) {
	p, c, sql := groupedFactsFixture(t, AnalyticalGroupDomainRaw, false, true)
	groupedFactsSet(t, p, &c, "sales", groupedFactConstraint("sales", "tag", "PRIVATE_SALES_TAG"))
	bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	for _, version := range []string{AnalyticalVersion, AnalyticalGrainVersion, AnalyticalCalendarVersion, AnalyticalQueryPopulationVersion, AnalyticalGroupingVersion, AnalyticalIntentVersion, AnalyticalGroupedPopulationsVersion, AnalyticalGroupedProgramsVersion, AnalyticalScopedPopulationsVersion, AnalyticalGroupedOwnedPopulationsVersion, AnalyticalGroupedSelectionVersion} {
		t.Run(version, func(t *testing.T) {
			bad := groupedFactsClone(t, c)
			bad.Version = version
			if _, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, bad); err == nil {
				t.Fatal("old version bound fact population")
			}
			if _, err := CheckAnalyticalPlan(t.Context(), p, bad); err == nil {
				t.Fatal("old version borrowed fact proof")
			}
		})
	}
	for name, mutate := range map[string]func(*AnalyticalContract){
		"no_fact_restriction":      func(c *AnalyticalContract) { c.GroupedPopulations.Lanes[1].FactPopulation = nil },
		"empty_fact_restriction":   func(c *AnalyticalContract) { c.GroupedPopulations.Lanes[1].FactPopulation.Constraints = nil },
		"unrecognized_fact_policy": func(c *AnalyticalContract) { c.GroupedPopulations.Lanes[1].FactPopulation.Policy = "unreviewed" },
		"missing_domain":           func(c *AnalyticalContract) { c.GroupedPopulations.Lanes[1].Domain = "" },
		"aggregate_predicate": func(c *AnalyticalContract) {
			c.GroupedPopulations.Lanes[1].FactPopulation.Constraints[0].Aggregation = "count"
		},
		"stale_revision": func(c *AnalyticalContract) {
			c.GroupedPopulations.Lanes[1].FactPopulation.Constraints[0].SourceRevision++
		},
		"wrong_lane": func(c *AnalyticalContract) {
			c.GroupedPopulations.Lanes[0].FactPopulation, c.GroupedPopulations.Lanes[1].FactPopulation = c.GroupedPopulations.Lanes[1].FactPopulation, nil
		},
		"shared_join_coordinate": func(c *AnalyticalContract) {
			c.GroupedPopulations.Lanes[1].FactPopulation.Constraints[0].Dataset = "dimensions"
		},
		"selected_grain_coordinate": func(c *AnalyticalContract) {
			c.GroupedPopulations.Lanes[1].FactPopulation.Constraints[0].Dataset, c.GroupedPopulations.Lanes[1].FactPopulation.Constraints[0].Column = "dimensions", "region"
		},
		"outer_population":        func(c *AnalyticalContract) { c.QueryPopulation = c.GroupedPopulations.Lanes[1].FactPopulation },
		"fact_in_final_selection": func(c *AnalyticalContract) { c.GroupSelection = c.GroupedPopulations.Lanes[1].FactPopulation },
		"fact_disguised_as_period": func(c *AnalyticalContract) {
			c.GroupedPopulations.Lanes[1].QueryPopulation = c.GroupedPopulations.Lanes[1].FactPopulation
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := groupedFactsClone(t, c)
			mutate(&bad)
			if _, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, bad); err == nil {
				t.Fatal("unsupported fact contract bound")
			}
			if _, err := CheckAnalyticalPlan(t.Context(), p, bad); err == nil {
				t.Fatal("unsupported fact contract acquired proof")
			}
		})
	}
}

func TestGroupedFactsRejectUnboundAndRelocatedPredicates(t *testing.T) {
	p, c, sql := groupedFactsFixture(t, AnalyticalGroupDomainRaw, false, false)
	groupedFactsSet(t, p, &c, "sales", groupedFactConstraint("sales", "tag", "PRIVATE_SALES_TAG"))
	for name, statement := range map[string]string{
		"model_parameter":  strings.Replace(sql, "s.value-i.value", "s.value-i.value+$1", 1),
		"outer_where":      sql + " WHERE keys.region='north'",
		"spine_filter":     strings.Replace(sql, "SELECT region FROM i)", "SELECT region FROM i WHERE region IS NOT NULL)", 1),
		"extra_fact_where": strings.Replace(sql, "s.id=d.id GROUP BY", "s.id=d.id WHERE s.channel='unrequested' GROUP BY", 1),
		"fact_having":      strings.Replace(sql, "GROUP BY d.region), i", "GROUP BY d.region HAVING sum(s.amount)>0), i", 1),
		"wrapper":          "SELECT w.region,w.net FROM (" + sql + ") w",
		"duplicate_fact":   strings.Replace(sql, "FROM analytics.items i", "FROM analytics.sales i", 1),
		"unreviewed_join":  strings.Replace(sql, "JOIN analytics.dimensions d ON s.id=d.id", "LEFT JOIN analytics.dimensions d ON s.id=d.id", 1),
	} {
		t.Run("unbound/"+name, func(t *testing.T) {
			bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, statement, nil, c)
			if err == nil {
				bad := p
				bad.candidate.statement, bad.candidate.parameters = bound.SQL, bound.Parameters
				if _, proofErr := CheckAnalyticalPlan(t.Context(), bad, c); proofErr == nil {
					t.Fatal("unreviewed statement bound and proved")
				}
			}
		})
	}
	for _, marker := range []string{"$1", "$2"} {
		statement := groupedFactsReplace(t, sql, "s.value-i.value", "s.value-i.value+"+marker)
		if _, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, statement, nil, c); err == nil {
			t.Fatal("unbound model marker captured a private parameter")
		}
	}
	if _, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, []Parameter{{Kind: "text", Value: "PRIVATE_MODEL_VALUE"}}, c); err == nil {
		t.Fatal("model parameter entered private binding")
	}
	bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	sales, start, end := groupedFactsBody(t, bound.SQL, "s")
	where, group := strings.LastIndex(sales, " WHERE "), strings.LastIndex(sales, " GROUP BY ")
	if where < 0 || group <= where {
		t.Fatal("missing owned fixture predicate")
	}
	predicate := strings.TrimSpace(sales[where+len(" WHERE ") : group])
	removed := bound.SQL[:start] + sales[:where] + sales[group:] + bound.SQL[end:]
	for name, statement := range map[string]string{
		"missing_fact":           removed,
		"wrong_fact_column":      groupedFactsReplace(t, bound.SQL, `"s"."tag"`, `"s"."channel"`),
		"joined_coordinate":      groupedFactsReplace(t, bound.SQL, `"s"."tag"`, `"d"."tag"`),
		"outer_relocation":       removed + " WHERE " + strings.ReplaceAll(predicate, `"s"."tag"`, `"keys"."region"`),
		"spine_relocation":       strings.Replace(removed, "SELECT region FROM s UNION", "SELECT region FROM s WHERE "+strings.ReplaceAll(predicate, `"s"."tag"`, "region")+" UNION", 1),
		"wrong_lane":             strings.Replace(removed, "i.sale_id=d.id GROUP BY", "i.sale_id=d.id WHERE "+strings.ReplaceAll(predicate, `"s"."tag"`, `"i"."tag"`)+" GROUP BY", 1),
		"copied_other_lane":      strings.Replace(bound.SQL, "i.sale_id=d.id GROUP BY", "i.sale_id=d.id WHERE "+strings.ReplaceAll(predicate, `"s"."tag"`, `"i"."tag"`)+" GROUP BY", 1),
		"broadened_fact":         strings.Replace(bound.SQL, predicate, "("+predicate+") OR TRUE", 1),
		"extra_fact_filter":      strings.Replace(bound.SQL, predicate, "("+predicate+") AND s.channel='unrequested'", 1),
		"metric_filter_removed":  strings.Replace(bound.SQL, " FILTER (WHERE s.status='paid')", "", 1),
		"outer_extra_filter":     bound.SQL + " WHERE s.value>0",
		"spine_duplicates":       strings.Replace(bound.SQL, " UNION ", " UNION ALL ", 1),
		"alignment_equality":     strings.Replace(bound.SQL, "IS NOT DISTINCT FROM", "=", 1),
		"missing_lane_zero_fill": strings.Replace(bound.SQL, "s.value-i.value", "coalesce(s.value,0)-coalesce(i.value,0)", 1),
	} {
		t.Run("bound/"+name, func(t *testing.T) {
			if statement == bound.SQL {
				t.Fatal("negative did not change fixture")
			}
			bad := p
			bad.candidate.statement, bad.candidate.parameters = statement, bound.Parameters
			if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
				t.Fatal("changed fact placement acquired proof")
			}
		})
	}
	for name, parameters := range map[string][]Parameter{
		"missing_value": nil,
		"changed_value": {{Kind: "text", Value: "PRIVATE_CHANGED_TAG"}},
		"changed_kind":  {{Kind: "number", Value: "7"}},
	} {
		t.Run("parameters/"+name, func(t *testing.T) {
			bad := p
			bad.candidate.statement, bad.candidate.parameters = bound.SQL, parameters
			if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
				t.Fatal("changed parameter vector acquired proof")
			}
		})
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := BindGroupedFactConstraints(cancelled, p.candidate.binding, sql, nil, c); err == nil {
		t.Fatal("cancelled binding succeeded")
	}
}

func TestGroupedFactsOwnershipRequiresExclusivePrimaryFact(t *testing.T) {
	for _, name := range []string{"fact_joined_by_other_lane", "parent_joined_by_only_one_lane"} {
		t.Run(name, func(t *testing.T) {
			p, c, sql := groupedFactsFixture(t, AnalyticalGroupDomainRaw, false, false)
			target := "sales"
			if name == "fact_joined_by_other_lane" {
				p.candidate.binding.Relations[0].UniqueKeys = [][]string{{"id"}}
				c.GroupedPopulations.Lanes[0].Joins = []AnalyticalJoin{
					{Left: "items", Right: "sales", Type: "inner", LeftColumns: []string{"sale_id"}, RightColumns: []string{"id"}},
					{Left: "sales", Right: "dimensions", Type: "inner", LeftColumns: []string{"id"}, RightColumns: []string{"id"}},
				}
				sql = groupedFactsReplace(t, sql, "FROM analytics.items i JOIN analytics.dimensions d ON i.sale_id=d.id", "FROM analytics.items i JOIN analytics.sales parent ON i.sale_id=parent.id JOIN analytics.dimensions d ON parent.id=d.id")
			} else {
				target = "segments"
				p.candidate.binding.Relations = append(p.candidate.binding.Relations, Relation{ID: target, Schema: "analytics", Name: target, Columns: []Column{{Name: "id", NativeType: "int4", Category: "numeric", Safe: true}, {Name: "tag", NativeType: "text", Category: "text", Nullable: true, Safe: true}}, UniqueKeys: [][]string{{"id"}}})
				c.GroupedPopulations.Lanes[1].Joins = append(c.GroupedPopulations.Lanes[1].Joins, AnalyticalJoin{Left: "sales", Right: target, Type: "inner", LeftColumns: []string{"id"}, RightColumns: []string{"id"}})
				sql = groupedFactsReplace(t, sql, "ON s.id=d.id GROUP BY", "ON s.id=d.id JOIN analytics.segments seg ON s.id=seg.id GROUP BY")
			}
			c.Binding = Hash(p.candidate.binding)
			c.Version = AnalyticalGroupedProgramsVersion
			p.candidate.statement = sql
			if _, err := CheckAnalyticalPlan(t.Context(), p, c); err != nil {
				t.Fatal("independent retained join baseline must prove", err)
			}
			c.Version = AnalyticalGroupedFactsVersion
			population, err := NewAnalyticalQueryPopulation(t.Context(), p.candidate.binding, target, []BusinessConstraint{groupedFactConstraint(target, "tag", "PRIVATE_AMBIGUOUS_TAG")})
			if err != nil {
				t.Fatal(err)
			}
			c.GroupedPopulations.Lanes[1].FactPopulation = population
			if _, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c); err == nil {
				t.Fatal("join membership created primary-fact ownership")
			}
			if _, err := CheckAnalyticalPlan(t.Context(), p, c); err == nil {
				t.Fatal("ambiguous ownership acquired proof")
			}
		})
	}
}

func TestGroupedFactsPreserveReviewedJoins(t *testing.T) {
	for _, restrictedLeft := range []bool{false, true} {
		t.Run(fmt.Sprintf("restricted_left_%t", restrictedLeft), func(t *testing.T) {
			p, c, sql := groupedFactsFixture(t, AnalyticalGroupDomainRaw, false, false)
			lane, from := 0, "FROM analytics.items i JOIN"
			if restrictedLeft {
				lane, from = 1, "FROM analytics.sales s JOIN"
			}
			c.GroupedPopulations.Lanes[lane].Joins[0].Type = "left"
			sql = groupedFactsReplace(t, sql, from, strings.Replace(from, " JOIN", " LEFT JOIN", 1))
			retained := groupedFactsClone(t, c)
			retained.Version = AnalyticalGroupedProgramsVersion
			p.candidate.statement = sql
			if _, err := CheckAnalyticalPlan(t.Context(), p, retained); err != nil {
				t.Fatal("reviewed retained LEFT join must prove", err)
			}
			groupedFactsSet(t, p, &c, "sales", groupedFactConstraint("sales", "tag", "PRIVATE_SALES_TAG"))
			bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
			if restrictedLeft {
				if err == nil {
					t.Fatal("unsupported bound LEFT join was rewritten or admitted")
				}
				if _, err := CheckAnalyticalPlan(t.Context(), p, c); err == nil {
					t.Fatal("unsupported bound LEFT join acquired proof")
				}
				return
			}
			if err != nil {
				t.Fatal("unrestricted LEFT join blocked independent fact restriction", err)
			}
			original, _, _ := groupedFactsBody(t, sql, "i")
			actual, _, _ := groupedFactsBody(t, bound.SQL, "i")
			if actual != original {
				t.Fatal("unrestricted reviewed join changed")
			}
			p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
			if _, err := CheckAnalyticalPlan(t.Context(), p, c); err != nil {
				t.Fatal("prove independent fact restriction", err)
			}
		})
	}
}

func TestGroupedFactsCannotWidenRetainedGroupedContracts(t *testing.T) {
	for _, version := range []string{AnalyticalGroupedProgramsVersion, AnalyticalGroupedOwnedPopulationsVersion, AnalyticalGroupedSelectionVersion} {
		t.Run(version, func(t *testing.T) {
			periods := version == AnalyticalGroupedOwnedPopulationsVersion
			selection := version == AnalyticalGroupedSelectionVersion
			p, c, sql := groupedFactsFixture(t, AnalyticalGroupDomainRaw, periods, selection)
			c.Version = version
			if err := ValidateAnalyticalGroupedPopulations(c, p.candidate.binding); err != nil {
				t.Fatal("retained grouped contract must be valid before new field", err)
			}
			groupedFactsSet(t, p, &c, "sales", groupedFactConstraint("sales", "tag", "PRIVATE_SALES_TAG"))
			if err := ValidateAnalyticalGroupedPopulations(c, p.candidate.binding); err == nil {
				t.Fatal("new field widened retained contract")
			}
			var err error
			switch version {
			case AnalyticalGroupedOwnedPopulationsVersion:
				_, err = BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, sql, nil, c)
			case AnalyticalGroupedSelectionVersion:
				_, err = BindGroupedSelectionConstraints(t.Context(), p.candidate.binding, sql, nil, c)
			default:
				_, err = CheckAnalyticalPlan(t.Context(), p, c)
			}
			if err == nil {
				t.Fatal("retained entry point ignored new fact population")
			}
		})
	}
}

func TestGroupedFactsMixedPopulationAndSelectionPlacement(t *testing.T) {
	p, c, sql := groupedFactsFixture(t, AnalyticalGroupDomainQualifying, true, true)
	groupedFactsSet(t, p, &c, "sales", groupedFactConstraint("sales", "tag", "PRIVATE_SALES_TAG"))
	groupedFactsSet(t, p, &c, "items", groupedFactConstraint("items", "tag", "PRIVATE_ITEMS_TAG"))
	bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	if _, err := CheckAnalyticalPlan(t.Context(), p, c); err != nil {
		t.Fatal("mixed baseline", err)
	}
	sales, start, end := groupedFactsBody(t, bound.SQL, "s")
	where := strings.LastIndex(sales, " WHERE ")
	if where < 0 {
		t.Fatal("missing qualifying lane predicate")
	}
	removedDomain := sales[:where] + strings.Replace(sales[where:], "s.status='paid'", "TRUE", 1)
	for name, statement := range map[string]string{
		"period_to_joined_parent":    strings.ReplaceAll(bound.SQL, `"s"."occurred"`, `"d"."occurred"`),
		"selection_to_nullable_lane": strings.ReplaceAll(bound.SQL, `"keys"."region"`, `"s"."region"`),
		"qualifying_domain_removed":  bound.SQL[:start] + removedDomain + bound.SQL[end:],
		"missing_final_selection":    bound.SQL[:strings.LastIndex(bound.SQL, " WHERE ")],
	} {
		t.Run(name, func(t *testing.T) {
			if statement == bound.SQL {
				t.Fatal("negative did not change fixture")
			}
			bad := p
			bad.candidate.statement = statement
			if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
				t.Fatal("mixed placement drift acquired proof")
			}
		})
	}
	for name, mutate := range map[string]func(*Binding){
		"source":   func(b *Binding) { b.Source = "another_source" },
		"context":  func(b *Binding) { b.Context = "source:another_context" },
		"revision": func(b *Binding) { b.Revision++ },
	} {
		t.Run(name+"_drift", func(t *testing.T) {
			bad := p
			mutate(&bad.candidate.binding)
			if _, err := BindGroupedFactConstraints(t.Context(), bad.candidate.binding, sql, nil, c); err == nil {
				t.Fatal("binding drift admitted")
			}
			if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
				t.Fatal("binding drift acquired proof")
			}
		})
	}
	byOwner := map[string]int{}
	for _, effect := range bound.Receipt.Bindings {
		if effect.Column == "tag" && len(effect.Parameters) == 1 {
			byOwner[effect.Population] = effect.Parameters[0] - 1
		}
	}
	if len(byOwner) != 2 {
		t.Fatal("missing independent fact custody")
	}
	bad := p
	bad.candidate.parameters = append([]Parameter(nil), bound.Parameters...)
	left, right := byOwner["sales"], byOwner["items"]
	bad.candidate.parameters[left], bad.candidate.parameters[right] = bad.candidate.parameters[right], bad.candidate.parameters[left]
	if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
		t.Fatal("valid private values swapped between facts")
	}
}

func TestGroupedFactsKeepExactNumericRangeAndSharedPeriods(t *testing.T) {
	for _, domain := range []string{AnalyticalGroupDomainRaw, AnalyticalGroupDomainQualifying} {
		t.Run(domain, func(t *testing.T) {
			p, c, sql := groupedFactsFixture(t, domain, true, true)
			for i := range c.GroupedPopulations.Lanes {
				lane := &c.GroupedPopulations.Lanes[i]
				constraint := lane.QueryPopulation.Constraints[0]
				constraint.Dataset = "dimensions"
				var err error
				lane.QueryPopulation, err = NewAnalyticalQueryPopulation(t.Context(), p.candidate.binding, "dimensions", []BusinessConstraint{constraint})
				if err != nil {
					t.Fatal("reviewed shared period fixture", err)
				}
			}
			number := businessFixtureConstraint()
			number.Operator, number.Bounds, number.Upper = "range", "[)", "9007199254740994.375"
			groupedFactsSet(t, p, &c, "sales", number, groupedFactConstraint("sales", "tag", "PRIVATE_SALES_TAG"))
			bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
			if err != nil {
				t.Fatal("bind exact numeric fact and shared periods", err)
			}
			groupedFactsReceipt(t, p, c, bound)
			p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
			if _, err := CheckAnalyticalPlan(t.Context(), p, c); err != nil {
				t.Fatal("prove exact numeric fact and shared periods", err)
			}
			for _, effect := range bound.Receipt.Bindings {
				if effect.Resolution != number.Resolution {
					continue
				}
				bad := p
				bad.candidate.parameters = append([]Parameter(nil), bound.Parameters...)
				bad.candidate.parameters[effect.Parameters[0]-1].Value = "9007199254740993.126"
				if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
					t.Fatal("exact decimal fact threshold rounded or ignored")
				}
			}
		})
	}
}

func TestGroupedFactsRejectResolutionCapture(t *testing.T) {
	for _, periods := range []bool{false, true} {
		p, c, sql := groupedFactsFixture(t, AnalyticalGroupDomainRaw, periods, false)
		one := groupedFactConstraint("sales", "tag", "CURRENT_SALES")
		two := groupedFactConstraint("items", "tag", "CURRENT_ITEMS")
		two.Resolution = one.Resolution
		if periods {
			two.Resolution = c.GroupedPopulations.Lanes[0].QueryPopulation.Constraints[0].Resolution
		}
		groupedFactsSet(t, p, &c, "sales", one)
		groupedFactsSet(t, p, &c, "items", two)
		if _, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c); err == nil {
			t.Fatal("fact resolution captured another population")
		}
	}
}
