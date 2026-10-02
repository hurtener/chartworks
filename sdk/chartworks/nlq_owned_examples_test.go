package chartworks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/sdk/chartworks"
)

func TestSQLRecoveryOwnedExampleSDKWire(t *testing.T) {
	request := chartworks.NLQExampleImportRequest{Example: chartworks.NLQPortableExample{SchemaVersion: 3, Question: "Reviewed unbound query template for current service-owned filters. Metric identities: []", SQL: "SELECT sum(amount) FROM analytics.sales", Origin: chartworks.NLQExampleOrigin{SchemaVersion: 1, BindingPolicy: chartworks.NLQOwnedExamplePolicy}}}
	raw, _ := json.Marshal(request)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/nlq/examples/import" || r.Header.Get("Authorization") != "Bearer sdk-owned-test" {
			t.Error("owned template bypassed authenticated import")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var got chartworks.NLQExampleImportRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&got); err != nil || !reflect.DeepEqual(request, got) {
			t.Error("policy lost in SDK request", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(chartworks.NLQExample{ID: "candidate", State: "candidate", Origin: got.Example.Origin}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "sdk-owned-test", nil })
	if err != nil {
		t.Fatal(err)
	}
	out, err := client.ImportExampleNLQ(context.Background(), request)
	if err != nil || out.ID != "candidate" || out.Origin.BindingPolicy != chartworks.NLQOwnedExamplePolicy || calls.Load() != 1 {
		t.Fatal("SDK template import", err)
	}
	out.Origin.BindingPolicy = "changed"
	current, _ := json.Marshal(request)
	if string(raw) != string(current) || strings.Contains(string(raw), `"value":`) || strings.Contains(string(raw), `"default":`) {
		t.Fatal("mutation/private values in SDK contract")
	}
}
