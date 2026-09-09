package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/foundation"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/securityapi"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/telemetry"
	"github.com/hurtener/chartworks/internal/topicapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

const phase23Mount = "/sdk-parity"

// The HTTP and MCP services deliberately have separate instances, as deployed
// clients may contact different processes. They share the real PostgreSQL store,
// source, native validator/executor, published semantics and provider fixture.
// No successful business outcome is replaced by a transport mock.
type phase23Fixture struct {
	*phase22Fixture
	handler      http.Handler
	wire         *httptest.Server
	httpRegistry *api.Registry
	query        *nlqexec.Service
	byo          *nlqbyo.Service
	security     *securityapi.Service
	reader       *phase19CountingExecutor
	httpToken    string
}

func newPhase23Fixture(t *testing.T) *phase23Fixture {
	t.Helper()
	f := newPhase22Fixture(t)
	query, _ := newPhase18Service(t, f.domain)
	reader := &phase19CountingExecutor{delegate: f.domain.f.executor}
	byo, _, _ := phase19Service(t, f.domain, config.DefaultQueryBundles(), nil, f.domain.service, reader)
	security, err := securityapi.New(f.domain.f.db)
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := telemetry.New(io.Discard, "json", true)
	if err != nil {
		t.Fatal(err)
	}
	options := config.DefaultCharts().ServiceOptions()
	options.MaxConcurrent = 64
	chart, err := chartservice.New(options, nil)
	if err != nil {
		t.Fatal(err)
	}
	var registries []*api.Registry
	for _, build := range []func() (*api.Registry, error){
		foundation.PublicRegistry,
		func() (*api.Registry, error) { return securityapi.APIRegistry(true) },
		func() (*api.Registry, error) { return sourceapi.SourceRegistry(true, false) },
		topicapi.Registry, nlqapi.ExecutionRegistry,
		func() (*api.Registry, error) { return nlqapi.BYORegistry(true) },
		chartapi.Registry,
		func() (*api.Registry, error) { return mcpserver.HTTPRegistry(config.DefaultMCP()) },
	} {
		r, err := build()
		if err != nil {
			t.Fatal(err)
		}
		registries = append(registries, r)
	}
	registry, err := api.Compose(registries...)
	if err != nil {
		t.Fatal(err)
	}
	var protected http.Handler = securityapi.Handler(f.authority.verifier, security, reporter, true)
	protected = chartapi.Handler(f.authority.verifier, chart, protected)
	protected = nlqapi.BYOHandler(f.authority.verifier, byo, protected)
	protected = nlqapi.ExecutionHandler(f.authority.verifier, query, protected)
	protected = topicapi.Handler(f.authority.verifier, nil, f.published, nil, protected)
	protected = sourceapi.Handler(f.authority.verifier, f.domain.f.s, f.domain.f.validator, protected)
	domains := protected
	protected = api.Guard(f.authority.verifier, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mcpserver.Path {
			f.server.Handler().ServeHTTP(w, r)
			return
		}
		domains.ServeHTTP(w, r)
	}))
	protected = assertRegisteredWireSchemas(t, registry, protected)
	cfg := loaded(t, configBytes(t, func(m map[string]any) {
		m["server"] = map[string]any{"base_path": phase23Mount}
	}), support.Database(t))
	outer, err := foundation.NewServerWithRegistry(cfg, reporter,
		func(ctx context.Context) foundation.Dependency {
			return foundation.Dependency{Ready: f.domain.f.db.Check(ctx) == nil}
		},
		f.authority.verifier.Check, registry, protected)
	if err != nil {
		t.Fatal(err)
	}
	handler := outer.Handler()
	wire := httptest.NewServer(handler)
	t.Cleanup(wire.Close)
	return &phase23Fixture{phase22Fixture: f, handler: handler, wire: wire, httpRegistry: registry, query: query, byo: byo, security: security, reader: reader, httpToken: f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase23-session", f.scopes, false)}
}
func (f *phase23Fixture) sdk(t testing.TB, token string, inProcess bool) *cw.Client {
	t.Helper()
	provider := func(context.Context) (string, error) { return token, nil }
	var client *cw.Client
	var err error
	if inProcess {
		client, err = cw.NewInProcessWithOptions(f.handler, provider, cw.InProcessOptions{BasePath: phase23Mount})
	} else {
		client, err = cw.New(f.wire.URL+phase23Mount, f.wire.Client(), provider)
	}
	if err != nil {
		t.Fatal(err)
	}
	return client
}
func (f *phase23Fixture) cli(ctx context.Context, token string, args []string, input []byte) (int, []byte, string) {
	var stdout, stderr bytes.Buffer
	lookup := func(key string) (string, bool) {
		switch key {
		case "CHARTWORKS_CLIENT_URL":
			return f.wire.URL + phase23Mount, true
		case "CHARTWORKS_TOKEN":
			return token, true
		}
		return "", false
	}
	code := foundation.CommandWithInput(ctx, append([]string{"client"}, args...), lookup, bytes.NewReader(input), &stdout, &stderr, foundation.Build{}, func(context.Context, config.Config, io.Writer) error {
		return errors.New("client unexpectedly started a service")
	})
	return code, stdout.Bytes(), stderr.String()
}

type phase23Surface struct {
	name string
	call func(context.Context, string, cw.CallOptions, any) (json.RawMessage, error)
}

func phase23Encode[Input, Output any](ctx context.Context, raw []byte, method func(context.Context, Input) (Output, error)) (json.RawMessage, error) {
	var input Input
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	output, err := method(ctx, input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(output)
}
func phase23Typed(ctx context.Context, client *cw.Client, id string, options cw.CallOptions) (json.RawMessage, error) {
	switch id {
	case "listTopics":
		return phase23Encode(ctx, options.Body, client.ListTopics)
	case "listDatasets":
		return phase23Encode(ctx, options.Body, client.ListDatasets)
	case "describeDataset":
		return phase23Encode(ctx, options.Body, client.DescribeDataset)
	case "getPublishedTopic":
		out, err := client.PublishedTopic(ctx, options.ResourceID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	case "preflightNLQ":
		return phase23Encode(ctx, options.Body, client.PreflightNLQ)
	case "planNLQ":
		return phase23Encode(ctx, options.Body, client.PlanNLQ)
	case "runNLQ":
		return phase23Encode(ctx, options.Body, client.RunNLQ)
	case "refineNLQ":
		return phase23Encode(ctx, options.Body, client.RefineNLQ)
	case "feedbackNLQ":
		return phase23Encode(ctx, options.Body, client.FeedbackNLQ)
	case "getQueryContext":
		return phase23Encode(ctx, options.Body, client.GetQueryContext)
	case "readQueryContext":
		return phase23Encode(ctx, options.Body, client.ReadQueryContext)
	case "submitSQL":
		return phase23Encode(ctx, options.Body, client.SubmitSQL)
	}
	return nil, errors.New("missing typed acceptance adapter")
}
func (f *phase23Fixture) surfaces(t *testing.T) []phase23Surface {
	t.Helper()
	mcpToken := f.token(t, f.domain.e.Tenant(), f.domain.e.User(), "phase23-session", f.scopes, true)
	network, local := f.sdk(t, f.httpToken, false), f.sdk(t, f.httpToken, true)
	mcpNetwork := f.sdk(t, mcpToken, false)
	mcpLocal, err := f.server.Client(func(context.Context) (string, error) { return mcpToken, nil })
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := network.OperationMatrix(t.Context(), mcpNetwork)
	if err != nil {
		t.Fatal("live operation matrix", err)
	}
	rows := map[string]cw.OperationInfo{}
	for _, row := range matrix {
		rows[row.ID] = row
	}
	rawHTTP := func(ctx context.Context, id string, options cw.CallOptions, _ any) (json.RawMessage, error) {
		row := rows[id]
		if row.ID == "" {
			return nil, cw.ErrUnknownOperation
		}
		path := strings.Replace(row.Path, "{id}", options.ResourceID, 1)
		request, err := http.NewRequestWithContext(ctx, row.Method, f.wire.URL+phase23Mount+path, bytes.NewReader(options.Body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+f.httpToken)
		if options.Body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := f.wire.Client().Do(request)
		if err != nil {
			return nil, err
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != 200 {
			return nil, &cw.StatusError{Status: response.StatusCode}
		}
		return io.ReadAll(io.LimitReader(response.Body, 32<<20))
	}
	unpack := func(structured []byte, failed bool) (json.RawMessage, error) {
		if failed {
			return nil, errors.New("MCP owner rejected acceptance request: " + string(structured))
		}
		var result struct {
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(structured, &result); err != nil {
			return nil, err
		}
		if result.Result == nil {
			return nil, errors.New("missing MCP core result")
		}
		return result.Result, nil
	}
	out := []phase23Surface{{name: "http", call: rawHTTP}}
	for _, entry := range []struct {
		name   string
		client *cw.Client
	}{{"sdk-http", network}, {"sdk-local", local}} {
		out = append(out, phase23Surface{name: entry.name, call: func(ctx context.Context, id string, options cw.CallOptions, _ any) (json.RawMessage, error) {
			return phase23Typed(ctx, entry.client, id, options)
		}})
		out = append(out, phase23Surface{name: entry.name + "-generic", call: func(ctx context.Context, id string, options cw.CallOptions, _ any) (json.RawMessage, error) {
			result, err := entry.client.Invoke(ctx, id, options)
			return result.Body, err
		}})
	}
	out = append(out, phase23Surface{name: "mcp-http", call: func(ctx context.Context, id string, _ cw.CallOptions, args any) (json.RawMessage, error) {
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": rows[id].MCPTool, "arguments": args}})
		if err != nil {
			return nil, err
		}
		raw, err := mcpNetwork.MCP(ctx, body)
		if err != nil {
			return nil, err
		}
		var rpc struct {
			Result phase22ToolResult `json:"result"`
			Error  json.RawMessage   `json:"error"`
		}
		if json.Unmarshal(raw, &rpc) != nil || rpc.Error != nil {
			return nil, errors.New("MCP protocol rejected acceptance request")
		}
		return unpack(rpc.Result.Structured, rpc.Result.IsError)
	}}, phase23Surface{name: "mcp-local", call: func(ctx context.Context, id string, _ cw.CallOptions, args any) (json.RawMessage, error) {
		body, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		result, err := mcpLocal.CallTool(ctx, rows[id].MCPTool, body)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return nil, err
		}
		return unpack(raw, result.IsError)
	}}, phase23Surface{name: "cli", call: func(ctx context.Context, id string, options cw.CallOptions, _ any) (json.RawMessage, error) {
		args := []string{"call", id, "--execute"}
		if options.ResourceID != "" {
			args = append(args, "--id", options.ResourceID)
		}
		if options.Body != nil {
			args = append(args, "--input", "-")
		}
		code, body, diagnostic := f.cli(ctx, f.httpToken, args, options.Body)
		if code != 0 {
			return nil, errors.New("CLI acceptance failed: " + diagnostic)
		}
		return body, nil
	}})
	return out
}
func phase23Call[Output any](t *testing.T, surface phase23Surface, id, resource string, input, mcpInput any) Output {
	t.Helper()
	var raw []byte
	var err error
	if input != nil {
		raw, err = json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
	}
	if mcpInput == nil {
		mcpInput = input
	}
	body, err := surface.call(t.Context(), id, cw.CallOptions{ResourceID: resource, Body: raw}, mcpInput)
	if err != nil {
		t.Fatalf("%s/%s: %v", surface.name, id, err)
	}
	var output Output
	if err = json.Unmarshal(body, &output); err != nil {
		t.Fatal("core output decode", err)
	}
	return output
}
