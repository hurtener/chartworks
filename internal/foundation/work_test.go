package foundation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/migration"
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
	v.Sources.Enabled = true
	v.Sources.Connections = []config.SourceConnection{{
		Tenant: "tenant", ID: "sales", Version: "v1", ReadDSN: "env:SOURCE_TEST_DSN",
		Relations: []config.SourceRelation{{Schema: "analytics", Name: "sales", Columns: []string{"id", "amount"}}},
	}}
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
	v.Features.MCP = true
	v.Jobs.Enabled = true
	v.Jobs.BrokerURL = "https://issuer.example.test/exchange/execution-authority"
	// #nosec G101 -- environment-variable references only; synthetic test credentials are resolved separately.
	v.Jobs.Credentials = []config.BrokerCredential{{Tenant: "tenant", ClientID: "env:CLIENT_TEST_ID", ClientSecret: "env:CLIENT_TEST_SECRET"}}
	lookup := func(key string) (string, bool) {
		if key == "SOURCE_TEST_DSN" {
			return support.Database(t), true
		}
		return "SYNTHETIC_ASSEMBLY_ONLY_NOT_A_LIVE_CREDENTIAL", true
	}
	w, err := setupWork(ctx, v, db, verifier, http.NotFoundHandler(), lookup, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if definition, _, ok := w.registry.Match(http.MethodPost, "/v1/nlq/routes"); !ok || definition.ID != "routeNLQ" {
		t.Fatalf("gateway-enabled work omitted NLQ registry entry: %#v", definition)
	}
	if definition, _, ok := w.registry.Match(http.MethodPost, "/v1/mcp"); !ok || definition.Surface != auth.MCP || definition.Action != "mcp.use" {
		t.Fatalf("MCP mount omitted verified audience/action: %#v", definition)
	}
	serverDSN := support.Database(t)
	if definition, _, ok := w.registry.Match(http.MethodPost, "/v1/nlq/plans"); !ok || definition.ID != "planNLQ" {
		t.Fatalf("gateway-enabled work omitted NLQ execution entry: %#v", definition)
	}
	cfg, err := config.Load(bytes.NewBufferString(`{"auth":{"issuer":"https://issuer.example.test","jwks_url":"https://issuer.example.test/jwks","audience":"chartworks:http"}}`), func(key string) (string, bool) {
		if key == "CHARTWORKS_STORE_URL" {
			return serverDSN, true
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
	t.Cleanup(func() {
		if err := response.Body.Close(); err != nil {
			t.Error("close OpenAPI response", err)
		}
	})
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
	if operation := document.Paths["/v1/nlq/plans"]["post"]; operation == nil || operation["operationId"] != "planNLQ" {
		t.Fatalf("runtime OpenAPI omitted executable NLQ plan: %#v", document.Paths["/v1/nlq/plans"])
	}
	// The migration service must capture the dispatch-capable queue, not the
	// metadata-only placeholder installed at the start of composition.
	actor, err := identity.FromVerified("tenant", "operator", "session", []string{"migration.read", "migration.write", "scheduling.write", "cw.tenant.read:tenant", "cw.tenant.write:tenant", "cw.execution_binding.use:maintenance", "cw.schedule.write:*"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scheduleRaw, _ := json.Marshal(scheduleImport{Key: "migration-work-schedule", Request: jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}})
	hash := strings.Repeat("a", 64)
	evidence := []migration.Evidence{}
	for _, group := range []struct {
		prefix string
		count  int
	}{{"B", 20}, {"R", 16}, {"Q", 10}, {"N", 16}} {
		for i := 1; i <= group.count; i++ {
			feature := fmt.Sprintf("%s%02d", group.prefix, i)
			evidence = append(evidence, migration.Evidence{Feature: feature, OwnerFeature: "EVAL-01", Disposition: "required", Outcome: "passed", EvidenceType: "live", Reference: "evidence-" + feature, Source: "evaluation", SourceVersion: hash, EvidenceHash: hash})
		}
	}
	evidence = append(evidence, migration.Evidence{Feature: "Q11", OwnerFeature: "EVAL-01", Disposition: "excluded", Outcome: "unsupported", EvidenceType: "operator", Reference: "excluded", Source: "synthetic", SourceVersion: hash, EvidenceHash: hash})
	manifest := migration.Manifest{Version: migration.ManifestVersion, Batch: "work-schedule", Cohort: "work-schedule", SourceSnapshot: hash, Engine: "postgres", Dialect: "postgres", Objects: []migration.Object{{Kind: migration.KindSchedule, ExternalRef: "schedule", Revision: 1, PayloadVersion: "v1", Payload: string(scheduleRaw), Lifecycle: "private_draft", Private: true, Origin: "synthetic"}}, Fields: []migration.FieldDisposition{{Path: "schedule.key", Status: "retained"}, {Path: "schedule.request", Status: "retained"}}, Evidence: evidence}
	if batch, importErr := w.migrations.Import(t.Context(), actor, migration.ImportRequest{Manifest: manifest}); importErr != nil || batch.Applied != 1 {
		t.Fatal("migration did not use dispatch queue", importErr, batch)
	}
	var imported int
	if err = support.Raw(t, support.Database(t)).QueryRow(t.Context(), `SELECT count(*) FROM chartworks.job_schedules WHERE tenant_id='tenant' AND client_key='migration-work-schedule' AND NOT enabled`).Scan(&imported); err != nil || imported != 1 {
		t.Fatal("migration schedule missing", err, imported)
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
