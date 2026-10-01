package drafts

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/gateway"
	"strings"
	"testing"
)

func TestActualDraftStrictProviderSchemas(t *testing.T) {
	for name, raw := range map[string][]byte{"topic_enhancement_step": enhancementSchema, "topic_quality_review": qualitySchema} {
		t.Run(name, func(t *testing.T) {
			domain, err := gateway.NewSchema(name, raw)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := gateway.NewStrictSchema(domain)
			if err != nil {
				t.Fatal("actual role schema incompatible", err)
			}
			if strings.Contains(string(projection.Document()), `"oneOf"`) {
				t.Fatal("unsupported oneOf on provider wire")
			}
			if name == "topic_quality_review" {
				return
			}
			var document map[string]any
			_ = json.Unmarshal(projection.Document(), &document)
			properties := document["properties"].(map[string]any)
			wire := map[string]any{}
			for key := range properties {
				wire[key] = nil
			}
			wire["results"] = []any{map[string]any{"dataset": "orders", "column": "region", "kind": "dimension", "name": "Region", "role": "categorical", "description": "Region label", "aliases": []any{}, "semantic_role": "attribute", "geography": nil, "temporal": nil}}
			encoded, _ := json.Marshal(wire)
			normalized, err := projection.Normalize(encoded, 64<<10)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			_ = json.Unmarshal(normalized, &value)
			if len(value) != 1 {
				t.Fatal("absent supplemental proposal became a semantic choice")
			}
			dimension := value["results"].([]any)[0].(map[string]any)
			if _, ok := dimension["geography"]; ok {
				t.Fatal("introduced null leaked")
			}
			if temporal, ok := dimension["temporal"]; !ok || temporal != nil {
				t.Fatal("mandatory domain null disappeared")
			}
			result := wire["results"].([]any)[0].(map[string]any)
			result["temporal"] = map[string]any{"grains": []any{"day"}, "calendar": "gregorian", "timezone": nil}
			encoded, _ = json.Marshal(wire)
			if _, err = projection.Normalize(encoded, 64<<10); err != nil {
				t.Fatal("nested optional timezone", err)
			}
			delete(result, "name")
			encoded, _ = json.Marshal(wire)
			if _, err = projection.Normalize(encoded, 64<<10); err == nil {
				t.Fatal("mandatory field vanished")
			}
		})
	}
}
