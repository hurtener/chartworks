package chartworks

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCW03NativeOutputExample(t *testing.T) {
	file, err := os.Open("../../examples/reporting/output-intent-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var output BlockOutput
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&output); err != nil {
		t.Fatal(err)
	}
	if output.ID != "monthly-summary" || output.Intent == nil || !output.Intent.Enabled || !output.Intent.DefaultSelected || output.Intent.DisplayOrder != 2 || len(output.Intent.Metadata) != 2 || output.Narrative == nil || output.Narrative.PolicyVersion != BlockNarrativePolicyVersion || output.Narrative.MaxClaims != 3 {
		t.Fatal("typed public example lost policy", output)
	}
	candidate, err := MigrateBlockDefinition(BlockDefinition{SchemaVersion: BlockDefinitionVersion, Outputs: []BlockOutput{output}})
	if err != nil || len(candidate.Outputs) != 1 {
		t.Fatal(candidate, err)
	}
	candidate.Outputs[0].Intent.Enabled = false
	if !output.Intent.Enabled {
		t.Fatal("migration aliased caller example")
	}
}
