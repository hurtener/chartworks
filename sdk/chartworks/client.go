// Package chartworks is the token-forwarding client for implemented Chartworks operations.
// Tokens come from Pengui. This client never signs, exchanges, renews or logs credentials.
package chartworks

import(
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "io"
 "net"
 "net/http"
 "net/url"
 "strings"
 "time"
)
// Policy is operational retention state, not access policy.
type Policy struct{Revision int64 `json:"revision"`;AuditDays int `json:"audit_days"`;OperationHours int `json:"operation_hours"`}
// Audit is an authorized metadata record.
type Audit struct{ID string `json:"id"`;Actor string `json:"actor"`;Action string `json:"action"`;Resource string `json:"resource"`;CreatedAt time.Time `json:"created_at"`}
// Operation is the accepted bounded sweep result.
type Operation struct{ID string `json:"id"`;Status string `json:"status"`;PolicyRevision int64 `json:"policy_revision"`;Cutoff time.Time `json:"cutoff"`;Limit int `json:"limit"`;DeletedEvents int64 `json:"deleted_events"`;DeletedOperations int64 `json:"deleted_operations"`}
// StatusError contains no private server/body/token content.
type StatusError struct{Status int}
func(e *StatusError) Error()string{return "chartworks: request rejected"}
// TokenProvider supplies current Pengui credentials. Errors are never echoed.
type TokenProvider func(context.Context)(string,error)
// Client is safe for concurrent requests provided its token provider is also safe.
type Client struct{base string;http *http.Client;token TokenProvider}
// New pins a trusted backend URL and refuses credential-bearing redirect requests.
func New(base string,client *http.Client,token TokenProvider)(*Client,error){
 u,err:=url.Parse(base);if err!=nil||u.Hostname()==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||(u.Path!=""&&u.Path!="/")||token==nil{return nil,errors.New("chartworks: invalid client configuration")}
 ip:=net.ParseIP(u.Hostname());if u.Scheme!="https"&&(u.Scheme!="http"||ip==nil||!ip.IsLoopback()){return nil,errors.New("chartworks: HTTPS required")}
 c:=http.Client{Timeout:30*time.Second};if client!=nil{c=*client};c.Jar=nil;c.CheckRedirect=func(*http.Request,[]*http.Request)error{return errors.New("chartworks: redirect refused")}
 return &Client{strings.TrimSuffix(base,"/"),&c,token},nil
}
func(c *Client) call(ctx context.Context,method,path,key string,body,out any)error{
 token,err:=c.token(ctx);if err!=nil||token==""||strings.ContainsAny(token," \r\n\t,"){return errors.New("chartworks: credential unavailable")}
 var input []byte;if body!=nil{input,err=json.Marshal(body);if err!=nil{return errors.New("chartworks: invalid request")}}
 req,err:=http.NewRequestWithContext(ctx,method,c.base+path,bytes.NewReader(input));if err!=nil{return errors.New("chartworks: invalid request")}
 req.Header.Set("Authorization","Bearer "+token);if body!=nil{req.Header.Set("Content-Type","application/json")};if key!=""{req.Header.Set("Idempotency-Key",key)}
 resp,err:=c.http.Do(req);if err!=nil{return errors.New("chartworks: transport failed")};defer func(){_ = resp.Body.Close()}()
 if resp.StatusCode!=http.StatusOK{return &StatusError{resp.StatusCode}}
 data,err:=io.ReadAll(io.LimitReader(resp.Body,(1<<20)+1));if err!=nil||len(data)>1<<20||json.Unmarshal(data,out)!=nil{return errors.New("chartworks: invalid response")};return nil
}
// RetentionPolicy reads only the caller's signed tenant configuration.
func(c *Client) RetentionPolicy(ctx context.Context)(Policy,error){var p Policy;err:=c.call(ctx,"GET","/v1/retention-policy","",nil,&p);return p,err}
// SetRetentionPolicy uses explicit optimistic concurrency, not an unconditional update.
func(c *Client) SetRetentionPolicy(ctx context.Context,expected int64,auditDays,operationHours int)(Policy,error){
 body:=struct{Expected int64 `json:"expected_revision"`;AuditDays int `json:"audit_days"`;OperationHours int `json:"operation_hours"`}{expected,auditDays,operationHours};var p Policy;err:=c.call(ctx,"PUT","/v1/retention-policy","",body,&p);return p,err
}
// AuditEvents reads a bounded tenant-isolated page.
func(c *Client) AuditEvents(ctx context.Context)([]Audit,error){var a []Audit;err:=c.call(ctx,"GET","/v1/audit-events","",nil,&a);return a,err}
// Sweep preserves the supplied logical operation key; no automatic retries are hidden here.
func(c *Client) Sweep(ctx context.Context,key string)(Operation,error){if key==""{return Operation{},errors.New("chartworks: idempotency key required")};var o Operation;err:=c.call(ctx,"POST","/v1/retention-sweeps",key,struct{}{},&o);return o,err}
