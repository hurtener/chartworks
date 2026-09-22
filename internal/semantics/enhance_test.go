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

func TestApplyRichEnhancementsCreatesReviewableMeaning(t *testing.T) {
	model, err := Compile(testPack())
	if err != nil {
		t.Fatal(err)
	}
	generatedMeasure := GeneratedEntityID(EnhancementMeasure, "orders", "amount")
	changed, err := ApplyRichEnhancements(model, "v2", []Enhancement{{
		Dataset: "orders", Column: "amount", Kind: EnhancementMeasure, Name: "Net sales", Description: "Reviewed net sales amount.", Aggregation: AggregationSum, Unit: "currency", Aliases: []string{"Revenue", "Ingresos"}, SemanticRole: SemanticRoleMeasureInput,
	}}, []KPI{{ID: "sales_index", Name: "Sales index", Description: "Indexed sales.", Expression: "net sales divided by target", Inputs: []Reference{{Kind: KindMeasure, ID: generatedMeasure}}}}, []RelationshipDecision{{
		ID: "amount_region_candidate", Left: Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}, Right: Reference{Kind: KindColumn, Dataset: "customers", ID: "region"}, Cardinality: CardinalityManyToMany, State: "candidate", Evidence: RelationshipEvidence{ID: "profile_join_evidence", LeftGrain: "order", RightGrain: "region", Provenance: "reviewed_profile"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	pack := changed.Pack()
	found := false
	for _, measure := range pack.Measures {
		if measure.ID == generatedMeasure {
			found = measure.Unit == "currency" && len(measure.Aliases) == 2 && measure.Description != ""
		}
	}
	if !found || len(pack.KPIs) != 3 || len(pack.RelationshipDecisions) != 1 || pack.RelationshipDecisions[0].State != "candidate" {
		t.Fatalf("rich enhancement lost reviewable meaning: %#v", pack)
	}
	for _, dataset := range pack.Datasets {
		if dataset.ID == "orders" {
			for _, column := range dataset.Columns {
				if column.ID == "amount" && column.SemanticRole != SemanticRoleMeasureInput {
					t.Fatal("semantic role not persisted")
				}
			}
		}
	}
}
