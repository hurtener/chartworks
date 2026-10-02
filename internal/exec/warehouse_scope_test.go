//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestWarehouseScopedLogicalOutputsNative(t *testing.T) {
	owner, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales", "cw.dataset.query:invoices"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{}
	v, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	scope := []RelationScope{{Dataset: "sales", Columns: []string{"id", "amount"}}, {Dataset: "invoices", Columns: []string{"id", "amount"}}}
	for _, d := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(d, func(t *testing.T) {
			adapter.binding = Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: d, Contract: "contract", Fingerprint: Hash("scoped-logical-outputs")}
			for _, name := range []string{"sales", "invoices"} {
				adapter.binding.Relations = append(adapter.binding.Relations, Relation{ID: name, Schema: "analytics", Name: name, Columns: []Column{{Name: "id", NativeType: "integer", Safe: true}, {Name: "amount", NativeType: "decimal", Safe: true}, {Name: "secret", NativeType: "integer", Safe: true}}})
			}
			check := func(sql string, want bool) {
				t.Helper()
				adapter.explains = 0
				plan, err := v.ValidateWithin(context.Background(), owner, Request{Source: "source", Context: "source:v1", SQL: sql}, scope)
				if (err == nil) != want || plan.Receipt().Validated != want || adapter.explains != map[bool]int{true: 1, false: 0}[want] {
					t.Errorf("want=%v sql=%s err=%v explains=%d", want, sql, err, adapter.explains)
				}
			}
			for _, sql := range []string{
				"SELECT amount AS total FROM analytics.sales ORDER BY total DESC",
				"SELECT amount AS secret FROM analytics.sales ORDER BY secret",
				"WITH s AS (SELECT sum(amount) AS total FROM analytics.sales), i AS (SELECT sum(amount) AS total FROM analytics.invoices) SELECT s.total-i.total AS net FROM s CROSS JOIN i",
				"SELECT s.total-i.total AS net FROM (SELECT sum(amount) AS total FROM analytics.sales) s CROSS JOIN (SELECT sum(amount) AS total FROM analytics.invoices) i",
				"WITH q AS (SELECT amount AS secret FROM analytics.sales) SELECT q.secret FROM q",
				"WITH q(x) AS (SELECT amount FROM analytics.sales), r AS (SELECT x FROM q) SELECT x FROM r",
				"SELECT q.x FROM (SELECT amount FROM analytics.sales) q(x)",
				"SELECT id FROM analytics.sales UNION ALL SELECT id FROM analytics.invoices ORDER BY id",
				"SELECT s.id FROM analytics.sales s JOIN analytics.invoices i ON s.id=i.id",
				"SELECT s.id FROM analytics.sales s WHERE EXISTS (SELECT i.id FROM analytics.invoices i WHERE i.id=s.id)",
			} {
				check(sql, true)
			}
			for _, sql := range []string{
				"SELECT amount AS total FROM analytics.sales ORDER BY secret",
				"SELECT amount AS secret FROM analytics.sales WHERE secret>0 ORDER BY secret",
				"SELECT amount AS secret FROM analytics.sales GROUP BY secret",
				"SELECT id AS total,amount AS total FROM analytics.sales ORDER BY total",
				"WITH q AS (SELECT secret AS total FROM analytics.sales) SELECT total FROM q",
				"WITH q AS (SELECT amount AS secret FROM analytics.sales WHERE secret>0) SELECT q.secret FROM q",
				"WITH q AS (SELECT secret AS total FROM analytics.sales) SELECT id FROM analytics.sales",
				"SELECT q.total FROM (SELECT secret AS total FROM analytics.sales) q",
				"WITH q AS (SELECT amount AS total FROM analytics.sales) SELECT q.secret FROM q",
				"WITH q AS (SELECT * FROM analytics.sales) SELECT q.id FROM q",
				"WITH q AS (SELECT amount AS x,secret AS y FROM analytics.sales) SELECT x FROM q",
				"WITH q AS (SELECT total FROM r), r AS (SELECT amount AS total FROM analytics.sales) SELECT total FROM q",
				"SELECT id FROM analytics.sales s JOIN analytics.invoices i ON s.id=i.id",
				"SELECT s.id FROM analytics.sales s WHERE EXISTS (SELECT i.id FROM analytics.invoices i WHERE i.secret=s.id)",
				"SELECT s.id FROM analytics.sales s WHERE EXISTS (SELECT i.id FROM analytics.invoices i WHERE secret=s.id)",
				"SELECT s.id FROM analytics.sales s WHERE EXISTS (SELECT i.id FROM analytics.invoices s WHERE s.secret>0)",
				"SELECT s.id FROM analytics.sales s NATURAL JOIN analytics.invoices i",
				"SELECT s.id FROM analytics.sales s JOIN analytics.invoices i USING(id)",
			} {
				check(sql, false)
			}
		})
	}
}
