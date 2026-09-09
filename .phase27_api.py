# Development helper only. Outputs are ordinary reviewed source files; remove
# this helper before the final read-only verification commit.
from pathlib import Path
import re

# method, suffix, action, SDK method, domain method, request, response, summary, feature
operations = [
 ('POST','/{id}/parameters/assist','Write','ParameterizeBlock','Parameterize','ParameterizeRequest','View','Append an AST-verified typed period amendment without publication','observe'),
 ('POST','/{id}/impact','Read','RecheckBlockImpact','RecheckImpact','ImpactRequest','Impact','Explicitly observe dependency impact without altering definitions','observe'),
 ('POST','/{id}/impact/apply','Write','ApplyBlockImpact','ApplyImpact','ApplyImpactRequest','View','Create a private draft for an exact current dependency proposal','observe'),
 ('POST','', 'Write','CreateBlock','Create','CreateRequest','View','Create an unvalidated private block draft',''),
 ('GET','', 'Read','ListBlocks','List','ListRequest','Page','List only currently authorized block definitions',''),
 ('POST','/capture','Write','CaptureBlock','CaptureQuery','CaptureRequest','View','Capture a completed authorized query as an unvalidated draft','capture'),
 ('POST','/questions/assess','Read','AssessBlockQuestions','AssessQuestions','QuestionRequest','Assessment','Assess authorized localized questions with bounded lexical matching',''),
 ('GET','/{id}','Read','ReadBlock','Read','Reference','View','Read a SQL-private published or authorized exact revision',''),
 ('GET','/{id}/sql','SQLRead','ReadBlockSQL','SQL','Reference','SQLView','Read SQL through separately scoped inspection authority',''),
 ('GET','/{id}/history','Read','BlockHistory','History',None,'History','Read permission-filtered immutable lifecycle history',''),
 ('PUT','/{id}','Write','EditBlock','Edit','EditRequest','View','Append a CAS-fenced private amendment without publication',''),
 ('POST','/{id}/validate','Validate','ValidateBlock','Validate','ValidateRequest','ValidationResult','Validate an exact revision through the existing bounded read core','validate'),
 ('POST','/{id}/preview','Preview','PreviewBlock','Preview','PreviewRequest','PreviewResult','Preview exact outputs privately without retaining artifacts','validate'),
 ('POST','/{id}/publish','Publish','PublishBlock','Publish','PublishRequest','State','Publish one exact revision against fresh validation evidence',''),
 ('POST','/{id}/certify','Certify','CertifyBlock','Certify','CertifyRequest','Attestation','Attest to one exact published revision and evidence receipt',''),
 ('POST','/{id}/withdraw','Certify','WithdrawBlockCertification','Withdraw','WithdrawRequest','Withdrawal','Withdraw an attestation while preserving its history',''),
 ('POST','/{id}/reject','Write','RejectBlock','Reject','TransitionRequest','State','Reject the current private draft without removing history',''),
 ('POST','/{id}/restore','Write','RestoreBlock','Restore','RestoreRequest','View','Copy an authorized revision into a new unvalidated draft',''),
 ('POST','/{id}/archive','Write','ArchiveBlock','Archive','TransitionRequest','State','Archive the default publication while retaining exact revisions',''),
 ('POST','/{id}/parameters/resolve','Read','ResolveBlockParameters','Resolve','ResolveRequest','ResolutionResult','Resolve typed defaults and logical period windows without execution',''),
]

imports = '''// Package reportingapi registers thin consumers of governed block authoring.
package reportingapi
import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "mime"
 "net/http"
 "net/url"
 "reflect"
 "strconv"
 "strings"
 "github.com/hurtener/chartworks/internal/access"
 "github.com/hurtener/chartworks/internal/api"
 "github.com/hurtener/chartworks/internal/auth"
 "github.com/hurtener/chartworks/internal/exec"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/nlqexec"
 "github.com/hurtener/chartworks/internal/reporting"
 "github.com/hurtener/chartworks/internal/store"
)
const MaxBodyBytes = 2 << 20
'''
rows=[]
for method,path,action,sdk,domain,inp,out,summary,feature in operations:
    input_type = 'nil' if method=='GET' else f'reflect.TypeFor[reporting.{inp}]()'
    rows.append(f'{{"{method}","/v1/blocks{path}",reporting.{action},"{sdk[0].lower()+sdk[1:]}","{summary}",{input_type},reflect.TypeFor[reporting.{out}](),"{feature}"}},')
registry='''
// Registry advertises only handlers backed by the supplied common services.
func Registry(validation, capture, observe bool) (*api.Registry,error) {
 entries := []struct{method,path string; action reporting.Access; id,summary string; in,out reflect.Type; feature string}{
'''+ '\n'.join(rows)+'''
 }
 definitions:=make([]api.Definition,0,len(entries))
 for _,entry:=range entries {
  if entry.feature=="validate" && !validation || entry.feature=="capture" && !capture || entry.feature=="observe" && !observe {continue}
  response,err:=api.SchemaFor(entry.id+"Response",entry.out,true);if err!=nil{return nil,err}
  d:=api.Definition{Operation:api.Operation{Method:entry.method,Path:entry.path,Action:entry.action.Action(),Effect:"governed_block_metadata"},ID:entry.id,Summary:entry.summary,ResourceLoader:"reporting.Service and PostgreSQL tenant/revision/parent/private eligibility",Audit:"block lifecycle and common read-attempt journal; no SQL in audit",Response:response,Replay:"never",Errors:[]api.ErrorResponse{
   {Status:400,Code:"invalid_request"},{Status:401,Code:"unauthenticated"},{Status:401,Code:"unauthorized"},{Status:403,Code:"forbidden"},{Status:404,Code:"not_found"},{Status:409,Code:"conflict"},{Status:409,Code:"stale_validation"},{Status:413,Code:"limit_exceeded"},{Status:422,Code:"invalid_query"},{Status:429,Code:"busy"},{Status:503,Code:"unavailable"},{Status:504,Code:"cancelled_or_timed_out"}}}
  if entry.in!=nil { d.MaxBodyBytes=MaxBodyBytes;d.Request,err=api.SchemaFor(entry.id+"Request",entry.in,false,api.NullableCollections);if err!=nil{return nil,err} }
  if entry.method=="GET" {d.Replay="read";d.Effect="authorized_metadata_read";d.Audit="read_only_no_domain_audit"}
  if entry.id=="listBlocks" {d.Query=[]api.Parameter{{Name:"after",In:"query",Type:"string",Max:128},{Name:"limit",In:"query",Type:"integer",Min:1,Max:100},{Name:"include_drafts",In:"query",Type:"boolean"}}}
  if entry.id=="readBlock" || entry.id=="readBlockSQL" {d.Query=[]api.Parameter{{Name:"revision",In:"query",Type:"integer",Min:1,Max:256},{Name:"draft",In:"query",Type:"boolean"}}}
  if entry.id=="previewBlock" || entry.id=="validateBlock" {d.Effect="explicit_bounded_source_read_private_evidence"}
  if entry.id=="resolveBlockParameters" || entry.id=="assessBlockQuestions" {d.Effect="authorized_metadata_read";d.Audit="read_only_no_domain_audit"}
  definitions=append(definitions,d)
 }
 return api.New(definitions)
}
'''
handler='''
// Handler verifies action authority before request bodies and delegates every
// addressed-resource and mutation check to the common service, not a router role.
func Handler(verifier *auth.Verifier, service *reporting.Service, next http.Handler) http.Handler {
 if verifier==nil || next==nil{return http.NotFoundHandler()};if service==nil{return next}
 registry,err:=Registry(service.CanValidate(),service.CanCapture(),service.CanObserve());if err!=nil{return http.HandlerFunc(func(w http.ResponseWriter,_ *http.Request){failure(w,err)})}
 requests:=make(chan struct{},service.Limits().MaxConcurrent*4)
 protected:=verifier.Middleware(auth.HTTP,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  headers(w)
  selected,id,_:=registry.Match(r.Method,r.URL.Path)
  if selected.Path=="" {w.WriteHeader(http.StatusMethodNotAllowed);return}
  e,err:=identity.FromContext(r.Context());if err!=nil{failure(w,access.ErrUnauthenticated);return}
  if err=access.Require(e,selected.Action);err!=nil{failure(w,err);return}
  if id!="" { action:=reporting.Access(strings.TrimPrefix(selected.Action,"reporting."));if err=reporting.Require(e,id,action);err!=nil{failure(w,err);return} }
  select{case requests<-struct{}{}:defer func(){<-requests}();default:failure(w,reporting.ErrBusy);return}
  if r.URL.RawPath!="" || r.Header.Get("Content-Encoding")!="" || r.Header.Get("Idempotency-Key")!="" {failure(w,reporting.ErrInvalid);return}
  parsedQuery,queryErr:=url.ParseQuery(r.URL.RawQuery)
  if queryErr!=nil || !validQuery(parsedQuery,selected.Query){failure(w,reporting.ErrInvalid);return}
  if r.Method=="GET" {body,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,1));if err!=nil || len(body)>0{failure(w,reporting.ErrInvalid);return}}
  var out any
  switch selected.ID {
'''
for method,path,action,sdk,domain,inp,out,summary,feature in operations:
    ident=sdk[0].lower()+sdk[1:]
    handler+=f'case "{ident}":\n'
    args='r.Context(),e'+(',id' if '{id}' in path else '')
    if method=='GET':
        if inp=='Reference':
            handler+=' var in reporting.Reference;in,err=reference(r.URL.Query())\n'
            handler+=f' if err==nil {{out,err=service.{domain}({args},in)}}\n'
        elif inp=='ListRequest':
            handler+=' var in reporting.ListRequest;in,err=listRequest(r.URL.Query())\n'
            handler+=f' if err==nil {{out,err=service.{domain}({args},in)}}\n'
        else: handler+=f' out,err=service.{domain}({args})\n'
    else:
        handler+=f' var in reporting.{inp};err=decode(w,r,selected,&in);if err==nil{{out,err=service.{domain}({args},in)}}\n'
handler+='''
  default:err=reporting.ErrInvalid
  }
  if err!=nil{failure(w,err);return}
  if err=r.Context().Err();err!=nil{failure(w,err);return}
  raw,err:=json.Marshal(out);if err!=nil{failure(w,err);return}
  if len(raw)>16<<20{failure(w,exec.ErrLimit);return}
  _,_=w.Write(append(raw,'\\n'))
 }))
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  _,_,found:=registry.Match(r.Method,r.URL.Path);if !found{next.ServeHTTP(w,r);return};protected.ServeHTTP(w,r)
 })
}
func validQuery(q url.Values, declared []api.Parameter)bool{
 if len(q)>len(declared){return false};for key,values:=range q{if len(values)!=1{return false};found:=false;for _,p:=range declared{if key==p.Name{found=true}};if !found{return false}};return true
}
func reference(q url.Values)(reporting.Reference,error){
 var out reporting.Reference
 if v,ok:=q["revision"];ok{n,err:=strconv.ParseInt(v[0],10,64);if err!=nil || n<1 || n>256 || strconv.FormatInt(n,10)!=v[0]{return out,reporting.ErrInvalid};out.Revision=n}
 if v,ok:=q["draft"];ok{if v[0]!="true" && v[0]!="false"{return out,reporting.ErrInvalid};out.Draft=v[0]=="true"}
 if out.Draft && out.Revision!=0{return out,reporting.ErrInvalid};return out,nil
}
func listRequest(q url.Values)(reporting.ListRequest,error){
 out:=reporting.ListRequest{After:q.Get("after"),Limit:20}
 if v,ok:=q["limit"];ok{n,err:=strconv.Atoi(v[0]);if err!=nil || n<1 || n>100 || strconv.Itoa(n)!=v[0]{return out,reporting.ErrInvalid};out.Limit=n}
 if v,ok:=q["include_drafts"];ok{if v[0]!="true" && v[0]!="false"{return out,reporting.ErrInvalid};out.IncludeDrafts=v[0]=="true"}
 return out,nil
}
func decode(w http.ResponseWriter,r *http.Request,d api.Definition,out any)error{
 media,params,err:=mime.ParseMediaType(r.Header.Get("Content-Type"));if err!=nil || media!="application/json" || len(params)!=0{return reporting.ErrInvalid}
 raw,err:=io.ReadAll(http.MaxBytesReader(w,r.Body,int64(d.MaxBodyBytes)));if err!=nil{var limit *http.MaxBytesError;if errors.As(err,&limit){return exec.ErrLimit};return reporting.ErrInvalid}
 if d.Request==nil || d.Request.Validate(raw,d.MaxBodyBytes)!=nil || json.Unmarshal(raw,out)!=nil{return reporting.ErrInvalid};return nil
}
func headers(w http.ResponseWriter){w.Header().Set("Content-Type","application/json");w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("Referrer-Policy","no-referrer")}
func failure(w http.ResponseWriter,err error){
 status,code:=503,"unavailable"
 switch{
 case errors.Is(err,access.ErrUnauthenticated):status,code=401,"unauthenticated"
 case errors.Is(err,access.ErrForbidden),errors.Is(err,nlqexec.ErrInspectionRequired):status,code=403,"forbidden"
 case errors.Is(err,access.ErrNotFound),errors.Is(err,store.ErrNotFound),errors.Is(err,nlqexec.ErrForeignSession):status,code=404,"not_found"
 case errors.Is(err,reporting.ErrInvalid),errors.Is(err,store.ErrInvalid),errors.Is(err,nlqexec.ErrInvalid):status,code=400,"invalid_request"
 case errors.Is(err,store.ErrConflict),errors.Is(err,nlqexec.ErrNoPlan):status,code=409,"conflict"
 case errors.Is(err,reporting.ErrStale),errors.Is(err,exec.ErrBinding):status,code=409,"stale_validation"
 case errors.Is(err,exec.ErrLimit):status,code=413,"limit_exceeded"
 case errors.Is(err,exec.ErrQuery):status,code=422,"invalid_query"
 case errors.Is(err,reporting.ErrBusy):status,code=429,"busy"
 case errors.Is(err,context.Canceled),errors.Is(err,context.DeadlineExceeded):status,code=504,"cancelled_or_timed_out"
 }
 headers(w);w.WriteHeader(status);_=json.NewEncoder(w).Encode(struct{Error string `json:"error"`}{code})
}
'''
Path('internal/reportingapi').mkdir(exist_ok=True)
Path('internal/reportingapi/http.go').write_text(imports+registry+handler)

sdk='''package chartworks
import (
 "context"
 "errors"
 "net/url"
 "strconv"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/reporting"
)
// ErrBlockRequest rejects a malformed coordinate before any network call.
var ErrBlockRequest=errors.New("chartworks: invalid block coordinate")
'''
aliases=['ParameterizeRequest','Rename','ImpactRequest','Impact','ApplyImpactRequest','Localized','TopicPin','TemplatePin','DimensionReference','Parameter','Value','Argument','Period','Window','Resolution','BoundValue','Resolved','Narrative','Output','Definition','Reference','State','Evidence','Attestation','Withdrawal','Health','Trust','View','SQLView','CreateRequest','EditRequest','TransitionRequest','RestoreRequest','PublishRequest','CertifyRequest','WithdrawRequest','ValidateRequest','PreviewRequest','ValidationResult','PreviewResult','CaptureRequest','ListRequest','Summary','Page','QuestionRequest','QuestionMatch','Assessment','Event','History','ResolveRequest','ResolutionResult','Provenance']
for name in aliases:
    sdk+=f'// Block{name} mirrors the common governed block wire contract.\ntype Block{name}=reporting.{name}\n'
for method,path,action,name,domain,inp,out,summary,feature in operations:
    params='ctx context.Context'+(', id string' if '{id}' in path else '')
    if inp:params+=f', in Block{inp}'
    sdk+=f'\n// {name} {summary[0].lower()+summary[1:]}. Mutations are never automatically replayed.\nfunc(c *Client){name}({params})(out Block{out},err error){{\n'
    if '{id}' in path:
        sdk+='if !identity.Identifier(id){return out,ErrBlockRequest}\n'
        sdk+=f'path:="/v1/blocks/"+id+"{path.split("{id}")[1]}"\n'
    else:sdk+=f'path:="/v1/blocks{path}"\n'
    if method=='GET' and inp=='Reference':
        sdk+='if in.Revision<0 || in.Revision>256 || in.Draft && in.Revision!=0{return out,ErrBlockRequest};q:=url.Values{};if in.Revision>0{q.Set("revision",strconv.FormatInt(in.Revision,10))};if in.Draft{q.Set("draft","true")};if len(q)>0{path+="?"+q.Encode()}\n'
    if method=='GET' and inp=='ListRequest':
        sdk+='if in.Limit==0{in.Limit=20};if in.Limit<1 || in.Limit>100 || in.After!="" && !identity.Identifier(in.After){return out,ErrBlockRequest};q:=url.Values{"limit":[]string{strconv.Itoa(in.Limit)}};if in.After!=""{q.Set("after",in.After)};if in.IncludeDrafts{q.Set("include_drafts","true")};path+="?"+q.Encode()\n'
    arg='nil' if method=='GET' else 'in'
    sdk+=f'err=c.callLimit(ctx,"{method}",path,"",{arg},&out,16<<20);return\n}}\n'
Path('sdk/chartworks/blocks.go').write_text(sdk)
