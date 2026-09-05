package auth

import (
 "context"
 "encoding/base64"
 "encoding/json"
 "errors"
 "net/http"
 "strconv"
 "strings"
 "time"

 "github.com/golang-jwt/jwt/v5"
 "github.com/hurtener/chartworks/internal/config"
 "github.com/hurtener/chartworks/internal/identity"
)

var(
 // ErrToken never wraps library errors or bearer content.
 ErrToken=errors.New("auth: invalid bearer")
 // ErrKeys reports unavailable trusted verification material without a key ID.
 ErrKeys=errors.New("auth: verification material unavailable")
)
// Surface selects a configured audience; it is not an arbitrary caller audience string.
type Surface uint8
const(
 // HTTP is the protected API audience.
 HTTP Surface=iota+1
 // MCP is the provider audience for the existing platform tool bridge.
 MCP
)
// Verifier is immutable apart from its concurrency-safe public-key cache.
type Verifier struct{cfg config.Auth;keys *KeyProbe;now func()time.Time}
// New pins validated configuration. The client/clock are trusted transport/test seams.
func New(cfg config.Auth,client *http.Client,now func()time.Time)(*Verifier,error){
 if err:=config.ValidateAuth(cfg);err!=nil{return nil,err}
 cfg.Algorithms=append([]string(nil),cfg.Algorithms...)
 if now==nil{now=time.Now}
 keys:=NewKeyProbe(cfg,client);keys.now=now
 return &Verifier{cfg:cfg,keys:keys,now:now},nil
}
// Check is the same cache/freshness observation used by actual token verification.
func(v *Verifier) Check(ctx context.Context)Dependency{return v.keys.Check(ctx)}
// Close releases the public-key transport; the owner joins requests first.
func(v *Verifier) Close(){v.keys.Close()}

func text(m map[string]json.RawMessage,name string)(string,bool){var s string;r,ok:=m[name];return s,ok&&json.Unmarshal(r,&s)==nil}
func stringValue(m map[string]json.RawMessage,name string)(string,bool){r,ok:=m[name];var s string;if !ok||json.Unmarshal(r,&s)!=nil{return "",false};return s,true}
func number(m map[string]json.RawMessage,name string)(int64,bool){r,ok:=m[name];if !ok{return 0,false};s:=string(r);n,e:=strconv.ParseInt(s,10,64);return n,e==nil&&n>0&&n<=9999999999}

// Verify accepts only Pengui's actual tenant/user/session/scopes bearer shape.
// Registration envelopes with audience/tenant_id or a singular scope are not bearers.
func(v *Verifier) Verify(ctx context.Context,token string,surface Surface)(identity.Envelope,error){
 fail:=func()(identity.Envelope,error){return identity.Envelope{},ErrToken}
 if ctx.Err()!=nil||len(token)==0||len(token)>v.cfg.MaxTokenBytes||strings.Count(token,".")!=2{return fail()}
 parts:=strings.Split(token,".");enc:=base64.RawURLEncoding.Strict()
 header,err:=enc.DecodeString(parts[0]);if err!=nil{return fail()}
 h,err:=Object(header,2048);if err!=nil{return fail()}
 for k:=range h {if k!="alg"&&k!="kid"&&k!="typ"{return fail()}}
 alg,ok:=stringValue(h,"alg");if !ok{return fail()};allowed:=false;for _,a:=range v.cfg.Algorithms{if a==alg{allowed=true}};if !allowed{return fail()}
 kid,ok:=stringValue(h,"kid");if !ok||!identity.Identifier(kid){return fail()}
 if _,exists:=h["typ"];exists{typ,valid:=stringValue(h,"typ");if !valid||typ!="JWT"{return fail()}}
 body,err:=enc.DecodeString(parts[1]);if err!=nil{return fail()};claims,err:=Object(body,v.cfg.MaxClaimBytes);if err!=nil{return fail()}
 for _,alias:=range []string{"scope","tenant_id","user_id","session_id","audience"}{if _,exists:=claims[alias];exists{return fail()}}
 tenant,tok:=stringValue(claims,"tenant");user,uok:=stringValue(claims,"user");session,sok:=stringValue(claims,"session")
 if !tok||!uok||!sok||!identity.Identifier(tenant)||!identity.Identifier(user)||!identity.Identifier(session){return fail()}
 if _,exists:=claims["sub"];exists{sub,valid:=stringValue(claims,"sub");if !valid||sub!=user{return fail()}}
 issued,iok:=number(claims,"iat");expires,eok:=number(claims,"exp")
 if !iok||!eok||expires<=issued||expires-issued>int64(time.Duration(v.cfg.MaxTokenLifetime)/time.Second){return fail()}
 if _,exists:=claims["nbf"];exists{nbf,valid:=number(claims,"nbf");if !valid||nbf>=expires{return fail()}}
 var scopes []string
 raw,present:=claims["scopes"];if !present||json.Unmarshal(raw,&scopes)!=nil||scopes==nil||len(scopes)>v.cfg.MaxScopes{return fail()}
 total:=0;for _,s:=range scopes{total+=len(s)};if total>v.cfg.MaxScopeBytes{return fail()}
 var audiences []string
 raw,present=claims["aud"];if !present{return fail()}
 var audience string
 if json.Unmarshal(raw,&audience)==nil{audiences=[]string{audience}}else if json.Unmarshal(raw,&audiences)!=nil{return fail()}
 if len(audiences)<1||len(audiences)>2{return fail()};seen:=map[string]bool{}
 for _,a:=range audiences{if len(a)==0||len(a)>2048||strings.TrimSpace(a)!=a||seen[a]{return fail()};seen[a]=true}
 expected:=""
 switch surface{case HTTP:expected=v.cfg.HTTPAudience();case MCP:expected=v.cfg.MCPAudience();default:return fail()}
 _,err=jwt.Parse(token,func(t *jwt.Token)(any,error){if t.Method.Alg()!=alg{return nil,ErrToken};return v.keys.key(ctx,kid,alg)},jwt.WithValidMethods(v.cfg.Algorithms),jwt.WithIssuer(v.cfg.Issuer),jwt.WithAudience(expected),jwt.WithExpirationRequired(),jwt.WithIssuedAt(),jwt.WithJSONNumber(),jwt.WithStrictDecoding(),jwt.WithLeeway(time.Duration(v.cfg.ClockSkew)),jwt.WithTimeFunc(v.now))
 if err!=nil||ctx.Err()!=nil{return fail()}
 env,err:=identity.FromVerified(tenant,user,session,scopes,time.Unix(expires,0).Add(time.Duration(v.cfg.ClockSkew)),v.now)
 if err!=nil{return fail()};return env,nil
}

// Bearer accepts exactly one Authorization header, never a cookie/query/body token.
func Bearer(r *http.Request)(string,error){
 values:=r.Header.Values("Authorization");if len(values)!=1{return "",ErrToken}
 scheme,value,ok:=strings.Cut(values[0]," ")
 if !ok||!strings.EqualFold(scheme,"Bearer")||value==""||strings.ContainsAny(value," \t\r\n,"){return "",ErrToken}
 return value,nil
}
// Middleware installs the verified immutable context and bounds work by token expiry.
func(v *Verifier) Middleware(surface Surface,next http.Handler)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  token,err:=Bearer(r);var e identity.Envelope
  if err==nil{e,err=v.Verify(r.Context(),token,surface)}
  if err!=nil{w.Header().Set("WWW-Authenticate",`Bearer realm="chartworks"`);w.Header().Set("Cache-Control","no-store");w.Header().Set("Content-Type","application/json");w.WriteHeader(http.StatusUnauthorized);_,_=w.Write([]byte(`{"error":"unauthorized"}`));return}
  ctx,err:=e.Context(r.Context());if err!=nil{w.WriteHeader(http.StatusUnauthorized);return}
  ctx,cancel:=context.WithDeadline(ctx,e.Deadline());defer cancel()
  next.ServeHTTP(w,r.WithContext(ctx))
 })
}
