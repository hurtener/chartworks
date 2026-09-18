package exec

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// Synthetic adapter tests verify the complete declared exact domain. They do
// not claim live warehouse qualification or use floating-point intermediates.
func TestBusinessDecimalNativeIntegralCapacity(t *testing.T) {
	binding := parserBinding()
	binding.Dialect = "bigquery"
	for _, tc := range []struct {
		precision, scale int
		cast             string
	}{
		{29, 0, "NUMERIC"},
		{38, 9, "NUMERIC"},
		{30, 0, "BIGNUMERIC"},
		{38, 0, "BIGNUMERIC"},
		{38, 8, "BIGNUMERIC"},
		{39, 10, "BIGNUMERIC"},
		{76, 38, "BIGNUMERIC"},
		{4, 4, "NUMERIC"},
	} {
		t.Run(strconv.Itoa(tc.precision)+"/"+strconv.Itoa(tc.scale), func(t *testing.T) {
			c := businessFixtureConstraint()
			c.Precision, c.Scale = tc.precision, tc.scale
			c.Value = strings.Repeat("9", tc.precision-tc.scale)
			if c.Value == "" {
				c.Value = "0"
			}
			if tc.scale > 0 {
				c.Value += "." + strings.Repeat("9", tc.scale)
			}
			if err := ValidateBusinessConstraints(binding, []BusinessConstraint{c}); err != nil {
				t.Fatal("representable exact declaration rejected", err)
			}
			out, err := BindBusinessConstraints(context.Background(), binding, "SELECT `id` FROM `analytics`.`sales`", nil, []BusinessConstraint{c})
			if err != nil || len(out.Parameters) != 1 || out.Parameters[0].Value != c.Value || !strings.Contains(out.SQL, " AS "+tc.cast+")") {
				t.Fatal("native decimal cast lost exact declared capacity", err)
			}
			if out.Receipt.Validation != nil || out.Receipt.Statement != Hash([]any{out.SQL, out.Parameters}) {
				t.Fatal("decimal binding fabricated read proof or lost exact parameter seal")
			}
		})
	}
	for _, precision := range []int{39, 76} {
		c := businessFixtureConstraint()
		c.Precision, c.Scale = precision, 0
		// Even a currently small value cannot make an unsupported declared
		// domain safe for a later correction in the same reviewed policy.
		c.Value = "1"
		var failure *BusinessConstraintError
		if err := ValidateBusinessConstraints(binding, []BusinessConstraint{c}); !errors.As(err, &failure) || failure.Code != "unsupported_exact_precision" {
			t.Fatal("unsupported integral capacity passed pre-provider validation", err)
		}
		out, err := BindBusinessConstraints(context.Background(), binding, "SELECT `id` FROM `analytics`.`sales`", nil, []BusinessConstraint{c})
		if !errors.Is(err, ErrUnsupported) || out.SQL != "" || len(out.Parameters) != 0 || len(out.Receipt.Bindings) != 0 {
			t.Fatal("unsupported precision produced a partial constraint", err)
		}
	}
}
