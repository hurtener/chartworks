//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestDatabricksNamedParameterContractNative(t *testing.T) {
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{binding: Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: "databricks", Contract: "contract", Fingerprint: Hash("named-parameter-contract"), Relations: []Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []Column{{Name: "id", NativeType: "integer", Safe: true}, {Name: "name", NativeType: "string", Safe: true}}}}}}
	validator, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	one := []Parameter{{Kind: "integer", Value: "7"}}
	two := append(append([]Parameter(nil), one...), Parameter{Kind: "integer", Value: "8"})
	for _, tc := range []struct {
		sql    string
		params []Parameter
		pass   bool
	}{
		{"SELECT id FROM analytics.sales WHERE id>:p1", one, true},
		{"SELECT id FROM analytics.sales WHERE id>:p1 OR id=:p1", one, true},
		{"SELECT id FROM analytics.sales WHERE id>:p2 AND id<:p1", two, true},
		{"SELECT id FROM analytics.sales WHERE id IS NOT DISTINCT FROM :p1", one, true},
		{"SELECT id FROM analytics.sales WHERE name=':p1'", nil, true},
		{"SELECT id FROM analytics.sales WHERE id>?", one, false},
		{"SELECT id FROM analytics.sales WHERE id>:p1", nil, false},
		{"SELECT id FROM analytics.sales WHERE id>:p1", two, false},
		{"SELECT id FROM analytics.sales WHERE id>:p2", one, false},
		{"SELECT id FROM analytics.sales WHERE id>:p01", one, false},
		{"SELECT id FROM analytics.sales WHERE id>:P1", one, false},
		{"SELECT id FROM analytics.sales WHERE id>@p1", one, false},
		{"SELECT id FROM analytics.sales WHERE name=':p1'", one, false},
	} {
		adapter.explains = 0
		plan, err := validator.Validate(context.Background(), e, Request{Source: "source", Context: "source:v1", SQL: tc.sql, Parameters: tc.params})
		if (err == nil) != tc.pass || plan.Receipt().Validated != tc.pass || adapter.explains != map[bool]int{true: 1, false: 0}[tc.pass] {
			t.Fatalf("pass=%v SQL=%s err=%v explains=%d", tc.pass, tc.sql, err, adapter.explains)
		}
	}
	profile, err := sqlpolicy.ForDialect("databricks")
	if err != nil || profile.ParameterStyle != "colon-p-numbered" || businessPlaceholder("databricks", 2) != ":p2" {
		t.Fatal("advertised and rendered marker drift")
	}
}
