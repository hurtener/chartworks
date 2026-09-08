package chartworks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestRunNLQAcceptsBoundedExecutionResultEnvelope(t *testing.T) {
	payload := strings.Repeat("x", 3<<20)
	server := nlqRunServer(t, payload)
	defer server.Close()

	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.RunNLQ(context.Background(), NLQRunRequest{QueryID: "query", Operation: "operation"})
	if err != nil {
		t.Fatalf("valid result over 2 MiB rejected: %v", err)
	}
	if result.Execution.Result == nil || len(result.Execution.Result.Rows) != 1 || len(result.Execution.Result.Rows[0]) != 1 {
		t.Fatalf("result shape lost: %#v", result.Execution.Result)
	}
	var value string
	if err := json.Unmarshal(result.Execution.Result.Rows[0][0], &value); err != nil || value != payload {
		t.Fatalf("result value mismatch: length=%d err=%v", len(value), err)
	}
}

func TestRunNLQRejectsExcessiveExecutionResultEnvelope(t *testing.T) {
	server := nlqRunServer(t, strings.Repeat("x", nlqRunResponseLimit))
	defer server.Close()

	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RunNLQ(context.Background(), NLQRunRequest{QueryID: "query", Operation: "operation"})
	if err == nil || !strings.Contains(err.Error(), "invalid response") {
		t.Fatalf("excessive result error=%v", err)
	}
}

func nlqRunServer(t *testing.T, value string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/nlq/runs" || r.Header.Get("Authorization") != "Bearer synthetic-token" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var request NLQRunRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.QueryID != "query" || request.Operation != "operation" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Errorf("encode result value: %v", err)
			return
		}
		response := NLQRunResult{
			QueryID:   "query",
			SessionID: "session",
			Status:    "succeeded",
			Execution: readexec.ExecutionReport{Result: &readexec.Result{
				Schema:  []readexec.Field{{Name: "value", Type: "text", Encoding: "string", NativeType: "text"}},
				Rows:    [][]json.RawMessage{{encoded}},
				Outcome: "succeeded",
				Bytes:   len(encoded),
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
}
