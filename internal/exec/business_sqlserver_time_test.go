package exec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBusinessSQLServerRowversionRejectedBeforeBinding(t *testing.T) {
	for _, native := range []string{"timestamp", "TIMESTAMP", "rowversion", "ROWVERSION"} {
		for _, temporal := range []string{"date", "timestamp", "timestamptz"} {
			t.Run(native+"/"+temporal, func(t *testing.T) {
				binding := parserBinding()
				binding.Dialect = "sqlserver"
				binding.Relations[0].Columns[1].NativeType = native
				binding.Relations[0].Columns[1].Category = "timestamp"
				c := sqlServerTimeConstraint(temporal)
				var failure *BusinessConstraintError
				err := ValidateBusinessConstraints(binding, []BusinessConstraint{c})
				if !errors.As(err, &failure) || failure.Code != "unsupported_constraint_type" || failure.Field != "target" || failure.Resolution != c.Resolution || !errors.Is(err, ErrUnsupported) {
					t.Fatal("binary rowversion passed calendar admission", err)
				}
				statement := "SELECT [id] FROM [analytics].[sales]"
				out, err := BindBusinessConstraints(context.Background(), binding, statement, nil, []BusinessConstraint{c})
				if !errors.Is(err, ErrUnsupported) || !reflect.DeepEqual(out, BusinessBoundQuery{}) {
					t.Fatal("rejected rowversion returned partial SQL, parameters or receipt", err)
				}
				unconstrained, err := BindBusinessConstraints(context.Background(), binding, statement, nil, nil)
				if err != nil || !reflect.DeepEqual(unconstrained, BusinessBoundQuery{SQL: statement}) {
					t.Fatal("calendar rejection changed unconstrained SQL", err)
				}
			})
		}
	}
}

func TestBusinessSQLServerCalendarTypesPreserved(t *testing.T) {
	cases := []struct {
		native   string
		temporal string
		cast     string
	}{
		{"date", "date", "DATE"},
		{"datetime", "timestamp", "DATETIME2"},
		{"datetime2", "timestamp", "DATETIME2"},
		{"DATETIME2", "timestamp", "DATETIME2"},
		{"datetime2(7)", "timestamp", "DATETIME2"},
		{"datetimeoffset", "timestamptz", "DATETIMEOFFSET"},
		{"datetimeoffset(7)", "timestamptz", "DATETIMEOFFSET"},
	}
	for _, tc := range cases {
		t.Run(tc.native, func(t *testing.T) {
			binding := parserBinding()
			binding.Dialect = "sqlserver"
			binding.Relations[0].Columns[1].NativeType = tc.native
			c := sqlServerTimeConstraint(tc.temporal)
			out, err := BindBusinessConstraints(context.Background(), binding, "SELECT [id] FROM [analytics].[sales]", nil, []BusinessConstraint{c})
			if err != nil || strings.Count(out.SQL, " AS "+tc.cast+")") != 2 || len(out.Parameters) != 2 {
				t.Fatal("supported SQL Server calendar type changed", err)
			}
			if out.Parameters[0].Value != c.Value || out.Parameters[1].Value != c.Upper || !strings.Contains(out.SQL, "@p1") || !strings.Contains(out.SQL, "@p2") {
				t.Fatal("calendar binding changed exact bounds or parameter positions")
			}
			if len(out.Receipt.Bindings) != 1 || !reflect.DeepEqual(out.Receipt.Bindings[0].Parameters, []int{1, 2}) || out.Receipt.Validation != nil {
				t.Fatal("calendar binding lost provenance or fabricated a read proof")
			}
		})
	}
}

func TestBusinessPostgresNativeTimestampStillSupported(t *testing.T) {
	binding := parserBinding()
	binding.Dialect = "postgres"
	binding.Relations[0].Columns[1].NativeType = "timestamp"
	c := sqlServerTimeConstraint("timestamp")
	out, err := BindBusinessConstraints(context.Background(), binding, `SELECT "id" FROM "analytics"."sales"`, nil, []BusinessConstraint{c})
	if err != nil || strings.Count(out.SQL, " AS TIMESTAMP)") != 2 || len(out.Parameters) != 2 {
		t.Fatal("SQL Server native-type rejection leaked into PostgreSQL", err)
	}
}

func sqlServerTimeConstraint(temporal string) BusinessConstraint {
	c := BusinessConstraint{}
	c.Resolution = Hash("reviewed-sqlserver-calendar-window")
	c.Dataset, c.Column, c.SourceRevision = "sales", "amount", 1
	c.Kind, c.Operator, c.Nulls = "time_window", "range", "exclude"
	c.TemporalType, c.Calendar, c.TimeZone = temporal, "gregorian", "America/Argentina/Buenos_Aires"
	c.Grain, c.Bounds = "day", "[)"
	c.Value, c.Upper = "2026-01-02", "2026-01-03"
	if temporal == "timestamptz" {
		c.Value, c.Upper = "2026-01-02T03:00:00Z", "2026-01-03T03:00:00Z"
	}
	return c
}
