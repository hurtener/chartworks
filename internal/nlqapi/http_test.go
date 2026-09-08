package nlqapi

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/nlqroute"
)

func TestRegistryIncludesNLQRoute(t *testing.T) {
	if _, err := api.SchemaFor("debugResponse", reflect.TypeFor[nlqroute.RouteResult](), true); err != nil {
		t.Fatalf("response schema: %v", err)
	}
	if _, err := api.SchemaFor("debugRequest", reflect.TypeFor[nlqroute.RouteRequest](), false, api.OptionalJSONFields); err != nil {
		t.Fatalf("request schema: %v", err)
	}
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	defs := r.Definitions()
	if len(defs) != 1 || defs[0].ID != "routeNLQ" || defs[0].Path != "/v1/nlq/routes" || defs[0].Request == nil || defs[0].Response == nil {
		t.Fatalf("unexpected route registry: %#v", defs)
	}
	if got, _, ok := r.Match("POST", "/v1/nlq/routes"); !ok || got.Action != "topics.read" {
		t.Fatalf("route did not match: %#v", got)
	}
	raw, err := os.ReadFile("../../docs/contracts/chartworks-nlq-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest []api.Operation
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	actual := r.Operations()
	sort.Slice(manifest, func(i, j int) bool {
		if manifest[i].Path != manifest[j].Path {
			return manifest[i].Path < manifest[j].Path
		}
		return manifest[i].Method < manifest[j].Method
	})
	sort.Slice(actual, func(i, j int) bool {
		if actual[i].Path != actual[j].Path {
			return actual[i].Path < actual[j].Path
		}
		return actual[i].Method < actual[j].Method
	})
	if !reflect.DeepEqual(manifest, actual) {
		t.Fatalf("manifest differs from actual registration: manifest=%#v actual=%#v", manifest, actual)
	}
}
