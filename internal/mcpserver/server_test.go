package mcpserver

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testInput struct {
	Item string `json:"item"`
}
type testOutput struct {
	Tenant  string `json:"tenant"`
	User    string `json:"user"`
	Session string `json:"session"`
	Item    string `json:"item"`
	Exact   string `json:"exact"`
}
type testAuthority struct {
	verifier *auth.Verifier
	cfg      config.Auth
	key      *ecdsa.PrivateKey
	clock    atomic.Int64
}

func newAuthority(t testing.TB) *testAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "use": "sig", "alg": "ES256", "kid": "fixture", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}}})
	jwks := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(doc) }))
	t.Cleanup(jwks.Close)
	f := &testAuthority{key: key, cfg: config.Defaults().Auth}
	f.clock.Store(time.Now().Unix())
	f.cfg.Issuer = jwks.URL + "/issuer"
	f.cfg.JWKSURL = jwks.URL + "/jwks"
	f.cfg.Audience = ""
	f.cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	f.verifier, err = auth.New(f.cfg, jwks.Client(), func() time.Time { return time.Unix(f.clock.Load(), 0) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.verifier.Close)
	return f
}
func (f *testAuthority) token(t testing.TB, tenant, user, audience string, scopes ...string) string {
	t.Helper()
	now := f.clock.Load()
	claims := jwt.MapClaims{"iss": f.cfg.Issuer, "aud": audience, "sub": user, "tenant": tenant, "user": user, "session": "session-" + user, "iat": now, "nbf": now - 1, "exp": now + 300, "scopes": scopes}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "fixture"
	value, err := token.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func testRegistry(t testing.TB, call func(context.Context, identity.Envelope, testInput) (testOutput, error)) *Registry {
	t.Helper()
	in, err := api.SchemaFor("fixtureInput", reflect.TypeFor[testInput](), false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := api.SchemaFor("fixtureOutput", reflect.TypeFor[testOutput](), true)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := api.New([]api.Definition{{Operation: api.Operation{Method: "POST", Path: "/v1/fixture", Action: "fixture.read", Effect: "metadata_read"}, ID: "fixtureRead", Summary: "Read an authorized synthetic unit fixture", ResourceLoader: "synthetic exact dataset reach", Audit: "read_only_no_domain_audit", MaxBodyBytes: 1024, Request: in, Response: out, Errors: []api.ErrorResponse{{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 403, Code: "forbidden"}, {Status: 404, Code: "not_found"}, {Status: 503, Code: "unavailable"}}}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := Bind(reg, "fixtureRead", "fixture_read", "discovery", "Read one synthetic unit-test item under exact signed dataset reach.", call, func(err error) Fault {
		if errors.Is(err, access.ErrNotFound) {
			return Fault{Code: "not_found"}
		}
		if errors.Is(err, access.ErrForbidden) {
			return Fault{Code: "forbidden"}
		}
		return Fault{Code: "unavailable"}
	})
	if err != nil {
		t.Fatal("bind", err)
	}
	binding, err = WithResource(binding, "chartworks://fixtures/{item}")
	if err != nil {
		t.Fatal("resource", err)
	}
	registry, err := NewRegistry([]Binding{binding})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
func fixtureCall(ctx context.Context, e identity.Envelope, in testInput) (testOutput, error) {
	if err := access.Require(e, "fixture.read", access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: in.Item}); err != nil {
		return testOutput{}, err
	}
	return testOutput{e.Tenant(), e.User(), e.Session(), in.Item, "900719925474099312345.123456789"}, ctx.Err()
}
func newTestServer(t testing.TB, f *testAuthority, call func(context.Context, identity.Envelope, testInput) (testOutput, error), settings config.MCP) *Server {
	t.Helper()
	s, err := New(f.verifier, testRegistry(t, call), settings, []string{"https://console.example"})
	if err != nil {
		t.Fatal("server", err)
	}
	return s
}
func rpc(t testing.TB, h http.Handler, token, body string, edit func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "http://127.0.0.1"+Path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if edit != nil {
		edit(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const readRPC = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fixture_read","arguments":{"item":"allowed"}}}`

func TestNetworkAndInProcessShareAuthority(t *testing.T) {
	f := newAuthority(t)
	s := newTestServer(t, f, fixtureCall, config.DefaultMCP())
	token := f.token(t, "one", "alice", f.cfg.MCPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:allowed")
	h := s.Handler()
	reg, err := HTTPRegistry(config.DefaultMCP())
	if err != nil {
		t.Fatal(err)
	}
	h = api.Guard(f.verifier, reg, h)
	cases := []string{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, readRPC, `{"jsonrpc":"2.0","id":3,"method":"resources/list"}`, `{"jsonrpc":"2.0","id":4,"method":"resources/templates/list"}`, `{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"chartworks://fixtures/allowed"}}`, `{"jsonrpc":"2.0","id":6,"method":"ping"}`}
	for _, body := range cases {
		w := rpc(t, h, token, body, nil)
		var wire struct {
			Error  json.RawMessage `json:"error"`
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		if json.Unmarshal(w.Body.Bytes(), &wire) != nil || w.Code != 200 || wire.Error != nil || wire.Result.IsError {
			t.Fatalf("RPC %s: %d %s", body, w.Code, w.Body.String())
		}
		if w.Header().Get("Mcp-Session-Id") != "" {
			t.Fatal("unexpected session credential")
		}
	}
	w := rpc(t, h, token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	if w.Code != 202 || w.Body.Len() != 0 {
		t.Fatal("notification", w.Code, w.Body.String())
	}
	client, err := s.Client(func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CallTool(t.Context(), "fixture_read", json.RawMessage(`{"item":"allowed"}`))
	if err != nil || result.IsError {
		t.Fatal("in process", err, result)
	}
	if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "900719925474099312345.123456789") {
		t.Fatal("exact value lost")
	}
	for _, bad := range []string{f.token(t, "one", "alice", f.cfg.HTTPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:allowed"), f.token(t, "one", "alice", f.cfg.MCPAudience(), "fixture.read", "cw.dataset.query:allowed"), "invalid"} {
		w = rpc(t, h, bad, readRPC, nil)
		if w.Code != 401 && w.Code != 403 {
			t.Fatal("authority bypass", w.Code, w.Body.String())
		}
	}
	denied := rpc(t, h, token, strings.ReplaceAll(readRPC, "allowed", "foreign"), nil)
	if denied.Code != 200 || !strings.Contains(denied.Body.String(), `"code":"not_found"`) {
		t.Fatal("resource bypass", denied.Code, denied.Body.String())
	}
}
func TestConcurrentPerRequestIdentity(t *testing.T) {
	f := newAuthority(t)
	s := newTestServer(t, f, fixtureCall, config.DefaultMCP())
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("user%d", i)
			tenant := fmt.Sprintf("tenant%d", i)
			token := f.token(t, tenant, name, f.cfg.MCPAudience(), "mcp.use", "fixture.read", "cw.dataset.query:allowed")
			client, err := s.Client(func(context.Context) (string, error) { return token, nil })
			if err != nil {
				t.Error(err)
				return
			}
			for range 3 {
				result, e := client.CallTool(t.Context(), "fixture_read", json.RawMessage(`{"item":"allowed"}`))
				if e != nil || result.IsError {
					t.Error("call", e)
					return
				}
				raw, _ := json.Marshal(result.StructuredContent)
				if !bytes.Contains(raw, []byte(`"tenant":"`+tenant+`"`)) || !bytes.Contains(raw, []byte(`"user":"`+name+`"`)) {
					t.Error("cross talk", string(raw))
				}
				w := rpc(t, s.Handler(), token, readRPC, nil)
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"tenant":"`+tenant+`"`) {
					t.Error("network cross talk", w.Code, w.Body.String())
				}
			}
		}()
	}
	wg.Wait()
}
