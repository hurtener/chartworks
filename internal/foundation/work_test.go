package foundation

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/telemetry"
	"github.com/hurtener/chartworks/test/support"
)

func TestWorkAssemblyLifecycle(t *testing.T) {
	ctx := context.Background()
	workDSN := support.Database(t)
	db := support.Open(t, workDSN)
	fixture := support.Raw(t, workDSN)
	var readerEntropy [16]byte
	if _, err := cryptorand.Read(readerEntropy[:]); err != nil {
		t.Fatal("create isolated read role", err)
	}
	readerRole := "cw_work_reader_" + hex.EncodeToString(readerEntropy[:])
	readerPassword := hex.EncodeToString(readerEntropy[:])
	if _, err := fixture.Exec(t.Context(), `CREATE SCHEMA analytics; CREATE TABLE analytics.sales(id bigint,amount numeric)`); err != nil {
		t.Fatal("create source fixture", err)
	}
	if _, err := fixture.Exec(t.Context(), "CREATE ROLE "+readerRole+" LOGIN PASSWORD '"+readerPassword+"'"); err != nil {
		t.Fatal("create isolated source reader", err)
	}
	if _, err := fixture.Exec(t.Context(), "GRANT USAGE ON SCHEMA analytics TO "+readerRole+"; GRANT SELECT ON analytics.sales TO "+readerRole); err != nil {
		t.Fatal("grant source reader access", err)
	}
	t.Cleanup(func() {
		if _, err := fixture.Exec(context.Background(), "DROP OWNED BY "+readerRole); err != nil {
			t.Error("drop isolated source reader grants", err)
		}
		if _, err := fixture.Exec(context.Background(), "DROP ROLE "+readerRole); err != nil {
			t.Error("drop isolated source reader", err)
		}
	})
	readerURL, err := url.Parse(workDSN)
	if err != nil {
		t.Fatal(err)
	}
	readerURL.User = url.UserPassword(readerRole, readerPassword)
	readerDSN := readerURL.String()
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
			return readerDSN, true
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
	actor, err := identity.FromVerified("tenant", "operator", "session", []string{"migration.read", "migration.write", "scheduling.write", "reporting.write", "reporting.publish", "reporting.execute", "ops.maintain", "sources.write", "sources.read", "cw.tenant.read:tenant", "cw.tenant.write:tenant", "cw.tenant.erase:tenant", "cw.execution_binding.use:maintenance", "cw.schedule.write:*", "cw.report.write:migration-report", "cw.report.publish:migration-report", "cw.report.execute:migration-report", "cw.source.write:sales", "cw.source.read:sales", "cw.execution_context.use:sales:v1"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.SetPolicy(t.Context(), support.Scope(t, "tenant", "operator"), 0, store.Policy{AuditDays: 7, OperationHours: 24}); err != nil {
		t.Fatal("configure imported schedule policy", err)
	}
	reportDefinition := reporting.DocumentDefinition{SchemaVersion: reporting.DocumentVersion, Locale: "en-US", Timezone: "UTC", Metadata: []reporting.DocumentMetadata{{Locale: "en-US", Title: "Synthetic migration schedule target"}}, Widgets: []reporting.Widget{{ID: "note", Kind: "text", Grid: reporting.GridCell{Width: 12, Height: 1}, Text: &reporting.TextWidget{Format: "plain", Text: "synthetic schedule target"}}}}
	reportState, err := w.documents.Create(t.Context(), actor, "report", "migration-report", reportDefinition)
	if err != nil {
		t.Fatal("create governed reporting schedule target", err)
	}
	reviewedReport, err := w.documents.Transition(t.Context(), actor, "report", "migration-report", reportState.Version, 1, "review", "synthetic migration schedule review")
	if err != nil {
		t.Fatal("review governed reporting schedule target", err)
	}
	if _, err = w.documents.Transition(t.Context(), actor, "report", "migration-report", reviewedReport.Version, 1, "publish", "synthetic migration schedule test"); err != nil {
		t.Fatal("publish governed reporting schedule target", err)
	}
	reportTarget := jobs.ReportingTarget{Type: "report", ID: "migration-report", Revision: 1, Locale: "en-US", Timezone: "UTC", Budget: jobs.ReportingBudget{TimeoutMillis: 1000, MaxRows: 1, MaxBytes: 1024, QueryAttempts: 1}}
	scheduleRaw, _ := json.Marshal(scheduleImport{Key: "migration-work-schedule", Request: jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.ReportingKind, BindingID: "maintenance", Reporting: &reportTarget}, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}})
	hash := strings.Repeat("a", 64)
	evidence := []migration.Evidence{}
	for _, group := range []struct {
		prefix string
		count  int
	}{{"B", 20}, {"R", 16}, {"Q", 10}, {"N", 16}} {
		for i := 1; i <= group.count; i++ {
			feature := fmt.Sprintf("%s%02d", group.prefix, i)
			evidence = append(evidence, migration.Evidence{Feature: feature, OwnerFeature: feature, Disposition: "required", Outcome: "passed", EvidenceType: "live", Reference: "evidence-" + feature, Source: "evaluation", SourceVersion: hash, EvidenceHash: hash, ComparisonHash: hash, Engine: "postgres", Dialect: "postgres", SourceSnapshot: hash, SourceRevision: 1})
		}
	}
	evidence = append(evidence, migration.Evidence{Feature: "Q11", OwnerFeature: "EVAL-01", Disposition: "excluded", Outcome: "unsupported", EvidenceType: "operator", Reference: "excluded", Source: "synthetic", SourceVersion: hash, EvidenceHash: hash})
	registered, err := w.sourceService.Create(t.Context(), actor, sources.CreateRequest{ID: "sales", Name: "Sales", Connection: "sales"})
	if err != nil {
		t.Fatal("register migration destination source", err)
	}
	snapshot := sourceSnapshot(registered)
	sourceEvidence := append([]migration.Evidence(nil), evidence...)
	for i := range sourceEvidence {
		if sourceEvidence[i].Disposition == "required" {
			sourceEvidence[i].Engine, sourceEvidence[i].Dialect = registered.Dialect, registered.Dialect
			sourceEvidence[i].SourceSnapshot, sourceEvidence[i].SourceRevision = snapshot, registered.Revision
		}
	}
	sourceRaw, _ := json.Marshal(sourceImport{Engine: registered.Dialect, Dialect: registered.Dialect, Snapshot: snapshot, Context: registered.ContextID, Revision: registered.Revision})
	sourceManifest := migration.Manifest{Version: migration.ManifestVersion, Batch: "work-source", Cohort: "work-source", SourceSnapshot: snapshot, Engine: registered.Dialect, Dialect: registered.Dialect, Mappings: []migration.Mapping{{Kind: migration.KindSource, ExternalRef: "source", Destination: registered.ID, Revision: registered.Revision}}, Objects: []migration.Object{{Kind: migration.KindSource, ExternalRef: "source", Revision: registered.Revision, PayloadVersion: "v1", Payload: string(sourceRaw), Lifecycle: "private_draft", Private: true, Origin: "synthetic"}}, Fields: []migration.FieldDisposition{{Path: "source.engine", Status: "retained"}, {Path: "source.dialect", Status: "retained"}, {Path: "source.snapshot", Status: "retained"}, {Path: "source.context", Status: "retained"}, {Path: "source.revision", Status: "retained"}}, Evidence: sourceEvidence}
	if batch, importErr := w.migrations.Import(t.Context(), actor, migration.ImportRequest{Manifest: sourceManifest}); importErr != nil || batch.Applied != 1 {
		t.Fatal("migration did not validate actual source binding", importErr, batch)
	}
	drifted := sourceManifest
	drifted.Batch, drifted.Cohort = "work-source-drift", "work-source-drift"
	drifted.SourceSnapshot = strings.Repeat("b", 64)
	drifted.Evidence = append([]migration.Evidence(nil), sourceEvidence...)
	for i := range drifted.Evidence {
		if drifted.Evidence[i].Disposition == "required" {
			drifted.Evidence[i].SourceSnapshot = drifted.SourceSnapshot
		}
	}
	drifted.Objects = append([]migration.Object(nil), sourceManifest.Objects...)
	driftedRaw, _ := json.Marshal(sourceImport{Engine: registered.Dialect, Dialect: registered.Dialect, Snapshot: drifted.SourceSnapshot, Context: registered.ContextID, Revision: registered.Revision})
	drifted.Objects[0].Payload = string(driftedRaw)
	if _, driftErr := w.migrations.DryRun(t.Context(), actor, migration.DryRunRequest{Manifest: drifted}); !errors.Is(driftErr, migration.ErrConflict) {
		t.Fatal("migration accepted caller-only source snapshot", driftErr)
	}
	mismatchedEngine := sourceManifest
	mismatchedEngine.Batch, mismatchedEngine.Cohort = "work-source-engine", "work-source-engine"
	mismatchedEngine.Engine, mismatchedEngine.Dialect = "mysql", "mysql"
	mismatchedEngine.Evidence = append([]migration.Evidence(nil), sourceEvidence...)
	for i := range mismatchedEngine.Evidence {
		if mismatchedEngine.Evidence[i].Disposition == "required" {
			mismatchedEngine.Evidence[i].Engine, mismatchedEngine.Evidence[i].Dialect = "mysql", "mysql"
		}
	}
	mismatchedEngine.Objects = append([]migration.Object(nil), sourceManifest.Objects...)
	mismatchedSource, _ := json.Marshal(sourceImport{Engine: "mysql", Dialect: "mysql", Snapshot: snapshot, Context: registered.ContextID, Revision: registered.Revision})
	mismatchedEngine.Objects[0].Payload = string(mismatchedSource)
	if _, engineErr := w.migrations.DryRun(t.Context(), actor, migration.DryRunRequest{Manifest: mismatchedEngine}); !errors.Is(engineErr, migration.ErrConflict) {
		t.Fatal("migration accepted caller-only engine and dialect", engineErr)
	}
	mismatchedRevision := sourceManifest
	mismatchedRevision.Batch, mismatchedRevision.Cohort = "work-source-revision", "work-source-revision"
	mismatchedRevision.Mappings = append([]migration.Mapping(nil), sourceManifest.Mappings...)
	mismatchedRevision.Mappings[0].Revision++
	if _, revisionErr := w.migrations.DryRun(t.Context(), actor, migration.DryRunRequest{Manifest: mismatchedRevision}); !errors.Is(revisionErr, migration.ErrConflict) {
		t.Fatal("migration accepted a stale mapped source revision", revisionErr)
	}
	manifest := migration.Manifest{Version: migration.ManifestVersion, Batch: "work-schedule", Cohort: "work-schedule", SourceSnapshot: hash, Engine: "postgres", Dialect: "postgres", Objects: []migration.Object{{Kind: migration.KindSchedule, ExternalRef: "schedule", Revision: 1, PayloadVersion: "v1", Payload: string(scheduleRaw), Lifecycle: "private_draft", Private: true, Origin: "synthetic"}}, Fields: []migration.FieldDisposition{{Path: "schedule.key", Status: "retained"}, {Path: "schedule.request", Status: "retained"}}, Evidence: evidence}
	scheduleBatch, importErr := w.migrations.Import(t.Context(), actor, migration.ImportRequest{Manifest: manifest})
	if importErr != nil || scheduleBatch.Applied != 1 {
		t.Fatal("migration did not use dispatch queue", importErr, scheduleBatch)
	}
	var imported int
	if err = support.Raw(t, workDSN).QueryRow(t.Context(), `SELECT count(*) FROM chartworks.job_schedules WHERE tenant_id='tenant' AND client_key=$1 AND NOT enabled`, migrationScheduleKey(scheduleBatch.Digest, "schedule", "migration-work-schedule")).Scan(&imported); err != nil || imported != 1 {
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
