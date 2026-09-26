package exec

import (
	"context"
	"errors"
	"testing"
)

func calendarPlan(t *testing.T, sql, native, zone string) (Plan, AnalyticalContract) {
	t.Helper()
	p, c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
	p.candidate.binding.Relations[0].Columns = append(p.candidate.binding.Relations[0].Columns, Column{Name: "event_at", NativeType: native, Category: "temporal", Nullable: true, Safe: true})
	c.Binding, c.Version = Hash(p.candidate.binding), AnalyticalCalendarVersion
	c.Grain = &AnalyticalGrain{Policy: AnalyticalCalendarPolicy, Dimensions: []string{"sales:dimension:event"}, Buckets: []AnalyticalBucket{{Column: "event_at", Grain: "month", Calendar: "gregorian", Timezone: zone}}}
	return p, c
}
func TestSQLRecoveryCalendarNativePartitions(t *testing.T) {
	for _, tc := range []struct{ native, expression, zone string }{
		{"date", "date_trunc('month', event_at::timestamp)", ""},
		{"timestamp without time zone", "date_trunc('month', event_at)::date", ""},
		{"timestamp", "pg_catalog.date_trunc('MONTH', event_at)", ""},
		{"timestamptz", "date_trunc('month', event_at, 'America/New_York')", "America/New_York"},
		{"timestamp with time zone", "date_trunc('month', event_at, 'UTC')", "UTC"},
	} {
		t.Run(tc.native, func(t *testing.T) {
			for _, group := range []string{"1", "period", tc.expression} {
				p, c := calendarPlan(t, "SELECT "+tc.expression+" AS period, sum(amount) FROM analytics.sales GROUP BY "+group, tc.native, tc.zone)
				before := Hash(c)
				r, err := CheckAnalyticalPlan(context.Background(), p, c)
				if err != nil || r == nil || r.Scope != AnalyticalCalendarScope || r.Version != AnalyticalCalendarVersion || Hash(c) != before {
					t.Fatalf("calendar proof: %v %+v", err, r)
				}
			}
		})
	}
}
func TestSQLRecoveryCalendarWrongAndAmbientPartitions(t *testing.T) {
	for _, expression := range []string{
		"date_trunc('year', event_at, 'America/New_York')",
		"date_trunc('month', event_at)",
		"date_trunc('month', event_at, 'UTC')",
		"date_trunc('month', event_at, 'America/New_York')::date",
		"date_trunc('month', event_at::timestamp)",
		"date_trunc($1, event_at, 'America/New_York')",
		"date_trunc('month', event_at + interval '1 day', 'America/New_York')",
		"extract(month from event_at)", "event_at",
	} {
		t.Run(expression, func(t *testing.T) {
			p, c := calendarPlan(t, "SELECT "+expression+" AS period, sum(amount) FROM analytics.sales GROUP BY 1", "timestamptz", "America/New_York")
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
				t.Fatal("unproved partition accepted")
			}
		})
	}
	p, c := calendarPlan(t, "SELECT date_trunc('month',event_at,'America/New_York'),sum(amount) FROM analytics.sales GROUP BY 1,id", "timestamptz", "America/New_York")
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalMismatch) {
		t.Fatal("hidden group", err)
	}
	p, c = calendarPlan(t, "SELECT date_trunc('month',event_at),sum(amount) FROM analytics.sales GROUP BY 1", "date", "")
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("date implicitly chose zoned overload")
	}
}
func TestSQLRecoveryCalendarProofBoundaries(t *testing.T) {
	statement := "SELECT date_trunc('month',event_at,'America/New_York'),sum(amount) FROM analytics.sales GROUP BY 1"
	for _, mutate := range []func(*AnalyticalContract){
		func(c *AnalyticalContract) { c.Version = AnalyticalGrainVersion },
		func(c *AnalyticalContract) { c.Grain.Policy = AnalyticalGrainPolicy },
		func(c *AnalyticalContract) { c.Grain.Buckets[0].Calendar = "fiscal" },
		func(c *AnalyticalContract) { c.Grain.Buckets[0].Timezone = "Local" },
		func(c *AnalyticalContract) { c.Grain.Buckets[0].Timezone = "" },
		func(c *AnalyticalContract) { c.Grain.Buckets[0].Column = "foreign" },
		func(c *AnalyticalContract) { c.Grain.Buckets[0].Grain = "week" },
		func(c *AnalyticalContract) { c.Grain.Buckets = append(c.Grain.Buckets, c.Grain.Buckets[0]) },
	} {
		p, c := calendarPlan(t, statement, "timestamptz", "America/New_York")
		mutate(&c)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrBinding) {
			t.Fatal("contract", err)
		}
	}
	p, c := calendarPlan(t, statement, "timestamptz", "America/New_York")
	p.nativeChecked = false
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrBinding) {
		t.Fatal("native gate bypassed", err)
	}
}
func TestSQLRecoveryCalendarExactTypes(t *testing.T) {
	for _, tc := range []struct{ native, category, want string }{
		{"date", "temporal", "date"}, {"timestamp", "temporal", "civil"}, {"timestamptz", "temporal", "instant"},
		{"timestamp(3)", "temporal", ""}, {"time", "temporal", ""}, {"text", "temporal", ""}, {"date", "text", ""},
	} {
		if got := AnalyticalCalendarKind(tc.native, tc.category); got != tc.want {
			t.Fatalf("%+v = %q", tc, got)
		}
	}
}
