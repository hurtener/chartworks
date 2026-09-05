package acceptance

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
)

func TestVindexRejectsUnstableCosine(t *testing.T) {
	f := newVectorFixture(t)
	ctx := context.Background()
	raw := support.Raw(t, f.dsn)
	// Native pgvector 0.8.2 accumulates float32 products even for finite inputs.
	var native float64
	if err := raw.QueryRow(ctx, `SELECT '[1e30,0]'::public.vector OPERATOR(public.<=>) '[1e30,0]'::public.vector`).Scan(&native); err != nil || !math.IsNaN(native) {
		t.Fatal("native finite-input cosine instability was not reproduced", native, err)
	}
	g, batch := vectorGeneration("numeric", "credential:v1", 2, 2)
	f.stage(t, f.e, g, batch)
	for _, magnitude := range []float32{1e30, 1e-30, math.MaxFloat32, math.SmallestNonzeroFloat32} {
		bad := append([]vindex.Facet(nil), batch...)
		bad[0].Vector = []float32{magnitude, 0}
		if err := f.s.Upsert(ctx, f.e, g, bad); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("unsafe cosine magnitude admitted", magnitude, err)
		}
		q := vectorQuery(g)
		q.Vector = bad[0].Vector
		if out, err := f.s.Search(ctx, f.e, []vindex.Query{q}); !errors.Is(err, store.ErrInvalid) || out != nil {
			t.Fatal("unsafe cosine query admitted", magnitude, err)
		}
	}
	for _, literal := range []string{"[1e30,0]", "[1e-30,0]"} {
		if _, err := raw.Exec(ctx, `UPDATE chartworks.vector_facets SET embedding=$2::public.vector WHERE tenant_id=$1`, f.e.Tenant(), literal); err == nil {
			t.Fatal("database accepted an unstable cosine norm")
		}
	}
	f.publish(t, f.e, g, 0)
	out := f.search(t, f.e, vectorQuery(g))
	if len(out[0].Hits) != 2 || out[0].Hits[0].Distance != 0 {
		t.Fatal("normal vectors lost correct cosine behavior", out)
	}
	// A compromised/changed backend must not inject a non-JSON numeric result.
	// This replaces the function in this disposable database only.
	if _, err := raw.Exec(ctx, `CREATE OR REPLACE FUNCTION public.cosine_distance(public.vector, public.vector) RETURNS double precision LANGUAGE sql IMMUTABLE STRICT AS $$ SELECT 'NaN'::double precision $$`); err != nil {
		t.Fatal(err)
	}
	if out, err := f.s.Search(ctx, f.e, []vindex.Query{vectorQuery(g)}); !errors.Is(err, store.ErrInvalid) || out != nil {
		t.Fatal("nonfinite backend distance escaped the atomic response boundary", out, err)
	}
}
