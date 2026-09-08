package nlqapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

type executionTokenFixture struct {
	cfg      config.Auth
	verifier *auth.Verifier
	server   *httptest.Server
	private  *ecdsa.PrivateKey
	now      time.Time
}

func newExecutionTokenFixture(t *testing.T) *executionTokenFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	document, err := json.Marshal(map[string]any{"keys": []any{map[string]any{
		"kty": "EC", "use": "sig", "alg": "ES256", "kid": "execution-test-key", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
		"y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &executionTokenFixture{private: key, now: time.Now().Truncate(time.Second)}
	fixture.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(document)
	}))
	t.Cleanup(fixture.server.Close)
	fixture.cfg = config.Defaults().Auth
	fixture.cfg.Issuer = fixture.server.URL + "/issuer"
	fixture.cfg.JWKSURL = fixture.server.URL + "/keys"
	fixture.cfg.Audience = ""
	fixture.cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	fixture.cfg.RefreshInterval = config.Duration(time.Second)
	fixture.cfg.JWKSMaxStale = config.Duration(10 * time.Second)
	fixture.verifier, err = auth.New(fixture.cfg, fixture.server.Client(), func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.verifier.Close)
	return fixture
}

func (f *executionTokenFixture) token(t *testing.T, session string, scopes []string) string {
	t.Helper()
	ordered := append([]string(nil), scopes...)
	sort.Strings(ordered)
	now := f.now.Unix()
	claims := jwt.MapClaims{
		"iss": f.cfg.Issuer, "aud": f.cfg.HTTPAudience(), "sub": "execution-user",
		"tenant": "execution-tenant", "user": "execution-user", "session": session,
		"iat": now, "nbf": now - 60, "exp": now + 300, "scopes": ordered,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "execution-test-key"
	signed, err := token.SignedString(f.private)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

type executionRouteReader struct{}

func (executionRouteReader) Route(context.Context, identity.Envelope, nlqroute.RouteRequest) (nlqroute.RouteResult, error) {
	return nlqroute.RouteResult{}, store.ErrNotFound
}

type executionTopicReader struct{}

func (executionTopicReader) Contract(context.Context, identity.Envelope, string) (topics.Contract, error) {
	return topics.Contract{}, store.ErrNotFound
}
func (executionTopicReader) RetainedContract(context.Context, identity.Envelope, string, string) (topics.Contract, error) {
	return topics.Contract{}, store.ErrNotFound
}

type executionSourceReader struct{}

func (executionSourceReader) Binding(context.Context, identity.Envelope, string, string) (readexec.Binding, error) {
	return readexec.Binding{}, store.ErrNotFound
}

type executionValidator struct{}

func (executionValidator) Validate(context.Context, identity.Envelope, readexec.Request) (readexec.Plan, error) {
	return readexec.Plan{}, store.ErrNotFound
}

type executionExecutor struct{}

func (executionExecutor) Execute(context.Context, identity.Envelope, readexec.Plan, readexec.Options) (readexec.ExecutionReport, error) {
	return readexec.ExecutionReport{}, store.ErrNotFound
}

type executionEngine struct{ calls int }

func (e *executionEngine) Generate(context.Context, gateway.Call, *gateway.Budget, string, string, string, *gateway.Schema) (gateway.Generated, error) {
	e.calls++
	return gateway.Generated{}, gateway.ErrDisabled
}
func (e *executionEngine) Embed(context.Context, gateway.Call, *gateway.Budget, string, []string) (gateway.Embedded, error) {
	e.calls++
	return gateway.Embedded{}, gateway.ErrDisabled
}
func (e *executionEngine) Rerank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	e.calls++
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (e *executionEngine) VisualRank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	e.calls++
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (*executionEngine) Space() string                          { return "execution-test" }
func (*executionEngine) EmbeddingSpace() gateway.EmbeddingSpace { return gateway.EmbeddingSpace{} }
func (*executionEngine) Close()                                 {}

type executionRepository struct{}

func (executionRepository) CreateSession(context.Context, store.Scope, nlqexec.SessionRecord) error {
	return nil
}
func (executionRepository) ReadSession(context.Context, store.Scope, string) (nlqexec.SessionRecord, error) {
	return nlqexec.SessionRecord{}, store.ErrNotFound
}
func (executionRepository) CreateQuery(context.Context, store.Scope, nlqexec.QueryRecord) error {
	return nil
}
func (executionRepository) ReadQuery(context.Context, store.Scope, string) (nlqexec.QueryRecord, error) {
	return nlqexec.QueryRecord{}, store.ErrNotFound
}
func (executionRepository) ReadOperation(context.Context, store.Scope, string) (nlqexec.QueryRecord, error) {
	return nlqexec.QueryRecord{}, store.ErrNotFound
}
func (executionRepository) UpdateQuery(context.Context, store.Scope, nlqexec.QueryRecord, int64) error {
	return nil
}
func (executionRepository) RecordFeedback(context.Context, store.Scope, nlqexec.FeedbackRecord) error {
	return nil
}
func (executionRepository) UpsertExample(context.Context, store.Scope, nlqexec.ExampleRecord) (nlqexec.ExampleRecord, error) {
	return nlqexec.ExampleRecord{}, store.ErrNotFound
}
func (executionRepository) ListExamples(context.Context, store.Scope, string, int) ([]nlqexec.ExampleRecord, error) {
	return nil, store.ErrNotFound
}
func (executionRepository) SetExampleState(context.Context, store.Scope, string, string) (nlqexec.ExampleRecord, error) {
	return nlqexec.ExampleRecord{}, store.ErrNotFound
}

func newExecutionTestService(t *testing.T) (*nlqexec.Service, *executionEngine) {
	t.Helper()
	engine := &executionEngine{}
	service, err := nlqexec.New(executionRouteReader{}, executionTopicReader{}, executionSourceReader{}, executionValidator{}, executionExecutor{}, engine, executionRepository{})
	if err != nil {
		t.Fatal(err)
	}
	return service, engine
}

func executionRequest(method, path, token, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func callExecutionHandler(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	h.ServeHTTP(response, executionRequest(method, path, token, body))
	return response
}

func executionErrorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body: %v (%s)", err, response.Body.String())
	}
	return body.Error
}

func TestExecutionHandlerDispatchesRegisteredErrorPaths(t *testing.T) {
	fixture := newExecutionTokenFixture(t)
	service, engine := newExecutionTestService(t)
	handler := ExecutionHandler(fixture.verifier, service, http.NotFoundHandler())
	scopes := []string{
		"query.preflight", "query.plan", "query.execute", "feedback.write",
		"cw.topic.read:topic", "cw.source.query:source", "cw.dataset.query:dataset", "cw.execution_context.use:context",
	}
	tests := []struct {
		name, path, body string
	}{
		{"preflight", "/v1/nlq/preflight", `{"topic":"topic","context":"context","locale":"en","question":"What is revenue?"}`},
		{"plan", "/v1/nlq/plans", `{"topic":"topic","context":"context","locale":"en","question":"What is revenue?","operation":"plan-op"}`},
		{"run", "/v1/nlq/runs", `{"query_id":"query","operation":"run-op"}`},
		{"refine", "/v1/nlq/refinements", `{"query_id":"query","topic":"topic","context":"context","locale":"en","question":"Show revenue by id"}`},
		{"feedback", "/v1/nlq/feedback", `{"query_id":"query","verdict":"positive"}`},
		{"example state", "/v1/nlq/examples/state", `{"example_id":"example","state":"active"}`},
		{"examples", "/v1/nlq/examples/read", `{"topic":"topic","limit":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session := strings.ReplaceAll(tc.name, " ", "_")
			response := callExecutionHandler(t, handler, http.MethodPost, tc.path, fixture.token(t, session, scopes), tc.body)
			if response.Code != http.StatusNotFound || executionErrorCode(t, response) != "not_found" {
				t.Fatalf("status=%d code=%q body=%s", response.Code, executionErrorCode(t, response), response.Body.String())
			}
		})
	}
	if engine.calls != 0 {
		t.Fatalf("error-only dispatch reached gateway %d times", engine.calls)
	}
}

func TestExecutionHandlerPreservesHTTPBoundaries(t *testing.T) {
	fixture := newExecutionTokenFixture(t)
	service, engine := newExecutionTestService(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	if response := callExecutionHandler(t, ExecutionHandler(nil, service, next), http.MethodGet, "/", "", ""); response.Code != http.StatusNotFound {
		t.Fatalf("nil verifier status=%d", response.Code)
	}
	if response := callExecutionHandler(t, ExecutionHandler(fixture.verifier, nil, next), http.MethodPost, "/v1/nlq/plans", "", ""); response.Code != http.StatusNoContent {
		t.Fatalf("nil service did not preserve next: status=%d", response.Code)
	}
	allScopes := []string{"query.plan", "cw.topic.read:topic"}
	token := fixture.token(t, "boundaries", allScopes)
	if response := callExecutionHandler(t, ExecutionHandler(fixture.verifier, service, next), http.MethodGet, "/v1/nlq/plans", token, ""); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("known method mismatch status=%d", response.Code)
	}
	if response := callExecutionHandler(t, ExecutionHandler(fixture.verifier, service, next), http.MethodPost, "/v1/nlq/plans?unexpected=1", token, `{"topic":"topic","context":"context","locale":"en","question":"What is revenue?"}`); response.Code != http.StatusBadRequest || executionErrorCode(t, response) != "invalid_request" {
		t.Fatalf("query string boundary status=%d code=%q", response.Code, executionErrorCode(t, response))
	}
	if response := callExecutionHandler(t, ExecutionHandler(fixture.verifier, service, next), http.MethodPost, "/v1/nlq/plans", token, `{"topic":7}`); response.Code != http.StatusBadRequest || executionErrorCode(t, response) != "invalid_request" {
		t.Fatalf("registered schema boundary status=%d code=%q", response.Code, executionErrorCode(t, response))
	}
	forbidden := fixture.token(t, "forbidden", []string{"cw.topic.read:topic"})
	if response := callExecutionHandler(t, ExecutionHandler(fixture.verifier, service, next), http.MethodPost, "/v1/nlq/plans", forbidden, `{"topic":"topic","context":"context","locale":"en","question":"What is revenue?"}`); response.Code != http.StatusForbidden || executionErrorCode(t, response) != "forbidden" {
		t.Fatalf("missing action status=%d code=%q", response.Code, executionErrorCode(t, response))
	}
	if response := callExecutionHandler(t, ExecutionHandler(fixture.verifier, service, next), http.MethodPost, "/v1/nlq/unknown", token, ""); response.Code != http.StatusNoContent {
		t.Fatalf("unknown path did not reach next: status=%d", response.Code)
	}
	if engine.calls != 0 {
		t.Fatalf("invalid or denied request reached gateway %d times", engine.calls)
	}
}
