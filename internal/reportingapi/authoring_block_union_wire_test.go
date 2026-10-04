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

func TestAuthoringBlockUnionMountedHTTPMCP(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwksBytes, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "use": "sig", "alg": "ES256", "kid": "fixture", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}}})
	jwks := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(jwksBytes) }))
	t.Cleanup(jwks.Close)
	cfg := config.Defaults().Auth
	cfg.Issuer, cfg.JWKSURL, cfg.Audience = jwks.URL+"/issuer", jwks.URL+"/jwks", ""
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	verifier, err := auth.New(cfg, jwks.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(verifier.Close)
	token := func(audience string) string {
		now := time.Now().Unix()
		scopes := []string{"mcp.use", "reporting.read", "reporting.write", "reporting.preview", "charts.bind", "cw.tenant.read:tenant", "cw.tenant.write:tenant", "cw.block.read:source", "cw.block.write:source", "cw.block.preview:source", "cw.block.write:copy"}
		claims := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iss": cfg.Issuer, "aud": audience, "sub": "actor", "tenant": "tenant", "user": "actor", "session": "session", "iat": now, "nbf": now - 1, "exp": now + 300, "scopes": scopes})
		claims.Header["kid"] = "fixture"
		value, err := claims.SignedString(key)
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
	bindings, err := authoringBlockMCPBindings(registry, service)
	if err != nil {
		t.Fatal(err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	server, err := mcpserver.New(verifier, mcpRegistry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := func(handler http.Handler, path, body, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1"+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+bearer)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, suffix := range []string{"block_mapping", "block_copy"} {
		for _, tc := range []struct {
			name, member, code, outcome string
			status                      int
		}{
			{"presentation-reaches-domain", `,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":0}}]}`, "unavailable", "unknown", 503},
			{"neither", ``, "invalid_request", "not_started", 400},
			{"null", `,"presentation":null`, "invalid_request", "not_started", 400},
			{"both", `,"mapping":{},"presentation":{"version":1,"edits":[{"column":"value","reset":["display_label"]}]}`, "invalid_request", "not_started", 400},
			{"null-plus-patch", `,"mapping":null,"presentation":{"version":1,"edits":[{"column":"value","reset":["display_label"]}]}`, "invalid_request", "not_started", 400},
			{"unknown", `,"presentation":{"version":1,"edits":[{"column":"value","set":{"formatter":"private-canary"}}]}`, "invalid_request", "not_started", 400},
			{"nested-null", `,"presentation":{"version":1,"edits":[{"column":"value","set":{"fraction_digits":null}}]}`, "invalid_request", "not_started", 400},
		} {
			t.Run(suffix+"/"+tc.name, func(t *testing.T) {
				body := presentationUnionJSON(tc.member, suffix == "block_copy")
				w := request(httpHandler, AuthoringPath+suffix, body, token(cfg.HTTPAudience()))
				var fault struct {
					Error string `json:"error"`
				}
				if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &fault) != nil || fault.Error != tc.code || strings.Contains(w.Body.String(), "canary") {
					t.Fatal("HTTP union boundary", w.Code, w.Body.String())
				}
				rpc := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"reporting_authoring_` + suffix + `_v1","arguments":` + body + `}}`
				w = request(server.Handler(), mcpserver.Path, rpc, token(cfg.MCPAudience()))
				var result struct {
					Result struct {
						IsError    bool `json:"isError"`
						Structured struct {
							Error mcpserver.Fault `json:"error"`
						} `json:"structuredContent"`
					} `json:"result"`
				}
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.Result.IsError || result.Result.Structured.Error.Code != tc.code || result.Result.Structured.Error.Outcome != tc.outcome || strings.Contains(w.Body.String(), "canary") {
					t.Fatal("MCP union boundary", w.Code, w.Body.String())
				}
			})
		}
	}
}
