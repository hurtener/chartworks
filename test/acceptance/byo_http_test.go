package acceptance

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/nlqapi"
)

// Exercise transport denials with the actual verifier and installed service,
// not just direct service calls or assertions against the declared manifest.
func TestBYOHTTPAuthorityAndClosedTransport(t *testing.T) {
	f := newPhase18Fixture(t)
	counter := &phase19CountingExecutor{delegate: f.f.executor}
	service, _, _ := phase19Service(t, f, config.DefaultQueryBundles(), nil, f.service, counter)
	_, token := phase19Authority(t, f, f.e.Tenant(), f.e.User(), "byo-http", phase19Scopes())
	_, noAction := phase19Authority(t, f, f.e.Tenant(), f.e.User(), "byo-http", nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	handler := nlqapi.BYOHandler(f.model.token.verifier, service, next)
	before := f.model.requests.Load()
	for _, tc := range []struct {
		name, method, path, token, body string
		headers                         map[string]string
		status                          int
	}{
		{"missing-bearer", "POST", "/v1/nlq/sql", "", "{}", nil, 401},
		{"missing-action-before-body-decode", "POST", "/v1/nlq/sql", noAction, "not JSON", nil, 403},
		{"unsupported-method", "GET", "/v1/nlq/sql", token, "", nil, 405},
		{"query-string", "POST", "/v1/nlq/sql?rows=1000", token, "{}", nil, 400},
		{"encoded-path", "POST", "/v1/nlq/%73ql", token, "{}", nil, 400},
		{"content-encoding", "POST", "/v1/nlq/sql", token, "{}", map[string]string{"Content-Encoding": "gzip"}, 400},
		{"incomplete-context-request", "POST", "/v1/nlq/contexts", token, "{}", nil, 400},
		{"incomplete-lookup-request", "POST", "/v1/nlq/contexts/read", token, "{}", nil, 400},
		{"unowned-path-falls-through", "POST", "/another-service", "", "", nil, 418},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := callProtected(t, handler, tc.method, tc.path, tc.token, tc.body, tc.headers)
			if response.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
		})
	}
	// An opaque ID with valid syntax is not a capability. The wire response must
	// request replanning without disclosing which private coordinate was absent.
	reference := fmt.Sprintf(`{"schema_version":1,"bundle_id":%q,"context":%q}`, strings.Repeat("a", 64), f.context)
	response := callProtected(t, handler, "POST", "/v1/nlq/contexts/read", token, reference, nil)
	if response.Code != 409 || !strings.Contains(response.Body.String(), "replan_required") || strings.Contains(response.Body.String(), f.context) {
		t.Fatal("missing reference did not produce a private replan response", response.Code, response.Body.String())
	}
	if counter.calls.Load() != 0 || f.model.requests.Load() != before {
		t.Fatal("transport denial reached execution or inference")
	}
	// Disabling context construction must neither claim that operation nor hide
	// the independently installed lookup/submission or unrelated service routes.
	offline, _, _ := phase19Service(t, f, config.DefaultQueryBundles(), nil, nil, counter)
	offlineHandler := nlqapi.BYOHandler(f.model.token.verifier, offline, next)
	if response := callProtected(t, offlineHandler, "POST", "/v1/nlq/contexts", token, "{}", nil); response.Code != 418 {
		t.Fatal("offline constructor claimed an uninstalled operation", response.Code)
	}
	if response := callProtected(t, offlineHandler, "POST", "/v1/nlq/contexts/read", token, reference, nil); response.Code != 409 {
		t.Fatal("offline lookup disappeared", response.Code)
	}
	disabled := nlqapi.BYOHandler(f.model.token.verifier, nil, next)
	if response := callProtected(t, disabled, "POST", "/v1/nlq/sql", "", "", nil); response.Code != 418 {
		t.Fatal("disabled service intercepted another handler", response.Code)
	}
}
