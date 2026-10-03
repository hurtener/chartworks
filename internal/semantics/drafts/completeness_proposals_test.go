package drafts

import (
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"reflect"
	"testing"
)

func TestCompletenessEnhancementSchemaAndProtectedLink(t *testing.T) {
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "amount"}
	link := &semantics.KnownAmountCompleteness{Policy: semantics.KnownAmountCompletenessPolicy, UnknownCount: semantics.Reference{Kind: semantics.KindKPI, ID: "unknown_amounts"}}
	pack := semantics.TopicPack{Measures: []semantics.Measure{{ID: semantics.GeneratedEntityID(semantics.EnhancementMeasure, field.Dataset, field.ID), Field: field, Aggregation: semantics.AggregationSum, Completeness: link}}}
	result := semantics.Enhancement{Dataset: field.Dataset, Column: field.ID, Kind: semantics.EnhancementMeasure, Name: "Known amount", Aggregation: semantics.AggregationSum}
	proposals := []semantics.Enhancement{result}
	if err := preserveProtectedEnhancementMeaning(pack, proposals); err != nil || !reflect.DeepEqual(proposals[0].Completeness, link) {
		t.Fatal("omitted link not preserved", err)
	}
	for _, edit := range []semantics.Enhancement{{Dataset: field.Dataset, Column: field.ID, Kind: semantics.EnhancementDimension}, {Dataset: field.Dataset, Column: field.ID, Kind: semantics.EnhancementMeasure, Completeness: &semantics.KnownAmountCompleteness{Policy: link.Policy, UnknownCount: semantics.Reference{Kind: semantics.KindKPI, ID: "other"}}}} {
		if err := preserveProtectedEnhancementMeaning(pack, []semantics.Enhancement{edit}); !errors.Is(err, gateway.ErrOutput) {
			t.Fatal("protected completeness changed", err)
		}
	}
	schema, err := gateway.NewSchema("completeness_shape", enhancementSchema)
	if err != nil {
		t.Fatal(err)
	}
	object := map[string]any{"dataset": "orders", "column": "amount", "kind": "measure", "name": "Known gross", "aggregation": "sum", "description": "Known sum requiring unknown count", "aliases": []string{}, "unit": "USD", "semantic_role": "measure_input", "completeness": link}
	body := map[string]any{"results": []any{object}}
	raw, _ := json.Marshal(body)
	if err = schema.Validate(raw, 65536); err != nil {
		t.Fatal("closed completeness proposal rejected", err)
	}
	object["completeness"] = map[string]any{"policy": link.Policy, "unknown_count": link.UnknownCount, "sql": "COUNT(*)"}
	raw, _ = json.Marshal(body)
	if err = schema.Validate(raw, 65536); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("novel SQL field admitted", err)
	}
}
