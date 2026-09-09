package clientcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/securityapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

type cliFixture struct {
	server          *httptest.Server
	env             map[string]string
	out, diagnostic bytes.Buffer
	calls, metadata atomic.Int64
	status          atomic.Int64
	mcp             atomic.Value
	tokenReads      int
}

// CLI transport fixtures use actual registered schemas. TestPhase23 separately
// exercises the real verified-envelope services and PostgreSQL effects.
func newCLIFixture(t *testing.T) *cliFixture {
	t.Helper()
	f := &cliFixture{env: map[string]string{"CHARTWORKS_TOKEN": "synthetic-client", "MCP_TOKEN": "synthetic-mcp"}}
	f.mcp.Store(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
	security, err := securityapi.APIRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := gateway.NewSchema("cliDocument", []byte(`{"type":"object","additionalProperties":true}`))
	if err != nil {
		t.Fatal(err)
	}
	document := api.Definition{Operation: api.Operation{Method: "GET", Path: "/openapi.json", Effect: "public_read"}, ID: "openapi", Summary: "Generated CLI fixture inventory", ResourceLoader: "unit.cli", Audit: "read_only", Public: true, Response: schema, Errors: []api.ErrorResponse{{Status: 503, Code: "unavailable"}}}
	mcp := api.Definition{Operation: api.Operation{Method: "POST", Path: "/v1/mcp", Action: "mcp.use", Effect: "protocol_dispatch"}, Surface: auth.MCP, ID: "mcpTransport", Summary: "MCP CLI transport fixture", ResourceLoader: "unit.cli", Audit: "owner_dispatch", Request: schema, Response: schema, MaxBodyBytes: 10 << 20, EmptySuccess: []int{202}, Errors: []api.ErrorResponse{{Status: 401, Code: "unauthenticated"}, {Status: 403, Code: "forbidden"}}}
	registry, err := api.New(append(security.Definitions(), document, mcp))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := registry.OpenAPI("Synthetic CLI fixture", "1")
	if err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/openapi.json" {
			f.metadata.Add(1)
			_, _ = w.Write(spec)
			return
		}
		f.calls.Add(1)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer synthetic") {
			t.Error("caller authority missing")
		}
		if status := f.status.Load(); status != 0 {
			w.WriteHeader(int(status))
			if status != 202 {
				_, _ = io.WriteString(w, "PRIVATE_DEPENDENCY_ERROR")
			}
			return
		}
		switch r.URL.Path {
		case "/v1/retention-policy":
			_, _ = io.WriteString(w, `{"revision":1,"audit_days":30,"operation_hours":24}`)
		case "/v1/audit-events":
			_, _ = io.WriteString(w, `[]`)
		case "/v1/access/diagnostics":
			_, _ = io.WriteString(w, `[{"operation":{"method":"GET","path":"/v1/retention-policy","action":"ops.read","permission":"read","mutation":false},"allowed":true}]`)
		case "/v1/retention-sweeps":
			if r.Header.Get("Idempotency-Key") == "" {
				t.Error("logical erasure key lost")
			}
			_, _ = io.WriteString(w, `{"id":"synthetic-sweep","status":"succeeded","policy_revision":1,"cutoff":"2026-09-09T00:00:00Z","limit":1000,"deleted_events":0,"deleted_operations":0}`)
		case "/v1/mcp":
			_, _ = io.WriteString(w, f.mcp.Load().(string))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	f.env["CHARTWORKS_CLIENT_URL"] = f.server.URL
	return f
}
func (f *cliFixture) streams(input io.Reader) IO {
	f.out.Reset()
	f.diagnostic.Reset()
	return IO{Stdin: input, Stdout: &f.out, Stderr: &f.diagnostic, HTTP: f.server.Client(), Lookup: func(name string) (string, bool) {
		if name == "CHARTWORKS_TOKEN" || name == "MCP_TOKEN" || name == "CUSTOM_TOKEN" {
			f.tokenReads++
		}
		value, ok := f.env[name]
		return value, ok
	}}
}
func (f *cliFixture) run(ctx context.Context, args []string, input string) int {
	return Command(ctx, args, f.streams(strings.NewReader(input)))
}

func TestCLIConfigurationDoesNotReadCredentialsOrStartWork(t *testing.T) {
	f := newCLIFixture(t)
	f.env["CHARTWORKS_TOKEN"] = "PRIVATE_CREDENTIAL"
	f.env["CHARTWORKS_CLIENT_TIMEOUT"] = "invalid-env-overridden-by-flag"
	if code := f.run(t.Context(), []string{"config", "--timeout", "2s", "--token-env", "CUSTOM_TOKEN"}, ""); code != 0 {
		t.Fatal("configuration", code, f.diagnostic.String())
	}
	if f.calls.Load() != 0 || f.metadata.Load() != 0 || f.tokenReads != 0 || strings.Contains(f.out.String(), "PRIVATE") || !strings.Contains(f.out.String(), `"timeout":"2s"`) {
		t.Fatal("configuration performed credential or dependency work")
	}
	streams := f.streams(nil)
	streams.OpenDescriptor = func(int) (*os.File, error) { t.Error("configuration opened a credential descriptor"); return nil, nil }
	if code := Command(t.Context(), []string{"config", "--timeout", "1s", "--token-fd", "3"}, streams); code != 0 || !strings.Contains(f.out.String(), "explicit_regular_file_descriptor") {
		t.Fatal("descriptor inspection", code)
	}
	if code := f.run(t.Context(), []string{"config"}, ""); code != 2 {
		t.Fatal("invalid effective timeout accepted", code)
	}
}

func TestCLIRegisteredOperationsAndEffects(t *testing.T) {
	f := newCLIFixture(t)
	for _, args := range [][]string{{"operations"}, {"operations", "--schemas"}, {"operations", "--mcp-token-env", "MCP_TOKEN"}} {
		if code := f.run(t.Context(), args, ""); code != 0 {
			t.Fatal("matrix", code, f.diagnostic.String())
		}
		var rows []cw.OperationInfo
		if json.Unmarshal(f.out.Bytes(), &rows) != nil || len(rows) < 6 {
			t.Fatal("missing generated matrix")
		}
	}
	if code := f.run(t.Context(), []string{"diagnostics"}, ""); code != 0 || !strings.Contains(f.out.String(), `"allowed":true`) {
		t.Fatal("read-only diagnostics", code)
	}
	if code := f.run(t.Context(), []string{"call", "getRetentionPolicy", "--execute"}, ""); code != 0 || !strings.Contains(f.out.String(), `"audit_days":30`) {
		t.Fatal("registered read", code, f.diagnostic.String())
	}
	if code := f.run(t.Context(), []string{"call", "setRetentionPolicy", "--execute", "--input", "-"}, `{"expected_revision":0,"audit_days":30,"operation_hours":24}`); code != 0 {
		t.Fatal("registered write", code, f.diagnostic.String())
	}
	if code := f.run(t.Context(), []string{"call", "listAuditEvents", "--execute", "--query", "limit=5"}, ""); code != 0 {
		t.Fatal("registered query", code, f.diagnostic.String())
	}
	if code := f.run(t.Context(), []string{"call", "sweepRetention", "--execute", "--idempotency-key", "logical-sweep"}, ""); code != 0 {
		t.Fatal("explicit scoped erasure transport", code, f.diagnostic.String())
	}
	before := f.calls.Load()
	for _, test := range []struct {
		args  []string
		input string
	}{
		{[]string{"call", "getRetentionPolicy"}, ""},
		{[]string{"call", "sweepRetention", "--execute"}, ""},
		{[]string{"call", "setRetentionPolicy", "--execute", "--input", "-"}, `{"tenant":"foreign"}`},
		{[]string{"call", "setRetentionPolicy", "--execute", "--input", "-"}, strings.Repeat("x", 4097)},
		{[]string{"call", "getRetentionPolicy", "--execute", "--id", "unexpected"}, ""},
		{[]string{"call", "getRetentionPolicy", "--execute", "--input", "-"}, "{}"},
		{[]string{"call", "getRetentionPolicy", "--execute", "--query", "tenant=foreign"}, ""},
		{[]string{"call", "listAuditEvents", "--execute", "--query", "limit=1", "--query", "limit=2"}, ""},
		{[]string{"call", "setRetentionPolicy", "--execute", "--attempts", "2"}, "{}"},
		{[]string{"call", "mcpTransport", "--execute"}, "{}"},
		{[]string{"call", "unbuiltReport", "--execute"}, "{}"},
	} {
		if code := f.run(t.Context(), test.args, test.input); code != 2 {
			t.Errorf("invalid invocation accepted: %v => %d", test.args, code)
		}
	}
	if f.calls.Load() != before {
		t.Fatal("invalid command reached a business operation")
	}
}

func TestCLIUsageNeverEchoesCredentialArguments(t *testing.T) {
	f := newCLIFixture(t)
	for _, args := range [][]string{
		nil, {"bootstrap"}, {"users"}, {"keys"}, {"grants"}, {"call"}, {"call", "--help"},
		{"config", "--token", "PRIVATE_ARG"}, {"config", "--url", "https://user:PRIVATE_ARG@example.test"},
		{"config", "--url", "https://example.test?token=PRIVATE_ARG"}, {"config", "PRIVATE_ARG"},
		{"config", "--timeout", "0s"}, {"config", "--timeout", "-1s"}, {"config", "--timeout", "16m"}, {"config", "--timeout", "bad"},
		{"config", "--attempts", "0"}, {"config", "--attempts", "4"}, {"config", "--attempts", "2"},
		{"config", "--token-fd", "2"}, {"config", "--token-fd", "1025"}, {"config", "--token-fd", "3", "--token-env", "CUSTOM_TOKEN"},
		{"config", "--token-env", "BAD-NAME"}, {"config", "--mcp-token-env", "BAD-NAME"},
		{"config", "--id", "unexpected"}, {"config", "--execute"}, {"config", "--input", "-"}, {"config", "--schemas"},
		{"call", "getRetentionPolicy", "--execute", "--input", "PRIVATE_FILENAME"},
		{"call", "getRetentionPolicy", "--execute", "--query", "invalid"}, {"call", "getRetentionPolicy", "--execute", "--query", "=value"},
		{"call", "getRetentionPolicy", "--execute", "--query", "name=" + strings.Repeat("x", 16<<10)},
		{"mcp"}, {"mcp", "--execute"}, {"mcp", "--execute", "--input", "-"},
		{"operations", "--mcp-token-env", "BAD-NAME"},
	} {
		if code := f.run(t.Context(), args, ""); code != 2 {
			t.Errorf("bad usage %v => %d", args, code)
		}
		if strings.Contains(f.out.String()+f.diagnostic.String(), "PRIVATE") {
			t.Fatal("argument or dependency detail echoed")
		}
	}
	for _, args := range [][]string{{"help"}, {"--help"}, {"config", "--help"}} {
		if code := f.run(t.Context(), args, ""); code != 0 {
			t.Fatal("help", code)
		}
	}
	delete(f.env, "CHARTWORKS_CLIENT_URL")
	if code := f.run(t.Context(), []string{"config"}, ""); code != 2 {
		t.Fatal("implicit backend URL", code)
	}
	if Command(nil, []string{"help"}, f.streams(nil)) != 2 || Command(t.Context(), nil, IO{}) != 2 { //nolint:staticcheck // Deliberate missing-context rejection regression.
		t.Fatal("missing injected dependencies")
	}
}

func TestCLIStatusCancellationAndInputErrors(t *testing.T) {
	f := newCLIFixture(t)
	for _, test := range []struct {
		status int64
		exit   int
	}{{401, 3}, {403, 3}, {404, 1}, {409, 4}, {410, 4}, {422, 1}, {500, 1}, {503, 1}} {
		f.status.Store(test.status)
		before := f.calls.Load()
		if code := f.run(t.Context(), []string{"call", "getRetentionPolicy", "--execute"}, ""); code != test.exit || f.calls.Load() != before+1 {
			t.Fatal("terminal/default single attempt", test.status, code)
		}
		if strings.Contains(f.out.String()+f.diagnostic.String(), "PRIVATE") {
			t.Fatal("raw dependency rejection echoed")
		}
	}
	f.status.Store(0)
	delete(f.env, "CHARTWORKS_TOKEN")
	if code := f.run(t.Context(), []string{"diagnostics"}, ""); code != 1 {
		t.Fatal("missing caller credential", code)
	}
	f.env["CHARTWORKS_TOKEN"] = "synthetic-client"
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if code := f.run(ctx, []string{"config"}, ""); code != 130 {
		t.Fatal("pre-cancellation lost", code)
	}
	streams := f.streams(nil)
	streams.HTTP = &http.Client{Transport: cliRoundTripper(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	if code := Command(t.Context(), []string{"diagnostics", "--timeout", "10ms"}, streams); code != 124 {
		t.Fatal("request deadline lost", code)
	}
	streams = f.streams(cliBadReader{})
	if code := Command(t.Context(), []string{"call", "setRetentionPolicy", "--execute", "--input", "-"}, streams); code != 1 || strings.Contains(f.diagnostic.String(), "PRIVATE") {
		t.Fatal("input read error", code)
	}
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	defer func() { _ = reader.Close() }()
	inputCtx, cancelInput := context.WithCancel(t.Context())
	defer cancelInput()
	// Cancel at the actual input read, not during the preceding registry lookup.
	// Under concurrent race instrumentation a 40ms wall clock could expire before
	// the CLI owned stdin, making an unrelated Write-to-open-pipe assertion hang.
	streams = f.streams(cliCancelOnRead{PipeReader: reader, cancel: cancelInput})
	if code := Command(inputCtx, []string{"call", "setRetentionPolicy", "--execute", "--input", "-", "--timeout", "5s"}, streams); code != 130 {
		t.Fatal("stdin was not canceled", code)
	}
	if _, err := writer.Write([]byte("after cancellation")); err == nil {
		t.Fatal("canceled input left open")
	}
	streams = f.streams(nil)
	if code := Command(t.Context(), []string{"call", "setRetentionPolicy", "--execute", "--input", "-"}, streams); code != 2 {
		t.Fatal("nil stdin accepted", code)
	}
}

type cliRoundTripper func(*http.Request) (*http.Response, error)

func (f cliRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type cliBadReader struct{}

func (cliBadReader) Read([]byte) (int, error) { return 0, errors.New("PRIVATE_INPUT_ERROR") }

type cliShortWriter struct{}

func (cliShortWriter) Write([]byte) (int, error) { return 0, nil }

type cliBadWriter struct{}

func (cliBadWriter) Write([]byte) (int, error) { return 0, errors.New("PRIVATE_OUTPUT_ERROR") }

func TestCLIExplicitRegularFileDescriptor(t *testing.T) {
	f := newCLIFixture(t)
	for _, body := range []string{"synthetic-file-token\n", "", strings.Repeat("x", (64<<10)+1)} {
		file, err := os.CreateTemp(t.TempDir(), "synthetic-token-")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.WriteString(body); err != nil {
			t.Fatal(err)
		}
		streams := f.streams(nil)
		streams.OpenDescriptor = func(fd int) (*os.File, error) {
			if fd != 3 {
				t.Error("wrong explicit descriptor")
			}
			return file, nil
		}
		code := Command(t.Context(), []string{"diagnostics", "--token-fd", "3"}, streams)
		want := 1
		if body == "synthetic-file-token\n" {
			want = 0
		}
		if code != want {
			t.Fatal("descriptor input", code, want)
		}
		if _, err := file.Stat(); err == nil {
			t.Fatal("owned descriptor not closed")
		}
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	streams := f.streams(nil)
	streams.OpenDescriptor = func(int) (*os.File, error) { return reader, nil }
	if code := Command(t.Context(), []string{"diagnostics", "--token-fd", "3"}, streams); code != 1 {
		t.Fatal("blocking token pipe accepted", code)
	}
	for _, open := range []func(int) (*os.File, error){nil, func(int) (*os.File, error) { return nil, errors.New("PRIVATE_FD") }, func(int) (*os.File, error) { return nil, nil }} {
		streams = f.streams(nil)
		streams.OpenDescriptor = open
		code := Command(t.Context(), []string{"diagnostics", "--token-fd", "3"}, streams)
		if code == 0 || strings.Contains(f.diagnostic.String(), "PRIVATE") {
			t.Fatal("missing/bad descriptor accepted or echoed", code)
		}
	}
	if environmentName("1BAD") || environmentName("") || environmentName(strings.Repeat("X", 129)) || !environmentName("_VALID_2") {
		t.Fatal("invalid token environment names")
	}
}

func TestCLIMCPStatusIsNotHTTPStatus(t *testing.T) {
	f := newCLIFixture(t)
	args := []string{"mcp", "--execute", "--input", "-"}
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	if code := f.run(t.Context(), args, request); code != 0 {
		t.Fatal("MCP success", code)
	}
	for _, test := range []struct {
		body string
		exit int
	}{
		{`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"PRIVATE_PROTOCOL_ERROR","data":"PRIVATE_DETAILS"}}`, 1},
		{`{"jsonrpc":"2.0","id":1,"result":{"isError":true,"structuredContent":{"error":{"code":"forbidden","outcome":"not_started"}}}}`, 3},
		{`{"jsonrpc":"2.0","id":1,"result":{"isError":true,"structuredContent":{"error":{"code":"expired","outcome":"unknown"}}}}`, 4},
		{`{"jsonrpc":"2.0","id":1,"result":{"isError":true,"structuredContent":{"error":{"code":"invalid_request","outcome":"not_started"}}}}`, 2},
		{`{"jsonrpc":"2.0","id":1,"result":{"isError":true,"structuredContent":{"error":{"code":"unavailable","outcome":"unknown"}}}}`, 1},
		{`{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-1}}`, 1},
		{`{"jsonrpc":"2.0","id":1,"result":{},"result":{}}`, 1},
		{`{"jsonrpc":"2.0","id":1}`, 1},
		{`{"jsonrpc":"2.0","id":1,"result":false}`, 1},
	} {
		f.mcp.Store(test.body)
		before := f.calls.Load()
		if code := f.run(t.Context(), args, request); code != test.exit || f.calls.Load() != before+1 {
			t.Fatal("MCP result misreported or retried", code, test.exit)
		}
		if strings.Contains(f.out.String()+f.diagnostic.String(), "PRIVATE") {
			t.Fatal("raw protocol error exposed")
		}
	}
	f.status.Store(202)
	if code := f.run(t.Context(), args, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != 0 || f.out.Len() != 0 {
		t.Fatal("initialized notification", code)
	}
	if code := f.run(t.Context(), args, "not-json"); code != 2 {
		t.Fatal("invalid MCP input", code)
	}
}

func TestCLIOutputFailuresAndInputBounds(t *testing.T) {
	f := newCLIFixture(t)
	for _, writer := range []io.Writer{cliShortWriter{}, cliBadWriter{}} {
		for _, args := range [][]string{{"help"}, {"config"}, {"operations"}, {"diagnostics"}, {"call", "getRetentionPolicy", "--execute"}} {
			streams := f.streams(nil)
			streams.Stdout = writer
			if code := Command(t.Context(), args, streams); code != 1 {
				t.Fatal("incomplete stdout reported success", args, code)
			}
		}
		if writeBody(writer, []byte("x")) != 1 || writeJSON(writer, map[string]int{"value": 1}) != 1 || writeUsage(writer, 0) != 1 {
			t.Fatal("output error ignored")
		}
	}
	if writeJSON(io.Discard, make(chan int)) != 1 || writeBody(io.Discard, nil) != 0 {
		t.Fatal("output encoding failure")
	}
	if _, err := readInput(t.Context(), strings.NewReader("too large"), 2); !errors.Is(err, cw.ErrInvalidCall) {
		t.Fatal("input bound", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readInput(ctx, strings.NewReader("never"), 10); !errors.Is(err, context.Canceled) {
		t.Fatal("input cancellation", err)
	}
	if _, err := descriptorProvider(nil)(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("descriptor pre-cancellation", err)
	}
	q := queryFlags{values: make(map[string][]string)}
	if q.String() != "" {
		t.Fatal("query flag echoed input")
	}
	for range 64 {
		q.values[strings.Repeat("x", len(q.values)+1)] = []string{"1"}
	}
	if q.Set("last=value") == nil {
		t.Fatal("unbounded query list")
	}
}

func FuzzCLIConfiguration(f *testing.F) {
	for _, value := range []string{"CHARTWORKS_TOKEN", "_PROVIDER_1", "bad-name", "PRIVATE\nVALUE", ""} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 1024 {
			return
		}
		var output, diagnostic bytes.Buffer
		streams := IO{Stdout: &output, Stderr: &diagnostic, Lookup: func(string) (string, bool) { return "", false }}
		code := Command(t.Context(), []string{"config", "--url", "https://example.test", "--token-env", name}, streams)
		if code != 0 && code != 2 {
			t.Fatal("unstable configuration exit", code)
		}
	})
}

// cliCancelOnRead deterministically targets in-flight stdin ownership rather
// than relying on network scheduling to beat a short fixture deadline.
type cliCancelOnRead struct {
	*io.PipeReader
	cancel context.CancelFunc
}

func (r cliCancelOnRead) Read(data []byte) (int, error) {
	r.cancel()
	return r.PipeReader.Read(data)
}
