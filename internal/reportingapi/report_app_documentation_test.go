package reportingapi

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/docs"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/staticdocs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReportAppDocumentationMountedHTTPMCPParity(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwksBytes, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "use": "sig", "alg": "ES256", "kid": "fixture", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}}})
	jwks := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(jwksBytes) }))
	t.Cleanup(jwks.Close)
	cfg := config.Defaults().Auth
	cfg.Issuer = jwks.URL + "/issuer"
	cfg.JWKSURL = jwks.URL + "/jwks"
	cfg.Audience = ""
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	verifier, err := auth.New(cfg, jwks.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(verifier.Close)
	token := func(audience string, scopes []string, tenant string, expired bool) string {
		now := time.Now().Unix()
		exp := now + 300
		if expired {
			exp = now - 300
			now -= 600
		}
		value := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iss": cfg.Issuer, "aud": audience, "sub": "actor", "tenant": tenant, "user": "actor", "session": "session", "iat": now, "nbf": now - 1, "exp": exp, "scopes": scopes})
		value.Header["kid"] = "fixture"
		signed, err := value.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	service := &reporting.Authoring{}
	app, err := mcpserver.NewAppResource("ui://chartworks/report-app/v1", "Synthetic report app", "Synthetic data-free resource for documentation wire tests", "<!doctype html><html><head><title>Synthetic resource</title></head><body><main>Synthetic documentation wire registration proof.</main></body></html>")
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := ReportAppBootstrapMCPBindings(service, app)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	server, err := mcpserver.New(verifier, registry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	httpHandler := ReportAppBootstrapHandler(verifier, service, http.NotFoundHandler())
	catalog, err := docs.ReportAuthoring()
	if err != nil {
		t.Fatal(err)
	}
	request := func(handler http.Handler, path string, body any, bearer string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://127.0.0.1"+path, strings.NewReader(string(raw)))
		r.Header.Set("Authorization", "Bearer "+bearer)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	rpc := func(method string, params any, bearer string) *httptest.ResponseRecorder {
		return request(server.Handler(), mcpserver.Path, map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}, bearer)
	}
	goodHTTP := token(cfg.HTTPAudience(), []string{"reporting.read"}, "tenant", false)
	goodMCP := token(cfg.MCPAudience(), []string{"mcp.use", "reporting.read"}, "tenant", false)
	listed := rpc("resources/list", map[string]any{}, goodMCP)
	var listing struct {
		Result mcp.ListResourcesResult `json:"result"`
	}
	if listed.Code != 200 || json.Unmarshal(listed.Body.Bytes(), &listing) != nil || len(listing.Result.Resources) != 13 {
		t.Fatal("mounted resource inventory", listed.Code, listed.Body.String())
	}
	byURI := map[string]*mcp.Resource{}
	for _, r := range listing.Result.Resources {
		byURI[r.URI] = r
	}
	for _, ref := range catalog.References() {
		advertised := byURI[ref.URI]
		if advertised == nil || advertised.MIMEType != ref.MIMEType || advertised.Description != ref.Description || strings.Contains(listed.Body.String(), "# Manual report authoring guide") {
			t.Fatal("full docs missing or dumped in discovery", ref.URI)
		}
		h := request(httpHandler, "/v1/reporting/authoring/v1/documentation", ReportAppDocumentationRequest{URI: ref.URI}, goodHTTP)
		var document staticdocs.Document
		if h.Code != 200 || json.Unmarshal(h.Body.Bytes(), &document) != nil || !reflect.DeepEqual(document.Reference, ref) {
			t.Fatal("HTTP document", h.Code, h.Body.String())
		}
		m := rpc("resources/read", map[string]any{"uri": ref.URI}, goodMCP)
		var result struct {
			Result mcp.ReadResourceResult `json:"result"`
		}
		if m.Code != 200 || json.Unmarshal(m.Body.Bytes(), &result) != nil || len(result.Result.Contents) != 1 || result.Result.Contents[0].Text != document.Text || result.Result.Contents[0].MIMEType != document.Reference.MIMEType {
			t.Fatal("MCP content parity", ref.URI, m.Code, m.Body.String())
		}
		meta, _ := json.Marshal(result.Result.Contents[0].Meta["chartworks/document"])
		var observed staticdocs.Reference
		if json.Unmarshal(meta, &observed) != nil || !reflect.DeepEqual(observed, ref) {
			t.Fatal("digest/reference parity", string(meta))
		}
	}
	// Reading guidance is action-protected, but no document contains tenant state.
	second := token(cfg.HTTPAudience(), []string{"reporting.read"}, "other", false)
	firstRef := catalog.References()[0]
	a := request(httpHandler, "/v1/reporting/authoring/v1/documentation", ReportAppDocumentationRequest{URI: firstRef.URI}, goodHTTP)
	b := request(httpHandler, "/v1/reporting/authoring/v1/documentation", ReportAppDocumentationRequest{URI: firstRef.URI}, second)
	if a.Body.String() != b.Body.String() {
		t.Fatal("static document depends on caller")
	}
	for _, tc := range []struct {
		name, uri string
		scopes    []string
		status    int
		code      string
		expired   bool
	}{
		{"withdrawn", firstRef.URI, []string{"reporting.write"}, 403, "forbidden", false},
		{"unknown-denied", staticdocs.Namespace + "unknown/v9", []string{}, 403, "forbidden", false},
		{"unknown-version", strings.TrimSuffix(firstRef.URI, "v1") + "v999", []string{"reporting.read"}, 404, "not_found", false},
		{"percent-alias", strings.Replace(firstRef.URI, "workflows", "%77orkflows", 1), []string{"reporting.read"}, 404, "not_found", false},
		{"query", firstRef.URI + "?all=true", []string{"reporting.read"}, 404, "not_found", false},
		{"traversal", staticdocs.Namespace + "../workflows/v1", []string{"reporting.read"}, 404, "not_found", false},
		{"expired", firstRef.URI, []string{"reporting.read"}, 401, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := request(httpHandler, "/v1/reporting/authoring/v1/documentation", ReportAppDocumentationRequest{URI: tc.uri}, token(cfg.HTTPAudience(), tc.scopes, "tenant", tc.expired))
			if h.Code != tc.status {
				t.Fatal("HTTP denial", h.Code, h.Body.String())
			}
			m := rpc("resources/read", map[string]any{"uri": tc.uri}, token(cfg.MCPAudience(), append([]string{"mcp.use"}, tc.scopes...), "tenant", tc.expired))
			if tc.expired {
				if m.Code != 401 {
					t.Fatal(m.Code, m.Body.String())
				}
				return
			}
			var fault struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(m.Body.Bytes(), &fault) != nil || fault.Error.Message != tc.code {
				t.Fatal("MCP denial", m.Code, m.Body.String())
			}
		})
	}
	deniedList := rpc("resources/list", map[string]any{}, token(cfg.MCPAudience(), []string{"mcp.use"}, "tenant", false))
	if strings.Contains(deniedList.Body.String(), staticdocs.Namespace) {
		t.Fatal("denied catalog exposed", deniedList.Body.String())
	}
	injected := request(httpHandler, "/v1/reporting/authoring/v1/documentation", map[string]any{"uri": firstRef.URI, "scopes": []string{"reporting.read"}}, goodHTTP)
	if injected.Code != 400 {
		t.Fatal("closed schema", injected.Code)
	}
	tools := rpc("tools/list", map[string]any{}, goodMCP)
	var toolList struct {
		Result mcp.ListToolsResult `json:"result"`
	}
	if json.Unmarshal(tools.Body.Bytes(), &toolList) != nil || len(toolList.Result.Tools) != 2 {
		t.Fatal("docs added tools", tools.Body.String())
	}
}
