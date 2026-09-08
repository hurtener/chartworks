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
}
