package foundation

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
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
	w.run(ctx)
	w.close()
	if _, err := setupWork(ctx, v, nil, verifier, http.NotFoundHandler(), missing, io.Discard); err == nil {
		t.Fatal("missing metadata store accepted")
	}
}
