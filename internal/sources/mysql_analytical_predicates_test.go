//go:build cgo && (linux || darwin)

package sources

import (
	"errors"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"testing"
	"time"
)

func TestSQLRecoveryMySQLOwnedAnalyticalPredicatesLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	constraint := readexec.BusinessConstraint{Resolution: readexec.Hash("synthetic-owned-category"), Dataset: f.dataset, Column: "category", SourceRevision: f.binding.Revision, Kind: "entity", Operator: "eq", Nulls: "exclude", Value: "A"}
	population, err := readexec.NewAnalyticalQueryPopulation(t.Context(), f.binding, f.dataset, []readexec.BusinessConstraint{constraint})
	if err != nil {
		t.Fatal(err)
	}
	c := readexec.AnalyticalContract{Version: readexec.AnalyticalGroupedPopulationsVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("synthetic-reviewed-predicate"), Dataset: f.dataset, Metrics: []readexec.AnalyticalMetric{{ID: "revenue", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount"}}}, Intent: &readexec.AnalyticalIntent{Policy: readexec.AnalyticalIntentPolicy}, QueryPopulation: population}
	for _, suffix := range []string{"", " WHERE amount>10", " LIMIT 1"} {
		bound, err := readexec.BindBusinessConstraints(t.Context(), f.binding, "SELECT sum(amount) AS value FROM "+f.schema+".sales"+suffix, nil, []readexec.BusinessConstraint{constraint})
		if err != nil {
			t.Fatal(err)
		}
		plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: bound.SQL, Parameters: bound.Parameters})
		if err != nil {
			t.Fatal("native service-owned predicate", err)
		}
		proof, err := readexec.CheckAnalyticalPlan(t.Context(), plan, c)
		if suffix != "" {
			if !errors.Is(err, readexec.ErrAnalyticalMismatch) || proof != nil {
				t.Fatal("extra narrowing admitted", err)
			}
			continue
		}
		if err != nil || proof.QueryPopulation != readexec.AnalyticalQueryPopulationPolicy {
			t.Fatal("owned predicate proof", err)
		}
		result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, "00000000000000000000000000000501", &cloudObserverCapture{})
		if err != nil || len(result.Result.Rows) != 1 || string(result.Result.Rows[0][0]) != `"20.00"` {
			t.Fatalf("owned predicate result: %v %#v", err, result.Result.Rows)
		}
		changed := append([]readexec.Parameter(nil), bound.Parameters...)
		changed[0].Value = "B"
		wrong, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: bound.SQL, Parameters: changed})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := readexec.CheckAnalyticalPlan(t.Context(), wrong, c); !errors.Is(err, readexec.ErrAnalyticalMismatch) {
			t.Fatal("changed private binding admitted", err)
		}
	}
}
