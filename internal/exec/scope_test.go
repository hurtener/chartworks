package exec

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestSemanticScopeOnlyNarrowsDetachedBindings(t *testing.T) {
	b := parserBinding()
	original := Hash(b)
	if got, err := narrowBinding(b, nil); err != nil || !reflect.DeepEqual(got, b) {
		t.Fatal("ordinary mode changed", err)
	}
	scope := []RelationScope{{Dataset: "sales", Columns: []string{"id", "amount"}}}
	got, err := narrowBinding(b, scope)
	if err != nil || len(got.Relations) != 1 || len(got.Relations[0].Columns) != 2 || got.Relations[0].Columns[0].Name != "amount" {
		t.Fatal("scope", got, err)
	}
	got.Relations[0].Columns[0].Name = "mutated"
	if Hash(b) != original || scope[0].Columns[0] != "id" {
		t.Fatal("caller data mutated")
	}
	for _, bad := range [][]RelationScope{
		{}, make([]RelationScope, 33), {{Dataset: "missing", Columns: []string{"id"}}},
		{{Dataset: "sales"}}, {{Dataset: "sales", Columns: make([]string, 257)}},
		{{Dataset: "sales", Columns: []string{"id", "id"}}}, {{Dataset: "sales", Columns: []string{"secret"}}},
		{{Dataset: "sales", Columns: []string{"custom"}}},
		{{Dataset: "sales", Columns: []string{"id"}}, {Dataset: "sales", Columns: []string{"amount"}}},
	} {
		if _, err := narrowBinding(b, bad); !errors.Is(err, ErrBinding) {
			t.Fatal("scope widened", err)
		}
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := narrowBinding(b, scope); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestScopedValidatorUsesCommonProofAndPhysicalBinding(t *testing.T) {
	binding := parserBinding()
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:*"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &warehouseCatalogAdapter{binding: binding}
	v, err := NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Source: "source", Context: "source:v1", SQL: "SELECT id FROM analytics.sales"}
	ordinary, err := v.Validate(context.Background(), e, request)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := v.ValidateWithin(context.Background(), e, request, []RelationScope{{Dataset: "sales", Columns: []string{"id"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Receipt().Validated || plan.Receipt().Manifest == ordinary.Receipt().Manifest {
		t.Fatal("scope missing from proof")
	}
	if _, _, err = plan.SQL(e, binding); err != nil {
		t.Fatal("actual source binding lost", err)
	}
	if _, _, err = plan.SQL(e, Binding{}); err == nil {
		t.Fatal("wrong source accepted")
	}
	for _, scope := range [][]RelationScope{nil, {}, {{Dataset: "items", Columns: []string{"quantity"}}}, {{Dataset: "sales", Columns: []string{"amount"}}}, {{Dataset: "sales", Columns: []string{"missing"}}}} {
		before := adapter.explains
		if _, err = v.ValidateWithin(context.Background(), e, request, scope); err == nil || adapter.explains != before {
			t.Fatal("scope denial reached native plan", err)
		}
	}
	for _, dialect := range []string{"mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			adapter.binding = binding.Clone()
			adapter.binding.Dialect = dialect
			p, err := v.ValidateWithin(context.Background(), e, request, []RelationScope{{Dataset: "sales", Columns: []string{"id"}}})
			if err != nil || !p.Receipt().Validated {
				t.Fatal("warehouse scope", err)
			}
			request.SQL = "SELECT name FROM analytics.sales"
			before := adapter.explains
			if _, err = v.ValidateWithin(context.Background(), e, request, []RelationScope{{Dataset: "sales", Columns: []string{"id"}}}); err == nil || adapter.explains != before {
				t.Fatal("warehouse column widened", err)
			}
			request.SQL = "SELECT id FROM analytics.sales"
		})
	}
}

func TestWarehouseParameterFallbackIsClosed(t *testing.T) {
	for _, test := range []struct {
		dialect, statement string
		want               int
	}{
		{"mysql", "SELECT id FROM analytics.sales WHERE name LIKE ? ESCAPE '!'", 1},
		{"sqlserver", "SELECT id FROM analytics.sales WHERE name LIKE @p1 ESCAPE '!' AND id>@p2", 2},
		{"bigquery", "SELECT id FROM `warehouse.analytics.sales` WHERE name LIKE @p1 ESCAPE '!'", 1},
		{"snowflake", "SELECT id FROM warehouse.analytics.sales WHERE name LIKE ? ESCAPE '!'", 1},
		{"databricks", "SELECT id FROM warehouse.analytics.sales WHERE name LIKE ? ESCAPE '!'", 1},
	} {
		got, err := warehouseParameterCount(t.Context(), test.statement, test.dialect)
		if err != nil || got != test.want {
			t.Fatal(test.dialect, got, err)
		}
	}
	if got, err := warehouseParameterCount(t.Context(), "SELECT '@p1', id FROM analytics.sales", "sqlserver"); err != nil || got != 0 {
		t.Fatal("literal marker counted", got, err)
	}
	if _, err := warehouseParameterCount(t.Context(), "SELECT id FROM analytics.sales -- @p1\n", "sqlserver"); err == nil {
		t.Fatal("comment-bearing fallback accepted")
	}
	if _, err := warehouseParameterCount(t.Context(), "SELECT id FROM analytics.sales WHERE id>@p2", "sqlserver"); err == nil {
		t.Fatal("non-contiguous marker accepted")
	}
}
