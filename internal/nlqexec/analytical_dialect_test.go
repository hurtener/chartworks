package nlqexec

import (
	"context"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
	"github.com/hurtener/chartworks/internal/semantics"
	"strings"
	"testing"
)

func TestSQLRecoveryWarehouseCalendarCompilerAndGuidance(t *testing.T) {
	for _, tc := range []struct{ dialect, native, syntax string }{
		{"mysql", "datetime(6)", "DATE_FORMAT"}, {"sqlserver", "datetime2", "DATETRUNC"}, {"bigquery", "timestamp", "TIMESTAMP_TRUNC"}, {"snowflake", "timestamp_ntz", "DATE_TRUNC"}, {"databricks", "timestamp_ntz", "DATE_TRUNC"},
	} {
		t.Run(tc.dialect, func(t *testing.T) {
			a := calendarAdmission("Revenue by month of Order date ordered by month of Order date descending", tc.native)
			a.binding.Dialect = tc.dialect
			analyticalReseal(&a)
			c, err := compileCurrentAnalytical(context.Background(), a)
			if err != nil || c.Grain == nil || len(c.Grain.Buckets) != 1 || c.Intent == nil {
				t.Fatal(err)
			}
			wantZone := ""
			if tc.dialect == "bigquery" {
				wantZone = "America/New_York"
			}
			if c.Grain.Buckets[0].Timezone != wantZone || exec.Hash(c.Grain.Buckets[0]) != exec.Hash(*c.Intent.Order[0].Bucket) {
				t.Fatal("calendar intent type/zone drift")
			}
			guidance := analyticalGrainGuidanceForDialect(tc.dialect, c)
			if !strings.Contains(guidance, tc.syntax) || strings.Contains(guidance, "date_trunc's third argument") {
				t.Fatal("incorrect dialect guidance")
			}
		})
	}
}
func TestSQLRecoveryWarehouseOrderingDefaults(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "databricks"} {
		a := grainAdmission("Revenue by Region ordered by Revenue ascending")
		a.binding.Dialect = dialect
		analyticalReseal(&a)
		c, err := compileCurrentAnalytical(context.Background(), a)
		if err != nil || c.Intent == nil || c.Intent.Order[0].Nulls != "first" {
			t.Fatal("native default ordering", dialect, err)
		}
	}
}

func TestSQLRecoveryMySQLUTCInstantCompiler(t *testing.T) {
	dimension := grainDimension{role: semantics.DimensionTemporal, temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainMonth}}}
	column := semantics.Column{SourceName: "created_at", NativeType: "timestamp(6)", Category: "temporal"}
	bucket, err := compileCalendarBucket(dimension, column, "month", "mysql")
	if err != nil || bucket.Timezone != "UTC" {
		t.Fatal("explicit UTC bucket compile", err)
	}
	contract := &exec.AnalyticalContract{Grain: &exec.AnalyticalGrain{Policy: exec.AnalyticalCalendarPolicy, Buckets: []exec.AnalyticalBucket{bucket}}}
	if !strings.Contains(analyticalGrainGuidanceForDialect("mysql", contract), sqlpolicy.MySQLUTCInstantSyntax) {
		t.Fatal("native exact conversion missing from guidance")
	}
	dimension.temporal.Timezone = "America/New_York"
	if _, err = compileCalendarBucket(dimension, column, "month", "mysql"); err == nil {
		t.Fatal("unproved local instant rendering sent to generation")
	}
	column.NativeType = "datetime(6)"
	bucket, err = compileCalendarBucket(dimension, column, "month", "mysql")
	if err != nil || bucket.Timezone != "" {
		t.Fatal("civil source acquired zone", err)
	}
}

func TestSQLRecoveryMySQLGroupedGuidance(t *testing.T) {
	for _, calendar := range []bool{false, true} {
		c := &exec.AnalyticalContract{Version: exec.AnalyticalGroupedProgramsVersion, GroupedPopulations: &exec.AnalyticalGroupedPopulations{Policy: exec.AnalyticalGroupedPopulationPolicy}, Grain: &exec.AnalyticalGrain{Columns: []string{"group"}}}
		if calendar {
			c.Grain.Columns = nil
			c.Grain.Buckets = []exec.AnalyticalBucket{{Column: "created_at", Grain: "month", Calendar: "gregorian"}}
		}
		got := analyticalGrainGuidanceForDialect("mysql", c)
		for _, required := range []string{"UNION (distinct", "<=>", "Exact compiled lane contract", "LEFT JOIN every lane", "Preserve missing lane measures as NULL"} {
			if !strings.Contains(got, required) {
				t.Fatalf("calendar=%v missing %s", calendar, required)
			}
		}
		if strings.Contains(got, "IS NOT DISTINCT FROM") {
			t.Fatal("non-MySQL equality leaked into dialect guidance")
		}
		if calendar && !strings.Contains(got, "DATE_FORMAT") {
			t.Fatal("calendar rendering missing")
		}
	}
}
