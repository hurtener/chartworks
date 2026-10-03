package sqlpolicy

import "testing"

func TestMySQLUTCInstantContract(t *testing.T) {
	for _, v := range []string{"timestamp", "TIMESTAMP(0)", "timestamp(6)"} {
		if !MySQLTimestampType(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"datetime", "datetime(6)", "timestamp(7)", "timestamp(12)", "timestamp with time zone", "timestamp(foo)", ""} {
		if MySQLTimestampType(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"+00:00", "UTC"} {
		if !MySQLUTCZone(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"utc", "+01:00", "SYSTEM", "America/New_York", ""} {
		if MySQLUTCZone(v) {
			t.Fatal(v)
		}
	}
	p, _ := ForDialect("mysql")
	if p.TemporalSyntax != MySQLUTCInstantSyntax {
		t.Fatal("missing guidance")
	}
	p, _ = ForDialect("postgres")
	if p.TemporalSyntax != "" {
		t.Fatal("cross-dialect guidance")
	}
}
