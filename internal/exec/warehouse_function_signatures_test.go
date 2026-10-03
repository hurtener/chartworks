//go:build cgo && (linux || darwin)

package exec

import (
	"context"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"testing"
	"time"
)

func TestWarehouseFunctionSignaturesNative(t *testing.T) {
	owner, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{}
	validator, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			adapter.binding = Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: dialect, Contract: "contract", Fingerprint: Hash("synthetic"), Relations: []Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []Column{{Name: "id", NativeType: "integer", Safe: true}}}}}
			positive := []string{"nullif(sum(id),0)", "abs(id)", "avg(id)", "min(id)", "max(id)", "lower('abc')", "sum(id)", "count(*)", "round(sum(id),2)", "upper('abc')", "coalesce(id,0)", "sum(id) OVER ()"}
			if dialect == "sqlserver" {
				positive = append(positive, "round(id,2,1)", "len('abc')")
			} else {
				positive = append(positive, "round(id)", "length('abc')")
			}
			if dialect == "bigquery" {
				positive = append(positive, "round(CAST(1.25 AS DECIMAL(10,2)),1,'ROUND_HALF_EVEN')", "round(coalesce(1,CAST(1.25 AS DECIMAL(10,2))),1,'ROUND_HALF_EVEN')")
			}
			if dialect == "snowflake" {
				positive = append(positive, "round(CAST(1.25 AS DECIMAL(10,2)),1,'HALF_TO_EVEN')")
			}
			for _, expr := range positive {
				adapter.explains = 0
				plan, err := validator.Validate(context.Background(), owner, Request{Source: "source", Context: "source:v1", SQL: "SELECT " + expr + " AS checked FROM analytics.sales"})
				if err != nil || !plan.Receipt().Validated || adapter.explains != 1 {
					t.Errorf("valid native call %s: %v explains=%d", expr, err, adapter.explains)
				}
			}
			negative := []string{"nullif()", "nullif(id)", "nullif(id,1,2)", "avg()", "min()", "max()", "lower()", "upper()", "length()", "sum()", "count()", "count(id,2)", "coalesce()", "abs(id,2)", "round(id,2,1,0)", "abs()", "lower('a') OVER ()", "sum(id) WITHIN GROUP (ORDER BY id)", "sum(mystery(id))", "sum(secret)"}
			if dialect == "bigquery" || dialect == "snowflake" {
				negative = append(negative, "round(id,2,'NOT_A_MODE')")
			}
			if dialect != "databricks" {
				negative = append(negative, "sum(id) FILTER (WHERE id > 0)")
			}
			if dialect == "bigquery" {
				negative = append(negative, "abs(CAST('abc' AS STRING))", "lower(1)", "round(1.25,1,'ROUND_HALF_EVEN')", "round(avg(1),1,'ROUND_HALF_EVEN')")
			}
			if dialect == "sqlserver" {
				negative = append(negative, "round(id)")
			} else {
				negative = append(negative, "round(id,2,1)")
			}
			for _, expr := range negative {
				adapter.explains = 0
				plan, err := validator.Validate(context.Background(), owner, Request{Source: "source", Context: "source:v1", SQL: "SELECT " + expr + " AS checked FROM analytics.sales"})
				if err == nil || plan.Receipt().Validated || adapter.explains != 0 {
					t.Errorf("invalid native call %s: %v explains=%d", expr, err, adapter.explains)
				}
			}
		})
	}
}
