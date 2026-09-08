package nlqapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

func TestRegistryIncludesNLQRoute(t *testing.T) {
	if _, err := api.SchemaFor("debugResponse", reflect.TypeFor[nlqroute.RouteResult](), true); err != nil {
		t.Fatalf("response schema: %v", err)
	}
	if _, err := api.SchemaFor("debugRequest", reflect.TypeFor[nlqroute.RouteRequest](), false, api.OptionalJSONFields); err != nil {
		t.Fatalf("request schema: %v", err)
	}
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	defs := r.Definitions()
	if len(defs) != 1 || defs[0].ID != "routeNLQ" || defs[0].Path != "/v1/nlq/routes" || defs[0].Request == nil || defs[0].Response == nil {
		t.Fatalf("unexpected route registry: %#v", defs)
	}
	if got, _, ok := r.Match("POST", "/v1/nlq/routes"); !ok || got.Action != "topics.read" {
		t.Fatalf("route did not match: %#v", got)
	}
	raw, err := os.ReadFile("../../docs/contracts/chartworks-nlq-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest []api.Operation
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	actual := r.Operations()
	sort.Slice(manifest, func(i, j int) bool {
		if manifest[i].Path != manifest[j].Path {
			return manifest[i].Path < manifest[j].Path
		}
		return manifest[i].Method < manifest[j].Method
	})
	sort.Slice(actual, func(i, j int) bool {
		if actual[i].Path != actual[j].Path {
			return actual[i].Path < actual[j].Path
		}
		return actual[i].Method < actual[j].Method
	})
	if !reflect.DeepEqual(manifest, actual) {
		t.Fatalf("manifest differs from actual registration: manifest=%#v actual=%#v", manifest, actual)
	}
}

func TestFailureMapsPublicErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"unauthenticated", access.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated"},
		{"forbidden", access.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"not found", access.ErrNotFound, http.StatusNotFound, "not_found"},
		{"invalid store", store.ErrInvalid, http.StatusBadRequest, "invalid_request"},
		{"invalid semantics", semantics.ErrInvalid, http.StatusBadRequest, "invalid_request"},
		{"invalid route", nlqroute.ErrInvalid, http.StatusBadRequest, "invalid_request"},
		{"conflict", store.ErrConflict, http.StatusConflict, "conflict"},
		{"binding", readexec.ErrBinding, http.StatusConflict, "context_changed"},
		{"limit", readexec.ErrLimit, http.StatusRequestEntityTooLarge, "limit_exceeded"},
		{"insufficient", nlq.ErrInsufficient, http.StatusUnprocessableEntity, "insufficient_context"},
		{"cancelled", context.Canceled, http.StatusGatewayTimeout, "cancelled_or_timed_out"},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout, "cancelled_or_timed_out"},
		{"space", gateway.ErrSpace, http.StatusConflict, "context_changed"},
		{"provider unavailable", gateway.ErrUnavailable, http.StatusServiceUnavailable, "unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			failure(response, tc.err)
			if response.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.status, response.Body.String())
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error != tc.code {
				t.Fatalf("code=%q want=%q", body.Error, tc.code)
			}
		})
	}
}

func TestDecodeBodyEnforcesRegisteredJSONBoundary(t *testing.T) {
	registry, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	requestSchema := registry.Definitions()[0].Request
	valid := `{"topic":"topic","context":"ctx","locale":"en","question":"What is revenue?"}`
	tests := []struct {
		name string
		body string
		mime string
		want error
	}{
		{name: "valid", body: valid, mime: "application/json"},
		{name: "media parameters rejected", body: valid, mime: "application/json; charset=utf-8", want: store.ErrInvalid},
		{name: "wrong media", body: valid, mime: "text/plain", want: store.ErrInvalid},
		{name: "malformed json", body: "{", mime: "application/json", want: store.ErrInvalid},
		{name: "schema rejected", body: `{"topic":7}`, mime: "application/json", want: store.ErrInvalid},
		{name: "body limit", body: strings.Repeat("a", MaxBodyBytes+1), mime: "application/json", want: readexec.ErrLimit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/nlq/routes", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", tc.mime)
			response := httptest.NewRecorder()
			var input nlqroute.RouteRequest
			err := decodeBody(response, req, requestSchema, &input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("decode error=%v want=%v", err, tc.want)
			}
			if tc.want == nil && (input.Topic != "topic" || input.Context != "ctx" || input.Question != "What is revenue?") {
				t.Fatalf("decoded input=%#v", input)
			}
		})
	}
}

func TestHandlerNilBoundariesRemainSafe(t *testing.T) {
	if got := Handler(nil, nil, http.NotFoundHandler()); got == nil {
		t.Fatal("nil verifier returned nil handler")
	}
	response := httptest.NewRecorder()
	Handler(nil, nil, http.NotFoundHandler()).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("nil verifier status=%d", response.Code)
	}

	cfg := config.Defaults().Auth
	cfg.Issuer = "https://issuer.example.test"
	cfg.JWKSURL = "https://issuer.example.test/jwks"
	cfg.Audience = ""
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	verifier, err := auth.New(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	response = httptest.NewRecorder()
	Handler(verifier, nil, next).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/nlq/routes", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("nil service did not preserve next handler: status=%d", response.Code)
	}
	if got := Handler(verifier, nil, nil); got == nil {
		t.Fatal("nil next returned nil handler")
	}
}
