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

func TestWarehouseTransparentParenthesesNative(t *testing.T) {
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			adapter := &warehouseCatalogAdapter{binding: Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: dialect, Contract: "contract", Fingerprint: Hash("synthetic-parentheses"), Relations: []Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []Column{{Name: "id", NativeType: "integer", Safe: true}, {Name: "secret", NativeType: "integer", Safe: false}}}}}}
			validator, err := NewValidator(adapter, config.DefaultReadValidation())
			if err != nil {
				t.Fatal(err)
			}
			marker := "?"
			if dialect == "databricks" {
				marker = ":p1"
			}
			if dialect == "sqlserver" || dialect == "bigquery" {
				marker = "@p1"
			}
			check := func(ctx context.Context, sql string, params []Parameter, pass bool) {
				t.Helper()
				adapter.explains = 0
				p, err := validator.Validate(ctx, e, Request{Source: "source", Context: "source:v1", SQL: sql, Parameters: params})
				if (err == nil) != pass || p.Receipt().Validated != pass || adapter.explains != map[bool]int{true: 1, false: 0}[pass] {
					t.Fatalf("pass=%v explains=%d: %v", pass, adapter.explains, err)
				}
			}
			query := "SELECT ((sum(id))) AS total FROM analytics.sales WHERE (id = " + marker + ")"
			check(context.Background(), query, []Parameter{{Kind: "integer", Value: "1"}}, true)
			check(context.Background(), query, nil, false)
			for _, sql := range []string{
				"SELECT (mystery(id)) AS value FROM analytics.sales",
				"SELECT ((secret)) AS value FROM analytics.sales",
				"SELECT (sum(id)) AS value FROM private.sales",
				"SELECT " + strings.Repeat("(", 65) + "sum(id)" + strings.Repeat(")", 65) + " AS value FROM analytics.sales",
			} {
				check(context.Background(), sql, nil, false)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			check(ctx, query, []Parameter{{Kind: "integer", Value: "1"}}, false)
		})
	}
}
