package chartworks_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/sdk/chartworks"
)

// External-package construction prevents an internal-only field type from
// shipping an unusable public SDK. The service still owns native review.
func TestSQLRecoveryParameterizedExampleSDKWire(t *testing.T) {
	schema := &chartworks.NLQExampleParameterSchema{Version: "example-parameters-v1", Slots: []chartworks.NLQExampleParameterSlot{{Position: 1, Kind: "number"}, {Position: 2, Kind: "text"}}}
	request := chartworks.NLQExampleImportRequest{Example: chartworks.NLQPortableExample{SchemaVersion: 2, Question: "Reviewed current question", SQL: "SELECT id FROM analytics.sales WHERE amount>$1 AND name=$2", ParameterSchema: schema}}
	before, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/nlq/examples/import" || r.Header.Get("Authorization") != "Bearer synthetic-sdk-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("typed template bypassed existing authenticated transport")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var got chartworks.NLQExampleImportRequest
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if err := d.Decode(&got); err != nil || !reflect.DeepEqual(got, request) {
			t.Error("public example schema changed shape or SQL", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(chartworks.NLQExample{ID: "candidate", State: "candidate", ParameterSchema: got.Example.ParameterSchema}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	c, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-sdk-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.ImportExampleNLQ(context.Background(), request)
	if err != nil || got.ID != "candidate" || !reflect.DeepEqual(got.ParameterSchema, schema) || calls.Load() != 1 {
		t.Fatal("SDK typed example forwarding", err)
	}
	after, _ := json.Marshal(request)
	if string(before) != string(after) || strings.Contains(string(after), `"value":`) || strings.Contains(string(after), `"default":`) {
		t.Fatal("mutated caller or included values/defaults")
	}
	got.ParameterSchema.Slots[0].Kind = "text"
	if schema.Slots[0].Kind != "number" {
		t.Fatal("decoded response aliases caller")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ImportExampleNLQ(ctx, request); !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Fatal("cancelled import reached transport", err)
	}
}
