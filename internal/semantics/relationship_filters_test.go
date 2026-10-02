package semantics

import (
	"reflect"
	"strings"
	"testing"
)

func relationshipFilterPack() TopicPack {
	p := metricPeriodPack()
	p.Datasets[1].Columns[1].Sensitivity = LiteralNonSensitive
	p.Joins[0].Evidence = RelationshipEvidence{ID: "reviewed_join", LeftGrain: "order", RightGrain: "customer", Provenance: "reviewed_profile"}
	p.Measures[0].Filters = []SemanticFilter{{ID: "eligible_parent", Field: Reference{Kind: KindColumn, Dataset: "customers", ID: "region"}, Operator: "eq", Values: []string{"north"}, Relationship: p.Joins[0].ID}}
	return p
}
func TestRelationshipFilterPersistsAndCannotStrandJoin(t *testing.T) {
	model, err := Compile(relationshipFilterPack())
	if err != nil {
		t.Fatal(err)
	}
	original := model.Pack()
	filtered := -1
	for i, m := range original.Measures {
		if len(m.Filters) > 0 {
			filtered = i
		}
	}
	copy := model.Pack()
	copy.Measures[filtered].Filters[0].Relationship = "other"
	if model.Pack().Measures[filtered].Filters[0].Relationship == "other" {
		t.Fatal("relationship aliases model")
	}
	if _, err := Compile(copy); err == nil {
		t.Fatal("unknown relationship admitted")
	}
	copy = model.Pack()
	copy.Measures[filtered].Filters[0].Relationship = ""
	dropped, err := Compile(copy)
	if err != nil || dropped.Digest() == model.Digest() {
		t.Fatal("dropping relationship did not invalidate exact reviewed digest", err)
	}
	if _, err := MutateEntities(model, "missing-join", []EntityMutation{{Operation: "delete", Kind: KindJoin, ID: original.Joins[0].ID}}); err == nil {
		t.Fatal("join deletion stranded a population predicate")
	}
	mapping := []ExportDatasetSlots{}
	bindings := DraftBindings{Topic: "imported", Version: "v1"}
	for _, d := range original.Datasets {
		m := ExportDatasetSlots{Dataset: d.ID, Slot: d.ID + "_slot"}
		b := ImportDatasetBinding{Slot: m.Slot, Source: SourceReference{Source: "new-source", Context: "new-context", Dataset: d.ID + "_new", ProfileVersion: d.ID + "_profile", ProfileDigest: strings.Repeat("c", 64), SourceRevision: 1}}
		for _, c := range d.Columns {
			m.Columns = append(m.Columns, ExportColumnSlot{Column: c.ID, Slot: c.ID})
			b.Columns = append(b.Columns, ImportColumnBinding{Slot: c.ID, ID: c.ID, SourceName: c.SourceName, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable})
		}
		mapping = append(mapping, m)
		bindings.Datasets = append(bindings.Datasets, b)
	}
	portable, err := ExportPortable(model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ImportDraftCandidate(portable, bindings)
	if err != nil {
		t.Fatal(err)
	}
	actual := candidate.Pack().Measures[filtered].Filters[0]
	if actual.Relationship != original.Joins[0].ID || actual.Field.Dataset != "customers_new" || !reflect.DeepEqual(actual.Values, []string{"north"}) {
		t.Fatal("portable relationship meaning lost", actual)
	}
	graph := dependencyGraphPack(original)
	found := false
	for _, r := range graph[Reference{Kind: KindMeasure, ID: original.Measures[filtered].ID}] {
		found = found || r == (Reference{Kind: KindJoin, ID: original.Joins[0].ID})
	}
	if !found {
		t.Fatal("relationship absent from dependency graph")
	}
}
