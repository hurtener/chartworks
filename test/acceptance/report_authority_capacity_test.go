package acceptance

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
)

// TestReportAuthorityCapacity extends Phase03/AC04 and Phase04 exact reach with
// twelve independent chart dependency sets, both audiences and smaller config.
func TestReportAuthorityCapacity(t *testing.T) {
	f := newTokenFixture(t)
	scopes := []string{"reporting.read", "reporting.preview", "cw.report.read:report", "cw.report.preview:report", "cw.execution_context.use:context-a"}
	resources := []access.Resource{{Tenant: "tenant-a", Kind: "report", Permission: "read", ID: "report"}, {Tenant: "tenant-a", Kind: "execution_context", Permission: "use", ID: "context-a"}}
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("chart-%02d", i)
		for _, permission := range []string{"read", "preview", "execute"} {
			scopes = append(scopes, "cw.block."+permission+":"+id)
			resources = append(resources, access.Resource{Tenant: "tenant-a", Kind: "block", Permission: permission, ID: id})
		}
	}
	for _, surface := range []auth.Surface{auth.HTTP, auth.MCP} {
		claims := f.claims("tenant-a", "author", scopes)
		if surface == auth.MCP {
			claims["aud"] = f.cfg.MCPAudience()
			claims["scopes"] = append(append([]string{}, scopes...), "mcp.use")
		}
		token := f.sign(t, claims, nil)
		e, err := f.verifier.Verify(t.Context(), token, surface)
		if err != nil || access.Require(e, "reporting.read", resources...) != nil {
			t.Fatal("complete twelve-chart authority rejected", surface, err)
		}
		for _, denied := range []access.Resource{{Tenant: "tenant-b", Kind: "report", Permission: "read", ID: "report"}, {Tenant: "tenant-a", Kind: "execution_context", Permission: "use", ID: "context-b"}, {Tenant: "tenant-a", Kind: "block", Permission: "read", ID: "chart-12"}} {
			if access.Require(e, "reporting.read", denied) == nil {
				t.Fatal("larger authority inferred ungranted reach")
			}
		}
		if access.Require(e, "reporting.publish", resources...) == nil {
			t.Fatal("larger authority inferred an operation")
		}
		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				current, err := f.verifier.Verify(context.Background(), token, surface)
				if err != nil || access.Require(current, "reporting.read", resources...) != nil {
					t.Error("concurrent exact authority failed", err)
				}
			})
		}
		wg.Wait()
	}
	legacy := f.cfg
	legacy.MaxScopes, legacy.MaxScopeBytes = 32, 4096
	v, err := auth.New(legacy, f.server.Client(), func() time.Time { return time.Unix(f.clock.Load(), 0) })
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if _, err := v.Verify(t.Context(), f.sign(t, f.claims("tenant-a", "author", scopes), nil), auth.HTTP); err == nil {
		t.Fatal("explicit smaller scope configuration ignored")
	}
	if _, err := v.Verify(t.Context(), f.sign(t, f.claims("tenant-a", "author", scopes[:5]), nil), auth.HTTP); err != nil {
		t.Fatal("legacy small token rejected", err)
	}
	wide := make([]string, 64)
	for i := range wide {
		wide[i] = fmt.Sprintf("%03d", i) + strings.Repeat("x", identity.MaxScopeBytes-3)
	}
	if _, err := f.verifier.Verify(t.Context(), f.sign(t, f.claims("tenant-a", "author", wide), nil), auth.HTTP); err != nil {
		t.Fatal("exact aggregate byte ceiling rejected", err)
	}
	if _, err := f.verifier.Verify(t.Context(), f.sign(t, f.claims("tenant-a", "author", append(wide, "x")), nil), auth.HTTP); err == nil {
		t.Fatal("aggregate scope byte overflow accepted")
	}
}
