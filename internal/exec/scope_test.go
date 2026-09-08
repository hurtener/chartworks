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
