package reportingapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

func bootstrapAuthority(t *testing.T, scopes ...string) identity.Envelope {
	t.Helper()
	e, err := identity.FromVerified("tenant", "actor", "session", scopes, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestReportAppBootstrapModesAndExactTargets(t *testing.T) {
	service, err := reporting.NewAuthoring(&reporting.Documents{}, &reporting.Compositions{})
	if err != nil {
		t.Fatal(err)
	}
	call := bootstrapReportApp(service)
	e := bootstrapAuthority(t, "reporting.read", "reporting.write", "cw.report.read:report", "cw.report.write:report")
	for _, mode := range []string{"chat", "plan", "apply"} {
		out, err := call(t.Context(), e, ReportAppBootstrapRequest{Version: 1, Mode: mode, Targets: []string{"report"}})
		if err != nil || out.Mode != mode || len(out.Targets) != 1 || !out.Targets[0].Capabilities.CanSave || len(out.Guidance.Steps) == 0 {
			t.Fatal(out, err)
		}
	}
	for _, in := range []ReportAppBootstrapRequest{{Version: 2, Mode: "chat"}, {Version: 1, Mode: "publish"}, {Version: 1, Mode: "chat", Targets: []string{"report", "report"}}, {Version: 1, Mode: "chat", Targets: []string{"*"}}, {Version: 1, Mode: "chat", Targets: make([]string, 17)}} {
		if _, err := call(t.Context(), e, in); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal(in, err)
		}
	}
	for _, scopes := range [][]string{{"reporting.read"}, {"reporting.read", "cw.report.read:other"}, {"reporting.read", "cw.report.read:*"}} {
		if _, err := call(t.Context(), bootstrapAuthority(t, scopes...), ReportAppBootstrapRequest{Version: 1, Mode: "apply", Targets: []string{"report"}}); !errors.Is(err, access.ErrNotFound) {
			t.Fatal(scopes, err)
		}
	}
	if _, err := call(t.Context(), identity.Envelope{}, ReportAppBootstrapRequest{Version: 1, Mode: "chat"}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := call(ctx, e, ReportAppBootstrapRequest{Version: 1, Mode: "plan"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestReportAppBootstrapHTTPMCPAndStaticGuide(t *testing.T) {
	registry, err := ReportAppBootstrapRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Definitions()) != 2 {
		t.Fatal("missing registered guidance")
	}
	for _, d := range registry.Definitions() {
		if d.Public || d.Action != "reporting.read" || d.Effect != "retained_metadata_read" || d.Replay != "never" {
			t.Fatal(d.ID)
		}
		if d.Request.Validate([]byte(`{"version":1,"mode":"apply","targets":[],"scopes":["reporting.write"]}`), MaxBodyBytes) == nil {
			t.Fatal("authority injection", d.ID)
		}
	}
	service, _ := reporting.NewAuthoring(&reporting.Documents{}, &reporting.Compositions{})
	app, err := mcpserver.NewAppResource("ui://chartworks/report-app/v1", "Synthetic report app", "Synthetic data-free resource for registration proof", "<!doctype html><html><head><title>Synthetic resource</title></head><body><main>Read only synthetic application registration proof.</main></body></html>")
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := ReportAppBootstrapMCPBindings(service, app)
	if err != nil {
		t.Fatal(err)
	}
	mcp, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(mcp.Manifest()) != 2 {
		t.Fatal("guidance tools missing")
	}
	for _, tool := range mcp.Manifest() {
		if !tool.Annotations.ReadOnlyHint {
			t.Fatal("bootstrap mutates")
		}
	}
	if _, err := reportAppGuide(t.Context(), bootstrapAuthority(t, "reporting.write"), struct{}{}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
}
