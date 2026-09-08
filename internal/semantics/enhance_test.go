package semantics

import "testing"

func TestApplyEnhancementsPreservesStableIDsAndUnresolved(t *testing.T) {
	model, err := Compile(testPack())
	if err != nil {
		t.Fatal(err)
	}
	dataset := "orders"
	amount := ""
	for _, candidate := range model.pack.Datasets {
		if candidate.ID == dataset {
			for _, column := range candidate.Columns {
				if column.ID == "amount" {
					amount = column.ID
				}
			}
		}
	}
	if amount == "" {
		t.Fatal("fixture amount column absent")
	}
	first, err := ApplyEnhancements(model, "v2", []Enhancement{{Dataset: dataset, Column: amount, Kind: EnhancementUnresolved, Reason: "Business aggregation requires review"}})
	if err != nil || len(first.pack.Unresolved) != 1 {
		t.Fatal("retain unresolved", err)
	}
	unresolvedID := first.pack.Unresolved[0].ID
	if unresolvedID != GeneratedEntityID(EnhancementUnresolved, dataset, amount) {
		t.Fatal("unstable unresolved ID", unresolvedID)
	}
	second, err := ApplyEnhancements(first, "v3", []Enhancement{{Dataset: dataset, Column: amount, Kind: EnhancementMeasure, Name: "Amount", Aggregation: AggregationSum}})
	if err != nil || len(second.pack.Unresolved) != 0 {
		t.Fatal("resolve checkpoint", err)
	}
	want := GeneratedEntityID(EnhancementMeasure, dataset, amount)
	found := false
	for _, measure := range second.pack.Measures {
		if measure.ID == want && measure.Field == (Reference{Kind: KindColumn, Dataset: dataset, ID: amount}) {
			found = true
		}
	}
	if !found {
		t.Fatal("stable generated measure absent")
	}
	if _, err = ApplyEnhancements(model, "v2", []Enhancement{{Dataset: dataset, Column: "missing", Kind: EnhancementMeasure, Name: "Bad", Aggregation: AggregationSum}}); err == nil {
		t.Fatal("missing column enhancement accepted")
	}
}
