package nlqexec

import (
	"context"
	"github.com/hurtener/chartworks/internal/exec"
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
