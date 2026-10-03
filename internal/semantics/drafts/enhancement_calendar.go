package drafts

import "github.com/hurtener/chartworks/internal/semantics"

func addCalendarVocabularySchema(properties map[string]any) {
	results := properties["results"].(map[string]any)
	items := results["items"].(map[string]any)
	found := 0
	for _, raw := range items["oneOf"].([]any) {
		fields := raw.(map[string]any)["properties"].(map[string]any)
		if fields["kind"].(map[string]any)["const"] != "dimension" {
			continue
		}
		temporal := fields["temporal"].(map[string]any)["anyOf"].([]any)
		for _, branch := range temporal {
			node := branch.(map[string]any)
			if node["type"] != "object" {
				continue
			}
			calendar := node["properties"].(map[string]any)["calendar"].(map[string]any)
			calendar["enum"] = semantics.EnhancementCalendarNames()
			calendar["description"] = "Use gregorian."
			found++
		}
	}
	if found != 1 {
		panic("invalid enhancement calendar vocabulary")
	}
}
