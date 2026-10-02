//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"strings"
	"testing"
	"time"
)

func TestWarehouseNullSafeEqualityNative(t *testing.T) {
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			adapter := &warehouseCatalogAdapter{binding: Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: dialect, Contract: "contract", Fingerprint: Hash("synthetic-null-equality"), Relations: []Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []Column{{Name: "id", NativeType: "integer", Safe: true}, {Name: "secret", NativeType: "integer", Safe: false}}}}}}
			validator, err := NewValidator(adapter, config.DefaultReadValidation())
			if err != nil {
				t.Fatal(err)
			}
			op := " IS NOT DISTINCT FROM "
			if dialect == "mysql" {
				op = " <=> "
			}
			marker := "?"
			if dialect == "databricks" {
				marker = ":p1"
			}
			if dialect == "sqlserver" || dialect == "bigquery" {
				marker = "@p1"
			}
			for _, tc := range []struct {
				left, right string
				params      []Parameter
				pass        bool
			}{
				{"id", "NULL", nil, true}, {"id", marker, []Parameter{{Kind: "integer", Value: "1"}}, true},
				{"secret", "NULL", nil, false}, {"id", "mystery(id)", nil, false}, {"id", marker, nil, false},
				{"id", strings.Repeat("(", 65) + "id" + strings.Repeat(")", 65), nil, false},
			} {
				adapter.explains = 0
				sql := "SELECT id FROM analytics.sales WHERE " + tc.left + op + tc.right
				_, err := validator.Validate(context.Background(), e, Request{Source: "source", Context: "source:v1", SQL: sql, Parameters: tc.params})
				if (err == nil) != tc.pass || adapter.explains != map[bool]int{true: 1, false: 0}[tc.pass] {
					t.Fatalf("left=%s right=%s pass=%v explains=%d error=%v", tc.left, tc.right, tc.pass, adapter.explains, err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			adapter.explains = 0
			if _, err := validator.Validate(ctx, e, Request{Source: "source", Context: "source:v1", SQL: "SELECT id FROM analytics.sales WHERE id" + op + "NULL"}); err == nil || adapter.explains != 0 {
				t.Fatal("canceled equality validation")
			}
		})
	}
}
