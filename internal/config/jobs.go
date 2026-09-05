package config

import (
 "net/url"
 "strings"
 "time"
)

// BrokerCredential is a reference to a Pengui-owned existing broker client. Neither value is logged.
type BrokerCredential struct {Tenant string `json:"tenant"`;ClientID string `json:"client_id"`;ClientSecret string `json:"client_secret"`}
// Jobs configures one durable worker family; metadata reads work with this capability disabled.
type Jobs struct {
 Enabled bool `json:"enabled"`
 Workers int `json:"workers"`;GlobalConcurrency int `json:"global_concurrency"`;TenantConcurrency int `json:"tenant_concurrency"`
 MaxPending int `json:"max_pending"`;MaxPendingPerTenant int `json:"max_pending_per_tenant"`;MaxAttempts int `json:"max_attempts"`;Batch int `json:"batch"`
 Lease Duration `json:"lease"`;Heartbeat Duration `json:"heartbeat"`;Poll Duration `json:"poll"`;AttemptTimeout Duration `json:"attempt_timeout"`;Backoff Duration `json:"backoff"`
 BrokerURL string `json:"broker_url"`
 Credentials []BrokerCredential `json:"credentials"`
}
func DefaultJobs()Jobs{return Jobs{Workers:4,GlobalConcurrency:16,TenantConcurrency:2,MaxPending:10000,MaxPendingPerTenant:1000,MaxAttempts:3,Batch:100,Lease:Duration(15*time.Second),Heartbeat:Duration(5*time.Second),Poll:Duration(500*time.Millisecond),AttemptTimeout:Duration(10*time.Second),Backoff:Duration(time.Second),Credentials:[]BrokerCredential{}}}
func ValidateJobs(j Jobs,a Auth)error{
 if j.Workers<1||j.Workers>32||j.GlobalConcurrency<j.Workers||j.GlobalConcurrency>128||j.TenantConcurrency<1||j.TenantConcurrency>j.GlobalConcurrency||j.MaxPending<1||j.MaxPending>100000||j.MaxPendingPerTenant<1||j.MaxPendingPerTenant>j.MaxPending||j.MaxAttempts<1||j.MaxAttempts>8||j.Batch<1||j.Batch>1000||j.Lease<Duration(time.Second)||j.Lease>Duration(time.Minute)||j.Heartbeat<Duration(10*time.Millisecond)||j.Heartbeat>=j.Lease/2||j.Poll<Duration(10*time.Millisecond)||j.Poll>Duration(5*time.Second)||j.AttemptTimeout<Duration(100*time.Millisecond)||j.AttemptTimeout>Duration(time.Minute)||j.Backoff<Duration(10*time.Millisecond)||j.Backoff>Duration(30*time.Second){return invalid("jobs","invalid worker or admission bounds")}
 if j.BrokerURL!=""{u,err:=url.Parse(j.BrokerURL);if err!=nil||!secureURL(j.BrokerURL)||!strings.HasSuffix(u.Path,"/exchange/execution-authority"){return invalid("jobs.broker_url","trusted Pengui execution endpoint required")}}
 if len(j.Credentials)>128{return invalid("jobs.credentials","too many tenant credentials")}
 seen:=map[string]bool{};for _,c:=range j.Credentials{
  if c.Tenant==""||len(c.Tenant)>128||seen[c.Tenant]{return invalid("jobs.credentials","unique tenant coordinates required")};for _,r:=range c.Tenant{if !(r>='a'&&r<='z'||r>='A'&&r<='Z'||r>='0'&&r<='9'||r=='_'||r=='-'||r=='.'||r==':'){return invalid("jobs.credentials","invalid tenant coordinate")}}
  if _,err:=reference(c.ClientID);err!=nil{return invalid("jobs.credentials.client_id","explicit environment reference required")};if _,err:=reference(c.ClientSecret);err!=nil{return invalid("jobs.credentials.client_secret","explicit environment reference required")};seen[c.Tenant]=true
 }
 if j.Enabled&&(j.BrokerURL==""||len(j.Credentials)==0||a.Audiences.Jobs==""||a.MaxTokenLifetime<Duration(30*time.Second)){return invalid("jobs","Pengui broker, credentials and execution audience required")};return nil
}
