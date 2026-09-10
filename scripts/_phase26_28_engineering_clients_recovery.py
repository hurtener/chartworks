# Isolated recovery payload; only verified Go source is published to the feature.
from pathlib import Path
files={
'internal/sourceapi/autopilot.go':r'''package sourceapi

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "mime"
 "net/http"
 "reflect"
 "github.com/hurtener/chartworks/internal/access"
 "github.com/hurtener/chartworks/internal/api"
 "github.com/hurtener/chartworks/internal/auth"
 "github.com/hurtener/chartworks/internal/engineering"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/store"
)

const proposalBodyLimit=2<<20

// AutopilotAPIRegistry exposes reviewed L2 work, never L3 policy or unattended effects.
// Disabling new planning preserves current-authority reads of retained proposals.
func AutopilotAPIRegistry(enabled bool)(*api.Registry,error){
 entries:=[]struct{
  method,path,action,id,summary,effect string
  in,out reflect.Type
 }{
  {"GET","/v1/engineering-proposals/{id}","engineering.autopilot.read","readEngineeringProposal","Read currently authorized reviewed proposal material and actual effects","retained_metadata_read",nil,reflect.TypeFor[engineering.AutopilotProposal]()},
  {"POST","/v1/engineering-proposals","engineering.autopilot.propose","proposeEngineering","Create a scope-bounded L2 proposal without executing or publishing its effects","reviewed_engineering_proposal",reflect.TypeFor[engineering.AutopilotGoal](),reflect.TypeFor[engineering.AutopilotProposal]()},
  {"PUT","/v1/engineering-proposals/{id}","engineering.autopilot.propose","editEngineeringProposal","Append a proposal revision and invalidate previous approval","reviewed_engineering_proposal_edit",reflect.TypeFor[engineering.AutopilotEditRequest](),reflect.TypeFor[engineering.AutopilotProposal]()},
  {"POST","/v1/engineering-proposals/{id}/review","engineering.autopilot.review","reviewEngineeringProposal","Independently approve or reject one exact proposal digest","reviewed_engineering_decision",reflect.TypeFor[engineering.AutopilotReviewRequest](),reflect.TypeFor[engineering.AutopilotProposal]()},
  {"POST","/v1/engineering-proposals/{id}/apply","engineering.autopilot.apply","applyEngineeringProposal","Apply the reviewed managed pipeline through ordinary scopes and staged effect gates","reviewed_engineering_apply",reflect.TypeFor[engineering.AutopilotApplyRequest](),reflect.TypeFor[engineering.AutopilotProposal]()},
  {"POST","/v1/engineering-proposals/{id}/compensate","engineering.autopilot.compensate","compensateEngineeringProposal","Quarantine an eligible owned generation without claiming global warehouse rollback","reviewed_engineering_compensation",reflect.TypeFor[engineering.AutopilotApplyRequest](),reflect.TypeFor[engineering.AutopilotProposal]()},
  {"POST","/v1/engineering-proposals/{id}/drift","engineering.autopilot.drift","detectEngineeringDrift","Observe drift and retain a deduplicated amendment without publishing changed meaning","reviewed_engineering_drift",reflect.TypeFor[struct{}](),reflect.TypeFor[engineering.AutopilotDrift]()},
 }
 definitions:=make([]api.Definition,0,len(entries))
 for _,entry:=range entries {
  if !enabled && entry.method!="GET" {continue}
  response,err:=api.SchemaFor(entry.id+"Response",entry.out,true)
  if err!=nil{return nil,err}
  d:=api.Definition{Operation:api.Operation{Method:entry.method,Path:entry.path,Action:entry.action,Effect:entry.effect},ID:entry.id,Summary:entry.summary,ResourceLoader:"engineering.Autopilot and PostgreSQL complete tenant, proposal, source, dataset and context selection",Audit:"immutable proposal decisions and actual pipeline effects; no SQL, prompts or tokens in audit",Response:response,Replay:"never",Errors:proposalErrors()}
  if entry.in!=nil {
   d.MaxBodyBytes=proposalBodyLimit
   d.Request,err=api.SchemaFor(entry.id+"Request",entry.in,false,api.NullableCollections)
   if err!=nil{return nil,err}
  } else {d.Replay="read";d.Audit="read_only_no_domain_audit"}
  definitions=append(definitions,d)
 }
 return api.New(definitions)
}

// AutopilotHandler forwards the verified envelope and closed DTOs to the one L2
// core. A request body cannot supply approving identity or overwrite evidence.
func AutopilotHandler(verifier *auth.Verifier,service *engineering.Autopilot,next http.Handler)http.Handler{
 if verifier==nil || next==nil{return http.NotFoundHandler()}
 if service==nil{return next}
 registry,err:=AutopilotAPIRegistry(service.Enabled())
 if err!=nil{return http.HandlerFunc(func(w http.ResponseWriter,_ *http.Request){proposalFailure(w,err)})}
 slots:=make(chan struct{},16)
 protected:=verifier.Middleware(auth.HTTP,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  proposalHeaders(w)
  d,id,_:=registry.Match(r.Method,r.URL.Path)
  if d.Path==""{w.WriteHeader(http.StatusMethodNotAllowed);return}
  e,err:=identity.FromContext(r.Context())
  if err!=nil{proposalFailure(w,access.ErrUnauthenticated);return}
  if !e.Has(d.Action){proposalFailure(w,access.ErrForbidden);return}
  select{case slots<-struct{}{}:defer func(){<-slots}();default:proposalFailure(w,engineering.ErrLimit);return}
  if r.URL.RawPath!="" || r.URL.RawQuery!="" || r.Header.Get("Content-Encoding")!="" || r.Header.Get("Idempotency-Key")!=""{proposalFailure(w,store.ErrInvalid);return}
  var out any
  if r.Method==http.MethodGet{
   raw,bodyErr:=io.ReadAll(http.MaxBytesReader(w,r.Body,1))
   if bodyErr!=nil || len(raw)!=0 {proposalFailure(w,store.ErrInvalid);return}
   out,err=service.Get(r.Context(),e,id)
  }else{out,err=dispatchProposal(w,r,e,service,d,id)}
  if err!=nil{proposalFailure(w,err);return}
  raw,err:=json.Marshal(out)
  if err!=nil{proposalFailure(w,err);return}
  if len(raw)>proposalBodyLimit{proposalFailure(w,engineering.ErrLimit);return}
  if !e.Valid(){proposalFailure(w,access.ErrUnauthenticated);return}
  if err=r.Context().Err();err!=nil{proposalFailure(w,err);return}
  _,_=w.Write(append(raw,'\n'))
 }))
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if _,_,known:=registry.Match(r.Method,r.URL.Path);known {protected.ServeHTTP(w,r);return};next.ServeHTTP(w,r)
 })
}

func dispatchProposal(w http.ResponseWriter,r *http.Request,e identity.Envelope,s *engineering.Autopilot,d api.Definition,id string)(any,error){
 switch d.ID{
 case "proposeEngineering":
  var in engineering.AutopilotGoal
  if err:=proposalBody(w,r,d,&in);err!=nil{return nil,err}
  return s.Propose(r.Context(),e,in)
 case "editEngineeringProposal":
  var in engineering.AutopilotEditRequest
  if err:=proposalBody(w,r,d,&in);err!=nil{return nil,err}
  return s.Edit(r.Context(),e,id,in)
 case "reviewEngineeringProposal":
  var in engineering.AutopilotReviewRequest
  if err:=proposalBody(w,r,d,&in);err!=nil{return nil,err}
  return s.Review(r.Context(),e,id,in)
 case "applyEngineeringProposal","compensateEngineeringProposal":
  var in engineering.AutopilotApplyRequest
  if err:=proposalBody(w,r,d,&in);err!=nil{return nil,err}
  if d.ID=="applyEngineeringProposal"{return s.Apply(r.Context(),e,id,in)}
  return s.Compensate(r.Context(),e,id,in)
 case "detectEngineeringDrift":
  var in struct{}
  if err:=proposalBody(w,r,d,&in);err!=nil{return nil,err}
  return s.DetectDrift(r.Context(),e,id)
 default:return nil,store.ErrNotFound
 }
}

func proposalBody(w http.ResponseWriter,r *http.Request,d api.Definition,out any)error{
 media,params,err:=mime.ParseMediaType(r.Header.Get("Content-Type"))
 if err!=nil || media!="application/json" || len(params)!=0{return store.ErrInvalid}
 raw,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,int64(d.MaxBodyBytes)))
 if err!=nil {var limit *http.MaxBytesError;if errors.As(err,&limit){return engineering.ErrLimit};return store.ErrInvalid}
 if d.Request==nil || d.Request.Validate(raw,d.MaxBodyBytes)!=nil || json.Unmarshal(raw,out)!=nil{return store.ErrInvalid}
 return nil
}
func proposalHeaders(w http.ResponseWriter){
 w.Header().Set("Content-Type","application/json");w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("Referrer-Policy","no-referrer")
}
func proposalClassify(err error)(int,string){
 switch{
 case errors.Is(err,engineering.ErrProposalReview):return 409,"review_required"
 case errors.Is(err,engineering.ErrProposalDrift):return 409,"proposal_drift"
 case errors.Is(err,engineering.ErrProposalConflict),errors.Is(err,store.ErrConflict),errors.Is(err,engineering.ErrState):return 409,"conflict"
 case errors.Is(err,engineering.ErrCompensationBlocked):return 409,"compensation_blocked"
 case errors.Is(err,engineering.ErrPipelineUncertain):return 409,"reconciliation_required"
 case errors.Is(err,engineering.ErrPipelineQuality):return 422,"pipeline_quality_failed"
 case errors.Is(err,engineering.ErrOwnership):return 403,"forbidden"
 case errors.Is(err,engineering.ErrInvalid):return 400,"invalid_request"
 case errors.Is(err,engineering.ErrLimit):return 413,"limit_exceeded"
 case errors.Is(err,context.Canceled),errors.Is(err,context.DeadlineExceeded):return 504,"cancelled_or_timed_out"
 default:return classify(err)
 }
}
func proposalFailure(w http.ResponseWriter,err error){
 status,code:=proposalClassify(err);proposalHeaders(w);w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(struct{Error string `json:"error"`}{code})
}
func proposalErrors()[]api.ErrorResponse{
 return []api.ErrorResponse{
  {Status:400,Code:"invalid_request"},{Status:401,Code:"unauthenticated"},{Status:401,Code:"unauthorized"},{Status:403,Code:"forbidden"},{Status:404,Code:"not_found"},
  {Status:409,Code:"conflict"},{Status:409,Code:"review_required"},{Status:409,Code:"proposal_drift"},{Status:409,Code:"compensation_blocked"},{Status:409,Code:"reconciliation_required"},{Status:409,Code:"context_changed"},
  {Status:410,Code:"expired"},{Status:413,Code:"limit_exceeded"},{Status:422,Code:"pipeline_quality_failed"},{Status:422,Code:"unsafe_sql"},{Status:429,Code:"busy"},
  {Status:503,Code:"unavailable"},{Status:504,Code:"cancelled_or_timed_out"},
 }
}
''',
'sdk/chartworks/runs.go':r'''package chartworks

import (
 "context"
 "net/url"
 "strconv"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/reporting"
)

// BlockRunRequest is caller intent, not SQL or an execution manifest.
type BlockRunRequest = reporting.RunRequest
// BlockRun is a content-free receipt and lifecycle summary.
type BlockRun = reporting.RunView
// ArtifactPage is a current-authority-filtered catalog page.
type ArtifactPage = reporting.ArtifactList
// ArtifactRows preserves exact raw JSON cells and ordered column contracts.
type ArtifactRows = reporting.ResultPage
// ArtifactOutput is the exact retained deterministic or narrative payload.
type ArtifactOutput = reporting.RetainedOutput
// ArtifactNarrative carries grounded text, evidence and actual call receipts.
type ArtifactNarrative = reporting.NarrativeResult

// AdmitBlockRun reserves one stable key and seals a manifest without executing SQL.
func(c *Client)AdmitBlockRun(ctx context.Context,block string,in BlockRunRequest)(out BlockRun,err error){
 if !identity.Identifier(block) || !identity.Identifier(in.Key){return out,ErrInvalidCall}
 err=c.callLimit(ctx,"POST","/v1/block-runs","",struct{Block string `json:"block"`; Request BlockRunRequest `json:"request"`}{block,in},&out,4<<20)
 return out,err
}
// ExecuteBlockRun explicitly performs one attempt or resumes a failed one.
// Unknown outcomes are not automatically retried by this SDK.
func(c *Client)ExecuteBlockRun(ctx context.Context,id string,resume bool)(out BlockRun,err error){
 if !identity.Identifier(id){return out,ErrInvalidCall}
 err=c.callLimit(ctx,"POST","/v1/block-runs/execute","",struct{ID string `json:"id"`;Resume bool `json:"resume"`}{id,resume},&out,4<<20)
 return out,err
}
// InspectBlockRun reads the original actor/session execution receipt, without SQL.
func(c *Client)InspectBlockRun(ctx context.Context,id string)(out BlockRun,err error){
 if !identity.Identifier(id){return out,ErrInvalidCall}
 err=c.callLimit(ctx,"GET","/v1/block-runs/"+url.PathEscape(id),"",nil,&out,4<<20);return out,err
}
// CancelBlockRun records durable intent; it is not a native-termination receipt.
func(c *Client)CancelBlockRun(ctx context.Context,id string)(out BlockRun,err error){
 if !identity.Identifier(id){return out,ErrInvalidCall}
 err=c.callLimit(ctx,"POST","/v1/block-runs/"+url.PathEscape(id)+"/cancel","",struct{}{},&out,4<<20);return out,err
}
// ListArtifacts lists only currently authorized retained metadata.
func(c *Client)ListArtifacts(ctx context.Context,after string,limit int)(out ArtifactPage,err error){
 if after!="" && !identity.Identifier(after) || limit<1 || limit>100 {return out,ErrInvalidCall}
 q:=url.Values{"after":{after},"limit":{strconv.Itoa(limit)}}
 err=c.callLimit(ctx,"GET","/v1/artifacts?"+q.Encode(),"",nil,&out,16<<20);return out,err
}
// ReadArtifact reads a retained summary/tombstone, never an implicit fresh run.
func(c *Client)ReadArtifact(ctx context.Context,id string)(out BlockRun,err error){
 if !identity.Identifier(id){return out,ErrInvalidCall}
 err=c.callLimit(ctx,"GET","/v1/artifacts/"+url.PathEscape(id),"",nil,&out,4<<20);return out,err
}
// ReadArtifactRows pages original exact cells; the server also applies its configured bound.
func(c *Client)ReadArtifactRows(ctx context.Context,id string,offset,limit int)(out ArtifactRows,err error){
 if !identity.Identifier(id) || offset<0 || offset>10000 || limit<1 || limit>1000 {return out,ErrInvalidCall}
 q:=url.Values{"offset":{strconv.Itoa(offset)},"limit":{strconv.Itoa(limit)}}
 err=c.callLimit(ctx,"GET","/v1/artifacts/"+url.PathEscape(id)+"/result?"+q.Encode(),"",nil,&out,8<<20);return out,err
}
// ReadArtifactOutput retrieves saved text/specification without model or SQL work.
func(c *Client)ReadArtifactOutput(ctx context.Context,id,output string)(out ArtifactOutput,err error){
 if !identity.Identifier(id) || !identity.Identifier(output) {return out,ErrInvalidCall}
 q:=url.Values{"output":{output}}
 err=c.callLimit(ctx,"GET","/v1/artifacts/"+url.PathEscape(id)+"/output?"+q.Encode(),"",nil,&out,68<<20);return out,err
}
// ExpireArtifacts requires retention authority and never shortens a live artifact.
func(c *Client)ExpireArtifacts(ctx context.Context,limit int)(int64,error){
 if limit<1 || limit>1000{return 0,ErrInvalidCall}
 var out struct{Expired int64 `json:"expired"`}
 err:=c.call(ctx,"POST","/v1/artifact-retention","",struct{Limit int `json:"limit"`}{limit},&out)
 return out.Expired,err
}
''',
'sdk/chartworks/autopilot.go':r'''package chartworks

import (
 "context"
 "net/url"
 "github.com/hurtener/chartworks/internal/engineering"
 "github.com/hurtener/chartworks/internal/identity"
)

// EngineeringGoal requests scope-bounded, independently reviewed L2 planning.
type EngineeringGoal = engineering.AutopilotGoal
// EngineeringProposal is versioned evidence plus actual staged-effect receipts.
type EngineeringProposal = engineering.AutopilotProposal
// EngineeringReviewRequest addresses the exact digest and CAS version for review.
type EngineeringReviewRequest = engineering.AutopilotReviewRequest
// EngineeringEditRequest appends material, invalidating existing approval.
type EngineeringEditRequest = engineering.AutopilotEditRequest
// EngineeringApplyRequest addresses reviewed material, not caller-supplied approval.
type EngineeringApplyRequest = engineering.AutopilotApplyRequest
// EngineeringDrift is review evidence, not an applied definition change.
type EngineeringDrift = engineering.AutopilotDrift

// ProposeEngineering performs bounded planning, not publication or managed writes.
func(c *Client)ProposeEngineering(ctx context.Context,in EngineeringGoal)(out EngineeringProposal,err error){
 if !identity.Identifier(in.ID){return out,ErrInvalidCall}
 err=c.callLimit(ctx,"POST","/v1/engineering-proposals","",in,&out,2<<20);return out,err
}
// ReadEngineeringProposal requires current complete source/context reach.
func(c *Client)ReadEngineeringProposal(ctx context.Context,id string)(out EngineeringProposal,err error){
 if !identity.Identifier(id){return out,ErrInvalidCall}
 err=c.callLimit(ctx,"GET","/v1/engineering-proposals/"+url.PathEscape(id),"",nil,&out,2<<20);return out,err
}
// EditEngineeringProposal cannot transfer author identity or preserve approval.
func(c *Client)EditEngineeringProposal(ctx context.Context,id string,in EngineeringEditRequest)(out EngineeringProposal,err error){
 if !identity.Identifier(id) || in.ExpectedVersion<1{return out,ErrInvalidCall}
 err=c.callLimit(ctx,"PUT","/v1/engineering-proposals/"+url.PathEscape(id),"",in,&out,2<<20);return out,err
}
// ReviewEngineeringProposal derives reviewer identity from the current Pengui token.
func(c *Client)ReviewEngineeringProposal(ctx context.Context,id string,in EngineeringReviewRequest)(out EngineeringProposal,err error){
 if !identity.Identifier(id) || in.ExpectedVersion<1 || in.Revision<1{return out,ErrInvalidCall}
 err=c.callLimit(ctx,"POST","/v1/engineering-proposals/"+url.PathEscape(id)+"/review","",in,&out,2<<20);return out,err
}
// ApplyEngineeringProposal invokes ordinary managed-write gates after review.
func(c *Client)ApplyEngineeringProposal(ctx context.Context,id string,in EngineeringApplyRequest)(out EngineeringProposal,err error){
 if !identity.Identifier(id) || in.ExpectedVersion<1 || in.Revision<1{return out,ErrInvalidCall}
 err=c.callLimit(ctx,"POST","/v1/engineering-proposals/"+url.PathEscape(id)+"/apply","",in,&out,2<<20);return out,err
}
// CompensateEngineeringProposal quarantines eligible owned outputs; it returns
// dependency/ownership conflicts rather than claiming global warehouse rollback.
func(c *Client)CompensateEngineeringProposal(ctx context.Context,id string,in EngineeringApplyRequest)(out EngineeringProposal,err error){
 if !identity.Identifier(id) || in.ExpectedVersion<1 || in.Revision<1{return out,ErrInvalidCall}
 err=c.callLimit(ctx,"POST","/v1/engineering-proposals/"+url.PathEscape(id)+"/compensate","",in,&out,2<<20);return out,err
}
// DetectEngineeringDrift creates review evidence without changing published meaning.
func(c *Client)DetectEngineeringDrift(ctx context.Context,id string)(out EngineeringDrift,err error){
 if !identity.Identifier(id){return out,ErrInvalidCall}
 err=c.callLimit(ctx,"POST","/v1/engineering-proposals/"+url.PathEscape(id)+"/drift","",struct{}{},&out,2<<20);return out,err
}
''',
}
for name,content in files.items():
    p=Path(name)
    assert not p.exists(),name
    p.write_text(content)
