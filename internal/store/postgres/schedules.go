package postgres

import (
 "context"
 "encoding/json"
 "errors"
 "time"

 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/jobs"
 "github.com/hurtener/chartworks/internal/store"
 "github.com/jackc/pgx/v5"
)

const scheduleColumns=`schedule_id,revision,enabled,tenant_id,creator_id,creator_session,request,next_due,previous_due`
func scanSchedule(row pgx.Row)(jobs.Schedule,error){
 var s jobs.Schedule;var raw []byte
 err:=row.Scan(&s.ID,&s.Revision,&s.Enabled,&s.Tenant,&s.Initiator,&s.InitiatorSession,&raw,&s.NextDue,&s.PreviousDue);if err!=nil{return s,err}
 if json.Unmarshal(raw,&s.Request)!=nil||s.Request.Validate()!=nil{return jobs.Schedule{},jobs.ErrInvalid};return s,nil
}
func scheduleTimes(spec jobs.Spec,now time.Time)(previous,next *time.Time,err error){
 if spec.Type=="manual"{return nil,nil,nil}
 n,err:=spec.Next(now);if err!=nil{return nil,nil,err};p,err:=spec.Previous(now);if err!=nil{return nil,nil,err};return &p,&n,nil
}
func(d *DB)CreateSchedule(ctx context.Context,scope store.Scope,session,key string,request jobs.ScheduleRequest,l jobs.Limits)(out jobs.Schedule,err error){
 if !scope.Valid()||!identity.Identifier(session)||!identity.Identifier(key)||request.Validate()!=nil||l.Validate()!=nil{return out,jobs.ErrInvalid}
 hash:=digestValue(request)
 err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
  if e:=queueLock(ctx,tx,l);e!=nil{return e}
  existing,e:=scanSchedule(tx.QueryRow(ctx,`SELECT `+scheduleColumns+` FROM chartworks.job_schedules WHERE tenant_id=$1 AND creator_id=$2 AND client_key=$3`,scope.Tenant(),scope.Actor(),key))
  if e==nil{if digestValue(existing.Request)!=hash{return store.ErrConflict};out=existing;return nil};if !errors.Is(e,pgx.ErrNoRows){return e}
  var count,total int;if e=tx.QueryRow(ctx,`SELECT count(*) FILTER(WHERE tenant_id=$1),count(*) FROM chartworks.job_schedules`,scope.Tenant()).Scan(&count,&total);e!=nil{return e};if count>=1000||total>=10000{return jobs.ErrBusy}
  var now time.Time;if e=tx.QueryRow(ctx,`SELECT clock_timestamp()`).Scan(&now);e!=nil{return e}
  previous,next,e:=scheduleTimes(request.Spec,now);if e!=nil{return e};id,e:=newID();if e!=nil{return e}
  _,e=tx.Exec(ctx,`INSERT INTO chartworks.job_schedules(tenant_id,schedule_id,creator_id,creator_session,client_key,request_hash,request,previous_due,next_due) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,scope.Tenant(),id,scope.Actor(),session,key,hash,request.JSON(),previous,next);if e!=nil{return e}
  out=jobs.Schedule{ID:id,Revision:1,Enabled:true,Tenant:scope.Tenant(),Initiator:scope.Actor(),InitiatorSession:session,Request:request,NextDue:next,PreviousDue:previous};return auditJob(ctx,tx,scope,"schedule.created",id)
 });return out,err
}
func(d *DB)ReadSchedule(ctx context.Context,scope store.Scope,id string)(out jobs.Schedule,err error){
 if !scope.Valid()||!identity.Identifier(id){return out,store.ErrScope}
 err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{var e error;out,e=scanSchedule(tx.QueryRow(ctx,`SELECT `+scheduleColumns+` FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2`,scope.Tenant(),id));return e});return out,err
}
func(d *DB)SetSchedule(ctx context.Context,scope store.Scope,id string,expected int64,enabled bool)(out jobs.Schedule,err error){
 if !scope.Valid()||!identity.Identifier(id)||expected<1||expected>=1<<62{return out,jobs.ErrInvalid}
 err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
  var e error;out,e=scanSchedule(tx.QueryRow(ctx,`SELECT `+scheduleColumns+` FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2 FOR UPDATE`,scope.Tenant(),id));if e!=nil{return e};if out.Revision!=expected{return store.ErrConflict}
  previous,next:=out.PreviousDue,out.NextDue
  if enabled&&!out.Enabled{var now time.Time;if e=tx.QueryRow(ctx,`SELECT clock_timestamp()`).Scan(&now);e!=nil{return e};previous,next,e=scheduleTimes(out.Request.Spec,now);if e!=nil{return e}}
  if _,e=tx.Exec(ctx,`UPDATE chartworks.job_schedules SET revision=revision+1,enabled=$3,previous_due=$4,next_due=$5 WHERE tenant_id=$1 AND schedule_id=$2`,scope.Tenant(),id,enabled,previous,next);e!=nil{return e}
  out.Revision++;out.Enabled=enabled;out.PreviousDue=previous;out.NextDue=next;return auditJob(ctx,tx,scope,"schedule.updated",id)
 });return out,err
}
func(d *DB)FireSchedule(ctx context.Context,scope store.Scope,session,id,key string,l jobs.Limits)(out jobs.Job,err error){
 if !scope.Valid()||!identity.Identifier(session)||!identity.Identifier(id)||!identity.Identifier(key)||l.Validate()!=nil{return out,jobs.ErrInvalid}
 err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
  if e:=queueLock(ctx,tx,l);e!=nil{return e}
  schedule,e:=scanSchedule(tx.QueryRow(ctx,`SELECT `+scheduleColumns+` FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2 FOR UPDATE`,scope.Tenant(),id));if e!=nil{return e};if !schedule.Enabled{return store.ErrConflict}
  var now time.Time;if e=tx.QueryRow(ctx,`SELECT clock_timestamp()`).Scan(&now);e!=nil{return e}
  out,e=admitJob(ctx,tx,scope,session,"manual:"+digestValue([]string{id,key}),schedule.Request.Target,l,now,now,id,schedule.Revision);if e!=nil{return e}
  _,e=tx.Exec(ctx,`INSERT INTO chartworks.job_occurrences(tenant_id,schedule_id,due_at,window_start,window_end,disposition,operation_id) VALUES($1,$2,$3,$4,$3,'queued',$5) ON CONFLICT DO NOTHING`,scope.Tenant(),id,out.DueAt,out.WindowStart,out.ID);return e
 });return out,err
}
func activeSchedule(ctx context.Context,tx pgx.Tx,tenant,id string)(bool,error){
 var active bool;err:=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM chartworks.operations WHERE tenant_id=$1 AND schedule_id=$2 AND dispatch_mode='queued' AND status IN ('pending','retry','running'))`,tenant,id).Scan(&active);return active,err
}
func skippedOccurrence(ctx context.Context,tx pgx.Tx,s jobs.Schedule,due,start,end,through time.Time,reason string)error{
 _,err:=tx.Exec(ctx,`INSERT INTO chartworks.job_occurrences(tenant_id,schedule_id,due_at,window_start,window_end,skipped_through,disposition) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,s.Tenant,s.ID,due,start,end,through,reason);return err
}
// TickSchedules advances at most 32 definitions and 32 catch-up occurrences per definition.
// Missed ranges are recorded as ranges, not an invented exact count of skipped cron occurrences.
func(d *DB)TickSchedules(ctx context.Context,l jobs.Limits)(admitted int,err error){
 if l.Validate()!=nil{return 0,jobs.ErrInvalid}
 err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
  if e:=queueLock(ctx,tx,l);e!=nil{return e}
  var now time.Time;if e:=tx.QueryRow(ctx,`SELECT clock_timestamp()`).Scan(&now);e!=nil{return e}
  rows,e:=tx.Query(ctx,`SELECT `+scheduleColumns+` FROM chartworks.job_schedules WHERE enabled AND next_due<=$1 ORDER BY next_due,schedule_id LIMIT 32 FOR UPDATE SKIP LOCKED`,now);if e!=nil{return e}
  list:=[]jobs.Schedule{};for rows.Next(){s,e:=scanSchedule(rows);if e!=nil{rows.Close();return e};list=append(list,s)};e=rows.Err();rows.Close();if e!=nil{return e}
  for _,s:=range list{
   scope,e:=store.NewScope(s.Tenant,s.Initiator);if e!=nil{return e};due,previous:=*s.NextDue,*s.PreviousDue
   maximum:=1;if s.Request.Spec.Missed=="catch_up"{maximum=s.Request.Spec.MaxCatchUp}
   processed:=0
   for !due.After(now){
    // Skip old intervals explicitly. Catch-up processes only the declared bounded oldest set.
    if s.Request.Spec.Missed=="skip"&&now.Sub(due)>5*time.Second||processed>=maximum{
     last,e:=s.Request.Spec.Previous(now);if e!=nil{return e};next,e:=s.Request.Spec.Next(now);if e!=nil{return e}
     if e=skippedOccurrence(ctx,tx,s,due,previous,last,last,"missed_skipped");e!=nil{return e};previous,due=last,next;break
    }
    active,e:=activeSchedule(ctx,tx,s.Tenant,s.ID);if e!=nil{return e}
    if active&&s.Request.Spec.Overlap=="skip"{
     if e=skippedOccurrence(ctx,tx,s,due,previous,due,due,"overlap_skipped");e!=nil{return e}
    }else{
     job,e:=admitJob(ctx,tx,scope,s.InitiatorSession,"occurrence:"+digestValue([]any{s.ID,due.UTC()}),s.Request.Target,l,due,previous,s.ID,s.Revision)
     if errors.Is(e,jobs.ErrBusy){break};if e!=nil{return e}
     if _,e=tx.Exec(ctx,`INSERT INTO chartworks.job_occurrences(tenant_id,schedule_id,due_at,window_start,window_end,disposition,operation_id) VALUES($1,$2,$3,$4,$3,'queued',$5) ON CONFLICT DO NOTHING`,s.Tenant,s.ID,due,previous,job.ID);e!=nil{return e};admitted++
    }
    processed++;previous=due;due,e=s.Request.Spec.Next(due);if e!=nil{return e}
   }
   if _,e=tx.Exec(ctx,`UPDATE chartworks.job_schedules SET previous_due=$3,next_due=$4 WHERE tenant_id=$1 AND schedule_id=$2`,s.Tenant,s.ID,previous,due);e!=nil{return e}
  };return nil
 });return admitted,err
}
