package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/topicapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/chartfixtures"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Uses the same real PostgreSQL, native validator/reader, published semantics,
// pgvector and recorded Bifrost provider boundary as phases 15-19. There are no
// success-returning replacements for discovery, planning, execution or feedback.
type phase22Fixture struct {
	domain    *phase17Fixture
	authority *tokenFixture
	server    *mcpserver.Server
	network   *httptest.Server
	bindings  []mcpserver.Binding
	registry  *mcpserver.Registry
	published *topics.Service
	scopes    []string
	bearer    string
}

func newPhase22Fixture(t *testing.T) *phase22Fixture {
	t.Helper()
	f := newPhase18Fixture(t)
	f.model.embeddingMode.Store("fixed")
	f.model.rerankMode.Store("fixed")
	f.model.mode.Store(phase18RawResponse(t, "SELECT id, amount FROM analytics.sales ORDER BY id"))
	query, published := newPhase18Service(t, f)
	byo, _, _ := phase19Service(t, f, config.DefaultQueryBundles(), nil, f.service, nil)
	options := config.DefaultCharts().ServiceOptions()
	options.MaxConcurrent = 64
	chart, err := chartservice.New(options, nil)
	if err != nil {
		t.Fatal(err)
	}
	var bindings []mcpserver.Binding
	for _, build := range []func() ([]mcpserver.Binding, error){
		func() ([]mcpserver.Binding, error) { return sourceapi.MCPBindings(f.f.s) },
		func() ([]mcpserver.Binding, error) { return topicapi.MCPBindings(published) },
		func() ([]mcpserver.Binding, error) { return nlqapi.ExecutionMCPBindings(query) },
		func() ([]mcpserver.Binding, error) { return nlqapi.BYOMCPBindings(byo) },
		func() ([]mcpserver.Binding, error) { return chartapi.MCPBindings(chart) },
	} {
		b, err := build()
		if err != nil {
			t.Fatal("real service binding", err)
		}
		bindings = append(bindings, b...)
	}
	registry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultMCP()
	settings.MaxConcurrent = 64
	server, err := mcpserver.New(f.model.token.verifier, registry, settings, []string{"https://console.example"})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := mcpserver.HTTPRegistry(settings)
	if err != nil {
		t.Fatal(err)
	}
	httpRegistry, err := sourceapi.SourceRegistry(true, false)
	if err != nil {
		t.Fatal(err)
	}
	topicRegistry, err := topicapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	combined, err := api.Compose(transport, httpRegistry, topicRegistry)
	if err != nil {
		t.Fatal(err)
	}
	metadata := sourceapi.Handler(f.model.token.verifier, f.f.s, f.f.validator, topicapi.Handler(f.model.token.verifier, nil, published, nil, http.NotFoundHandler()))
	h := api.Guard(f.model.token.verifier, combined, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mcpserver.Path {
			server.Handler().ServeHTTP(w, r)
			return
		}
		metadata.ServeHTTP(w, r)
	}))
	network := httptest.NewServer(h)
	t.Cleanup(network.Close)
	set := map[string]bool{}
	for _, scope := range append(append(phase18Scopes(f.e.Tenant(), true), phase19Scopes()...), "mcp.use", "charts.read", "charts.select", "charts.bind", "cw.tenant.read:"+f.e.Tenant()) {
		set[scope] = true
	}
	scopes := make([]string, 0, len(set))
	for scope := range set {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	result := &phase22Fixture{f, f.model.token, server, network, bindings, registry, published, scopes, ""}
	result.bearer = result.token(t, f.e.Tenant(), f.e.User(), "phase22-session", scopes, true)
	return result
}
func (f *phase22Fixture) token(t testing.TB, tenant, user, session string, scopes []string, mcpAudience bool) string {
	t.Helper()
	claims := f.authority.claims(tenant, user, scopes)
	claims["session"] = session
	if mcpAudience {
		claims["aud"] = f.authority.cfg.MCPAudience()
	}
	return f.authority.sign(t, claims, nil)
}
func (f *phase22Fixture) client(t testing.TB, token string) *cw.Client {
	t.Helper()
	c, err := cw.New(f.network.URL, f.network.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func phase22RPC(t testing.TB, c *cw.Client, method string, params any) json.RawMessage {
	t.Helper()
	request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.MCP(context.Background(), request)
	if err != nil {
		t.Fatal("MCP transport", err)
	}
	var wire struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err = json.Unmarshal(raw, &wire); err != nil || wire.Error != nil || wire.Result == nil {
		t.Fatalf("protocol response %s: %v", raw, err)
	}
	return wire.Result
}

type phase22ToolResult struct {
	IsError    bool            `json:"isError"`
	Structured json.RawMessage `json:"structuredContent"`
}

func phase22RawTool(t testing.TB, c *cw.Client, name string, input any) phase22ToolResult {
	t.Helper()
	raw := phase22RPC(t, c, "tools/call", map[string]any{"name": name, "arguments": input})
	var out phase22ToolResult
	if err := json.Unmarshal(raw, &out); err != nil || out.Structured == nil {
		t.Fatalf("tool wire response %s: %v", raw, err)
	}
	return out
}
func phase22Call[Out any](t testing.TB, c *cw.Client, name string, input any) Out {
	t.Helper()
	wire := phase22RawTool(t, c, name, input)
	if wire.IsError {
		t.Fatalf("%s failed: %s", name, wire.Structured)
	}
	var result struct {
		Result Out `json:"result"`
	}
	if err := json.Unmarshal(wire.Structured, &result); err != nil {
		t.Fatal(err)
	}
	return result.Result
}
func phase22Fault(t testing.TB, out phase22ToolResult) mcpserver.Fault {
	t.Helper()
	if !out.IsError {
		t.Fatal("operation unexpectedly succeeded", string(out.Structured))
	}
	var wire struct {
		Error mcpserver.Fault `json:"error"`
	}
	if err := json.Unmarshal(out.Structured, &wire); err != nil || wire.Error.Code == "" {
		t.Fatal("untyped failure", err, string(out.Structured))
	}
	return wire.Error
}
func phase22Without(scopes []string, prefix string) []string {
	var out []string
	for _, s := range scopes {
		if !strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

func TestPhase22(t *testing.T) {
	f := newPhase22Fixture(t)
	client := f.client(t, f.bearer)
	ref := f.domain.pack.Datasets[0].Source
	dataset := sources.DatasetDescribeRequest{Source: ref.Source, Context: ref.Context, Dataset: ref.Dataset}
	t.Run("AC01", func(t *testing.T) {
		beforeModel, beforeSource := f.domain.model.requests.Load(), f.domain.f.lookups.Load()
		bare := f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase22-session", []string{"mcp.use"}, true)
		for _, tool := range f.registry.Manifest() {
			out := phase22RawTool(t, f.client(t, bare), tool.Name, map[string]any{})
			if fault := phase22Fault(t, out); fault.Code != "forbidden" || fault.Outcome != "not_started" {
				t.Fatalf("%s bypassed common action gate: %+v", tool.Name, fault)
			}
		}
		for _, token := range []string{"invalid", f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase22-session", f.scopes, false), f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase22-session", phase22Without(f.scopes, "mcp.use"), true)} {
			_, err := f.client(t, token).MCP(t.Context(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
			var status *cw.StatusError
			if !errors.As(err, &status) || (status.Status != 401 && status.Status != 403) {
				t.Fatal("transport authority bypass", err)
			}
		}
		crossTenant := f.token(t, "other-tenant", f.domain.e.User(), "phase22-session", f.scopes, true)
		if code := phase22Fault(t, phase22RawTool(t, f.client(t, crossTenant), "describe_dataset", dataset)).Code; code != "not_found" {
			t.Fatal("cross-tenant metadata", code)
		}
		restricted := phase22Without(f.scopes, "cw.execution_context.use:")
		restricted = append(restricted, "cw.execution_context.use:other-context")
		token := f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase22-session", restricted, true)
		if code := phase22Fault(t, phase22RawTool(t, f.client(t, token), "describe_dataset", dataset)).Code; code != "not_found" {
			t.Fatal("same-tenant context bypass", code)
		}
		for _, uri := range []string{"chartworks://topics/foreign", "chartworks://datasets/" + ref.Source + "/other-context/" + ref.Dataset, "chartworks://charts/catalog?token=secret", "https://private.example/data"} {
			inprocess, err := f.server.Client(func(context.Context) (string, error) { return token, nil })
			if err != nil {
				t.Fatal(err)
			}
			if _, err = inprocess.ReadResource(t.Context(), uri); err == nil {
				t.Fatal("resource bypass", uri)
			}
		}
		if f.domain.model.requests.Load() != beforeModel || f.domain.f.lookups.Load() != beforeSource {
			t.Fatal("denied metadata/tools performed model or warehouse credential work")
		}
	})
	t.Run("AC02", func(t *testing.T) {
		list := phase22Call[[]topics.Summary](t, client, "list_topics", topics.ListRequest{Limit: 100})
		if len(list) != 2 {
			t.Fatal("real publication discovery", list)
		}
		published := phase22Call[topics.Published](t, client, "describe_topic", topicapi.PublishedTopicRequest{Topic: f.domain.pack.Topic})
		if published.Definition.Topic != f.domain.pack.Topic {
			t.Fatal("wrong publication", published)
		}
		datasets := phase22Call[[]sources.Dataset](t, client, "list_datasets", sources.DatasetListRequest{Source: ref.Source, Context: ref.Context, Limit: 32})
		if len(datasets) == 0 {
			t.Fatal("no real retained datasets")
		}
		got := phase22Call[sources.Dataset](t, client, "describe_dataset", dataset)
		if got.Relation.ID != ref.Dataset || len(got.Relation.Columns) == 0 {
			t.Fatal("wrong dataset", got)
		}
		question := phase18Question(f.domain, nlq.LanguageEnglish, f.domain.pack.Topic)
		preflight := phase22Call[nlqexec.PreflightResult](t, client, "preflight_question", nlqexec.PreflightRequest{QuestionRequest: question})
		if preflight.QueryID == "" || preflight.Route.Context == nil {
			t.Fatal("preflight did not persist real routing", preflight)
		}
		plan := phase22Call[nlqexec.PlanResult](t, client, "plan_question", nlqexec.PlanRequest{QuestionRequest: question, Operation: "phase22-run"})
		if plan.Status != "planned" || plan.QueryID == "" {
			t.Fatal("no validated plan", plan)
		}
		run := phase22Call[nlqexec.RunResult](t, client, "run_question", nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "phase22-run", Rows: 10, Bytes: 4096})
		if run.Status != "succeeded" || run.Execution.Result == nil || len(run.Execution.Result.Rows) != 2 {
			t.Fatal("real read did not execute", run)
		}
		refined := phase22Call[nlqexec.PlanResult](t, client, "refine_question", nlqexec.RefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Show revenue by id", Kinds: question.Kinds, LimitPerKind: 1}})
		if refined.QueryID == "" || refined.QueryID == plan.QueryID {
			t.Fatal("refinement did not create real child plan")
		}
		feedback := phase22Call[nlqapi.FeedbackResult](t, client, "submit_feedback", nlqexec.FeedbackRequest{QueryID: plan.QueryID, Verdict: "positive", Note: "Synthetic MCP review"})
		if !feedback.Accepted {
			t.Fatal("feedback not persisted")
		}
		contextResult := phase22Call[nlqbyo.CreateResult](t, client, "get_query_context", nlqbyo.CreateRequest{SchemaVersion: 1, Route: nlqroute.RouteRequest{Topic: f.domain.pack.Topic, Context: ref.Context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 1}})
		if contextResult.Bundle == nil {
			t.Fatal("no exact BYO context", contextResult)
		}
		submission := nlqbyo.SubmitRequest{Reference: contextResult.Bundle.Reference, Operation: "phase22-step", SQL: "SELECT id, amount FROM analytics.sales ORDER BY id", Parameters: []readexec.Parameter{}}
		step := phase22Call[nlqbyo.SubmitResult](t, client, "submit_sql", submission)
		if !step.ValuesAvailable || step.Result == nil || len(step.Result.Rows) != 2 {
			t.Fatal("real BYO step missing values", step)
		}
		replay := phase22Call[nlqbyo.SubmitResult](t, client, "submit_sql", submission)
		if !replay.Replayed || replay.ValuesAvailable || replay.Result != nil {
			t.Fatal("retry reran/reconstructed values", replay)
		}
		view := phase22Call[nlqbyo.View](t, client, "read_query_context", contextResult.Bundle.Reference)
		if len(view.Steps) != 1 || view.Steps[0].Operation != "phase22-step" {
			t.Fatal("missing real receipt", view)
		}
		otherSession := f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "different-session", f.scopes, true)
		if !phase22RawTool(t, f.client(t, otherSession), "run_question", nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "phase22-other"}).IsError {
			t.Fatal("session confused with stateless transport")
		}
		if !phase22RawTool(t, f.client(t, otherSession), "read_query_context", contextResult.Bundle.Reference).IsError {
			t.Fatal("BYO reference became authority")
		}
	})
	t.Run("AC03", func(t *testing.T) {
		inventory := f.registry.Manifest()
		if len(inventory) != 18 {
			t.Fatal("missing concrete bindings", len(inventory))
		}
		expected := []string{"list_topics", "describe_topic", "list_datasets", "describe_dataset", "preflight_question", "plan_question", "run_question", "refine_question", "get_query_context", "submit_sql", "submit_feedback"}
		seen := map[string]bool{}
		httpRegistry := phase21Registry(t)
		definitions := map[string]api.Definition{}
		for _, d := range httpRegistry.Definitions() {
			definitions[d.ID] = d
		}
		for _, tool := range inventory {
			seen[tool.Name] = true
			id, _ := tool.Meta["chartworks/operation"].(string)
			owner, ok := definitions[id]
			if !ok || owner.Action != tool.Meta["chartworks/action"] || owner.Effect != tool.Meta["chartworks/effect"] || owner.Audit != tool.Meta["chartworks/audit"] {
				t.Fatalf("MCP/HTTP registry drift: %s", tool.Name)
			}
			if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || tool.Annotations.OpenWorldHint == nil || tool.InputSchema == nil || tool.OutputSchema == nil {
				t.Fatal("unknown annotation/schema", tool.Name)
			}
			paid, _ := tool.Meta["chartworks/maySpend"].(bool)
			persists, _ := tool.Meta["chartworks/persists"].(bool)
			if (paid || persists) && tool.Annotations.ReadOnlyHint {
				t.Fatal("paid/persisted operation advertised as pure", tool.Name)
			}
			if strings.Contains(tool.Name, "question") || tool.Name == "submit_sql" || tool.Name == "submit_feedback" || tool.Name == "get_query_context" {
				if !persists {
					t.Fatal("persistence omitted", tool.Name)
				}
			}
			if tool.Name == "preflight_question" && (!paid || tool.Annotations.IdempotentHint) {
				t.Fatal("preflight side effects understated")
			}
		}
		for _, name := range expected {
			if !seen[name] {
				t.Fatal("missing core contract", name)
			}
		}
		for _, build := range []func() ([]mcpserver.Binding, error){func() ([]mcpserver.Binding, error) { return sourceapi.MCPBindings(nil) }, func() ([]mcpserver.Binding, error) { return topicapi.MCPBindings(nil) }, func() ([]mcpserver.Binding, error) { return nlqapi.ExecutionMCPBindings(nil) }, func() ([]mcpserver.Binding, error) { return nlqapi.BYOMCPBindings(nil) }, func() ([]mcpserver.Binding, error) { return chartapi.MCPBindings(nil) }} {
			b, err := build()
			if err != nil || len(b) != 0 {
				t.Fatal("uninstalled service advertised", err)
			}
		}
		selected, err := mcpserver.SelectGroups(f.bindings, []string{"charts"})
		if err != nil || len(selected) != 5 {
			t.Fatal("disabled groups retained", err)
		}
		if _, err = mcpserver.SelectGroups(f.bindings, []string{"reporting"}); err == nil {
			t.Fatal("unbuilt group accepted")
		}
	})
	t.Run("AC04", func(t *testing.T) {
		before := f.domain.model.requests.Load()
		for _, input := range []any{map[string]any{"topic": f.domain.pack.Topic, "tenant": "foreign"}, map[string]any{"topic": nil}} {
			out := phase22RawTool(t, client, "describe_topic", input)
			if phase22Fault(t, out).Code != "invalid_request" {
				t.Fatal("open identity schema")
			}
		}
		malformed := []string{`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"describe_topic","arguments":{"topic":"one","topic":"two"}}}`, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`}
		for _, raw := range malformed {
			// Send malformed envelopes to the actual server, not the SDK's earlier
			// local JSON check. A locally rejected request has no HTTP status.
			response := callProtected(t, f.network.Config.Handler, "POST", "http://127.0.0.1"+mcpserver.Path, f.bearer, raw, map[string]string{"Accept": "application/json, text/event-stream"})
			if response.Code != http.StatusBadRequest {
				t.Fatal("ambiguous protocol accepted", response.Code, response.Body.String())
			}
		}
		for _, uri := range []string{"chartworks://topics/missing-private-topic", "chartworks://topics/%2e%2e", "chartworks://topics/missing?token=private-secret"} {
			request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": map[string]any{"uri": uri}})
			raw, err := client.MCP(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"error"`) || strings.Contains(string(raw), "private") || strings.Contains(string(raw), "stack") {
				t.Fatal("unsafe resource diagnostic", string(raw))
			}
		}
		if f.domain.model.requests.Load() != before {
			t.Fatal("malformed requests incurred model work")
		}
		// Empty HTTP GET is an honest 405, not an advertised SSE implementation.
		req, _ := http.NewRequestWithContext(t.Context(), "GET", f.network.URL+mcpserver.Path, nil)
		req.Header.Set("Authorization", "Bearer "+f.bearer)
		response, err := f.network.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != 405 {
			t.Fatal("unexpected stateful stream", response.StatusCode)
		}
	})
	t.Run("AC05", func(t *testing.T) {
		// Shared clients fetch one token from each exact call context. Neither a
		// process-global bearer nor an initialization/session token is authoritative.
		type bearerKey struct{}
		tokens := func(ctx context.Context) (string, error) {
			token, _ := ctx.Value(bearerKey{}).(string)
			return token, nil
		}
		local, err := f.server.Client(tokens)
		if err != nil {
			t.Fatal(err)
		}
		remote, err := cw.New(f.network.URL, f.network.Client(), tokens)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := range 12 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tenant := fmt.Sprintf("mcp-tenant-%d", i)
				token := f.token(t, tenant, fmt.Sprintf("actor-%d", i), fmt.Sprintf("session-%d", i), []string{"mcp.use", "charts.read", "cw.tenant.read:" + tenant}, true)
				ctx := context.WithValue(t.Context(), bearerKey{}, token)
				for range 3 {
					out, err := local.CallTool(ctx, "chart_catalog", json.RawMessage(`{}`))
					if err != nil || out.IsError {
						t.Error("per-request in-process authority", err)
						return
					}
					request := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"chart_catalog","arguments":{}}}`)
					raw, err := remote.MCP(ctx, request)
					if err != nil || strings.Contains(string(raw), `"isError":true`) {
						t.Error("per-request transport authority", err, string(raw))
						return
					}
				}
			}()
		}
		wg.Wait()
		token := f.token(t, "wrong-tenant", "reader", "other", []string{"mcp.use", "charts.read", "cw.tenant.read:" + f.domain.e.Tenant()}, true)
		result, err := local.CallTool(context.WithValue(t.Context(), bearerKey{}, token), "chart_catalog", json.RawMessage(`{}`))
		if err != nil || !result.IsError {
			t.Fatal("previous caller authority reused", err)
		}
		if _, err = local.ListTools(t.Context()); err == nil {
			t.Fatal("missing request token reused previous caller")
		}
	})
	t.Run("AC06", func(t *testing.T) {
		beforeModel, beforeSource := f.domain.model.requests.Load(), f.domain.f.lookups.Load()
		sourceList := phase22Call[[]sources.Source](t, client, "list_sources", struct{}{})
		if len(sourceList) == 0 {
			t.Fatal("missing source metadata")
		}
		catalog := phase22Call[chartservice.CatalogResult](t, client, "chart_catalog", struct{}{})
		if len(catalog.Kinds) != 14 {
			t.Fatal("catalog spec count")
		}
		data, bindings, order := chartfixtures.Fixture(charts.Table, "order")
		selected := phase22Call[chartservice.SelectionResult](t, client, "select_chart", chartservice.SelectRequest{Data: data})
		if len(selected.Provenance.Receipt.Calls) != 0 {
			t.Fatal("deterministic selection incurred cost")
		}
		specified := phase22Call[chartservice.BuildResult](t, client, "specify_chart", chartservice.SpecifyRequest{Data: data, Kind: charts.Table, Bindings: bindings, Order: order, Options: charts.DefaultOptions()})
		built := phase22Call[chartservice.BuildResult](t, client, "build_chart", chartservice.BuildRequest{Data: data, Mapping: specified.Output.Mapping})
		if !reflect.DeepEqual(built.Output, specified.Output) {
			t.Fatal("saved mapping parity")
		}
		proposal := phase22Call[charts.Proposal](t, client, "rebind_chart", chartservice.BuildRequest{Data: data, Mapping: specified.Output.Mapping})
		if proposal.Status != "review_required" {
			t.Fatal("rebind silently approved")
		}
		local, err := f.server.Client(func(context.Context) (string, error) { return f.bearer, nil })
		if err != nil {
			t.Fatal(err)
		}
		list, err := local.ListTools(t.Context())
		if err != nil || len(list.Tools) != 18 {
			t.Fatal("in-process discovery", err)
		}
		resources, err := local.ListResources(t.Context())
		if err != nil || len(resources.Resources) != 1 {
			t.Fatal("exact resources", err)
		}
		templates, err := local.ListResourceTemplates(t.Context())
		if err != nil || len(templates.ResourceTemplates) != 2 {
			t.Fatal("metadata templates", err)
		}
		for _, uri := range []string{"chartworks://charts/catalog", "chartworks://topics/" + f.domain.pack.Topic, "chartworks://datasets/" + ref.Source + "/" + ref.Context + "/" + ref.Dataset} {
			resource, err := local.ReadResource(t.Context(), uri)
			if err != nil || len(resource.Contents) != 1 || resource.Contents[0].MIMEType != "application/json" {
				t.Fatal("real resource read", uri, err)
			}
			var remoteResource mcp.ReadResourceResult
			if err = json.Unmarshal(phase22RPC(t, client, "resources/read", map[string]any{"uri": uri}), &remoteResource); err != nil || !reflect.DeepEqual(resource.Contents, remoteResource.Contents) {
				t.Fatal("HTTP/in-process resource parity", uri, err)
			}
		}
		// Prove filtering happens before LIMIT, not after a broad page is selected.
		narrow := phase22Without(f.scopes, "cw.topic.read:")
		narrow = append(narrow, "cw.topic.read:"+f.domain.related.Topic)
		narrowToken := f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase22-session", narrow, true)
		page := phase22Call[[]topics.Summary](t, f.client(t, narrowToken), "list_topics", topics.ListRequest{Limit: 1})
		if len(page) != 1 || page[0].Topic != f.domain.related.Topic {
			t.Fatal("topic authorization occurred after pagination", page)
		}
		all := phase22Call[[]sources.Dataset](t, client, "list_datasets", sources.DatasetListRequest{Source: ref.Source, Context: ref.Context, Limit: 32})
		last := all[len(all)-1].Relation.ID
		narrow = phase22Without(f.scopes, "cw.dataset.query:")
		narrow = append(narrow, "cw.dataset.query:"+last)
		narrowToken = f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase22-session", narrow, true)
		datasets := phase22Call[[]sources.Dataset](t, f.client(t, narrowToken), "list_datasets", sources.DatasetListRequest{Source: ref.Source, Context: ref.Context, Limit: 1})
		if len(datasets) != 1 || datasets[0].Relation.ID != last {
			t.Fatal("dataset authorization occurred after pagination", datasets)
		}
		after := phase22Call[[]sources.Dataset](t, client, "list_datasets", sources.DatasetListRequest{Source: ref.Source, Context: ref.Context, After: last, Limit: 32})
		if len(after) != 0 {
			t.Fatal("keyset page repeated values")
		}
		// New phase-21 HTTP and Go methods use the same real catalog services.
		httpToken := f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase22-session", f.scopes, false)
		httpClient := f.client(t, httpToken)
		httpTopics, err := httpClient.ListTopics(t.Context(), cw.TopicListRequest{Limit: 100})
		if err != nil || len(httpTopics) != 2 {
			t.Fatal("topic HTTP/SDK", err)
		}
		httpDatasets, err := httpClient.ListDatasets(t.Context(), cw.DatasetListRequest{Source: ref.Source, Context: ref.Context, Limit: 32})
		if err != nil || !reflect.DeepEqual(httpDatasets, all) {
			t.Fatal("dataset HTTP/SDK", err)
		}
		described, err := httpClient.DescribeDataset(t.Context(), dataset)
		if err != nil || described.Relation.ID != ref.Dataset {
			t.Fatal("describe HTTP/SDK", err)
		}
		if f.domain.model.requests.Load() != beforeModel || f.domain.f.lookups.Load() != beforeSource {
			t.Fatal("metadata/resources/caller-data transforms performed model or warehouse work")
		}
		// Every output contract advertised by a registered tool is executable JSON schema.
		for _, tool := range f.registry.Manifest() {
			raw, err := json.Marshal(tool.OutputSchema)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = gateway.NewSchema(tool.Name+"_contract", raw); err != nil {
				t.Fatal("invalid output contract", tool.Name, err)
			}
		}
	})
}
