//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec/signatureparser"
)

func TestWarehouseUTCInstantScope(t *testing.T) {
	binding := Binding{Dialect: "mysql", Relations: []Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []Column{{Name: "stamp", NativeType: "timestamp(6)", Safe: true}, {Name: "civil", NativeType: "datetime(6)", Safe: true}, {Name: "secret", NativeType: "timestamp(6)", Safe: false}}}}}
	for _, tc := range []struct {
		sql  string
		want bool
	}{
		{"SELECT CAST(stamp AT TIME ZONE '+00:00' AS DATETIME(6)) AS utc FROM analytics.sales", true},
		{"SELECT CAST(s.stamp AT TIME ZONE 'UTC' AS DATETIME(6)) AS utc FROM analytics.sales s", true},
		{"SELECT CAST(civil AT TIME ZONE '+00:00' AS DATETIME(6)) AS utc FROM analytics.sales", false},
		{"SELECT CAST(secret AT TIME ZONE '+00:00' AS DATETIME(6)) AS utc FROM analytics.sales", false},
		{"SELECT CAST(stamp AT TIME ZONE 'America/New_York' AS DATETIME(6)) AS utc FROM analytics.sales", false},
		{"SELECT CAST(stamp AT TIME ZONE '+01:00' AS DATETIME(6)) AS utc FROM analytics.sales", false},
		{"SELECT CAST(stamp AT TIME ZONE civil AS DATETIME(6)) AS utc FROM analytics.sales", false},
		{"SELECT CAST(stamp AT TIME ZONE '+00:00' AS DATETIME(3)) AS utc FROM analytics.sales", false},
		{"SELECT CAST(stamp AT TIME ZONE '+00:00' AS DATETIME) AS utc FROM analytics.sales", false},
		{"SELECT stamp AT TIME ZONE '+00:00' AS utc FROM analytics.sales", false},
		{"SELECT CAST(coalesce(stamp,stamp) AT TIME ZONE '+00:00' AS DATETIME(6)) AS utc FROM analytics.sales", false},
		{"WITH q AS (SELECT stamp FROM analytics.sales) SELECT CAST(stamp AT TIME ZONE '+00:00' AS DATETIME(6)) AS utc FROM q", false},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			ast, err := signatureparser.Inspect(context.Background(), tc.sql, "mysql", 100000, 64)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = warehouseResolveScope(context.Background(), ast, binding, binding, 0)
			if (err == nil) != tc.want {
				t.Fatalf("want=%v got=%v", tc.want, err)
			}
		})
	}
}

func TestWarehouseUTCInstantValidatorNative(t *testing.T) {
	owner, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{binding: Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: "mysql", Contract: "contract", Fingerprint: Hash("utc-instant"), Relations: []Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []Column{{Name: "stamp", NativeType: "timestamp(6)", Safe: true}, {Name: "civil", NativeType: "datetime(6)", Safe: true}, {Name: "secret", NativeType: "timestamp(6)", Safe: false}}}}}}
	validator, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"CAST(stamp AT TIME ZONE '+00:00' AS DATETIME(6))", true},
		{"DATE_FORMAT(CAST(stamp AT TIME ZONE 'UTC' AS DATETIME(6)),'%Y-%m-01')", true},
		{"CAST(civil AT TIME ZONE '+00:00' AS DATETIME(6))", false},
		{"CAST(secret AT TIME ZONE '+00:00' AS DATETIME(6))", false},
		{"CAST(stamp AT TIME ZONE 'America/New_York' AS DATETIME(6))", false},
		{"CAST(stamp AT TIME ZONE '+00:00' AS DATETIME(3))", false},
		{"stamp AT TIME ZONE '+00:00'", false},
	} {
		adapter.explains = 0
		plan, err := validator.Validate(context.Background(), owner, Request{Source: "source", Context: "source:v1", SQL: "SELECT " + tc.expr + " AS utc FROM analytics.sales"})
		if (err == nil) != tc.want || plan.Receipt().Validated != tc.want || adapter.explains != map[bool]int{true: 1, false: 0}[tc.want] {
			t.Errorf("%s want=%v err=%v explains=%d", tc.expr, tc.want, err, adapter.explains)
		}
	}
}
