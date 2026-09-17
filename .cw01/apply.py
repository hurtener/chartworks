"""Apply the inspected explicit-timezone finding and bounded regression tests."""
from pathlib import Path
import subprocess

expected = {
    "internal/exec/business_model.go": "33f1612d93dc511928dcfaa4c68cf7a613f7404d",
    "internal/semantics/clarification_compile.go": "70c57ca366ab3d89bfc195a9c849ec26cde4ea9b",
    "internal/semantics/clarification_values.go": "61362ad35d10bc9e131ef974f70e2a8a56fbde44",
}
pending = {}
for path, sha in expected.items():
    if subprocess.check_output(["git", "hash-object", path], text=True).strip() != sha:
        raise SystemExit(f"{path}: inspected source changed; re-review required")
    pending[path] = Path(path).read_text()

def replace(path, before, after):
    if pending[path].count(before) != 1:
        raise SystemExit(f"{path}: reviewed anchor changed")
    pending[path] = pending[path].replace(before, after, 1)

replace("internal/semantics/clarification_compile.go",
        'len(e.TimeZone) == 0 || len(e.TimeZone) > 128',
        'len(e.TimeZone) == 0 || e.TimeZone == "Local" || len(e.TimeZone) > 128')
replace("internal/semantics/clarification_values.go",
        'value.TimeZone != effect.TimeZone || len(value.TimeZone) > 128',
        'value.TimeZone != effect.TimeZone || value.TimeZone == "" || value.TimeZone == "Local" || len(value.TimeZone) > 128')
replace("internal/exec/business_model.go",
        '\t\t\tif _, err := time.LoadLocation(c.TimeZone); err != nil {',
        '\t\t\t// Empty and Local are runtime defaults, not reviewed timezone pins.\n\t\t\tif c.TimeZone == "" || c.TimeZone == "Local" {\n\t\t\t\treturn businessError(c, "time_zone", "invalid_time_zone")\n\t\t\t}\n\t\t\tif _, err := time.LoadLocation(c.TimeZone); err != nil {')

new_files = {
"internal/semantics/clarification_timezone_test.go": r'''package semantics

import "testing"

func TestClarificationRejectsImplicitTimezones(t *testing.T) {
	for _, zone := range []string{"", "Local"} {
		t.Run("policy/"+zone, func(t *testing.T) {
			slot := cw01TimeSlot()
			slot.Effect.TimeZone = zone
			subject, definition := cw01Definition(t, slot)
			if _, err := CompilePublishedRules(subject, definition); err == nil {
				t.Fatal("host-dependent temporal policy compiled")
			}
			for _, locale := range []string{"en", "es"} {
				input := ClarificationValue{Time: &ClarificationTimeInput{
					Start: "2026-01-01", End: "2026-02-01", Grain: "month",
					Calendar: "gregorian", TimeZone: zone,
				}}
				out, err := ResolveClarificationValue(slot, input, locale)
				if err == nil || err.Code != "calendar_mismatch" || err.Message == "" || out.Time != nil || out.Effect != nil {
					t.Fatal("implicit timezone produced a partial resolution or no repair error")
				}
			}
		})
	}
	slot := cw01TimeSlot()
	slot.Effect.TimeZone = "UTC"
	subject, definition := cw01Definition(t, slot)
	if _, err := CompilePublishedRules(subject, definition); err != nil {
		t.Fatal("explicit UTC policy rejected", err)
	}
	out, err := ResolveClarificationValue(slot, ClarificationValue{Time: &ClarificationTimeInput{
		Start: "2026-01-01", End: "2026-02-01", Grain: "month", Calendar: "gregorian", TimeZone: "UTC",
	}}, "en")
	if err != nil || out.Time == nil || out.Time.StartUTC != "2026-01-01T00:00:00Z" || out.Time.EndUTC != "2026-02-01T00:00:00Z" {
		t.Fatal("explicit UTC lost exact canonical boundaries", err)
	}
}
''',
"internal/exec/business_timezone_test.go": r'''package exec

import (
	"context"
	"errors"
	"testing"
)

func TestBusinessBindingRejectsImplicitTimezones(t *testing.T) {
	binding := parserBinding()
	binding.Relations[0].Columns[1].NativeType = "timestamptz"
	c := BusinessConstraint{
		Resolution: Hash("reviewed-explicit-zone"), Dataset: "sales", Column: "amount", SourceRevision: 1,
		Kind: "time_window", Operator: "range", Nulls: "exclude", Bounds: "[)",
		TemporalType: "timestamptz", Calendar: "gregorian", Grain: "day",
		Value: "2026-01-02T00:00:00Z", Upper: "2026-01-03T00:00:00Z",
	}
	for _, zone := range []string{"", "Local"} {
		c.TimeZone = zone
		var failure *BusinessConstraintError
		if err := ValidateBusinessConstraints(binding, []BusinessConstraint{c}); !errors.As(err, &failure) || failure.Code != "invalid_time_zone" {
			t.Fatal("implicit zone passed pre-provider binding validation", err)
		}
		out, err := BindBusinessConstraints(context.Background(), binding, "SELECT id FROM analytics.sales", nil, []BusinessConstraint{c})
		if !errors.Is(err, ErrBinding) || out.SQL != "" || len(out.Parameters) != 0 || len(out.Receipt.Bindings) != 0 {
			t.Fatal("implicit zone left a partial query or predicate", err)
		}
	}
	c.TimeZone = "UTC"
	out, err := BindBusinessConstraints(context.Background(), binding, "SELECT id FROM analytics.sales", nil, []BusinessConstraint{c})
	if err != nil || len(out.Parameters) != 2 || out.Parameters[0].Value != c.Value || out.Parameters[1].Value != c.Upper {
		t.Fatal("explicit UTC binding lost exact interval", err)
	}
}
''',
}
for path, text in new_files.items():
    if Path(path).exists():
        raise SystemExit(f"{path}: new regression already exists; do not overwrite")
    pending[path] = text
for path, text in sorted(pending.items()):
    Path(path).write_text(text)
print("Applied explicit-timezone validation and source/parser regressions")
