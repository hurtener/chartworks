package exec

import (
	"context"
	"strings"
	"testing"
)

func groupedCalendarFixture(t *testing.T, instant bool) (Plan, AnalyticalContract) {
	p, c := groupedProgramFixture(t, groupedSQL)
	native, unit, zone, expression := "date", "month", "", "date_trunc('month',d.ordered_at::timestamp)"
	if instant {
		native, unit, zone, expression = "timestamptz", "day", "America/New_York", "date_trunc('day',d.ordered_at,'America/New_York')"
	}
	p.candidate.binding.Relations[2].Columns = append(p.candidate.binding.Relations[2].Columns, Column{Name: "ordered_at", NativeType: native, Category: "temporal", Nullable: true, Safe: true}, Column{Name: "other_at", NativeType: native, Category: "temporal", Nullable: true, Safe: true})
	c.Grain.Columns = nil
	c.Grain.Buckets = []AnalyticalBucket{{Column: "dimensions/ordered_at", Grain: unit, Calendar: "gregorian", Timezone: zone}}
	c.Grain.Dimensions = []string{"order_calendar"}
	p.candidate.statement = strings.ReplaceAll(strings.ReplaceAll(groupedSQL, "d.region AS region", expression+" AS region"), "GROUP BY d.region", "GROUP BY 1")
	c.Binding = Hash(p.candidate.binding)
	return p, c
}
func TestSQLRecoveryGroupedSharedCalendar(t *testing.T) {
	for _, instant := range []bool{false, true} {
		p, c := groupedCalendarFixture(t, instant)
		original := p.candidate.statement
		for _, sql := range []string{original, "SELECT q.region,q.net FROM (" + original + ") q"} {
			p.candidate.statement = sql
			r, err := CheckAnalyticalPlan(context.Background(), p, c)
			if err != nil || r.Scope != strings.ReplaceAll(AnalyticalCalendarScope, "single_base_relation", "independent_grouped_populations") {
				t.Fatal("shared calendar", r, err)
			}
		}
		c.Version = AnalyticalGroupedPopulationsVersion
		p.candidate.statement = original
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("v7 widened into calendar populations")
		}
		c.Version = AnalyticalGroupedProgramsVersion
		for _, bad := range []string{strings.Replace(original, "d.ordered_at", "d.other_at", 1), strings.Replace(original, "IS NOT DISTINCT FROM", "=", 1), strings.Replace(original, "date_trunc('", "date_trunc('year", 1)} {
			p.candidate.statement = bad
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
				t.Fatal("wrong calendar identity admitted")
			}
		}
		if instant {
			p.candidate.statement = strings.Replace(original, "America/New_York", "UTC", 1)
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
				t.Fatal("wrong lane timezone admitted")
			}
		}
	}
}
