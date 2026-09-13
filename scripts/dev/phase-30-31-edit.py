"""Temporary exact branch integration; remove before final PR."""
from pathlib import Path
import re
import subprocess
changed=set()
def edit(path,old,new,marker=None,count=1):
 p=Path(path);s=p.read_text()
 if (marker is not None and marker in s) or new in s:return
 if s.count(old)!=count:raise RuntimeError(f'{path}: expected {count} anchors, got {s.count(old)}: {old[:150]!r}')
 p.write_text(s.replace(old,new));changed.add(path)
def transform(path,fn):
 p=Path(path);old=p.read_text();new=fn(old)
 if new!=old:p.write_text(new);changed.add(path)
def function(path,signature,fn):
 def run(s):
  start=s.index(signature);end=s.find('\nfunc ',start+len(signature))
  if end<0:end=len(s)
  return s[:start]+fn(s[start:end])+s[end:]
 transform(path,run)

p='internal/jobs/reporting_target.go'
edit(p,'type ReportingTarget struct {','type ReportingTarget struct {\n\tPartialFailure string `json:"partial_failure,omitempty" jsonschema:"enum=fail_closed,enum=allow_partial"`\n\tRecipients []string `json:"recipients,omitempty"`','PartialFailure string')
edit(p,'type ReportingPin struct {','type ReportingPin struct {\n\tBlock string `json:"block"`','Block string `json:"block"`')
transform(p,lambda s:s.replace('b.QueryAttempts <= 8','b.QueryAttempts <= 800').replace('b.ModelCalls <= 8','b.ModelCalls <= 64').replace('b.ModelTokens <= 128<<10','b.ModelTokens <= 16<<20'))
edit(p,'if !identity.Identifier(p.ID) || p.Revision < 1','if !identity.Identifier(p.ID) || !identity.Identifier(p.Block) || p.Revision < 1')
edit(p,'func (t ReportingTarget) Valid() bool {','''func (t ReportingTarget) Valid() bool {
    if !slices.Contains([]string{"", "fail_closed", "allow_partial"},t.PartialFailure) || len(t.Recipients)>32 { return false }
    recipients:=map[string]bool{}
    for _,id:=range t.Recipients { if !identity.Identifier(id) || recipients[id] { return false }; recipients[id]=true }
    if t.Type=="saved_question" && len(t.Arguments)!=0 { return false }''','recipients:=map[string]bool{}')
edit(p,'reporting ReportingExecutor) (*Service, error) {','reporting ReportingExecutor, observer ...func(string)) (*Service, error) {')
function(p,'func NewWithReporting(',lambda s:s.replace('New(repo, authority, limits)','New(repo, authority, limits, observer...)'))
p='internal/jobs/jobs.go'
edit(p,'type Job struct {','type Job struct {\n\tDelivery *ReportingReceipt `json:"delivery,omitempty"`','Delivery *ReportingReceipt')
edit(p,'j.ManifestHash == j.Digest()','j.ManifestHash == j.Digest() && (j.Reporting == nil || j.Reporting.Input == ReportingInput(j))')
p='internal/jobs/service.go'
edit(p,'\tcase errors.Is(err, ErrAuthority):','\tcase errors.Is(err, ErrReportingBudget):\n\t\tcode, permanent = "reporting_budget", true\n\tcase errors.Is(err, ErrReportingAttention):\n\t\tcode, permanent = "reporting_attention", true\n\tcase errors.Is(err, ErrAuthority):')
p='internal/store/postgres/jobs.go'
edit(p,'\tj.ManifestHash = j.Digest()','''    if request.Reporting != nil {
        resolved, resolveErr := reportingDispatchTx(ctx, tx, scope.Tenant(), *request.Reporting)
        if resolveErr != nil { return jobs.Job{}, resolveErr }
        j.Reporting = &resolved
        j.Reporting.Input = jobs.ReportingInput(j)
    }
\tj.ManifestHash = j.Digest()''')
edit(p,'\tvar dispatched, input []byte\n','''\tvar dispatched, input []byte
    if j.Reporting != nil {
        dispatched, input, err = reportingDispatchJSON(j)
        if err != nil { return jobs.Job{}, err }
    }
''')
edit(p,'\tif err = auditJob(ctx, tx, scope, "job.accepted", j.ID); err != nil {','\tif err = insertReportingDeliveryTx(ctx, tx, j); err != nil { return jobs.Job{}, err }\n\tif err = auditJob(ctx, tx, scope, "job.accepted", j.ID); err != nil {')
function(p,'func (d *DB) ReadJob(',lambda s:s.replace('\n\t\treturn e\n','\n\t\tif e == nil { out.Delivery, e = reportingReceiptTx(ctx, tx, out) }\n\t\treturn e\n',1))
function(p,'func (d *DB) ListJobs(',lambda s:s.replace('\t\treturn rows.Err()','''        if err:=rows.Err(); err!=nil { return err }
        rows.Close()
        for i:=range out { var err error; out[i].Delivery,err=reportingReceiptTx(ctx,tx,out[i]); if err!=nil{return err} }
        return nil''',1))
transform(p,lambda s:s.replace('case "authority_blocked", "attempt_failed", "attempt_timeout", "definition_changed":','case "authority_blocked", "attempt_failed", "attempt_timeout", "definition_changed", "reporting_budget", "reporting_attention":'))
p='internal/store/postgres/reporting_jobs.go'
edit(p,'j.Reporting.Input != jobs.ReportingInput(j) || !j.Valid()','j.Reporting.Input != jobs.ReportingInput(j) || !j.Valid() || !reportingWindow(j)')
edit(p,'\t\tif !normalized {','\t\tif err != nil { return err }\n\t\tif !normalized {')
p='internal/reporting/runs_service.go'
edit(p,'s.seal(ctx, e, id, in, requestHash, task)','s.seal(ctx, e, id, in, requestHash, task, nil)')
edit(p,'func (s *Runs) seal(ctx context.Context, e identity.Envelope, id string, in RunRequest, requestHash string, task jobs.RequestTask) (RunView, error) {','func (s *Runs) seal(ctx context.Context, e identity.Envelope, id string, in RunRequest, requestHash string, task jobs.RequestTask, invocation *jobs.Invocation) (RunView, error) {')
edit(p,'\tsealed, err := s.repo.SealFrozenRun(ctx, e, task, proof)\n\treturn sealed.View, err','''    var sealed RunRecord
    if invocation == nil {
        sealed, err = s.repo.SealFrozenRun(ctx, e, task, proof)
    } else {
        repo, ok := s.repo.(ScheduledRunRepository)
        if !ok { return RunView{}, ErrUnavailable }
        sealed, err = repo.SealScheduledFrozenRun(ctx, *invocation, proof)
    }
\treturn sealed.View, err''')
p='internal/reporting/scheduled.go'
transform(p,lambda s:s.replace('ResolveWidgetArguments(', 'widgetArguments(').replace('Narrative: narrative, PartialPolicy: "fail"','Narrative: narrative, PartialPolicy: "allow_partial"'))
p='internal/reporting/compositions_service.go'
edit(p,'\tdefinition, err := ProjectStoredDocument(root.Revision.Raw, kind)','''    if task.Dispatch != nil && task.Dispatch.Kind == jobs.ReportingKind {
        dispatch:=task.Dispatch.Reporting
        if kind!="report" || dispatch==nil || dispatch.Blocked!="" || in.Preview || root.Revision.Number!=dispatch.Revision || root.Revision.Digest!=dispatch.Digest { return CompositionManifest{}, ErrStale }
    }
\tdefinition, err := ProjectStoredDocument(root.Revision.Raw, kind)''')
edit(p,'''\t\td, err := ProjectStoredDocument(source.snapshot.Revision.Raw, "report")
\t\tif err != nil {
\t\t\treturn CompositionManifest{}, err
\t\t}''','''\t\td, err := ProjectStoredDocument(source.snapshot.Revision.Raw, "report")
\t\tif err != nil { return CompositionManifest{}, err }
        if task.Dispatch != nil && task.Dispatch.Kind == jobs.ReportingKind {
            d,err=scheduledDefinition(d,task.Dispatch.Reporting)
            if err!=nil{return CompositionManifest{},err}
        }''','d,err=scheduledDefinition')
transform(p,lambda s:s.replace('case errors.Is(err, ErrBudget), errors.Is(err, exec.ErrLimit):','case errors.Is(err, ErrBudget), errors.Is(err, exec.ErrLimit), errors.Is(err, jobs.ErrReportingBudget):'))

p='internal/store/postgres/frozen_runs_write.go'
sig='func (d *DB) SealFrozenRun(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedRun) (out reporting.RunRecord, err error) {'
edit(p,sig,sig+'\n return d.sealFrozenRun(ctx,e,task,proof,nil)\n}\n\nfunc (d *DB) sealFrozenRun(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedRun, invocation *jobs.Invocation) (out reporting.RunRecord, err error) {','func (d *DB) sealFrozenRun(')
function(p,'func (d *DB) sealFrozenRun(',lambda s:s.replace('err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {','err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {\n if invocation!=nil { if _,err:=requestFenceTx(ctx,tx,*invocation);err!=nil{return err} }',1).replace('!time.Now().Before(actual.Expires) || actual.State != "pending"','!time.Now().Before(actual.Expires) || (invocation==nil && (actual.State!="pending" || actual.Dispatch!=nil)) || (invocation!=nil && (actual.State!="running" || actual.Dispatch==nil))',1))
p='internal/store/postgres/compositions_admission.go'
sig='func (d *DB) SealComposition(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedComposition) (out reporting.CompositionRecord, err error) {'
edit(p,sig,sig+'\n return d.sealComposition(ctx,e,task,proof,nil)\n}\n\nfunc (d *DB) sealComposition(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedComposition, invocation *jobs.Invocation) (out reporting.CompositionRecord, err error) {','func (d *DB) sealComposition(')
function(p,'func (d *DB) sealComposition(',lambda s:s.replace('err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {','err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {\n if invocation!=nil { if _,err:=requestFenceTx(ctx,tx,*invocation);err!=nil{return err} }',1).replace('actual.State != "pending" || !time.Now().Before(actual.Expires)','!time.Now().Before(actual.Expires) || (invocation==nil && (actual.State!="pending" || actual.Dispatch!=nil)) || (invocation!=nil && (actual.State!="running" || actual.Dispatch==nil))',1))
p='internal/store/postgres/request_jobs.go'
function(p,'func completeRequestTx(',lambda s:s.replace('\tl := i.Lease()','\tl := i.Lease()\n if l.Task.Dispatch!=nil && l.Task.Dispatch.Kind==jobs.ReportingKind { return completeReportingRequestTx(ctx,tx,i) }',1))
p='internal/gateway/bifrost/engine.go'
def reserve(s):
 n=s.count('b.Reserve(call,')
 if n==0 and 'gateway.ReserveAttempt(ctx, b, call,' in s:return s
 if n!=3:raise RuntimeError(f'expected 3 actual SDK attempt reservations, got {n}')
 return s.replace('b.Reserve(call,','gateway.ReserveAttempt(ctx, b, call,')
transform(p,reserve)
p='internal/exec/execution.go'
edit(p,'\tif err = x.repo.BeginRead(ctx, scope, a, x.settings.MaxReadAttempts); err != nil {','\tif err = reserveAttempt(ctx, e, o); err != nil { return ExecutionReport{}, err }\n\tif err = x.repo.BeginRead(ctx, scope, a, x.settings.MaxReadAttempts); err != nil {')

# Bootstrap every consumer before any worker starts or route captures its queue.
p='internal/foundation/work.go'
edit(p,'sourceapi.Handler(verifier, w.sourceService, validator, workapi.Handler(verifier, w.engine, w.queue, next))','sourceapi.Handler(verifier, w.sourceService, validator, next)')
def bootstrap(s):
 marker='reporting.NewScheduled(delivery, db)'
 if marker in s:return s
 start=s.index('\tif v.Jobs.Enabled {\n\t\tobserve := func(stage string)')
 end=s.index('\n\tw.handler = sourceapi.Handler',start)
 s=s[:start]+s[end:]
 start=s.index('\tw.autopilot, err = engineering.NewAutopilotWithSchedules(')
 end=s.index('\n\tdocumentRegistry, delivery, handler, err := mountDocuments',start)
 runtime=s[start:end]
 s=s[:start]+s[end:]
 anchor='\tw.handler = handler\n'
 addition='''\tw.handler = handler
    if v.Jobs.Enabled {
        scheduled,makeErr:=reporting.NewScheduled(delivery, db)
        if makeErr!=nil { w.close();return nil,makeErr }
        var pipeline jobs.PipelineExecutor
        if w.pipelines.Enabled(){pipeline=w.pipelines}
        observe:=func(stage string){w.logger.Error("durable work failed; inspect current job receipt and dependency status","stage",stage)}
        w.queue,err=jobs.NewWithReporting(db,w.broker,jobLimits(v.Jobs),pipeline,scheduled,observe)
        if err!=nil{w.close();return nil,err}
    }
'''+runtime+'\n\tw.handler=workapi.Handler(verifier,w.engine,w.queue,w.handler)\n'
 if s.count(anchor)!=1:raise RuntimeError('bootstrap handler anchor')
 return s.replace(anchor,addition,1)
transform(p,bootstrap)

p='internal/store/postgres/migrations/033_reporting_occurrences.sql'
def schema(s):
 s=s.replace('query_limit BETWEEN 1 AND 8','query_limit BETWEEN 1 AND 800').replace('query_reservations BETWEEN 0 AND 8','query_reservations BETWEEN 0 AND 800')
 s=s.replace('model_call_limit BETWEEN 0 AND 8','model_call_limit BETWEEN 0 AND 64').replace('model_call_reservations BETWEEN 0 AND 8','model_call_reservations BETWEEN 0 AND 64').replace('131072','16777216')
 if 'reporting.delivery_completed' not in s:
  s+='''
-- Fixed reporting failure stages remain content-free across retries/history.
DO $$ DECLARE previous text; relation text; constraint_name text; BEGIN
 FOR relation,constraint_name IN VALUES ('operations','operations_error_code_check'),('operation_attempts','operation_attempts_error_code_check') LOOP
  SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
  WHERE conrelid=('chartworks.'||relation)::regclass AND conname=constraint_name;
  EXECUTE format('ALTER TABLE chartworks.%I DROP CONSTRAINT %I',relation,constraint_name);
  EXECUTE format('ALTER TABLE chartworks.%I ADD CONSTRAINT %I CHECK ((%s) OR error_code IN (''reporting_budget'',''reporting_attention''))',relation,constraint_name,previous);
 END LOOP;
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''reporting.delivery_completed'',''reporting.delivery_blocked''))',previous);
END $$;
'''
 return s
transform(p,schema)
# Newly added source is formatted along with edits, including files not changed
# by this integration step. The script never modifies locked dependencies.
for path in ['internal/jobs/reporting_runtime.go','internal/reporting/scheduled.go','internal/reporting/scheduled_proof.go','internal/store/postgres/reporting_jobs.go','internal/store/postgres/reporting_seals.go','internal/exec/reservation.go','internal/gateway/reservation.go']:
 changed.add(path)
subprocess.run(['gofmt','-w',*sorted(p for p in changed if p.endswith('.go'))],check=True)
subprocess.run(['git','diff','--check'],check=True)
