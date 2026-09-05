package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestSourceReferenceExcerpt(t *testing.T) {
	data, err := os.ReadFile("../../examples/chartworks.sources.json")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	// The example is an excerpt, so supply the existing synthetic verifier fixture.
	document["auth"] = good().Auth
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bytes.NewReader(data), func(string) (string, bool) {
		return "synthetic-reference-value", true
	}, Overrides{}); err != nil {
		t.Fatal("published source excerpt is not consumable", err)
	}
}
