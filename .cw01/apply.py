"""Apply the exact-decimal adapter review finding without relaxing precision."""
from pathlib import Path
import subprocess

expected = {
    "internal/exec/business_model.go": "0368e7bd1b6a70e765d0c2eaef9d13d403915e4b",
    "internal/exec/business_sql.go": "77c9fe68280874d34199cc01574807bfb45135de",
}
pending = {}
for path, sha in expected.items():
    if subprocess.check_output(["git", "hash-object", path], text=True).strip() != sha:
        raise SystemExit(f"{path}: reviewed source changed")
    pending[path] = Path(path).read_text()

def replace(path, before, after):
    if pending[path].count(before) != 1:
        raise SystemExit(f"{path}: reviewed anchor changed")
    pending[path] = pending[path].replace(before, after, 1)

replace("internal/exec/business_model.go",
        '\t\t\tlower, ok := businessDecimal(c.Value, c.Precision, c.Scale)',
        '\t\t\t// Native decimal capacity also bounds integral digits. Do not\n\t\t\t// admit a declaration that only fits an approximate or partial digit.\n\t\t\tif binding.Dialect == "bigquery" && c.Precision-c.Scale > 38 {\n\t\t\t\treturn businessError(c, "precision", "unsupported_exact_precision")\n\t\t\t}\n\t\t\tlower, ok := businessDecimal(c.Value, c.Precision, c.Scale)')
replace("internal/exec/business_sql.go",
        'if c.Precision > 38 || c.Scale > 9 {',
        'if c.Precision-c.Scale > 29 || c.Scale > 9 {')

path = "internal/exec/business_precision_test.go"
if Path(path).exists():
    raise SystemExit("precision regression already exists; do not overwrite")
pending[path] = r'''package exec

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
'''
for path, text in sorted(pending.items()):
    Path(path).write_text(text)
print("Applied exact native decimal-capacity validation, cast selection and boundary tests")
