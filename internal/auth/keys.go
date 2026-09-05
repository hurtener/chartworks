package auth

import (
 "context"
 "crypto/ecdsa"
 "crypto/elliptic"
 "crypto/rsa"
 "encoding/json"
 "errors"
 "io"
 "math/big"
 "net/http"
 "sync"
 "time"

 "github.com/hurtener/chartworks/internal/config"
)

// Dependency describes verification material freshness, not token authorization.
type Dependency struct{Ready bool;ValidUntil time.Time}
type publicKey struct{algorithm string;key any}

// KeyProbe is the one bounded public-key cache used by readiness and token verification.
// No per-kid negative cache or unbounded request-triggered goroutines are created.
type KeyProbe struct{
 mu sync.Mutex
 auth config.Auth
 client *http.Client
 keys map[string]publicKey
 until,next time.Time
 flight chan struct{}
 now func()time.Time
}

// NewKeyProbe uses only trusted configuration and refuses redirects and cookies.
func NewKeyProbe(cfg config.Auth,client *http.Client)*KeyProbe{
 c:=http.Client{Transport:http.DefaultTransport.(*http.Transport).Clone()}
 if client!=nil{c=*client};c.Timeout=time.Duration(cfg.RequestTimeout);c.Jar=nil
 c.CheckRedirect=func(*http.Request,[]*http.Request)error{return errors.New("auth: key redirect refused")}
 cfg.Algorithms=append([]string(nil),cfg.Algorithms...)
 return &KeyProbe{auth:cfg,client:&c,now:time.Now}
}

// Check refreshes at most once per configured interval. Failure never extends last success.
func(p *KeyProbe) Check(ctx context.Context)Dependency{
 p.refresh(ctx)
 p.mu.Lock();defer p.mu.Unlock()
 return Dependency{Ready:p.now().Before(p.until),ValidUntil:p.until}
}
func(p *KeyProbe) refresh(ctx context.Context){
 p.mu.Lock()
 if ch:=p.flight;ch!=nil {p.mu.Unlock();select{case <-ctx.Done():case <-ch:};return}
 if p.now().Before(p.next){p.mu.Unlock();return}
 interval:=time.Duration(p.auth.RefreshInterval);if interval<=0{interval=time.Second}
 p.next=p.now().Add(interval);p.flight=make(chan struct{});ch:=p.flight;p.mu.Unlock()
 keys,err:=p.fetch(ctx)
 p.mu.Lock()
 if err==nil {p.keys=keys;p.until=p.now().Add(time.Duration(p.auth.JWKSMaxStale))}
 p.flight=nil;close(ch);p.mu.Unlock()
}
func(p *KeyProbe) fetch(ctx context.Context)(map[string]publicKey,error){
 ctx,cancel:=context.WithTimeout(ctx,time.Duration(p.auth.RequestTimeout));defer cancel()
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,p.auth.JWKSURL,nil);if err!=nil{return nil,ErrKeys}
 resp,err:=p.client.Do(req);if err!=nil{return nil,ErrKeys};defer func(){_ = resp.Body.Close()}()
 if resp.StatusCode!=http.StatusOK{return nil,ErrKeys}
 data,err:=io.ReadAll(io.LimitReader(resp.Body,(1<<20)+1));if err!=nil||len(data)>1<<20{return nil,ErrKeys}
 return parseKeys(data,p.auth.Algorithms)
}
func(p *KeyProbe) key(ctx context.Context,kid,algorithm string)(any,error){
 lookup:=func()(publicKey,bool){p.mu.Lock();defer p.mu.Unlock();k,ok:=p.keys[kid];return k,ok&&p.now().Before(p.until)}
 k,ok:=lookup();if !ok{p.refresh(ctx);k,ok=lookup()}
 if !ok{return nil,ErrKeys}
 if k.algorithm!=""&&k.algorithm!=algorithm{return nil,ErrKeys}
 return k.key,nil
}
// Close releases idle sockets after all request/monitor work has joined.
func(p *KeyProbe) Close(){p.client.CloseIdleConnections()}

func parseKeys(data []byte,allowed []string)(map[string]publicKey,error){
 if !validKeys(data,allowed){return nil,ErrKeys}
 var doc struct{Keys []map[string]json.RawMessage `json:"keys"`}
 if json.Unmarshal(data,&doc)!=nil{return nil,ErrKeys}
 result:=make(map[string]publicKey,len(doc.Keys))
 for _,k:=range doc.Keys{
  key:=publicKey{algorithm:str(k,"alg")}
  switch str(k,"kty"){
  case "RSA":key.key=&rsa.PublicKey{N:new(big.Int).SetBytes(decode(str(k,"n"))),E:int(new(big.Int).SetBytes(decode(str(k,"e"))).Int64())}
  case "EC":
   var curve elliptic.Curve
   switch str(k,"crv"){case "P-256":curve=elliptic.P256();key.algorithm="ES256";case "P-384":curve=elliptic.P384();key.algorithm="ES384";case "P-521":curve=elliptic.P521();key.algorithm="ES512"}
   key.key=&ecdsa.PublicKey{Curve:curve,X:new(big.Int).SetBytes(decode(str(k,"x"))),Y:new(big.Int).SetBytes(decode(str(k,"y")))}
  }
  result[str(k,"kid")]=key
 }
 return result,nil
}
// ValidPublicKeys is a non-secret validation seam retained for foundation conformance.
func ValidPublicKeys(data []byte,allowed []string)bool{return validKeys(data,allowed)}
