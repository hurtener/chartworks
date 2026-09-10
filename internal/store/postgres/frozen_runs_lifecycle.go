package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.RunRepository = (*DB)(nil)

// ReuseFrozenRun reuses only an already readable, same-partition, exact-key
// successful artifact. Original observation/expiry are preserved through chains;
// original usage receipts are provenance, not another provider charge.
func(d *DB) ReuseFrozenRun(ctx context.Context,inv jobs.Invocation,id string)(out reporting.RunRecord,reused bool,err error){
	lease:=inv.Lease()
	e,err:=inv.Current("reporting.run",lease.Task.Input.Target,lease.Task.Input.InputHash)
	if err!=nil{return out,false,err}
	if id!=lease.Task.ID{return out,false,store.ErrInvalid}
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,false,err};defer stop()
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		if _,fenceErr:=requestFenceTx(ctx,tx,inv);fenceErr!=nil{return fenceErr}
		h,readErr:=frozenReadHeadTx(ctx,tx,e,id,true,true);if readErr!=nil{return readErr}
		out,readErr=frozenReadTx(ctx,tx,e,id,true,true);if readErr!=nil{return readErr}
		if out.Manifest==nil||!time.Now().Before(h.view.Expires){return reporting.ErrExpired}
		m:=*out.Manifest
		if m.ReuseMaxAge<=0||out.Result!=nil||h.view.State!="sealed"{return nil}
		if readErr=frozenCurrentTx(ctx,tx,e,m);readErr!=nil{return readErr}
		if !e.Has("reporting.read"){return nil}
		args,readErr:=frozenArgs(e,id,false);if readErr!=nil{return readErr}
		age:=min(time.Duration(m.ReuseMaxAge)*time.Second,time.Duration(m.Limits.MaxReuseAge))
		var candidate string
		readErr=tx.QueryRow(ctx,`SELECT h.operation_id`+frozenFrom+frozenEligibility+` AND h.operation_id<>$2 AND h.reuse_key=$8 AND h.state='succeeded' AND h.payload_expires_at>clock_timestamp() AND h.observed_at>=$9 ORDER BY h.observed_at DESC,h.operation_id LIMIT 1 FOR SHARE OF h`,append(args,m.ReuseKey,time.Now().Add(-age))...).Scan(&candidate)
		if errors.Is(readErr,pgx.ErrNoRows){return nil};if readErr!=nil{return readErr}
		previous,readErr:=frozenReadTx(ctx,tx,e,candidate,false,true);if readErr!=nil{return readErr}
		if previous.Manifest==nil||previous.Result==nil||previous.View.Observed==nil||previous.Manifest.ReuseKey!=m.ReuseKey||previous.View.State!="succeeded"{return store.ErrConflict}
		if previous.View.Context!=m.Binding.Context||previous.View.PartitionDigest!=readexec.Hash(m.Binding)||previous.View.Private!=m.Private||previous.View.Locale!=m.Locale||previous.View.RevisionDigest!=m.Revision.Digest{return store.ErrInvalid}
		if _,_,readErr=reporting.FrozenCompletion(m,previous.Outputs);readErr!=nil{return readErr}
		result,marshalErr:=json.Marshal(previous.Result);if marshalErr!=nil||len(result)>m.Limits.MaxResultBytes{return reporting.ErrBudget}
		payloads:=make([][]byte,len(previous.Outputs));added:=int64(len(result))
		for index,output:=range previous.Outputs{
			payloads[index],marshalErr=json.Marshal(output);if marshalErr!=nil{return store.ErrInvalid}
			added+=int64(len(payloads[index]))
		}
		if readErr=frozenBytesTx(ctx,tx,e,h,added);readErr!=nil{return readErr}
		if _,readErr=tx.Exec(ctx,`UPDATE chartworks.frozen_run_payloads SET result=$3,result_digest=$4 WHERE tenant_id=$1 AND operation_id=$2 AND result IS NULL`,e.Tenant(),id,result,readexec.Hash(*previous.Result));readErr!=nil{return readErr}
		for index,output:=range previous.Outputs{
			if _,readErr=tx.Exec(ctx,`INSERT INTO chartworks.frozen_run_outputs(tenant_id,operation_id,output_id,ordinal,kind,state,payload) VALUES($1,$2,$3,$4,$5,$6,$7)`,e.Tenant(),id,output.ID,index,output.Kind,output.State,payloads[index]);readErr!=nil{return readErr}
		}
		if _,readErr=tx.Exec(ctx,`UPDATE chartworks.frozen_runs SET state='succeeded',reused_from=$3,observed_at=$4,finished_at=clock_timestamp(),reserved_bytes=0,payload_expires_at=LEAST(payload_expires_at,$5) WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),id,candidate,*previous.View.Observed,previous.View.Expires);readErr!=nil{return readErr}
		if readErr=completeRequestTx(ctx,tx,inv);readErr!=nil{return readErr}
		if readErr=frozenAudit(ctx,tx,e,"reporting.run_reused",id);readErr!=nil{return readErr}
		out,readErr=frozenReadTx(ctx,tx,e,id,true,true);reused=readErr==nil;return readErr
	})
	if err!=nil{return reporting.RunRecord{},false,err};return out,reused,nil
}

// expireFrozenRows is also used by ordinary retention maintenance. Only values
// are erased; immutable identity/expiry and bounded query receipts remain as
// permitted tombstones. A later rendition owner must share this payload lifetime.
func expireFrozenRows(ctx context.Context,tx pgx.Tx,scope store.Scope,asOf time.Time,limit int)(int64,error){
	if !scope.Valid()||asOf.IsZero()||limit<1||limit>1000{return 0,store.ErrInvalid}
	if err:=frozenQuotaLock(ctx,tx,scope.Tenant());err!=nil{return 0,err}
	rows,err:=tx.Query(ctx,`SELECT operation_id FROM chartworks.frozen_runs WHERE tenant_id=$1 AND state<>'expired' AND payload_expires_at<=$2 ORDER BY payload_expires_at,operation_id LIMIT $3 FOR UPDATE SKIP LOCKED`,scope.Tenant(),asOf,limit)
	if err!=nil{return 0,err}
	ids:=[]string{}
	for rows.Next(){var id string;if err=rows.Scan(&id);err!=nil{rows.Close();return 0,err};ids=append(ids,id)}
	err=rows.Err();rows.Close();if err!=nil{return 0,err}
	for _,id:=range ids{
		if _,err=tx.Exec(ctx,`UPDATE chartworks.read_attempts SET cancel_requested=true WHERE tenant_id=$1 AND operation_id=$2 AND finished_at IS NULL`,scope.Tenant(),id);err!=nil{return 0,err}
		if _,err=tx.Exec(ctx,`DELETE FROM chartworks.frozen_run_outputs WHERE tenant_id=$1 AND operation_id=$2`,scope.Tenant(),id);err!=nil{return 0,err}
		if _,err=tx.Exec(ctx,`DELETE FROM chartworks.frozen_run_payloads WHERE tenant_id=$1 AND operation_id=$2`,scope.Tenant(),id);err!=nil{return 0,err}
		if _,err=tx.Exec(ctx,`UPDATE chartworks.frozen_runs SET state='expired',code='retention_expired',retained_bytes=0,reserved_bytes=0 WHERE tenant_id=$1 AND operation_id=$2`,scope.Tenant(),id);err!=nil{return 0,err}
		if err=auditJob(ctx,tx,scope,"reporting.artifact_expired",id);err!=nil{return 0,err}
	}
	return int64(len(ids)),nil
}

// ExpireFrozenArtifacts explicitly runs bounded tenant retention. It cannot
// shorten an artifact's lifetime, select another tenant or execute source work.
func(d *DB) ExpireFrozenArtifacts(ctx context.Context,e identity.Envelope,limit int)(count int64,err error){
	if limit<1||limit>1000{return 0,store.ErrInvalid}
	if err=access.Require(e,"reporting.retention",access.Resource{Tenant:e.Tenant(),Kind:"tenant",Permission:"erase",ID:e.Tenant()});err!=nil{return 0,err}
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return 0,err};defer stop()
	scope,err:=store.NewScope(e.Tenant(),e.User());if err!=nil{return 0,err}
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{var expireErr error;count,expireErr=expireFrozenRows(ctx,tx,scope,time.Now(),limit);return expireErr})
	if err!=nil{return 0,err};return count,nil
}
