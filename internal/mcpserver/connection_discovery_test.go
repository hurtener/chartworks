package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestPenguiConnectionDiscoveryCannotInvokeOrRead(t *testing.T) {
	f := newAuthority(t)
	var calls atomic.Int64
	s := newTestServer(t, f, func(ctx context.Context, e identity.Envelope, in testInput) (testOutput, error) {
		calls.Add(1)
		return fixtureCall(ctx, e, in)
	}, config.DefaultMCP())
	registry, err := HTTPRegistry(config.DefaultMCP())
	if err != nil {
		t.Fatal(err)
	}
	handler := api.Guard(f.verifier, registry, s.Handler())
	token := f.token(t, "one", "svc:coordinator", f.cfg.MCPAudience(), "capability:connect")
	for _, tc := range []struct{ method, params, want string }{
		{"initialize", `{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}`, `"serverInfo"`},
		{"tools/list", `{}`, `"fixture_read"`},
		{"resources/list", `{}`, `"resources"`},
		{"resources/templates/list", `{}`, `"chartworks://fixtures/{item}"`},
		{"ping", `{}`, `"result"`},
	} {
		t.Run(tc.method, func(t *testing.T) {
			w := rpc(t, handler, token, `{"jsonrpc":"2.0","id":1,"method":"`+tc.method+`","params":`+tc.params+`}`, nil)
			var response struct {
				Error json.RawMessage `json:"error"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Error) != 0 || !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	// Optional discovery must return the protocol's unsupported-method signal,
	// not an HTTP request failure. No prompt capability is advertised.
	w := rpc(t, handler, token, `{"jsonrpc":"2.0","id":1,"method":"prompts/list","params":{}}`, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"code":-32601`) {
		t.Fatalf("unsupported prompt discovery: %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{readRPC, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"chartworks://fixtures/allowed"}}`, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"ui://chartworks/report-app/v1"}}`} {
		w := rpc(t, handler, token, body, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"message":"forbidden"`) {
			t.Fatalf("connection token read/invoke: %d %s", w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct {
		user, aud string
		scopes    []string
	}{
		{"alice", f.cfg.MCPAudience(), []string{"capability:connect"}},
		{"svc:coordinator", f.cfg.MCPAudience(), []string{"capability:connect", "fixture.read"}},
		{"svc:coordinator", f.cfg.HTTPAudience(), []string{"capability:connect"}},
		{"svc:coordinator", f.cfg.MCPAudience(), []string{"fixture.read"}},
	} {
		w := rpc(t, handler, f.token(t, "one", tc.user, tc.aud, tc.scopes...), `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, nil)
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("invalid connection profile admitted: %d", w.Code)
		}
	}
	f.clock.Add(400)
	if w := rpc(t, handler, token, readRPC, nil); w.Code != 401 {
		t.Fatalf("expired connection admitted: %d", w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("static discovery reached a domain handler", calls.Load())
	}
}
