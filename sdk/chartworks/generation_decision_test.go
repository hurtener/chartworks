package chartworks_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/sdk/chartworks"
)

func TestSQLRecoveryDecisionSDKBoundedProblem(t *testing.T) {
	good := `{"version":"generation-decision-v1","outcome":"clarify","questions":["¿Qué métrica corresponde?"]}`
	if p := chartworks.DecodeGenerationProblem([]byte(good)); p == nil || p.Outcome != "clarify" {
		t.Fatal("valid public problem")
	}
	for _, bad := range []string{`{}`, strings.Replace(good, `"clarify"`, `"ready"`, 1), strings.Replace(good, `"version":`, `"extra":true,"version":`, 1), strings.Replace(good, `"clarify"`, `"clarify","outcome":"ready"`, 1), strings.Replace(good, "¿Qué métrica corresponde?", strings.Repeat("q", 513), 1)} {
		if chartworks.DecodeGenerationProblem([]byte(bad)) != nil {
			t.Fatal("malformed untrusted metadata accepted")
		}
	}
}
func TestSQLRecoveryDecisionSDKNoAutomaticRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture-token" || r.URL.Path != "/v1/nlq/plans" {
			t.Error("transport boundary")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(422)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "generation_clarification_required", "generation": chartworks.GenerationProblem{Version: "generation-decision-v1", Outcome: "clarify", Questions: []string{"Choose the intended metric."}}})
	}))
	defer server.Close()
	client, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "fixture-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	out, err := client.PlanNLQ(context.Background(), chartworks.NLQPlanRequest{})
	var status *chartworks.StatusError
	if !errors.As(err, &status) || status.Status != 422 || status.Generation == nil || status.Generation.Outcome != "clarify" || out.QueryID != "" || calls.Load() != 1 || strings.Contains(status.Error(), "intended metric") {
		t.Fatal("SDK lost outcome, leaked text or retried mutation", err)
	}
}
