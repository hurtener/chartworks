package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// operationLease is the shared storage coordinate, not identity authority. Both
// broker and fresh-caller dispatch use these same attempt/fence transitions.
type operationLease struct { tenant,id,owner,manifest,mode string; fence int64 }

func claimOperationLease(ctx context.Context,tx pgx.Tx,tenant,id,owner string,ttl time.Duration) (fence int64,attempt int,until time.Time,err error) {
	if _,err=tx.Exec(ctx,`UPDATE chartworks.operation_attempts SET state='abandoned',finished_at=clock_timestamp(),error_code='lease_lost' WHERE tenant_id=$1 AND operation_id=$2 AND state='acquiring'`,tenant,id);err!=nil{return}
	err=tx.QueryRow(ctx,`UPDATE chartworks.operations SET status='running',fence=fence+1,attempt_count=attempt_count+1,lease_owner=$3,lease_until=LEAST(expires_at,clock_timestamp()+$4*interval '1 millisecond'),error_code='' WHERE tenant_id=$1 AND operation_id=$2 AND status IN('pending','retry','running') AND (lease_until IS NULL OR lease_until<=clock_timestamp()) AND attempt_count<max_attempts AND expires_at>clock_timestamp() RETURNING fence,attempt_count,lease_until`,tenant,id,owner,ttl.Milliseconds()).Scan(&fence,&attempt,&until)
	if errors.Is(err,pgx.ErrNoRows){err=store.ErrConflict}
	if err!=nil{return}
	_,err=tx.Exec(ctx,`INSERT INTO chartworks.operation_attempts(tenant_id,operation_id,fence,attempt,owner_id,state) VALUES($1,$2,$3,$4,$5,'acquiring')`,tenant,id,fence,attempt,owner)
	return
}
func renewOperationLease(ctx context.Context,tx pgx.Tx,l operationLease,ttl time.Duration) error {
	tag,err:=tx.Exec(ctx,`UPDATE chartworks.operations SET lease_until=LEAST(expires_at,clock_timestamp()+$5*interval '1 millisecond') WHERE tenant_id=$1 AND operation_id=$2 AND dispatch_mode=$6 AND manifest_hash=$7 AND status='running' AND lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`,l.tenant,l.id,l.owner,l.fence,ttl.Milliseconds(),l.mode,l.manifest)
	if err!=nil{return err};if tag.RowsAffected()!=1{return store.ErrConflict};return nil
}
func failOperationLease(ctx context.Context,tx pgx.Tx,l operationLease,code string,permanent bool,delay time.Duration) error {
	var attempts,maximum int
	err:=tx.QueryRow(ctx,`SELECT attempt_count,max_attempts FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2 AND dispatch_mode=$5 AND manifest_hash=$6 AND status='running' AND lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp() FOR UPDATE`,l.tenant,l.id,l.owner,l.fence,l.mode,l.manifest).Scan(&attempts,&maximum)
	if errors.Is(err,pgx.ErrNoRows){return store.ErrConflict};if err!=nil{return err}
	state:="retry";if permanent{state="blocked"}else if attempts>=maximum{state="failed"}
	if _,err=tx.Exec(ctx,`UPDATE chartworks.operations SET status=$5,error_code=$6,lease_owner=NULL,lease_until=NULL,finished_at=CASE WHEN $5='retry' THEN NULL ELSE clock_timestamp() END,next_attempt_at=clock_timestamp()+$7*interval '1 millisecond' WHERE tenant_id=$1 AND operation_id=$2 AND lease_owner=$3 AND fence=$4`,l.tenant,l.id,l.owner,l.fence,state,code,delay.Milliseconds());err!=nil{return err}
	_,err=tx.Exec(ctx,`UPDATE chartworks.operation_attempts SET state=$4,error_code=$5,finished_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2 AND fence=$3 AND state='acquiring'`,l.tenant,l.id,l.fence,state,code)
	return err
}
