package acceptance

import (
 "bytes"
 "context"
 "errors"
 "io"
 "net"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/hurtener/chartworks/internal/config"
 "github.com/hurtener/chartworks/internal/foundation"
 "github.com/hurtener/chartworks/internal/maintenance"
 "github.com/hurtener/chartworks/internal/store"
 "github.com/hurtener/chartworks/internal/telemetry"
 "github.com/hurtener/chartworks/test/support"
)

func checkOperationExpiry(t *testing.T){
 t.Helper();ctx:=context.Background();dsn:=support.Database(t);db:=support.Open(t,dsn);scope:=support.Scope(t,"expiry","actor");policy(t,db,scope)
 c:=support.Raw(t,dsn);service,e:=maintenance.New(db,10,3*time.Second);if e!=nil{t.Fatal(e)}
 op,e:=service.Sweep(ctx,scope,"expired-key");if e!=nil{t.Fatal(e)}
 sql(t,c,`DELETE FROM chartworks.audit_events WHERE tenant_id='expiry' AND operation_id=$1`,op.ID)
 sql(t,c,`UPDATE chartworks.operations SET created_at=clock_timestamp()-interval '2 days',expires_at=clock_timestamp()-interval '1 day' WHERE tenant_id='expiry' AND operation_id=$1`,op.ID)
 if _,e=service.Sweep(ctx,scope,"expired-key");!errors.Is(e,store.ErrExpired){t.Fatal("expired key silently reran")}
 if _,e=service.Sweep(ctx,scope,"compaction");e!=nil{t.Fatal(e)}
 if count(t,c,`SELECT count(*) FROM chartworks.operations WHERE tenant_id='expiry' AND operation_id=$1 AND status='expired'`,op.ID)!=1{t.Fatal("idempotency tombstone missing")}
 if _,e=service.Sweep(ctx,scope,"expired-key");!errors.Is(e,store.ErrExpired){t.Fatal("compacted key silently reran")}
 pending,e:=db.ReserveSweep(ctx,scope,"limit-key",hash("same"),10);if e!=nil{t.Fatal(e)}
 if _,e=db.ReserveSweep(ctx,scope,"limit-key",hash("same"),20);!errors.Is(e,store.ErrConflict){t.Fatal("same hash bypassed manifest limits")}
 if _,e=db.Claim(ctx,scope,pending.ID,"worker",0);!errors.Is(e,store.ErrInvalid){t.Fatal("invalid lease TTL")}
 if e=db.Renew(ctx,scope,store.Lease{},time.Second);!errors.Is(e,store.ErrInvalid){t.Fatal("invalid lease")}
 if _,e=db.CommitSweep(ctx,scope,store.Lease{});!errors.Is(e,store.ErrInvalid){t.Fatal("empty fence")}
 if _,e=db.ReserveSweep(ctx,scope,"bad-key","not-a-hash",1);!errors.Is(e,store.ErrInvalid){t.Fatal("invalid request hash")}
 if _,e=db.ReserveSweep(ctx,support.Scope(t,"missing-policy","actor"),"key",hash("x"),1);!errors.Is(e,store.ErrNotFound){t.Fatal("invented implicit policy")}
 // Real PostgreSQL delay forces expiry after deletion began but before its final fenced write.
 sql(t,c,`INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id,created_at) VALUES('expiry',repeat('c',32),'actor','retention_policy.updated','retention',clock_timestamp()-interval '30 days')`)
 sql(t,c,`CREATE FUNCTION chartworks.delay_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.2); RETURN OLD; END; $$; CREATE TRIGGER delay_delete BEFORE DELETE ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.delay_delete()`)
 lease,e:=db.Claim(ctx,scope,pending.ID,"worker",100*time.Millisecond);if e!=nil{t.Fatal(e)}
 if _,e=db.CommitSweep(ctx,scope,lease);!errors.Is(e,store.ErrConflict){t.Fatal("mid-transaction expiry committed")}
 if count(t,c,`SELECT count(*) FROM chartworks.audit_events WHERE tenant_id='expiry' AND event_id=repeat('c',32)`)!=1{t.Fatal("expired commit left partial deletion")}
 sql(t,c,`DROP TRIGGER delay_delete ON chartworks.audit_events`)
 next,e:=db.Claim(ctx,scope,pending.ID,"new-worker",time.Second);if e!=nil{t.Fatal(e)}
 if _,e=service.Configure(ctx,scope,1,store.Policy{AuditDays:30,OperationHours:24});e!=nil{t.Fatal(e)}
 if _,e=db.CommitSweep(ctx,scope,next);!errors.Is(e,store.ErrConflict){t.Fatal("changed retention policy silently applied old destructive work")}
 db.Close();if _,e=db.Policy(ctx,scope);!errors.Is(e,store.ErrUnavailable){t.Fatal("closed driver accepted work")}
 if _,e=maintenance.New(nil,1,time.Second);!errors.Is(e,store.ErrInvalid){t.Fatal("nil store defaulted")}
}

func checkRealStartup(t *testing.T){
 t.Helper();dsn:=support.Database(t)
 l,e:=net.Listen("tcp","127.0.0.1:0");if e!=nil{t.Fatal(e)};address:=l.Addr().String();if e=l.Close();e!=nil{t.Fatal(e)}
 data:=configBytes(t,func(m map[string]any){m["server"]=map[string]any{"listen":address};m["auth"].(map[string]any)["jwks_url"]="https://127.0.0.1:1/keys";m["auth"].(map[string]any)["request_timeout"]="100ms"})
 cfg:=loaded(t,data,dsn);ctx,cancel:=context.WithCancel(context.Background());defer cancel();done:=make(chan error,1)
 start:=time.Now();go func(){done<-foundation.Start(ctx,cfg,io.Discard)}()
 client:=&http.Client{Timeout:time.Second}
 waitFor(t,func()bool{code,_:=response(t,client,"http://"+address,"/healthz");return code==200})
 t.Logf("real startup to liveness including fresh schema migration: %s",time.Since(start))
 if code,_:=response(t,client,"http://"+address,"/readyz");code!=503{t.Fatal("unverified key health pretended ready")}
 cancel();select{case e:=<-done:if e!=nil{t.Fatal(e)};case<-time.After(2*time.Second):t.Fatal("real startup lifecycle did not join")}
 // Binding conflict is a safe error, and the opened store is closed on failure.
 blocker,e:=net.Listen("tcp",address);if e!=nil{t.Fatal(e)};defer func(){_ = blocker.Close()}()
 if e=foundation.Start(context.Background(),cfg,io.Discard);e==nil{t.Fatal("occupied listener accepted")}
 invalid:=loaded(t,data,"://"+canary);if e=foundation.Start(context.Background(),invalid,io.Discard);e==nil||strings.Contains(e.Error(),canary){t.Fatal("unsafe connection failure")}
 if e=foundation.Start(context.Background(),cfg,nil);e==nil{t.Fatal("nil logging target accepted")}
}
func TestCommandAndMetricsNegatives(t *testing.T){
 r,e:=telemetry.New(io.Discard,"json",false);if e!=nil{t.Fatal(e)};if e=r.Record(context.Background(),telemetry.Started,true);e!=nil{t.Fatal(e)};if e=r.Dependency("store",false);e!=nil{t.Fatal(e)}
 w:=httptest.NewRecorder();r.Handler().ServeHTTP(w,httptest.NewRequest("GET","/metrics",nil));if w.Code!=404{t.Fatal("disabled exporter active")}
 var output bytes.Buffer;b:=foundation.Build{Version:"test"}
 if foundation.Command(context.Background(),[]string{"config-check"},nil,&output,&output,b,nil)!=2{t.Fatal("nil environment")}
 if foundation.Command(context.Background(),[]string{"config-check"},func(string)(string,bool){return "",false},&output,&output,b,nil)!=2{t.Fatal("missing file default")}
 if foundation.Command(context.Background(),[]string{"config-check","--defaults"},environment("fixture"),badWriter{},&output,b,nil)!=1{t.Fatal("defaults output failure")}
 path:=filepath.Join(t.TempDir(),"bad.json");if e=os.WriteFile(path,[]byte(`{"store":{"dsn":"`+canary+`"}}`),0600);e!=nil{t.Fatal(e)}
 if foundation.Command(context.Background(),[]string{"config-check","--config",path},environment("fixture"),&output,&output,b,nil)!=2||strings.Contains(output.String(),canary){t.Fatal("config error unsafe")}
 if e=store.Policy{}.Validate();e==nil{t.Fatal("empty policy")}
}
var _ config.Duration=config.Duration(time.Second)
