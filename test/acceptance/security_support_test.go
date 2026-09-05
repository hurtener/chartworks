package acceptance

import(
 "bytes"
 "context"
 "crypto/ecdsa"
 "crypto/elliptic"
 "crypto/rand"
 "encoding/base64"
 "encoding/json"
 "io"
 "net/http"
 "net/http/httptest"
 "sort"
 "sync"
 "sync/atomic"
 "testing"
 "time"

 "github.com/golang-jwt/jwt/v5"
 "github.com/hurtener/chartworks/internal/auth"
 "github.com/hurtener/chartworks/internal/config"
 "github.com/hurtener/chartworks/internal/identity"
)

type tokenFixture struct{
 cfg config.Auth
 verifier *auth.Verifier
 server *httptest.Server
 private *ecdsa.PrivateKey
 clock atomic.Int64
 requests atomic.Int64
 mu sync.Mutex
 doc []byte
 status int
}
func publicDocument(t testing.TB,key *ecdsa.PrivateKey,kid string)[]byte{
 t.Helper();b,err:=json.Marshal(map[string]any{"keys":[]any{map[string]any{"kty":"EC","use":"sig","alg":"ES256","kid":kid,"crv":"P-256","x":base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte,32))),"y":base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte,32)))}}});if err!=nil{t.Fatal(err)};return b
}
func newTokenFixture(t testing.TB)*tokenFixture{
 t.Helper();key,err:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader);if err!=nil{t.Fatal(err)}
 f:=&tokenFixture{private:key,status:200};f.clock.Store(time.Now().Unix());f.doc=publicDocument(t,key,"test-key")
 f.server=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  f.requests.Add(1);f.mu.Lock();defer f.mu.Unlock()
  if f.status==302{w.Header().Set("Location",f.server.URL+"/redirect-target")};w.WriteHeader(f.status);_,_=w.Write(f.doc)
 }))
 t.Cleanup(f.server.Close)
 f.cfg=config.Defaults().Auth;f.cfg.Issuer=f.server.URL+"/issuer";f.cfg.JWKSURL=f.server.URL+"/keys";f.cfg.Audience="";f.cfg.Audiences=config.Audiences{HTTP:"chartworks:http",MCP:"chartworks:mcp"};f.cfg.RefreshInterval=config.Duration(time.Second);f.cfg.JWKSMaxStale=config.Duration(10*time.Second)
 f.verifier,err=auth.New(f.cfg,f.server.Client(),func()time.Time{return time.Unix(f.clock.Load(),0)});if err!=nil{t.Fatal(err)};t.Cleanup(f.verifier.Close);return f
}
func(f *tokenFixture) set(doc []byte,status int){f.mu.Lock();defer f.mu.Unlock();f.doc=append([]byte(nil),doc...);f.status=status}
func(f *tokenFixture) claims(tenant,user string,scopes []string)jwt.MapClaims{
 now:=f.clock.Load();ordered:=append([]string{},scopes...);sort.Strings(ordered)
 return jwt.MapClaims{"iss":f.cfg.Issuer,"aud":f.cfg.HTTPAudience(),"sub":user,"tenant":tenant,"user":user,"session":"test-session","iat":now,"nbf":now-60,"exp":now+300,"scopes":ordered,"runtime_id":"synthetic-runtime"}
}
func(f *tokenFixture) sign(t testing.TB,claims jwt.MapClaims,header map[string]any)string{
 t.Helper();token:=jwt.NewWithClaims(jwt.SigningMethodES256,claims);token.Header["kid"]="test-key";for k,v:=range header{token.Header[k]=v};s,err:=token.SignedString(f.private);if err!=nil{t.Fatal(err)};return s
}
func(f *tokenFixture) envelope(t testing.TB,tenant,user string,scopes ...string)identity.Envelope{
 t.Helper();s:=f.sign(t,f.claims(tenant,user,scopes),nil);e,err:=f.verifier.Verify(context.Background(),s,auth.HTTP);if err!=nil{t.Fatal(err)};return e
}
func callProtected(t testing.TB,h http.Handler,method,path,token,body string,extra map[string]string)*httptest.ResponseRecorder{
 t.Helper();req:=httptest.NewRequest(method,path,bytes.NewBufferString(body));if token!=""{req.Header.Set("Authorization","Bearer "+token)};if method!="GET"{req.Header.Set("Content-Type","application/json")};for k,v:=range extra{req.Header.Set(k,v)};w:=httptest.NewRecorder();h.ServeHTTP(w,req);return w
}
func bodyText(t testing.TB,r *http.Response)string{t.Helper();defer func(){_ = r.Body.Close()}();b,e:=io.ReadAll(r.Body);if e!=nil{t.Fatal(e)};return string(b)}
