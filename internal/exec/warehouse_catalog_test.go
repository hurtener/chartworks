package exec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

type warehouseCatalogAdapter struct {
	binding  Binding
	explains int
}

func (a *warehouseCatalogAdapter) Binding(context.Context, identity.Envelope, string, string) (Binding, error) {
	return a.binding.Clone(), nil
}

func (a *warehouseCatalogAdapter) Explain(_ context.Context, e identity.Envelope, candidate Candidate) error {
	a.explains++
	_, _, err := candidate.SQL(e, a.binding)
	return err
}

func TestWarehouseCatalogResolution(t *testing.T) {
	e, err := identity.FromVerified("tenant", "actor", "session", []string{
		"sources.query",
		"cw.source.query:source",
		"cw.execution_context.use:source:v1",
		"cw.dataset.query:sales",
	}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{}
	validator, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		dialect   string
		catalog   string
		schema    string
		table     string
		qualified string
		foreign   string
	}{
		{name: "mysql", dialect: "mysql", catalog: "analytics", schema: "analytics", table: "sales", qualified: "analytics.sales", foreign: "foreign.analytics.sales"},
		{name: "sqlserver", dialect: "sqlserver", catalog: "warehouse", schema: "analytics", table: "sales", qualified: "warehouse.analytics.sales", foreign: "foreign.analytics.sales"},
		{name: "bigquery", dialect: "bigquery", catalog: "synthetic-project", schema: "analytics", table: "sales", qualified: "`synthetic-project.analytics.sales`", foreign: "`foreign-project.analytics.sales`"},
		{name: "snowflake", dialect: "snowflake", catalog: "database", schema: "analytics", table: "sales", qualified: "database.analytics.sales", foreign: "foreign.analytics.sales"},
		{name: "databricks", dialect: "databricks", catalog: "catalog", schema: "analytics", table: "sales", qualified: "catalog.analytics.sales", foreign: "foreign.analytics.sales"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter.binding = Binding{
				Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1,
				Dialect: test.dialect, Catalog: test.catalog, Contract: "contract", Fingerprint: Hash("synthetic"),
				Relations: []Relation{{ID: "sales", Schema: test.schema, Name: test.table, Columns: []Column{{Name: "id", NativeType: "integer", Safe: true}}}},
			}
			for _, table := range []string{test.schema + "." + test.table, test.qualified} {
				adapter.explains = 0
				plan, validateErr := validator.Validate(t.Context(), e, Request{Source: "source", Context: "source:v1", SQL: "SELECT id FROM " + table})
				if validateErr != nil || !plan.Receipt().Validated || adapter.explains != 1 {
					t.Fatalf("valid qualification %q rejected: %v", table, validateErr)
				}
			}
			adapter.explains = 0
			plan, validateErr := validator.Validate(t.Context(), e, Request{Source: "source", Context: "source:v1", SQL: "SELECT id FROM " + test.foreign})
			if !errors.Is(validateErr, ErrUnsafe) || plan.Receipt().Validated || adapter.explains != 0 {
				t.Fatalf("foreign catalog %q reached native planning: %#v %v", test.foreign, plan.Receipt(), validateErr)
			}
		})
	}
}

func TestWarehouseQualifiedNameRequiresRecordedCatalog(t *testing.T) {
	binding := Binding{Dialect: "bigquery", Relations: []Relation{{Schema: "analytics", Name: "sales"}}}
	if warehouseRelationMatches(binding, binding.Relations[0], "project.analytics.sales", false) {
		t.Fatal("qualified name matched a legacy binding without a catalog")
	}
	binding.Catalog = "project"
	if warehouseRelationMatches(binding, binding.Relations[0], "server.project.analytics.sales", false) {
		t.Fatal("four-part name crossed a catalog component boundary")
	}
}
