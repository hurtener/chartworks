"""Temporary branch-only editing aid; remove before final review."""
from pathlib import Path
import subprocess
changed=set()
def replace(path,old,new):
 p=Path(path);text=p.read_text()
 if new in text:return
 if text.count(old)!=1:raise RuntimeError(f'Expected one anchor in {path}: {old[:200]!r}')
 p.write_text(text.replace(old,new));changed.add(path)

p='internal/reporting/runs_service.go'
signature='func (s *Runs) Admit(ctx context.Context, e identity.Envelope, id string, input RunRequest) (RunView, error) {'
replace(p,signature,signature+'\n return s.admit(ctx,e,id,input,nil)\n}\n\nfunc (s *Runs) admit(ctx context.Context,e identity.Envelope,id string,input RunRequest,parent *jobs.Invocation)(RunView,error){')
replace(p,'\ttask, err := s.runner.Admit(ctx, e, in.Key, jobs.RequestInput{Kind: "reporting.run", Target: id, InputHash: requestHash})',''' var task jobs.RequestTask
 request:=jobs.RequestInput{Kind:"reporting.run",Target:id,InputHash:requestHash}
 if parent==nil {task,err=s.runner.Admit(ctx,e,in.Key,request)} else {task,err=s.runner.AdmitNested(ctx,*parent,in.Key,request)}''')
replace(p,'\texisting, err := s.repo.ReadFrozenRun(ctx, e, task.ID, true)', ''' out,err:=s.seal(ctx,e,id,in,requestHash,task)
 if err!=nil && parent!=nil && ctx.Err()==nil {
  if _,cancelErr:=s.runner.Cancel(ctx,e,task.ID);cancelErr!=nil && !errors.Is(cancelErr,store.ErrConflict){return out,errors.Join(err,cancelErr)}
 }
 return out,err
}

// seal uses the same proof and storage path for caller and accepted scheduled
// work. The supplied task must match the canonical request, actor and session.
func (s *Runs) seal(ctx context.Context,e identity.Envelope,id string,in RunRequest,requestHash string,task jobs.RequestTask)(RunView,error){
 if err:=task.Require(e);err!=nil{return RunView{},err}
 if task.Input!=(jobs.RequestInput{Kind:"reporting.run",Target:id,InputHash:requestHash}){return RunView{},jobs.ErrAuthority}
\texisting, err := s.repo.ReadFrozenRun(ctx, e, task.ID, true)''')

p='internal/reporting/runs_execution.go'
signature='func (s *Runs) Run(ctx context.Context, e identity.Envelope, id string, resume bool) (RunView, error) {'
replace(p,signature,signature+'\n return s.run(ctx,e,id,resume,nil)\n}\n\nfunc (s *Runs) run(ctx context.Context,e identity.Envelope,id string,resume bool,parent *jobs.Invocation)(RunView,error){')
replace(p,'''\t_, runErr := s.runner.Run(ctx, e, task, min(time.Duration(r.Manifest.Limits.Timeout), time.Duration(s.limits.Timeout)), func(work context.Context, inv jobs.Invocation) error {
\t\treturn s.continueFrozen(work, e, inv, r)
\t})''',''' handler:=func(work context.Context,inv jobs.Invocation)error{return s.continueFrozen(work,e,inv,r)}
 timeout:=min(time.Duration(r.Manifest.Limits.Timeout),time.Duration(s.limits.Timeout))
 var runErr error
 if parent==nil {_,runErr=s.runner.Run(ctx,e,task,timeout,handler)} else {_,runErr=s.runner.RunNested(ctx,*parent,task,timeout,handler)}
 if runErr!=nil && parent!=nil && ctx.Err()==nil {
  if _,cancelErr:=s.runner.Cancel(ctx,e,task.ID);cancelErr!=nil && !errors.Is(cancelErr,store.ErrConflict){return r.View,errors.Join(runErr,cancelErr)}
 }''')
p='internal/reporting/compositions_execution.go'
replace(p,'s.executeBlock(ctx, e, m, group)', 's.executeBlock(ctx, e, inv, m, group)')
replace(p,'func (s *Compositions) executeBlock(ctx context.Context, e identity.Envelope, m CompositionManifest, g CompositionGroup)', 'func (s *Compositions) executeBlock(ctx context.Context, e identity.Envelope, inv jobs.Invocation, m CompositionManifest, g CompositionGroup)')
replace(p,'''\tchild, err := s.runs.Admit(ctx, e, g.Block, RunRequest{Key: "composition:" + m.ID + ":" + g.ID,
\t\tReference: Reference{Revision: g.Revision}, Arguments: g.Arguments, Resolution: g.Resolution,
\t\tOutputs: g.Outputs, Policy: policy, Locale: g.Locale, Narrative: g.Narrative, PartialPolicy: "allow_partial"})''','''\tchild, err := s.runs.admit(ctx, e, g.Block, RunRequest{Key: "composition:" + m.ID + ":" + g.ID,
\t\tReference: Reference{Revision: g.Revision}, Arguments: g.Arguments, Resolution: g.Resolution,
\t\tOutputs: g.Outputs, Policy: policy, Locale: g.Locale, Narrative: g.Narrative, PartialPolicy: "allow_partial"}, &inv)''')
replace(p,'s.runs.Run(ctx, e, child.ID, resume)', 's.runs.run(ctx, e, child.ID, resume, &inv)')
subprocess.run(['gofmt','-w',*sorted(changed)],check=True)
subprocess.run(['git','diff','--check'],check=True)
