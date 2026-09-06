package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ jobs.RequestRepository = (*DB)(nil)
const requestColumns = `operation_id,tenant_id,actor_id,initiator_session,request_manifest,status,error_code,attempt_count,max_attempts,created_at,expires_at,manifest_hash`

func scanRequest(row pgx.Row) (task jobs.RequestTask,err error) {
	var raw []byte
	err=row.Scan(&task.ID,&task.Tenant,&task.Actor,&task.Session,&raw,&task.State,&task.Code,&task.Attempts,&task.MaxAttempts,&task.Created,&task.Expires,&task.ManifestHash)
	if err!=nil{return task,err}
	if json.Unmarshal(raw,&task.Input)!=nil || !task.Valid(){return jobs.RequestTask{},store.ErrInvalid}
	return task,nil
}
func requestContext(ctx context.Context,e identity.Envelope)(context.Context,context.CancelFunc,error){
	if ctx==nil || !e.Valid(){return nil,nil,access.ErrUnauthenticated}
	child,stop:=context.WithDeadline(ctx,e.Deadline());return child,stop,nil
}
func readRequestTx(ctx context.Context,tx pgx.Tx,e identity.Envelope,id string,lock bool)(jobs.RequestTask,error){
	if !e.Valid() || !identity.Identifier(id){return jobs.RequestTask{},access.ErrUnauthenticated}
	query:=`SELECT `+requestColumns+` FROM chartworks.operations WHERE tenant_id=$1 AND actor_id=$2 AND initiator_session=$3 AND operation_id=$4 AND dispatch_mode='request'`
	if lock{query+=` FOR UPDATE`}
	task,err:=scanRequest(tx.QueryRow(ctx,query,e.Tenant(),e.User(),e.Session(),id))
	if err!=nil{return jobs.RequestTask{},err}
	if err=task.Require(e);err!=nil{return jobs.RequestTask{},err}
	return task,nil
}

// AdmitRequest reserves content-free request work in the existing operation
// ledger under the same cross-replica queue-capacity lock as broker work.
func(d *DB) AdmitRequest(ctx context.Context,e identity.Envelope,key string,input jobs.RequestInput,l jobs.Limits)(out jobs.RequestTask,err error){
	if !identity.Identifier(key)||l.Validate()!=nil{return out,jobs.ErrInvalid}
	if err=input.Require(e);err!=nil{return out,err}
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,err};defer stop()
	scope,_:=store.NewScope(e.Tenant(),e.User())
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		if err:=queueLock(ctx,tx,l);err!=nil{return err}
		clientKey:=digestValue([]string{"request-v1",key})
		hash:=digestValue([]any{input,e.Session()})
		previous,err:=scanRequest(tx.QueryRow(ctx,`SELECT `+requestColumns+` FROM chartworks.operations WHERE tenant_id=$1 AND actor_id=$2 AND kind=$3 AND client_key=$4 AND dispatch_mode='request'`,e.Tenant(),e.User(),input.Kind,clientKey))
		if err==nil{
			if previous.Input!=input||previous.Session!=e.Session(){return store.ErrConflict}
			if err=previous.Require(e);err!=nil{return err}
			out=previous;return nil
		}
		if !errors.Is(err,pgx.ErrNoRows){return err}
		if err=queueCapacity(ctx,tx,e.Tenant(),l);err!=nil{return err}
		// Terminal receipts are bounded too. Erasure/retention is explicit; a
		// completed key cannot disappear merely because it is inconvenient.
		var retained int
		if err=tx.QueryRow(ctx,`SELECT count(*) FROM chartworks.operations WHERE tenant_id=$1 AND actor_id=$2 AND dispatch_mode='request'`,e.Tenant(),e.User()).Scan(&retained);err!=nil{return err}
		if retained>=l.MaxPendingPerTenant{return jobs.ErrBusy}
		var now time.Time;var revision int64;var hours int
		if err=tx.QueryRow(ctx,`SELECT clock_timestamp(),p.current_revision,r.operation_hours FROM chartworks.policies p JOIN chartworks.policy_revisions r ON(r.tenant_id,r.revision)=(p.tenant_id,p.current_revision) WHERE p.tenant_id=$1 FOR SHARE OF p`,e.Tenant()).Scan(&now,&revision,&hours);err!=nil{return err}
		id,err:=newID();if err!=nil{return err};now=now.UTC().Truncate(time.Microsecond)
		out=jobs.RequestTask{ID:id,Tenant:e.Tenant(),Actor:e.User(),Session:e.Session(),Input:input,State:"pending",MaxAttempts:l.MaxAttempts,Created:now,Expires:now.Add(time.Duration(hours)*time.Hour)}
		out.ManifestHash=out.Digest();if !out.Valid(){return jobs.ErrInvalid}
		body,_:=json.Marshal(input)
		if !e.Valid(){return access.ErrUnauthenticated}
		_,err=tx.Exec(ctx,`INSERT INTO chartworks.operations(tenant_id,operation_id,actor_id,kind,client_key,request_hash,policy_revision,cutoff,batch_limit,created_at,expires_at,dispatch_mode,initiator_id,initiator_session,due_at,window_start,window_end,manifest_hash,max_attempts,next_attempt_at,request_manifest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,$8,$9,'request',$3,$10,$8,$8,$8,$11,$12,$8,$13)`,out.Tenant,out.ID,out.Actor,input.Kind,clientKey,hash,revision,now,out.Expires,out.Session,out.ManifestHash,out.MaxAttempts,body)
		if err!=nil{return err};return auditJob(ctx,tx,scope,"request.accepted",out.ID)
	})
	if err!=nil{return jobs.RequestTask{},err};return out,nil
}

// ReadRequest never falls back to a tenant-wide actor or session lookup.
func(d *DB) ReadRequest(ctx context.Context,e identity.Envelope,id string)(out jobs.RequestTask,err error){
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,err};defer stop()
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{var e2 error;out,e2=readRequestTx(ctx,tx,e,id,false);return e2})
	if err!=nil{return jobs.RequestTask{},err};return out,nil
}

// ResumeRequest requires new supplied authority and preserves the accepted
// manifest/version. It does not reset attempts, timestamps or expiry.
func(d *DB) ResumeRequest(ctx context.Context,e identity.Envelope,id string)(out jobs.RequestTask,err error){
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,err};defer stop()
	scope,_:=store.NewScope(e.Tenant(),e.User())
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		var err error;out,err=readRequestTx(ctx,tx,e,id,true);if err!=nil{return err}
		if out.State=="succeeded"{return nil}
		if (out.State!="cancelled"&&out.State!="retry"&&out.State!="pending")||out.Attempts>=out.MaxAttempts{return store.ErrConflict}
		if !time.Now().Before(out.Expires){return store.ErrExpired}
		if _,err=tx.Exec(ctx,`UPDATE chartworks.operations SET status='pending',finished_at=NULL,error_code='',next_attempt_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),id);err!=nil{return err}
		out.State="pending";out.Code="";return auditJob(ctx,tx,scope,"request.resumed",id)
	})
	if err!=nil{return jobs.RequestTask{},err};return out,nil
}

// CancelRequest makes cancellation durable before a live owner's next observation.
func(d *DB) CancelRequest(ctx context.Context,e identity.Envelope,id string)(out jobs.RequestTask,err error){
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,err};defer stop()
	scope,_:=store.NewScope(e.Tenant(),e.User())
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		var err error;out,err=readRequestTx(ctx,tx,e,id,true);if err!=nil{return err}
		if out.State=="cancelled"||out.State=="succeeded"{return nil}
		if out.State!="pending"&&out.State!="retry"&&out.State!="running"{return store.ErrConflict}
		if _,err=tx.Exec(ctx,`UPDATE chartworks.operation_attempts SET state='cancelled',error_code='cancelled',finished_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2 AND state='acquiring'`,e.Tenant(),id);err!=nil{return err}
		if _,err=tx.Exec(ctx,`UPDATE chartworks.operations SET status='cancelled',error_code='cancelled',finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),id);err!=nil{return err}
		out.State="cancelled";out.Code="cancelled";return auditJob(ctx,tx,scope,"request.cancelled",id)
	})
	if err!=nil{return jobs.RequestTask{},err};return out,nil
}

// ClaimRequest shares persistent global/tenant concurrency with broker dispatch.
func(d *DB) ClaimRequest(ctx context.Context,e identity.Envelope,id,owner string,l jobs.Limits)(out jobs.RequestLease,err error){
	if !identity.Identifier(owner)||l.Validate()!=nil{return out,jobs.ErrInvalid}
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,err};defer stop()
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		if err:=queueLock(ctx,tx,l);err!=nil{return err}
		task,err:=readRequestTx(ctx,tx,e,id,true);if err!=nil{return err}
		if !time.Now().Before(task.Expires){return store.ErrExpired}
		var global,tenant int
		if err=tx.QueryRow(ctx,`SELECT count(*),count(*) FILTER(WHERE tenant_id=$1) FROM chartworks.operations WHERE dispatch_mode IN('queued','request') AND status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`,e.Tenant()).Scan(&global,&tenant);err!=nil{return err}
		if global>=l.GlobalConcurrency||tenant>=l.TenantConcurrency{return jobs.ErrBusy}
		var eligible bool
		if err=tx.QueryRow(ctx,`SELECT status IN('pending','retry','running') AND next_attempt_at<=clock_timestamp() AND (lease_until IS NULL OR lease_until<=clock_timestamp()) AND attempt_count<max_attempts FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),id).Scan(&eligible);err!=nil{return err}
		if !eligible{return store.ErrConflict}
		fence,attempt,until,err:=claimOperationLease(ctx,tx,e.Tenant(),id,owner,l.Lease);if err!=nil{return err}
		task.State="running";task.Attempts=attempt
		out=jobs.RequestLease{Task:task,Owner:owner,Fence:fence,Attempt:attempt,Until:until};return nil
	})
	if err!=nil{return jobs.RequestLease{},err};return out,nil
}
func requestCoordinates(i jobs.Invocation)operationLease{
	l:=i.Lease();return operationLease{tenant:l.Task.Tenant,id:l.Task.ID,owner:l.Owner,manifest:l.Task.ManifestHash,mode:"request",fence:l.Fence}
}

// PulseRequest only renews the original currently authorized live fence.
func(d *DB) PulseRequest(ctx context.Context,i jobs.Invocation,renew bool,ttl time.Duration)(state string,err error){
	l:=i.Lease();e,err:=i.Current(l.Task.Input.Kind,l.Task.Input.Target,l.Task.Input.InputHash);if err!=nil{return "",err}
	if ttl<time.Second||ttl>time.Minute{return "",jobs.ErrInvalid}
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return "",err};defer stop()
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		var live bool
		if err:=tx.QueryRow(ctx,`SELECT status,lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp() FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2 AND manifest_hash=$5 AND dispatch_mode='request'`,l.Task.Tenant,l.Task.ID,l.Owner,l.Fence,l.Task.ManifestHash).Scan(&state,&live);err!=nil{return err}
		if state=="succeeded"||state=="cancelled"{return nil}
		if state!="running"||!live{return store.ErrConflict}
		if renew{return renewOperationLease(ctx,tx,requestCoordinates(i),ttl)};return nil
	})
	return state,err
}

// FailRequest can seal failure after caller expiry, using only opaque ownership
// and the live persistent fence. It cannot authorize warehouse work/publication.
func(d *DB) FailRequest(ctx context.Context,i jobs.Invocation,code string,permanent bool,delay time.Duration)error{
	if !i.Valid()||delay<0||delay>time.Minute{return jobs.ErrInvalid}
	switch code{case "authority_blocked","attempt_failed","attempt_timeout","definition_changed":default:return jobs.ErrInvalid}
	return d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{return failOperationLease(ctx,tx,requestCoordinates(i),code,permanent,delay)})
}

// requestFenceTx is called by concrete domain publication, not a generic
// success-returning completion endpoint. Holding this row resolves cancellation
// against publication in the SAME metadata transaction.
func requestFenceTx(ctx context.Context,tx pgx.Tx,i jobs.Invocation)(jobs.RequestTask,error){
	l:=i.Lease();e,err:=i.Current(l.Task.Input.Kind,l.Task.Input.Target,l.Task.Input.InputHash);if err!=nil{return jobs.RequestTask{},err}
	task,err:=readRequestTx(ctx,tx,e,l.Task.ID,true);if err!=nil{return jobs.RequestTask{},err}
	var live bool
	if err=tx.QueryRow(ctx,`SELECT status='running' AND lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp() AND manifest_hash=$5 FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`,task.Tenant,task.ID,l.Owner,l.Fence,l.Task.ManifestHash).Scan(&live);err!=nil{return jobs.RequestTask{},err}
	if !live{return jobs.RequestTask{},store.ErrConflict};return task,nil
}
func completeRequestTx(ctx context.Context,tx pgx.Tx,i jobs.Invocation)error{
	l:=i.Lease();e,err:=i.Current(l.Task.Input.Kind,l.Task.Input.Target,l.Task.Input.InputHash);if err!=nil{return err}
	tag,err:=tx.Exec(ctx,`UPDATE chartworks.operations SET status='succeeded',error_code='',finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL WHERE tenant_id=$1 AND operation_id=$2 AND status='running' AND lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`,l.Task.Tenant,l.Task.ID,l.Owner,l.Fence)
	if err!=nil{return err};if tag.RowsAffected()!=1{return store.ErrConflict}
	if _,err=tx.Exec(ctx,`UPDATE chartworks.operation_attempts SET state='succeeded',executor_id=$4,finished_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2 AND fence=$3 AND state='acquiring'`,l.Task.Tenant,l.Task.ID,l.Fence,e.User());err!=nil{return err}
	scope,_:=store.NewScope(e.Tenant(),e.User());return auditJob(ctx,tx,scope,"request.succeeded",l.Task.ID)
}
