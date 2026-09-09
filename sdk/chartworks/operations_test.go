package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/gateway"
)

const unitResult = `{"value":9007199254740993}`

// This is a transport unit fixture, not a substitute for the real PostgreSQL and
// verified-envelope services exercised by TestPhase23 in test/acceptance.
func unitOperationRegistry(t *testing.T) *api.Registry {
	t.Helper()
	schema := func(name, body string) *gateway.Schema {
		s, err := gateway.NewSchema(name, []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	empty := schema("unitEmpty", `{"type":"object","additionalProperties":false}`)
	value := schema("unitValue", `{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"],"additionalProperties":false}`)
	anyObject := schema("unitObject", `{"type":"object","additionalProperties":true}`)
	binary := schema("unitBinary", `{"type":"string","format":"binary"}`)
	text := schema("unitText", `{"type":"string"}`)
	faults := []api.ErrorResponse{{Status: 401, Code: "unauthorized"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 409, Code: "conflict"}, {Status: 410, Code: "expired"}, {Status: 422, Code: "invalid_request"}, {Status: 429, Code: "busy"}, {Status: 503, Code: "unavailable"}}
	definition := func(id, method, path, effect string) api.Definition {
		return api.Definition{Operation: api.Operation{Method: method, Path: path, Action: "ops.read", Effect: effect}, ID: id, Summary: "Synthetic registered transport operation", ResourceLoader: "unit.transport", Audit: "unit.metadata", Response: value, Errors: faults}
	}
	read := definition("readFixture", "GET", "/v1/reads/{id}", "read")
	read.Query = []api.Parameter{{Name: "limit", In: "query", Type: "integer", Min: 1, Max: 5}}
	head := definition("readHead", "HEAD", "/v1/reads/{id}", "read")
	write := definition("writeFixture", "POST", "/v1/writes", "write")
	write.Request, write.MaxBodyBytes = empty, 1024
	erase := definition("eraseFixture", "POST", "/v1/erase", "erase")
	erase.Replay = "keyed"
	erase.Request, erase.MaxBodyBytes = value, 1024
	erase.Headers = []api.Parameter{{Name: "Idempotency-Key", In: "header", Type: "string", Required: true, Max: 128, Pattern: "^[A-Za-z0-9_.:-]+$"}}
	upload := definition("uploadFixture", "POST", "/v1/uploads", "write")
	upload.Request, upload.MaxBodyBytes, upload.RequestContentType = binary, 2048, "application/octet-stream"
	mcp := definition("mcpFixture", "POST", "/v1/mcp", "protocol_dispatch")
	mcp.Surface, mcp.Request, mcp.MaxBodyBytes = auth.MCP, anyObject, 10<<20
	mcp.Response = anyObject
	mcp.EmptySuccess = []int{202}
	document := definition("openapi", "GET", "/openapi.json", "public_read")
	document.Public, document.Action, document.Response = true, "", anyObject
	metric := definition("textFixture", "GET", "/v1/text", "read")
	metric.Response, metric.ResponseContentType = text, "text/plain; version=0.0.4"
	r, err := api.New([]api.Definition{read, head, write, erase, upload, mcp, document, metric})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func unitOperationServer(t *testing.T, prefix string, serve http.HandlerFunc) (*Client, *httptest.Server, []byte) {
	t.Helper()
	document, err := unitOperationRegistry(t).OpenAPI("Synthetic transport fixture", "1")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, prefix+"/") {
			t.Error("base prefix escaped")
			w.WriteHeader(404)
			return
		}
		if r.URL.Path == prefix+"/openapi.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(document)
			return
		}
		serve(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL+prefix, server.Client(), func(context.Context) (string, error) { return "synthetic-current", nil })
	if err != nil {
		t.Fatal(err)
	}
	return client, server, document
}

func TestOperationCatalogAndInvocation(t *testing.T) {
	var calls atomic.Int64
	client, _, document := unitOperationServer(t, "/charts", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-current" {
			t.Error("missing current provider token")
		}
		switch r.URL.Path {
		case "/charts/v1/reads/item":
			if r.Method == "HEAD" {
				return
			}
			if r.URL.Query().Get("limit") != "2" {
				t.Error("query lost")
			}
		case "/charts/v1/writes":
			body, _ := io.ReadAll(r.Body)
			if string(body) != "{}" {
				t.Error("empty contract changed")
			}
		case "/charts/v1/uploads":
			body, _ := io.ReadAll(r.Body)
			if string(body) != "raw\x00bytes" || r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Error("binary upload changed")
			}
		case "/charts/v1/text":
			_, _ = io.WriteString(w, "fixture_total 2\n")
			return
		default:
			t.Error("unregistered path", r.URL.Path)
		}
		_, _ = io.WriteString(w, unitResult)
	})
	rows, err := client.Operations(t.Context())
	if err != nil || len(rows) != 8 {
		t.Fatal("catalog", len(rows), err)
	}
	parsed, err := ParseOperations(document)
	if err != nil || len(parsed) != len(rows) {
		t.Fatal("offline matrix", err)
	}
	for i, row := range rows {
		if i > 0 && rows[i-1].ID >= row.ID {
			t.Fatal("unstable ordering")
		}
		if row.ID == "mcpFixture" && (row.SDKMethod != "MCP" || row.CLICommand != "client mcp") {
			t.Fatal("MCP projection")
		}
		if row.ID == "eraseFixture" && row.Replay != "keyed" {
			t.Fatal("logical key not registered")
		}
		rows[i].Path = "/v1/evil"
		rows[i].RequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true}`)
	}
	result, err := client.Invoke(t.Context(), "readFixture", CallOptions{ResourceID: "item", Query: url.Values{"limit": {"2"}}})
	if err != nil || string(result.Body) != unitResult {
		t.Fatal("lossless read", string(result.Body), err)
	}
	if _, err = client.Invoke(t.Context(), "writeFixture", CallOptions{}); err != nil {
		t.Fatal("empty request", err)
	}
	if _, err = client.Invoke(t.Context(), "uploadFixture", CallOptions{Body: []byte("raw\x00bytes")}); err != nil {
		t.Fatal("raw request", err)
	}
	result, err = client.Invoke(t.Context(), "readHead", CallOptions{ResourceID: "item"})
	if err != nil || len(result.Body) != 0 {
		t.Fatal("bodyless HEAD", err)
	}
	result, err = client.Invoke(t.Context(), "textFixture", CallOptions{})
	if err != nil || string(result.Body) != "fixture_total 2\n" || !strings.HasPrefix(result.ContentType, "text/plain") {
		t.Fatal("text response", err)
	}
	if calls.Load() != 5 {
		t.Fatal("unexpected core calls", calls.Load())
	}
}

func TestInvocationRejectsUnsafeCoordinatesAndReplays(t *testing.T) {
	var calls atomic.Int64
	client, _, _ := unitOperationServer(t, "", func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); _, _ = io.WriteString(w, unitResult) })
	cases := []struct {
		id      string
		options CallOptions
		want    error
	}{
		{"futureReport", CallOptions{}, ErrUnknownOperation},
		{"createUser", CallOptions{}, ErrUnknownOperation},
		{"../escape", CallOptions{}, ErrInvalidCall},
		{"readFixture", CallOptions{}, ErrInvalidCall},
		{"readFixture", CallOptions{ResourceID: ".."}, ErrInvalidCall},
		{"readFixture", CallOptions{ResourceID: "a/b"}, ErrInvalidCall},
		{"readFixture", CallOptions{ResourceID: "item", Query: url.Values{"token": {"PRIVATE"}}}, ErrInvalidCall},
		{"readFixture", CallOptions{ResourceID: "item", Query: url.Values{"limit": {"0"}}}, ErrInvalidCall},
		{"readFixture", CallOptions{ResourceID: "item", Query: url.Values{"limit": {"2", "3"}}}, ErrInvalidCall},
		{"readFixture", CallOptions{ResourceID: "item", Query: url.Values{"limit": {"bad"}}}, ErrInvalidCall},
		{"readFixture", CallOptions{ResourceID: "item", IdempotencyKey: "unexpected"}, ErrInvalidCall},
		{"readFixture", CallOptions{ResourceID: "item", Body: []byte("{}")}, ErrInvalidCall},
		{"writeFixture", CallOptions{ResourceID: "unexpected"}, ErrInvalidCall},
		{"writeFixture", CallOptions{Body: []byte(`{"tenant":"foreign"}`)}, ErrInvalidCall},
		{"writeFixture", CallOptions{Body: []byte(`{"a":1,"a":2}`)}, ErrInvalidCall},
		{"writeFixture", CallOptions{Body: []byte(strings.Repeat("x", 1025))}, ErrInvalidCall},
		{"writeFixture", CallOptions{Attempts: 2}, ErrUnsafeRetry},
		{"eraseFixture", CallOptions{Attempts: 2}, ErrUnsafeRetry},
		{"eraseFixture", CallOptions{Body: []byte(`{"value":1}`)}, ErrInvalidCall},
		{"eraseFixture", CallOptions{IdempotencyKey: "bad\r\nkey"}, ErrInvalidCall},
		{"readFixture", CallOptions{Attempts: 4}, ErrInvalidCall},
		{"readFixture", CallOptions{Attempts: -1}, ErrInvalidCall},
		{"mcpFixture", CallOptions{}, ErrInvalidCall},
	}
	for _, test := range cases {
		_, err := client.Invoke(t.Context(), test.id, test.options)
		if !errors.Is(err, test.want) {
			t.Errorf("%s: %v, want %v", test.id, err, test.want)
		}
		if err != nil && strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("input echoed")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid invocation reached a core", calls.Load())
	}
}

func TestExplicitRetryPreservesKeyBodyAndFreshProvider(t *testing.T) {
	original := []byte(`{"value":1}`)
	var calls atomic.Int64
	var providers atomic.Int64
	client, _, _ := unitOperationServer(t, "", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"value":1}` || r.Header.Get("Idempotency-Key") != "logical-once" {
			t.Error("replay changed intent")
		}
		if n == 1 {
			copy(original, []byte(`{"value":2}`))
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, unitResult)
	})
	client.token = func(context.Context) (string, error) { providers.Add(1); return "fresh-from-caller", nil }
	result, err := client.Invoke(t.Context(), "eraseFixture", CallOptions{Body: original, IdempotencyKey: "logical-once", Attempts: 3})
	if err != nil || string(result.Body) != unitResult || calls.Load() != 2 || providers.Load() != 3 {
		t.Fatal("bounded replay", calls.Load(), providers.Load(), err)
	}
	for _, status := range []int{401, 403, 404, 409, 410, 500} {
		var attempts atomic.Int64
		c, _, _ := unitOperationServer(t, "", func(w http.ResponseWriter, _ *http.Request) {
			attempts.Add(1)
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "PRIVATE_SERVER_ERROR")
		})
		_, err = c.Invoke(t.Context(), "readFixture", CallOptions{ResourceID: "item", Attempts: 3})
		var rejected *StatusError
		if !errors.As(err, &rejected) || rejected.Status != status || attempts.Load() != 1 || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("terminal status retried", status, attempts.Load(), err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var attempts atomic.Int64
	c, _, _ := unitOperationServer(t, "", func(w http.ResponseWriter, _ *http.Request) { attempts.Add(1); w.WriteHeader(503); cancel() })
	_, err = c.Invoke(ctx, "readFixture", CallOptions{ResourceID: "item", Attempts: 3})
	if !errors.Is(err, context.Canceled) || attempts.Load() != 1 {
		t.Fatal("cancellation lost during replay", err, attempts.Load())
	}
}

func TestCatalogRejectsAmbiguousOrForeignMetadata(t *testing.T) {
	document, err := unitOperationRegistry(t).OpenAPI("Synthetic fixture", "1")
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(map[string]any){
		func(root map[string]any) { root["openapi"] = "2.0" },
		func(root map[string]any) { root["paths"] = map[string]any{} },
		func(root map[string]any) {
			root["paths"].(map[string]any)["https://outside.example/v1/read"] = map[string]any{}
		},
		func(root map[string]any) { root["paths"].(map[string]any)["/v1/../escape"] = map[string]any{} },
		func(root map[string]any) {
			op := root["paths"].(map[string]any)["/v1/writes"].(map[string]any)["post"].(map[string]any)
			op["operationId"] = "readFixture"
		},
		func(root map[string]any) {
			op := root["paths"].(map[string]any)["/v1/writes"].(map[string]any)["post"].(map[string]any)
			op["x-chartworks-auth"] = "local_admin"
		},
		func(root map[string]any) {
			op := root["paths"].(map[string]any)["/v1/writes"].(map[string]any)["post"].(map[string]any)
			op["x-chartworks-max-body-bytes"] = 0
		},
		func(root map[string]any) {
			op := root["paths"].(map[string]any)["/v1/writes"].(map[string]any)["post"].(map[string]any)
			delete(op, "requestBody")
		},
		func(root map[string]any) {
			op := root["paths"].(map[string]any)["/v1/writes"].(map[string]any)["post"].(map[string]any)
			delete(op, "responses")
		},
		func(root map[string]any) {
			op := root["paths"].(map[string]any)["/v1/reads/{id}"].(map[string]any)["get"].(map[string]any)
			delete(op, "parameters")
		},
		func(root map[string]any) {
			op := root["paths"].(map[string]any)["/v1/reads/{id}"].(map[string]any)["get"].(map[string]any)
			op["parameters"] = []any{map[string]any{"name": "id", "in": "cookie", "required": true, "schema": map[string]any{"type": "string"}}}
		},
	}
	for i, mutate := range mutations {
		var value map[string]any
		if json.Unmarshal(document, &value) != nil {
			t.Fatal("fixture JSON")
		}
		mutate(value)
		bad, _ := json.Marshal(value)
		if _, err = ParseOperations(bad); !errors.Is(err, ErrInvalidCatalog) {
			t.Errorf("mutation %d accepted: %v", i, err)
		}
	}
	for _, bad := range [][]byte{nil, []byte(`{"paths":{},"paths":{}}`), []byte(strings.Repeat("x", operationCatalogLimit+1))} {
		if _, err = ParseOperations(bad); !errors.Is(err, ErrInvalidCatalog) {
			t.Fatal("unbounded/ambiguous catalog", err)
		}
	}
}

func TestOperationMatrixMatchesBothSuppliedAudiences(t *testing.T) {
	var mode atomic.Int64
	client, server, _ := unitOperationServer(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mcp" || r.Header.Get("Authorization") != "Bearer synthetic-mcp" || r.Header.Get("Mcp-Protocol-Version") == "" {
			t.Error("MCP audience or mount lost")
		}
		id, action := "readFixture", "ops.read"
		if mode.Load() == 1 {
			id = "unbuiltReport"
		}
		if mode.Load() == 2 {
			action = "ops.maintain"
		}
		if mode.Load() == 3 {
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32603}}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"tools": []any{map[string]any{"name": "read_fixture", "_meta": map[string]any{"chartworks/operation": id, "chartworks/action": action, "chartworks/effect": "read", "chartworks/audit": "unit.metadata"}}}}})
	})
	mcp, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "synthetic-mcp", nil })
	if err != nil {
		t.Fatal(err)
	}
	rows, err := client.OperationMatrix(t.Context(), mcp)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.ID == "readFixture" {
			found = row.MCPTool == "read_fixture" && row.SDKMethod == "Invoke"
		}
	}
	if !found {
		t.Fatal("missing generated surface link")
	}
	if _, err = client.OperationMatrix(t.Context(), nil); err != nil {
		t.Fatal("optional MCP lookup", err)
	}
	for _, bad := range []int64{1, 2, 3} {
		mode.Store(bad)
		if _, err = client.OperationMatrix(t.Context(), mcp); !errors.Is(err, ErrInvalidCatalog) {
			t.Fatal("MCP contract drift accepted", bad, err)
		}
	}
}

func TestInvocationRejectsMalformedResponseAndDeadline(t *testing.T) {
	client, _, _ := unitOperationServer(t, "", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"wrong":"PRIVATE"}`) })
	if _, err := client.Invoke(t.Context(), "readFixture", CallOptions{ResourceID: "item"}); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("response schema ignored", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Invoke(ctx, "readFixture", CallOptions{ResourceID: "item"}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	client.http.Timeout = time.Millisecond
	client.token = func(ctx context.Context) (string, error) { <-ctx.Done(); return "", errors.New("PRIVATE_PROVIDER") }
	if _, err := client.Operations(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("provider deadline lost", err)
	}
}
