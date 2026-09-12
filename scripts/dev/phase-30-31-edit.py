"""Temporary branch-only editing aid; remove before final review."""
from pathlib import Path
import subprocess
changed = set()
def replace(path, old, new):
    p=Path(path); text=p.read_text()
    if new in text: return
    if text.count(old)!=1: raise RuntimeError(f'Expected one anchor in {path}: {old[:180]!r}')
    p.write_text(text.replace(old,new)); changed.add(path)
def write(path, text):
    p=Path(path)
    if p.exists(): raise RuntimeError(f'Unexpected existing file: {path}')
    p.write_text(text)
    subprocess.run(['git','add','-N','--',path],check=True)
    changed.add(path)

write('internal/store/postgres/migrations/031_nested_report_leases.sql', '''-- Sequential frozen children share their owning report's counted root slot.
-- Parentage is immutable operational provenance, never an identity grant.
ALTER TABLE chartworks.operations ADD COLUMN nested_parent text;
ALTER TABLE chartworks.operations ADD COLUMN nested_fence bigint;
ALTER TABLE chartworks.operations ADD CONSTRAINT nested_parent_reference
 FOREIGN KEY(tenant_id,nested_parent) REFERENCES chartworks.operations(tenant_id,operation_id);
ALTER TABLE chartworks.operations ADD CONSTRAINT nested_child_shape CHECK (
 (nested_parent IS NULL AND nested_fence IS NULL) OR
 (nested_parent IS NOT NULL AND nested_fence IS NOT NULL AND nested_fence>0 AND nested_parent<>operation_id
  AND dispatch_mode='request' AND kind='reporting.run'));
CREATE UNIQUE INDEX one_unfinished_report_child ON chartworks.operations(tenant_id,nested_parent)
 WHERE nested_parent IS NOT NULL AND status IN('pending','retry','running');
CREATE INDEX report_child_parent ON chartworks.operations(tenant_id,nested_parent) WHERE nested_parent IS NOT NULL;
CREATE FUNCTION chartworks.protect_report_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE parent chartworks.operations%ROWTYPE;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.nested_parent IS DISTINCT FROM OLD.nested_parent OR
     (NEW.nested_fence IS NOT NULL AND NEW.nested_fence<OLD.nested_fence) THEN
   RAISE EXCEPTION 'immutable report child parent' USING ERRCODE='55000';
  END IF;
 END IF;
 IF NEW.nested_parent IS NOT NULL AND (TG_OP='INSERT' OR NEW.nested_fence IS DISTINCT FROM OLD.nested_fence) THEN
  SELECT * INTO STRICT parent FROM chartworks.operations
   WHERE tenant_id=NEW.tenant_id AND operation_id=NEW.nested_parent FOR KEY SHARE;
  IF parent.nested_parent IS NOT NULL OR parent.kind NOT IN('report.run','dashboard.run','reporting.scheduled') OR
     (parent.kind='reporting.scheduled' AND parent.request_manifest->>'kind'<>'report.run') OR
     parent.actor_id<>NEW.actor_id OR
     CASE WHEN parent.dispatch_mode='queued' THEN parent.operation_id ELSE parent.initiator_session END <> NEW.initiator_session OR
     parent.status<>'running' OR parent.fence<>NEW.nested_fence OR parent.lease_until<=clock_timestamp() OR parent.expires_at<=clock_timestamp() OR
     NEW.expires_at>parent.expires_at THEN
   RAISE EXCEPTION 'invalid report child ownership' USING ERRCODE='55000';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER report_child_immutable BEFORE INSERT OR UPDATE ON chartworks.operations
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_report_child();

-- An orphan/old-fence child is counted independently until its live lease ends.
-- It cannot continue publishing: the domain transaction also checks its parent.
CREATE VIEW chartworks.active_execution_roots AS
 SELECT o.tenant_id,o.operation_id FROM chartworks.operations o
 WHERE o.dispatch_mode IN('queued','request') AND o.status='running'
 AND o.lease_until>clock_timestamp() AND o.expires_at>clock_timestamp()
 AND NOT EXISTS(SELECT 1 FROM chartworks.operations p
  WHERE p.tenant_id=o.tenant_id AND p.operation_id=o.nested_parent
  AND p.nested_parent IS NULL AND p.status='running' AND p.fence=o.nested_fence
  AND p.lease_until>clock_timestamp() AND p.expires_at>clock_timestamp());
CREATE VIEW chartworks.pending_execution_roots AS
 SELECT o.tenant_id,o.operation_id FROM chartworks.operations o
 WHERE o.dispatch_mode IN('queued','request') AND o.status IN('pending','retry','running')
 AND NOT EXISTS(SELECT 1 FROM chartworks.operations p
  WHERE p.tenant_id=o.tenant_id AND p.operation_id=o.nested_parent
  AND p.nested_parent IS NULL AND p.status IN('pending','retry','running'));
''')

write('internal/store/postgres/nested_requests.go', '''package postgres

import (
 "context"
 "errors"
 "time"

 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/jobs"
 "github.com/hurtener/chartworks/internal/store"
 "github.com/jackc/pgx/v5"
)

var _ jobs.NestedRequestRepository = (*DB)(nil)

func nestedRequestAuthority(parent jobs.Invocation, input jobs.RequestInput) (identity.Envelope, error) {
 p:=parent.Lease().Task
 if _, nested:=parent.Parent(); nested || (p.Input.Kind!="report.run" && p.Input.Kind!="dashboard.run") || input.Kind!="reporting.run" || input.Context!="" {
  return identity.Envelope{}, jobs.ErrAuthority
 }
 e,err:=parent.Current(p.Input.Kind,p.Input.Target,p.Input.InputHash)
 if err!=nil {return identity.Envelope{},err}
 if err=input.Require(e);err!=nil{return identity.Envelope{},err}
 return e,nil
}

// AdmitNestedRequest requires the opaque live report owner; request input alone
// can never turn an ordinary task into a capacity-exempt child.
func(d *DB) AdmitNestedRequest(ctx context.Context,parent jobs.Invocation,key string,input jobs.RequestInput,l jobs.Limits)(jobs.RequestTask,error){
 e,err:=nestedRequestAuthority(parent,input)
 if err!=nil{return jobs.RequestTask{},err}
 return d.admitRequest(ctx,e,key,input,l,&parent)
}

// requireRequestParentTx locks parents before children, consistently with parent
// cancellation and publication. Only a matching current parent fence is accepted.
func requireRequestParentTx(ctx context.Context,tx pgx.Tx,i jobs.Invocation) error {
 l:=i.Lease()
 parent,nested:=i.Parent()
 expected:=""
 var fence int64
 if nested {
  if _,err:=nestedRequestAuthority(parent,l.Task.Input);err!=nil{return err}
  if _,err:=requestFenceTx(ctx,tx,parent);err!=nil{return err}
  expected=parent.Lease().Task.ID
  fence=parent.Lease().Fence
 }
 var actual string
 var actualFence int64
 if err:=tx.QueryRow(ctx,`SELECT COALESCE(nested_parent,''),COALESCE(nested_fence,0) FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`,l.Task.Tenant,l.Task.ID).Scan(&actual,&actualFence);err!=nil{return err}
 if actual!=expected || actualFence!=fence{return jobs.ErrAuthority}
 return nil
}

func checkNestedReplayTx(ctx context.Context,tx pgx.Tx,task jobs.RequestTask,parent *jobs.Invocation) error {
 expected:=""
 if parent!=nil {expected=parent.Lease().Task.ID}
 var actual string
 if err:=tx.QueryRow(ctx,`SELECT COALESCE(nested_parent,'') FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`,task.Tenant,task.ID).Scan(&actual);err!=nil{return err}
 if actual!=expected{return store.ErrConflict}
 return nil
}

func prepareNestedAdmissionTx(ctx context.Context,tx pgx.Tx,parent *jobs.Invocation,input jobs.RequestInput) error {
 if parent==nil{return nil}
 if _,err:=nestedRequestAuthority(*parent,input);err!=nil{return err}
 task,err:=requestFenceTx(ctx,tx,*parent)
 if err!=nil{return err}
 var count int
 if err=tx.QueryRow(ctx,`SELECT count(*) FROM chartworks.operations WHERE tenant_id=$1 AND nested_parent=$2`,task.Tenant,task.ID).Scan(&count);err!=nil{return err}
 if count>=100{return jobs.ErrBusy}
 return nil
}

// ClaimNestedRequest transfers no authority. The parent's existing root slot
// covers exactly one sequential child, enforced by a database unique index.
func(d *DB) ClaimNestedRequest(ctx context.Context,parent jobs.Invocation,id,owner string,l jobs.Limits)(out jobs.RequestLease,err error){
 if !identity.Identifier(id)||!identity.Identifier(owner)||l.Validate()!=nil{return out,jobs.ErrInvalid}
 p:=parent.Lease().Task
 e,err:=parent.Current(p.Input.Kind,p.Input.Target,p.Input.InputHash)
 if err!=nil{return out,err}
 ctx,stop,err:=requestContext(ctx,e)
 if err!=nil{return out,err}
 defer stop()
 err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error{
  if err:=queueLock(ctx,tx,l);err!=nil{return err}
  if _,err:=requestFenceTx(ctx,tx,parent);err!=nil{return err}
  task,err:=readRequestTx(ctx,tx,e,id,true)
  if err!=nil{return err}
  if _,err=nestedRequestAuthority(parent,task.Input);err!=nil{return err}
  if task.Dispatch!=nil{return jobs.ErrAuthority}
  if err=checkNestedReplayTx(ctx,tx,task,&parent);err!=nil{return err}
  if !time.Now().Before(task.Expires){return store.ErrExpired}
  var eligible bool
  if err=tx.QueryRow(ctx,`SELECT status IN('pending','retry','running') AND next_attempt_at<=clock_timestamp() AND (lease_until IS NULL OR lease_until<=clock_timestamp()) AND attempt_count<max_attempts FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),id).Scan(&eligible);err!=nil{return err}
  if !eligible{return store.ErrConflict}
  fence,attempt,until,err:=claimOperationLease(ctx,tx,e.Tenant(),id,owner,l.Lease)
  if err!=nil{return err}
  if _,err=tx.Exec(ctx,`UPDATE chartworks.operations SET nested_fence=$3 WHERE tenant_id=$1 AND operation_id=$2`,e.Tenant(),id,parent.Lease().Fence);err!=nil{return err}
  task.State="running";task.Attempts=attempt
  out=jobs.RequestLease{Task:task,Owner:owner,Fence:fence,Attempt:attempt,Until:until}
  return nil
 })
 if errors.Is(err,pgx.ErrNoRows){err=store.ErrConflict}
 if err!=nil{return jobs.RequestLease{},err}
 return out,nil
}
''')

p='internal/store/postgres/request_jobs.go'
signature='func (d *DB) AdmitRequest(ctx context.Context, e identity.Envelope, key string, input jobs.RequestInput, l jobs.Limits) (out jobs.RequestTask, err error) {'
replace(p,signature, signature+'\n return d.admitRequest(ctx,e,key,input,l,nil)\n}\n\nfunc (d *DB) admitRequest(ctx context.Context, e identity.Envelope, key string, input jobs.RequestInput, l jobs.Limits, parent *jobs.Invocation) (out jobs.RequestTask, err error) {')
replace(p,'\t\t\tout = previous\n\t\t\treturn nil', '\t\t\tif err=checkNestedReplayTx(ctx,tx,previous,parent);err!=nil{return err}\n\t\t\tout = previous\n\t\t\treturn nil')
replace(p,'\t\tif err = queueCapacity(ctx, tx, e.Tenant(), l); err != nil {\n\t\t\treturn err\n\t\t}\n\t\t// Retained terminal', '\t\tif parent==nil {\n if err=queueCapacity(ctx,tx,e.Tenant(),l);err!=nil{return err}\n } else if err=prepareNestedAdmissionTx(ctx,tx,parent,input);err!=nil{return err}\n\t\t// Retained terminal')
replace(p,'\t\tout.ManifestHash = out.Digest()', '\t\tif parent!=nil && out.Expires.After(parent.Lease().Task.Expires){out.Expires=parent.Lease().Task.Expires}\n\t\tout.ManifestHash = out.Digest()')
replace(p,'\t\tbody, _ := json.Marshal(input)', '\t\tvar parentID string\n var parentFence int64\n if parent!=nil { parentID=parent.Lease().Task.ID;parentFence=parent.Lease().Fence }\n\t\tbody, _ := json.Marshal(input)')
replace(p,'next_attempt_at,request_manifest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,$8,$9,\'request\',$3,$10,$8,$8,$8,$11,$12,$8,$13)', 'next_attempt_at,request_manifest,nested_parent,nested_fence) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,$8,$9,\'request\',$3,$10,$8,$8,$8,$11,$12,$8,$13,NULLIF($14,\'\'),NULLIF($15,0))')
replace(p,'out.ManifestHash, out.MaxAttempts, body)', 'out.ManifestHash, out.MaxAttempts, body, parentID, parentFence)')
# Root claiming cannot adopt an already delegated child.
replace(p,'\t\tif task.Dispatch != nil {\n\t\t\treturn jobs.ErrAuthority\n\t\t}\n\t\tif !time.Now()', '\t\tif task.Dispatch != nil {\n\t\t\treturn jobs.ErrAuthority\n\t\t}\n if err=checkNestedReplayTx(ctx,tx,task,nil);err!=nil{return err}\n\t\tif !time.Now()')
replace(p,"SELECT count(*),count(*) FILTER(WHERE tenant_id=$1) FROM chartworks.operations WHERE dispatch_mode IN('queued','request') AND status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()", "SELECT count(*),count(*) FILTER(WHERE tenant_id=$1) FROM chartworks.active_execution_roots")
replace(p,'\t\tvar live bool\n\t\tif err := tx.QueryRow(ctx, `SELECT status,', '\t\tif err:=requireRequestParentTx(ctx,tx,i);err!=nil{return err}\n\t\tvar live bool\n\t\tif err := tx.QueryRow(ctx, `SELECT status,')
replace(p,'\ttask, err := readRequestTx(ctx, tx, e, l.Task.ID, true)', '\tif err:=requireRequestParentTx(ctx,tx,i);err!=nil{return jobs.RequestTask{},err}\n\ttask, err := readRequestTx(ctx, tx, e, l.Task.ID, true)')
replace(p,'\ttag, err := tx.Exec(ctx, `UPDATE chartworks.operations SET status=\'succeeded\'', '\tif err:=requireRequestParentTx(ctx,tx,i);err!=nil{return err}\n\ttag, err := tx.Exec(ctx, `UPDATE chartworks.operations SET status=\'succeeded\'')
p='internal/store/postgres/jobs.go'
replace(p,"SELECT count(*),count(*) FILTER(WHERE tenant_id=$1) FROM chartworks.operations WHERE dispatch_mode IN ('queued','request') AND status IN ('pending','retry','running')", "SELECT count(*),count(*) FILTER(WHERE tenant_id=$1) FROM chartworks.pending_execution_roots")
replace(p,"SELECT count(*) FROM chartworks.operations WHERE dispatch_mode IN ('queued','request') AND status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()", "SELECT count(*) FROM chartworks.active_execution_roots")
replace(p,"SELECT count(*) FROM chartworks.operations a WHERE a.dispatch_mode IN ('queued','request') AND a.tenant_id=o.tenant_id AND a.status='running' AND a.lease_until>clock_timestamp()", "SELECT count(*) FROM chartworks.active_execution_roots a WHERE a.tenant_id=o.tenant_id")
subprocess.run(['gofmt','-w',*sorted(p for p in changed if p.endswith('.go'))],check=True)
subprocess.run(['git','diff','--check'],check=True)
