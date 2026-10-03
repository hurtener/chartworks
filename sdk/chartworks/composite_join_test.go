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

func TestSQLRecoveryCompositeJoinSDKWire(t *testing.T) {
	request := chartworks.SaveTopicDraftRequest{Expected: 2, Change: "Reviewed complete composite relationship", Pack: chartworks.TopicPack{SchemaVersion: 1, Topic: "commerce", Version: "v3", Joins: []chartworks.TopicJoin{{ID: "relation", Left: chartworks.TopicReference{Kind: "column", Dataset: "orders", ID: "customer"}, Right: chartworks.TopicReference{Kind: "column", Dataset: "customers", ID: "id"}, AdditionalKeys: []chartworks.TopicJoinKeyPair{{Left: chartworks.TopicReference{Kind: "column", Dataset: "orders", ID: "realm"}, Right: chartworks.TopicReference{Kind: "column", Dataset: "customers", ID: "realm"}}}}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/topic-drafts" || r.Header.Get("Authorization") != "Bearer synthetic-composite-token" {
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
	client, err := chartworks.New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-composite-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.SaveTopicDraft(context.Background(), request)
	if err != nil || !reflect.DeepEqual(got.Pack.Joins, request.Pack.Joins) {
		t.Fatal("public composite relationship forwarding", err)
	}
	got.Pack.Joins[0].AdditionalKeys[0].Left.ID = "changed"
	if request.Pack.Joins[0].AdditionalKeys[0].Left.ID != "realm" {
		t.Fatal("decoded response aliases caller policy")
	}
}
