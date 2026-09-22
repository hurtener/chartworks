package exec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBusinessAdmissionRejectsPartialAndMixedValues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*BusinessConstraint)
	}{
		{"uppercase-resolution", func(c *BusinessConstraint) { c.Resolution = strings.ToUpper(c.Resolution) }},
		{"short-resolution", func(c *BusinessConstraint) { c.Resolution = "aa" }},
		{"null-with-value", func(c *BusinessConstraint) { c.Null, c.Nulls = true, "only" }},
		{"null-with-upper", func(c *BusinessConstraint) { c.Null, c.Nulls, c.Value, c.Upper = true, "only", "", "2" }},
		{"null-only-without-null", func(c *BusinessConstraint) { c.Nulls = "only" }},
		{"excluded-null", func(c *BusinessConstraint) { c.Null, c.Value = true, "" }},
		{"invalid-utf8", func(c *BusinessConstraint) { c.Value = string([]byte{0xff}) }},
		{"control-byte", func(c *BusinessConstraint) { c.Value = "1\x00" }},
		{"negative-scale", func(c *BusinessConstraint) { c.Scale = -1 }},
		{"missing-unit", func(c *BusinessConstraint) { c.Unit = "" }},
		{"oversized-unit", func(c *BusinessConstraint) { c.Unit = strings.Repeat("u", 65) }},
		{"unexpected-upper", func(c *BusinessConstraint) { c.Upper = "2" }},
		{"unexpected-bounds", func(c *BusinessConstraint) { c.Bounds = "[]" }},
		{"invalid-range-upper", func(c *BusinessConstraint) { c.Operator, c.Bounds, c.Value, c.Upper = "range", "[)", "1", "NaN" }},
		{"reversed-range", func(c *BusinessConstraint) { c.Operator, c.Bounds, c.Value, c.Upper = "range", "[]", "2", "1" }},
		{"open-singleton", func(c *BusinessConstraint) { c.Operator, c.Bounds, c.Value, c.Upper = "range", "[)", "1", "1" }},
		{"invalid-range-bounds", func(c *BusinessConstraint) { c.Operator, c.Bounds, c.Value, c.Upper = "range", "??", "1", "2" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			binding := parserBinding()
			c := businessFixtureConstraint()
			tc.mutate(&c)
			before := binding.Clone()
			if err := ValidateBusinessConstraints(binding, []BusinessConstraint{c}); !errors.Is(err, ErrBinding) && !errors.Is(err, ErrUnsupported) {
				t.Fatal("invalid constraint passed pre-provider admission", err)
			}
			out, err := BindBusinessConstraints(context.Background(), binding, "SELECT id FROM analytics.sales", nil, []BusinessConstraint{c})
			if err == nil || out.SQL != "" || len(out.Parameters) != 0 || len(out.Receipt.Bindings) != 0 || out.Receipt.Validation != nil {
				t.Fatal("rejected constraint produced a partial execution candidate")
			}
			if !reflect.DeepEqual(binding, before) {
				t.Fatal("failed admission mutated source metadata")
			}
		})
	}
	c := businessFixtureConstraint()
	if err := ValidateBusinessConstraints(Binding{}, []BusinessConstraint{c}); !errors.Is(err, ErrBinding) {
		t.Fatal("zero binding accepted", err)
	}
	if err := ValidateBusinessConstraints(parserBinding(), make([]BusinessConstraint, 65)); !errors.Is(err, ErrBinding) {
		t.Fatal("unbounded group accepted", err)
	}
	if err := ValidateBusinessConstraints(parserBinding(), []BusinessConstraint{c, c}); !errors.Is(err, ErrBinding) {
		t.Fatal("duplicate resolution accepted", err)
	}
}

func TestBusinessAdmissionScalarNativeTypes(t *testing.T) {
	for _, kind := range []string{"boolean", "entity", "text"} {
		t.Run(kind, func(t *testing.T) {
			binding := parserBinding()
			c := businessFixtureConstraint()
			c.Kind, c.Operator, c.Precision, c.Scale, c.Unit = kind, "eq", 0, 0, ""
			c.Value = "reviewed-value"
			binding.Relations[0].Columns[1].NativeType = "text"
			binding.Relations[0].Columns[1].Category = "text"
			if kind == "boolean" {
				binding.Relations[0].Columns[1].NativeType = "boolean"
				binding.Relations[0].Columns[1].Category = "boolean"
				c.Value = "true"
			}
			if err := ValidateBusinessConstraints(binding, []BusinessConstraint{c}); err != nil {
				t.Fatal("reviewed scalar/native combination rejected", err)
			}
			for _, mutate := range []func(*BusinessConstraint){
				func(x *BusinessConstraint) { x.Operator = "range" },
				func(x *BusinessConstraint) { x.Upper = "extra" },
				func(x *BusinessConstraint) { x.Value = "" },
				func(x *BusinessConstraint) { x.Value = strings.Repeat("x", 513) },
			} {
				bad := c
				mutate(&bad)
				if err := ValidateBusinessConstraints(binding, []BusinessConstraint{bad}); !errors.Is(err, ErrBinding) {
					t.Fatal("invalid scalar shape passed native admission", err)
				}
			}
			binding.Relations[0].Columns[1].Safe = false
			if err := ValidateBusinessConstraints(binding, []BusinessConstraint{c}); !errors.Is(err, ErrBinding) {
				t.Fatal("unsafe native column admitted", err)
			}
		})
	}
}
