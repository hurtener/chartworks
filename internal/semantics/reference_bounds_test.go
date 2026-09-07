package semantics

import (
	"strings"
	"testing"
)

func TestSortedReferencesRejectOversizedCoordinatesDuringShapeValidation(t *testing.T) {
	oversized := strings.Repeat("x", 1<<20)
	for _, tc := range []struct {
		name string
		ref  Reference
	}{
		{"id", Reference{Kind: KindMeasure, ID: oversized}},
		{"dataset", Reference{Kind: KindColumn, Dataset: oversized, ID: "amount"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refs := make([]Reference, 32)
			for i := range refs {
				refs[i] = tc.ref
			}
			t.Run("kpi_inputs", func(t *testing.T) {
				pack := testPack()
				pack.KPIs[0].Inputs = refs
				// This stage precedes cloning/canonicalOrder. Checking it directly
				// proves early rejection without a runtime-specific allocation cap.
				if got := validationCode(t, validateShape(pack)); got != CodeInvalidReference {
					t.Fatalf("pre-sort shape code=%s", got)
				}
				_, err := Compile(pack)
				if got := validationCode(t, err); got != CodeInvalidReference {
					t.Fatalf("compiler code=%s", got)
				}
			})
			for _, collection := range []string{"rule_scope", "pattern_targets"} {
				t.Run(collection, func(t *testing.T) {
					model, rules := testRules(t)
					if collection == "rule_scope" {
						rules.Rules[0].Scope.Targets = refs
					} else {
						rules.Patterns[0].Targets = refs
					}
					if got := validationCode(t, validateRuleShape(rules)); got != CodeInvalidReference {
						t.Fatalf("pre-sort shape code=%s", got)
					}
					_, err := CompileRules(model, rules)
					if got := validationCode(t, err); got != CodeInvalidReference {
						t.Fatalf("compiler code=%s", got)
					}
				})
			}
		})
	}
}
