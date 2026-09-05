package acceptance

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/foundation"
	"github.com/hurtener/chartworks/internal/telemetry"
	"github.com/hurtener/chartworks/test/support"
)

const canary="CREDENTIAL_CANARY_do_not_log_937"
func configBytes(t testing.TB,change func(map[string]any))[]byte{
	t.Helper();m:=map[string]any{"auth":map[string]any{"issuer":"https://issuer.example","jwks_url":"https://issuer.example/keys","audience":"chartworks-test"}}
	if change!=nil{change(m)};b,e:=json.Marshal(m);if e!=nil{t.Fatal(e)};return b
}
func environment(dsn string)func(string)(string,bool){return func(k string)(string,bool){if k=="CHARTWORKS_STORE_URL"{return dsn,true};return "",false}}
func loaded(t testing.TB,b []byte,dsn string)config.Config{t.Helper();c,e:=config.Load(bytes.NewReader(b),environment(dsn),config.Overrides{});if e!=nil{t.Fatal(e)};return c}
func keyDocument(t testing.TB)[]byte{
	t.Helper();k,e:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader);if e!=nil{t.Fatal("test key generation failed")}
	b,e:=json.Marshal(map[string]any{"keys":[]any{map[string]any{"kty":"EC","kid":"fixture","crv":"P-256","alg":"ES256","use":"sig","x":base64.RawURLEncoding.EncodeToString(k.X.FillBytes(make([]byte,32))),"y":base64.RawURLEncoding.EncodeToString(k.Y.FillBytes(make([]byte,32)))}}});if e!=nil{t.Fatal(e)};return b
}
func waitFor(t testing.TB,fn func()bool){t.Helper();deadline:=time.Now().Add(8*time.Second);for time.Now().Before(deadline){if fn(){return};time.Sleep(10*time.Millisecond)};t.Fatal("condition did not become true before deadline")}
func response(t testing.TB,client *http.Client,address,path string)(int,string){t.Helper();r,e:=client.Get(address+path);if e!=nil{return 0,""};defer func(){_ = r.Body.Close()}();b,e:=io.ReadAll(io.LimitReader(r.Body,8192));if e!=nil{t.Fatal("read health response failed")};return r.StatusCode,string(b)}

type badReader struct{}
func (badReader)Read([]byte)(int,error){return 0,errors.New(canary)}
type badWriter struct{}
func (badWriter)Write([]byte)(int,error){return 0,errors.New(canary)}

func TestPhase01(t *testing.T){
	t.Run("AC01",func(t *testing.T){
		valid:=configBytes(t,nil)
		for _,b:=range [][]byte{[]byte(`{"auth":{"mode":"self_issue"}}`),[]byte(`{"server":{"listen":"`+canary+`","unknown":true}}`),[]byte(`{"auth":null}`),[]byte(`{"auth":{},"auth":{}}`),[]byte(`[]`),[]byte(`{} {}`),[]byte(`{"server":{"read_timeout":123}}`),[]byte(`{"server":{"read_timeout":"`+canary+`"}}`),bytes.Repeat([]byte("x"),(1<<20)+1)}{
			_,e:=config.Load(bytes.NewReader(b),environment(canary),config.Overrides{});if e==nil||strings.Contains(e.Error(),canary){t.Fatal("unsafe or accepted invalid configuration")}
		}
		if _,e:=config.Load(badReader{},environment(canary),config.Overrides{});e==nil||strings.Contains(e.Error(),canary){t.Fatal("reader error leaked")}
		if _,e:=config.Load(bytes.NewReader(valid),environment(""),config.Overrides{});e==nil{t.Fatal("missing secret accepted")}
		mutations:=[]func(map[string]any){
			func(m map[string]any){m["store"]=map[string]any{"dsn":canary}},
			func(m map[string]any){m["server"]=map[string]any{"listen":"0.0.0.0:8080"}},
			func(m map[string]any){m["server"]=map[string]any{"max_body_bytes":0}},
			func(m map[string]any){m["auth"].(map[string]any)["algorithms"]=[]string{"HS256"}},
			func(m map[string]any){m["auth"].(map[string]any)["jwks_url"]="http://issuer.example/keys"},
			func(m map[string]any){m["auth"].(map[string]any)["clock_skew"]="-1s"},
			func(m map[string]any){m["gateway"]=map[string]any{"driver":"local"}},
			func(m map[string]any){m["features"]=map[string]any{"mcp":true}},
			func(m map[string]any){m["telemetry"]=map[string]any{"otel":true}},
		}
		for _,f:=range mutations{if _,e:=config.Load(bytes.NewReader(configBytes(t,f)),environment(canary),config.Overrides{});e==nil{t.Fatal("invalid feature/security config accepted")}}
		c:=loaded(t,valid,"postgres://fixture:"+canary+"@127.0.0.1/fixture")
		b,e:=json.Marshal(c);if e!=nil{t.Fatal(e)}
		if strings.Contains(string(b)+fmt.Sprintf("%v %#v",c,c),canary){t.Fatal("configuration printed a resolved credential")}
	})
	t.Run("AC02",func(t *testing.T){
		dsn:=support.Database(t);db:=support.Open(t,dsn);doc:=keyDocument(t);var fail atomic.Bool
		keys:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if fail.Load(){w.WriteHeader(503);return};_,_ = w.Write(doc)}));defer keys.Close()
		b:=configBytes(t,func(m map[string]any){m["auth"].(map[string]any)["jwks_url"]=keys.URL;m["auth"].(map[string]any)["refresh_interval"]="20ms";m["auth"].(map[string]any)["jwks_max_stale"]="100ms";m["auth"].(map[string]any)["request_timeout"]="100ms"})
		c:=loaded(t,b,dsn);r,e:=telemetry.New(io.Discard,"json");if e!=nil{t.Fatal(e)};probe:=foundation.NewKeyProbe(c.Values().Auth,keys.Client())
		s,e:=foundation.NewServer(c,r,func(ctx context.Context)foundation.Dependency{return foundation.Dependency{Ready:db.Check(ctx)==nil}},probe.Check);if e!=nil{t.Fatal(e)}
		l,e:=net.Listen("tcp","127.0.0.1:0");if e!=nil{t.Fatal(e)};ctx,cancel:=context.WithCancel(context.Background());defer cancel();done:=make(chan error,1);go func(){done<-s.Serve(ctx,l)}()
		address:="http://"+l.Addr().String();client:=&http.Client{Timeout:time.Second}
		waitFor(t,func()bool{status,_:=response(t,client,address,"/readyz");return status==200})
		fail.Store(true);waitFor(t,func()bool{status,body:=response(t,client,address,"/readyz");return status==503&&strings.Contains(body,"verification_keys")})
		if status,_:=response(t,client,address,"/healthz");status!=200{t.Fatal("dependency outage killed liveness")}
		fail.Store(false);waitFor(t,func()bool{status,_:=response(t,client,address,"/readyz");return status==200})
		db.Close();waitFor(t,func()bool{status,body:=response(t,client,address,"/readyz");return status==503&&strings.Contains(body,`"store":"unavailable"`)})
		cancel();select{case e:=<-done:if e!=nil{t.Fatal(e)};case <-time.After(2*time.Second):t.Fatal("shutdown did not join")}
		if status,_:=response(t,client,address,"/healthz");status!=0{t.Fatal("listener survived shutdown")}
		if _,e=foundation.NewServer(config.Config{},r,nil,nil);e==nil{t.Fatal("missing probes accepted")}
	})
	t.Run("AC03",func(t *testing.T){
		var logs bytes.Buffer;r,e:=telemetry.New(&logs,"json");if e!=nil{t.Fatal(e)}
		for _,event:=range []telemetry.Event{telemetry.Started,telemetry.Stopped,telemetry.ConfigurationRejected,telemetry.DependencyChanged,telemetry.AuditCommitted,telemetry.Request}{for _,ok:=range []bool{true,false}{if e=r.Record(context.Background(),event,ok);e!=nil{t.Fatal(e)}}}
		if e=r.Record(context.Background(),telemetry.Event(canary),true);e==nil||strings.Contains(e.Error(),canary){t.Fatal("unbounded event accepted/leaked")}
		if e=r.Dependency(canary,true);e==nil||strings.Contains(e.Error(),canary){t.Fatal("unbounded metric label")}
		if e=r.Dependency("store",true);e!=nil{t.Fatal(e)}
		w:=httptest.NewRecorder();r.Handler().ServeHTTP(w,httptest.NewRequest("GET","/metrics",nil));if w.Code!=200{t.Fatal("metrics failed")}
		for _,name:=range telemetry.MetricNames(){if !strings.Contains(w.Body.String(),name){t.Fatalf("registered metric not exported: %s",name)}}
		if strings.Contains(logs.String()+w.Body.String(),canary){t.Fatal("secret in observability")}
		if _,e=telemetry.New(nil,"json");e==nil{t.Fatal("nil writer accepted")};if _,e=telemetry.New(io.Discard,"unknown");e==nil{t.Fatal("unknown log format accepted")}
	})
	t.Run("AC04",func(t *testing.T){
		c:=loaded(t,configBytes(t,nil),"fixture");r,e:=telemetry.New(io.Discard,"text");if e!=nil{t.Fatal(e)}
		s,e:=foundation.NewServer(c,r,func(context.Context)foundation.Dependency{return foundation.Dependency{}},func(context.Context)foundation.Dependency{return foundation.Dependency{}});if e!=nil{t.Fatal(e)}
		for _,test:=range []struct{method,path,body string;code int}{{"GET","/healthz","",200},{"HEAD","/healthz","",200},{"GET","/readyz","",503},{"GET","/capabilities","",200},{"POST","/healthz","",405},{"GET","/healthz","x",400},{"GET","/v1/admin/keys","",404},{"GET","/metrics","",404},{"GET","/mcp","",404}}{
			w:=httptest.NewRecorder();q:=httptest.NewRequest(test.method,test.path,strings.NewReader(test.body));q.Header.Set("Authorization","Bearer "+canary);s.Handler().ServeHTTP(w,q);if w.Code!=test.code{t.Fatalf("%s: got %d",test.path,w.Code)}
			if strings.Contains(w.Body.String(),canary){t.Fatal("credential echoed")};if test.path=="/capabilities"&&!strings.Contains(w.Body.String(),`"business_api":false`){t.Fatal("unimplemented capability advertised")}
		}
		q:=httptest.NewRequest("GET","/healthz",nil);q.ContentLength=100<<20;w:=httptest.NewRecorder();s.Handler().ServeHTTP(w,q);if w.Code!=413{t.Fatal("body limit not applied")}
		if e=s.Serve(context.Background(),nil);e==nil{t.Fatal("nil listener accepted")}
	})
	t.Run("AC05",func(t *testing.T){
		dir:=t.TempDir();path:=filepath.Join(dir,"config.json");if e:=os.WriteFile(path,configBytes(t,nil),0600);e!=nil{t.Fatal(e)}
		lookup:=func(k string)(string,bool){switch k{case "CHARTWORKS_STORE_URL":return canary,true;case "CHARTWORKS_CONFIG":return path,true};return "",false}
		start:=func(context.Context,config.Config,io.Writer)error{return nil}
		build:=foundation.Build{Version:"test",Commit:"fixture",Date:"2026-09-05"}
		for _,tc:=range []struct{args []string;code int}{{nil,2},{[]string{"unknown"},2},{[]string{"version"},0},{[]string{"version","extra"},2},{[]string{"mcp"},3},{[]string{"config-check"},0},{[]string{"config-check","--defaults"},0},{[]string{"serve","--defaults"},2},{[]string{"serve","--unknown",canary},2},{[]string{"serve","--config",path,"--listen","127.0.0.1:0"},0},{[]string{"config-check","--config",filepath.Join(dir,"missing")},2}}{
			var out,errout bytes.Buffer;code:=foundation.Command(context.Background(),tc.args,lookup,&out,&errout,build,start);if code!=tc.code{t.Fatalf("command %v: %d; %s",tc.args,code,errout.String())};if strings.Contains(out.String()+errout.String(),canary){t.Fatal("command leaked input")}
		}
		var out bytes.Buffer
		if foundation.Command(context.Background(),[]string{"serve"},lookup,&out,&out,build,nil)!=1{t.Fatal("nil lifecycle accepted")}
		if foundation.Command(context.Background(),[]string{"serve"},lookup,&out,&out,build,func(context.Context,config.Config,io.Writer)error{return errors.New(canary)})!=1||strings.Contains(out.String(),canary){t.Fatal("startup error leaked")}
		if foundation.Command(context.Background(),[]string{"version"},lookup,badWriter{},&out,build,start)!=1{t.Fatal("write failure ignored")}
		b:=configBytes(t,func(m map[string]any){m["server"]=map[string]any{"listen":"env:LISTEN"}})
		c,e:=config.Load(bytes.NewReader(b),func(k string)(string,bool){if k=="LISTEN"{return "127.0.0.1:9000",true};return lookup(k)},config.Overrides{Listen:"127.0.0.1:9001"});if e!=nil||c.Values().Server.Listen!="127.0.0.1:9001"{t.Fatal("source precedence failed")}
	})
	t.Run("AC06",func(t *testing.T){
		b:=configBytes(t,nil);c:=loaded(t,b,canary);r,e:=telemetry.New(io.Discard,"json");if e!=nil{t.Fatal(e)}
		var wg sync.WaitGroup;for i:=0;i<100;i++{wg.Add(1);go func(){defer wg.Done();v:=c.Values();v.Auth.Algorithms[0]="HS256";if _,e:=config.Load(bytes.NewReader(b),environment(canary),config.Overrides{});e!=nil{t.Error(e)};if e:=r.Record(context.Background(),telemetry.Request,true);e!=nil{t.Error(e)};w:=httptest.NewRecorder();r.Handler().ServeHTTP(w,httptest.NewRequest("GET","/metrics",nil));if w.Code!=200{t.Error("concurrent export failed")}}()};wg.Wait()
		if c.Values().Auth.Algorithms[0]!="RS256"{t.Fatal("mutable live configuration")}
		t.Logf("measured test environment: %s %s/%s CPUs=%d; benchmarks record startup/config and idle health cost separately",runtime.Version(),runtime.GOOS,runtime.GOARCH,runtime.NumCPU())
	})
}

func BenchmarkConfigurationStartup(b *testing.B){data:=configBytes(b,nil);b.ReportAllocs();b.ResetTimer();for i:=0;i<b.N;i++{if _,e:=config.Load(bytes.NewReader(data),environment("fixture"),config.Overrides{});e!=nil{b.Fatal(e)}}}
func BenchmarkIdleHealth(b *testing.B){cfg:=loaded(b,configBytes(b,nil),"fixture");r,e:=telemetry.New(io.Discard,"json");if e!=nil{b.Fatal(e)};s,e:=foundation.NewServer(cfg,r,func(context.Context)foundation.Dependency{return foundation.Dependency{}},func(context.Context)foundation.Dependency{return foundation.Dependency{}});if e!=nil{b.Fatal(e)};handler:=s.Handler();b.ReportAllocs();b.ResetTimer();for i:=0;i<b.N;i++{handler.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/healthz",nil))}}
