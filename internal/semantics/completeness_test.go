package semantics

import (
	"reflect"
	"testing"
)

func completenessPack() TopicPack {
	p := testPack()
	p.Datasets[0].Columns[0].SemanticRole = SemanticRoleFactKey
	p.Measures = append(p.Measures, Measure{ID: "known_amount_count", Name: "Known amount count", Field: Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}, Aggregation: AggregationCount, Unit: "orders"})
	p.KPIs = append(p.KPIs, KPI{ID: "unknown_amounts", Name: "Unknown amount count", Expression: "order_count - known_amount_count", Inputs: []Reference{{Kind: KindMeasure, ID: "order_count"}, {Kind: KindMeasure, ID: "known_amount_count"}}})
	p.Measures[1].Completeness = &KnownAmountCompleteness{Policy: KnownAmountCompletenessPolicy, UnknownCount: Reference{Kind: KindKPI, ID: "unknown_amounts"}}
	return p
}
func completenessCatalog(p TopicPack) CompletenessCatalog {
	c := CompletenessCatalog{Measures: p.Measures, KPIs: p.KPIs, Columns: map[Reference]Column{}}
	for _, d := range p.Datasets {
		for _, column := range d.Columns {
			c.Columns[Reference{Kind: KindColumn, Dataset: d.ID, ID: column.ID}] = column
		}
	}
	return c
}
func TestKnownAmountCompletenessHasExactAcyclicCountMeaning(t *testing.T) {
	p := completenessPack()
	model, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := ResolveKnownAmountCompleteness(completenessCatalog(model.Pack()), "revenue")
	if err != nil || binding.RowCount.ID != "order_count" || binding.KnownCount.ID != "known_amount_count" || binding.UnknownCount.ID != "unknown_amounts" || len(binding.PopulationDigest) != 64 {
		t.Fatal("bad completeness binding", binding, err)
	}
	p.Measures[1].Completeness.UnknownCount.ID = "changed"
	got := model.Pack()
	for _, m := range got.Measures {
		if m.ID == "revenue" && m.Completeness.UnknownCount.ID != "unknown_amounts" {
			t.Fatal("input metadata aliases model")
		}
	}
	for i := range got.Measures {
		if got.Measures[i].ID == "revenue" {
			got.Measures[i].Completeness.UnknownCount.ID = "changed"
		}
	}
	if _, err := ResolveKnownAmountCompleteness(completenessCatalog(model.Pack()), "revenue"); err != nil {
		t.Fatal("output metadata aliases model")
	}
	if _, err := MutateEntities(model, "missing-companion", []EntityMutation{{Operation: "delete", Kind: KindKPI, ID: "unknown_amounts"}}); err == nil {
		t.Fatal("companion delete stranded link")
	}
	graph := dependencyGraphPack(model.Pack())
	found := false
	for _, ref := range graph[Reference{Kind: KindMeasure, ID: "revenue"}] {
		found = found || ref == (Reference{Kind: KindKPI, ID: "unknown_amounts"})
	}
	if !found {
		t.Fatal("obligation absent from dependency closure")
	}
}
func TestKnownAmountCompletenessRejectsWrongPopulationFormulaAndCycles(t *testing.T) {
	for name, change := range map[string]func(*TopicPack){
		"non sum":          func(p *TopicPack) { p.Measures[1].Aggregation = AggregationAverage },
		"text amount":      func(p *TopicPack) { p.Datasets[0].Columns[1].Category = "text" },
		"missing KPI":      func(p *TopicPack) { p.Measures[1].Completeness.UnknownCount.ID = "missing" },
		"row nullable":     func(p *TopicPack) { p.Datasets[0].Columns[0].Nullable = true },
		"row not identity": func(p *TopicPack) { p.Datasets[0].Columns[0].SemanticRole = SemanticRoleAttribute },
		"distinct":         func(p *TopicPack) { p.Measures[0].Aggregation = AggregationDistinctCount },
		"wrong amount":     func(p *TopicPack) { p.Measures[2].Field = p.Measures[0].Field },
		"extra term":       func(p *TopicPack) { p.KPIs[2].Expression = "order_count - known_amount_count + 1" },
		"reversed":         func(p *TopicPack) { p.KPIs[2].Expression = "known_amount_count - order_count" },
		"different fact": func(p *TopicPack) {
			p.Measures[0].Field = Reference{Kind: KindColumn, Dataset: "customers", ID: "customer_key"}
		},
		"period override": func(p *TopicPack) { p.KPIs[2].Periods = &MetricPeriodBindings{Policy: MetricPeriodBindingsPolicy} },
		"nested cycle": func(p *TopicPack) {
			p.KPIs[2].Inputs[0] = Reference{Kind: KindMeasure, ID: "revenue"}
			p.KPIs[2].Expression = "revenue - known_amount_count"
		},
		"count backlink": func(p *TopicPack) { p.Measures[0].Completeness = p.Measures[1].Completeness },
		"population mismatch": func(p *TopicPack) {
			p.Measures[0].Filters = []SemanticFilter{{ID: "known_only", Field: p.Measures[2].Field, Operator: "not_null"}}
		},
		"KPI extra predicate": func(p *TopicPack) {
			p.KPIs[2].Filters = []SemanticFilter{{ID: "known_only", Field: p.Measures[2].Field, Operator: "not_null"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := completenessPack()
			change(&p)
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid completeness meaning admitted")
			}
		})
	}
}
func TestKnownAmountPopulationIgnoresOnlyCosmeticFilterIDs(t *testing.T) {
	p := completenessPack()
	filter := SemanticFilter{ID: "first", Field: p.Measures[1].Field, Operator: "not_null"}
	for i := range p.Measures {
		p.Measures[i].Filters = []SemanticFilter{filter}
		p.Measures[i].Filters[0].ID = "filter_" + p.Measures[i].ID
	}
	model, err := Compile(p)
	if err != nil {
		t.Fatal("cosmetic filter IDs changed population", err)
	}
	binding, err := ResolveKnownAmountCompleteness(completenessCatalog(model.Pack()), "revenue")
	if err != nil {
		t.Fatal(err)
	}
	first := model.Pack()
	for _, m := range first.Measures {
		if m.ID == "revenue" && !reflect.DeepEqual(m.Completeness, p.Measures[1].Completeness) {
			t.Fatal("metadata changed")
		}
	}
	changed := filter
	changed.Relationship = "different"
	if completenessPopulation([]SemanticFilter{filter}) == completenessPopulation([]SemanticFilter{changed}) || binding.PopulationDigest == "" {
		t.Fatal("relationship omitted from population identity")
	}
}

func TestKnownAmountCompletenessPortableRoundTrip(t *testing.T) {
	model, err := Compile(completenessPack())
	if err != nil {
		t.Fatal(err)
	}
	mappings := []ExportDatasetSlots{}
	bindings := DraftBindings{Topic: "imported", Version: "v1"}
	for _, dataset := range model.Pack().Datasets {
		mapping := ExportDatasetSlots{Dataset: dataset.ID, Slot: dataset.ID + "_slot"}
		origin := dataset.Source
		origin.Dataset = dataset.ID + "_new"
		binding := ImportDatasetBinding{Slot: mapping.Slot, Source: origin}
		for _, column := range dataset.Columns {
			mapping.Columns = append(mapping.Columns, ExportColumnSlot{Column: column.ID, Slot: column.ID})
			binding.Columns = append(binding.Columns, ImportColumnBinding{Slot: column.ID, ID: column.ID + "_new", SourceName: column.SourceName, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
		}
		mappings = append(mappings, mapping)
		bindings.Datasets = append(bindings.Datasets, binding)
	}
	portable, err := ExportPortable(model, mappings)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ImportDraftCandidate(portable, bindings)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveKnownAmountCompleteness(completenessCatalog(candidate.Pack()), "revenue")
	if err != nil || resolved.AmountField.Dataset != "orders_new" || resolved.AmountField.ID != "amount_new" || resolved.RowField.ID != "customer_key_new" || resolved.UnknownCount.ID != "unknown_amounts" {
		t.Fatal("portable mapping changed completeness graph", resolved, err)
	}
	for i := range portable.Measures {
		if portable.Measures[i].Completeness != nil {
			portable.Measures[i].Completeness.UnknownCount.ID = "changed"
		}
	}
	if _, err = ResolveKnownAmountCompleteness(completenessCatalog(candidate.Pack()), "revenue"); err != nil {
		t.Fatal("portable link aliases imported candidate", err)
	}
}
