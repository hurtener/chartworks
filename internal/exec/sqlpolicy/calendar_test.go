package sqlpolicy

import "testing"

func TestCalendarSignatureDialectAndUnitDomains(t *testing.T) {
	for _, tc := range []struct {
		d, n, form string
		index      int
	}{{"mysql", "extract", "keyword", 0}, {"sqlserver", "datetrunc", "keyword", 0}, {"bigquery", "date_trunc", "keyword", 1}, {"bigquery", "datetime_trunc", "keyword", 1}, {"bigquery", "timestamp_trunc", "keyword", 1}, {"snowflake", "date_trunc", "string", 0}, {"databricks", "date_trunc", "string", 0}} {
		for _, unit := range []string{"day", "MONTH", "quarter", "Year"} {
			if _, ok := CalendarUnit(tc.d, tc.n, tc.index, tc.form, unit); !ok {
				t.Fatal("required unit rejected", tc, unit)
			}
		}
		if _, ok := CalendarUnit(tc.d, tc.n, tc.index, tc.form, "month "); ok {
			t.Fatal("nonliteral unit normalized")
		}
		if _, ok := CalendarUnit(tc.d, tc.n, tc.index+1, tc.form, "month"); ok {
			t.Fatal("wrong argument authorized")
		}
		if _, ok := CalendarUnit(tc.d, tc.n, tc.index, "wrong", "month"); ok {
			t.Fatal("wrong grammar form authorized")
		}
		s, _ := FunctionSignature(tc.d, tc.n)
		s.Units[0] = "changed"
		fresh, _ := FunctionSignature(tc.d, tc.n)
		if fresh.Units[0] != "day" {
			t.Fatal("unit mutation leaked")
		}
	}
	if AllowsFunction("mysql", []string{"date_trunc"}) || AllowsFunction("sqlserver", []string{"date_format"}) || AllowsFunction("snowflake", []string{"makedate"}) {
		t.Fatal("cross-dialect function leaked")
	}
}
