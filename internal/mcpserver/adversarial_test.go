package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTransportRejectsAmbiguityBeforeDomain(t *testing.T) {
	f := newAuthority(t)
	var calls atomic.Int64
	s := newTestServer(t, f, func(ctx context.Context, e identity.Envelope, in testInput) (testOutput, error) {
		calls.Add(1)
		return fixtureCall(ctx, e, in)
	}, config.DefaultMCP())
	token := f.token(t, "one", "alice", f.cfg.MCPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:allowed")
	for _, tc := range []struct {
		name   string
		body   string
		edit   func(*http.Request)
		status int
	}{
		{"query token", readRPC, func(r *http.Request) { r.URL.RawQuery = "token=private" }, 400},
		{"empty query", readRPC, func(r *http.Request) { r.URL.ForceQuery = true }, 400},
		{"escaped path", readRPC, func(r *http.Request) { r.URL.RawPath = "/v1/%6dcp" }, 400},
		{"origin", readRPC, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, 403},
		{"duplicate origin", readRPC, func(r *http.Request) {
			r.Header.Add("Origin", "https://console.example")
			r.Header.Add("Origin", "https://console.example")
		}, 403},
		{"host", readRPC, func(r *http.Request) { r.Host = "evil.example" }, 403},
		{"forwarded host", readRPC, func(r *http.Request) { r.Host = "evil.example"; r.Header.Set("X-Forwarded-Host", "localhost") }, 403},
		{"bad port", readRPC, func(r *http.Request) { r.Host = "localhost:0" }, 403},
		{"cookie only", readRPC, func(r *http.Request) { r.Header.Del("Authorization"); r.Header.Set("Cookie", "token="+token) }, 401},
		{"dual bearer", readRPC, func(r *http.Request) { r.Header.Add("Authorization", "Bearer "+token) }, 401},
		{"empty encoding", readRPC, func(r *http.Request) { r.Header.Set("Content-Encoding", "") }, 400},
		{"duplicate encoding", readRPC, func(r *http.Request) { r.Header.Set("Content-Encoding", ""); r.Header.Add("Content-Encoding", "gzip") }, 400},
		{"oversized accept", readRPC, func(r *http.Request) {
			r.Header.Set("Accept", "application/json; x="+strings.Repeat("x", 1024)+", text/event-stream")
		}, 400},
		{"aggregate accept size", readRPC, func(r *http.Request) {
			r.Header.Set("Accept", "application/json; x="+strings.Repeat("x", 600))
			r.Header.Add("Accept", "text/event-stream; x="+strings.Repeat("x", 600))
		}, 400},
		{"compression", readRPC, func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }, 400},
		{"session", readRPC, func(r *http.Request) { r.Header.Set("Mcp-Session-Id", "invented") }, 400},
		{"resume", readRPC, func(r *http.Request) { r.Header.Set("Last-Event-Id", "foreign") }, 400},
		{"unknown protocol", readRPC, func(r *http.Request) { r.Header.Set("Mcp-Protocol-Version", "9999-12-31") }, 400},
		{"duplicate protocol", readRPC, func(r *http.Request) {
			r.Header.Add("Mcp-Protocol-Version", "2025-11-25")
			r.Header.Add("Mcp-Protocol-Version", "2025-11-25")
		}, 400},
		{"wrong media", readRPC, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		{"duplicate media", readRPC, func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }, 415},
		{"missing accept", readRPC, func(r *http.Request) { r.Header.Del("Accept") }, 400},
		{"unacceptable JSON", readRPC, func(r *http.Request) { r.Header.Set("Accept", "application/json;q=0, text/event-stream") }, 400},
		{"bad accept", readRPC, func(r *http.Request) { r.Header.Set("Accept", "not a media type") }, 400},
		{"unknown envelope field", strings.Replace(readRPC, `"jsonrpc"`, `"token":"private","jsonrpc"`, 1), nil, 400},
		{"duplicate method", strings.Replace(readRPC, `"method":`, `"method":"ping","method":`, 1), nil, 400},
		{"fractional id", strings.Replace(readRPC, `"id":1`, `"id":1.1`, 1), nil, 400},
		{"injection id", strings.Replace(readRPC, `"id":1`, `"id":"<script>private</script>"`, 1), nil, 400},
		{"unsafe integer", strings.Replace(readRPC, `"id":1`, `"id":9007199254740993`, 1), nil, 400},
		{"batch", "[" + readRPC + "]", nil, 400},
		{"trailing", readRPC + `{}`, nil, 400},
		{"invalid utf8", string([]byte{255}), nil, 400},
		{"unknown method", `{"jsonrpc":"2.0","id":1,"method":"private_native_method"}`, nil, 400},
		{"missing id", `{"jsonrpc":"2.0","method":"ping"}`, nil, 400},
		{"notification id", `{"jsonrpc":"2.0","id":1,"method":"notifications/initialized"}`, nil, 400},
		{"get", readRPC, func(r *http.Request) { r.Method = "GET" }, 405},
		{"get origin", readRPC, func(r *http.Request) { r.Method = "GET"; r.Header.Set("Origin", "https://evil.example") }, 403},
		{"other mount", readRPC, func(r *http.Request) { r.URL.Path = "/v1/other" }, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := rpc(t, s.Handler(), token, tc.body, tc.edit)
			if w.Code != tc.status {
				t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), token) {
				t.Fatal("unsafe failure disclosure")
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("rejected transport reached domain", calls.Load())
	}
	settings := config.DefaultMCP()
	settings.MaxRequestBytes = 1024
	small := newTestServer(t, f, fixtureCall, settings)
	w := rpc(t, small.Handler(), token, strings.Repeat(" ", 1025), nil)
	if w.Code != 413 {
		t.Fatal("request body cap", w.Code)
	}
	w = rpc(t, s.Handler(), token, readRPC, func(r *http.Request) { r.Body = io.NopCloser(failingReader{}) })
	if w.Code != 400 {
		t.Fatal("body I/O failure", w.Code)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("secret native body failure") }

func TestDispatchSafetyAndBoundedOutcomes(t *testing.T) {
	f := newAuthority(t)
	token := f.token(t, "one", "alice", f.cfg.MCPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:allowed")
	var mode atomic.Int64
	s := newTestServer(t, f, func(ctx context.Context, e identity.Envelope, in testInput) (testOutput, error) {
		switch mode.Load() {
		case 1:
			panic("secret SQL stack")
		case 2:
			return testOutput{}, errors.New("secret SQL error")
		case 3:
			o, err := fixtureCall(ctx, e, in)
			o.Item = strings.Repeat("private", 10000)
			return o, err
		case 4:
			f.clock.Add(400)
			return fixtureCall(ctx, e, in)
		}
		return fixtureCall(ctx, e, in)
	}, config.DefaultMCP())
	client, err := s.Client(func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, args, code string }{
		{"fixture_read", `{"item":"allowed","tenant":"other"}`, "invalid_request"},
		{"fixture_read", `{"item":"allowed","item":"foreign"}`, "invalid_request"},
		{"fixture_read", `{"item":null}`, "invalid_request"},
		{"fixture_read", `{}`, "invalid_request"},
		{"absent", `{}`, "not_found"},
		{"fixture_read", `{"item":"foreign"}`, "not_found"},
	} {
		out, e := client.CallTool(t.Context(), tc.name, json.RawMessage(tc.args))
		if e != nil || !out.IsError || s.faultFor(out).Code != tc.code {
			t.Fatalf("%s: %#v %v", tc.code, out, e)
		}
	}
	for _, m := range []int64{1, 2} {
		mode.Store(m)
		out, e := client.CallTool(t.Context(), "fixture_read", json.RawMessage(`{"item":"allowed"}`))
		if e != nil || !out.IsError || s.faultFor(out).Code != "unavailable" || s.faultFor(out).Outcome != "unknown" {
			t.Fatal("unsafe started failure", out, e)
		}
		b, _ := json.Marshal(out)
		if strings.Contains(string(b), "secret") {
			t.Fatal("native failure disclosure")
		}
	}
	mode.Store(0)
	e, err := f.verifier.Verify(t.Context(), token, auth.MCP)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := e.Context(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out := s.dispatch(ctx, "fixture_read", json.RawMessage(`{"item":"allowed"}`)); !out.IsError || s.faultFor(out).Code != "unauthenticated" {
		t.Fatal("admission bypass")
	}
	if out := s.dispatch(context.Background(), "fixture_read", nil); !out.IsError {
		t.Fatal("missing envelope")
	}
	wrong, _ := s.Client(func(context.Context) (string, error) { return "", errors.New("private token provider") })
	if _, err = wrong.ListTools(t.Context()); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	// The absent context is deliberately invalid input, not an optional default.
	var absentContext context.Context
	if _, err = client.ListTools(absentContext); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("nil context", err)
	}
	noAction, _ := s.Client(func(context.Context) (string, error) {
		return f.token(t, "one", "alice", f.cfg.MCPAudience(), "mcp.use"), nil
	})
	out, err := noAction.CallTool(t.Context(), "fixture_read", json.RawMessage(`{"item":"allowed"}`))
	if err != nil || s.faultFor(out).Code != "forbidden" {
		t.Fatal("missing action", out, err)
	}
	list, err := noAction.ListTools(t.Context())
	if err != nil || len(list.Tools) != 0 {
		t.Fatal("unauthorized catalog", list, err)
	}
	resources, err := noAction.ListResources(t.Context())
	if err != nil || len(resources.Resources) != 0 {
		t.Fatal(err)
	}
	templates, err := noAction.ListResourceTemplates(t.Context())
	if err != nil || len(templates.ResourceTemplates) != 0 {
		t.Fatal(err)
	}
	if _, err = client.ReadResource(t.Context(), "chartworks://fixtures/foreign"); err == nil || !strings.Contains(err.Error(), "not_found") {
		t.Fatal("foreign resource", err)
	}
	if _, err = client.ReadResource(t.Context(), "https://secrets.example/allowed"); err == nil {
		t.Fatal("external resource")
	}
	mode.Store(4)
	out, err = client.CallTool(t.Context(), "fixture_read", json.RawMessage(`{"item":"allowed"}`))
	if err != nil || !out.IsError {
		t.Fatal("expired in-flight authority exposed values", out, err)
	}
	if _, err = client.ListTools(t.Context()); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("expired authority reused", err)
	}
}

func TestRequestDeadlineCancellationAndCapacity(t *testing.T) {
	f := newAuthority(t)
	token := f.token(t, "one", "alice", f.cfg.MCPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:allowed")
	settings := config.DefaultMCP()
	settings.MaxConcurrent = 1
	settings.Timeout = config.Duration(time.Second)
	entered := make(chan struct{}, 1)
	s := newTestServer(t, f, func(ctx context.Context, _ identity.Envelope, _ testInput) (testOutput, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return testOutput{}, ctx.Err()
	}, settings)
	client, _ := s.Client(func(context.Context) (string, error) { return token, nil })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		out, _ := client.CallTool(ctx, "fixture_read", json.RawMessage(`{"item":"allowed"}`))
		done <- out
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("call did not start")
	}
	if _, err := client.ListTools(t.Context()); !errors.Is(err, errBusy) {
		t.Fatal("unbounded admission", err)
	}
	w := rpc(t, s.Handler(), token, readRPC, nil)
	if w.Code != 429 {
		t.Fatal("network admission cap", w.Code, w.Body.String())
	}
	cancel()
	select {
	case out := <-done:
		if out == nil || !out.IsError {
			t.Fatal("cancelled output", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call did not cancel")
	}
	start := time.Now()
	out, err := client.CallTool(t.Context(), "fixture_read", json.RawMessage(`{"item":"allowed"}`))
	if err != nil || !out.IsError || time.Since(start) > 5*time.Second {
		t.Fatal("deadline not enforced", out, err)
	}
	cancelled, cancel2 := context.WithCancel(t.Context())
	cancel2()
	if _, err = client.ListTools(cancelled); err == nil {
		t.Fatal("cancelled request admitted")
	}
	// The SDK detaches HTTP contexts: prove the exact incoming deadline is restored.
	ctx, cancel3 := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel3()
	start = time.Now()
	w = rpc(t, s.Handler(), token, readRPC, func(r *http.Request) { *r = *r.WithContext(ctx) })
	if time.Since(start) > 2*time.Second || strings.Contains(w.Body.String(), `"exact"`) {
		t.Fatal("HTTP cancellation detached", w.Code, w.Body.String())
	}
}

func TestResponseLimitsAndSanitization(t *testing.T) {
	f := newAuthority(t)
	settings := config.DefaultMCP()
	settings.MaxResponseBytes = 16384
	s := newTestServer(t, f, func(ctx context.Context, e identity.Envelope, in testInput) (testOutput, error) {
		out, err := fixtureCall(ctx, e, in)
		out.Item = strings.Repeat("x", 9000)
		return out, err
	}, settings)
	token := f.token(t, "one", "alice", f.cfg.MCPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:allowed")
	client, _ := s.Client(func(context.Context) (string, error) { return token, nil })
	out, err := client.CallTool(t.Context(), "fixture_read", json.RawMessage(`{"item":"allowed"}`))
	if err != nil || !out.IsError || s.faultFor(out).Code != "limit_exceeded" {
		t.Fatal("duplicated content exceeded response budget", out, err)
	}
	for _, code := range []int{-32700, -32600, -32601, -32602, -32603, -32000} {
		raw := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"error":{"code":%d,"message":"private stack SQL","data":"token"}}`, code))
		safe := sanitizeProtocol(raw, 16384)
		if safe == nil || strings.Contains(string(safe), "private") || strings.Contains(string(safe), "token") {
			t.Fatal("unsafe SDK error", string(safe))
		}
	}
	for _, raw := range []string{`{`, `{"a":1,"a":2}`, `{"error":42}`} {
		if sanitizeProtocol([]byte(raw), 100) != nil {
			t.Fatal("invalid response accepted")
		}
	}
	b := newBoundedResponse(4)
	b.WriteHeader(201)
	b.WriteHeader(500)
	if _, err = b.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	b.Flush()
	if _, err = b.Write([]byte("5")); err == nil {
		t.Fatal("unbounded response")
	}
	status, body, large := b.result()
	if status != 201 || len(body) != 4 || !large {
		t.Fatal(status, string(body), large)
	}
	for _, err := range []error{access.ErrUnauthenticated, access.ErrForbidden, errBusy, context.Canceled, errors.New("private")} {
		w := httpError(err)
		if w.status < 400 || strings.Contains(w.code, "private") {
			t.Fatal(w)
		}
	}
	for _, status := range []int{400, 403, 413, 415, 418} {
		if safeTransportError(status).status < 400 {
			t.Fatal(status)
		}
	}
}

func TestRegistrationAndResourceAmbiguity(t *testing.T) {
	reg := testRegistry(t, fixtureCall)
	base := reg.bindings[0]
	if _, err := NewRegistry(nil); err == nil {
		t.Fatal("empty catalog")
	}
	for _, bindings := range [][]Binding{{{}}, {base, base}} {
		if _, err := NewRegistry(bindings); err == nil {
			t.Fatal("invalid registration")
		}
	}
	a := base
	a.resource = "chartworks://fixtures/{item}/fixed"
	b := base
	b.name = "other"
	b.definition.ID = "other"
	b.resource = "chartworks://fixtures/fixed/{item}"
	if _, err := NewRegistry([]Binding{a, b}); err == nil {
		t.Fatal("overlapping resource templates allow ambiguous action dispatch")
	}
	if !resourcePatternsOverlap("chartworks://fixtures/fixed", "chartworks://fixtures/{item}") || resourcePatternsOverlap("chartworks://fixtures/a", "chartworks://fixtures/b") {
		t.Fatal("resource collision analysis")
	}
	for _, uri := range []string{"https://x/y", "chartworks://fixtures/%61llowed", "chartworks://fixtures/a?token=secret", "chartworks://fixtures/a#fragment", "chartworks://fixtures/../a", "chartworks://user@fixtures/a", "chartworks://fixtures//a", "chartworks://fixtures/a\\b"} {
		if _, _, ok := reg.resource(uri); ok {
			t.Fatal("unsafe resource URI", uri)
		}
	}
	if _, err := WithResource(Binding{}, "chartworks://fixtures/a"); err == nil {
		t.Fatal("unbound resource")
	}
	for _, pattern := range []string{"chartworks://fixtures/{unknown}", "chartworks://fixtures/{item}/{item}", "chartworks://fixtures/fixed", "chartworks://fixtures/{item}/.."} {
		b := base
		b.resource = ""
		if _, err := WithResource(b, pattern); err == nil {
			t.Fatal("bad resource projection", pattern)
		}
	}
	if _, err := SelectGroups([]Binding{base}, []string{"unknown"}); err == nil {
		t.Fatal("unknown group")
	}
	if _, err := SelectGroups([]Binding{base}, []string{"discovery", "discovery"}); err == nil {
		t.Fatal("duplicate group")
	}
	if got, err := SelectGroups([]Binding{base}, []string{"charts"}); err != nil || len(got) != 0 {
		t.Fatal("disabled group retained", err)
	}
	detached := reg.Manifest()
	detached[0].Name = "tampered"
	detached[0].Meta["chartworks/action"] = "other"
	detached[0].InputSchema.(json.RawMessage)[0] = 'x'
	if reg.Manifest()[0].Name != "fixture_read" || reg.Manifest()[0].Meta["chartworks/action"] != "fixture.read" {
		t.Fatal("mutable registration")
	}
	if !json.Valid(reg.Manifest()[0].InputSchema.(json.RawMessage)) {
		t.Fatal("mutable schema")
	}
	for _, effect := range []string{"metadata_read", "retained_metadata_read", "byo_context_read", "caller_data_transform_no_persistence", "caller_data_selection_optional_gateway_rank", "nlq_routing_and_preflight_commit", "nlq_generation_and_plan_commit", "nlq_validated_read_execution", "nlq_refine_generation_and_plan_commit", "nlq_feedback_commit", "byo_context_retrieval_and_commit", "byo_validated_read_and_receipt"} {
		ef, ok := effectFor(effect)
		if !ok || ef.readOnly && (ef.paid || ef.persists) {
			t.Fatal("unsafe annotation", effect)
		}
	}
	if _, ok := effectFor("unclassified"); ok {
		t.Fatal("unknown effect")
	}
	definition := base.definition
	owner, err := api.New([]api.Definition{definition})
	if err != nil {
		t.Fatal(err)
	}
	mapper := func(error) Fault { return Fault{Code: "unavailable"} }
	if _, err = Bind(owner, definition.ID, "invalid-name", "discovery", "A synthetic valid description.", fixtureCall, mapper); err == nil {
		t.Fatal("invalid name")
	}
	if _, err = Bind(owner, "missing", "test", "discovery", "A synthetic valid description.", fixtureCall, mapper); err == nil {
		t.Fatal("missing owner")
	}
	if _, err = Bind(owner, definition.ID, "test", "discovery", "short", fixtureCall, mapper); err == nil {
		t.Fatal("missing description")
	}
	if _, err = Bind(owner, definition.ID, "test", "discovery", "A synthetic valid description.", func(context.Context, identity.Envelope, struct{}) (testOutput, error) { return testOutput{}, nil }, mapper); err == nil {
		t.Fatal("input schema mismatch")
	}
	if _, err = Bind(owner, definition.ID, "test", "discovery", "A synthetic valid description.", func(context.Context, identity.Envelope, testInput) (struct{}, error) { return struct{}{}, nil }, mapper); err == nil {
		t.Fatal("output schema mismatch")
	}
	other, _ := api.SchemaFor("other", reflect.TypeFor[struct{}](), false)
	if sameSchema(other, base.input) || sameSchema(nil, base.input) {
		t.Fatal("schema equality")
	}
	if !toolName("a_1") || toolName("") || toolName(strings.Repeat("a", 49)) {
		t.Fatal("name bounds")
	}
}

func TestConstructionAndUntrustedMapper(t *testing.T) {
	f := newAuthority(t)
	reg := testRegistry(t, fixtureCall)
	for _, origins := range [][]string{{"*"}, {"https://user:secret@host"}, {"https://host/path"}, {"https://host?token=x"}, {"https://host", "https://host"}} {
		if _, err := New(f.verifier, reg, config.DefaultMCP(), origins); err == nil {
			t.Fatal("invalid origins accepted", origins)
		}
	}
	if _, err := New(nil, reg, config.DefaultMCP(), nil); err == nil {
		t.Fatal("nil verifier")
	}
	if _, err := New(f.verifier, nil, config.DefaultMCP(), nil); err == nil {
		t.Fatal("nil registry")
	}
	settings := config.DefaultMCP()
	settings.MaxConcurrent = 0
	if _, err := HTTPRegistry(settings); err == nil {
		t.Fatal("invalid transport bounds")
	}
	s := newTestServer(t, f, fixtureCall, config.DefaultMCP())
	if _, err := s.Client(nil); err == nil {
		t.Fatal("nil token provider")
	}
	// Test the rejection boundary with an intentionally absent call context.
	var absentContext context.Context
	if err := s.admit(absentContext, func(context.Context) error { return nil }); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	token := f.token(t, "one", "alice", f.cfg.MCPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:allowed")
	b := reg.bindings[0]
	b.invoke = func(context.Context, identity.Envelope, json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("private")
	}
	b.classify = func(error) Fault {
		return Fault{Code: "private error text", Receipt: &gateway.Receipt{Warning: strings.Repeat("x", 20000)}}
	}
	r, err := NewRegistry([]Binding{b})
	if err != nil {
		t.Fatal(err)
	}
	s, err = New(f.verifier, r, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := s.Client(func(context.Context) (string, error) { return token, nil })
	out, err := c.CallTool(t.Context(), "fixture_read", json.RawMessage(`{"item":"allowed"}`))
	if err != nil || s.faultFor(out).Code != "unavailable" || s.faultFor(out).Receipt != nil {
		t.Fatal("unsafe mapper result", out, err)
	}
}

func FuzzMCPBoundaries(f *testing.F) {
	for _, seed := range []string{readRPC, "{}", "chartworks://fixtures/allowed", `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"native"}}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 16384 {
			return
		}
		_ = requestSchema.Validate([]byte(input), 16384)
		_ = sanitizeProtocol([]byte(input), 16384)
		_, _ = resourceParts(input)
		_, _ = resourceArguments("chartworks://fixtures/{item}", input)
		_ = accepted([]string{input})
	})
}
