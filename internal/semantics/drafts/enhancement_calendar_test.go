package drafts

import (
	"encoding/json"
	"testing"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestEnhancementCalendarSchemaMatchesCanonicalVocabulary(t *testing.T) {
	schema, err := gateway.NewSchema("calendar_vocabulary", enhancementSchema)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = gateway.NewStrictSchema(schema); err != nil {
		t.Fatal("calendar vocabulary is incompatible with strict transport", err)
	}
	for _, calendar := range append(semantics.EnhancementCalendarNames(), "fiscal", "gregORian", "gregorian ", "") {
		item := map[string]any{"dataset": "orders", "column": "ordered_at", "kind": "dimension", "name": "Order date", "role": "temporal", "description": "Synthetic date", "aliases": []string{}, "semantic_role": "event_time", "temporal": map[string]any{"calendar": calendar, "timezone": "UTC", "grains": []string{"day", "month", "quarter", "year"}}}
		raw, _ := json.Marshal(map[string]any{"results": []any{item}})
		known := false
		for _, name := range semantics.EnhancementCalendarNames() {
			known = known || calendar == name
		}
		if (schema.Validate(raw, 64<<10) == nil) != known {
			t.Fatal("provider vocabulary and authoring boundary disagree")
		}
	}
}
