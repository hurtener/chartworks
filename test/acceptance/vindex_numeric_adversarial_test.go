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

func TestVindexFiniteCosineDomainAndSealedErasure(t *testing.T) {
	f:=newVectorFixture(t); ctx:=context.Background()
	g,b:=vectorGeneration("published","credential:v1",3,2)
	f.stage(t,f.e,g,b); f.publish(t,f.e,g,0)
	for _,value:=range []float32{math.MaxFloat32,math.SmallestNonzeroFloat32,1e30,1e-30} {
		if _,err:=vindex.VectorLiteral([]float32{value,value,value},3); !errors.Is(err,store.ErrInvalid) { t.Fatal("finite vector produced unsafe float32 cosine norm",value,err) }
		q:=vectorQuery(g); q.Vector=[]float32{value,value,value}
		if _,err:=f.s.Search(ctx,f.e,[]vindex.Query{q}); !errors.Is(err,store.ErrInvalid) { t.Fatal("unsafe query vector reached distance calculation",err) }
	}
	raw:=support.Raw(t,f.dsn)
	if _,err:=raw.Exec(ctx,`DELETE FROM chartworks.vector_facets WHERE tenant_id=$1 AND facet_id='facet-0000'`,f.e.Tenant()); err==nil { t.Fatal("direct erasure made a sealed generation partial") }
	staging,stageBatch:=vectorGeneration("staging","credential:v1",3,2)
	if err:=f.s.Begin(ctx,f.e,staging); err!=nil { t.Fatal(err) }
	if _,err:=raw.Exec(ctx,`UPDATE chartworks.vector_facets SET generation_id='staging' WHERE tenant_id=$1 AND generation_id='published'`,f.e.Tenant()); err==nil { t.Fatal("moving a sealed facet into staging bypassed immutability") }
	for _,literal:=range []string{"[1e30,1e30,1e30]","[1e-30,1e-30,1e-30]"} {
		if _,err:=raw.Exec(ctx,`INSERT INTO chartworks.vector_facets(tenant_id,topic_id,context_id,generation_id,facet_id,kind,source_id,body,space_key,dimensions,embedding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,3,$10::public.vector)`,f.e.Tenant(),staging.Topic,staging.Context,staging.ID,stageBatch[0].ID,stageBatch[0].Kind,stageBatch[0].SourceID,stageBatch[0].Text,staging.Space.Key(),literal); err==nil { t.Fatal("SQL norm constraint accepted unsafe finite vector") }
	}
	if got:=f.search(t,f.e,vectorQuery(g)); len(got[0].Hits)!=2 { t.Fatal("failed tampering changed sealed evidence") }
	if err:=f.s.Delete(ctx,f.e,vindex.Removal{Topic:g.Topic,Context:g.Context,Version:g.Version}); err!=nil { t.Fatal("legitimate parent cascade blocked",err) }
	if got:=f.search(t,f.e,vectorQuery(g)); len(got[0].Hits)!=0 { t.Fatal("authorized erasure failed") }
}
