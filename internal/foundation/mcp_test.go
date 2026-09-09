package foundation

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/mcpserver"
)

func TestMCPAssemblyFeatureAndGroupBoundaries(t *testing.T) {
	v := config.Defaults()
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(418) })
	registry, handler, err := mountMCP(v, nil, nil, nil, nil, nil, nil, nil, fallback)
	if err != nil || registry != nil || handler == nil {
		t.Fatal("disabled MCP altered composition", err)
	}
	v.Features.MCP = true
	if _, _, err := mountMCP(v, nil, nil, nil, nil, nil, nil, nil, fallback); err == nil {
		t.Fatal("advertised missing services")
	}
	v.Auth.Issuer = "https://issuer.example.test"
	v.Auth.JWKSURL = "https://issuer.example.test/jwks"
	v.Auth.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp", Jobs: "chartworks:execution"}
	verifier, err := auth.New(v.Auth, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	chart, err := chartservice.New(v.Charts.ServiceOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	v.MCP.Groups = []string{"charts"}
	registry, handler, err = mountMCP(v, verifier, nil, nil, nil, nil, chart, nil, fallback)
	if err != nil {
		t.Fatal(err)
	}
	definition, _, ok := registry.Match(http.MethodPost, mcpserver.Path)
	if !ok || definition.Surface != auth.MCP || definition.Action != "mcp.use" {
		t.Fatal("missing audience or action")
	}
	for _, test := range []struct {
		path   string
		status int
	}{{mcpserver.Path, 401}, {"/outside-mcp", 418}} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, test.path, nil))
		if w.Code != test.status {
			t.Fatal(test.path, w.Code)
		}
	}
	v.MCP.Groups = []string{"discovery"}
	if _, _, err = mountMCP(v, verifier, nil, nil, nil, nil, chart, nil, fallback); err == nil {
		t.Fatal("empty enabled group advertised")
	}
	v.MCP.Groups = []string{"unknown"}
	if _, _, err = mountMCP(v, verifier, nil, nil, nil, nil, chart, nil, fallback); err == nil {
		t.Fatal("unknown group advertised")
	}
	v.MCP.Groups = []string{"charts"}
	v.Server.CORSAllowlist = []string{"*"}
	if _, _, err = mountMCP(v, verifier, nil, nil, nil, nil, chart, nil, fallback); err == nil {
		t.Fatal("unsafe MCP origin accepted")
	}
}
