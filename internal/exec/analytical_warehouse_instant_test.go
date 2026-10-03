//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"strings"
	"testing"
)

func TestSQLRecoveryWarehouseInstantCalendar(t *testing.T) {
	utc := "CAST(created_at AT TIME ZONE '+00:00' AS DATETIME(6))"
	month := "CAST(DATE_FORMAT(" + utc + ",'%Y-%m-01') AS DATE)"
	quarter := "MAKEDATE(EXTRACT(YEAR FROM " + utc + "),1) + INTERVAL (EXTRACT(QUARTER FROM " + utc + ")-1) QUARTER"
	for _, tc := range []struct{ expr, unit string }{{month, "month"}, {quarter, "quarter"}} {
		p, c := analyticalFixture(t, "SELECT "+tc.expr+",sum(amount) FROM analytics.sales GROUP BY 1", analyticalMetrics(analyticalMeasure("sum", "amount")))
		p.candidate.binding.Dialect = "mysql"
		for i := range p.candidate.binding.Relations[0].Columns {
			col := &p.candidate.binding.Relations[0].Columns[i]
			if col.Name == "created_at" {
				col.NativeType = "timestamp(6)"
				col.Category = "temporal"
			}
		}
		c.Version = AnalyticalIntentVersion
		c.Binding = Hash(p.candidate.binding)
		c.Grain = &AnalyticalGrain{Policy: AnalyticalCalendarPolicy, Dimensions: []string{"event"}, Buckets: []AnalyticalBucket{{Column: "created_at", Calendar: "gregorian", Grain: tc.unit, Timezone: "UTC"}}}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal(tc.unit, err)
		}
		for _, bad := range []string{strings.Replace(tc.expr, "+00:00", "+01:00", 1), strings.ReplaceAll(tc.expr, "DATETIME(6)", "DATETIME(0)"), strings.ReplaceAll(tc.expr, utc, "created_at")} {
			p.candidate.statement = "SELECT " + bad + ",sum(amount) FROM analytics.sales GROUP BY 1"
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
				t.Fatal("unproved instant conversion", bad)
			}
		}
		p.candidate.statement = "SELECT " + tc.expr + ",sum(amount) FROM analytics.sales GROUP BY 1"
		c.Grain.Buckets[0].Timezone = "America/New_York"
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("UTC bucket impersonated reviewed local zone")
		}
	}
	// A proof-only warehouse node must not grant meaning to a PostgreSQL AST.
	checker := analyticalChecker{binding: Binding{Dialect: "postgres"}}
	if _, handled, err := checker.warehouseCalendarTerm(map[string]any{"WarehouseCalendarBucket": map[string]any{"timezone": "UTC"}}); !handled || err == nil {
		t.Fatal("warehouse node crossed dialect boundary")
	}
}
