package exec

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
