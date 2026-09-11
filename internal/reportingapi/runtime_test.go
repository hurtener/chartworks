package reportingapi

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
)

func TestRuntimeRegistry(t *testing.T) {
	for _, execution := range []bool{false, true} {
		for _, planning := range []bool{false, true} {
			r, err := RuntimeRegistry(execution, planning)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range r.Definitions() {
				if d.Response == nil || d.Method != "GET" && d.Request == nil {
					t.Fatal("missing wire schema", d.ID)
				}
			}
		}
	}
}

func TestRuntimeManifestParity(t *testing.T) {
	raw, err := os.ReadFile("../../docs/contracts/chartworks-runtime-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []api.Operation
	if json.Unmarshal(raw, &want) != nil {
		t.Fatal("invalid published manifest")
	}
	registry, err := RuntimeRegistry(true, true)
	if err != nil {
		t.Fatal(err)
	}
	got := []api.Operation{}
	for _, d := range registry.Definitions() {
		got = append(got, d.Operation)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Method+" "+got[i].Path < got[j].Method+" "+got[j].Path })
	if !reflect.DeepEqual(want, got) {
		t.Fatal("published operations drifted from runtime registration")
	}
}
