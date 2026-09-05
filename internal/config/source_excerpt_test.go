package config

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestSourceReferenceExcerpt(t *testing.T) {
	data, err := os.ReadFile("../../examples/chartworks.sources.json")
	if err != nil {
		t.Fatal(err)
	}
	var excerpt map[string]json.RawMessage
	if err = json.Unmarshal(data, &excerpt); err != nil {
		t.Fatal(err)
	}
	if len(excerpt) != 2 || excerpt["sources"] == nil || excerpt["exec"] == nil {
		t.Fatal("unexpected reference sections")
	}
	authority, err := json.Marshal(good().Auth)
	if err != nil {
		t.Fatal(err)
	}
	excerpt["auth"] = authority
	input, err := json.Marshal(excerpt)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(name string) (string, bool) {
		if name != "CHARTWORKS_STORE_URL" {
			t.Fatalf("configuration resolved warehouse credentials: %s", name)
		}
		return "synthetic", true
	}
	cfg, err := Load(bytes.NewReader(input), lookup, Overrides{})
	if err != nil {
		t.Fatal("published source excerpt rejected", err)
	}
	before := cfg.Values()
	if !before.Sources.Enabled || len(before.Sources.Connections) != 1 || before.Sources.Connections[0].ReadDSN != "env:CHARTWORKS_SOURCE_READ" || before.Exec.Concurrency != 2 {
		t.Fatal("reference lost typed configuration")
	}
	wire, err := json.Marshal(map[string]any{"auth": before.Auth, "sources": before.Sources, "exec": before.Exec})
	if err != nil {
		t.Fatal(err)
	}
	again, err := Load(bytes.NewReader(wire), lookup, Overrides{})
	if err != nil || !reflect.DeepEqual(before, again.Values()) {
		t.Fatal("configuration roundtrip changed source/read policy", err)
	}
}
