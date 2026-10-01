package topicfeedback

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/gateway"
	"testing"
)

func TestActualFeedbackStrictProviderSchema(t *testing.T) {
	schema, err := proposalSchema()
	if err != nil {
		t.Fatal(err)
	}
	p, err := gateway.NewStrictSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	var projected map[string]any
	_ = json.Unmarshal(p.Document(), &projected)
	fields := projected["properties"].(map[string]any)["edits"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	edit := map[string]any{}
	for key := range fields {
		edit[key] = nil
	}
	edit["kind"], edit["id"], edit["description"] = "measure", "amount", "Exact meaning"
	raw, _ := json.Marshal(map[string]any{"edits": []any{edit}})
	out, err := p.Normalize(raw, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"edits":[{"description":"Exact meaning","id":"amount","kind":"measure"}]}` {
		t.Fatal("absent edits became destructive changes", string(out))
	}
}
