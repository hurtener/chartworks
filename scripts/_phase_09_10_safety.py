from pathlib import Path

def replace(path,old,new):
 p=Path(path);s=p.read_text()
 if new in s:return
 assert old in s,(path,old[:100])
 p.write_text(s.replace(old,new,1))

# Cancellation is durable intent. Only the live owner signals its original pgx
# connection (and secret); external callers never pg_cancel_backend a reusable PID.
replace('internal/exec/execution.go','native, runErr := x.adapter.ExecuteRead(ctx, e, p, limits, a.ID, observer)','''nativeContext,joinWatcher:=watchRead(ctx,observer)
 native, runErr := x.adapter.ExecuteRead(nativeContext, e, p, limits, a.ID, observer)
 if watchErr:=joinWatcher();watchErr!=nil { runErr=watchErr;native.Result=Result{} }''')
p=Path('internal/exec/execution.go');s=p.read_text()
if 'func watchRead(' not in s:
 s+='''
// watchRead owns one bounded, joined cancellation observer per active read.
// Metadata failure cancels work instead of silently losing the cancellation fence.
func watchRead(ctx context.Context,observer Observer) (context.Context,func() error) {
 child,cancel:=context.WithCancel(ctx)
 stop:=make(chan struct{});done:=make(chan error,1)
 go func(){
  ticker:=time.NewTicker(25*time.Millisecond);defer ticker.Stop()
  for { select {
  case <-stop:done<-nil;return
  case <-ctx.Done():done<-nil;return
  case <-ticker.C:
   probe,end:=context.WithTimeout(child,time.Second)
   err:=observer.Check(probe);end()
   if err!=nil { cancel();done<-err;return }
  } }
 }()
 return child,func() error { close(stop);err:=<-done;cancel();return err }
}
'''
p.write_text(s)
replace('internal/exec/execution.go','report := ExecutionReport{Attempt: current}','''current,err=x.repo.GetRead(cleanup,scope,a.ID)
 if err!=nil { return ExecutionReport{},ErrUncertain }
 status=current.Status
 report := ExecutionReport{Attempt: current}''')
p=Path('internal/store/postgres/read_executions.go');s=p.read_text()
s=s.replace('SET status=$4,remote_state=$5,finished_at=$6,rows_returned=$7,bytes_returned=$8,code=$9 WHERE',"SET status=CASE WHEN cancel_requested AND $4 NOT IN ('uncertain','interrupted') THEN 'cancelled' ELSE $4 END,remote_state=$5,finished_at=$6,rows_returned=CASE WHEN cancel_requested THEN 0 ELSE $7 END,bytes_returned=CASE WHEN cancel_requested THEN 0 ELSE $8 END,code=CASE WHEN cancel_requested AND $4 NOT IN ('uncertain','interrupted') THEN 'cancelled' ELSE $9 END WHERE")
p.write_text(s)
p=Path('internal/sources/read.go');s=p.read_text();a=s.index('func (s *Service) ControlRead(')
if 'SELECT pg_cancel_backend' in s[a:]:
 b=s.index('\t\tif cancel {',a);end=s.index('\n\t\tvar exists bool',b)
 s=s[:b]+s[end:]
 s=s.replace('// A successful pg_cancel_backend return is a request receipt, not termination proof.','// Cancellation signals are sent only by the live owner using its original connection secret.\n// ControlRead itself only observes; it cannot accidentally cancel a reused backend PID.')
 p.write_text(s)
# The existing metadata suite must count the actual forward migration, not evade it.
p=Path('test/acceptance/phase02_test.go');s=p.read_text();s=s.replace('`SELECT count(*) FROM chartworks.schema_migrations`) != 5','`SELECT count(*) FROM chartworks.schema_migrations`) != 6')
if '"read_attempts"' not in s:s=s.replace('"vector_facets"','"read_attempts", "vector_facets"',1)
p.write_text(s)
# Lookup by logical operation is necessary after a client loses the first response/attempt ID.
replace('internal/exec/execution.go','GetRead(context.Context, store.Scope, string) (Attempt, error)','GetRead(context.Context, store.Scope, string) (Attempt, error)\n GetReadOperation(context.Context,store.Scope,string) (Attempt,error)')
p=Path('internal/exec/execution.go');s=p.read_text()
if 'func (x *Executor) ByOperation(' not in s:s+='''
// ByOperation recovers a content-free attempt after a lost response. It never reruns values.
func (x *Executor) ByOperation(ctx context.Context,e identity.Envelope,operation string) (Attempt,error) {
 if ctx==nil || !e.Valid() || !identity.Identifier(operation) { return Attempt{},ErrBinding }
 scope,err:=store.NewScope(e.Tenant(),e.User());if err!=nil { return Attempt{},err }
 a,err:=x.repo.GetReadOperation(ctx,scope,operation);if err!=nil { return Attempt{},err }
 return x.Inspect(ctx,e,a.ID)
}
'''
p.write_text(s)
p=Path('internal/store/postgres/read_executions.go');s=p.read_text()
if 'func (d *DB) GetReadOperation(' not in s:s+='''
// GetReadOperation finds an actor's latest physical attempt for a retained logical key.
func (d *DB) GetReadOperation(ctx context.Context,s store.Scope,operation string) (a readexec.Attempt,err error) {
 if !s.Valid() { return a,store.ErrScope };if !identity.Identifier(operation) { return a,store.ErrInvalid }
 err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx) error {
  var e error;a,e=scanRead(tx.QueryRow(ctx,`SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 ORDER BY attempt_number DESC LIMIT 1`,s.Tenant(),s.Actor(),operation));return e
 })
 if err!=nil { return readexec.Attempt{},err };return a,nil
}
'''
p.write_text(s)
# Surface files in this commit have not yet been gofmt'd.
p=Path('internal/sourceapi/execution.go');s=p.read_text()
if 'Path:"/v1/read-operations/{id}"' not in s and 'Path: "/v1/read-operations/{id}"' not in s:
 s=s.replace('return []Operation{','return []Operation{\n {Method:"GET",Path:"/v1/read-operations/{id}",Action:"sources.query",Effect:"attempt_metadata_read"},',1)
 s=s.replace('switch op.Path {','switch op.Path {\n case "/v1/read-operations/{id}":out,err=executor.ByOperation(r.Context(),e,id)',1)
p.write_text(s)
p=Path('sdk/chartworks/execution.go');s=p.read_text()
if 'func (c *Client) ReadOperation(' not in s:s+='''
// ReadOperation recovers the latest attempt ID without replaying a lost query response.
func (c *Client) ReadOperation(ctx context.Context,id string) (out ReadAttempt,err error) {
 if !wireID(id) { return out,errors.New("chartworks: invalid operation identifier") }
 err=c.call(ctx,"GET","/v1/read-operations/"+id,"",nil,&out);return out,err
}
'''
p.write_text(s)
