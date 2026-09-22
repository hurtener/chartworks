package onboardingapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/store"
)

func TestOnboardingWireSchemas(t *testing.T) {
	types := []struct {
		name  string
		value any
	}{{"start", onboarding.StartRequest{}}, {"run", onboarding.Run{}}, {"resume", onboarding.ResumeRequest{}}, {"answer", onboarding.AnswerRequest{}}, {"cancel", onboarding.CancelRequest{}}, {"drift", onboarding.DriftRequest{}}, {"amendment", onboarding.Amendment{}}}
	for _, item := range types {
		t.Run(item.name, func(t *testing.T) {
			if _, err := api.SchemaFor("test"+item.name, reflect.TypeOf(item.value), false, api.OptionalJSONFields); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRegistryMatchesOnboardingManifest(t *testing.T) {
	registry, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../docs/contracts/chartworks-onboarding-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest []api.Operation
	if json.Unmarshal(raw, &manifest) != nil || !reflect.DeepEqual(manifest, registry.Operations()) {
		t.Fatal("onboarding operation manifest drift")
	}
	if len(registry.Definitions()) != 6 {
		t.Fatal("onboarding operation count", len(registry.Definitions()))
	}
	if _, err = registry.OpenAPI("Chartworks onboarding", "1"); err != nil {
		t.Fatal(err)
	}
}

type apiRepository struct{}

func (apiRepository) CreateOnboarding(context.Context, identity.Envelope, onboarding.Run, string) (onboarding.Run, error) {
	return onboarding.Run{}, store.ErrUnavailable
}
func (apiRepository) ReadOnboarding(context.Context, identity.Envelope, string) (onboarding.Run, error) {
	return onboarding.Run{}, store.ErrNotFound
}
func (apiRepository) SaveOnboarding(context.Context, identity.Envelope, onboarding.Run, int64) (onboarding.Run, error) {
	return onboarding.Run{}, store.ErrUnavailable
}

type apiAdapter struct{}

func (apiAdapter) ResolveRunAuthority(_ context.Context, _ identity.Envelope, r onboarding.Run) ([]onboarding.RunAuthority, error) {
	return []onboarding.RunAuthority{{Source: r.Input.Source, Context: r.Input.Context}}, nil
}

func (apiAdapter) Connect(context.Context, identity.Envelope, onboarding.StartRequest, string) (onboarding.StepResult, error) {
	return onboarding.StepResult{}, nil
}
func (apiAdapter) Inspect(context.Context, identity.Envelope, onboarding.Run, string) (onboarding.StepResult, error) {
	return onboarding.StepResult{}, nil
}
func (apiAdapter) Profile(context.Context, identity.Envelope, onboarding.Run, string) (onboarding.StepResult, error) {
	return onboarding.StepResult{}, nil
}
func (apiAdapter) DraftSemantics(context.Context, identity.Envelope, onboarding.Run, string) (onboarding.StepResult, error) {
	return onboarding.StepResult{}, nil
}
func (apiAdapter) PublishReviewed(context.Context, identity.Envelope, onboarding.Run, onboarding.ReviewReference, string) (onboarding.StepResult, error) {
	return onboarding.StepResult{}, nil
}
func (apiAdapter) ProposeQueriesBlocksReports(context.Context, identity.Envelope, onboarding.Run, string) (onboarding.StepResult, error) {
	return onboarding.StepResult{}, nil
}
func (apiAdapter) ProposeDriftAmendment(context.Context, identity.Envelope, onboarding.Run, onboarding.DriftRequest, string) (onboarding.Amendment, error) {
	return onboarding.Amendment{}, nil
}

func TestMCPBindingsAndHTTPBoundaries(t *testing.T) {
	service, err := onboarding.New(apiRepository{}, apiAdapter{}, onboarding.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := Registry()
	definition := registry.Definitions()[0]
	requestShape, requestErr := api.SchemaFor("probe", reflect.TypeFor[onboarding.StartRequest](), false, api.OptionalJSONFields)
	responseShape, responseErr := api.SchemaFor("probe", reflect.TypeFor[onboarding.Run](), true)
	if requestErr != nil || responseErr != nil {
		t.Fatal(requestErr, responseErr)
	}
	if !reflect.DeepEqual(requestShape.Document(), definition.Request.Document()) || !reflect.DeepEqual(responseShape.Document(), definition.Response.Document()) {
		t.Fatal("MCP and HTTP start schemas diverged")
	}
	bindings, err := MCPBindings(service)
	if err != nil || len(bindings) != 6 {
		t.Fatal(bindings, err)
	}
	if bindings, err = MCPBindings(nil); err != nil || bindings != nil {
		t.Fatal(bindings, err)
	}
	valid := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"expected_version":1}`))
	valid.Header.Set("Content-Type", "application/json")
	var resume onboarding.ResumeRequest
	if err = body(valid, &resume); err != nil || resume.ExpectedVersion != 1 {
		t.Fatal(resume, err)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"expected_version":1,"extra":true}`)),
		httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{} {}`)),
		httptest.NewRequest(http.MethodPost, "/", io.LimitReader(strings.NewReader(strings.Repeat("x", MaxBodyBytes+1)), MaxBodyBytes+1)),
	} {
		request.Header.Set("Content-Type", "application/json")
		if body(request, &resume) == nil {
			t.Fatal("invalid body accepted")
		}
	}
	wrong := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	if body(wrong, &resume) == nil {
		t.Fatal("missing content type accepted")
	}
	w := httptest.NewRecorder()
	Handler(nil, service, http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/onboarding/id", nil))
	if w.Code != http.StatusNotFound {
		t.Fatal(w.Code)
	}
}

func TestFailureProjection(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{access.ErrUnauthenticated, 401, "unauthenticated"}, {access.ErrForbidden, 403, "forbidden"},
		{store.ErrNotFound, 404, "not_found"}, {onboarding.ErrAttention, 409, "attention_required"},
		{onboarding.ErrCancelled, 409, "cancelled"}, {store.ErrConflict, 409, "conflict"},
		{onboarding.ErrBudget, 429, "budget_exhausted"}, {onboarding.ErrInvalid, 400, "invalid_request"},
		{errBodyLimit, 413, "limit_exceeded"}, {store.ErrUnavailable, 503, "unavailable"},
		{context.Canceled, 504, "cancelled_or_timed_out"}, {errors.New("private native detail"), 503, "unavailable"},
	} {
		w := httptest.NewRecorder()
		failure(w, test.err)
		if w.Code != test.status || !strings.Contains(w.Body.String(), test.code) || strings.Contains(w.Body.String(), "private native") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestMCPFailureProjection(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{access.ErrUnauthenticated, "unauthenticated"}, {access.ErrForbidden, "forbidden"},
		{store.ErrNotFound, "not_found"}, {store.ErrConflict, "conflict"},
		{onboarding.ErrAttention, "attention_required"}, {onboarding.ErrCancelled, "cancelled"},
		{onboarding.ErrBudget, "budget_exhausted"}, {store.ErrInvalid, "invalid_request"},
		{context.DeadlineExceeded, "cancelled_or_timed_out"}, {errors.New("private detail"), "unavailable"},
	} {
		if got := onboardingFault(test.err); got.Code != test.code || got.Outcome != "" {
			t.Fatal(test.err, got)
		}
	}
}

func TestHandlerDispatchesEveryOnboardingOperation(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwks, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{
		"kty": "EC", "use": "sig", "alg": "ES256", "kid": "onboarding-key", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
		"y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}}})
	jwksServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	t.Cleanup(jwksServer.Close)
	now := time.Now().Truncate(time.Second)
	cfg := config.Defaults().Auth
	cfg.Issuer, cfg.JWKSURL, cfg.Audience = jwksServer.URL+"/issuer", jwksServer.URL+"/keys", ""
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	verifier, err := auth.New(cfg, jwksServer.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(verifier.Close)
	scopes := []string{"onboarding.read", "onboarding.write", "onboarding.cancel", "cw.tenant.write:*", "cw.onboarding.read:run", "cw.onboarding.write:run", "cw.onboarding.cancel:run", "cw.source.read:source", "cw.execution_context.use:context"}
	sort.Strings(scopes)
	claims := jwt.MapClaims{"iss": cfg.Issuer, "aud": cfg.HTTPAudience(), "sub": "actor", "tenant": "tenant", "user": "actor", "session": "session", "iat": now.Unix(), "nbf": now.Unix() - 60, "exp": now.Unix() + 300, "scopes": scopes}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "onboarding-key"
	bearer, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	service, _ := onboarding.New(apiRepository{}, apiAdapter{}, onboarding.DefaultLimits())
	handler := Handler(verifier, service, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Fallthrough", "true")
		w.WriteHeader(http.StatusNotFound)
	}))
	start := onboarding.StartRequest{ID: "run", Key: "run-key", Mode: onboarding.ModeConnect, Locale: "en", Source: "source", Context: "context", Dataset: "dataset", Profile: "profile", Topic: "topic", TopicVersion: "topic-v1", Block: "block", Report: "report"}
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/v1/onboarding", start},
		{http.MethodGet, "/v1/onboarding/run", nil},
		{http.MethodPost, "/v1/onboarding/resume", onboarding.ResumeRequest{ID: "run", ExpectedVersion: 1}},
		{http.MethodPost, "/v1/onboarding/answers", onboarding.AnswerRequest{ID: "run", ExpectedVersion: 1, Answers: []onboarding.Answer{}}},
		{http.MethodPost, "/v1/onboarding/cancel", onboarding.CancelRequest{ID: "run", ExpectedVersion: 1, Reason: "stop"}},
		{http.MethodPost, "/v1/onboarding/drift", onboarding.DriftRequest{ID: "run", ExpectedVersion: 1}},
	}
	for _, test := range cases {
		var reader io.Reader
		if test.body != nil {
			raw, _ := json.Marshal(test.body)
			reader = strings.NewReader(string(raw))
		}
		request := httptest.NewRequest(test.method, test.path, reader)
		request.Header.Set("Authorization", "Bearer "+bearer)
		if test.body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Header().Get("X-Fallthrough") != "" {
			t.Fatal("registered route fell through", test.path)
		}
	}
	bindings, err := MCPBindings(service)
	if err != nil {
		t.Fatal(err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	mcpServer, err := mcpserver.New(verifier, mcpRegistry, config.Defaults().MCP, nil)
	if err != nil {
		t.Fatal(err)
	}
	mcpClaims := claims
	mcpClaims["aud"] = cfg.MCPAudience()
	mcpClaims["scopes"] = append(scopes, "mcp.use")
	mcpToken := jwt.NewWithClaims(jwt.SigningMethodES256, mcpClaims)
	mcpToken.Header["kid"] = "onboarding-key"
	mcpBearer, err := mcpToken.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mcpServer.Client(func(context.Context) (string, error) { return mcpBearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		name string
		body any
	}{
		{"start_onboarding", start},
		{"get_onboarding", IDRequest{ID: "run"}},
		{"resume_onboarding", onboarding.ResumeRequest{ID: "run", ExpectedVersion: 1}},
		{"answer_onboarding", onboarding.AnswerRequest{ID: "run", ExpectedVersion: 1}},
		{"cancel_onboarding", onboarding.CancelRequest{ID: "run", ExpectedVersion: 1, Reason: "stop"}},
		{"propose_onboarding_drift", onboarding.DriftRequest{ID: "run", ExpectedVersion: 1}},
	} {
		raw, _ := json.Marshal(call.body)
		result, callErr := client.CallTool(t.Context(), call.name, raw)
		if callErr != nil || result == nil || !result.IsError {
			t.Fatal("MCP binding did not project the domain error", call.name, callErr)
		}
	}
}
