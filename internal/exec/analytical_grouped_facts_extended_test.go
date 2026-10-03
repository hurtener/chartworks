package exec

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func groupedFactsExtendedFixture(t *testing.T, count int, grain, domain string) (Plan, AnalyticalContract, string) {
	t.Helper()
	p, c, sql := groupedFactsFixture(t, domain, false, false)
	for _, id := range []string{"credits", "taxes"}[:count-2] {
		relation := p.candidate.binding.Relations[1]
		relation.ID, relation.Name = id, id
		p.candidate.binding.Relations = append(p.candidate.binding.Relations, relation)
		c.GroupedPopulations.Lanes = append(c.GroupedPopulations.Lanes, AnalyticalGroupedLane{Dataset: id, Domain: domain, Joins: []AnalyticalJoin{{Left: id, Right: "dimensions", Type: "inner", LeftColumns: []string{"sale_id"}, RightColumns: []string{"id"}}}})
		c.Metrics[0].Expression = AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{c.Metrics[0].Expression, analyticalMeasure("sum", id+"/quantity", AnalyticalFilter{Column: id + "/status", Kind: "eq", Values: []string{"paid"}})}}
		where := ""
		if domain == AnalyticalGroupDomainQualifying {
			where = " WHERE f.status='paid'"
		}
		lane := id + ` AS (SELECT d.region AS region,sum(f.quantity) FILTER (WHERE f.status='paid') AS value FROM analytics.` + id + ` f JOIN analytics.dimensions d ON f.sale_id=d.id` + where + ` GROUP BY d.region), `
		sql = strings.Replace(sql, "keys AS (", lane+"keys AS (", 1)
		sql = strings.Replace(sql, ") SELECT keys.region", " UNION SELECT region FROM "+id+") SELECT keys.region", 1)
		sql = strings.Replace(sql, " AS net FROM", "-"+id+".value AS net FROM", 1)
		sql += " LEFT JOIN " + id + " ON keys.region IS NOT DISTINCT FROM " + id + ".region"
	}
	sort.Slice(c.GroupedPopulations.Lanes, func(i, j int) bool {
		return c.GroupedPopulations.Lanes[i].Dataset < c.GroupedPopulations.Lanes[j].Dataset
	})
	if grain == "composite" {
		p.candidate.binding.Relations[2].Columns = append(p.candidate.binding.Relations[2].Columns, Column{Name: "segment", NativeType: "text", Category: "text", Nullable: true, Safe: true})
		c.Grain.Columns = append(c.Grain.Columns, "dimensions/segment")
		c.Grain.Dimensions = append(c.Grain.Dimensions, "segment")
		sql = strings.ReplaceAll(sql, "d.region AS region,", "d.region AS region,d.segment AS segment,")
		sql = strings.ReplaceAll(sql, "GROUP BY d.region)", "GROUP BY d.region,d.segment)")
		sql = strings.ReplaceAll(sql, "SELECT region FROM", "SELECT region,segment FROM")
		sql = strings.Replace(sql, "SELECT keys.region,", "SELECT keys.region,keys.segment,", 1)
		for _, alias := range []string{"s", "i", "credits", "taxes"}[:count] {
			sql = strings.ReplaceAll(sql, "keys.region IS NOT DISTINCT FROM "+alias+".region", "keys.region IS NOT DISTINCT FROM "+alias+".region AND keys.segment IS NOT DISTINCT FROM "+alias+".segment")
		}
	} else {
		c.Grain.Columns = nil
		c.Grain.Buckets = []AnalyticalBucket{{Column: "dimensions/occurred", Grain: "month", Calendar: "gregorian", Timezone: "America/New_York"}}
		c.Grain.Dimensions = []string{"shared_calendar"}
		sql = strings.ReplaceAll(strings.ReplaceAll(sql, "d.region AS region", "date_trunc('month',d.occurred,'America/New_York') AS region"), "GROUP BY d.region", "GROUP BY 1")
	}
	c.Binding = Hash(p.candidate.binding)
	for _, lane := range c.GroupedPopulations.Lanes {
		groupedFactsSet(t, p, &c, lane.Dataset, groupedFactConstraint(lane.Dataset, "tag", "PRIVATE_"+strings.ToUpper(lane.Dataset)))
	}
	return p, c, sql
}

func TestGroupedFactsExtendedNativeMatrix(t *testing.T) {
	for _, count := range []int{3, 4} {
		for _, grain := range []string{"composite", "calendar"} {
			for _, domain := range []string{AnalyticalGroupDomainRaw, AnalyticalGroupDomainQualifying} {
				t.Run(fmt.Sprintf("lanes_%d_%s_%s", count, grain, domain), func(t *testing.T) {
					p, c, base := groupedFactsExtendedFixture(t, count, grain, domain)
					bound, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, base, nil, c)
					if err != nil {
						t.Fatal("extended bind", err)
					}
					groupedFactsReceipt(t, p, c, bound)
					p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
					receipt, err := CheckAnalyticalPlan(t.Context(), p, c)
					if err != nil {
						t.Fatal("extended native proof", err)
					}
					ordinal := 1
					if grain == "composite" {
						ordinal = 2
					}
					if len(receipt.Outputs) != 1 || receipt.Outputs[0].Column != ordinal || receipt.Contract != Hash(c) || len(bound.Receipt.Bindings) != count {
						t.Fatal("extended output or receipt custody")
					}
					keyNames := "region"
					if grain == "composite" {
						keyNames += ",segment"
					}
					mutations := map[string]string{
						"spine_missing_fact": groupedFactsReplace(t, bound.SQL, "SELECT "+keyNames+" FROM credits", "SELECT "+keyNames+" FROM s"),
						"union_all":          strings.Replace(bound.SQL, " UNION SELECT ", " UNION ALL SELECT ", 1),
						"wrong_fact_origin":  strings.Replace(bound.SQL, `"s"."tag"`, `"d"."tag"`, 1),
						"ordinary_equality":  strings.Replace(bound.SQL, "IS NOT DISTINCT FROM", "=", 1),
						"outer_filter":       bound.SQL + " WHERE credits.value>0",
					}
					if grain == "composite" {
						mutations["spine_reordered_key"] = groupedFactsReplace(t, bound.SQL, "SELECT region,segment FROM credits", "SELECT segment,region FROM credits")
						mutations["incomplete_last_alignment"] = groupedFactsReplace(t, bound.SQL, " AND keys.segment IS NOT DISTINCT FROM credits.segment", "")
						mutations["wrong_second_origin"] = strings.Replace(bound.SQL, "d.segment AS segment", "d.region AS segment", 1)
					} else {
						mutations["wrong_calendar_origin"] = strings.Replace(bound.SQL, "d.occurred", "s.occurred", 1)
						mutations["wrong_calendar_zone"] = strings.Replace(bound.SQL, "America/New_York", "UTC", 1)
						mutations["wrong_calendar_unit"] = strings.Replace(bound.SQL, "'month'", "'day'", 1)
					}
					for name, sql := range mutations {
						t.Run(name, func(t *testing.T) {
							if sql == bound.SQL {
								t.Fatal("vacuous mutation")
							}
							bad := p
							bad.candidate.statement = sql
							if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
								t.Fatal("changed population/grain acquired native proof")
							}
						})
					}
					bad := p
					bad.candidate.parameters = append([]Parameter(nil), bound.Parameters...)
					bad.candidate.parameters[0], bad.candidate.parameters[1] = bad.candidate.parameters[1], bad.candidate.parameters[0]
					if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
						t.Fatal("cross-population private values swapped")
					}

				})
			}
		}
	}
}

func TestGroupedFactsExtendedOwnershipBoundaries(t *testing.T) {
	p, c, base := groupedFactsExtendedFixture(t, 4, "calendar", AnalyticalGroupDomainRaw)
	// Four independently selected facts do not grant any scope to their common
	// joined dimension, even though every lane reaches the same calendar field.
	_, err := AnalyticalGroupedFactOwner(c, groupedFactConstraint("dimensions", "tag", "PRIVATE_SHARED"))
	var failure *AnalyticalError
	if !errors.As(err, &failure) || failure.Code != "analytical_fact_predicate_unsupported" || !failure.Unsupported {
		t.Fatal("shared dimension did not retain typed refusal", err)
	}
	bad := groupedFactsClone(t, c)
	bad.GroupedPopulations.Lanes[0].Joins[0].Type = "left"
	if _, err := BindGroupedFactConstraints(t.Context(), p.candidate.binding, strings.Replace(base, "JOIN analytics.dimensions", "LEFT JOIN analytics.dimensions", 1), nil, bad); !errors.As(err, &failure) || failure.Code != "analytical_fact_predicate_unsupported" || !failure.Unsupported {
		t.Fatal("affected LEFT did not retain typed refusal", err)
	}
}
