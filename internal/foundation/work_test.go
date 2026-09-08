package foundation

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/telemetry"
	"github.com/hurtener/chartworks/test/support"
)

func TestWorkAssemblyLifecycle(t *testing.T) {
	ctx := context.Background()
	db := support.Open(t, support.Database(t))
	v := config.Defaults()
	v.Auth.Issuer = "https://issuer.example.test"
	v.Auth.JWKSURL = "https://issuer.example.test/jwks"
	v.Auth.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp", Jobs: "chartworks:execution"}
	verifier, err := auth.New(v.Auth, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	// #nosec G101 -- environment-variable references only; synthetic test credentials are resolved separately.
	v.Gateway.Bifrost.Providers = []config.Provider{{Name: "openai", APIKey: "env:MODEL_TEST_KEY"}}
	for _, role := range config.RoleNames() {
		if config.OptionalRole(role) {
			continue
		}
		v.Gateway.Roles[role] = config.Role{Provider: "openai", Model: "synthetic-model", ModelRevision: "test-1", Timeout: config.Duration(time.Second), MaxTokens: 32, Dimensions: 2, MaxBatchItems: 2, MaxBatchBytes: 1024}
	}
	v.Features.Gateway = true
	v.Jobs.Enabled = true
	v.Jobs.BrokerURL = "https://issuer.example.test/exchange/execution-authority"
	// #nosec G101 -- environment-variable references only; synthetic test credentials are resolved separately.
	v.Jobs.Credentials = []config.BrokerCredential{{Tenant: "tenant", ClientID: "env:CLIENT_TEST_ID", ClientSecret: "env:CLIENT_TEST_SECRET"}}
	lookup := func(string) (string, bool) { return "SYNTHETIC_ASSEMBLY_ONLY_NOT_A_LIVE_CREDENTIAL", true }
	w, err := setupWork(ctx, v, db, verifier, http.NotFoundHandler(), lookup, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if definition, _, ok := w.registry.Match(http.MethodPost, "/v1/nlq/routes"); !ok || definition.ID != "routeNLQ" {
		t.Fatalf("gateway-enabled work omitted NLQ registry entry: %#v", definition)
	}
	cfg, err := config.Load(bytes.NewBufferString(`{"auth":{"issuer":"https://issuer.example.test","jwks_url":"https://issuer.example.test/jwks","audience":"chartworks:http"}}`), func(key string) (string, bool) {
		if key == "CHARTWORKS_STORE_URL" {
			return support.Database(t), true
		}
		return "", false
	}, config.Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := telemetry.New(io.Discard, "json", true)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServerWithRegistry(cfg, reporter, func(context.Context) Dependency { return Dependency{Ready: true} }, func(context.Context) Dependency { return Dependency{Ready: true} }, w.registry, w.handler)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	response, err := httpServer.Client().Get(httpServer.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("runtime OpenAPI status=%d", response.StatusCode)
	}
	var document struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	if operation := document.Paths["/v1/nlq/routes"]["post"]; operation == nil || operation["operationId"] != "routeNLQ" {
		t.Fatalf("runtime OpenAPI omitted executable NLQ route: %#v", document.Paths["/v1/nlq/routes"])
	}
	// SDK construction makes no model request; an empty durable queue makes no broker pull.
	w.run(ctx)
	w.close()
	w.close()
	missing := func(string) (string, bool) { return "", false }
	if _, err := setupWork(ctx, v, db, verifier, http.NotFoundHandler(), missing, io.Discard); err == nil {
		t.Fatal("missing model key accepted")
	}
	v.Features.Gateway = false
	if _, err := setupWork(ctx, v, db, verifier, http.NotFoundHandler(), missing, io.Discard); err == nil {
		t.Fatal("missing broker credential accepted")
	}
	bad := v
	bad.Jobs.BrokerURL = "http://untrusted.test/exchange/execution-authority"
	if _, err := setupWork(ctx, bad, db, verifier, http.NotFoundHandler(), lookup, io.Discard); err == nil {
		t.Fatal("insecure broker accepted")
	}
	bad = v
	bad.Jobs.GlobalConcurrency++
	if _, err := setupWork(ctx, bad, db, verifier, http.NotFoundHandler(), lookup, io.Discard); err == nil {
		t.Fatal("replicas silently disagree on queue bounds")
	}
	v.Jobs.Enabled = false
	w, err = setupWork(ctx, v, db, verifier, http.NotFoundHandler(), missing, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, known := w.registry.Match(http.MethodPost, "/v1/nlq/routes"); known {
		t.Fatal("gateway-disabled work registered NLQ without a matching handler")
	}
	w.run(ctx)
	w.close()
	if _, err := setupWork(ctx, v, nil, verifier, http.NotFoundHandler(), missing, io.Discard); err == nil {
		t.Fatal("missing metadata store accepted")
	}
}
