package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/foundation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/securityapi"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/telemetry"
	"github.com/hurtener/chartworks/internal/topicapi"
	"github.com/hurtener/chartworks/internal/workapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

type phase21Engine struct{}

func (phase21Engine) Generate(context.Context, gateway.Call, *gateway.Budget, string, string, string, *gateway.Schema) (gateway.Generated, error) {
	return gateway.Generated{}, gateway.ErrDisabled
}
func (phase21Engine) Embed(context.Context, gateway.Call, *gateway.Budget, string, []string) (gateway.Embedded, error) {
	return gateway.Embedded{}, gateway.ErrDisabled
}
func (phase21Engine) Rerank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (phase21Engine) VisualRank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (phase21Engine) Space() string                          { return "phase21-fixture" }
func (phase21Engine) EmbeddingSpace() gateway.EmbeddingSpace { return gateway.EmbeddingSpace{} }
func (phase21Engine) Close()                                 {}

type phase21Repository struct{}

func (phase21Repository) ConfigureQueue(context.Context, jobs.Limits) error { return nil }
func (phase21Repository) AdmitJob(context.Context, store.Scope, string, string, jobs.Submission, jobs.Limits) (jobs.Job, error) {
	return jobs.Job{}, jobs.ErrEmpty
}
func (phase21Repository) ReadJob(context.Context, store.Scope, string) (jobs.Job, error) {
	return jobs.Job{}, jobs.ErrEmpty
}
func (phase21Repository) ListJobs(context.Context, store.Scope, access.Selection, int) ([]jobs.Job, error) {
	return nil, jobs.ErrEmpty
}
func (phase21Repository) CancelJob(context.Context, store.Scope, string) (jobs.Job, error) {
	return jobs.Job{}, jobs.ErrEmpty
}
func (phase21Repository) ClaimJob(context.Context, string, jobs.Limits) (jobs.Lease, error) {
	return jobs.Lease{}, jobs.ErrEmpty
}
func (phase21Repository) HeartbeatJob(context.Context, jobs.Lease, time.Duration) error {
	return jobs.ErrEmpty
}
func (phase21Repository) CompleteJob(context.Context, jobs.Lease, auth.Execution) (jobs.Job, error) {
	return jobs.Job{}, jobs.ErrEmpty
}
func (phase21Repository) FinishAttempt(context.Context, jobs.Lease, string, bool, time.Duration) error {
	return jobs.ErrEmpty
}
func (phase21Repository) CreateSchedule(context.Context, store.Scope, string, string, jobs.ScheduleRequest, jobs.Limits) (jobs.Schedule, error) {
	return jobs.Schedule{}, jobs.ErrEmpty
}
func (phase21Repository) ReadSchedule(context.Context, store.Scope, string) (jobs.Schedule, error) {
	return jobs.Schedule{}, jobs.ErrEmpty
}
func (phase21Repository) SetSchedule(context.Context, store.Scope, string, int64, bool) (jobs.Schedule, error) {
	return jobs.Schedule{}, jobs.ErrEmpty
}
func (phase21Repository) FireSchedule(context.Context, store.Scope, string, string, string, jobs.Limits) (jobs.Job, error) {
	return jobs.Job{}, jobs.ErrEmpty
}
func (phase21Repository) TickSchedules(context.Context, jobs.Limits) (int, error) {
	return 0, jobs.ErrEmpty
}

type phase21Authority struct{}

func (phase21Authority) Acquire(context.Context, jobs.Job) (auth.Execution, error) {
	return auth.Execution{}, jobs.ErrAuthority
}

func phase21Registry(t *testing.T) *api.Registry {
	t.Helper()
	public, err := foundation.PublicRegistry()
	if err != nil {
		t.Fatal(err)
	}
	security, err := securityapi.APIRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := jobs.New(phase21Repository{}, phase21Authority{}, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	work, err := workapi.APIRegistry(phase21Engine{}, queue)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := sourceapi.SourceRegistry(true, true)
	if err != nil {
		t.Fatal(err)
	}
	engineering, err := sourceapi.EngineeringAPIRegistry(true, true, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := sourceapi.ExecutionAPIRegistry()
	if err != nil {
		t.Fatal(err)
	}
	pipelines, err := sourceapi.PipelineAPIRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	topics, err := topicapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	nlq, err := nlqapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	nlqExec, err := nlqapi.ExecutionRegistry()
	if err != nil {
		t.Fatal(err)
	}
	byo, err := nlqapi.BYORegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	chart, err := chartapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	composed, err := api.Compose(public, security, work, sources, engineering, execution, pipelines, topics, nlq, nlqExec, byo, chart)
	if err != nil {
		t.Fatal(err)
	}
	return composed
}

func TestPhase21(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		verifyPhase21RegisteredDenials(t)
		f := newSourceFixture(t, nil)
		registry, err := sourceapi.SourceRegistry(true, true)
		if err != nil {
			t.Fatal(err)
		}
		h := sourceapi.Handler(f.token.verifier, f.s, f.validator, http.NotFoundHandler())
		before := f.lookups.Load()
		bare := f.token.sign(t, f.token.claims("source-a", "reader", nil), nil)
		for _, operation := range registry.Operations() {
			path := strings.ReplaceAll(operation.Path, "{id}", "missing")
			if got := callProtected(t, h, operation.Method, path, "", "{}", nil); got.Code != http.StatusUnauthorized {
				t.Fatalf("missing bearer %s %s: %d", operation.Method, path, got.Code)
			}
			if got := callProtected(t, h, operation.Method, path, bare, "{}", nil); got.Code != http.StatusForbidden {
				t.Fatalf("missing signed action %s %s: %d", operation.Method, path, got.Code)
			}
		}
		if f.lookups.Load() != before {
			t.Fatal("denied source operation resolved a credential")
		}
		created := f.create(t, "phase21-source")
		token := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil)
		server := httptest.NewServer(h)
		defer server.Close()
		client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
		if err != nil {
			t.Fatal(err)
		}
		got, err := client.Source(context.Background(), created.ID)
		if err != nil || got.ID != created.ID || got.ContextID != created.ContextID {
			t.Fatalf("real PostgreSQL source read: %+v %v", got, err)
		}
	})

	t.Run("AC02", func(t *testing.T) {
		f := newTokenFixture(t)
		_, h, counted := protectedFixture(t, f)
		registry, err := securityapi.APIRegistry(true)
		if err != nil {
			t.Fatal(err)
		}
		bare := f.sign(t, f.claims("tenant", "reader", nil), nil)
		before := counted.reads.Load()
		for _, operation := range registry.Operations() {
			path := strings.ReplaceAll(operation.Path, "{id}", "missing")
			if got := callProtected(t, h, operation.Method, path, "", "{}", nil); got.Code != http.StatusUnauthorized {
				t.Fatalf("missing bearer %s %s: %d", operation.Method, path, got.Code)
			}
			if got := callProtected(t, h, operation.Method, path, bare, "{}", nil); got.Code != http.StatusForbidden {
				t.Fatalf("missing scope %s %s: %d", operation.Method, path, got.Code)
			}
		}
		if counted.reads.Load() != before {
			t.Fatal("denied security operation reached the store")
		}
		foreign := f.sign(t, f.claims("tenant", "reader", operationalScopes("foreign")), nil)
		if got := callProtected(t, h, "GET", "/v1/retention-policy", foreign, "", nil); got.Code != http.StatusNotFound {
			t.Fatalf("foreign tenant reach status=%d", got.Code)
		}
		valid := f.sign(t, f.claims("tenant", "reader", operationalScopes("tenant")), nil)
		if got := callProtected(t, h, "GET", "/v1/access/diagnostics", valid, "", nil); got.Code != http.StatusOK {
			t.Fatalf("valid protected read status=%d", got.Code)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		cfg := loaded(t, configBytes(t, func(m map[string]any) {
			m["server"] = map[string]any{"cors_allowlist": []string{"https://app.example"}}
		}), "fixture")
		reporter, err := telemetry.New(io.Discard, "json", true)
		if err != nil {
			t.Fatal(err)
		}
		registry := phase21Registry(t)
		s, err := foundation.NewServerWithRegistry(cfg, reporter, func(context.Context) foundation.Dependency { return foundation.Dependency{Ready: true} }, func(context.Context) foundation.Dependency { return foundation.Dependency{Ready: true} }, registry, http.NotFoundHandler())
		if err != nil {
			t.Fatal(err)
		}
		h := s.Handler()
		allowed := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		allowed.Header.Set("Origin", "https://app.example")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, allowed)
		if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
			t.Fatalf("allowed origin: %d %v", response.Code, response.Header())
		}
		denied := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		denied.Header.Set("Origin", "https://foreign.example")
		response = httptest.NewRecorder()
		h.ServeHTTP(response, denied)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "forbidden_origin") {
			t.Fatalf("denied origin: %d %s", response.Code, response.Body.String())
		}
		options := httptest.NewRequest(http.MethodOptions, "/v1/sources", nil)
		options.Header.Set("Origin", "https://app.example")
		response = httptest.NewRecorder()
		h.ServeHTTP(response, options)
		if response.Code != http.StatusNoContent {
			t.Fatalf("CORS preflight status=%d", response.Code)
		}
		for _, request := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "/openapi.json?extra=1", nil),
			httptest.NewRequest(http.MethodGet, "/healthz", strings.NewReader("body")),
		} {
			response = httptest.NewRecorder()
			h.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("ambiguous public request status=%d", response.Code)
			}
		}
		for _, path := range []string{"/healthz", "/readyz", "/capabilities"} {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				response = httptest.NewRecorder()
				h.ServeHTTP(response, httptest.NewRequest(method, path+"?extra=1", nil))
				if response.Code != http.StatusBadRequest {
					t.Fatalf("undeclared public query accepted for %s %s: %d", method, path, response.Code)
				}
			}
		}
		large := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		large.ContentLength = 100 << 20
		response = httptest.NewRecorder()
		h.ServeHTTP(response, large)
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("large public body status=%d", response.Code)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		registry := phase21Registry(t)
		definitions := registry.Definitions()
		wire, err := registry.OpenAPI("Chartworks HTTP API", "21")
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Servers []map[string]any                     `json:"servers"`
			Paths   map[string]map[string]map[string]any `json:"paths"`
		}
		if err := json.Unmarshal(wire, &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Servers) != 1 || document.Servers[0]["url"] != "/" {
			t.Fatalf("server metadata=%v", document.Servers)
		}
		for _, definition := range definitions {
			path, ok := document.Paths[definition.Path]
			if !ok {
				t.Fatalf("unregistered OpenAPI path %s", definition.Path)
			}
			operation, ok := path[strings.ToLower(definition.Method)]
			if !ok || operation["operationId"] != definition.ID {
				t.Fatalf("unregistered OpenAPI operation %s %s", definition.Method, definition.Path)
			}
			if definition.Public {
				if operation["x-chartworks-auth"] != "none" {
					t.Fatalf("public operation advertised bearer: %s", definition.ID)
				}
			} else if operation["security"] == nil || operation["x-chartworks-action"] != definition.Action {
				t.Fatalf("protected metadata incomplete: %s", definition.ID)
			}
			responses := operation["responses"].(map[string]any)
			for _, failure := range definition.Errors {
				if _, ok := responses[strconv.Itoa(failure.Status)]; !ok {
					t.Fatalf("error %s/%s missing status %d", definition.Method, definition.Path, failure.Status)
				}
			}
			if definition.Request != nil && operation["requestBody"] == nil {
				t.Fatalf("request schema missing: %s", definition.ID)
			}
		}
	})

	t.Run("AC05", func(t *testing.T) {
		registry := phase21Registry(t)
		for _, definition := range registry.Definitions() {
			path := strings.ToLower(definition.Path)
			for _, forbidden := range []string{"/admin", "/auth", "/credentials", "/sessions", "/embed", "/reports", "/render"} {
				if strings.Contains(path, forbidden) {
					t.Fatalf("unimplemented or identity route registered: %s", definition.Path)
				}
			}
			if strings.HasPrefix(path, "/v1/") && definition.Public {
				t.Fatalf("protected path marked public: %s", definition.Path)
			}
			if !definition.Public && (definition.Action == "" || definition.ResourceLoader == "" || definition.Audit == "") {
				t.Fatalf("protected definition lacks authority metadata: %s", definition.ID)
			}
			if definition.Response == nil || len(definition.Errors) == 0 {
				t.Fatalf("definition lacks wire contract: %s", definition.ID)
			}
		}
		if registry, err := workapi.APIRegistry(nil, nil); err != nil || registry != nil {
			t.Fatalf("disabled work capabilities still registered: %v %v", registry, err)
		}
	})

	t.Run("AC06", func(t *testing.T) {
		cfg := loaded(t, configBytes(t, nil), "fixture")
		reporter, err := telemetry.New(io.Discard, "json", true)
		if err != nil {
			t.Fatal(err)
		}
		registry := phase21Registry(t)
		s, err := foundation.NewServerWithRegistry(cfg, reporter, func(context.Context) foundation.Dependency { return foundation.Dependency{Ready: true} }, func(context.Context) foundation.Dependency { return foundation.Dependency{Ready: true} }, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(s.Handler())
		defer server.Close()
		client := server.Client()
		var wg sync.WaitGroup
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, path := range []string{"/healthz", "/readyz", "/capabilities", "/openapi.json"} {
					response, err := client.Get(server.URL + path)
					if err != nil {
						t.Error(err)
						return
					}
					want := http.StatusOK
					if path == "/readyz" {
						want = http.StatusServiceUnavailable
					}
					if response.StatusCode != want {
						t.Errorf("%s status=%d", path, response.StatusCode)
					}
					_ = response.Body.Close()
				}
			}()
		}
		wg.Wait()
		response, err := client.Get(server.URL + "/capabilities")
		if err != nil {
			t.Fatal(err)
		}
		var capabilities foundation.CapabilitiesResponse
		if err := json.NewDecoder(response.Body).Decode(&capabilities); err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if !capabilities.BusinessAPI || !capabilities.Authentication || capabilities.Phase != "01-21-http" {
			t.Fatalf("capabilities=%+v", capabilities)
		}
		if !contains(capabilities.Implemented, "http_api") || !contains(capabilities.Implemented, "openapi") || !contains(capabilities.Implemented, "output_specifications") || contains(capabilities.Implemented, "reporting") {
			t.Fatalf("capability projection=%v", capabilities.Implemented)
		}
		request, err := http.NewRequest(http.MethodHead, server.URL+"/openapi.json", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err = client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || len(body) != 0 {
			t.Fatalf("HEAD openapi status=%d body=%d err=%v", response.StatusCode, len(body), err)
		}
	})
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

var _ jobs.Repository = phase21Repository{}
var _ jobs.Authority = phase21Authority{}
var _ gateway.Engine = phase21Engine{}
