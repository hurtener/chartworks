package nlqexec

import (
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

// This characterizes the retained compiler boundary, not a new admission policy.
// Both selected SUM leaves already require the exact sales paid population; the
// refund leaf reaches that field through the reviewed complete-key parent join.
func scalarPredicateCharacterization(t *testing.T, activity bool) (admission, []exec.BusinessConstraint) {
	t.Helper()
	a := scalarPeriodAdmission(t, activity)
	a.publications[0].Definition.Measures[0].Filters = []semantics.SemanticFilter{{
		ID: "paid-orders", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Operator: "eq", Values: []string{"paid"},
	}}
	analyticalReseal(&a)
	a.metricPeriods[0].PackDigest = a.publications[0].Digest
	a.route.Resolutions = []semantics.ClarificationResolution{{Topic: "sales_topic", Pattern: "paid-population", Slot: "status"}}
	a.route.SourceBindingDigest = exec.Hash(a.binding)
	return a, []exec.BusinessConstraint{{Resolution: exec.Hash("explicit-reviewed-paid"), Dataset: "sales", Column: "region_native", SourceRevision: 1, Kind: "text", Operator: "eq", Nulls: "exclude", Value: "paid"}}
}

func TestScalarPredicateCharacterizationRetainedCompilerBoundary(t *testing.T) {
	for _, activity := range []bool{false, true} {
		a, constraints := scalarPredicateCharacterization(t, activity)
		baseline, err := compileAnalyticalVersion(t.Context(), a, 10, []exec.BusinessConstraint{})
		if err != nil || baseline == nil || baseline.ScalarPopulations == nil || len(baseline.ScalarPopulations.Lanes) != 2 {
			t.Fatal("reviewed scalar control must compile", activity, err)
		}
		leaves := baseline.Metrics[0].Expression.Args
		if len(leaves) != 2 || len(leaves[0].Filters) != 1 || len(leaves[1].Filters) != 1 || leaves[0].Filters[0].Column != "region_native" || leaves[1].Filters[0].Column != "region_native" || leaves[0].Filters[0].Values[0] != "paid" || leaves[1].Filters[0].Values[0] != "paid" {
			t.Fatal("fixture lost full selected-leaf paid entailment")
		}
		if join := baseline.ScalarPopulations.Lanes[0].Joins; len(join) != 1 || len(join[0].LeftColumns) != 2 {
			t.Fatal("refund parent lost complete reviewed keys")
		}
		// Retained version 12 preserves its original owned-constraint dispatch. It falls back to
		// the scalar implementation; retained v9/v10 reject the same constraint.
		for _, version := range []int{9, 10, 12} {
			got, err := compileAnalyticalVersion(t.Context(), a, version, constraints)
			var detail *exec.AnalyticalError
			if got != nil || !errors.As(err, &detail) || detail.Code != "analytical_shape_unsupported" {
				t.Fatalf("retained scalar boundary changed: activity=%t version=%d contract=%v error=%v", activity, version, got, err)
			}
			t.Logf("activity=%t version=%d redundant paid predicate rejected: %s", activity, version, detail.Code)
		}
	}
}

func TestScalarPredicateCharacterizationPreservesCurrentNegatives(t *testing.T) {
	for name, change := range map[string]func(*admission, []exec.BusinessConstraint){
		"one_selected_leaf_not_entailed": func(a *admission, _ []exec.BusinessConstraint) {
			a.publications[0].Definition.Measures[0].Filters = nil
			analyticalReseal(a)
			a.metricPeriods[0].PackDigest = a.publications[0].Digest
		},
		"nonentailed_value": func(_ *admission, c []exec.BusinessConstraint) { c[0].Value = "cancelled" },
		"wrong_fact":        func(_ *admission, c []exec.BusinessConstraint) { c[0].Dataset = "refunds" },
		"wrong_source":      func(a *admission, _ []exec.BusinessConstraint) { a.binding.Source = "other" },
		"wrong_revision":    func(_ *admission, c []exec.BusinessConstraint) { c[0].SourceRevision++ },
		"wrong_relationship": func(a *admission, _ []exec.BusinessConstraint) {
			a.publications[0].Definition.Measures[2].Filters[0].Relationship = "unreviewed-parent"
			analyticalReseal(a)
			a.metricPeriods[0].PackDigest = a.publications[0].Digest
		},
		"null_only": func(_ *admission, c []exec.BusinessConstraint) { c[0].Nulls, c[0].Value = "only", "" },
	} {
		t.Run(name, func(t *testing.T) {
			a, constraints := scalarPredicateCharacterization(t, false)
			change(&a, constraints)
			if got, err := compileAnalyticalVersion(t.Context(), a, 12, constraints); got != nil || err == nil {
				t.Fatal("unproved scalar predicate accepted", got, err)
			}
		})
	}
}
