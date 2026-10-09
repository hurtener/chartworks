package acceptance

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestReportAppWriteDependencyDiscovery(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	f.block(t, "original-block", f.base)
	f.block(t, "proposed-block", f.base)
	if _, err := f.blocks.Create(ctx, f.blockAuthor, reporting.CreateRequest{ID: "private-proposal", Definition: f.base}); err != nil {
		t.Fatal(err)
	}
	private, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, "private-proposal", reporting.Reference{Revision: 1}, reporting.Write)
	if err != nil {
		t.Fatal(err)
	}
	base := phase29Text("Baseline title")
	base.Widgets = append(base.Widgets, phase29BlockWidget("old", "original-block", 1, "table-main"))
	state := f.report(t, "write-report", base, false)
	s, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	actor := func(tenant, user string, scopes []string) identity.Envelope {
		t.Helper()
		e, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, f.f.f.token.claims(tenant, user, scopes), nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	seed := []string{"reporting.discover", "cw.report.write:write-report", "cw.tenant.write:" + f.author.Tenant()}
	e := actor(f.author.Tenant(), f.author.User(), seed)
	proposal := phase29Text("Private proposed title")
	proposal.Widgets = append(proposal.Widgets, phase29BlockWidget("new", "proposed-block", 1, "table-main"))
	create := reporting.WriteDependencyRequest{Operation: "create", ID: "write-report", Definition: proposal}
	beforeSource, beforeModel, beforeAttempts := f.f.f.lookups.Load(), f.f.model.requests.Load(), f.attemptCount(t)
	t.Run("create-derives-new-pins-without-content-authority", func(t *testing.T) {
		out, err := s.WriteDependencies(ctx, e, create)
		if err != nil || out.Version != "report-write-dependencies-v1" || out.Operation != "create" || out.ID != create.ID || out.BaseRevision != 0 || out.BaseDigest != "" || len(out.DefinitionDigest) != 64 || len(out.Blocks) != 1 || out.Blocks[0].ID != "proposed-block" || out.Blocks[0].Revision != 1 {
			t.Fatal(out, err)
		}
		raw, _ := json.Marshal(out)
		for _, secret := range []string{proposal.Metadata[0].Title, f.base.SQL, `"definition"`, `"sql"`, `"schema"`, `"actor"`, `"session"`} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("discovery returned content", secret)
			}
		}
		if _, err := s.Create(ctx, e, reporting.AuthoringCreateRequest{ID: create.ID, Definition: proposal}); err == nil {
			t.Fatal("discovery seed became write authority")
		}
		if _, err := f.f.f.db.ReadBlock(ctx, e, "proposed-block", reporting.Reference{Revision: 1}, reporting.Read); err == nil {
			t.Fatal("discovery seed became block read authority")
		}
	})
	t.Run("save-includes-removed-baseline-and-new-proposal", func(t *testing.T) {
		in := create
		in.Operation, in.Revision, in.ExpectedVersion = "save", 1, state.Version
		// Write access can inspect the native private baseline without report preview.
		out, err := s.WriteDependencies(ctx, e, in)
		if err != nil || out.BaseRevision != 1 || len(out.BaseDigest) != 64 || out.ExpectedVersion != state.Version || len(out.Blocks) != 2 {
			t.Fatal(out, err)
		}
		for _, id := range []string{"original-block", "proposed-block"} {
			if !slices.Contains(out.References, reporting.ResourceReference{Kind: "block", Permission: "read", ID: id}) {
				t.Fatal("missing baseline or proposal dependency", id)
			}
		}
		in.ExpectedVersion++
		if out, err := s.WriteDependencies(ctx, e, in); err == nil || out.ID != "" {
			t.Fatal("stale save returned requirements", out, err)
		}
	})
	t.Run("private-pin-retains-custody-and-preview-requirement", func(t *testing.T) {
		in := create
		in.Definition = phase29Text("Private pin")
		w := phase29BlockWidget("private", "private-proposal", 1, "table-main")
		w.Block.Policy, w.Block.Digest, w.Block.Revision = "private_preview", private.Revision.Digest, 1
		in.Definition.ReportPages = []reporting.ReportPage{{ID: "page", Title: "Page", Widgets: append(in.Definition.Widgets, w)}}
		in.Definition.SchemaVersion, in.Definition.Widgets = reporting.PagedDocumentVersion, nil
		out, err := s.WriteDependencies(ctx, e, in)
		if err != nil || len(out.Blocks) != 1 || !out.Blocks[0].Private || !slices.Contains(out.References, reporting.ResourceReference{Kind: "block", Permission: "preview", ID: "private-proposal"}) {
			t.Fatal(out, err)
		}
		if out, err := s.WriteDependencies(ctx, actor(f.author.Tenant(), "another-user", seed), in); err == nil || out.ID != "" {
			t.Fatal("cross-actor private pin", out, err)
		}
		in.Definition.ReportPages[0].Widgets[1].Block.Digest = strings.Repeat("0", 64)
		if _, err := s.WriteDependencies(ctx, e, in); err == nil {
			t.Fatal("private digest mismatch")
		}
		in.Definition.ReportPages[0].Widgets[1].Block.Policy, in.Definition.ReportPages[0].Widgets[1].Block.Digest = "published", ""
		if _, err := s.WriteDependencies(ctx, e, in); err == nil {
			t.Fatal("private pin disguised as public")
		}
	})
	t.Run("exact-root-tenant-and-closed-intent", func(t *testing.T) {
		for _, scopes := range [][]string{
			{"reporting.discover", "cw.report.write:write-report"},
			{"reporting.write", "cw.report.write:write-report", "cw.tenant.write:" + f.author.Tenant()},
			{"reporting.discover", "cw.report.write:other", "cw.tenant.write:" + f.author.Tenant()},
			{"reporting.discover", "cw.report.write:*", "cw.tenant.write:" + f.author.Tenant()},
		} {
			if out, err := s.WriteDependencies(ctx, actor(f.author.Tenant(), f.author.User(), scopes), create); err == nil || out.ID != "" {
				t.Fatal("insufficient seed", out, err)
			}
		}
		foreign := actor("foreign", f.author.User(), []string{"reporting.discover", "cw.report.write:write-report", "cw.tenant.write:foreign"})
		if _, err := s.WriteDependencies(ctx, foreign, create); err == nil {
			t.Fatal("cross-tenant block discovery")
		}
		query := create
		query.Definition = phase29Text("No query lane")
		query.Definition.Widgets = append(query.Definition.Widgets, f.queryWidget())
		if _, err := s.WriteDependencies(ctx, e, query); err == nil {
			t.Fatal("query accepted as manual definition")
		}
		registry, err := reportingapi.DependencyRegistry()
		if err != nil {
			t.Fatal(err)
		}
		handler := assertRegisteredWireSchemas(t, registry, reportingapi.DependencyHandler(f.f.f.token.verifier, s, http.NotFoundHandler()))
		bearer := f.f.f.token.sign(t, f.f.f.token.claims(f.author.Tenant(), f.author.User(), seed), nil)
		raw, _ := json.Marshal(create)
		if w := callProtected(t, handler, "POST", reportingapi.WriteDependencyDiscoveryPath, bearer, string(raw), map[string]string{"Content-Type": "application/json"}); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		injected := strings.TrimSuffix(string(raw), "}") + `,"references":[]}`
		if w := callProtected(t, handler, "POST", reportingapi.WriteDependencyDiscoveryPath, bearer, injected, map[string]string{"Content-Type": "application/json"}); w.Code != 400 {
			t.Fatal("dependency injection", w.Code)
		}
	})

	t.Run("native-create-save-consumes-exact-discovery-requirements", func(t *testing.T) {
		in := create
		in.ID = "allocated-report"
		discover := actor(f.author.Tenant(), f.author.User(), []string{"reporting.discover", "cw.report.write:" + in.ID, "cw.tenant.write:" + f.author.Tenant()})
		manifest, err := s.WriteDependencies(ctx, discover, in)
		if err != nil {
			t.Fatal(err)
		}
		authority := func(m reporting.WriteDependencyManifest) identity.Envelope {
			scopes := []string{"reporting.read", "reporting.write", "cw.report.write:" + in.ID, "cw.tenant.write:" + f.author.Tenant()}
			scopes = append(scopes, m.MetadataActions...)
			for _, ref := range m.References {
				scopes = append(scopes, "cw."+ref.Kind+"."+ref.Permission+":"+ref.ID)
				if ref.Permission == "preview" {
					scopes = append(scopes, "reporting.preview")
				}
			}
			slices.Sort(scopes)
			scopes = slices.Compact(scopes)
			return actor(f.author.Tenant(), f.author.User(), scopes)
		}
		created, err := s.Create(ctx, authority(manifest), reporting.AuthoringCreateRequest{ID: in.ID, Definition: in.Definition})
		if err != nil {
			t.Fatal("native create", err)
		}
		in.Operation, in.ExpectedVersion, in.Revision = "save", created.Version, created.DraftRevision
		in.Definition = phase29Text("Removed chart, still checks baseline")
		manifest, err = s.WriteDependencies(ctx, discover, in)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(manifest.References, reporting.ResourceReference{Kind: "block", Permission: "read", ID: "proposed-block"}) {
			t.Fatal("removed baseline dependency omitted")
		}
		saved, err := s.Save(ctx, authority(manifest), reporting.AuthoringSaveRequest{Report: in.ID, ExpectedVersion: in.ExpectedVersion, Revision: in.Revision, Definition: in.Definition})
		if err != nil || saved.DraftRevision != 2 || saved.Version != created.Version+1 {
			t.Fatal("native save", saved, err)
		}
	})
	if f.f.f.lookups.Load() != beforeSource || f.f.model.requests.Load() != beforeModel || f.attemptCount(t) != beforeAttempts {
		t.Fatal("metadata discovery performed source/model/attempt work")
	}
}
