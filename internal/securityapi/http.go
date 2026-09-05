package securityapi

import(
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strconv"

 "github.com/hurtener/chartworks/internal/access"
 "github.com/hurtener/chartworks/internal/auth"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/store"
 "github.com/hurtener/chartworks/internal/telemetry"
)
// Operation is the single actual-route enforcement/audit registration, not a grants catalog.
type Operation struct{Method string `json:"method"`;Path string `json:"path"`;Action string `json:"action"`;Permission string `json:"permission"`;Mutation bool `json:"mutation"`}
// Operations returns a detached registry of implemented endpoints only.
func Operations()[]Operation{return []Operation{
 {"GET","/v1/retention-policy","ops.read","read",false},
 {"PUT","/v1/retention-policy","ops.write","write",true},
 {"GET","/v1/audit-events","ops.audit","read",false},
 {"POST","/v1/retention-sweeps","ops.maintain","erase",true},
 {"GET","/v1/access/diagnostics","ops.inspect","read",false},
 {"GET","/metrics","ops.metrics","read",false},
}}
// Handler composes verifier, central scope registry and the real service.
func Handler(v *auth.Verifier,s *Service,r *telemetry.Reporter,metrics bool)http.Handler{
 return v.Middleware(auth.HTTP,http.HandlerFunc(func(w http.ResponseWriter,req *http.Request){
  w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff")
  e,err:=identity.FromContext(req.Context());if err!=nil{failure(w,access.ErrUnauthenticated);return}
  var op Operation;found:=false
  for _,candidate:=range Operations(){if candidate.Path==req.URL.Path&&candidate.Method==req.Method{op=candidate;found=true;break}}
  if !found||(!metrics&&op.Path=="/metrics"){failure(w,access.ErrNotFound);return}
  if err=access.Require(e,op.Action,access.Tenant(e,op.Permission));err!=nil{_ = r.Record(req.Context(),telemetry.Request,false);failure(w,err);return}
  // Unknown query fields cannot become authority or silently alter a protected operation.
  query,err:=parseQuery(req,op.Path);if err!=nil{failure(w,store.ErrInvalid);return}
  if req.Method=="GET"&&(req.ContentLength!=0||len(req.TransferEncoding)!=0){failure(w,store.ErrInvalid);return}
  _ = r.Record(req.Context(),telemetry.Request,true)
  switch op.Path{
  case "/metrics":r.Handler().ServeHTTP(w,req)
  case "/v1/access/diagnostics":
   type check struct{Operation Operation `json:"operation"`;Allowed bool `json:"allowed"`}
   checks:=[]check{};for _,item:=range Operations(){if !metrics&&item.Path=="/metrics"{continue};checks=append(checks,check{item,access.Require(e,item.Action,access.Tenant(e,item.Permission))==nil})}
   respond(w,checks)
  case "/v1/retention-policy":
   if req.Method=="GET"{p,err:=s.Policy(req.Context(),e);if err!=nil{failure(w,err);return};respond(w,p);return}
   var body struct{Expected int64 `json:"expected_revision"`;AuditDays int `json:"audit_days"`;OperationHours int `json:"operation_hours"`}
   if !bodyDecode(w,req,&body,"expected_revision","audit_days","operation_hours"){return}
   p,err:=s.Configure(req.Context(),e,body.Expected,store.Policy{AuditDays:body.AuditDays,OperationHours:body.OperationHours});if err!=nil{failure(w,err);return};respond(w,p)
  case "/v1/audit-events":
   limit:=100;if raw:=query;raw!=""{limit,err=strconv.Atoi(raw);if err!=nil||limit<1||limit>1000{failure(w,store.ErrInvalid);return}}
   a,err:=s.Audits(req.Context(),e,limit);if err!=nil{failure(w,err);return};if a==nil{a=[]store.Audit{}};respond(w,a)
  case "/v1/retention-sweeps":
   if len(req.Header.Values("Idempotency-Key"))!=1||!store.Identifier(req.Header.Get("Idempotency-Key")){failure(w,store.ErrInvalid);return}
   var empty struct{};if !bodyDecode(w,req,&empty){return}
   result,err:=s.Sweep(req.Context(),e,req.Header.Get("Idempotency-Key"));if err!=nil{failure(w,err);return};respond(w,result)
  }
 }))
}
func parseQuery(req *http.Request,path string)(string,error){
 q,err:=req.URL.Query(),error(nil)
 if req.URL.RawQuery!="" {q,err = parseRawQuery(req.URL.RawQuery)}
 if err!=nil{return "",store.ErrInvalid}
 for k,v:=range q{if path!="/v1/audit-events"||k!="limit"||len(v)!=1{return "",store.ErrInvalid}}
 return q.Get("limit"),nil
}
func bodyDecode(w http.ResponseWriter,r *http.Request,out any,fields ...string)bool{
 if r.Header.Get("Content-Type")!="application/json"{failure(w,store.ErrInvalid);return false}
 b,err:=io.ReadAll(io.LimitReader(r.Body,4097));if err!=nil||len(b)>4096{failure(w,store.ErrInvalid);return false}
 m,err:=auth.Object(b,4096);if err!=nil||len(m)!=len(fields){failure(w,store.ErrInvalid);return false}
 for _,field:=range fields{if _,ok:=m[field];!ok{failure(w,store.ErrInvalid);return false}}
 if json.Unmarshal(b,out)!=nil{failure(w,store.ErrInvalid);return false};return true
}
func respond(w http.ResponseWriter,v any){w.Header().Set("Content-Type","application/json");_ = json.NewEncoder(w).Encode(v)}
func failure(w http.ResponseWriter,err error){
 status:=http.StatusInternalServerError;code:="internal_error"
 switch{
 case errors.Is(err,access.ErrUnauthenticated):status=http.StatusUnauthorized;code="unauthorized"
 case errors.Is(err,access.ErrForbidden):status=http.StatusForbidden;code="forbidden"
 case errors.Is(err,access.ErrNotFound)||errors.Is(err,store.ErrNotFound):status=http.StatusNotFound;code="not_found"
 case errors.Is(err,store.ErrInvalid)||errors.Is(err,store.ErrScope):status=http.StatusUnprocessableEntity;code="invalid_request"
 case errors.Is(err,store.ErrConflict):status=http.StatusConflict;code="conflict"
 case errors.Is(err,store.ErrExpired):status=http.StatusGone;code="expired"
 case errors.Is(err,store.ErrUnavailable):status=http.StatusServiceUnavailable;code="unavailable"
 }
 w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(map[string]string{"error":code})
}
