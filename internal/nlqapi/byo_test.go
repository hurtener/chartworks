package nlqapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/nlqbyo"
)

func TestBYOVersionOneManifestGoldenAndClosedSchema(t *testing.T) {
	r, err := BYORegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../docs/contracts/chartworks-byo-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []api.Operation
	if err = json.Unmarshal(raw, &want); err != nil || !reflect.DeepEqual(want, r.Operations()) {
		t.Fatal("manifest drift", err, r.Operations())
	}
	doc, err := r.OpenAPI("Chartworks BYO", "1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range r.Definitions() {
		if !bytes.Contains(doc, []byte(d.ID)) || d.Request.Validate([]byte(`{"tenant":"foreign"}`), MaxBodyBytes) == nil {
			t.Fatal("operation not closed", d.ID)
		}
	}
	offline, err := BYORegistry(false)
	if err != nil || len(offline.Operations()) != 2 {
		t.Fatal(err)
	}
	if _, _, found := offline.Match("POST", "/v1/nlq/contexts"); found {
		t.Fatal("uninstalled route advertised")
	}
	golden, err := os.ReadFile("testdata/byo-submit-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := r.Match("POST", "/v1/nlq/sql")
	if err = d.Request.Validate(golden, MaxBodyBytes); err != nil {
		t.Fatal("v1 golden rejected", err)
	}
	var in nlqbyo.SubmitRequest
	decoder := json.NewDecoder(bytes.NewReader(golden))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&in); err != nil || !nlqbyo.ReferenceValid(in.Reference) || in.Operation != "external-step-1" || len(in.Parameters) != 1 {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var original, roundTrip any
	if json.Unmarshal(golden, &original) != nil || json.Unmarshal(encoded, &roundTrip) != nil || !reflect.DeepEqual(original, roundTrip) {
		t.Fatal("v1 wire compatibility lost")
	}
	for _, bad := range []string{strings.Replace(string(golden), `"schema_version": 1`, `"schema_version": 2`, 1), strings.Replace(string(golden), `"context":`, `"tenant":"foreign","context":`, 1), strings.Replace(string(golden), `"parameters":`, `"limits":{"rows":99999},"parameters":`, 1)} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/nlq/sql", strings.NewReader(bad))
		var value nlqbyo.SubmitRequest
		if err = decodeBody(w, req, d.Request, &value); err == nil {
			t.Fatal("unknown version/authority/limits admitted")
		}
	}
	for _, cause := range []struct {
		err    error
		status int
		code   string
	}{{nlqbyo.ErrInvalid, 400, "invalid_request"}, {nlqbyo.ErrReplan, 409, "replan_required"}, {nlqbyo.ErrBudget, 429, "bundle_budget_exhausted"}, {nlqbyo.ErrUnavailable, 503, "unavailable"}} {
		w := httptest.NewRecorder()
		failure(w, cause.err)
		if w.Code != cause.status || !strings.Contains(w.Body.String(), cause.code) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	BYOHandler(nil, nil, http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest("POST", "/v1/nlq/sql", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
