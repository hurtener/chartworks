package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
)

func TestSourceStoreRejectsUnscopedOrPartialRecords(t *testing.T) {
	f := newSourceFixture(t, nil)
	ctx := context.Background()
	source := f.create(t, "sales")
	scope := support.Scope(t, f.e.Tenant(), f.e.User())
	record, err := f.db.ReadSource(ctx, scope, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if f.db.PutSource(ctx, store.Scope{}, 0, record) == nil {
		t.Fatal("unscoped source mutation")
	}
	foreign := record
	foreign.Binding = record.Binding.Clone()
	foreign.Binding.Tenant = "foreign"
	if f.db.PutSource(ctx, scope, 0, foreign) == nil {
		t.Fatal("foreign source record")
	}
	bad := record
	bad.Source.ContextID = "other"
	if f.db.PutSource(ctx, scope, 0, bad) == nil || f.db.PutSource(ctx, scope, -1, record) == nil {
		t.Fatal("invalid source revision admitted")
	}
	if _, err := f.db.ReadSource(ctx, store.Scope{}, source.ID); err == nil {
		t.Fatal("unscoped source read")
	}
	if _, err := f.db.ListSources(ctx, scope, access.Selection{}, 100); err == nil {
		t.Fatal("unrestricted source list")
	}
	selection, err := access.Constrain(f.e, "sources.read", "source", "read")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ListSources(ctx, scope, selection, 101); err == nil {
		t.Fatal("source result limit bypassed")
	}
	if f.db.WithSource(ctx, scope, source.ID, nil) == nil {
		t.Fatal("missing bounded callback")
	}
	if err := f.db.WithSource(ctx, scope, "absent", func(context.Context, sources.Record) error { return nil }); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("missing source callback invoked", err)
	}
	if !errors.Is(f.db.PutSource(ctx, scope, 0, record), store.ErrConflict) {
		t.Fatal("source identity overwritten")
	}
	// A failing audit must roll back the pointer and immutable revision together.
	raw := support.Raw(t, f.dsn)
	if _, err := raw.Exec(ctx, `CREATE FUNCTION chartworks.fail_source_rotation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='source.rotated' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_source_rotation BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.fail_source_rotation()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Rotate(ctx, f.e, source.ID, source.Revision); err == nil {
		t.Fatal("failed audit reported rotation success")
	}
	after, err := f.s.Get(ctx, f.e, source.ID)
	if err != nil || after != source {
		t.Fatal("failed rotation advanced publication", err)
	}
	var count int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.source_revisions WHERE tenant_id=$1 AND source_id=$2`, f.e.Tenant(), source.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed rotation retained partial revision", count, err)
	}
}

func TestVindexRepositoryBoundsAndAtomicFailure(t *testing.T) {
	f := newVectorFixture(t)
	ctx := context.Background()
	scope := support.Scope(t, f.e.Tenant(), f.e.User())
	g, b := vectorGeneration("v1", "credential:v1", 2, 2)
	q := vectorQuery(g)
	if f.db.BeginGeneration(ctx, store.Scope{}, g) == nil || f.db.BeginGeneration(ctx, scope, vindex.Generation{}) == nil {
		t.Fatal("invalid generation scope")
	}
	if f.db.UpsertFacets(ctx, store.Scope{}, g, b) == nil || f.db.UpsertFacets(ctx, scope, g, nil) == nil {
		t.Fatal("invalid vector batch")
	}
	if _, err := f.db.PublishGeneration(ctx, store.Scope{}, g, 0); err == nil {
		t.Fatal("unscoped activation")
	}
	if _, err := f.db.PublishGeneration(ctx, scope, g, -1); err == nil {
		t.Fatal("unversioned activation")
	}
	if _, err := f.db.PublishGeneration(ctx, scope, g, 0); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("missing generation published", err)
	}
	if _, err := f.db.ArchiveGeneration(ctx, store.Scope{}, g.Topic, g.Context, 1); err == nil {
		t.Fatal("unscoped archive")
	}
	if _, err := f.db.ArchiveGeneration(ctx, scope, g.Topic, g.Context, 0); err == nil {
		t.Fatal("unversioned archive")
	}
	if _, err := f.db.SearchFacets(ctx, store.Scope{}, []vindex.Query{q}); err == nil {
		t.Fatal("unscoped vector query")
	}
	if _, err := f.db.SearchFacets(ctx, scope, nil); err == nil {
		t.Fatal("empty vector query")
	}
	if _, err := f.db.SearchFacets(ctx, scope, []vindex.Query{{}}); err == nil {
		t.Fatal("invalid vector query")
	}
	if f.db.DeleteFacets(ctx, store.Scope{}, vindex.Removal{}) == nil || f.db.DeleteFacets(ctx, scope, vindex.Removal{Version: "partial"}) == nil {
		t.Fatal("ambiguous cleanup")
	}
	if _, err := f.db.ExplainFacets(ctx, store.Scope{}, q); err == nil {
		t.Fatal("unscoped diagnostic")
	}
	if _, err := f.db.ExplainFacets(ctx, scope, vindex.Query{}); err == nil {
		t.Fatal("invalid diagnostic")
	}
	f.stage(t, f.e, g, b)
	f.publish(t, f.e, g, 0)
	wrong := q
	wrong.Space.Revision = "different"
	if _, err := f.db.ExplainFacets(ctx, scope, wrong); !errors.Is(err, store.ErrConflict) {
		t.Fatal("diagnostic accepted incompatible space", err)
	}
	raw := support.Raw(t, f.dsn)
	if _, err := raw.Exec(ctx, `CREATE FUNCTION chartworks.fail_facet_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='facets.generation_staged' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_facet_audit BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.fail_facet_audit()`); err != nil {
		t.Fatal(err)
	}
	newGeneration, _ := vectorGeneration("new", "other:v1", 2, 2)
	if f.s.Begin(ctx, f.e, newGeneration) == nil {
		t.Fatal("failed staging audit reported success")
	}
	var count int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.vector_heads WHERE tenant_id=$1 AND context_id=$2`, f.e.Tenant(), newGeneration.Context).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed staging left partial head", count, err)
	}
}

func TestVindexBatchResponseCapIsAtomic(t *testing.T) {
	f := newVectorFixture(t)
	g, b := vectorGeneration("large", "credential:v1", 2, 80)
	kinds := []string{"topic", "entity", "measure", "dimension", "kpi", "relationship", "rule", "example"}
	for i := range b {
		b[i].Kind = kinds[i%len(kinds)]
		b[i].Text = strings.Repeat("x", 4000)
		g.Expected[i].Kind = b[i].Kind
		g.Expected[i].TextHash = vindex.TextHash(b[i].Text)
	}
	f.stage(t, f.e, g, b)
	f.publish(t, f.e, g, 0)
	q := vectorQuery(g)
	q.Kinds = kinds
	queries := make([]vindex.Query, 8)
	for i := range queries {
		queries[i] = q
		queries[i].ID = string(rune('a' + i))
	}
	if results, err := f.s.Search(context.Background(), f.e, queries); !errors.Is(err, store.ErrInvalid) || results != nil {
		t.Fatal("response cap leaked a partial batch", err)
	}
}
