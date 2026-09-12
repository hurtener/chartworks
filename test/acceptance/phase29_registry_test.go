package acceptance

import (
	"encoding/json"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

// Extend phase 21's cumulative registered wire inventory, not a parallel list
// of endpoints that the running application cannot actually expose.
func TestDocumentCombinedRegistry(t *testing.T) {
	base := phase21Registry(t)
	documents, err := reportingapi.DocumentsRegistry()
	if err != nil {
		t.Fatal(err)
	}
	combined, err := api.Compose(base, documents)
	if err != nil || len(combined.Definitions()) != len(base.Definitions())+26 {
		t.Fatal("document endpoints do not compose with the full phase 21 inventory", err)
	}
	body, err := combined.OpenAPI("Chartworks", "phase29")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if json.Unmarshal(body, &contract) != nil {
		t.Fatal("combined OpenAPI is not valid JSON")
	}
	for _, operation := range documents.Operations() {
		if len(contract.Paths[operation.Path]) == 0 {
			t.Fatal("implemented document endpoint missing from cumulative OpenAPI", operation)
		}
	}
}
