package engineering

import "testing"

func TestProfileNativeTemporalTypes(t *testing.T) {
	for _, native := range []string{"date", "timestamp", "timestamptz", "timestamp with time zone", "timestamp without time zone", "timestamp(0) with time zone", "timestamp(6) without time zone", "timestamptz(3)"} {
		if !profileTimeType(native) {
			t.Errorf("qualified native temporal type rejected: %q", native)
		}
	}
	for _, native := range []string{"time", "timetz", "interval", "text", "numeric(30,3)", "analytics.timestamp", "timestamp(9) with time zone", "timestamp with time zone; SELECT 1", " timestamp", "timestamp\n"} {
		if profileTimeType(native) {
			t.Errorf("unsupported type accepted: %q", native)
		}
	}
}
