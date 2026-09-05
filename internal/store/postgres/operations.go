package postgres

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/hurtener/chartworks/internal/store"
)

const operationColumns = `operation_id,status,policy_revision,cutoff,batch_limit,deleted_events,deleted_operations`
func scanOperation(row pgx.Row)(o store.Operation,err error){err=row.Scan(&o.ID,&o.Status,&o.PolicyRevision,&o.Cutoff,&o.Limit,&o.DeletedEvents,&o.DeletedOperations);return o,err}

// ReserveSweep atomically accepts a key/hash and a database-clock retention manifest.
// A replay returns the original manifest/result, even if policy or wall clock changed.
func (d *DB) ReserveSweep(ctx context.Context,s store.Scope,key,hash string,limit int)(out store.Operation,err error){
	if err=checkScope(s);err!=nil{return out,err}
	b,e:=hex.DecodeString(hash);if !store.Identifier(key)||e!=nil||len(b)!=32||len(hash)!=64||limit<1||limit>1000{return out,store.ErrInvalid}
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error {
		_,e:=tx.Exec(ctx,`INSERT INTO chartworks.operations(tenant_id,operation_id,actor_id,kind,client_key,request_hash,policy_revision,cutoff,batch_limit,expires_at)
 SELECT $1,$2,$3,'retention.sweep',$4,$5,r.revision,clock_timestamp()-make_interval(days=>r.audit_days),$6,clock_timestamp()+make_interval(hours=>r.operation_hours)
 FROM chartworks.policies p JOIN chartworks.policy_revisions r ON (r.tenant_id,r.revision)=(p.tenant_id,p.current_revision) WHERE p.tenant_id=$1
 ON CONFLICT(tenant_id,actor_id,kind,client_key) DO NOTHING`,s.Tenant(),id(),s.Actor(),key,hash,limit)
		if e!=nil{return e}
		var storedHash string
		row:=tx.QueryRow(ctx,`SELECT `+operationColumns+`,request_hash FROM chartworks.operations WHERE tenant_id=$1 AND actor_id=$2 AND kind='retention.sweep' AND client_key=$3`,s.Tenant(),s.Actor(),key)
		if e=row.Scan(&out.ID,&out.Status,&out.PolicyRevision,&out.Cutoff,&out.Limit,&out.DeletedEvents,&out.DeletedOperations,&storedHash);e!=nil{return e};if storedHash!=hash{return store.ErrConflict};return nil
	});return out,err
}
func validLease(l store.Lease)bool{return store.Identifier(l.OperationID)&&store.Identifier(l.Owner)&&l.Fence>0}
func validTTL(ttl time.Duration)bool{return ttl>=time.Millisecond&&ttl<=5*time.Minute}
// Claim acquires or reclaims a lease, incrementing its monotonic fencing number.
func (d *DB) Claim(ctx context.Context,s store.Scope,opID,owner string,ttl time.Duration)(out store.Lease,err error){
	if err=checkScope(s);err!=nil{return out,err};if !store.Identifier(opID)||!store.Identifier(owner)||!validTTL(ttl){return out,store.ErrInvalid}
	out=store.Lease{OperationID:opID,Owner:owner}
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error {
		e:=tx.QueryRow(ctx,`UPDATE chartworks.operations SET status='running',fence=fence+1,lease_owner=$4,lease_until=clock_timestamp()+$5::bigint*interval '1 microsecond'
 WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND status<>'succeeded' AND expires_at>clock_timestamp() AND (lease_until IS NULL OR lease_until<=clock_timestamp()) RETURNING fence`,s.Tenant(),s.Actor(),opID,owner,ttl.Microseconds()).Scan(&out.Fence)
		if e==pgx.ErrNoRows{return store.ErrConflict};return e
	});if err!=nil{return store.Lease{},err};return out,nil
}
// Renew never resurrects an expired lease, even before another worker claims it.
func (d *DB) Renew(ctx context.Context,s store.Scope,l store.Lease,ttl time.Duration)error{
	if e:=checkScope(s);e!=nil{return e};if !validLease(l)||!validTTL(ttl){return store.ErrInvalid}
	return d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		tag,e:=tx.Exec(ctx,`UPDATE chartworks.operations SET lease_until=clock_timestamp()+$6::bigint*interval '1 microsecond' WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND lease_owner=$4 AND fence=$5 AND status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`,s.Tenant(),s.Actor(),l.OperationID,l.Owner,l.Fence,ttl.Microseconds());if e!=nil{return e};if tag.RowsAffected()!=1{return store.ErrConflict};return nil
	})
}
// CommitSweep deletes a bounded eligible batch and commits its audit/result with the same fence.
func (d *DB) CommitSweep(ctx context.Context,s store.Scope,l store.Lease)(out store.Operation,err error){
	if err=checkScope(s);err!=nil{return out,err};if !validLease(l){return out,store.ErrInvalid}
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		var e error
		out,e=scanOperation(tx.QueryRow(ctx,`SELECT `+operationColumns+` FROM chartworks.operations WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND lease_owner=$4 AND fence=$5 AND status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp() FOR UPDATE`,s.Tenant(),s.Actor(),l.OperationID,l.Owner,l.Fence));if e==pgx.ErrNoRows{return store.ErrConflict};if e!=nil{return e}
		var current int64
		if e=tx.QueryRow(ctx,`SELECT current_revision FROM chartworks.policies WHERE tenant_id=$1 FOR SHARE`,s.Tenant()).Scan(&current);e!=nil{return e};if current!=out.PolicyRevision{return store.ErrConflict}
		tag,e:=tx.Exec(ctx,`DELETE FROM chartworks.audit_events WHERE (tenant_id,event_id) IN (SELECT tenant_id,event_id FROM chartworks.audit_events WHERE tenant_id=$1 AND created_at<$2 ORDER BY created_at,event_id LIMIT $3 FOR UPDATE SKIP LOCKED)`,s.Tenant(),out.Cutoff,out.Limit);if e!=nil{return e};out.DeletedEvents=tag.RowsAffected()
		tag,e=tx.Exec(ctx,`DELETE FROM chartworks.operations WHERE (tenant_id,operation_id) IN (SELECT o.tenant_id,o.operation_id FROM chartworks.operations o WHERE o.tenant_id=$1 AND o.operation_id<>$2 AND o.status='succeeded' AND o.expires_at<clock_timestamp() AND NOT EXISTS (SELECT 1 FROM chartworks.audit_events a WHERE a.tenant_id=o.tenant_id AND a.operation_id=o.operation_id) ORDER BY o.expires_at,o.operation_id LIMIT $3 FOR UPDATE SKIP LOCKED)`,s.Tenant(),out.ID,out.Limit);if e!=nil{return e};out.DeletedOperations=tag.RowsAffected()
		// Check expiry again after the work. Expiration while holding the row lock rolls EVERYTHING back.
		tag,e=tx.Exec(ctx,`UPDATE chartworks.operations SET status='succeeded',finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL,deleted_events=$6,deleted_operations=$7 WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND lease_owner=$4 AND fence=$5 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`,s.Tenant(),s.Actor(),out.ID,l.Owner,l.Fence,out.DeletedEvents,out.DeletedOperations);if e!=nil{return e};if tag.RowsAffected()!=1{return store.ErrConflict}
		_,e=tx.Exec(ctx,`INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id,operation_id) VALUES($1,$2,$3,'retention.sweep',$4,$4)`,s.Tenant(),id(),s.Actor(),out.ID);if e!=nil{return e};out.Status="succeeded";return nil
	});if err!=nil{return store.Operation{},err};return out,nil
}
