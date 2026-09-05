package acceptance

import (
 "context"
 "encoding/json"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"

 "github.com/hurtener/chartworks/internal/auth"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/jobs"
 broker "github.com/hurtener/chartworks/internal/jobs/pengui"
 "github.com/hurtener/chartworks/internal/store"
 "github.com/hurtener/chartworks/internal/store/postgres"
 "github.com/hurtener/chartworks/test/support"
)

type queueFixture struct {db *postgres.DB;dsn string;token *tokenFixture;provider *broker.Provider;service *jobs.Service;limits jobs.Limits;actor identity.Envelope;mode atomic.Int64;calls atomic.Int64;mu sync.Mutex;requests []map[string]string;tokens []string}
func newQueueFixture(t *testing.T,change func(*jobs.Limits))*queueFixture{
 t.Helper();q:=&queueFixture{dsn:support.Database(t),token:newTokenFixture(t),limits:jobs.Defaults()};q.db=support.Open(t,q.dsn)
 q.limits.Workers=2;q.limits.GlobalConcurrency=4;q.limits.TenantConcurrency=2;q.limits.Lease=time.Second;q.limits.Heartbeat=100*time.Millisecond;q.limits.Poll=20*time.Millisecond;q.limits.AttemptTimeout=500*time.Millisecond;q.limits.Backoff=20*time.Millisecond
 if change!=nil{change(&q.limits)}
 q.actor=q.token.envelope(t,"queue-tenant","operator","scheduling.write","scheduling.read","scheduling.execute","scheduling.cancel","ops.maintain","cw.tenant.write:queue-tenant","cw.tenant.erase:queue-tenant","cw.execution_binding.use:maintenance","cw.execution_binding.use:other","cw.run.read:*","cw.run.write:*","cw.schedule.read:*","cw.schedule.write:*","cw.schedule.execute:*")
 if _,err:=q.db.SetPolicy(context.Background(),support.Scope(t,"queue-tenant","operator"),0,store.Policy{AuditDays:7,OperationHours:24});err!=nil{t.Fatal(err)}
 cfg:=q.token.cfg;cfg.Audiences.Jobs="chartworks:execution"
 verifier,err:=auth.New(cfg,q.token.server.Client(),nil);if err!=nil{t.Fatal(err)};t.Cleanup(verifier.Close)
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  q.calls.Add(1);client,secret,ok:=r.BasicAuth();if !ok||client!="broker"||secret!="SYNTHETIC_BROKER_SECRET"||r.URL.Path!="/exchange/execution-authority"{w.WriteHeader(401);return}
  data,_:=io.ReadAll(io.LimitReader(r.Body,4097));var input struct{Version int `json:"version"`;Binding string `json:"binding_id"`;Job string `json:"job_id"`;Manifest string `json:"manifest_hash"`};if json.Unmarshal(data,&input)!=nil||input.Version!=1{w.WriteHeader(400);return}
  q.mu.Lock();q.requests=append(q.requests,map[string]string{"binding":input.Binding,"job":input.Job,"manifest":input.Manifest});q.mu.Unlock()
  switch q.mode.Load(){case 1:w.WriteHeader(403);return;case 2:w.WriteHeader(503);return;case 9:<-r.Context().Done();return}
  scopes:=[]string{"ops.maintain","cw.tenant.erase:queue-tenant","cw.execution_binding.use:"+input.Binding,"cw.run.execute:"+input.Job}
  claims:=q.token.claims("queue-tenant",jobs.Executor(input.Binding),scopes);now:=time.Now().Unix();claims["iat"]=now;claims["nbf"]=now-60;claims["exp"]=now+30;claims["aud"]="chartworks:execution";claims["session"]=input.Job;claims["execution_version"]=1;claims["execution_binding"]=input.Binding;claims["execution_binding_revision"]=1;claims["execution_manifest"]=input.Manifest
  switch q.mode.Load(){case 3:claims["exp"]=now-1;claims["iat"]=now-31;case 4:claims["scopes"]=[]string{"ops.maintain"};case 5:claims["execution_manifest"]=strings.Repeat("b",64);case 6:claims["execution_binding"]="other";case 7:claims["aud"]=q.token.cfg.HTTPAudience();case 8:claims["user"]="svc:coordinator";claims["sub"]="svc:coordinator"}
  token:=q.token.sign(t,claims,nil);q.mu.Lock();q.tokens=append(q.tokens,token);q.mu.Unlock()
  w.Header().Set("Content-Type","application/json");_ = json.NewEncoder(w).Encode(map[string]any{"version":1,"access_token":token,"token_type":"Bearer","expires_in":30,"binding_id":input.Binding,"binding_revision":1})
 }));t.Cleanup(server.Close)
 q.provider,err=broker.New(server.URL+"/exchange/execution-authority",map[string]broker.Credential{"queue-tenant":{ClientID:"broker",Secret:"SYNTHETIC_BROKER_SECRET"}},verifier,server.Client());if err!=nil{t.Fatal(err)};t.Cleanup(q.provider.Close)
 q.service,err=jobs.New(q.db,q.provider,q.limits);if err!=nil{t.Fatal(err)};return q
}
func(q *queueFixture)submit(t *testing.T,key string)jobs.Job{t.Helper();j,err:=q.service.Submit(context.Background(),q.actor,key,jobs.Submission{Kind:jobs.MaintenanceKind,BindingID:"maintenance"});if err!=nil{t.Fatal(err)};return j}
func(q *queueFixture)oldAudit(t *testing.T)string{t.Helper();raw:=support.Raw(t,q.dsn);id:=strings.Repeat("f",32);_,err:=raw.Exec(context.Background(),`INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id,created_at) VALUES('queue-tenant',$1,'operator','retention_policy.updated','retention',clock_timestamp()-interval '30 days') ON CONFLICT DO NOTHING`,id);if err!=nil{t.Fatal(err)};return id}
func(q *queueFixture)oldAuditCount(t *testing.T)int{t.Helper();raw:=support.Raw(t,q.dsn);var n int;if err:=raw.QueryRow(context.Background(),`SELECT count(*) FROM chartworks.audit_events WHERE tenant_id='queue-tenant' AND event_id=$1`,strings.Repeat("f",32)).Scan(&n);err!=nil{t.Fatal(err)};return n}
