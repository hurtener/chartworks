package chartworks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/sdk/chartworks"
)

func TestSQLRecoveryGroundedConceptSDKPolicyAndChoices(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/nlq/routes" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("wrong request boundary", r.URL.Path)
		}
		var in chartworks.NLQRouteRequest
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil || in.ConceptPolicy != chartworks.NLQGroundedConceptPolicy {
			t.Error("lost grounded policy", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"concept_selection":{"policy":"grounded-v1","input_digest":"input","catalog_digest":"catalog","choice":{"version":"grounded-concept-choice-v1","question_digest":"question","candidate_digest":"candidates","decision":"clarify","alternatives":["a","b"],"digest":"digest"},"options":[{"id":"a","topic":"topic","reference":{"kind":"measure","id":"revenue"},"label":"Revenue"}]}}`))
	}))
	defer server.Close()
	client, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "fixture", nil })
	if err != nil {
		t.Fatal(err)
	}
	in := chartworks.NLQRouteRequest{ConceptPolicy: chartworks.NLQGroundedConceptPolicy, Topic: "topic", Context: "context", Question: "income", Locale: "en"}
	out, err := client.RouteNLQ(context.Background(), in)
	if err != nil || out.Concepts == nil || len(out.Concepts.Options) != 1 || out.Concepts.Options[0].Reference.ID != "revenue" || calls != 1 {
		t.Fatal("lost reviewed option", err)
	}
}
