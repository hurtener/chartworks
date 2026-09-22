package drafts

import (
	"encoding/json"
	"testing"

	"github.com/hurtener/chartworks/internal/gateway"
)

func TestRichEnhancementSchemaIsClosedAndAccepted(t *testing.T) {
	if _, err := gateway.NewSchema("rich_topic_enhancement_test", enhancementSchema); err != nil {
		t.Fatalf("rich enhancement schema rejected: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(enhancementSchema, &document); err != nil {
		t.Fatal(err)
	}
	properties, ok := document["properties"].(map[string]any)
	if !ok || properties["results"] == nil || properties["kpis"] == nil || properties["relationships"] == nil {
		t.Fatalf("rich authoring branches absent: %#v", properties)
	}
}
