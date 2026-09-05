from pathlib import Path
p=Path('go.mod');s=p.read_text();
if 'github.com/robfig/cron/v3' not in s:s=s.replace('require (','require (\n github.com/robfig/cron/v3 v3.0.1',1)
p.write_text(s)
p=Path('internal/jobs/jobs.go');s=p.read_text().replace('FireSchedule(context.Context,store.Scope,string,string,Limits)','FireSchedule(context.Context,store.Scope,string,string,string,Limits)');p.write_text(s)
p=Path('internal/jobs/service.go');s=p.read_text().replace('s.repo.FireSchedule(ctx,scope,id,key,s.limits)','s.repo.FireSchedule(ctx,scope,e.Session(),id,key,s.limits)');p.write_text(s)
p=Path('internal/identity/identity.go');s=p.read_text().replace('case "source",','case "schedule", "source",');p.write_text(s)
p=Path('internal/store/postgres/postgres.go');s=p.read_text().replace('"github.com/hurtener/chartworks/internal/store"','"github.com/hurtener/chartworks/internal/store"\n "github.com/hurtener/chartworks/internal/jobs"');s=s.replace('[]error{store.ErrScope,','[]error{jobs.ErrInvalid, jobs.ErrBusy, jobs.ErrEmpty, jobs.ErrAuthority, jobs.ErrTransient, store.ErrScope,');p.write_text(s)
p=Path('internal/store/postgres/migrate.go');s=p.read_text().replace("'audit_events','operations')","'audit_events','operations','queue_limits','operation_attempts','job_schedules','job_occurrences')").replace('if count != 5 {','if count != 9 {');p.write_text(s)
p=Path('internal/store/postgres/operations.go');s=p.read_text()
# Never let the inline lease path take over an independently authorized queued operation.
s=s.replace("WHERE tenant_id=$1 AND actor_id=$2 AND kind='retention.sweep'", "WHERE dispatch_mode='inline' AND tenant_id=$1 AND actor_id=$2 AND kind='retention.sweep'")
s=s.replace('WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3',"WHERE dispatch_mode='inline' AND tenant_id=$1 AND actor_id=$2 AND operation_id=$3")
s=s.replace('WHERE tenant_id=$1 AND operation_id=$2 AND actor_id=$3',"WHERE dispatch_mode='inline' AND tenant_id=$1 AND operation_id=$2 AND actor_id=$3")
p.write_text(s)
p=Path('internal/store/postgres/migrations/003_durable_dispatch.sql');s=p.read_text();s += "\nCREATE UNIQUE INDEX queued_caller_idempotency ON chartworks.operations(tenant_id,initiator_id,client_key) WHERE dispatch_mode='queued';\n";p.write_text(s)
p=Path('internal/store/postgres/jobs.go');s=p.read_text().replace("WHERE tenant_id=$1 AND actor_id=$2 AND kind='retention.sweep' AND client_key=$3 AND dispatch_mode='queued'`,scope.Tenant(),executor,clientKey)","WHERE tenant_id=$1 AND initiator_id=$2 AND kind='retention.sweep' AND client_key=$3 AND dispatch_mode='queued'`,scope.Tenant(),scope.Actor(),clientKey)")
p.write_text(s)
