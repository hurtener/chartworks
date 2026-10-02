//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"testing"
)

func TestSQLRecoveryWarehouseCalendarPartitions(t *testing.T) {
	cases := []struct{ dialect, native, expression, unit, timezone string }{
		{"mysql", "datetime", `CAST(DATE_FORMAT(created_at,'%Y-%m-01') AS DATE)`, "month", ""},
		{"mysql", "date", `MAKEDATE(EXTRACT(YEAR FROM created_at),1) + INTERVAL (EXTRACT(QUARTER FROM created_at)-1) QUARTER`, "quarter", ""},
		{"sqlserver", "datetime2", `DATETRUNC(month,created_at)`, "month", ""},
		{"sqlserver", "date", `DATETRUNC(quarter,created_at)`, "quarter", ""},
		{"bigquery", "timestamp", `TIMESTAMP_TRUNC(created_at,MONTH,'UTC')`, "month", "UTC"},
		{"bigquery", "timestamp", `TIMESTAMP_TRUNC(created_at,QUARTER,'America/New_York')`, "quarter", "America/New_York"},
		{"bigquery", "date", `DATE_TRUNC(created_at,QUARTER)`, "quarter", ""},
		{"bigquery", "datetime", `DATETIME_TRUNC(created_at,MONTH)`, "month", ""},
		{"snowflake", "timestamp_ntz", `DATE_TRUNC('quarter',created_at)`, "quarter", ""},
		{"databricks", "timestamp_ntz", `DATE_TRUNC('month',created_at)`, "month", ""},
	}
	for _, tc := range cases {
		t.Run(tc.dialect+tc.native+tc.unit, func(t *testing.T) {
			sql := "SELECT " + tc.expression + ",sum(amount) FROM analytics.sales GROUP BY " + tc.expression
			p, c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
			p.candidate.binding.Dialect = tc.dialect
			for i := range p.candidate.binding.Relations[0].Columns {
				col := &p.candidate.binding.Relations[0].Columns[i]
				if col.Name == "created_at" {
					col.NativeType, col.Category = tc.native, "temporal"
				}
			}
			c.Version = AnalyticalIntentVersion
			c.Binding = Hash(p.candidate.binding)
			c.Grain = &AnalyticalGrain{Policy: AnalyticalCalendarPolicy, Dimensions: []string{"event"}, Buckets: []AnalyticalBucket{{Column: "created_at", Calendar: "gregorian", Grain: tc.unit, Timezone: tc.timezone}}}
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
				t.Fatal(err)
			}
			wrong := *c.Grain
			wrong.Buckets = append([]AnalyticalBucket(nil), c.Grain.Buckets...)
			wrong.Buckets[0].Grain = "day"
			c.Grain = &wrong
			if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
				t.Fatal("different bucket admitted")
			}
		})
	}
}

func TestSQLRecoveryWarehouseCalendarRejectsLossyForms(t *testing.T) {
	for _, tc := range []struct{ dialect, native, expr string }{
		{"mysql", "timestamp", `CAST(DATE_FORMAT(created_at,'%Y-%m-01') AS DATE)`},
		{"mysql", "datetime", `CAST(DATE_FORMAT(created_at,'%m') AS DATE)`},
		{"mysql", "date", `MAKEDATE(EXTRACT(YEAR FROM created_at),1) + INTERVAL EXTRACT(QUARTER FROM created_at) QUARTER`},
		{"databricks", "timestamp", `DATE_TRUNC('month',created_at)`},
		{"snowflake", "timestamp_tz", `DATE_TRUNC('month',created_at)`},
		{"sqlserver", "datetimeoffset", `DATETRUNC(month,created_at)`},
		{"bigquery", "timestamp", `TIMESTAMP_TRUNC(created_at,"MONTH",'UTC')`},
	} {
		p, c := analyticalFixture(t, "SELECT "+tc.expr+",sum(amount) FROM analytics.sales GROUP BY 1", analyticalMetrics(analyticalMeasure("sum", "amount")))
		p.candidate.binding.Dialect = tc.dialect
		for i := range p.candidate.binding.Relations[0].Columns {
			col := &p.candidate.binding.Relations[0].Columns[i]
			if col.Name == "created_at" {
				col.NativeType, col.Category = tc.native, "temporal"
			}
		}
		c.Version = AnalyticalIntentVersion
		c.Binding = Hash(p.candidate.binding)
		c.Grain = &AnalyticalGrain{Policy: AnalyticalCalendarPolicy, Dimensions: []string{"event"}, Buckets: []AnalyticalBucket{{Column: "created_at", Calendar: "gregorian", Grain: "month"}}}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal(tc.dialect, tc.expr)
		}
	}
}

func TestSQLRecoveryBigQueryUntypedDecimalIsNotExact(t *testing.T) {
	expr := AnalyticalExpression{Op: "*", Args: []AnalyticalExpression{analyticalMeasure("sum", "amount"), {Op: "number", Value: "1.1"}}}
	p, c := analyticalFixture(t, "SELECT sum(amount)*1.1 FROM analytics.sales", analyticalMetrics(expr))
	p.candidate.binding.Dialect = "bigquery"
	c.Version = AnalyticalIntentVersion
	c.Binding = Hash(p.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("FLOAT64 literal certified as exact NUMERIC")
	}
}
