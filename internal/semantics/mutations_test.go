package semantics

import (
	"strings"
	"testing"
)

func TestEntityMutationAndDatasetReplacementRewriteAllReferences(t *testing.T) {
	model, _ := testRules(t)
	pack := model.Pack()
	updated := pack.Measures[0]
	updated.Name = "Reviewed revenue"
	mutated, err := MutateEntities(model, "v2", []EntityMutation{{Operation: "put", Kind: KindMeasure, ID: updated.ID, Measure: &updated}})
	if err != nil || mutated.Pack().Measures[0].Name != updated.Name || mutated.Pack().Version != "v2" {
		t.Fatal("entity update", err)
	}
	if _, err = MutateEntities(mutated, "v3", []EntityMutation{{Operation: "delete", Kind: KindMeasure, ID: updated.ID}}); err == nil {
		t.Fatal("referenced measure deletion accepted")
	}

	pack = model.Pack()
	old := pack.Datasets[0]
	columns := append([]Column(nil), old.Columns...)
	for i := range columns {
		columns[i].SourceName = "new_" + columns[i].SourceName
	}
	source := old.Source
	source.Dataset = "orders_v2"
	source.ProfileVersion = "profile-v2"
	replacement := DatasetReplacement{Dataset: "orders_v2", Source: source, Columns: columns}
	rebound, err := ReplaceDataset(model, "v2", old.ID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	out := rebound.Pack()
	seenReplacement := false
	check := func(ref Reference) {
		if (ref.Kind == KindColumn && ref.Dataset == old.ID) || (ref.Kind == KindDataset && ref.ID == old.ID) {
			t.Fatal("old dataset reference remained", ref)
		}
		if (ref.Kind == KindColumn && ref.Dataset == "orders_v2") || (ref.Kind == KindDataset && ref.ID == "orders_v2") {
			seenReplacement = true
		}
	}
	for _, measure := range out.Measures {
		check(measure.Field)
	}
	for _, dimension := range out.Dimensions {
		check(dimension.Field)
	}
	for _, kpi := range out.KPIs {
		for _, input := range kpi.Inputs {
			check(input)
		}
	}
	for _, join := range out.Joins {
		check(join.Left)
		check(join.Right)
	}
	for _, entity := range out.CanonicalEntities {
		for _, key := range entity.Keys {
			check(key)
		}
	}
	if !seenReplacement {
		t.Fatal("replacement dataset was not referenced")
	}
	columns[0].ID = "unknown"
	if _, err = ReplaceDataset(model, "v3", old.ID, DatasetReplacement{Dataset: "orders_v3", Source: replacement.Source, Columns: columns}); err == nil {
		t.Fatal("incomplete semantic column mapping accepted")
	}
}

func TestRelationshipDecisionMutationAndDatasetReplacementPreserveReviewedMeaning(t *testing.T) {
	model, _ := testRules(t)
	pack := model.Pack()
	decision := RelationshipDecision{
		ID:          "candidate_amount_customer",
		Left:        Reference{Kind: KindColumn, Dataset: pack.Datasets[0].ID, ID: pack.Datasets[0].Columns[1].ID},
		Right:       Reference{Kind: KindColumn, Dataset: pack.Datasets[1].ID, ID: pack.Datasets[1].Columns[0].ID},
		Cardinality: CardinalityManyToOne,
		State:       "candidate",
		Evidence:    RelationshipEvidence{ID: "reviewed_grain", LeftGrain: "order", RightGrain: "customer", Provenance: "reviewed_profile"},
	}
	mutated, err := MutateEntities(model, "v2", []EntityMutation{{Operation: "put", Kind: KindRelationshipDecision, ID: decision.ID, RelationshipDecision: &decision}})
	if err != nil || len(mutated.Pack().RelationshipDecisions) != 1 {
		t.Fatal("relationship decision authoring", err)
	}

	old := mutated.Pack().Datasets[0]
	columns := append([]Column(nil), old.Columns...)
	columns[0].Aliases = nil
	columns[0].SemanticRole = ""
	oldAliases := []string{"Order key", "Pedido"}
	base := mutated.Pack()
	for i := range base.Datasets {
		if base.Datasets[i].ID == old.ID {
			base.Datasets[i].Columns[0].Aliases = oldAliases
			base.Datasets[i].Columns[0].SemanticRole = SemanticRoleFactKey
		}
	}
	mutated, err = Compile(base)
	if err != nil {
		t.Fatal(err)
	}
	source := old.Source
	source.Dataset = "orders_reviewed"
	source.ProfileVersion = "profile_reviewed"
	rebound, err := ReplaceDataset(mutated, "v3", old.ID, DatasetReplacement{Dataset: source.Dataset, Source: source, Columns: columns})
	if err != nil {
		t.Fatal(err)
	}
	got := rebound.Pack()
	var reboundColumn Column
	for _, dataset := range got.Datasets {
		if dataset.ID == source.Dataset {
			reboundColumn = dataset.Columns[0]
		}
	}
	left := got.RelationshipDecisions[0].Left
	right := got.RelationshipDecisions[0].Right
	if reboundColumn.SemanticRole != SemanticRoleFactKey || len(reboundColumn.Aliases) != 2 || (left.Dataset != source.Dataset && right.Dataset != source.Dataset) {
		t.Fatalf("reviewed meaning was not preserved across rebind: %#v", got)
	}
}

func TestDatasetReplacementRewritesEnhancedUnresolvedReference(t *testing.T) {
	model, _ := testRules(t)
	old := model.Pack().Datasets[0]
	enhanced, err := ApplyEnhancements(model, "v2", []Enhancement{{
		Dataset: old.ID,
		Column:  old.Columns[0].ID,
		Kind:    EnhancementUnresolved,
		Reason:  "Business meaning requires review",
	}})
	if err != nil {
		t.Fatal(err)
	}
	unresolved := enhanced.Pack().Unresolved[0]
	columns := append([]Column(nil), old.Columns...)
	source := old.Source
	source.Dataset = "orders_v2"
	source.ProfileVersion = "profile_v2"
	source.ProfileDigest = strings.Repeat("c", 64)
	rebound, err := ReplaceDataset(enhanced, "v3", old.ID, DatasetReplacement{Dataset: source.Dataset, Source: source, Columns: columns})
	if err != nil {
		t.Fatal("enhanced draft rebind", err)
	}
	got := rebound.Pack().Unresolved
	if len(got) != 1 || got[0].ID != unresolved.ID || got[0].Dataset != source.Dataset || got[0].Column != unresolved.Column || got[0].Reason != unresolved.Reason {
		t.Fatalf("unresolved mapping changed during rebind: %#v", got)
	}
	if got[0].ID == GeneratedEntityID(EnhancementUnresolved, source.Dataset, got[0].Column) {
		t.Fatal("rebound unresolved ID lost its stable origin", got[0].ID)
	}
	resolved, err := ApplyEnhancements(rebound, "v4", []Enhancement{{
		Dataset:     source.Dataset,
		Column:      unresolved.Column,
		Kind:        EnhancementMeasure,
		Name:        "Reviewed amount",
		Aggregation: AggregationSum,
	}})
	if err != nil {
		t.Fatal("resolve rebound semantic", err)
	}
	resolvedPack := resolved.Pack()
	if len(resolvedPack.Unresolved) != 0 {
		t.Fatalf("resolved semantic retained unresolved origin: %#v", resolvedPack.Unresolved)
	}
	wantField := Reference{Kind: KindColumn, Dataset: source.Dataset, ID: unresolved.Column}
	wantID := GeneratedEntityID(EnhancementMeasure, source.Dataset, unresolved.Column)
	found := false
	for _, measure := range resolvedPack.Measures {
		if measure.ID == wantID && measure.Field == wantField {
			found = true
		}
	}
	if !found {
		t.Fatal("resolved semantic missing executable measure", wantID, wantField)
	}
}

func TestDatasetReplacementRichValuesRequireDestinationSensitivityEvidence(t *testing.T) {
	pack := testPack()
	var old Dataset
	for i := range pack.Datasets {
		if pack.Datasets[i].ID != "customers" {
			continue
		}
		for j := range pack.Datasets[i].Columns {
			if pack.Datasets[i].Columns[j].ID == "region" {
				pack.Datasets[i].Columns[j].Sensitivity = LiteralNonSensitive
			}
		}
		old = pack.Datasets[i]
	}
	pack.Dimensions[0].Values = []GovernedValue{{ID: "north", Value: "N", Aliases: []string{"North"}, Sensitivity: LiteralNonSensitive, Provenance: ValueProvenance{Kind: "reviewed_profile", Evidence: "profile_v2", Policy: "low_cardinality"}}}
	pack.Dimensions[0].Geography = true
	pack.Dimensions[0].Filters = []SemanticFilter{{ID: "north_only", Field: Reference{Kind: KindColumn, Dataset: old.ID, ID: "region"}, Operator: "eq", Values: []string{"N"}}}
	model, err := Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	columns := append([]Column(nil), old.Columns...)
	for i := range columns {
		columns[i].Sensitivity = ""
	}
	source := old.Source
	source.Dataset = "customers_v2"
	source.ProfileVersion = "customers_profile_v3"
	if _, err = ReplaceDataset(model, "v2", old.ID, DatasetReplacement{Dataset: source.Dataset, Source: source, Columns: columns}); validationCode(t, err) != CodeEvidenceMismatch {
		t.Fatalf("rich values survived destination without sensitivity evidence: %v", err)
	}
	if model.Digest() == "" || model.Pack().Datasets[0].ID == source.Dataset {
		t.Fatal("failed rebind mutated prior model")
	}
	for i := range columns {
		if columns[i].ID == "region" {
			columns[i].Sensitivity = LiteralNonSensitive
		}
	}
	rebound, err := ReplaceDataset(model, "v2", old.ID, DatasetReplacement{Dataset: source.Dataset, Source: source, Columns: columns})
	if err != nil {
		t.Fatal(err)
	}
	got := rebound.Pack()
	if got.Dimensions[0].Field.Dataset != source.Dataset || got.Dimensions[0].Values[0].ID != "north" || !got.Dimensions[0].Geography || got.Dimensions[0].Filters[0].Field.Dataset != source.Dataset {
		t.Fatalf("evidence-backed rebind lost stable rich meaning: %#v", got.Dimensions[0])
	}
}
