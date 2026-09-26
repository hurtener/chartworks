package chartworks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/sdk/chartworks"
)

func TestSQLRecoveryGroupingSDKReplacementAndTotal(t *testing.T) {
	for _, keys := range [][]chartworks.NLQGroupingKey{{{Topic: "topic", Dimension: "date", Grain: "month"}}, {}} {
		in := chartworks.NLQRefineRequest{QueryID: "parent"}
		in.Grouping = &chartworks.NLQGroupingSelection{Policy: chartworks.NLQGroupingPolicy, Keys: keys}
		before, _ := json.Marshal(in)
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != http.MethodPost || r.URL.Path != "/v1/nlq/refinements" || r.Header.Get("Authorization") != "Bearer fixture" {
				t.Error("unexpected transport")
			}
			var got chartworks.NLQRefineRequest
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			if err := d.Decode(&got); err != nil || !reflect.DeepEqual(got, in) {
				t.Error("typed grouping changed", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(chartworks.NLQPlanResult{QueryID: "child"})
		}))
		client, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "fixture", nil })
		if err != nil {
			t.Fatal(err)
		}
		out, err := client.RefineNLQ(context.Background(), in)
		server.Close()
		if err != nil || out.QueryID != "child" || calls != 1 {
			t.Fatal(err)
		}
		after, _ := json.Marshal(in)
		if string(before) != string(after) {
			t.Fatal("SDK mutated input")
		}
	}
}
