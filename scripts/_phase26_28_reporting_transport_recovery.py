# Recovery payload for the interrupted local implementation. This file is kept
# only on the isolated development branch; the resulting Go files are verified
# before publication to the feature branch.
from pathlib import Path

files = {
'internal/reportingapi/runs_registry.go': r'''package reportingapi

import (
 "reflect"
 "github.com/hurtener/chartworks/internal/api"
 "github.com/hurtener/chartworks/internal/reporting"
)

// RunAdmission reserves intent, never caller-provided authority or result data.
type RunAdmission struct {
 Block string `json:"block"`
 Request reporting.RunRequest `json:"request"`
}

// RunExecution explicitly starts or resumes an already admitted frozen run.
type RunExecution struct {
 ID string `json:"id"`
 Resume bool `json:"resume"`
}

// RetentionRequest bounds one explicitly authorized expiration pass.
type RetentionRequest struct { Limit int `json:"limit"` }

// RetentionResult counts artifacts whose expired values were erased.
type RetentionResult struct { Expired int64 `json:"expired"` }

// RunsRegistry preserves retained reads and cancellation when sources are disabled.
func RunsRegistry(execution bool) (*api.Registry, error) {
 entries := []struct {
  method, path, action, id, summary, effect string
  in, out reflect.Type
  execute bool
 }{
  {"POST", "/v1/block-runs", "reporting.execute", "admitBlockRun", "Reserve and seal an exact block run without executing a source or model", "reporting_run_admission", reflect.TypeFor[RunAdmission](), reflect.TypeFor[reporting.RunView](), true},
  {"POST", "/v1/block-runs/execute", "reporting.execute", "executeBlockRun", "Execute or explicitly resume one fenced frozen run with optional saved narrative", "reporting_run_execution", reflect.TypeFor[RunExecution](), reflect.TypeFor[reporting.RunView](), true},
  {"GET", "/v1/block-runs/{id}", "reporting.execute", "inspectBlockRun", "Inspect an actor and session bound run without source or model work", "retained_metadata_read", nil, reflect.TypeFor[reporting.RunView](), false},
  {"POST", "/v1/block-runs/{id}/cancel", "jobs.cancel", "cancelBlockRun", "Record cancellation intent without claiming native work has stopped", "durable_cancellation", reflect.TypeFor[struct{}](), reflect.TypeFor[reporting.RunView](), false},
  {"GET", "/v1/artifacts", "reporting.read", "listArtifacts", "List retained artifact summaries under current target and partition reach", "retained_artifact_read", nil, reflect.TypeFor[reporting.ArtifactList](), false},
  {"GET", "/v1/artifacts/{id}", "reporting.read", "readArtifact", "Read a retained run summary or permitted expiry tombstone without executing work", "retained_artifact_read", nil, reflect.TypeFor[reporting.RunView](), false},
  {"GET", "/v1/artifacts/{id}/result", "reporting.read", "readArtifactRows", "Page exact ordered retained result values without issuing another query", "retained_artifact_read", nil, reflect.TypeFor[reporting.ResultPage](), false},
  {"GET", "/v1/artifacts/{id}/output", "reporting.read", "readArtifactOutput", "Read one exact retained output including saved narrative provenance", "retained_artifact_read", nil, reflect.TypeFor[reporting.RetainedOutput](), false},
  {"POST", "/v1/artifact-retention", "reporting.retention", "expireArtifacts", "Erase expired values and outputs in one bounded authorized tenant pass", "retained_artifact_expiration", reflect.TypeFor[RetentionRequest](), reflect.TypeFor[RetentionResult](), false},
 }
 definitions := make([]api.Definition, 0, len(entries))
 for _, entry := range entries {
  if entry.execute && !execution { continue }
  response, err := api.SchemaFor(entry.id+"Response", entry.out, true)
  if err != nil { return nil, err }
  d := api.Definition{Operation: api.Operation{Method:entry.method, Path:entry.path, Action:entry.action, Effect:entry.effect}, ID:entry.id, Summary:entry.summary,
   ResourceLoader:"reporting.Runs and PostgreSQL current target, original actor/session, privacy and execution partition selection",
   Audit:"shared operation/read journal and frozen artifact checkpoints; no SQL, values or tokens in audit", Response:response, Replay:"never", Errors:runErrors()}
  if entry.in != nil {
   d.MaxBodyBytes=MaxBodyBytes
   d.Request,err=api.SchemaFor(entry.id+"Request",entry.in,false,api.NullableCollections)
   if err != nil { return nil,err }
  }
  if entry.method=="GET" { d.Replay="read"; d.Audit="read_only_no_domain_audit" }
  switch entry.id {
  case "listArtifacts":
   d.Query=[]api.Parameter{{Name:"after",In:"query",Type:"string",Max:128},{Name:"limit",In:"query",Type:"integer",Min:1,Max:100}}
  case "readArtifactRows":
   d.Query=[]api.Parameter{{Name:"offset",In:"query",Type:"integer",Min:0,Max:10000},{Name:"limit",In:"query",Type:"integer",Min:1,Max:1000}}
  case "readArtifactOutput":
   d.Query=[]api.Parameter{{Name:"output",In:"query",Type:"string",Required:true,Max:128}}
  }
  definitions=append(definitions,d)
 }
 return api.New(definitions)
}

func runErrors() []api.ErrorResponse {
 return []api.ErrorResponse{
  {Status:400,Code:"invalid_request"},{Status:401,Code:"unauthenticated"},{Status:401,Code:"unauthorized"},{Status:403,Code:"forbidden"},{Status:404,Code:"not_found"},
  {Status:409,Code:"conflict"},{Status:409,Code:"stale_validation"},{Status:409,Code:"incomplete"},{Status:409,Code:"reconciliation_required"},{Status:410,Code:"expired"},
  {Status:413,Code:"limit_exceeded"},{Status:422,Code:"invalid_query"},{Status:429,Code:"busy"},{Status:429,Code:"budget_exhausted"},{Status:503,Code:"unavailable"},{Status:504,Code:"cancelled_or_timed_out"},
 }
}
''',
'internal/reportingapi/runs_http.go': r'''package reportingapi

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "strconv"
 "github.com/hurtener/chartworks/internal/access"
 "github.com/hurtener/chartworks/internal/api"
 "github.com/hurtener/chartworks/internal/auth"
 "github.com/hurtener/chartworks/internal/exec"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/jobs"
 "github.com/hurtener/chartworks/internal/reporting"
 "github.com/hurtener/chartworks/internal/store"
)

// RunsHandler applies the common verifier before closed requests reach the core.
func RunsHandler(verifier *auth.Verifier, service *reporting.Runs, next http.Handler) http.Handler {
 if verifier==nil || next==nil { return http.NotFoundHandler() }
 if service==nil { return next }
 registry,err:=RunsRegistry(service.CanExecute())
 if err!=nil { return http.HandlerFunc(func(w http.ResponseWriter,_ *http.Request){runFailure(w,err)}) }
 slots:=make(chan struct{},16)
 protected:=verifier.Middleware(auth.HTTP,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  headers(w)
  d,id,_:=registry.Match(r.Method,r.URL.Path)
  if d.Path=="" {w.WriteHeader(http.StatusMethodNotAllowed);return}
  e,err:=identity.FromContext(r.Context())
  if err!=nil {runFailure(w,access.ErrUnauthenticated);return}
  if !e.Has(d.Action) {runFailure(w,access.ErrForbidden);return}
  select {case slots<-struct{}{}:defer func(){<-slots}();default:runFailure(w,reporting.ErrBusy);return}
  if r.URL.RawPath!="" || r.Header.Get("Content-Encoding")!="" || r.Header.Get("Idempotency-Key")!="" {runFailure(w,reporting.ErrInvalid);return}
  q,err:=url.ParseQuery(r.URL.RawQuery)
  if err!=nil || !validQuery(q,d.Query) {runFailure(w,reporting.ErrInvalid);return}
  if r.Method==http.MethodGet {
   body,bodyErr:=io.ReadAll(http.MaxBytesReader(w,r.Body,1))
   if bodyErr!=nil || len(body)>0 {runFailure(w,reporting.ErrInvalid);return}
  }
  out,err:=dispatchRun(w,r,e,service,d,id,q)
  if err!=nil {runFailure(w,err);return}
  raw,err:=json.Marshal(out)
  if err!=nil {runFailure(w,err);return}
  if len(raw)>service.ResponseByteLimit() {runFailure(w,exec.ErrLimit);return}
  if !e.Valid() {runFailure(w,access.ErrUnauthenticated);return}
  if err=r.Context().Err();err!=nil {runFailure(w,err);return}
  _,_=w.Write(append(raw,'\n'))
 }))
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if _,_,known:=registry.Match(r.Method,r.URL.Path);known {protected.ServeHTTP(w,r);return}
  next.ServeHTTP(w,r)
 })
}

func dispatchRun(w http.ResponseWriter,r *http.Request,e identity.Envelope,s *reporting.Runs,d api.Definition,id string,q url.Values)(any,error){
 switch d.ID {
 case "admitBlockRun":
  var in RunAdmission
  if err:=decode(w,r,d,&in);err!=nil{return nil,err}
  return s.Admit(r.Context(),e,in.Block,in.Request)
 case "executeBlockRun":
  var in RunExecution
  if err:=decode(w,r,d,&in);err!=nil{return nil,err}
  return s.Run(r.Context(),e,in.ID,in.Resume)
 case "inspectBlockRun":return s.Inspect(r.Context(),e,id)
 case "cancelBlockRun":
  var in struct{}
  if err:=decode(w,r,d,&in);err!=nil{return nil,err}
  return s.Cancel(r.Context(),e,id)
 case "listArtifacts":
  limit,err:=runInteger(q,"limit",20,1,100)
  if err!=nil{return nil,err}
  return s.List(r.Context(),e,q.Get("after"),limit)
 case "readArtifact":return s.Get(r.Context(),e,id)
 case "readArtifactRows":
  offset,err:=runInteger(q,"offset",0,0,10000)
  if err!=nil{return nil,err}
  limit,err:=runInteger(q,"limit",min(100,s.PageRowLimit()),1,s.PageRowLimit())
  if err!=nil{return nil,err}
  return s.Rows(r.Context(),e,id,offset,limit)
 case "readArtifactOutput":return s.Output(r.Context(),e,id,q.Get("output"))
 case "expireArtifacts":
  var in RetentionRequest
  if err:=decode(w,r,d,&in);err!=nil{return nil,err}
  count,err:=s.Expire(r.Context(),e,in.Limit)
  return RetentionResult{Expired:count},err
 default:return nil,store.ErrNotFound
 }
}

func runInteger(q url.Values,name string,fallback,minimum,maximum int)(int,error){
 values,found:=q[name]
 if !found{return fallback,nil}
 if len(values)!=1{return 0,reporting.ErrInvalid}
 n,err:=strconv.Atoi(values[0])
 if err!=nil || n<minimum || n>maximum || strconv.Itoa(n)!=values[0] {return 0,reporting.ErrInvalid}
 return n,nil
}

func runClassify(err error)(int,string){
 switch {
 case errors.Is(err,reporting.ErrExpired),errors.Is(err,store.ErrExpired):return 410,"expired"
 case errors.Is(err,reporting.ErrIncomplete):return 409,"incomplete"
 case errors.Is(err,exec.ErrUncertain):return 409,"reconciliation_required"
 case errors.Is(err,reporting.ErrBudget):return 429,"budget_exhausted"
 case errors.Is(err,jobs.ErrBusy):return 429,"busy"
 case errors.Is(err,jobs.ErrInvalid):return 400,"invalid_request"
 case errors.Is(err,jobs.ErrAuthority):return 403,"forbidden"
 default:return classify(err)
 }
}
func runFailure(w http.ResponseWriter,err error){
 status,code:=runClassify(err);headers(w);w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(struct{Error string `json:"error"`}{code})
}
''',
'internal/reportingapi/runs_mcp.go': r'''package reportingapi

import (
 "context"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/mcpserver"
 "github.com/hurtener/chartworks/internal/reporting"
)

// ArtifactReference identifies an existing run, never execution authority.
type ArtifactReference struct { ID string `json:"id"` }
// ArtifactPageRequest addresses a bounded current-authority catalog page.
type ArtifactPageRequest struct { After string `json:"after"`; Limit int `json:"limit"` }
// ArtifactRowsRequest addresses exact retained cells, not a source query.
type ArtifactRowsRequest struct { ID string `json:"id"`; Offset int `json:"offset"`; Limit int `json:"limit"` }
// ArtifactOutputRequest addresses a selected output from the original run.
type ArtifactOutputRequest struct { ID string `json:"id"`; Output string `json:"output"` }

// RunMCPBindings shares the actual HTTP contracts and frozen domain core. It
// exposes no authoring, certification, retention or L2 mutation as an agent tool.
func RunMCPBindings(s *reporting.Runs)([]mcpserver.Binding,error){
 if s==nil{return nil,nil}
 registry,err:=RunsRegistry(s.CanExecute())
 if err!=nil{return nil,err}
 mapper:=func(err error)mcpserver.Fault{_,code:=runClassify(err);return mcpserver.Fault{Code:code}}
 var bindings []mcpserver.Binding
 summary,err:=mcpserver.Bind(registry,"readArtifact","read_artifact","reporting","Read an existing retained artifact summary or expiry tombstone with current target and partition reach. Never queries a warehouse or regenerates narrative text.",func(ctx context.Context,e identity.Envelope,in ArtifactReference)(reporting.RunView,error){return s.Get(ctx,e,in.ID)},mapper)
 if err!=nil{return nil,err}
 summary,err=mcpserver.WithResource(summary,"chartworks://artifacts/{+id}")
 if err!=nil{return nil,err}
 bindings=append(bindings,summary)
 list,err:=mcpserver.Bind(registry,"listArtifacts","list_artifacts","reporting","List a bounded page of retained artifacts currently readable in your target and execution contexts. Use an empty after cursor initially and a limit from 1 to 100. No source or model work.",func(ctx context.Context,e identity.Envelope,in ArtifactPageRequest)(reporting.ArtifactList,error){return s.List(ctx,e,in.After,in.Limit)},mapper)
 if err!=nil{return nil,err};bindings=append(bindings,list)
 rows,err:=mcpserver.Bind(registry,"readArtifactRows","read_artifact_rows","reporting","Read an offset page of exact ordered cells from a retained artifact. Limit must fit the server page bound. Expired values are not re-queried; no warehouse or model work occurs.",func(ctx context.Context,e identity.Envelope,in ArtifactRowsRequest)(reporting.ResultPage,error){return s.Rows(ctx,e,in.ID,in.Offset,in.Limit)},mapper)
 if err!=nil{return nil,err};bindings=append(bindings,rows)
 output,err:=mcpserver.Bind(registry,"readArtifactOutput","read_artifact_output","reporting","Read one saved output of an existing artifact using its output ID. Returns exact retained chart data or grounded narrative provenance; never regenerates or executes a source query.",func(ctx context.Context,e identity.Envelope,in ArtifactOutputRequest)(reporting.RetainedOutput,error){return s.Output(ctx,e,in.ID,in.Output)},mapper)
 if err!=nil{return nil,err};bindings=append(bindings,output)
 if s.CanExecute(){
  admit,err:=mcpserver.Bind(registry,"admitBlockRun","admit_block_run","reporting","Reserve and seal a block revision, parameters and selected outputs under a stable request key. Persists admission; does not execute SQL or call a model. Use execute_block_run with the returned ID. Changed requests require a new key.",func(ctx context.Context,e identity.Envelope,in RunAdmission)(reporting.RunView,error){return s.Admit(ctx,e,in.Block,in.Request)},mapper)
  if err!=nil{return nil,err};bindings=append(bindings,admit)
  execute,err:=mcpserver.Bind(registry,"executeBlockRun","execute_block_run","reporting","Execute one explicitly admitted frozen run by ID. May query a warehouse and generate an explicitly enabled saved narrative, never new SQL or chart choices. Inspect receipts on failure; set resume only for an intentional retry.",func(ctx context.Context,e identity.Envelope,in RunExecution)(reporting.RunView,error){return s.Run(ctx,e,in.ID,in.Resume)},mapper)
  if err!=nil{return nil,err};bindings=append(bindings,execute)
 }
 return bindings,nil
}
''',
}
for name, content in files.items():
    p=Path(name)
    assert not p.exists(), name
    p.write_text(content)
