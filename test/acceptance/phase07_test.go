package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
)

type vectorFixture struct {
	dsn   string
	db    *postgres.DB
	s     *vindex.Service
	token *tokenFixture
	e     identity.Envelope
}

func newVectorFixture(t *testing.T) *vectorFixture {
	t.Helper()
	f := &vectorFixture{dsn: support.Database(t), token: newTokenFixture(t)}
	f.db = support.Open(t, f.dsn)
	var err error
	f.s, err = vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.e = f.actor(t, "vector-a")
	return f
}
func (f *vectorFixture) actor(t *testing.T, tenant string) identity.Envelope {
	return f.token.envelope(t, tenant, "operator", "topics.write", "topics.read", "topics.publish", "ops.maintain", "cw.topic.write:*", "cw.topic.read:*", "cw.topic.publish:*", "cw.topic.erase:*", "cw.execution_context.use:*", "cw.tenant.erase:"+tenant)
}
func vectorGeneration(id, partition string, dimensions, count int) (vindex.Generation, []vindex.Facet) {
	g := vindex.Generation{ID: id, Topic: "sales", Version: id, Context: partition, SourceGeneration: "source:" + id, Space: vindex.Space{Provider: "openrouter", Route: "reference", Endpoint: "default", Model: "synthetic", Revision: "r1", Dimensions: dimensions, Preprocessing: "utf8-verbatim-v1", InputType: "document", Normalization: "provider"}}
	batch := make([]vindex.Facet, count)
	for i := range batch {
		id := fmt.Sprintf("facet-%04d", i)
		kind := "measure"
		if i%2 == 1 {
			kind = "dimension"
		}
		text := "synthetic repeated evidence"
		vector := make([]float32, dimensions)
		vector[0] = 1
		batch[i] = vindex.Facet{ID: id, Kind: kind, SourceID: fmt.Sprintf("origin-%04d", i), Text: text, Vector: vector}
		g.Expected = append(g.Expected, vindex.Origin{ID: id, Kind: kind, SourceID: batch[i].SourceID, TextHash: vindex.TextHash(text)})
	}
	return g, batch
}
func (f *vectorFixture) stage(t *testing.T, e identity.Envelope, g vindex.Generation, batch []vindex.Facet) {
	t.Helper()
	if err := f.s.Begin(context.Background(), e, g); err != nil {
		t.Fatal("begin", err)
	}
	for start := 0; start < len(batch); start += 64 {
		end := start + 64
		if end > len(batch) {
			end = len(batch)
		}
		if err := f.s.Upsert(context.Background(), e, g, batch[start:end]); err != nil {
			t.Fatal("upsert", err)
		}
	}
}
func (f *vectorFixture) publish(t *testing.T, e identity.Envelope, g vindex.Generation, revision int64) vindex.Publication {
	t.Helper()
	p, err := f.s.Publish(context.Background(), e, g, revision)
	if err != nil {
		t.Fatal("publish", err)
	}
	return p
}
func vectorQuery(g vindex.Generation) vindex.Query {
	v := make([]float32, g.Space.Dimensions)
	v[0] = 1
	return vindex.Query{ID: "query", Topic: g.Topic, Context: g.Context, Space: g.Space, Vector: v, Kinds: []string{"measure", "dimension"}, LimitPerKind: 10}
}
func (f *vectorFixture) search(t *testing.T, e identity.Envelope, q ...vindex.Query) []vindex.Result {
	t.Helper()
	r, err := f.s.Search(context.Background(), e, q)
	if err != nil {
		t.Fatal("search", err)
	}
	return r
}

func TestPhase07(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newVectorFixture(t)
		g, b := vectorGeneration("v1", "credential-a:v1", 3, 4)
		f.stage(t, f.e, g, b)
		if got := f.search(t, f.e, vectorQuery(g)); len(got[0].Hits) != 0 {
			t.Fatal("unpublished facets visible")
		}
		f.publish(t, f.e, g, 0)
		foreign := f.actor(t, "vector-b")
		if got := f.search(t, foreign, vectorQuery(g)); len(got[0].Hits) != 0 {
			t.Fatal("foreign tenant visible")
		}
		f.stage(t, foreign, g, b)
		f.publish(t, foreign, g, 0)
		g2, b2 := vectorGeneration("v2", "credential-b:v1", 3, 2)
		f.stage(t, f.e, g2, b2)
		f.publish(t, f.e, g2, 0)
		narrow := f.token.envelope(t, "vector-a", "reader", "topics.read", "cw.topic.read:sales", "cw.execution_context.use:credential-a:v1")
		q1, q2 := vectorQuery(g), vectorQuery(g2)
		q2.ID = "second"
		if out, err := f.s.Search(context.Background(), narrow, []vindex.Query{q1, q2}); err == nil || out != nil {
			t.Fatal("batch leaked disallowed context")
		}
		if got := f.search(t, narrow, q1); len(got[0].Hits) != 4 || got[0].Publication.Generation != "v1" {
			t.Fatal("authorized selection lost")
		}
	})
	t.Run("AC02", func(t *testing.T) {
		f := newVectorFixture(t)
		g, b := vectorGeneration("v1", "credential:v1", 1024, 2)
		f.stage(t, f.e, g, b)
		f.publish(t, f.e, g, 0)
		const legacyKey = "829aab48534d3913a21ba49d3555bd45b6fca31976c8172d1c546921fe562e30"
		var storedKey string
		if err := support.Raw(t, f.dsn).QueryRow(context.Background(), `SELECT space_key FROM chartworks.vector_generations WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 AND generation_id=$4`, f.e.Tenant(), g.Topic, g.Context, g.ID).Scan(&storedKey); err != nil || storedKey != legacyKey || g.Space.Key() != legacyKey {
			t.Fatal("preexisting phase 07 embedding space key changed", storedKey, g.Space.Key(), err)
		}
		if got := f.search(t, f.e, vectorQuery(g)); len(got) != 1 || got[0].Publication.Generation != g.ID || len(got[0].Hits) != len(b) {
			t.Fatal("preexisting ready generation became unreadable", got)
		}
		for _, change := range []func(*vindex.Space){func(s *vindex.Space) { s.Revision = "r2" }, func(s *vindex.Space) { s.Preprocessing = "different" }, func(s *vindex.Space) { s.InputType = "query" }, func(s *vindex.Space) { s.Normalization = "l2" }, func(s *vindex.Space) { s.Route = "other" }, func(s *vindex.Space) { s.Endpoint = "https://provider.example/v2" }} {
			bad := g.Clone()
			change(&bad.Space)
			if err := f.s.Upsert(context.Background(), f.e, bad, b); err == nil {
				t.Fatal("same-dimension incompatible upsert")
			}
			q := vectorQuery(bad)
			if _, err := f.s.Search(context.Background(), f.e, []vindex.Query{q}); !errors.Is(err, store.ErrConflict) {
				t.Fatal("space mismatch not rejected", err)
			}
		}
		bad := append([]vindex.Facet(nil), b...)
		bad[0].Vector = []float32{1}
		if err := f.s.Upsert(context.Background(), f.e, g, bad); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("wrong dimensions accepted")
		}
		g2, b2 := vectorGeneration("v2", "credential:v1", 1024, 4)
		g2.Space.Revision = "r2"
		f.stage(t, f.e, g2, b2)
		f.publish(t, f.e, g2, 1)
		got := f.search(t, f.e, vectorQuery(g2))[0]
		if got.Publication.Version != "v2" || len(got.Hits) != 4 {
			t.Fatal("generation activation mixed versions")
		}
		for _, h := range got.Hits {
			if h.Generation != "v2" {
				t.Fatal("old vector in new generation")
			}
		}
	})
	t.Run("AC03", func(t *testing.T) {
		f := newVectorFixture(t)
		g, b := vectorGeneration("v1", "credential:v1", 4, 6)
		for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		f.stage(t, f.e, g, b)
		f.publish(t, f.e, g, 0)
		q := vectorQuery(g)
		q.LimitPerKind = 2
		second := q
		second.ID = "repeated-text"
		got := f.search(t, f.e, q, second)
		scalar := f.search(t, f.e, q)
		if !reflect.DeepEqual(got[0], scalar[0]) || got[1].ID != "repeated-text" || len(got[0].Hits) != 4 {
			t.Fatal("batch/scalar semantics differ")
		}
		want := []string{"facet-0001", "facet-0003", "facet-0000", "facet-0002"}
		for i, h := range got[0].Hits {
			if h.ID != want[i] || h.SourceID != "origin-"+strings.TrimPrefix(h.ID, "facet-") {
				t.Fatal("unstable ties or lost origin", h)
			}
		}
		got[0].Hits[0].Text = "mutated"
		if next := f.search(t, f.e, q); next[0].Hits[0].Text == "mutated" {
			t.Fatal("mutable read-through state")
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newVectorFixture(t)
		ctx := context.Background()
		g, b := vectorGeneration("old", "credential:v1", 2, 2)
		f.stage(t, f.e, g, b)
		f.publish(t, f.e, g, 0)
		g2, b2 := vectorGeneration("new", "credential:v1", 2, 4)
		f.stage(t, f.e, g2, b2)
		p := f.publish(t, f.e, g2, 1)
		foreign := f.actor(t, "vector-b")
		f.stage(t, foreign, g, b)
		f.publish(t, foreign, g, 0)
		if err := f.s.Delete(ctx, f.e, vindex.Removal{Topic: g.Topic, Context: g.Context, Version: g.Version}); err != nil {
			t.Fatal(err)
		}
		if got := f.search(t, f.e, vectorQuery(g2)); len(got[0].Hits) != 4 {
			t.Fatal("delete affected other generation")
		}
		if _, err := f.s.Archive(ctx, f.e, g.Topic, g.Context, p.Revision); err != nil {
			t.Fatal(err)
		}
		if got := f.search(t, f.e, vectorQuery(g2)); !got[0].Publication.Archived || len(got[0].Hits) != 0 {
			t.Fatal("archive did not invalidate")
		}
		if _, err := f.s.Archive(ctx, f.e, g.Topic, g.Context, p.Revision); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale archive accepted")
		}
		if err := f.s.Delete(ctx, f.e, vindex.Removal{Topic: g.Topic, Context: g.Context}); err != nil {
			t.Fatal(err)
		}
		if got := f.search(t, foreign, vectorQuery(g)); len(got[0].Hits) != 2 {
			t.Fatal("foreign generation erased")
		}
		if err := f.s.Delete(ctx, foreign, vindex.Removal{}); err != nil {
			t.Fatal(err)
		}
		if got := f.search(t, foreign, vectorQuery(g)); len(got[0].Hits) != 0 {
			t.Fatal("tenant cleanup retained facets")
		}
	})
	t.Run("AC05", func(t *testing.T) {
		f := newVectorFixture(t)
		ctx := context.Background()
		g, b := vectorGeneration("old", "credential:v1", 3, 4)
		f.stage(t, f.e, g, b)
		f.publish(t, f.e, g, 0)
		g2, b2 := vectorGeneration("new", "credential:v1", 3, 4)
		f.stage(t, f.e, g2, b2[:1])
		if _, err := f.s.Publish(ctx, f.e, g2, 1); !errors.Is(err, store.ErrConflict) {
			t.Fatal("incomplete generation published")
		}
		if got := f.search(t, f.e, vectorQuery(g)); got[0].Publication.Generation != "old" {
			t.Fatal("interrupted embedding changed publication")
		}
		f.stage(t, f.e, g2, b2[1:])
		f.publish(t, f.e, g2, 1)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for revision := int64(2); revision < 22; revision++ {
				target := g
				if revision%2 == 1 {
					target = g2
				}
				if _, err := f.s.Publish(ctx, f.e, target, revision); err != nil {
					t.Error("concurrent publish", err)
					return
				}
			}
		}()
		for worker := 0; worker < 16; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for n := 0; n < 4; n++ {
					q := vectorQuery(g)
					q2 := q
					q2.ID = "second"
					results, err := f.s.Search(ctx, f.e, []vindex.Query{q, q2})
					if err != nil {
						t.Error(err)
						return
					}
					if results[0].Publication != results[1].Publication {
						t.Error("batch saw multiple snapshots")
					}
					for _, r := range results {
						if len(r.Hits) != 4 {
							t.Error("partial generation visible")
						}
						for _, h := range r.Hits {
							if h.Generation != r.Publication.Generation {
								t.Error("mixed generation")
							}
						}
					}
				}
			}()
		}
		wg.Wait()
		if err := f.s.Upsert(ctx, f.e, g, b); !errors.Is(err, store.ErrConflict) {
			t.Fatal("published vector mutated")
		}
		raw := support.Raw(t, f.dsn)
		if _, err := raw.Exec(ctx, `UPDATE chartworks.vector_generations SET version_id='tampered' WHERE tenant_id=$1`, f.e.Tenant()); err == nil {
			t.Fatal("immutable generation trigger missing")
		}
		if _, err := raw.Exec(ctx, `UPDATE chartworks.vector_facets SET body='tampered' WHERE tenant_id=$1`, f.e.Tenant()); err == nil {
			t.Fatal("sealed facet trigger missing")
		}
	})
	t.Run("AC06", func(t *testing.T) {
		f := newVectorFixture(t)
		ctx := context.Background()
		g, b := vectorGeneration("benchmark", "credential:v1", 1024, 128)
		f.stage(t, f.e, g, b)
		f.publish(t, f.e, g, 0)
		q := vectorQuery(g)
		plan, err := f.s.Explain(ctx, f.e, q)
		if err != nil || !json.Valid(plan) || !strings.Contains(string(plan), "vector_facets") {
			t.Fatal("real pgvector query plan absent", err)
		}
		var pg, version string
		raw := support.Raw(t, f.dsn)
		if err := raw.QueryRow(ctx, `SELECT current_setting('server_version'),extversion FROM pg_extension WHERE extname='vector'`).Scan(&pg, &version); err != nil {
			t.Fatal("pgvector unavailable", err)
		}
		queries := make([]vindex.Query, 8)
		for i := range queries {
			queries[i] = q
			queries[i].ID = fmt.Sprint("q", i)
		}
		start := time.Now()
		for n := 0; n < 8; n++ {
			f.search(t, f.e, queries...)
		}
		t.Logf("MEASURED pgvector=%s PostgreSQL=%s go=%s platform=%s/%s dimensions=1024 facets=128 warm_batches=8 queries_per_batch=8 elapsed_ns=%d", version, pg, runtime.Version(), runtime.GOOS, runtime.GOARCH, time.Since(start).Nanoseconds())
		t.Logf("QUERY_PLAN %s", plan)
	})
}

func TestVindexAdversarialBounds(t *testing.T) {
	f := newVectorFixture(t)
	ctx := context.Background()
	g, b := vectorGeneration("v1", "credential:v1", 2, 2)
	if _, err := vindex.New(nil); err == nil {
		t.Fatal("nil repository")
	}
	for _, v := range [][]float32{nil, {1}, {0, 0}, {float32(math.Inf(1)), 1}, {float32(math.NaN()), 1}} {
		if _, err := vindex.VectorLiteral(v, 2); err == nil {
			t.Fatal("invalid vector")
		}
	}
	for _, mutate := range []func(*vindex.Generation){func(g *vindex.Generation) { g.ID = "bad/" }, func(g *vindex.Generation) { g.Space.Dimensions = 16001 }, func(g *vindex.Generation) { g.Space.Model = "" }, func(g *vindex.Generation) { g.Space.Endpoint = "https://user:secret@provider.example" }, func(g *vindex.Generation) { g.Expected = nil }, func(g *vindex.Generation) { g.Expected = append(g.Expected, g.Expected[0]) }, func(g *vindex.Generation) { g.Expected[0].TextHash = "xyz" }, func(g *vindex.Generation) { g.Expected[0].Kind = "admin" }} {
		bad := g.Clone()
		mutate(&bad)
		if bad.Valid() || f.s.Begin(ctx, f.e, bad) == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	for _, mutate := range []func([]vindex.Facet){func(b []vindex.Facet) { b[0].ID = "foreign" }, func(b []vindex.Facet) { b[0].Kind = "rule" }, func(b []vindex.Facet) { b[0].SourceID = "foreign" }, func(b []vindex.Facet) { b[0].Text = "altered" }, func(b []vindex.Facet) { b[1] = b[0] }} {
		bad := append([]vindex.Facet(nil), b...)
		mutate(bad)
		if f.s.Upsert(ctx, f.e, g, bad) == nil {
			t.Fatal("misassociated output")
		}
	}
	f.stage(t, f.e, g, b)
	changed := g.Clone()
	changed.Version = "other"
	if !errors.Is(f.s.Begin(ctx, f.e, changed), store.ErrConflict) {
		t.Fatal("generation key overwritten")
	}
	if _, err := f.s.Publish(ctx, f.e, g, -1); err == nil {
		t.Fatal("unversioned publication")
	}
	if _, err := f.s.Publish(ctx, f.e, g, 1); !errors.Is(err, store.ErrConflict) {
		t.Fatal("wrong publication fence")
	}
	f.publish(t, f.e, g, 0)
	q := vectorQuery(g)
	for _, mutate := range []func(*vindex.Query){func(q *vindex.Query) { q.ID = "" }, func(q *vindex.Query) { q.Kinds = nil }, func(q *vindex.Query) { q.Kinds = []string{"rule", "rule"} }, func(q *vindex.Query) { q.Kinds = []string{"admin"} }, func(q *vindex.Query) { q.LimitPerKind = 11 }, func(q *vindex.Query) { q.Vector = nil }} {
		bad := q
		mutate(&bad)
		if _, err := f.s.Search(ctx, f.e, []vindex.Query{bad}); err == nil {
			t.Fatal("invalid query")
		}
		if _, err := f.s.Explain(ctx, f.e, bad); err == nil {
			t.Fatal("invalid diagnostic")
		}
	}
	if _, err := f.s.Search(ctx, f.e, nil); err == nil {
		t.Fatal("empty batch")
	}
	if _, err := f.s.Search(ctx, f.e, []vindex.Query{q, q}); err == nil {
		t.Fatal("duplicate query origin")
	}
	bare := f.token.envelope(t, f.e.Tenant(), "reader")
	if f.s.Begin(ctx, bare, g) == nil || f.s.Upsert(ctx, bare, g, b) == nil || f.s.Delete(ctx, bare, vindex.Removal{}) == nil {
		t.Fatal("missing authority")
	}
	if _, err := f.s.Publish(ctx, bare, g, 1); err == nil {
		t.Fatal("unauthorized publish")
	}
	if _, err := f.s.Archive(ctx, bare, g.Topic, g.Context, 1); err == nil {
		t.Fatal("unauthorized archive")
	}
	if _, err := f.s.Explain(ctx, bare, q); err == nil {
		t.Fatal("unauthorized plan")
	}
	if f.s.Delete(ctx, f.e, vindex.Removal{Version: "v1"}) == nil || f.s.Delete(ctx, f.e, vindex.Removal{Topic: g.Topic, Context: g.Context, Version: "bad/"}) == nil {
		t.Fatal("ambiguous cleanup")
	}
	if _, err := f.s.Archive(ctx, f.e, g.Topic, g.Context, 0); err == nil {
		t.Fatal("unversioned archive")
	}
	if err := f.s.Delete(ctx, f.e, vindex.Removal{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Explain(ctx, f.e, q); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("empty generation plan", err)
	}
}
