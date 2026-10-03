package nlqroute

import (
	"encoding/json"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq/conceptchoice"
)

// The grouping producer has decision-bound cardinality. Its nested tagged union
// projects into the provider's supported object-root/anyOf subset; the original
// domain schema and structural choice proof remain independently authoritative.
// Existing concept-choice schemas and persisted proof formats do not change.
func groupingIntentSchema(ids []string) (*gateway.Schema, error) {
	base, err := conceptSchema(ids)
	if err != nil {
		return nil, err
	}
	branch := func(decision string, minSelected, maxSelected, minAlternatives, maxAlternatives int) (map[string]any, error) {
		var value map[string]any
		if err := json.Unmarshal(base.Document(), &value); err != nil {
			return nil, err
		}
		properties := value["properties"].(map[string]any)
		properties["decision"] = map[string]any{"type": "string", "enum": []string{decision}}
		selected := properties["selected"].(map[string]any)
		selected["minItems"], selected["maxItems"] = minSelected, maxSelected
		alternatives := properties["alternatives"].(map[string]any)
		alternatives["minItems"], alternatives["maxItems"] = minAlternatives, maxAlternatives
		return value, nil
	}
	variants := make([]any, 0, 3)
	for _, shape := range []struct {
		decision                                                 string
		selectedMin, selectedMax, alternativeMin, alternativeMax int
	}{
		{"select", 1, conceptchoice.MaxSelected, 0, 0},
		{"clarify", 0, 0, 2, conceptchoice.MaxSelected},
		{"no_match", 0, 0, 0, 0},
	} {
		v, err := branch(shape.decision, shape.selectedMin, shape.selectedMax, shape.alternativeMin, shape.alternativeMax)
		if err != nil {
			return nil, err
		}
		variants = append(variants, v)
	}
	raw, err := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": []string{"choice"}, "properties": map[string]any{"choice": map[string]any{"oneOf": variants}}})
	if err != nil {
		return nil, ErrInvalid
	}
	return gateway.NewSchema("nlq_grouping_intent", raw)
}
