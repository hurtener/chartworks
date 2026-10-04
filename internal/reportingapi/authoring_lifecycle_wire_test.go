package reportingapi

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestAuthoringLifecycleHTTPMCPNativeAuthorityAndSafeFaults(t *testing.T) {
	// Synthetic trusted-issuer fixture exercises both actual wire adapters. No
	// repository/source/model is installed: denials must precede domain I/O.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "use": "sig", "alg": "ES256", "kid": "fixture", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}}})
	jwks := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(doc) }))
	t.Cleanup(jwks.Close)
	cfg := config.Defaults().Auth
	cfg.Issuer, cfg.JWKSURL, cfg.Audience = jwks.URL+"/issuer", jwks.URL+"/jwks", ""
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	verifier, err := auth.New(cfg, jwks.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(verifier.Close)
	token := func(audience string, scopes []string) string {
		now := time.Now().Unix()
		signed := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iss": cfg.Issuer, "aud": audience, "sub": "actor", "tenant": "tenant", "user": "actor", "session": "session", "iat": now, "nbf": now - 1, "exp": now + 300, "scopes": scopes})
		signed.Header["kid"] = "fixture"
		value, err := signed.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	service := &reporting.Authoring{}
	httpHandler := AuthoringHandler(verifier, service, http.NotFoundHandler())
	registry, err := AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := authoringLifecycleMCPBindings(registry, service)
	if err != nil {
		t.Fatal(err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	mcpServer, err := mcpserver.New(verifier, mcpRegistry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	requests := lifecycleSchemaRequests()
	cases := []struct {
		name          string
		entry         int
		input         any
		scopes        []string
		status        int
		code, outcome string
	}{
		{"inspect-needs-report-write", 0, reporting.AuthoringLifecycleRequest{Report: "report", Revision: 1}, []string{"reporting.read", "cw.report.read:report"}, 403, "forbidden", "unknown"},
		{"publish-entry-needs-publish", 1, requests[1], []string{"reporting.read", "cw.block.read:chart"}, 403, "forbidden", "not_started"},
		{"publish-needs-independent-read", 1, requests[1], []string{"reporting.publish", "cw.block.publish:chart"}, 403, "forbidden", "unknown"},
		{"rebind-needs-independent-read", 2, requests[2], []string{"reporting.write", "cw.report.write:report"}, 403, "forbidden", "unknown"},
		{"review-needs-exact-write", 3, requests[3], []string{"reporting.write", "cw.report.write:other"}, 404, "not_found", "unknown"},
		{"publish-needs-native-publish", 3, reporting.AuthoringReportTransitionRequest{Report: "report", Revision: 2, ExpectedVersion: 4, Operation: "publish"}, []string{"reporting.write", "cw.report.write:report"}, 403, "forbidden", "unknown"},
		{"reject-needs-native-publish", 3, reporting.AuthoringReportTransitionRequest{Report: "report", Revision: 2, ExpectedVersion: 4, Operation: "reject", Note: "Explicit note"}, []string{"reporting.write", "cw.report.write:report"}, 403, "forbidden", "unknown"},
		{"publish-only-cannot-bypass-editor-entry", 3, reporting.AuthoringReportTransitionRequest{Report: "report", Revision: 2, ExpectedVersion: 4, Operation: "publish"}, []string{"reporting.publish", "cw.report.publish:report"}, 403, "forbidden", "not_started"},
		{"reject-requires-note", 3, reporting.AuthoringReportTransitionRequest{Report: "report", Revision: 2, ExpectedVersion: 4, Operation: "reject"}, []string{"reporting.write", "reporting.publish", "cw.report.publish:report"}, 400, "invalid_request", "unknown"},
		{"hidden-publish-operation-rejected", 3, reporting.AuthoringReportTransitionRequest{Report: "report", Revision: 2, ExpectedVersion: 4, Operation: "Publish"}, []string{"reporting.write"}, 400, "invalid_request", "not_started"},
		{"wildcard-reach-rejected", 0, requests[0], []string{"reporting.read", "cw.block.read:*"}, 403, "forbidden", "unknown"},
	}
	for i, entry := range authoringLifecycleEntries(nil) {
		var injected map[string]any
		raw, _ := json.Marshal(requests[i])
		_ = json.Unmarshal(raw, &injected)
		injected["scopes"] = []string{"private-authority-canary"}
		cases = append(cases, struct {
			name          string
			entry         int
			input         any
			scopes        []string
			status        int
			code, outcome string
		}{"injection-" + entry.definition.ID, i, injected, []string{entry.definition.Action}, 400, "invalid_request", "not_started"})
	}
	entries := authoringLifecycleEntries(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.input)
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+entries[tc.entry].definition.Path, strings.NewReader(string(body)))
			r.Header.Set("Authorization", "Bearer "+token(cfg.HTTPAudience(), tc.scopes))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			httpHandler.ServeHTTP(w, r)
			var fault struct {
				Error string `json:"error"`
			}
			if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &fault) != nil || fault.Error != tc.code || strings.Contains(w.Body.String(), "canary") {
				t.Fatal("HTTP authority/schema fault changed", w.Code, w.Body.String())
			}
			rpc, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": entries[tc.entry].definition.ID, "arguments": tc.input}})
			r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+mcpserver.Path, strings.NewReader(string(rpc)))
			r.Header.Set("Authorization", "Bearer "+token(cfg.MCPAudience(), append([]string{"mcp.use"}, tc.scopes...)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			w = httptest.NewRecorder()
			mcpServer.Handler().ServeHTTP(w, r)
			var wire struct {
				Result struct {
					IsError    bool `json:"isError"`
					Structured struct {
						Error mcpserver.Fault `json:"error"`
					} `json:"structuredContent"`
				} `json:"result"`
			}
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &wire) != nil || !wire.Result.IsError || wire.Result.Structured.Error.Code != tc.code || wire.Result.Structured.Error.Outcome != tc.outcome || strings.Contains(w.Body.String(), "canary") {
				t.Fatal("MCP authority/schema/outcome fault changed", w.Code, w.Body.String())
			}
		})
	}
}
