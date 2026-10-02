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

func TestSQLRecoveryGroupedPopulationSDKWire(t *testing.T) {
	request := chartworks.SaveTopicDraftRequest{Expected: 2, Change: "Reviewed independent group alignment", Pack: chartworks.TopicPack{SchemaVersion: 1, Topic: "commerce", Version: "v3", GroupedPopulation: &chartworks.TopicGroupedPopulationPolicy{Policy: chartworks.TopicGroupedPopulationUnionPolicy, Datasets: []string{"orders", "refunds"}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/topic-drafts" || r.Header.Get("Authorization") != "Bearer synthetic-grouped-token" {
			t.Error("existing authenticated authoring transport changed")
			w.WriteHeader(400)
			return
		}
		var got chartworks.SaveTopicDraftRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&got); err != nil || !reflect.DeepEqual(got, request) {
			t.Error("reviewed policy altered", err)
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chartworks.TopicDraft{Pack: got.Pack})
	}))
	defer server.Close()
	client, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-grouped-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.SaveTopicDraft(context.Background(), request)
	if err != nil || !reflect.DeepEqual(got.Pack.GroupedPopulation, request.Pack.GroupedPopulation) {
		t.Fatal("public grouped policy forwarding", err)
	}
	got.Pack.GroupedPopulation.Datasets[0] = "changed"
	if request.Pack.GroupedPopulation.Datasets[0] != "orders" {
		t.Fatal("decoded response aliases caller policy")
	}
}
