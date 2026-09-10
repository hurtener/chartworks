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

func frozenCurrentTx(ctx context.Context,tx pgx.Tx,e identity.Envelope,m reporting.RunManifest) error {
	var id string
	if err:=tx.QueryRow(ctx,`SELECT block_id FROM chartworks.block_heads WHERE tenant_id=$1 AND block_id=$2 FOR SHARE`,e.Tenant(),m.Block).Scan(&id);err!=nil{return err}
	if err:=blockCurrentFence(ctx,tx,e,reporting.Mutation{Topics:m.Revision.Definition.Topics,Watch:m.Dependencies});err!=nil{return err}
	snapshot,err:=blockTx(ctx,tx,e,m.Block,reporting.Reference{Revision:m.Revision.Number},reporting.Execute)
	if err!=nil{return err}
	return reporting.CheckFrozenEligibility(e,m,snapshot,time.Now())
}

func frozenQuotaLock(ctx context.Context,tx pgx.Tx,tenant string)error{
	_,err:=tx.Exec(ctx,`SELECT pg_advisory_xact_lock(hashtextextended($1,7214062828))`,tenant)
	return err
}

func frozenAudit(ctx context.Context,tx pgx.Tx,e identity.Envelope,action,id string)error{
	scope,err:=store.NewScope(e.Tenant(),e.User());if err!=nil{return err}
	return auditJob(ctx,tx,scope,action,id)
}

// SealFrozenRun chooses one exact manifest for an already reserved request key.
// Quota is reserved before any query/model call, across all service replicas.
func(d *DB) SealFrozenRun(ctx context.Context,e identity.Envelope,task jobs.RequestTask,proof reporting.PreparedRun)(out reporting.RunRecord,err error){
	m,err:=proof.Checked(e);if err!=nil{return out,err}
	if err=task.Require(e);err!=nil{return out,err}
	if task.Input.Kind!="reporting.run"||task.ID!=m.ID||task.Input.Target!=m.Block||task.Input.InputHash!=m.RequestHash||task.ManifestHash!=m.TaskHash||!task.Created.Equal(m.Created){return out,store.ErrInvalid}
	body,err:=json.Marshal(m);if err!=nil||int64(len(body))>int64(m.Limits.MaxArtifactBytes){return out,reporting.ErrBudget}
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,err};defer stop()
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		actual,readErr:=readRequestTx(ctx,tx,e,task.ID,true);if readErr!=nil{return readErr}
		if actual.ManifestHash!=m.TaskHash||actual.Input.InputHash!=m.RequestHash{return store.ErrConflict}
		var previousHash string
		readErr=tx.QueryRow(ctx,`SELECT request_hash FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),m.ID).Scan(&previousHash)
		if readErr==nil{
			if previousHash!=m.RequestHash{return store.ErrConflict}
			out,readErr=frozenReadTx(ctx,tx,e,m.ID,true,true);return readErr
		}
		if !errors.Is(readErr,pgx.ErrNoRows){return readErr}
		if !time.Now().Before(actual.Expires)||actual.State!="pending"{return store.ErrExpired}
		if readErr=frozenCurrentTx(ctx,tx,e,m);readErr!=nil{return readErr}
		if readErr=frozenQuotaLock(ctx,tx,e.Tenant());readErr!=nil{return readErr}
		var count int;var charged int64
		if readErr=tx.QueryRow(ctx,`SELECT count(*),COALESCE(sum(GREATEST(retained_bytes,reserved_bytes)),0) FROM chartworks.frozen_runs WHERE tenant_id=$1`,e.Tenant()).Scan(&count,&charged);readErr!=nil{return readErr}
		if count>=m.Limits.MaxRequests||charged>m.Limits.MaxTenantBytes-int64(m.Limits.MaxArtifactBytes){return reporting.ErrBudget}
		_,readErr=tx.Exec(ctx,`INSERT INTO chartworks.frozen_runs(tenant_id,operation_id,actor_id,session_id,block_id,revision,revision_digest,request_hash,task_hash,manifest_digest,reuse_key,private,source_id,context_id,partition_digest,locale,timezone,frozen_version,created_at,expires_at,payload_expires_at,retained_bytes,reserved_bytes,max_artifact_bytes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$20,$21,$22,$22)`,e.Tenant(),m.ID,m.Actor,m.Session,m.Block,m.Revision.Number,m.Revision.Digest,m.RequestHash,m.TaskHash,m.Digest(),m.ReuseKey,m.Private,m.Binding.Source,m.Binding.Context,readexec.Hash(m.Binding),m.Locale,m.Resolved.Timezone,m.Version,m.Created,m.Expires,len(body),m.Limits.MaxArtifactBytes)
		if readErr!=nil{return readErr}
		if _,readErr=tx.Exec(ctx,`INSERT INTO chartworks.frozen_run_payloads(tenant_id,operation_id,manifest) VALUES($1,$2,$3)`,e.Tenant(),m.ID,body);readErr!=nil{return readErr}
		if readErr=frozenAudit(ctx,tx,e,"reporting.run_sealed",m.ID);readErr!=nil{return readErr}
		out,readErr=frozenReadTx(ctx,tx,e,m.ID,true,true);return readErr
	})
	if err!=nil{return reporting.RunRecord{},err};return out,nil
}

func frozenJournalTx(ctx context.Context,tx pgx.Tx,e identity.Envelope,h frozenHead,a readexec.Attempt)error{
	actual,err:=scanRead(tx.QueryRow(ctx,`SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND attempt_id=$4 FOR SHARE`,e.Tenant(),h.actor,h.view.ID,a.ID))
	if err!=nil{return err}
	if !frozenAttemptMatches(h,actual)||readexec.Hash(actual)!=readexec.Hash(a){return store.ErrInvalid}
	raw,err:=json.Marshal(actual);if err!=nil{return store.ErrInvalid}
	_,err=tx.Exec(ctx,`INSERT INTO chartworks.frozen_run_attempts(tenant_id,operation_id,attempt_number,receipt) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,operation_id,attempt_number) DO UPDATE SET receipt=EXCLUDED.receipt WHERE frozen_run_attempts.receipt->>'id'=EXCLUDED.receipt->>'id' AND frozen_run_attempts.receipt->'manifest'=EXCLUDED.receipt->'manifest'`,e.Tenant(),h.view.ID,actual.Number,raw)
	return err
}

func frozenBytesTx(ctx context.Context,tx pgx.Tx,e identity.Envelope,h frozenHead,delta int64)error{
	if delta>h.maximum-h.view.RetainedBytes||delta < -h.view.RetainedBytes{return reporting.ErrBudget}
	tag,err:=tx.Exec(ctx,`UPDATE chartworks.frozen_runs SET retained_bytes=retained_bytes+$3 WHERE tenant_id=$1 AND operation_id=$2 AND retained_bytes+$3 BETWEEN 0 AND max_artifact_bytes`,e.Tenant(),h.view.ID,delta)
	if err!=nil{return err};if tag.RowsAffected()!=1{return reporting.ErrBudget};return nil
}

func frozenResultTx(ctx context.Context,tx pgx.Tx,e identity.Envelope,h frozenHead,w reporting.RunWrite)error{
	if w.Result==nil||w.Attempt==nil{return store.ErrInvalid}
	if err:=reporting.CheckFrozenResult(ctx,w.Manifest,*w.Result,*w.Attempt);err!=nil{return err}
	if err:=frozenJournalTx(ctx,tx,e,h,*w.Attempt);err!=nil{return err}
	var oldHash *string
	if err:=tx.QueryRow(ctx,`SELECT result_digest FROM chartworks.frozen_run_payloads WHERE tenant_id=$1 AND operation_id=$2 FOR UPDATE`,e.Tenant(),h.view.ID).Scan(&oldHash);err!=nil{return err}
	hash:=readexec.Hash(*w.Result)
	if oldHash!=nil{if *oldHash==hash{return nil};return store.ErrConflict}
	raw,err:=json.Marshal(w.Result);if err!=nil{return store.ErrInvalid}
	if err=frozenBytesTx(ctx,tx,e,h,int64(len(raw)));err!=nil{return err}
	if _,err=tx.Exec(ctx,`UPDATE chartworks.frozen_run_payloads SET result=$3,result_digest=$4 WHERE tenant_id=$1 AND operation_id=$2 AND result IS NULL`,e.Tenant(),h.view.ID,raw,hash);err!=nil{return err}
	_,err=tx.Exec(ctx,`UPDATE chartworks.frozen_runs SET state='normalized',observed_at=$3 WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),h.view.ID,*w.Attempt.Finished)
	return err
}

func frozenOutputTx(ctx context.Context,tx pgx.Tx,e identity.Envelope,h frozenHead,w reporting.RunWrite)error{
	if w.Output==nil||h.view.State!="normalized"{return store.ErrConflict}
	o:=*w.Output;starting:=w.Kind=="output_start"
	if err:=reporting.CheckFrozenOutput(w.Manifest,o,starting);err!=nil{return err}
	ordinal:=-1
	for index,output:=range w.Manifest.Outputs{if output.ID==o.ID{ordinal=index;break}}
	if ordinal<0{return store.ErrInvalid}
	var oldRaw []byte;var oldState string
	err:=tx.QueryRow(ctx,`SELECT state,payload FROM chartworks.frozen_run_outputs WHERE tenant_id=$1 AND operation_id=$2 AND output_id=$3 FOR UPDATE`,e.Tenant(),h.view.ID,o.ID).Scan(&oldState,&oldRaw)
	exists:=err==nil
	if err!=nil&&!errors.Is(err,pgx.ErrNoRows){return err}
	if starting&&exists{return store.ErrConflict}
	if !starting&&exists{
		var old reporting.RetainedOutput
		if json.Unmarshal(oldRaw,&old)!=nil{return store.ErrInvalid}
		if oldState!="indeterminate"{if old.Digest==o.Digest{return nil};return store.ErrConflict}
		if o.Kind!="narrative"||o.ReservedCalls!=old.ReservedCalls||o.ReservedTokens!=old.ReservedTokens{return store.ErrInvalid}
	}
	if !starting&&!exists&&o.Kind=="narrative"&&(o.State!="failed"||o.Code!="narrative_unavailable"||o.ReservedCalls!=0||o.ReservedTokens!=0){return store.ErrConflict}
	if starting{
		if o.ReservedCalls>w.Manifest.Limits.NarrativeCalls-h.view.ReservedCalls||o.ReservedTokens>w.Manifest.Limits.NarrativeTokens-h.view.ReservedTokens{return reporting.ErrBudget}
		if _,err=tx.Exec(ctx,`UPDATE chartworks.frozen_runs SET reserved_calls=reserved_calls+$3,reserved_tokens=reserved_tokens+$4 WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),h.view.ID,o.ReservedCalls,o.ReservedTokens);err!=nil{return err}
	}
	raw,err:=json.Marshal(o);if err!=nil{return store.ErrInvalid}
	if err=frozenBytesTx(ctx,tx,e,h,int64(len(raw)-len(oldRaw)));err!=nil{return err}
	if exists{
		_,err=tx.Exec(ctx,`UPDATE chartworks.frozen_run_outputs SET state=$4,payload=$5 WHERE tenant_id=$1 AND operation_id=$2 AND output_id=$3`,e.Tenant(),h.view.ID,o.ID,o.State,raw)
	}else{
		_,err=tx.Exec(ctx,`INSERT INTO chartworks.frozen_run_outputs(tenant_id,operation_id,output_id,ordinal,kind,state,payload) VALUES($1,$2,$3,$4,$5,$6,$7)`,e.Tenant(),h.view.ID,o.ID,ordinal,o.Kind,o.State,raw)
	}
	return err
}

// CheckpointFrozenRun holds the same live operation fence as cancellation and
// completion. Old owners cannot publish query results, model receipts or outputs.
func(d *DB) CheckpointFrozenRun(ctx context.Context,inv jobs.Invocation,proof reporting.PreparedRunWrite)(out reporting.RunRecord,err error){
	w,err:=proof.Checked(inv);if err!=nil{return out,err}
	e,err:=inv.Current("reporting.run",w.Manifest.Block,w.Manifest.RequestHash);if err!=nil{return out,err}
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,err};defer stop()
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		if _,fenceErr:=requestFenceTx(ctx,tx,inv);fenceErr!=nil{return fenceErr}
		h,readErr:=frozenReadHeadTx(ctx,tx,e,w.Manifest.ID,true,true);if readErr!=nil{return readErr}
		if h.view.ManifestDigest!=w.Manifest.Digest()||h.taskHash!=inv.Lease().Task.ManifestHash{return store.ErrConflict}
		if !time.Now().Before(h.view.Expires)||h.view.State=="expired"{return reporting.ErrExpired}
		if h.view.State!="sealed"&&h.view.State!="normalized"{return store.ErrConflict}
		if readErr=frozenCurrentTx(ctx,tx,e,w.Manifest);readErr!=nil{return readErr}
		action:="reporting.query_checkpoint"
		switch w.Kind{
		case "attempt":
			if w.Attempt==nil{return store.ErrInvalid}
			readErr=frozenJournalTx(ctx,tx,e,h,*w.Attempt)
		case "result": readErr=frozenResultTx(ctx,tx,e,h,w)
		case "output_start","output":
			readErr=frozenOutputTx(ctx,tx,e,h,w)
			action="reporting.output_checkpoint";if w.Kind=="output_start"{action="reporting.output_started"}
		case "complete":
			current,currentErr:=frozenReadTx(ctx,tx,e,w.Manifest.ID,true,true);if currentErr!=nil{return currentErr}
			if current.Result==nil{return reporting.ErrIncomplete}
			state,code,completionErr:=reporting.FrozenCompletion(w.Manifest,current.Outputs);if completionErr!=nil{return completionErr}
			if state!=w.Outcome||code!=w.Code{return store.ErrInvalid}
			_,readErr=tx.Exec(ctx,`UPDATE chartworks.frozen_runs SET state=$3,code=$4,reserved_bytes=0,finished_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),h.view.ID,state,code)
			if readErr==nil{readErr=completeRequestTx(ctx,tx,inv)}
			action="reporting.run_completed"
		default:return store.ErrInvalid
		}
		if readErr!=nil{return readErr}
		if readErr=frozenAudit(ctx,tx,e,action,w.Manifest.ID);readErr!=nil{return readErr}
		out,readErr=frozenReadTx(ctx,tx,e,w.Manifest.ID,true,true);return readErr
	})
	if err!=nil{return reporting.RunRecord{},err};return out,nil
}

// CancelFrozenRun records durable cancellation and forwards intent into the
// common read journal. A returned cancellation is not proof of native termination.
func(d *DB) CancelFrozenRun(ctx context.Context,e identity.Envelope,id string)(out reporting.RunView,err error){
	if !e.Valid(){return out,access.ErrUnauthenticated};if !e.Has("jobs.cancel"){return out,access.ErrForbidden}
	ctx,stop,err:=requestContext(ctx,e);if err!=nil{return out,err};defer stop()
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
		task,readErr:=readOwnedRequestTx(ctx,tx,e,id,true);if readErr!=nil{return readErr}
		if task.Input.Kind!="reporting.run"{return store.ErrNotFound}
		if readErr=access.Require(e,"jobs.cancel",access.Resource{Tenant:e.Tenant(),Kind:"block",Permission:"execute",ID:task.Input.Target});readErr!=nil{return readErr}
		// Read/cancel does not require the run action. Its actor/session and
		// current context eligibility are checked before loading any payload.
		var contextID string;var private bool
		if readErr=tx.QueryRow(ctx,`SELECT context_id,private FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2 AND actor_id=$3 AND session_id=$4`,e.Tenant(),id,e.User(),e.Session()).Scan(&contextID,&private);readErr!=nil{return readErr}
		if readErr=access.Require(e,"jobs.cancel",access.Resource{Tenant:e.Tenant(),Kind:"execution_context",Permission:"use",ID:contextID});readErr!=nil{return readErr}
		if private{if readErr=reporting.Require(e,task.Input.Target,reporting.Preview);readErr!=nil{return readErr}}
		if _,readErr=cancelRequestTaskTx(ctx,tx,e,task);readErr!=nil{return readErr}
		if _,readErr=tx.Exec(ctx,`UPDATE chartworks.read_attempts SET cancel_requested=true WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND finished_at IS NULL`,e.Tenant(),e.User(),id);readErr!=nil{return readErr}
		if readErr=frozenAudit(ctx,tx,e,"reporting.run_cancelled",id);readErr!=nil{return readErr}
		out=reporting.RunView{ID:id,Block:task.Input.Target,State:"cancelled",Context:contextID,Private:private,QueryAttempts:[]readexec.Attempt{},Outputs:[]reporting.OutputSummary{},Parameters:[]reporting.BoundValue{}}
		if task.State=="succeeded"{out.State="completed"}
		return nil
	})
	if err!=nil{return reporting.RunView{},err};return out,nil
}
