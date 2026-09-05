package acceptance
import(
 "context"
 "net/http"
 "testing"
 "strings"
 "io"
 "github.com/hurtener/chartworks/internal/securityapi"
 "github.com/hurtener/chartworks/internal/telemetry"
)
func TestProtectedRequestNegatives(t *testing.T){
 f:=newTokenFixture(t);s,h,_:=protectedFixture(t,f);token:=f.sign(t,f.claims("tenant","user",operationalScopes("tenant")),nil)
 for _,body:=range []string{`{}`,`{"tenant":"foreign","expected_revision":0,"audit_days":7,"operation_hours":24}`,`{"expected_revision":0,"audit_days":7,"operation_hours":24,"AUDIT_DAYS":1}`,`{"expected_revision":0,"audit_days":7,"audit_days":1,"operation_hours":24}`,`{"expected_revision":0,"audit_days":null,"operation_hours":24}`,`{"expected_revision":0,"audit_days":0,"operation_hours":24}`,strings.Repeat("x",4097),`{"expected_revision":0,"audit_days":"secret","operation_hours":24}`} {w:=callProtected(t,h,"PUT","/v1/retention-policy",token,body,nil);if w.Code!=422{t.Fatal("invalid mutation accepted",w.Code)};if strings.Contains(w.Body.String(),"secret"){t.Fatal("echoed invalid value")}}
 if callProtected(t,h,"PUT","/v1/retention-policy",token,`{"expected_revision":0,"audit_days":7,"operation_hours":24}`,map[string]string{"Content-Type":"text/plain"}).Code!=422{t.Fatal("wrong media type")}
 if callProtected(t,h,"GET","/v1/retention-policy",token,`{}`,nil).Code!=422{t.Fatal("GET body accepted")}
 if callProtected(t,h,"POST","/v1/retention-sweeps",token,`{}`,nil).Code!=422{t.Fatal("missing key accepted")}
 if callProtected(t,h,"POST","/v1/retention-sweeps",token,`{"tenant":"foreign"}`,map[string]string{"Idempotency-Key":"key"}).Code!=422{t.Fatal("sweep authority field accepted")}
 for _,query:=range []string{"limit=bad","limit=0","limit=1001","limit=1&limit=2","tenant=foreign","limit=%zz","limit=1;tenant=foreign"}{if callProtected(t,h,"GET","/v1/audit-events?"+query,token,"",nil).Code!=422{t.Fatal("invalid query accepted")}}
 if callProtected(t,h,"DELETE","/v1/retention-policy",token,"",nil).Code!=404{t.Fatal("unregistered method")}
 if callProtected(t,h,"GET","/metrics",token,"",nil).Code!=200{t.Fatal("scoped aggregate metrics unavailable")}
 r,err:=telemetry.New(io.Discard,"json");if err!=nil{t.Fatal(err)};disabled:=securityapi.Handler(f.verifier,s,r,false);if callProtected(t,disabled,"GET","/metrics",token,"",nil).Code!=404{t.Fatal("disabled metrics exposed")}
 c:=f.claims("tenant","user",operationalScopes("tenant"));c["aud"]=f.cfg.MCPAudience();wrong:=f.sign(t,c,nil);if callProtected(t,h,"GET","/v1/retention-policy",wrong,"",nil).Code!=http.StatusUnauthorized{t.Fatal("MCP token replayed to HTTP")}
 ctx,cancel:=context.WithCancel(context.Background());cancel();_ = ctx
}
