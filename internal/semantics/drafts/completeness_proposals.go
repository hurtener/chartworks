package drafts

import "github.com/hurtener/chartworks/internal/semantics"

func addCompletenessSchema(properties map[string]any) {
	variants := properties["results"].(map[string]any)["items"].(map[string]any)["oneOf"].([]any)
	for _, variant := range variants {
		p := variant.(map[string]any)["properties"].(map[string]any)
		kind := p["kind"].(map[string]any)
		if kind["const"] != "measure" {
			continue
		}
		p["completeness"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"policy", "unknown_count"}, "properties": map[string]any{"policy": map[string]any{"const": semantics.KnownAmountCompletenessPolicy}, "unknown_count": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "id"}, "properties": map[string]any{"kind": map[string]any{"const": "kpi"}, "id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}}}}}
	}
}
