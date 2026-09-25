package chartworks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/sdk/chartworks"
)

func TestSQLRecoveryInterpretationSDKWire(t *testing.T) {
	in := chartworks.NLQRefineRequest{QueryID: "parent"}
	in.InterpretationSelections = []chartworks.NLQInterpretationSelection{{Topic: "topic", Dimension: "region", Value: "north", Operator: "eq"}}
	in.InterpretationEdits = []chartworks.NLQInterpretationEdit{{Target: "topic:date:time", Action: "replace", Period: &chartworks.NLQInterpretationPeriod{Start: "2026-01-01", End: "2026-04-01", Grain: "quarter"}}}
	before, _ := json.Marshal(in)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Method != http.MethodPost || r.URL.Path != "/v1/nlq/refinements" {
			t.Error("wrong existing transport")
		}
		var got chartworks.NLQRefineRequest
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if err := d.Decode(&got); err != nil || !reflect.DeepEqual(in, got) {
			t.Error("SDK changed typed interpretation", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chartworks.NLQPlanResult{QueryID: "child"})
	}))
	defer server.Close()
	client, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	out, err := client.RefineNLQ(context.Background(), in)
	if err != nil || out.QueryID != "child" || calls.Load() != 1 {
		t.Fatal(err)
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("SDK mutated caller")
	}
}
