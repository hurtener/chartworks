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
	"github.com/hurtener/chartworks/test/support"
)

func TestReportAppDependencyDiscovery(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	f.block(t, "discovery-block", f.base)
	if _, err := f.blocks.Create(ctx, f.blockAuthor, reporting.CreateRequest{ID: "private-block", Definition: f.base}); err != nil {
		t.Fatal(err)
	}
	d := phase29Text("Secret report title must not appear in discovery")
	d.Widgets = append(d.Widgets, phase29BlockWidget("data", "discovery-block", 1, "table-main"))
	draft := f.report(t, "discovery-report", d, false)
	dynamic := phase29Text("Dynamic definitions are outside manual discovery")
	dynamic.Widgets = append(dynamic.Widgets, f.queryWidget())
	f.report(t, "dynamic-report", dynamic, true)
	s, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	seed := []string{"reporting.discover", "cw.report.read:discovery-report", "reporting.preview", "cw.report.preview:discovery-report"}
	actor := func(tenant, user string, scopes []string) identity.Envelope {
		t.Helper()
		claims := f.f.f.token.claims(tenant, user, scopes)
		e, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	request := reporting.DependencyRequest{Kind: "report", ID: "discovery-report", Revision: 1}
	beforeSource, beforeModel, beforeAttempts := f.f.f.lookups.Load(), f.f.model.requests.Load(), f.attemptCount(t)
	t.Run("exact-root-without-dependency-grants-discovers-only-identifiers", func(t *testing.T) {
		e := actor(f.author.Tenant(), f.author.User(), seed)
		out, err := s.Dependencies(ctx, e, request)
		if err != nil || out.ID != request.ID || out.Revision != 1 || !out.Private || len(out.Blocks) != 1 || len(out.References) < 4 {
			t.Fatal(out, err)
		}
		block, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, "discovery-block", reporting.Reference{}, reporting.Write)
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range block.References {
			if !slices.Contains(out.References, ref) {
				t.Fatal("missing native dependency", ref)
			}
		}
		if out.Blocks[0].Digest != block.Revision.Digest || out.Blocks[0].Private || out.Blocks[0].Topic != block.State.Topic {
			t.Fatal(out.Blocks)
		}
		raw, _ := json.Marshal(out)
		for _, secret := range []string{d.Metadata[0].Title, f.base.SQL, `"definition"`, `"sql"`, `"schema"`, `"actor"`, `"session"`} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("content escaped metadata projection", secret)
			}
		}
		if _, err := s.Read(ctx, e, reporting.AuthoringReadRequest{Report: request.ID, Revision: 1}); err == nil {
			t.Fatal("metadata seed became content authority")
		}
		reader := actor(f.author.Tenant(), f.author.User(), append(slices.Clone(seed), "reporting.read"))
		if _, err := f.f.f.db.ReadDocument(ctx, reader, "report", request.ID, reporting.DocumentReference{Revision: 1}, reporting.Read, false); err == nil {
			t.Fatal("ordinary content read lost dependency enforcement")
		}
	})
	t.Run("editing-selector-follows-native-draft-then-review", func(t *testing.T) {
		e := actor(f.author.Tenant(), f.author.User(), seed)
		out, err := s.Dependencies(ctx, e, reporting.DependencyRequest{Kind: "report", ID: request.ID, Stage: "editing"})
		if err != nil || out.Revision != 1 || !out.Private {
			t.Fatal(out, err)
		}
		if _, err := f.documents.Transition(ctx, f.author, "report", request.ID, draft.Version, draft.DraftRevision, "review", ""); err != nil {
			t.Fatal(err)
		}
		out, err = s.Dependencies(ctx, e, reporting.DependencyRequest{Kind: "report", ID: request.ID, Stage: "editing"})
		if err != nil || out.Revision != 1 || !out.Private {
			t.Fatal("missing native review fallback", out, err)
		}
		if _, err := s.Dependencies(ctx, e, reporting.DependencyRequest{Kind: "report", ID: request.ID, Stage: "draft"}); err == nil {
			t.Fatal("explicit draft used review fallback")
		}
	})
	t.Run("current-grants-and-custody-before-projection", func(t *testing.T) {
		for _, scopes := range [][]string{
			{"cw.report.read:discovery-report", "reporting.preview", "cw.report.preview:discovery-report"},
			{"reporting.discover", "cw.report.read:other", "reporting.preview", "cw.report.preview:discovery-report"},
			{"reporting.discover", "cw.report.read:discovery-report"},
			{"reporting.discover", "cw.report.read:*", "reporting.preview", "cw.report.preview:discovery-report"},
		} {
			if out, err := s.Dependencies(ctx, actor(f.author.Tenant(), f.author.User(), scopes), request); err == nil || out.ID != "" {
				t.Fatal("insufficient seed disclosed metadata", out, err)
			}
		}
		if _, err := s.Dependencies(ctx, actor("foreign", f.author.User(), seed), request); err == nil {
			t.Fatal("cross-tenant discovery")
		}
		if _, err := s.Dependencies(ctx, actor(f.author.Tenant(), f.author.User(), seed), reporting.DependencyRequest{Kind: "report", ID: "discovery-report", Revision: 257}); err == nil {
			t.Fatal("unbounded revision")
		}
	})
	t.Run("private-block-requires-original-actor-and-preview", func(t *testing.T) {
		in := reporting.DependencyRequest{Kind: "block", ID: "private-block", Revision: 1}
		scopes := []string{"reporting.discover", "cw.block.read:private-block", "reporting.preview", "cw.block.preview:private-block"}
		out, err := s.Dependencies(ctx, actor(f.author.Tenant(), f.author.User(), scopes), in)
		if err != nil || !out.Private || len(out.Blocks) != 1 {
			t.Fatal(out, err)
		}
		if _, err := s.Dependencies(ctx, actor(f.author.Tenant(), "another-user", scopes), in); err == nil {
			t.Fatal("private custody bypass")
		}
		if _, err := s.Dependencies(ctx, actor(f.author.Tenant(), f.author.User(), scopes[:2]), in); err == nil {
			t.Fatal("private preview bypass")
		}
	})
	t.Run("replayable-query-without-session-query-index-is-unsupported", func(t *testing.T) {
		e := actor(f.author.Tenant(), f.author.User(), []string{"reporting.discover", "cw.report.read:dynamic-report"})
		if _, err := s.Dependencies(ctx, e, reporting.DependencyRequest{Kind: "report", ID: "dynamic-report", Revision: 1}); err == nil {
			t.Fatal("dynamic query returned an incomplete manual dependency manifest")
		}
	})
	t.Run("registered-wire-rejects-authority-and-dependency-injection", func(t *testing.T) {
		registry, err := reportingapi.DependencyRegistry()
		if err != nil {
			t.Fatal(err)
		}
		handler := assertRegisteredWireSchemas(t, registry, reportingapi.DependencyHandler(f.f.f.token.verifier, s, http.NotFoundHandler()))
		bearer := f.f.f.token.sign(t, f.f.f.token.claims(f.author.Tenant(), f.author.User(), seed), nil)
		for _, body := range []string{`{"kind":"report","id":"discovery-report","revision":1,"tenant":"foreign"}`, `{"kind":"report","id":"discovery-report","revision":1,"references":[]}`} {
			if w := callProtected(t, handler, "POST", reportingapi.DependencyDiscoveryPath, bearer, body, map[string]string{"Content-Type": "application/json"}); w.Code != 400 {
				t.Fatal("untrusted field disposition", w.Code)
			}
		}
		if w := callProtected(t, handler, "POST", reportingapi.DependencyDiscoveryPath, bearer, `{"kind":"report","id":"discovery-report","revision":1}`, map[string]string{"Content-Type": "application/json"}); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	})
	t.Run("complete-or-fail-never-truncate", func(t *testing.T) {
		raw := support.Raw(t, f.f.f.dsn)
		_, err := raw.Exec(ctx, `INSERT INTO chartworks.document_references SELECT $1,'report','discovery-report',1,'execution_context','use','extra:'||n FROM generate_series(1,129) n`, f.author.Tenant())
		if err != nil {
			t.Fatal(err)
		}
		if out, err := s.Dependencies(ctx, actor(f.author.Tenant(), f.author.User(), seed), request); err == nil || out.ID != "" {
			t.Fatal("truncated manifest", out, err)
		}
	})
	if beforeSource != f.f.f.lookups.Load() || beforeModel != f.f.model.requests.Load() || beforeAttempts != f.attemptCount(t) {
		t.Fatal("discovery performed source/model work")
	}
}

func TestReportAppDependenciesResolveCurrentPublishedPin(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	f.block(t, "moving-block", f.base)
	d := phase29Text("Current published dependency")
	d.Widgets = append(d.Widgets, phase29BlockWidget("data", "moving-block", 1, "table-main"))
	f.report(t, "moving-report", d, true)
	prior, err := f.blocks.Read(ctx, f.blockAuthor, "moving-block", reporting.Reference{})
	if err != nil {
		t.Fatal(err)
	}
	definition := phase27Copy(t, f.base)
	definition.Metadata[0].Title += " amended"
	draft, err := f.blocks.Edit(ctx, f.blockAuthor, "moving-block", reporting.EditRequest{ExpectedVersion: prior.State.Version, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, f.blocks, f.blockAuthor, draft)
	// An additional persisted requirement on the new immutable revision must
	// not disappear behind the report's older observed dependency snapshot.
	raw := support.Raw(t, f.f.f.dsn)
	if _, err := raw.Exec(ctx, `INSERT INTO chartworks.block_revision_references VALUES($1,'moving-block',2,'execution_context','use','new-partition:v2')`, f.author.Tenant()); err != nil {
		t.Fatal(err)
	}
	claims := f.f.f.token.claims(f.author.Tenant(), f.author.User(), []string{"reporting.discover", "cw.report.read:moving-report"})
	e, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.f.f.db.DiscoverReportDependencies(ctx, e, reporting.DependencyRequest{Kind: "report", ID: "moving-report", Revision: 1})
	if err != nil || len(out.Blocks) != 1 || out.Blocks[0].Revision != 2 || !slices.Contains(out.References, reporting.ResourceReference{Kind: "execution_context", Permission: "use", ID: "new-partition:v2"}) {
		t.Fatal("used historical publication dependencies", out, err)
	}
}
