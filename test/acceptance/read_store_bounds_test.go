package acceptance

import (
 "context"
 "errors"
 "strings"
 "testing"
 "time"

 readexec "github.com/hurtener/chartworks/internal/exec"
 "github.com/hurtener/chartworks/internal/store"
 "github.com/hurtener/chartworks/test/support"
)

func TestReadStoreBoundsAndAtomicAdmission(t *testing.T) {
 f:=newSourceFixture(t,nil);source:=f.create(t,"sales");p:=f.plan(t,source,`SELECT id FROM analytics.sales`)
 ctx:=context.Background();scope,err:=store.NewScope(f.e.Tenant(),f.e.User());if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 a:=readexec.Attempt{ID:strings.Repeat("c",32),Number:1,Manifest:readexec.Manifest{Operation:"store-bounds",Session:f.e.Session(),Receipt:p.Receipt(),Limits:readexec.Limits{Rows:10,Bytes:4096,Timeout:time.Second,CancelGrace:time.Second,PlannerCost:100000}},Created:now,Deadline:now.Add(time.Second)}
 q:=readexec.RemoteQuery{PID:1,Started:now,Tag:"cw-read:"+a.ID}
 for _,call:=range []func()error{
  func()error{return f.db.BeginRead(ctx,store.Scope{},a,3)},func()error{return f.db.DispatchRead(ctx,store.Scope{},a.ID,q,false)},func()error{_,e:=f.db.GetRead(ctx,store.Scope{},a.ID);return e},func()error{_,e:=f.db.GetReadOperation(ctx,store.Scope{},"store-bounds");return e},func()error{return f.db.CancelRead(ctx,store.Scope{},a.ID)},func()error{return f.db.FinishRead(ctx,store.Scope{},a,false)},
 }{if !errors.Is(call(),store.ErrScope){t.Fatal("unscoped attempt store access")}}
 for _,call:=range []func()error{
  func()error{return f.db.BeginRead(ctx,scope,a,0)},func()error{return f.db.DispatchRead(ctx,scope,a.ID,readexec.RemoteQuery{},false)},func()error{_,e:=f.db.GetRead(ctx,scope,"../bad");return e},func()error{_,e:=f.db.GetReadOperation(ctx,scope,"../bad");return e},func()error{return f.db.CancelRead(ctx,scope,"../bad")},func()error{return f.db.FinishRead(ctx,scope,a,false)},
 }{if !errors.Is(call(),store.ErrInvalid){t.Fatal("invalid attempt coordinates accepted")}}
 meta:=support.Raw(t,f.dsn)
 sql(t,meta,`CREATE FUNCTION chartworks.fail_read_admission() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='read.accepted' THEN RAISE EXCEPTION 'synthetic admission failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_read_admission BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.fail_read_admission()`)
 if err=f.db.BeginRead(ctx,scope,a,3);err==nil{t.Fatal("failed audit allowed partial admission")}
 if _,err=f.db.GetRead(ctx,scope,a.ID);!errors.Is(err,store.ErrNotFound){t.Fatal("partial attempt remained",err)}
 sql(t,meta,`DROP TRIGGER fail_read_admission ON chartworks.audit_events`)
 if err=f.db.BeginRead(ctx,scope,a,3);err!=nil{t.Fatal(err)}
 if err=f.db.DispatchRead(ctx,scope,a.ID,q,false);err!=nil{t.Fatal(err)}
 if err=f.db.DispatchRead(ctx,scope,a.ID,q,true);err!=nil{t.Fatal(err)}
 if err=f.db.DispatchRead(ctx,scope,a.ID,q,true);!errors.Is(err,readexec.ErrCancelled){t.Fatal("dispatch acknowledgment replayed",err)}
 bad:=a;bad.Status="invented";bad.Finished=&now
 if err=f.db.FinishRead(ctx,scope,bad,false);!errors.Is(err,store.ErrInvalid){t.Fatal("invented terminal state")}
 bad.Status="succeeded";bad.RemoteState="stopped"
 if err=f.db.FinishRead(ctx,scope,bad,true);!errors.Is(err,store.ErrInvalid){t.Fatal("reconciliation invented successful values")}
 bad.Status="failed";bad.Code="source_unavailable"
 if err=f.db.FinishRead(ctx,scope,bad,false);err!=nil{t.Fatal(err)}
 if err=f.db.CancelRead(ctx,scope,a.ID);!errors.Is(err,store.ErrConflict){t.Fatal("terminal cancellation reopened receipt",err)}
 if err=f.db.FinishRead(ctx,scope,bad,false);!errors.Is(err,store.ErrConflict){t.Fatal("terminal receipt overwritten",err)}
 gap:=a;gap.Number=3;gap.ID=strings.Repeat("d",32)
 if err=f.db.BeginRead(ctx,scope,gap,3);!errors.Is(err,store.ErrConflict){t.Fatal("attempt numbering skipped",err)}
 first:=a;first.Manifest.Operation="missing-first";first.Number=2;first.ID=strings.Repeat("e",32)
 if err=f.db.BeginRead(ctx,scope,first,3);!errors.Is(err,store.ErrConflict){t.Fatal("attempt without predecessor",err)}
}
