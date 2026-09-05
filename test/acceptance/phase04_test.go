package acceptance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/securityapi"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/telemetry"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

type countedRepository struct {
	store.Foundation
	reads atomic.Int64
}

func (c *countedRepository) Policy(ctx context.Context, s store.Scope) (store.Policy, error) {
	c.reads.Add(1)
	return c.Foundation.Policy(ctx, s)
}
func (c *countedRepository) Audits(ctx context.Context, s store.Scope, limit int) ([]store.Audit, error) {
	c.reads.Add(1)
	return c.Foundation.Audits(ctx, s, limit)
}
func operationalScopes(tenant string) []string {
	return []string{"ops.read", "ops.write", "ops.audit", "ops.maintain", "ops.inspect", "ops.metrics", "cw.tenant.read:" + tenant, "cw.tenant.write:" + tenant, "cw.tenant.erase:" + tenant}
}
func protectedFixture(t *testing.T, f *tokenFixture) (*securityapi.Service, http.Handler, *countedRepository) {
	t.Helper()
	db := support.Open(t, support.Database(t))
	counted := &countedRepository{Foundation: db}
	s, err := securityapi.New(counted)
	if err != nil {
		t.Fatal(err)
	}
	r, err := telemetry.New(io.Discard, "json", true)
	if err != nil {
		t.Fatal(err)
	}
	return s, securityapi.Handler(f.verifier, s, r, true), counted
}
func TestPhase04(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newTokenFixture(t)
		e := f.envelope(t, "tenant", "user", "reporting.execute", "cw.report.execute:r1")
		r := access.Resource{Tenant: "tenant", Kind: "report", Permission: "execute", ID: "r1"}
		if access.Require(e, "reporting.execute", r) != nil {
			t.Fatal("exact reach denied")
		}
		for _, change := range []access.Resource{{Tenant: "foreign", Kind: "report", Permission: "execute", ID: "r1"}, {Tenant: "tenant", Kind: "report", Permission: "execute", ID: "r2"}, {Tenant: "tenant", Kind: "report", Permission: "read", ID: "r1"}, {Tenant: "tenant", Kind: "unknown", Permission: "execute", ID: "r1"}, {Tenant: "tenant", Kind: "report", Permission: "execute", ID: "*"}, {Tenant: "tenant", Kind: "tenant", Permission: "execute", ID: "foreign"}} {
			if access.Require(e, "reporting.execute", change) == nil {
				t.Fatal("scope broadened")
			}
		}
		if access.Require(e, "reporting.publish", r) == nil || access.Require(e, "reporting.execute") == nil || access.Require(identity.Envelope{}, "reporting.execute", r) == nil {
			t.Fatal("missing action/target/envelope allowed")
		}
		wildcard := f.envelope(t, "tenant", "svc:worker", "reporting.execute", "cw.report.execute:*")
		r.ID = "r2"
		if access.Require(wildcard, "reporting.execute", r) != nil {
			t.Fatal("explicit wildcard denied")
		}
		r.Tenant = "foreign"
		if access.Require(wildcard, "reporting.execute", r) == nil {
			t.Fatal("wildcard crossed tenant")
		}
		for _, scopes := range [][]string{{"admin"}, {"creator"}, {"agent"}, {"reporting.execute"}, {"cw.report.execute:r1"}} {
			x := f.envelope(t, "tenant", "svc:worker", scopes...)
			r.Tenant = "tenant"
			r.ID = "r1"
			if access.Require(x, "reporting.execute", r) == nil {
				t.Fatal("implicit privilege")
			}
		}
	})
	t.Run("AC02", func(t *testing.T) {
		f := newTokenFixture(t)
		s, h, counted := protectedFixture(t, f)
		good := f.envelope(t, "tenant", "user", operationalScopes("tenant")...)
		if _, err := s.Configure(context.Background(), good, 0, store.Policy{AuditDays: 7, OperationHours: 24}); err != nil {
			t.Fatal(err)
		}
		for _, scopes := range [][]string{nil, {"ops.read"}, {"ops.read", "cw.tenant.read:foreign"}} {
			e := f.envelope(t, "tenant", "user", scopes...)
			before := counted.reads.Load()
			if _, err := s.Policy(context.Background(), e); err == nil {
				t.Fatal("denied read succeeded")
			}
			if counted.reads.Load() != before {
				t.Fatal("denied read reached real database")
			}
		}
		empty := access.Selection{}
		if empty.Contains("tenant", "r1") || empty.All() || empty.Tenant() != "" {
			t.Fatal("zero selection widened")
		}
		e := f.envelope(t, "tenant", "user", "query.execute", "cw.source.query:s2", "cw.source.query:s1")
		selection, err := access.Constrain(e, "query.execute", "source", "query")
		if err != nil {
			t.Fatal(err)
		}
		ids := selection.IDs()
		ids[0] = "foreign"
		if selection.Tenant() != "tenant" || selection.All() || !selection.Contains("tenant", "s1") || selection.Contains("foreign", "s1") || selection.Contains("tenant", "s3") || selection.Contains("tenant", "") {
			t.Fatal("bad pre-query constraint")
		}
		for _, args := range [][3]string{{"missing", "source", "query"}, {"query.execute", "unknown", "query"}, {"query.execute", "report", "read"}} {
			if _, err = access.Constrain(e, args[0], args[1], args[2]); err == nil {
				t.Fatal("invalid query selection")
			}
		}
		all := f.envelope(t, "tenant", "user", "query.execute", "cw.source.query:*")
		sel, err := access.Constrain(all, "query.execute", "source", "query")
		if err != nil || !sel.All() || !sel.Contains("tenant", "any") || sel.Contains("foreign", "any") {
			t.Fatal("bad wildcard query predicate")
		}
		token := f.sign(t, f.claims("tenant", "user", operationalScopes("tenant")), nil)
		w := callProtected(t, h, "GET", "/v1/retention-policy", token, "", map[string]string{"X-Tenant-ID": "foreign", "X-User-ID": "administrator"})
		if w.Code != 200 || strings.Contains(w.Body.String(), "foreign") {
			t.Fatal("unsigned header override")
		}
		w = callProtected(t, h, "GET", "/v1/retention-policy?tenant=foreign", token, "", nil)
		if w.Code != 422 {
			t.Fatal("accepted authority query override")
		}
	})
	t.Run("AC03", func(t *testing.T) {
		f := newTokenFixture(t)
		refs := []string{"reporting.execute", "reporting.read", "cw.report.execute:report1", "cw.report.read:report1", "cw.source.query:source1", "cw.execution_context.use:context1:v1"}
		e := f.envelope(t, "tenant", "user", refs...)
		contextRef := access.Resource{Tenant: "tenant", Kind: "execution_context", Permission: "use", ID: "context1:v1"}
		x := access.Execution{Target: access.Resource{Tenant: "tenant", Kind: "report", Permission: "execute", ID: "report1"}, Dependencies: []access.Resource{{Tenant: "tenant", Kind: "source", Permission: "query", ID: "source1"}}, Contexts: []access.Resource{contextRef}}
		if access.RequireExecution(e, x) != nil {
			t.Fatal("valid resolved execution denied")
		}
		for _, kind := range []string{"no-dependencies", "no-context", "other-source", "foreign-context", "wrong-version", "wrong-permission"} {
			copyX := x
			copyX.Dependencies = append([]access.Resource(nil), x.Dependencies...)
			copyX.Contexts = append([]access.Resource(nil), x.Contexts...)
			switch kind {
			case "no-dependencies":
				copyX.Dependencies = nil
			case "no-context":
				copyX.Contexts = nil
			case "other-source":
				copyX.Dependencies[0].ID = "other"
			case "foreign-context":
				copyX.Contexts[0].Tenant = "foreign"
			case "wrong-version":
				copyX.Contexts[0].ID = "context1:v2"
			case "wrong-permission":
				copyX.Target.Permission = "read"
			}
			if access.RequireExecution(e, copyX) == nil {
				t.Fatal("incomplete/foreign execution accepted", kind)
			}
		}
		a := access.Artifact{Tenant: "tenant", RunID: "run1", ParentKind: "report", ParentID: "report1", Published: true, Contexts: []access.Resource{contextRef}}
		if access.RequireArtifact(e, a) != nil {
			t.Fatal("published authorized artifact denied")
		}
		exact := f.envelope(t, "tenant", "user", "reporting.read", "cw.run.read:run1", "cw.execution_context.use:context1:v1")
		a.Published = false
		if access.RequireArtifact(exact, a) != nil || exact.Has("query.execute") {
			t.Fatal("artifact read incorrectly needs execute")
		}
		if access.RequireArtifact(e, a) == nil {
			t.Fatal("unpublished parent shortcut")
		}
		a.Private = true
		if access.RequireArtifact(exact, a) == nil {
			t.Fatal("private preview leak")
		}
		preview := f.envelope(t, "tenant", "user", "reporting.read", "reporting.preview", "cw.run.read:run1", "cw.report.preview:report1", "cw.execution_context.use:context1:v1")
		if access.RequireArtifact(preview, a) != nil {
			t.Fatal("authorized private preview denied")
		}
		a.Published = true
		if access.RequireArtifact(exact, a) == nil {
			t.Fatal("publication made preview public")
		}
		a.Contexts[0].ID = "context1:v2"
		if access.RequireArtifact(preview, a) == nil {
			t.Fatal("cross-partition artifact reuse")
		}
		a.Contexts = nil
		if access.RequireArtifact(preview, a) == nil {
			t.Fatal("unknown data partition accepted")
		}
		if access.RequireArtifact(identity.Envelope{}, a) == nil {
			t.Fatal("zero envelope artifact")
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newTokenFixture(t)
		actions := map[string]string{"reporting.preview": "preview", "reporting.sql.read": "read", "reporting.publish": "publish", "reporting.certify": "certify", "reporting.export": "export", "reporting.schedule.manage": "execute"}
		for action, permission := range actions {
			resource := access.Resource{Tenant: "tenant", Kind: "block", Permission: permission, ID: "block1"}
			e := f.envelope(t, "tenant", "user", action, "cw.block."+permission+":block1")
			if access.Require(e, action, resource) != nil {
				t.Fatal("explicit action denied")
			}
			for other := range actions {
				if other == action {
					continue
				}
				if access.Require(e, other, resource) == nil {
					t.Fatal("distinct action collapsed")
				}
			}
		}
		if _, err := securityapi.New(nil); err == nil {
			t.Fatal("nil store accepted")
		}
	})
	t.Run("AC05", func(t *testing.T) {
		f := newTokenFixture(t)
		_, h, counted := protectedFixture(t, f)
		for _, scopes := range [][]string{nil, {"ops.inspect"}, {"ops.inspect", "cw.tenant.read:foreign"}, {"admin", "cw.tenant.read:tenant"}} {
			token := f.sign(t, f.claims("tenant", "user", scopes), nil)
			w := callProtected(t, h, "GET", "/v1/access/diagnostics", token, "", nil)
			if w.Code == 200 {
				t.Fatal("unprivileged diagnostics")
			}
		}
		token := f.sign(t, f.claims("tenant", "user", []string{"ops.inspect", "cw.tenant.read:tenant"}), nil)
		w := callProtected(t, h, "GET", "/v1/access/diagnostics", token, "", nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), "test-session") || strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "\"user\"") {
			t.Fatal("diagnostic disclosure")
		}
		if counted.reads.Load() != 0 {
			t.Fatal("diagnostics loaded private data")
		}
		for _, path := range []string{"/auth/token", "/v1/grants", "/v1/principals", "/v1/roles", "/v1/bootstrap", "/v1/embed-tokens"} {
			if callProtected(t, h, "POST", path, token, `{}`, nil).Code != 404 {
				t.Fatal("forbidden administration route")
			}
		}
	})
	t.Run("AC06", func(t *testing.T) {
		f := newTokenFixture(t)
		s, h, _ := protectedFixture(t, f)
		for _, op := range securityapi.Operations() {
			if callProtected(t, h, op.Method, op.Path, "", "", nil).Code != 401 {
				t.Fatal("unprotected registry entry", op.Path)
			}
			action := f.sign(t, f.claims("tenant", "user", []string{op.Action}), nil)
			if callProtected(t, h, op.Method, op.Path, action, `{}`, nil).Code != 404 {
				t.Fatal("operation-only token bypass", op.Path)
			}
			reach := f.sign(t, f.claims("tenant", "user", []string{"cw.tenant." + op.Permission + ":tenant"}), nil)
			if callProtected(t, h, op.Method, op.Path, reach, `{}`, nil).Code != 403 {
				t.Fatal("reach-only token bypass", op.Path)
			}
		}
		server := httptest.NewServer(h)
		defer server.Close()
		clients := map[string]*cw.Client{}
		for _, tenant := range []string{"alpha", "beta"} {
			name := tenant
			token := f.sign(t, f.claims(name, "svc:"+name, operationalScopes(name)), nil)
			c, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
			if err != nil {
				t.Fatal(err)
			}
			clients[name] = c
			p, err := c.SetRetentionPolicy(context.Background(), 0, len(name), 24)
			if err != nil || p.Revision != 1 || p.AuditDays != len(name) {
				t.Fatal("real SDK write failed", err)
			}
			p, err = c.RetentionPolicy(context.Background())
			if err != nil || p.AuditDays != len(name) {
				t.Fatal("real SDK read failed", err)
			}
			op, err := c.Sweep(context.Background(), "same-key")
			if err != nil || op.Status != "succeeded" {
				t.Fatal("protected sweep failed", err)
			}
			again, err := c.Sweep(context.Background(), "same-key")
			if err != nil || again.ID != op.ID {
				t.Fatal("authorized replay changed operation")
			}
			a, err := c.AuditEvents(context.Background())
			if err != nil || len(a) < 2 {
				t.Fatal("authorized audit missing")
			}
			for _, v := range a {
				if v.Actor != "svc:"+name {
					t.Fatal("foreign audit read")
				}
			}
		}
		var wg sync.WaitGroup
		for i := 0; i < 32; i++ {
			for name, client := range clients {
				wg.Add(1)
				go func() {
					defer wg.Done()
					p, err := client.RetentionPolicy(context.Background())
					if err != nil || p.AuditDays != len(name) {
						t.Error("concurrent tenant isolation failed", err)
					}
				}()
			}
		}
		wg.Wait()
		e := f.envelope(t, "alpha", "user", "ops.read", "cw.tenant.read:alpha")
		if _, err := s.Configure(context.Background(), e, 1, store.Policy{AuditDays: 1, OperationHours: 1}); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("in-process write bypass")
		}
		if _, err := s.Audits(context.Background(), e, 10); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("in-process audit bypass")
		}
		if _, err := s.Sweep(context.Background(), e, "key"); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("in-process sweep bypass")
		}
	})
}
