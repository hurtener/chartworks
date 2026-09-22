package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The native type must not be accepted as both a wall-clock value and an
// instant. These are synthetic adapter contract tests, not live cloud tests.
func TestBusinessTemporalSourceTypeIdentity(t *testing.T) {
	cases := []struct {
		dialect, native string
		wall, instant   bool
	}{
		{"postgres", "timestamp", true, false},
		{"postgres", "timestamptz", false, true},
		{"bigquery", "timestamp", false, true},
		{"bigquery", "datetime", true, false},
		{"databricks", "timestamp", false, true},
		{"databricks", "timestamp_ntz", true, false},
		{"snowflake", "timestamp_ntz", true, false},
		{"snowflake", "timestamp_tz", false, true},
		{"sqlserver", "datetime2", true, false},
		{"sqlserver", "datetimeoffset", false, true},
		{"mysql", "datetime", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.dialect+"/"+tc.native, func(t *testing.T) {
			column := Column{NativeType: tc.native}
			wall := BusinessConstraint{Kind: "time_window", TemporalType: "timestamp"}
			instant := BusinessConstraint{Kind: "time_window", TemporalType: "timestamptz"}
			if got := businessColumnCompatible(tc.dialect, column, wall); got != tc.wall {
				t.Errorf("wall-clock source compatibility = %t; want %t", got, tc.wall)
			}
			if got := businessColumnCompatible(tc.dialect, column, instant); got != tc.instant {
				t.Errorf("instant source compatibility = %t; want %t", got, tc.instant)
			}
		})
	}
}

func TestBusinessTemporalNativeMismatchFailsBeforeBinding(t *testing.T) {
	binding := parserBinding()
	binding.Dialect = "databricks"
	binding.Relations[0].Columns[1].NativeType = "timestamp"
	c := BusinessConstraint{
		Resolution: Hash("reviewed-wall-clock"), Dataset: "sales", Column: "amount",
		SourceRevision: 1, Kind: "time_window", Operator: "range", Nulls: "exclude",
		TemporalType: "timestamp", Calendar: "gregorian", TimeZone: "America/Argentina/Buenos_Aires",
		Grain: "day", Bounds: "[)", Value: "2026-01-02", Upper: "2026-01-03",
	}
	if err := ValidateBusinessConstraints(binding, []BusinessConstraint{c}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("native instant accepted a wall-clock policy at pre-provider validation", err)
	}
	out, err := BindBusinessConstraints(context.Background(), binding, "SELECT `id` FROM `analytics`.`sales`", nil, []BusinessConstraint{c})
	if !errors.Is(err, ErrUnsupported) || out.SQL != "" || len(out.Parameters) != 0 || len(out.Receipt.Bindings) != 0 {
		t.Fatal("type mismatch returned a partially bound constraint", err)
	}
	binding.Relations[0].Columns[1].NativeType = "timestamp_ntz"
	out, err = BindBusinessConstraints(context.Background(), binding, "SELECT `id` FROM `analytics`.`sales`", nil, []BusinessConstraint{c})
	if err != nil || strings.Count(out.SQL, "AS TIMESTAMP_NTZ)") != 2 || len(out.Parameters) != 2 {
		t.Fatal("wall-clock bounds did not retain an explicit non-zone cast", err)
	}
	if out.Parameters[0].Value != c.Value || out.Parameters[1].Value != c.Upper || out.Receipt.Validation != nil {
		t.Fatal("wall-clock binding changed exact bounds or fabricated read proof")
	}
	binding.Relations[0].Columns[1].NativeType = "timestamp"
	c.TemporalType = "timestamptz"
	c.Value, c.Upper = "2026-01-02T03:00:00Z", "2026-01-03T03:00:00Z"
	out, err = BindBusinessConstraints(context.Background(), binding, "SELECT `id` FROM `analytics`.`sales`", nil, []BusinessConstraint{c})
	if err != nil || strings.Count(out.SQL, "AS TIMESTAMP)") != 2 || len(out.Parameters) != 2 {
		t.Fatal("instant bounds lost their distinct cast", err)
	}
	if out.Parameters[0].Value != c.Value || out.Parameters[1].Value != c.Upper {
		t.Fatal("instant binding changed explicit UTC bounds")
	}
}
