package nlqapi

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqbyo"
)

func TestBYOContextVersionOneGolden(t *testing.T) {
	golden, err := os.ReadFile("testdata/byo-context-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := BYORegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	operation, _, _ := registry.Match("POST", "/v1/nlq/contexts")
	if err := operation.Response.Validate(golden, 2<<20); err != nil {
		t.Fatal("version-one context response no longer accepted", err)
	}
	var decoded nlqbyo.CreateResult
	decoder := json.NewDecoder(bytes.NewReader(golden))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	b := decoded.Bundle
	if b == nil || !nlqbyo.ReferenceValid(b.Reference) || b.Context.Constraints == nil || len(b.Context.Constraints.Required) != 1 || len(b.Context.Constraints.Excluded) != 1 || len(b.Context.Metrics) != 1 || b.Context.Examples[0].Source != "external_input_unreviewed" || b.Semantics[0].RuleVersion != "rules-v1" || !b.Requirements.ReadOnly {
		t.Fatal("mandatory context, provenance or SQL requirements were lost")
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var before, after any
	if json.Unmarshal(golden, &before) != nil || json.Unmarshal(encoded, &after) != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("version-one context wire round trip changed")
	}
	unknownVersion := strings.Replace(string(golden), `"schema_version": 1`, `"schema_version": 2`, 1)
	if err := operation.Response.Validate([]byte(unknownVersion), 2<<20); err == nil {
		t.Fatal("unknown context version was accepted")
	}
}
