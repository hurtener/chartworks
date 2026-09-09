package acceptance

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/config"
)

func TestChartPublishedContracts(t *testing.T) {
	registry, err := chartapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../docs/contracts/chartworks-chart-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest []api.Operation
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != len(registry.Definitions()) {
		t.Fatal("manifest inventory diverged")
	}
	actual := map[string]api.Operation{}
	for _, d := range registry.Definitions() {
		actual[d.Method+" "+d.Path] = d.Operation
	}
	for _, op := range manifest {
		key := op.Method + " " + op.Path
		if actual[key] != op {
			t.Fatal("manifest differs from actual registration", key)
		}
		delete(actual, key)
	}
	if len(actual) != 0 {
		t.Fatal("duplicate manifest operation")
	}
	data, err = os.ReadFile("../../examples/chartworks.charts.json")
	if err != nil {
		t.Fatal(err)
	}
	var excerpt struct {
		Charts config.Charts `json:"charts"`
	}
	if err = json.Unmarshal(data, &excerpt); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(excerpt.Charts, config.DefaultCharts()) {
		t.Fatal("example does not match active defaults")
	}
}
