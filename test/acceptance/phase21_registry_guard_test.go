package acceptance

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
)

func verifyPhase21RegisteredDenials(t *testing.T) {
	t.Helper()
	fixture := newTokenFixture(t)
	registry := phase21Registry(t)
	var reached atomic.Int64
	terminal := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached.Add(1); w.WriteHeader(http.StatusNoContent) })
	guarded := api.Guard(fixture.verifier, registry, terminal)
	bare := fixture.sign(t, fixture.claims("registered-tenant", "reader", nil), nil)
	seenNLQ, seenBYO, seenCharts, seenBlocks := false, false, false, false
	for _, d := range registry.Definitions() {
		if d.Public {
			continue
		}
		path := strings.ReplaceAll(d.Path, "{id}", "missing")
		bareClaims := fixture.claims("registered-tenant", "reader", nil)
		if d.Surface == auth.MCP {
			bareClaims["aud"] = fixture.cfg.MCPAudience()
		}
		operationBare := fixture.sign(t, bareClaims, nil)
		seenNLQ = seenNLQ || d.ID == "routeNLQ"
		seenBYO = seenBYO || d.ID == "submitSQL"
		seenCharts = seenCharts || d.ID == "chartCatalog"
		seenBlocks = seenBlocks || d.ID == "createBlock"
		before := reached.Load()
		for _, test := range []struct {
			token  string
			status int
		}{{"", 401}, {operationBare, 403}, {"invalid-bearer", 401}} {
			response := callProtected(t, guarded, d.Method, path, test.token, "{not JSON}", nil)
			if response.Code != test.status {
				t.Fatalf("registered denial %s %s: %d %s", d.Method, path, response.Code, response.Body.String())
			}
		}
		if reached.Load() != before {
			t.Fatal("registered denial reached a domain handler")
		}
		goodClaims := fixture.claims("registered-tenant", "reader", []string{d.Action})
		if d.Surface == auth.MCP {
			goodClaims["aud"] = fixture.cfg.MCPAudience()
		}
		good := fixture.sign(t, goodClaims, nil)
		response := callProtected(t, guarded, d.Method, path, good, "", nil)
		if response.Code != 204 || reached.Load() != before+1 {
			t.Fatalf("declared action not consumed: %s %d", d.ID, response.Code)
		}
		// The terminal is only a dispatch probe. Actual resource isolation remains
		// exercised by each domain's real service/HTTP/store acceptance cases.
	}
	if !seenNLQ || !seenBYO || !seenCharts || !seenBlocks {
		t.Fatal("cumulative registry omitted an implemented domain")
	}
	before := reached.Load()
	if response := callProtected(t, guarded, "POST", "/v1/unregistered", bare, "", nil); response.Code != 404 {
		t.Fatal("unregistered handler admitted")
	}
	if response := callProtected(t, guarded, "TRACE", "/v1/charts/catalog", bare, "", nil); response.Code != 405 {
		t.Fatal("unsupported method did not fail safely")
	}
	if reached.Load() != before {
		t.Fatal("unknown/method-denied route dispatched")
	}
	if response := callProtected(t, guarded, "GET", "/healthz", "", "", nil); response.Code != 204 {
		t.Fatal("explicit public route acquired bearer requirement")
	}
	for _, handler := range []http.Handler{api.Guard(nil, registry, terminal), api.Guard(fixture.verifier, nil, terminal), api.Guard(fixture.verifier, registry, nil)} {
		if response := callProtected(t, handler, "GET", "/v1/charts/catalog", bare, "", nil); response.Code != 404 {
			t.Fatal("incomplete guard installed")
		}
	}
}

func TestPhase21CumulativeRegistryGuard(t *testing.T) { verifyPhase21RegisteredDenials(t) }
