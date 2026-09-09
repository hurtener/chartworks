package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/foundation"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/securityapi"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/telemetry"
	"github.com/hurtener/chartworks/internal/topicapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func TestPhase23(t *testing.T) {
	f := newPhase23Fixture(t)
	t.Run("AC01", func(t *testing.T) { phase23Parity(t, f) })
	t.Run("AC02", func(t *testing.T) { phase23Inventory(t, f) })
	t.Run("AC03", func(t *testing.T) { phase23Authority(t, f) })
	t.Run("AC04", func(t *testing.T) { phase23Command(t, f) })
	t.Run("AC05", func(t *testing.T) { phase23Replay(t, f) })
	t.Run("AC06", func(t *testing.T) { phase23Erasure(t, f) })
}

func phase23Parity(t *testing.T, f *phase23Fixture) {
	t.Helper()
	ref := f.domain.pack.Datasets[0].Source
	var expectedData json.RawMessage
	var expectedPublished json.RawMessage
	for _, surface := range f.surfaces(t) {
		t.Run(surface.name, func(t *testing.T) {
			listed := phase23Call[[]topics.Summary](t, surface, "listTopics", "", topics.ListRequest{Limit: 32}, nil)
			if len(listed) < 2 {
				t.Fatal("missing real published topics")
			}
			published := phase23Call[topics.Published](t, surface, "getPublishedTopic", f.domain.pack.Topic, nil, topicapi.PublishedTopicRequest{Topic: f.domain.pack.Topic})
			if published.Definition.Topic != f.domain.pack.Topic {
				t.Fatal("wrong published topic")
			}
			canonical, err := json.Marshal(published)
			if err != nil {
				t.Fatal(err)
			}
			if expectedPublished == nil {
				expectedPublished = canonical
			} else if !bytes.Equal(canonical, expectedPublished) {
				t.Fatal("published core value differs by surface")
			}
			datasets := phase23Call[[]sources.Dataset](t, surface, "listDatasets", "", sources.DatasetListRequest{Source: ref.Source, Context: ref.Context, Limit: 32}, nil)
			if len(datasets) == 0 {
				t.Fatal("missing retained source datasets")
			}
			dataset := phase23Call[sources.Dataset](t, surface, "describeDataset", "", sources.DatasetDescribeRequest{Source: ref.Source, Context: ref.Context, Dataset: ref.Dataset}, nil)
			if dataset.Relation.ID != ref.Dataset || len(dataset.Relation.Columns) == 0 {
				t.Fatal("wrong actual dataset")
			}
			question := phase18Question(f.domain, nlq.LanguageEnglish, f.domain.pack.Topic)
			preflight := phase23Call[nlqexec.PreflightResult](t, surface, "preflightNLQ", "", nlqexec.PreflightRequest{QuestionRequest: question}, nil)
			if preflight.QueryID == "" || preflight.Route.Context == nil {
				t.Fatal("preflight did not persist governed routing")
			}
			operation := "phase23-" + surface.name
			plan := phase23Call[nlqexec.PlanResult](t, surface, "planNLQ", "", nlqexec.PlanRequest{QuestionRequest: question, Operation: operation}, nil)
			if plan.Status != "planned" || plan.SQL != "SELECT id, amount FROM analytics.sales ORDER BY id" {
				t.Fatal("validated SQL differs by surface", plan.Status)
			}
			run := phase23Call[nlqexec.RunResult](t, surface, "runNLQ", "", nlqexec.RunRequest{QueryID: plan.QueryID, Operation: operation, Rows: 10, Bytes: 4096}, nil)
			if run.Status != "succeeded" || run.Execution.Result == nil || len(run.Execution.Result.Rows) != 2 {
				t.Fatal("actual read did not execute")
			}
			data, err := json.Marshal(run.Execution.Result.Rows)
			if err != nil {
				t.Fatal(err)
			}
			if expectedData == nil {
				expectedData = data
			} else if !bytes.Equal(data, expectedData) {
				t.Fatal("exact returned values differ by surface")
			}
			refined := phase23Call[nlqexec.PlanResult](t, surface, "refineNLQ", "", nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Show revenue by id", Kinds: question.Kinds, LimitPerKind: 1}}, nil)
			if refined.QueryID == "" || refined.QueryID == plan.QueryID || refined.Status != "planned" {
				t.Fatal("refinement lost child lineage")
			}
			feedback := phase23Call[nlqapi.FeedbackResult](t, surface, "feedbackNLQ", "", nlqexec.FeedbackRequest{QueryID: plan.QueryID, Verdict: "positive", Note: "Synthetic cross-surface review"}, nil)
			if !feedback.Accepted {
				t.Fatal("feedback was not retained")
			}
			created := phase23Call[nlqbyo.CreateResult](t, surface, "getQueryContext", "", nlqbyo.CreateRequest{SchemaVersion: 1, Route: nlqroute.RouteRequest{Topic: f.domain.pack.Topic, Context: ref.Context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 1}}, nil)
			if created.Bundle == nil || created.Bundle.Reference.Context != ref.Context {
				t.Fatal("missing exact opaque BYO context")
			}
			submission := nlqbyo.SubmitRequest{Reference: created.Bundle.Reference, Operation: operation + "-byo", SQL: "SELECT id, amount FROM analytics.sales ORDER BY id", Parameters: []readexec.Parameter{}}
			step := phase23Call[nlqbyo.SubmitResult](t, surface, "submitSQL", "", submission, nil)
			if !step.ValuesAvailable || step.Result == nil || len(step.Result.Rows) != 2 {
				t.Fatal("missing BYO values")
			}
			data, err = json.Marshal(step.Result.Rows)
			if err != nil || !bytes.Equal(data, expectedData) {
				t.Fatal("BYO differs from governed read", err)
			}
			replay := phase23Call[nlqbyo.SubmitResult](t, surface, "submitSQL", "", submission, nil)
			if !replay.Replayed || replay.ValuesAvailable || replay.Result != nil || replay.Step.Operation != submission.Operation {
				t.Fatal("receipt replay reran or invented retained values")
			}
			view := phase23Call[nlqbyo.View](t, surface, "readQueryContext", "", created.Bundle.Reference, nil)
			if len(view.Steps) != 1 || view.Steps[0].Operation != submission.Operation {
				t.Fatal("durable BYO evidence missing")
			}
		})
	}
}

// Cumulative phase-21 coverage: compare the complete owning registry, not a
// hand-maintained subset. Reporting rows cannot appear before their owners land.
func phase23Inventory(t *testing.T, f *phase23Fixture) {
	t.Helper()
	registry := phase21Registry(t)
	document, err := registry.OpenAPI("Chartworks installed inventory", "23")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := cw.ParseOperations(document)
	if err != nil || len(rows) != len(registry.Definitions()) {
		t.Fatal("full registry cannot be called by generated clients", len(rows), err)
	}
	byID := map[string]cw.OperationInfo{}
	for _, row := range rows {
		byID[row.ID] = row
		if row.Audience == "mcp" {
			if row.SDKMethod != "MCP" || row.CLICommand != "client mcp" {
				t.Fatal("MCP protocol command missing")
			}
		} else if row.SDKMethod != "Invoke" || row.CLICommand != "client call "+row.ID {
			t.Fatal("ordinary operation has no SDK/CLI path")
		}
		for _, forbidden := range []string{"/reports", "/artifacts", "/dashboards", "/auth", "/admin", "/credentials", "/grants", "/users"} {
			if strings.Contains(row.Path, forbidden) {
				t.Fatal("unbuilt/identity route exposed", row.ID)
			}
		}
	}
	for _, definition := range registry.Definitions() {
		row, ok := byID[definition.ID]
		if !ok || row.Method != definition.Method || row.Path != definition.Path || row.Action != definition.Action || row.Effect != definition.Effect || row.Audit != definition.Audit || row.ResourceLoader != definition.ResourceLoader || row.MaxBodyBytes != definition.MaxBodyBytes {
			t.Fatal("operation metadata drift", definition.ID)
		}
	}
	client := f.sdk(t, f.httpToken, false)
	mcpToken := f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase23-session", f.scopes, true)
	matrix, err := client.OperationMatrix(t.Context(), f.sdk(t, mcpToken, false))
	if err != nil {
		t.Fatal(err)
	}
	tools := 0
	for _, row := range matrix {
		if row.MCPTool != "" {
			tools++
		}
	}
	if tools != len(f.registry.Manifest()) {
		t.Fatal("installed MCP/HTTP/SDK/CLI matrix incomplete", tools)
	}
	beforeModel, beforeSource := f.domain.model.requests.Load(), f.domain.f.lookups.Load()
	for _, future := range []string{"runReport", "readArtifact", "exportArtifact", "createGrant"} {
		if _, err := client.Invoke(t.Context(), future, cw.CallOptions{}); !errors.Is(err, cw.ErrUnknownOperation) {
			t.Fatal("future command synthesized", future, err)
		}
		code, _, _ := f.cli(t.Context(), f.httpToken, []string{"call", future, "--execute"}, nil)
		if code != 2 {
			t.Fatal("CLI future command synthesized", future, code)
		}
	}
	if f.domain.model.requests.Load() != beforeModel || f.domain.f.lookups.Load() != beforeSource {
		t.Fatal("inventory or unbuilt commands performed source/model work")
	}
}

func phase23Authority(t *testing.T, f *phase23Fixture) {
	t.Helper()
	e, err := f.authority.verifier.Verify(t.Context(), f.httpToken, auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	ambient, err := e.Context(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var supplied atomic.Value
	supplied.Store(f.httpToken)
	client, err := cw.NewInProcessWithOptions(f.handler, func(context.Context) (string, error) { return supplied.Load().(string), nil }, cw.InProcessOptions{BasePath: phase23Mount})
	if err != nil {
		t.Fatal(err)
	}
	ref := f.domain.pack.Datasets[0].Source
	request := cw.DatasetDescribeRequest{Source: ref.Source, Context: ref.Context, Dataset: ref.Dataset}
	if _, err := client.DescribeDataset(ambient, request); err != nil {
		t.Fatal("valid in-process authority", err)
	}
	beforeModel, beforeSource := f.domain.model.requests.Load(), f.domain.f.lookups.Load()
	mcpToken := f.token(t, e.Tenant(), e.User(), "phase23-session", f.scopes, true)
	bare := f.token(t, e.Tenant(), e.User(), "phase23-session", nil, false)
	for _, token := range []string{"invalid", mcpToken, bare} {
		supplied.Store(token)
		_, err := client.DescribeDataset(ambient, request)
		var rejected *cw.StatusError
		if !errors.As(err, &rejected) || (rejected.Status != 401 && rejected.Status != 403) {
			t.Fatal("ambient envelope bypassed fresh verification", err)
		}
	}
	if f.domain.model.requests.Load() != beforeModel || f.domain.f.lookups.Load() != beforeSource {
		t.Fatal("authority denial reached a domain dependency")
	}
	supplied.Store(f.httpToken)
	if _, err := client.DescribeDataset(t.Context(), request); err != nil {
		t.Fatal("caller-provided renewal not honored", err)
	}
	foreign := f.token(t, "phase23-foreign", e.User(), "phase23-session", f.scopes, false)
	for _, inProcess := range []bool{false, true} {
		_, err := f.sdk(t, foreign, inProcess).DescribeDataset(t.Context(), request)
		var rejected *cw.StatusError
		if !errors.As(err, &rejected) || rejected.Status != 404 {
			t.Fatal("cross-tenant data escaped", err)
		}
	}
	narrow := phase22Without(f.scopes, "cw.execution_context.use:")
	narrow = append(narrow, "cw.execution_context.use:unrelated-context")
	token := f.token(t, e.Tenant(), e.User(), "phase23-session", narrow, false)
	if _, err := f.sdk(t, token, true).DescribeDataset(t.Context(), request); err == nil {
		t.Fatal("in-process resource reach widened")
	}
	mcpLocal, err := f.server.Client(func(context.Context) (string, error) { return f.httpToken, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcpLocal.ListTools(ambient); err == nil {
		t.Fatal("HTTP audience used for in-process MCP")
	}
}

func phase23Command(t *testing.T, f *phase23Fixture) {
	t.Helper()
	before := f.domain.model.requests.Load()
	for _, args := range [][]string{{"config"}, {"operations"}, {"operations", "--schemas"}} {
		code, output, diagnostic := f.cli(t.Context(), f.httpToken, args, nil)
		if code != 0 || !json.Valid(output) || diagnostic != "" || strings.Contains(string(output), f.httpToken) {
			t.Fatal("injected CLI read/configuration", args, code)
		}
	}
	for _, args := range [][]string{{"bootstrap"}, {"users"}, {"grants"}, {"keys"}, {"config", "--token", "PRIVATE_ARG"}, {"call", "getPublishedTopic", "--id", f.domain.pack.Topic}} {
		code, output, diagnostic := f.cli(t.Context(), f.httpToken, args, nil)
		if code != 2 || strings.Contains(string(output)+diagnostic, "PRIVATE_ARG") {
			t.Fatal("unsafe CLI command or diagnostics", code)
		}
	}
	body, _ := json.Marshal(topicapi.PublishedTopicRequest{Topic: f.domain.pack.Topic})
	code, output, diagnostic := f.cli(t.Context(), "not-a-token", []string{"call", "getPublishedTopic", "--id", f.domain.pack.Topic, "--execute"}, body)
	if code != 3 || len(output) != 0 || strings.Contains(diagnostic, "not-a-token") {
		t.Fatal("CLI denial exit/projection", code)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	code, _, _ = f.cli(ctx, f.httpToken, []string{"config"}, nil)
	if code != 130 {
		t.Fatal("canceled command reported success", code)
	}
	if f.domain.model.requests.Load() != before {
		t.Fatal("configuration/diagnostics invoked a model")
	}
}

func phase23Replay(t *testing.T, f *phase23Fixture) {
	t.Helper()
	ops, _ := phase23Operational(t, f)
	token := f.token(t, "retry-tenant", "svc:retry", "phase23-ops", operationalScopes("retry-tenant"), false)
	client := ops.sdk(t, token, false)
	if _, err := client.SetRetentionPolicy(t.Context(), 0, 7, 24); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int64
	interrupted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == phase23Mount+"/v1/retention-sweeps" && attempts.Add(1) == 1 {
			committed := httptest.NewRecorder()
			ops.handler.ServeHTTP(committed, r)
			if committed.Code != 200 {
				t.Error("fault injection did not follow an actual commit", committed.Code)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		ops.handler.ServeHTTP(w, r)
	}))
	t.Cleanup(interrupted.Close)
	retrying, err := cw.New(interrupted.URL+phase23Mount, interrupted.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	result, err := retrying.Invoke(t.Context(), "sweepRetention", cw.CallOptions{IdempotencyKey: "logical-once", Attempts: 3})
	if err != nil || attempts.Load() != 2 {
		t.Fatal("safe post-commit replay", attempts.Load(), err)
	}
	var operation cw.Operation
	if json.Unmarshal(result.Body, &operation) != nil || operation.Status != "succeeded" {
		t.Fatal("missing retained completion receipt")
	}
	replayed, err := client.Sweep(t.Context(), "logical-once")
	if err != nil || replayed.ID != operation.ID {
		t.Fatal("post-commit replay changed operation", err)
	}
	audits, err := client.AuditEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	completed := 0
	for _, event := range audits {
		if event.Action == "retention.sweep" {
			completed++
		}
	}
	if completed != 1 {
		t.Fatal("retries committed repeated erasure", completed)
	}
	var forbiddenCalls atomic.Int64
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == phase23Mount+"/v1/retention-policy" {
			forbiddenCalls.Add(1)
		}
		ops.handler.ServeHTTP(w, r)
	}))
	t.Cleanup(denied.Close)
	bad, err := cw.New(denied.URL+phase23Mount, denied.Client(), func(context.Context) (string, error) { return "invalid", nil })
	if err != nil {
		t.Fatal(err)
	}
	_, err = bad.Invoke(t.Context(), "getRetentionPolicy", cw.CallOptions{Attempts: 3})
	var rejected *cw.StatusError
	if !errors.As(err, &rejected) || rejected.Status != 401 || forbiddenCalls.Load() != 1 {
		t.Fatal("authentication denial was replayed", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = client.Invoke(ctx, "sweepRetention", cw.CallOptions{IdempotencyKey: "canceled", Attempts: 3}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was not preserved", err)
	}

	// A real opaque context expires; its reference does not trigger fresh routing,
	// inference or execution. This is not a fabricated future artifact endpoint.
	var nanos atomic.Int64
	nanos.Store(time.Now().UnixNano())
	clock := func() time.Time { return time.Unix(0, nanos.Load()).UTC() }
	limits := config.DefaultQueryBundles()
	limits.TTL = config.Duration(time.Second)
	timed, _, _ := phase19Service(t, f.domain, limits, clock, f.domain.service, f.reader)
	e, err := f.authority.verifier.Verify(t.Context(), f.httpToken, auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	bundle := phase19Create(t, timed, f.domain, e)
	nanos.Store(bundle.ExpiresAt.UnixNano())
	beforeModel, beforeReads := f.domain.model.requests.Load(), f.reader.calls.Load()
	expiredHandler := nlqapi.BYOHandler(f.authority.verifier, timed, http.NotFoundHandler())
	expired, err := cw.NewInProcess(expiredHandler, func(context.Context) (string, error) { return f.httpToken, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = expired.ReadQueryContext(t.Context(), bundle.Reference); !errors.As(err, &rejected) || rejected.Status != 409 {
		t.Fatal("expired reference did not require explicit replan", err)
	}
	if _, err = expired.SubmitSQL(t.Context(), cw.SQLSubmission{Reference: bundle.Reference, Operation: "expired", SQL: "SELECT id FROM analytics.sales"}); !errors.As(err, &rejected) || rejected.Status != 409 {
		t.Fatal("expired submission revived work", err)
	}
	if f.domain.model.requests.Load() != beforeModel || f.reader.calls.Load() != beforeReads {
		t.Fatal("expiry denial incurred fresh work")
	}
}

func phase23Operational(t *testing.T, f *phase23Fixture) (*phase23Fixture, string) {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	service, err := securityapi.New(db)
	if err != nil {
		t.Fatal(err)
	}
	public, err := foundation.PublicRegistry()
	if err != nil {
		t.Fatal(err)
	}
	security, err := securityapi.APIRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := api.Compose(public, security)
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := telemetry.New(io.Discard, "json", true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := loaded(t, configBytes(t, func(m map[string]any) { m["server"] = map[string]any{"base_path": phase23Mount} }), dsn)
	handler := api.Guard(f.authority.verifier, registry, securityapi.Handler(f.authority.verifier, service, reporter, true))
	outer, err := foundation.NewServerWithRegistry(cfg, reporter, func(ctx context.Context) foundation.Dependency {
		return foundation.Dependency{Ready: db.Check(ctx) == nil}
	}, f.authority.verifier.Check, registry, handler)
	if err != nil {
		t.Fatal(err)
	}
	copy := *f
	copy.handler = outer.Handler()
	copy.security = service
	copy.httpRegistry = registry
	copy.wire = httptest.NewServer(copy.handler)
	t.Cleanup(copy.wire.Close)
	return &copy, dsn
}

func phase23Erasure(t *testing.T, f *phase23Fixture) {
	t.Helper()
	ops, dsn := phase23Operational(t, f)
	tokens := map[string]string{}
	clients := map[string]*cw.Client{}
	for _, tenant := range []string{"erase-a", "erase-b"} {
		token := f.token(t, tenant, "svc:"+tenant, "phase23-erasure", operationalScopes(tenant), false)
		tokens[tenant] = token
		clients[tenant] = ops.sdk(t, token, tenant == "erase-a")
		if _, err := clients[tenant].SetRetentionPolicy(t.Context(), 0, 7, 24); err != nil {
			t.Fatal(err)
		}
	}
	raw := support.Raw(t, dsn)
	for _, tenant := range []string{"erase-a", "erase-b"} {
		if _, err := raw.Exec(t.Context(), `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id,created_at) VALUES($1,repeat('a',32),'synthetic-seed','retention_policy.updated','retention',clock_timestamp()-interval '30 days')`, tenant); err != nil {
			t.Fatal(err)
		}
	}
	beforeA, err := clients["erase-a"].AuditEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	beforeB, err := clients["erase-b"].AuditEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []string{"erase-a", "erase-b"} {
		code, output, _ := ops.cli(t.Context(), tokens[tenant], []string{"diagnostics"}, nil)
		if code != 0 || !json.Valid(output) || strings.Contains(string(output), tokens[tenant]) {
			t.Fatal("scoped read-only diagnostics", code)
		}
	}
	afterA, _ := clients["erase-a"].AuditEvents(t.Context())
	afterB, _ := clients["erase-b"].AuditEvents(t.Context())
	if !reflect.DeepEqual(beforeA, afterA) || !reflect.DeepEqual(beforeB, afterB) {
		t.Fatal("diagnostics changed durable audit state")
	}
	readOnly := f.token(t, "erase-a", "svc:erase-a", "phase23-erasure", []string{"ops.read", "ops.inspect", "cw.tenant.read:erase-a"}, false)
	code, _, _ := ops.cli(t.Context(), readOnly, []string{"call", "sweepRetention", "--execute", "--idempotency-key", "denied"}, nil)
	if code != 3 {
		t.Fatal("explicit CLI erasure manufactured authority", code)
	}
	foreign := f.token(t, "erase-a", "svc:erase-a", "phase23-erasure", operationalScopes("erase-b"), false)
	if _, err := ops.sdk(t, foreign, true).Sweep(t.Context(), "foreign"); err == nil {
		t.Fatal("cross-tenant erasure accepted")
	}
	code, output, diagnostic := ops.cli(t.Context(), tokens["erase-a"], []string{"call", "sweepRetention", "--execute", "--idempotency-key", "erase-once"}, nil)
	if code != 0 {
		t.Fatal("explicit authorized CLI erasure", code, diagnostic)
	}
	var receipt cw.Operation
	if json.Unmarshal(output, &receipt) != nil || receipt.DeletedEvents != 1 || receipt.Status != "succeeded" {
		t.Fatal("actual bounded erasure receipt", receipt)
	}
	afterB, err = clients["erase-b"].AuditEvents(t.Context())
	if err != nil || !reflect.DeepEqual(beforeB, afterB) {
		t.Fatal("erasure affected foreign tenant", err)
	}
	var remaining int
	if err := raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.audit_events WHERE tenant_id='erase-a' AND event_id=repeat('a',32)`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("authorized row was not erased", err)
	}
	replayed, err := clients["erase-a"].Sweep(t.Context(), "erase-once")
	if err != nil || replayed.ID != receipt.ID {
		t.Fatal("CLI/SDK logical erasure key drift", err)
	}
	other, err := clients["erase-b"].Sweep(t.Context(), "erase-once")
	if err != nil || other.ID == receipt.ID || other.DeletedEvents != 1 {
		t.Fatal("logical keys not tenant isolated", err)
	}
}
