package exec

import (
	"context"
	"sort"
	"strings"
	"testing"
)

const groupedSQL = `WITH s AS (SELECT d.region AS region, sum(s.amount) AS value FROM analytics.sales s LEFT JOIN analytics.dimensions d ON s.id=d.id GROUP BY d.region), i AS (SELECT d.region AS region, sum(i.quantity) AS value FROM analytics.items i LEFT JOIN analytics.dimensions d ON i.sale_id=d.id GROUP BY d.region), keys AS (SELECT region FROM s UNION SELECT region FROM i) SELECT keys.region, s.value-i.value AS net FROM keys LEFT JOIN s ON keys.region IS NOT DISTINCT FROM s.region LEFT JOIN i ON keys.region IS NOT DISTINCT FROM i.region`

func groupedFixture(t *testing.T, sql string) (Plan, AnalyticalContract) {
	p, c := independentFixture(t, sql)
	p.candidate.binding.Relations = append(p.candidate.binding.Relations, Relation{ID: "dimensions", Schema: "analytics", Name: "dimensions", Columns: []Column{{Name: "id", NativeType: "int4", Category: "numeric", Safe: true}, {Name: "region", NativeType: "text", Category: "text", Nullable: true, Safe: true}}, UniqueKeys: [][]string{{"id"}}})
	// Match exact discovered join types; the metric quantity remains exact numeric.
	for i := range p.candidate.binding.Relations {
		for j := range p.candidate.binding.Relations[i].Columns {
			col := &p.candidate.binding.Relations[i].Columns[j]
			if col.Name == "id" || col.Name == "sale_id" {
				col.NativeType = "int4"
				col.Category = "numeric"
			}
		}
	}
	c.Version = AnalyticalGroupedPopulationsVersion
	c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy}
	c.QueryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
	c.Populations = nil
	c.Grain = &AnalyticalGrain{Policy: AnalyticalGroupingPolicy, Columns: []string{"dimensions/region"}, Dimensions: []string{"region"}}
	c.GroupedPopulations = &AnalyticalGroupedPopulations{Policy: AnalyticalGroupedPopulationPolicy, Lanes: []AnalyticalGroupedLane{
		{Dataset: "items", Joins: []AnalyticalJoin{{Left: "items", Right: "dimensions", Type: "left", LeftColumns: []string{"sale_id"}, RightColumns: []string{"id"}}}},
		{Dataset: "sales", Joins: []AnalyticalJoin{{Left: "sales", Right: "dimensions", Type: "left", LeftColumns: []string{"id"}, RightColumns: []string{"id"}}}},
	}}
	c.Binding = Hash(p.candidate.binding)
	return p, c
}

func TestSQLRecoveryGroupedPopulationProof(t *testing.T) {
	p, c := groupedFixture(t, groupedSQL)
	before := Hash(c)
	got, err := CheckAnalyticalPlan(context.Background(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != "selected_metric_expression_population_and_grouping;independent_grouped_populations" || Hash(c) != before {
		t.Fatal("wrong or mutable proof", got)
	}
	for _, version := range []string{AnalyticalVersion, AnalyticalGrainVersion, AnalyticalCalendarVersion, AnalyticalQueryPopulationVersion, AnalyticalGroupingVersion, AnalyticalIntentVersion} {
		c.Version = version
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("retained policy widened", version)
		}
	}
}

func TestSQLRecoveryGroupedPopulationAdversaries(t *testing.T) {
	cases := map[string]string{
		"duplicate_spine":        strings.Replace(groupedSQL, " UNION ", " UNION ALL ", 1),
		"intersection_spine":     strings.Replace(groupedSQL, " UNION ", " INTERSECT ", 1),
		"missing_lane_keys":      strings.Replace(groupedSQL, "SELECT region FROM i)", "SELECT region FROM s)", 1),
		"extra_key_filter":       strings.Replace(groupedSQL, "SELECT region FROM i)", "SELECT region FROM i WHERE region IS NOT NULL)", 1),
		"lane_null_filter":       strings.Replace(groupedSQL, "GROUP BY d.region), keys", "WHERE d.region IS NOT NULL GROUP BY d.region), keys", 1),
		"lane_limit":             strings.Replace(groupedSQL, "GROUP BY d.region), i", "GROUP BY d.region LIMIT 1), i", 1),
		"lane_having":            strings.Replace(groupedSQL, "GROUP BY d.region), i", "GROUP BY d.region HAVING sum(s.amount)>0), i", 1),
		"ordinary_equality":      strings.Replace(groupedSQL, "IS NOT DISTINCT FROM", "=", 1),
		"missing_group_coverage": strings.Replace(groupedSQL, "keys LEFT JOIN s", "keys JOIN s", 1),
		"wrong_anchor":           strings.Replace(groupedSQL, "keys.region IS NOT DISTINCT FROM i.region", "s.region IS NOT DISTINCT FROM i.region", 1),
		"wrong_output_key":       strings.Replace(groupedSQL, "SELECT keys.region", "SELECT s.region", 1),
		"zero_fill":              strings.Replace(groupedSQL, "s.value-i.value", "coalesce(s.value,0)-coalesce(i.value,0)", 1),
		"metric_substitution":    strings.Replace(groupedSQL, "sum(i.quantity)", "avg(i.quantity)", 1),
		"outer_filter":           groupedSQL + " WHERE s.value>0",
		"raw_fanout":             strings.Replace(groupedSQL, "FROM analytics.sales s LEFT JOIN", "FROM analytics.sales s JOIN analytics.items x ON s.id=x.sale_id LEFT JOIN", 1),
	}
	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			p, c := groupedFixture(t, sql)
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
				t.Fatal("unproved grouping accepted", sql)
			}
		})
	}
	p, c := groupedFixture(t, groupedSQL)
	p.candidate.binding.Relations[2].UniqueKeys = nil
	c.Binding = Hash(p.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("dimension duplicate risk admitted")
	}
}

func TestSQLRecoveryGroupedCountsStaySourceBound(t *testing.T) {
	sql := strings.ReplaceAll(strings.ReplaceAll(groupedSQL, "sum(s.amount)", "count(*)"), "sum(i.quantity)", "count(*)")
	p, c := groupedFixture(t, sql)
	c.Metrics = analyticalMetrics(AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{analyticalMeasure("count", "id"), analyticalMeasure("count", "items/sale_id")}})
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	p.candidate.statement = strings.Replace(sql, "s.value-i.value", "i.value-s.value", 1)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("different fact counts collapsed")
	}
}

func TestSQLRecoveryGroupedThreeAndFourLanes(t *testing.T) {
	for count := 3; count <= 4; count++ {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			p, c := groupedFixture(t, groupedSQL)
			sql := groupedSQL
			for _, id := range []string{"credits", "taxes"}[:count-2] {
				relation := p.candidate.binding.Relations[1]
				relation.ID = id
				relation.Name = id
				p.candidate.binding.Relations = append(p.candidate.binding.Relations, relation)
				c.GroupedPopulations.Lanes = append(c.GroupedPopulations.Lanes, AnalyticalGroupedLane{Dataset: id, Joins: []AnalyticalJoin{{Left: id, Right: "dimensions", Type: "left", LeftColumns: []string{"sale_id"}, RightColumns: []string{"id"}}}})
				c.Metrics[0].Expression = AnalyticalExpression{Op: "-", Args: []AnalyticalExpression{c.Metrics[0].Expression, analyticalMeasure("sum", id+"/quantity")}}
				lane := id + ` AS (SELECT d.region AS region, sum(f.quantity) AS value FROM analytics.` + id + ` f LEFT JOIN analytics.dimensions d ON f.sale_id=d.id GROUP BY d.region), `
				sql = strings.Replace(sql, "keys AS (", lane+"keys AS (", 1)
				sql = strings.Replace(sql, ") SELECT keys.region", " UNION SELECT region FROM "+id+") SELECT keys.region", 1)
				sql = strings.Replace(sql, " AS net FROM", "-"+id+".value AS net FROM", 1)
				sql += " LEFT JOIN " + id + " ON keys.region IS NOT DISTINCT FROM " + id + ".region"
			}
			sort.Slice(c.GroupedPopulations.Lanes, func(i, j int) bool {
				return c.GroupedPopulations.Lanes[i].Dataset < c.GroupedPopulations.Lanes[j].Dataset
			})
			p.candidate.statement = sql
			c.Binding = Hash(p.candidate.binding)
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
				t.Fatal(sql, err)
			}
			p.candidate.statement = strings.Replace(sql, "UNION SELECT region FROM credits", "UNION ALL SELECT region FROM credits", 1)
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
				t.Fatal("nested spine duplicates accepted")
			}
		})
	}
}

func TestSQLRecoveryGroupedCompositeGrain(t *testing.T) {
	p, c := groupedFixture(t, groupedSQL)
	p.candidate.binding.Relations[2].Columns = append(p.candidate.binding.Relations[2].Columns, Column{Name: "segment", NativeType: "text", Category: "text", Nullable: true, Safe: true})
	c.Grain.Columns = append(c.Grain.Columns, "dimensions/segment")
	c.Grain.Dimensions = append(c.Grain.Dimensions, "segment")
	sql := strings.ReplaceAll(groupedSQL, "d.region AS region,", "d.region AS region,d.segment AS segment,")
	sql = strings.ReplaceAll(sql, "GROUP BY d.region)", "GROUP BY d.region,d.segment)")
	sql = strings.ReplaceAll(sql, "SELECT region FROM", "SELECT region,segment FROM")
	sql = strings.Replace(sql, "SELECT keys.region,", "SELECT keys.region,keys.segment,", 1)
	sql = strings.ReplaceAll(sql, "keys.region IS NOT DISTINCT FROM s.region", "keys.region IS NOT DISTINCT FROM s.region AND keys.segment IS NOT DISTINCT FROM s.segment")
	sql = strings.ReplaceAll(sql, "keys.region IS NOT DISTINCT FROM i.region", "keys.region IS NOT DISTINCT FROM i.region AND keys.segment IS NOT DISTINCT FROM i.segment")
	p.candidate.statement = sql
	c.Binding = Hash(p.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(sql, "SELECT region,segment FROM i", "SELECT segment,region FROM i", 1),
		strings.Replace(sql, " AND keys.segment IS NOT DISTINCT FROM i.segment", "", 1),
		strings.Replace(sql, "keys.segment IS NOT DISTINCT FROM i.segment", "keys.region IS NOT DISTINCT FROM i.region", 1),
	} {
		p.candidate.statement = bad
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("incomplete/duplicated composite alignment accepted")
		}
	}
}
