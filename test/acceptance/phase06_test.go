package acceptance

import (
 "context"
 "errors"
 "fmt"
 "strings"
 "sync"
 "testing"
 "time"

 "github.com/hurtener/chartworks/internal/jobs"
 "github.com/hurtener/chartworks/internal/store"
 "github.com/hurtener/chartworks/test/support"
)

func TestPhase06(t *testing.T){
 t.Run("AC01",func(t *testing.T){
  q:=newQueueFixture(t,nil);var wg sync.WaitGroup;ids:=make(chan string,32)
  for i:=0;i<32;i++{wg.Add(1);go func(){defer wg.Done();j,err:=q.service.Submit(context.Background(),q.actor,"same-key",jobs.Submission{Kind:jobs.MaintenanceKind,BindingID:"maintenance"});if err!=nil{t.Error(err);return};ids<-j.ID}()};wg.Wait();close(ids)
  unique:=map[string]bool{};for id:=range ids{unique[id]=true};if len(unique)!=1{t.Fatalf("idempotency produced %d jobs",len(unique))}
  if _,err:=q.service.Submit(context.Background(),q.actor,"same-key",jobs.Submission{Kind:jobs.MaintenanceKind,BindingID:"other"});!errors.Is(err,store.ErrConflict){t.Fatalf("binding change did not conflict: %v",err)}
  list,err:=q.service.List(context.Background(),q.actor,100);if err!=nil||len(list)!=1{t.Fatal("duplicate accepted operations")}
  raw:=support.Raw(t,q.dsn);if _,err:=raw.Exec(context.Background(),`UPDATE chartworks.operations SET due_at=due_at+interval '1 hour' WHERE operation_id=$1`,list[0].ID);err==nil{t.Fatal("accepted manifest was mutable")}
 })
 t.Run("AC02",func(t *testing.T){
  q:=newQueueFixture(t,nil);q.oldAudit(t);j:=q.submit(t,"fencing")
  first,err:=q.db.ClaimJob(context.Background(),"worker-first",q.limits);if err!=nil{t.Fatal(err)}
  raw:=support.Raw(t,q.dsn);if _,err:=raw.Exec(context.Background(),`UPDATE chartworks.operations SET lease_until=clock_timestamp()-interval '1 second' WHERE operation_id=$1`,j.ID);err!=nil{t.Fatal(err)}
  second,err:=q.db.ClaimJob(context.Background(),"worker-second",q.limits);if err!=nil||second.Fence<=first.Fence{t.Fatal("reclaim did not fence")}
  authority,err:=q.provider.Acquire(context.Background(),j);if err!=nil{t.Fatal(err)}
  if _,err:=q.db.CompleteJob(context.Background(),first,authority);!errors.Is(err,store.ErrConflict){t.Fatalf("stale completion: %v",err)}
  if q.oldAuditCount(t)!=1{t.Fatal("stale worker had effects")}
  finished,err:=q.db.CompleteJob(context.Background(),second,authority);if err!=nil||finished.State!="succeeded"||q.oldAuditCount(t)!=0{t.Fatalf("winner completion: %v",err)}
  var count int;if err:=raw.QueryRow(context.Background(),`SELECT count(*) FROM chartworks.operation_attempts WHERE operation_id=$1 AND state='abandoned'`,j.ID).Scan(&count);err!=nil||count!=1{t.Fatal("lost attempt not recorded")}
 })
 t.Run("AC03",func(t *testing.T){
  q:=newQueueFixture(t,nil)
  for _,kind:=range []string{"event","condition","condition_check","custom","shell"}{
   spec:=jobs.Spec{Type:kind,Timezone:"UTC",Missed:"skip",Overlap:"skip"};if spec.Validate()==nil{t.Fatalf("stub trigger accepted: %s",kind)}
   if _,err:=q.service.Submit(context.Background(),q.actor,"unsupported",jobs.Submission{Kind:kind,BindingID:"maintenance"});err==nil{t.Fatal("unimplemented handler admitted")}
  }
  request:=jobs.ScheduleRequest{Target:jobs.Submission{Kind:jobs.MaintenanceKind,BindingID:"maintenance"},Spec:jobs.Spec{Type:"manual",Timezone:"UTC",Missed:"skip",Overlap:"queue"}}
  schedule,err:=q.service.CreateSchedule(context.Background(),q.actor,"manual",request);if err!=nil{t.Fatal(err)}
  first,err:=q.service.Fire(context.Background(),q.actor,schedule.ID,"fire");if err!=nil{t.Fatal(err)};second,err:=q.service.Fire(context.Background(),q.actor,schedule.ID,"fire");if err!=nil||first.ID!=second.ID{t.Fatal("manual occurrence replay changed")}
  if err:=q.service.RunOnce(context.Background());err!=nil{t.Fatal(err)}
  for _,spec:=range []jobs.Spec{{Type:"cron",Cron:"30 2 * * *",Timezone:"America/New_York",Missed:"skip",Overlap:"skip"},{Type:"interval",IntervalSeconds:60,Anchor:time.Date(2026,1,1,0,0,0,0,time.UTC),Timezone:"UTC",Missed:"catch_up",MaxCatchUp:2,Overlap:"queue"}}{if _,err:=q.service.CreateSchedule(context.Background(),q.actor,fmt.Sprint(spec.Type),jobs.ScheduleRequest{Target:request.Target,Spec:spec});err!=nil{t.Fatal(err)}}
 })
 t.Run("AC04",func(t *testing.T){
  q:=newQueueFixture(t,func(l *jobs.Limits){l.MaxPending=2;l.MaxPendingPerTenant=1})
  j:=q.submit(t,"capacity");if _,err:=q.service.Submit(context.Background(),q.actor,"over-capacity",jobs.Submission{Kind:jobs.MaintenanceKind,BindingID:"maintenance"});!errors.Is(err,jobs.ErrBusy){t.Fatalf("pending limit: %v",err)}
  if _,err:=q.service.Cancel(context.Background(),q.actor,j.ID);err!=nil{t.Fatal(err)};if err:=q.service.RunOnce(context.Background());!errors.Is(err,jobs.ErrEmpty){t.Fatal("cancelled job claimed")}
  q.submit(t,"cancel-at-dispatch");q.mode.Store(9)
  ctx,cancel:=context.WithCancel(context.Background());done:=make(chan error,1);go func(){done<-q.service.Run(ctx)}()
  deadline:=time.Now().Add(3*time.Second);for q.calls.Load()==0&&time.Now().Before(deadline){time.Sleep(5*time.Millisecond)}
  if q.calls.Load()==0{cancel();t.Fatal("worker did not dispatch")}
  if err:=q.service.Run(context.Background());!errors.Is(err,jobs.ErrRunning){cancel();t.Fatal("duplicate worker lifecycle")}
  cancel();select{case err:=<-done:if err!=nil{t.Fatal(err)};case<-time.After(2*time.Second):t.Fatal("shutdown did not join cancelled worker")}
 })
 t.Run("AC05",func(t *testing.T){
  q:=newQueueFixture(t,nil);j:=q.submit(t,"retry-window");q.mode.Store(2)
  if err:=q.service.RunOnce(context.Background());err==nil{t.Fatal("broker outage hidden")}
  pending,err:=q.service.Get(context.Background(),q.actor,j.ID);if err!=nil||pending.State!="retry"||pending.Digest()!=j.ManifestHash{t.Fatal("retry changed manifest")}
  raw:=support.Raw(t,q.dsn);if _,err:=raw.Exec(context.Background(),`UPDATE chartworks.operations SET next_attempt_at=clock_timestamp() WHERE operation_id=$1`,j.ID);err!=nil{t.Fatal(err)}
  q.mode.Store(0);if err:=q.service.RunOnce(context.Background());err!=nil{t.Fatal(err)}
  completed,err:=q.service.Get(context.Background(),q.actor,j.ID);if err!=nil||completed.State!="succeeded"||!completed.DueAt.Equal(j.DueAt)||!completed.WindowStart.Equal(j.WindowStart)||completed.Attempts!=2{t.Fatal("retry did not preserve accepted temporal coordinates")}
  q.mu.Lock();for _,r:=range q.requests{if r["job"]!=j.ID||r["manifest"]!=j.ManifestHash{t.Error("broker request changed accepted target")}};q.mu.Unlock()
  spec:=jobs.Spec{Type:"interval",IntervalSeconds:60,Anchor:time.Date(2026,1,1,0,0,0,0,time.UTC),Timezone:"UTC",Missed:"catch_up",MaxCatchUp:2,Overlap:"queue"}
  s,err:=q.service.CreateSchedule(context.Background(),q.actor,"catchup",jobs.ScheduleRequest{Target:jobs.Submission{Kind:jobs.MaintenanceKind,BindingID:"maintenance"},Spec:spec});if err!=nil{t.Fatal(err)}
  var now time.Time;if err:=raw.QueryRow(context.Background(),`SELECT clock_timestamp()`).Scan(&now);err!=nil{t.Fatal(err)};due:=now.Truncate(time.Minute).Add(-3*time.Minute)
  if _,err:=raw.Exec(context.Background(),`UPDATE chartworks.job_schedules SET next_due=$2,previous_due=$2-interval '1 minute' WHERE schedule_id=$1`,s.ID,due);err!=nil{t.Fatal(err)}
  count,err:=q.db.TickSchedules(context.Background(),q.limits);if err!=nil||count!=2{t.Fatalf("catchup count %d: %v",count,err)}
  if count,err:=q.db.TickSchedules(context.Background(),q.limits);err!=nil||count!=0{t.Fatal("occurrence duplicated after restart")}
  var skipped int;if err:=raw.QueryRow(context.Background(),`SELECT count(*) FROM chartworks.job_occurrences WHERE schedule_id=$1 AND disposition='missed_skipped' AND skipped_through IS NOT NULL`,s.ID).Scan(&skipped);err!=nil||skipped!=1{t.Fatal("bounded catchup range not explicit")}
 })
 t.Run("AC06",func(t *testing.T){
  q:=newQueueFixture(t,nil);q.oldAudit(t)
  for mode:=int64(1);mode<=8;mode++{if mode==2{continue};q.mode.Store(mode);j:=q.submit(t,fmt.Sprintf("denied-%d",mode))
   if err:=q.service.RunOnce(context.Background());err==nil{t.Fatalf("bad fresh authority %d executed",mode)}
   after,err:=q.service.Get(context.Background(),q.actor,j.ID);if err!=nil||after.State!="blocked"||q.oldAuditCount(t)!=1{t.Fatalf("authority mode %d effects/state: %s %v",mode,after.State,err)}
  }
  q.mode.Store(0);j:=q.submit(t,"valid-broker");if err:=q.service.RunOnce(context.Background());err!=nil{t.Fatal(err)};if q.oldAuditCount(t)!=0{t.Fatal("valid fresh authority did not perform maintenance")}
  raw:=support.Raw(t,q.dsn);var stored string;if err:=raw.QueryRow(context.Background(),`SELECT row_to_json(o)::text FROM chartworks.operations o WHERE operation_id=$1`,j.ID).Scan(&stored);err!=nil{t.Fatal(err)}
  q.mu.Lock();defer q.mu.Unlock();for _,token:=range q.tokens{if strings.Contains(stored,token){t.Fatal("JWT persisted")}};if strings.Contains(stored,"SYNTHETIC_BROKER_SECRET"){t.Fatal("broker credential persisted")}
  // The matching Pengui companion tests exercise its real vault, binding loader and signer.
  // This path exercises the actual HTTP consumer, JWKS verifier and PostgreSQL effect, not a fake Authority implementation.
 })
}
