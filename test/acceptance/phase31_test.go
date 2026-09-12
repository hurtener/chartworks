package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/viewerfixtures"
)

// Each criterion invokes its owning provider/component behavior. In particular,
// AC04/AC06/AC08 launch the actual bundled resource in Chromium; a missing
// browser is an acceptance failure, never a skip or a metadata-only substitute.
func TestPhase31(t *testing.T) {
	t.Run("AC01", testPhase31AppsDiscovery)
	t.Run("AC02", testPhase31CatalogAndRetainedReads)
	t.Run("AC03", testPhase31ArtifactStates)
	t.Run("AC04", func(t *testing.T) { viewerfixtures.Run(t, "charts") })
	t.Run("AC05", testPhase31ExplicitExecution)
	t.Run("AC06", func(t *testing.T) { viewerfixtures.Run(t, "interaction") })
	t.Run("AC07", testPhase31ProviderParity)
	t.Run("AC08", func(t *testing.T) {
		t.Run("provider-boundaries", testPhase31ProviderBounds)
		t.Run("actual-component-boundaries", func(t *testing.T) { viewerfixtures.Run(t, "security") })
	})
}

type phase31Fixture struct {
	domain *phase29ExecutionFixture
	service *reporting.Delivery
	registry *api.Registry
	mcp *mcpserver.Server
	handler http.Handler
	wire *httptest.Server
	scopes []string
}

func newPhase31Fixture(t *testing.T, dynamic bool) *phase31Fixture {
	t.Helper()
	d := newPhase29Execution(t, dynamic)
	service, err := reporting.NewDelivery(d.blocks, d.runs, d.documents, d.compositions, d.f.f.db, d.limits.Viewer)
	if err != nil { t.Fatal(err) }
	bindings, err := reportingapi.DeliveryMCPBindings(service, true)
	if err != nil { t.Fatal(err) }
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil { t.Fatal(err) }
	settings := config.DefaultMCP()
	server, err := mcpserver.New(d.f.model.token.verifier, mcpRegistry, settings, []string{"https://console.example"})
	if err != nil { t.Fatal(err) }
	httpRegistry, err := reportingapi.DeliveryRegistry(true)
	if err != nil { t.Fatal(err) }
	transport, err := mcpserver.HTTPRegistry(settings)
	if err != nil { t.Fatal(err) }
	registry, err := api.Compose(httpRegistry, transport)
	if err != nil { t.Fatal(err) }
	domain := reportingapi.DeliveryHandler(d.f.model.token.verifier, service, true, http.NotFoundHandler())
	handler := api.Guard(d.f.model.token.verifier, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mcpserver.Path { server.Handler().ServeHTTP(w, r); return }
		domain.ServeHTTP(w, r)
	}))
	handler = assertRegisteredWireSchemas(t, registry, handler)
	wire := httptest.NewServer(handler)
	t.Cleanup(wire.Close)
	// The transport action is added without exceeding the issuer's signed scope
	// bound. Removed actions concern authoring/maintenance, not execution reach.
	scopes := slices.DeleteFunc(phase29RuntimeScopes(d.execute.Tenant()), func(s string) bool {
		return s == "reporting.retention" || s == "jobs.read" || s == "jobs.cancel" || s == "reporting.preview" || s == "cw.block.preview:*" || s == "cw.report.preview:*" || s == "cw.dashboard.preview:*"
	})
	scopes = append(scopes, "mcp.use")
	if len(scopes) > 32 { t.Fatal("fixture exceeds real signed scope ceiling") }
	return &phase31Fixture{domain:d, service:service, registry:registry, mcp:server, handler:handler, wire:wire, scopes:scopes}
}

func (f *phase31Fixture) token(t testing.TB, user, tenant string, scopes []string, mcpAudience bool) string {
	t.Helper()
	authority := f.domain.f.model.token
	claims := authority.claims(tenant, user, scopes)
	claims["session"] = f.domain.execute.Session()
	if mcpAudience { claims["aud"] = authority.cfg.MCPAudience() }
	return authority.sign(t, claims, nil)
}

func (f *phase31Fixture) client(t testing.TB, scopes []string, mcpAudience bool) *cw.Client {
	t.Helper()
	bearer := f.token(t, f.domain.execute.User(), f.domain.execute.Tenant(), scopes, mcpAudience)
	client, err := cw.New(f.wire.URL, f.wire.Client(), func(context.Context) (string,error) { return bearer,nil })
	if err != nil { t.Fatal(err) }
	return client
}

func (f *phase31Fixture) reader(t *testing.T, block, report string) identity.Envelope {
	t.Helper()
	scopes := []string{"reporting.read", "cw.execution_context.use:"+f.domain.base.Context}
	if block != "" { scopes = append(scopes, "cw.block.read:"+block) }
	if report != "" { scopes = append(scopes, "cw.report.read:"+report) }
	return phase27Actor(t, f.domain.f, "viewer-reader", scopes)
}

func phase31Request(kind, id, key string) reporting.ReportingRunRequest {
	return reporting.ReportingRunRequest{Target:reporting.DeliveryTarget{Kind:kind,ID:id,Revision:1},Key:key,
		Arguments:[]reporting.Argument{},Pages:[]reporting.PageInput{},Outputs:[]string{},Timezone:"UTC",Locale:"en"}
}

func (f *phase31Fixture) run(t *testing.T, kind, id, key string) reporting.ReportingRunResult {
	t.Helper()
	out, err := f.service.Run(t.Context(), f.domain.execute, phase31Request(kind,id,key))
	if err != nil || out.Run == "" || (out.State != "succeeded" && out.State != "completed") { t.Fatalf("actual reporting run: %+v %v",out,err) }
	return out
}

func phase31Wire(t testing.TB, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }
	return b
}
