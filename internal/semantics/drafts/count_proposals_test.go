package drafts

import (
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"reflect"
	"testing"
)

func TestSupplementalCountsPreservePrimaryAndVocabulary(t *testing.T) {
	model, _, value := vocabularyFixture(t)
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "column_01"}
	primary := semantics.Enhancement{Dataset: field.Dataset, Column: field.ID, Kind: semantics.EnhancementMeasure, Name: "Known gross", Description: "Known numeric amount", Aggregation: semantics.AggregationSum, Unit: "USD"}
	wire := enhancementWire{Results: []semantics.Enhancement{primary}, CountProposals: []CountProposal{{Dataset: field.Dataset, Column: field.ID, Name: "Known amount count", Description: "Count only non-NULL amounts", Unit: "orders"}}, FilterProposals: []VocabularyFilterProposal{{Measure: GeneratedCountMeasureID(field.Dataset, field.ID), ID: "paid", Operator: "eq", Nulls: "exclude", VocabularyIDs: []string{value.ID}}}}
	catalog, err := enhancementMetricCatalog(model, []semantics.Reference{field}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateEnhancementOutput([]semantics.Reference{field}, catalog, wire); err != nil {
		t.Fatal(err)
	}
	if err = resolveVocabularyProposals(model.Pack(), &wire, []AuthoringValue{value}); err != nil {
		t.Fatal(err)
	}
	expanded, err := applyCountProposals(model, wire.CountProposals)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := semantics.ApplyRichEnhancements(expanded, "v2", wire.Results, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var count, gross *semantics.Measure
	for _, m := range changed.Pack().Measures {
		m := m
		if m.ID == GeneratedCountMeasureID(field.Dataset, field.ID) {
			count = &m
		}
		if m.ID == semantics.GeneratedEntityID(semantics.EnhancementMeasure, field.Dataset, field.ID) {
			gross = &m
		}
	}
	if count == nil || gross == nil || count.Aggregation != semantics.AggregationCount || gross.Aggregation != semantics.AggregationSum || len(count.Filters) != 1 || count.Filters[0].Values[0] != "P" {
		t.Fatal("primary/count/filter meaning lost")
	}
	replay, err := applyCountProposals(changed, wire.CountProposals)
	if err != nil || replay.Digest() != changed.Digest() {
		t.Fatal("count replay changed digest", err)
	}
	without := wire.CountProposals[0]
	without.Filters = nil
	kept, err := applyCountProposals(changed, []CountProposal{without})
	if err != nil || kept.Digest() != changed.Digest() {
		t.Fatal("omitted filters erased reviewed population", err)
	}
	conflict := wire.CountProposals[0]
	conflict.Filters = append([]semantics.SemanticFilter(nil), count.Filters...)
	conflict.Filters[0].Values = []string{"other"}
	if _, err = applyCountProposals(changed, []CountProposal{conflict}); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("count population silently overwritten", err)
	}
}

func TestSupplementalCountsAreCurrentBoundedAndClosed(t *testing.T) {
	model, _, _ := vocabularyFixture(t)
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "column_01"}
	proposal := CountProposal{Dataset: field.Dataset, Column: field.ID, Name: "Known value count", Description: "Counts non-NULL values", Unit: "values"}
	for name, proposals := range map[string][]CountProposal{"foreign": {{Dataset: "outside", Column: field.ID}}, "later_page": {{Dataset: field.Dataset, Column: "column_02"}}, "duplicate": {proposal, proposal}, "budget": make([]CountProposal, 9)} {
		t.Run(name, func(t *testing.T) {
			if err := validateCountProposals([]semantics.Reference{field}, proposals); !errors.Is(err, gateway.ErrOutput) {
				t.Fatal("invalid count accepted", err)
			}
		})
	}
	catalog, err := enhancementMetricCatalog(model, []semantics.Reference{field}, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, metric := range catalog {
		if metric.ID == GeneratedCountMeasureID(field.Dataset, field.ID) {
			found = metric.Aggregation == "count" && metric.Field != nil && *metric.Field == field
		}
	}
	if !found {
		t.Fatal("count ID lacks exact field/aggregation catalog evidence")
	}
	var schema map[string]any
	if json.Unmarshal(enhancementSchema, &schema) != nil {
		t.Fatal("schema")
	}
	count := schema["properties"].(map[string]any)["count_proposals"].(map[string]any)
	item := count["items"].(map[string]any)
	props := item["properties"].(map[string]any)
	if item["additionalProperties"] != false || props["aggregation"] != nil || props["distinct"] != nil || props["id"] != nil || count["maxItems"] != float64(8) {
		t.Fatal("count shortcut or model-chosen ID admitted")
	}
	if GeneratedCountMeasureID(field.Dataset, field.ID) == semantics.GeneratedEntityID(semantics.EnhancementMeasure, field.Dataset, field.ID) || GeneratedCountMeasureID("", field.ID) != "" {
		t.Fatal("count identity not separated")
	}
	copyCatalog, err := enhancementMetricCatalog(model, []semantics.Reference{field}, true)
	if err != nil || !reflect.DeepEqual(catalog, copyCatalog) {
		t.Fatal("nondeterministic catalog")
	}
	wire := enhancementWire{Results: []semantics.Enhancement{{Dataset: field.Dataset, Column: field.ID, Kind: semantics.EnhancementDimension}}, KPIs: []semantics.KPI{{ID: "premature", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: GeneratedCountMeasureID(field.Dataset, field.ID)}}}}}
	if err := validateEnhancementOutput([]semantics.Reference{field}, catalog, wire); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("uncreated count referenced by KPI", err)
	}
	wire.CountProposals = []CountProposal{proposal}
	if err := validateEnhancementOutput([]semantics.Reference{field}, catalog, wire); err != nil {
		t.Fatal("same-step count unavailable to KPI", err)
	}
}

func TestSupplementalCountSchemaRejectsDistinctIDsAndSQL(t *testing.T) {
	schema, err := gateway.NewSchema("supplemental_count_closed", enhancementSchema)
	if err != nil {
		t.Fatal(err)
	}
	proposal := map[string]any{"dataset": "orders", "column": "id", "name": "Paid order count", "description": "Count non-NULL identities without deduplication", "aliases": []string{}, "unit": "orders"}
	result := map[string]any{"dataset": "orders", "column": "id", "kind": "dimension", "name": "Order ID", "description": "Composite identity component", "aliases": []string{}, "role": "identifier", "semantic_role": "fact_key", "temporal": nil}
	body := map[string]any{"results": []any{result}, "count_proposals": []any{proposal}}
	raw, _ := json.Marshal(body)
	if err = schema.Validate(raw, 65536); err != nil {
		t.Fatal("valid supplemental count shape rejected", err)
	}
	for key, value := range map[string]any{"id": "chosen-id", "aggregation": "distinct_count", "distinct": true, "sql": "COUNT(DISTINCT id)", "filters": []any{}} {
		proposal[key] = value
		raw, _ = json.Marshal(body)
		if err = schema.Validate(raw, 65536); !errors.Is(err, gateway.ErrOutput) {
			t.Fatal("unreviewed count shortcut admitted", key, err)
		}
		delete(proposal, key)
	}
}
