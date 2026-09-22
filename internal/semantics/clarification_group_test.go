package semantics

import "testing"

func TestPhysicalClarificationConjunctions(t *testing.T) {
	number := func(op, value, upper, nulls string) ClarificationResolution {
		return ClarificationResolution{Value: value, Upper: upper, Effect: &ClarificationEffect{Kind: "number", Operator: op, Unit: "USD", Nulls: nulls, Bounds: "[)"}}
	}
	for _, tc := range []struct {
		name     string
		group    []ClarificationResolution
		conflict bool
	}{
		{"contradictory-cross-topic-bounds", []ClarificationResolution{number("gte", "10", "", "exclude"), number("lt", "5", "", "exclude")}, true},
		{"compatible-intersection", []ClarificationResolution{number("gte", "10", "", "exclude"), number("lt", "20", "", "exclude")}, false},
		{"explicit-null-intersection", []ClarificationResolution{number("gte", "10", "", "include"), number("lt", "5", "", "include")}, false},
		{"exclusive-adjacent-ranges", []ClarificationResolution{number("range", "0", "10", "exclude"), number("range", "10", "20", "exclude")}, true},
		{"unreviewed-fragment", []ClarificationResolution{{Value: "10"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ClarificationResolutionGroupConflict(tc.group) != tc.conflict {
				t.Fatal("incorrect exact conjunction")
			}
		})
	}
	a, b := number("gte", "10", "", "exclude"), number("gte", "10", "", "exclude")
	b.Effect.Unit = "kg"
	if !ClarificationResolutionGroupConflict([]ClarificationResolution{a, b}) {
		t.Fatal("incompatible units composed")
	}
}
