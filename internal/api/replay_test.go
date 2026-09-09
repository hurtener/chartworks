package api

import (
	"encoding/json"
	"testing"
)

func TestReplayNeedsExplicitOwningContract(t *testing.T) {
	d := definition(t)
	d.Headers = []Parameter{{Name: "Idempotency-Key", In: "header", Type: "string", Required: true, Max: 128}}
	check := func(d Definition, want string) {
		t.Helper()
		r, err := New([]Definition{d})
		if err != nil {
			t.Fatal(err)
		}
		data, err := r.OpenAPI("replay contract", "1")
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			t.Fatal("document")
		}
		op := doc["paths"].(map[string]any)[d.Path].(map[string]any)["post"].(map[string]any)
		if op["x-chartworks-replay"] != want {
			t.Fatal("implicit replay authority")
		}
	}
	check(d, "never")
	d.Replay = "keyed"
	check(d, "keyed")
	for _, edit := range []func(*Definition){
		func(d *Definition) { d.Headers = nil },
		func(d *Definition) { d.Headers[0].Required = false },
		func(d *Definition) { d.Headers[0].Max = 0 },
		func(d *Definition) { d.Replay = "unknown" },
		func(d *Definition) { d.Replay = "read" },
	} {
		bad := cloneDefinition(d)
		edit(&bad)
		if _, err := New([]Definition{bad}); err == nil {
			t.Fatal("unproven replay registered")
		}
	}
}
