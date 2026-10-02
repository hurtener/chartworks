package chartworks_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/sdk/chartworks"
)

// Use only the public SDK to construct the newly added wire types. Domain
// validation, old-value custody and SQL-role proofs remain in the service.
func TestSQLRecoveryParameterEditSDKWire(t *testing.T) {
	input := chartworks.NLQRefineRequest{QueryID: "parent", ParameterEdits: []chartworks.NLQParameterEdit{{Position: 1, Replacement: chartworks.NLQParameter{Kind: "number", Value: "9007199254740993.126"}}, {Position: 2, Replacement: chartworks.NLQParameter{Kind: "text", Value: "cliente nuevo — 東京"}}}}
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/nlq/refinements" || r.Header.Get("Authorization") != "Bearer synthetic-sdk-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("typed edit bypassed existing authenticated transport")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var got chartworks.NLQRefineRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&got); err != nil || !reflect.DeepEqual(got, input) {
			t.Error("public request changed precision, Unicode, position or shape")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(chartworks.NLQPlanResult{QueryID: "child"}); err != nil {
			t.Error("response encoding", err)
		}
	}))
	defer server.Close()
	client, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-sdk-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.RefineNLQ(context.Background(), input)
	if err != nil || result.QueryID != "child" || calls.Load() != 1 {
		t.Fatal("SDK typed edit forwarding", err)
	}
	after, err := json.Marshal(input)
	if err != nil || string(before) != string(after) {
		t.Fatal("SDK mutated caller input")
	}
	logged := fmt.Sprintf("%v %#v", input.ParameterEdits, input.ParameterEdits)
	for _, edit := range input.ParameterEdits {
		if strings.Contains(logged, edit.Replacement.Value) {
			t.Fatal("SDK edit alias lost value-log redaction")
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.RefineNLQ(cancelled, input); !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Fatal("cancelled typed edit reached server", err)
	}
	input.QueryID = "../other-query"
	if _, err := client.RefineNLQ(context.Background(), input); err == nil || calls.Load() != 1 {
		t.Fatal("invalid query identity reached transport")
	}
}
