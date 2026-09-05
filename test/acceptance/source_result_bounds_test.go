package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestSourceResultBoundsAndRecovery(t *testing.T) {
	t.Run("rows",func(t *testing.T){
		f:=newSourceFixture(t,func(c *config.Sources){c.MaxRows=1})
		s:=f.create(t,"sales")
		p:=f.plan(t,s,`SELECT id FROM analytics.sales ORDER BY id`)
		out,err:=f.s.Read(context.Background(),f.e,p)
		if !errors.Is(err,readexec.ErrLimit) || out.Values!=nil { t.Fatal("row limit returned partial evidence",err) }
		p=f.plan(t,s,`SELECT id FROM analytics.sales WHERE id=1`)
		if out,err=f.s.Read(context.Background(),f.e,p); err!=nil || len(out.Values)!=1 { t.Fatal("pool did not recover after bounded cancellation",err) }
	})
	t.Run("bytes",func(t *testing.T){
		f:=newSourceFixture(t,func(c *config.Sources){c.MaxBytes=1024})
		s:=f.create(t,"sales")
		if _,err:=f.admin.Exec(context.Background(),`UPDATE analytics.sales SET name=repeat('x',2048) WHERE id=1`); err!=nil { t.Fatal(err) }
		p:=f.plan(t,s,`SELECT name FROM analytics.sales WHERE id=1`)
		out,err:=f.s.Read(context.Background(),f.e,p)
		if !errors.Is(err,readexec.ErrLimit) || out.Values!=nil { t.Fatal("byte limit returned partial evidence",err) }
		if _,err=f.admin.Exec(context.Background(),`UPDATE analytics.sales SET name=repeat('x',65536) WHERE id=1`); err!=nil { t.Fatal(err) }
		out,err=f.s.Read(context.Background(),f.e,p)
		if err==nil || out.Values!=nil { t.Fatal("oversized protocol message was not rejected") }
		if _,err=f.admin.Exec(context.Background(),`UPDATE analytics.sales SET name='restored' WHERE id=1`); err!=nil { t.Fatal(err) }
		if out,err=f.s.Read(context.Background(),f.e,p); err!=nil || len(out.Values)!=1 || *out.Values[0][0]!="restored" { t.Fatal("pool did not recover after protocol bound",err) }
	})
}
