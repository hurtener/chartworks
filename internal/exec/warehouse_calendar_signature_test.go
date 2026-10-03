//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"testing"
	"time"
)

func TestWarehouseCalendarSignatureNative(t *testing.T) {
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{}
	v, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		dialect   string
		good, bad []string
	}{
		{"mysql", []string{"date_format(event_at,'%Y-%m-01')", "makedate(extract(year FROM event_at),1)", "extract(quarter FROM event_at)"}, []string{"date_format(event_at)", "makedate(2026)", "extract(week FROM event_at)", "date_trunc('month',event_at)"}},
		{"sqlserver", []string{"datetrunc(month,event_at)", "datetrunc(QUARTER,event_at)"}, []string{"datetrunc(secret,event_at)", "datetrunc('month',event_at)", "datetrunc(sales.month,event_at)", "datetrunc(month)"}},
		{"bigquery", []string{"date_trunc(event_at,MONTH)", "datetime_trunc(event_at,QUARTER)", "timestamp_trunc(event_at,YEAR,'UTC')"}, []string{"date_trunc(event_at,secret)", "date_trunc(event_at,'MONTH')", "date_trunc(event_at,sales.MONTH)", "timestamp_trunc(event_at)"}},
		{"snowflake", []string{"date_trunc('month',event_at)", "date_trunc('QUARTER',event_at)"}, []string{"date_trunc('secret',event_at)", "date_trunc('month')", "date_trunc(month,event_at)"}},
		{"databricks", []string{"date_trunc('month',event_at)", "date_trunc('QUARTER',event_at)"}, []string{"date_trunc('secret',event_at)", "date_trunc('month')", "date_trunc(month,event_at)"}},
	} {
		t.Run(tc.dialect, func(t *testing.T) {
			hiddenUnit := "month"
			if tc.dialect == "bigquery" {
				hiddenUnit = "MONTH"
			}
			adapter.binding = Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: tc.dialect, Contract: "contract", Fingerprint: Hash("calendar"), Relations: []Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []Column{{Name: "event_at", NativeType: "timestamp", Safe: true}, {Name: hiddenUnit, NativeType: "integer", Safe: false}}}}}
			checkSQL := func(sql string, want bool) {
				t.Helper()
				adapter.explains = 0
				plan, err := v.Validate(context.Background(), e, Request{Source: "source", Context: "source:v1", SQL: sql})
				if (err == nil) != want || plan.Receipt().Validated != want || adapter.explains != map[bool]int{true: 1, false: 0}[want] {
					t.Errorf("%s want=%v: %v explains=%d", sql, want, err, adapter.explains)
				}
			}
			check := func(expr string, want bool) {
				t.Helper()
				checkSQL("SELECT "+expr+" AS bucket FROM analytics.sales", want)
			}
			for _, expr := range tc.good {
				check(expr, true)
			}
			for _, expr := range tc.bad {
				check(expr, false)
			}
			switch tc.dialect {
			case "bigquery":
				for _, expr := range []string{"date_trunc(MONTH,MONTH)", "date_trunc(event_at,MONTH) AS first_bucket,MONTH", "date_trunc(event_at,MONTH)+sum(MONTH)", "date_trunc(event_at,`MONTH`)"} {
					check(expr, false)
				}
			case "sqlserver":
				for _, expr := range []string{"datetrunc(month,month)", "datetrunc(month,event_at) AS first_bucket,month", "datetrunc(month,event_at)+sum(month)", "datetrunc([month],event_at)"} {
					check(expr, false)
				}
			}
			if tc.dialect == "bigquery" || tc.dialect == "sqlserver" {
				call := "date_trunc(event_at,MONTH)"
				if tc.dialect == "sqlserver" {
					call = "datetrunc(month,event_at)"
				}
				sql := "SELECT " + call + " AS bucket FROM analytics.sales WHERE " + hiddenUnit + ">0"
				checkSQL(sql, false)
				adapter.binding.Relations[0].Columns[1].Safe = true
				checkSQL(sql, true)
			}
		})
	}
}
