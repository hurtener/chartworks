package securityapi

import (
	"encoding/json"
	"testing"
)

func TestAPIRegistryMatchesOperationalSurface(t *testing.T) {
	for _, metrics := range []bool{false, true} {
		registry, err := APIRegistry(metrics)
		if err != nil {
			t.Fatal(err)
		}
		definitions := registry.Definitions()
		want := 5
		if metrics {
			want++
		}
		if len(definitions) != want {
			t.Fatalf("metrics=%t definitions=%d want=%d", metrics, len(definitions), want)
		}
		for _, definition := range definitions {
			if definition.Public || definition.Action == "" || definition.Response == nil || len(definition.Errors) == 0 {
				t.Fatalf("incomplete protected definition: %#v", definition)
			}
		}
		if got := registry.Operations(); len(got) != want {
			t.Fatalf("metrics=%t operations=%d", metrics, len(got))
		}
		wire, err := registry.OpenAPI("Security", "21")
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(wire, &document); err != nil {
			t.Fatal(err)
		}
		paths := document["paths"].(map[string]any)
		if _, ok := paths["/metrics"]; ok != metrics {
			t.Fatalf("metrics path presence=%t want=%t", ok, metrics)
		}
		if audit := paths["/v1/audit-events"].(map[string]any)["get"].(map[string]any); len(audit["parameters"].([]any)) != 1 {
			t.Fatal("audit limit metadata missing")
		}
		sweep := paths["/v1/retention-sweeps"].(map[string]any)["post"].(map[string]any)
		if len(sweep["parameters"].([]any)) != 1 || sweep["requestBody"] == nil {
			t.Fatal("retention sweep header/body metadata missing")
		}
	}
}
