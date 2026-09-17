package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A native MySQL TIMESTAMP cannot be treated as a zone-free DATETIME. This is
// a synthetic pre-provider admission regression, not a new warehouse support claim.
func TestBusinessMySQLTimestampRejectedBeforeBinding(t *testing.T) {
	for _, native := range []string{"timestamp", "TIMESTAMP", "timestamp(6)"} {
		for _, temporal := range []string{"timestamp", "timestamptz"} {
			t.Run(native+"/"+temporal, func(t *testing.T) {
				binding := parserBinding()
				binding.Dialect = "mysql"
				binding.Relations[0].Columns[1].NativeType = native
				c := mysqlTimeConstraint()
				c.TemporalType = temporal
				if temporal == "timestamptz" {
					c.Value, c.Upper = "2026-01-02T03:00:00Z", "2026-01-03T03:00:00Z"
				}
				var failure *BusinessConstraintError
				err := ValidateBusinessConstraints(binding, []BusinessConstraint{c})
				if !errors.As(err, &failure) || failure.Code != "unsupported_constraint_type" || failure.Field != "target" || !errors.Is(err, ErrUnsupported) {
					t.Fatal("session-sensitive native type passed pre-provider admission", err)
				}
				out, err := BindBusinessConstraints(context.Background(), binding, "SELECT `id` FROM `analytics`.`sales`", nil, []BusinessConstraint{c})
				if !errors.Is(err, ErrUnsupported) || out.SQL != "" || len(out.Parameters) != 0 || len(out.Receipt.Bindings) != 0 || out.Receipt.Validation != nil {
					t.Fatal("unsupported temporal target returned a partial binding", err)
				}
				statement := "SELECT `id` FROM `analytics`.`sales`"
				unconstrained, err := BindBusinessConstraints(context.Background(), binding, statement, nil, nil)
				if err != nil || unconstrained.SQL != statement || len(unconstrained.Parameters) != 0 || unconstrained.Receipt.Validation != nil {
					t.Fatal("temporal restriction changed a query without scalar constraints", err)
				}
			})
		}
	}
}

func TestBusinessMySQLWallClockAndDatePreserved(t *testing.T) {
	for _, native := range []string{"datetime", "datetime(6)", "date"} {
		t.Run(native, func(t *testing.T) {
			binding := parserBinding()
			binding.Dialect = "mysql"
			binding.Relations[0].Columns[1].NativeType = native
			c := mysqlTimeConstraint()
			cast := "DATETIME"
			if native == "date" {
				c.TemporalType, cast = "date", "DATE"
			}
			out, err := BindBusinessConstraints(context.Background(), binding, "SELECT `id` FROM `analytics`.`sales`", nil, []BusinessConstraint{c})
			if err != nil || strings.Count(out.SQL, " AS "+cast+")") != 2 || len(out.Parameters) != 2 {
				t.Fatal("supported date or wall-clock binding changed", err)
			}
			if out.Parameters[0].Value != c.Value || out.Parameters[1].Value != c.Upper || out.Receipt.Validation != nil {
				t.Fatal("binding changed exact bounds or fabricated read proof")
			}
		})
	}
}

func mysqlTimeConstraint() BusinessConstraint {
	c := BusinessConstraint{}
	c.Resolution = Hash("reviewed-mysql-window")
	c.Dataset, c.Column, c.SourceRevision = "sales", "amount", 1
	c.Kind, c.Operator, c.Nulls = "time_window", "range", "exclude"
	c.TemporalType, c.Calendar, c.TimeZone = "timestamp", "gregorian", "America/Argentina/Buenos_Aires"
	c.Grain, c.Bounds = "day", "[)"
	c.Value, c.Upper = "2026-01-02", "2026-01-03"
	return c
}
