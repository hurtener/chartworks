package nlqbyo

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// This seam observes the unchanged SQL/typed parameters delivered to native
// planning. The production validator/parser is real; no live cloud is claimed.
type parameterContractAdapter struct {
	binding  exec.Binding
	explains int
}

func (a *parameterContractAdapter) Binding(context.Context, identity.Envelope, string, string) (exec.Binding, error) {
	return a.binding.Clone(), nil
}

func (a *parameterContractAdapter) Explain(_ context.Context, e identity.Envelope, candidate exec.Candidate) error {
	a.explains++
	_, _, err := candidate.SQL(e, a.binding)
	return err
}

func TestParameterContractMatchesSharedValidator(t *testing.T) {
	e, err := identity.FromVerified("tenant", "actor", "session", []string{
		"sources.query", "cw.source.query:source", "cw.execution_context.use:context", "cw.dataset.query:sales",
	}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &parameterContractAdapter{}
	validator, err := exec.NewValidator(adapter, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ dialect, style, first string }{
		{"postgres", "$1, $2, ...", "$1"},
		{"mysql", "? (positional)", "?"},
		{"sqlserver", "@p1, @p2, ...", "@p1"},
		{"bigquery", "@p1, @p2, ...", "@p1"},
		{"snowflake", "? (positional)", "?"},
		{"databricks", ":p1, :p2, ...", ":p1"},
	} {
		t.Run(tc.dialect, func(t *testing.T) {
			if got := parameterStyle(tc.dialect); got != tc.style {
				t.Fatalf("advertised parameter style %q, want %q", got, tc.style)
			}
			adapter.binding = exec.Binding{
				Tenant: "tenant", Source: "source", Context: "context", Revision: 1,
				Dialect: tc.dialect, Contract: "contract", Fingerprint: exec.Hash("synthetic"),
				Relations: []exec.Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []exec.Column{{Name: "id", NativeType: "integer", Safe: true}}}},
			}
			// Construct SQL from the advertised marker rather than a second dialect
			// switch that could silently disagree with the public bundle contract.
			marker := strings.Split(strings.Fields(parameterStyle(tc.dialect))[0], ",")[0]
			if marker != tc.first {
				t.Fatalf("first marker %q, want %q", marker, tc.first)
			}
			request := exec.Request{Source: "source", Context: "context", SQL: "SELECT id FROM analytics.sales WHERE id > " + marker, Parameters: []exec.Parameter{{Kind: "integer", Value: "7"}}}
			adapter.explains = 0
			plan, err := validator.ValidateWithin(t.Context(), e, request, []exec.RelationScope{{Dataset: "sales", Columns: []string{"id"}}})
			if err != nil || !plan.Receipt().Validated || adapter.explains != 1 {
				t.Fatalf("advertised SQL rejected before native planning: %v", err)
			}
			statement, parameters, err := plan.SQL(e, adapter.binding)
			if err != nil || statement != request.SQL || !reflect.DeepEqual(parameters, request.Parameters) {
				t.Fatal("parameter markers or bound values changed", err)
			}
			for _, bad := range [][]exec.Parameter{nil, {{Kind: "integer", Value: "7"}, {Kind: "integer", Value: "8"}}} {
				request.Parameters = bad
				adapter.explains = 0
				if _, err := validator.ValidateWithin(t.Context(), e, request, []exec.RelationScope{{Dataset: "sales", Columns: []string{"id"}}}); err == nil || adapter.explains != 0 {
					t.Fatal("missing or extra parameter reached native planning")
				}
			}
		})
	}
}
