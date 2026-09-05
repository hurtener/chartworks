from pathlib import Path
import json

def edit(path,old,new):
 p=Path(path);s=p.read_text();assert old in s,(path,old);p.write_text(s.replace(old,new))

# Reuse the already adversarially tested public-key parser, rather than two implementations.
p=Path('internal/foundation/keys.go');s=p.read_text()
validation=s[s.index('func str('):s.index('// Close releases')]+s[s.index('func uniqueValue('):]
header='''package auth
import("bytes";"crypto/ecdh";"encoding/base64";"encoding/json";"errors";"io";"math/big";"strings")
'''
validation=validation.replace('base64.RawURLEncoding.DecodeString(s)','base64.RawURLEncoding.Strict().DecodeString(s)')
Path('internal/auth/key_validation.go').write_text(header+validation)
p.write_text('''// Package foundation composes configuration, verified operational APIs and lifecycle.
package foundation
import("github.com/hurtener/chartworks/internal/auth";"github.com/hurtener/chartworks/internal/config";"net/http")
// Dependency is a sanitized observation from the shared verifier cache.
type Dependency = auth.Dependency
// KeyProbe is retained for earlier foundation callers; the implementation is shared with auth.
type KeyProbe = auth.KeyProbe
// NewKeyProbe delegates to the single trusted public-key loader.
func NewKeyProbe(cfg config.Auth,c *http.Client)*KeyProbe{return auth.NewKeyProbe(cfg,c)}
func validKeys(data []byte,allowed []string)bool{return auth.ValidPublicKeys(data,allowed)}
''')
edit('internal/config/config.go','Audience         string   `json:"audience"`','''Audience         string   `json:"audience,omitempty"`
 Audiences Audiences `json:"audiences,omitempty"`
 MaxTokenBytes int `json:"max_token_bytes"`
 MaxClaimBytes int `json:"max_claim_bytes"`
 MaxScopes int `json:"max_scopes"`
 MaxScopeBytes int `json:"max_scope_bytes"`''')
edit('internal/config/config.go','Auth{Algorithms:','Auth{MaxTokenBytes:32768,MaxClaimBytes:24576,MaxScopes:128,MaxScopeBytes:8192,Algorithms:')
edit('internal/config/config.go','{"auth.audience", &v.Auth.Audience}','{"auth.audience", &v.Auth.Audience}, {"auth.audiences.http", &v.Auth.Audiences.HTTP}, {"auth.audiences.mcp", &v.Auth.Audiences.MCP}')
edit('internal/config/config.go','if len(v.Auth.Audience) == 0 || len(v.Auth.Audience) > 512 || strings.ContainsAny(v.Auth.Audience, "\\r\\n\\t ") {\n\t\treturn invalid("auth.audience", "bounded exact audience required")\n\t}', 'if err:=ValidateAuth(v.Auth);err!=nil{return err}')
# Preserve loopback-only binding as a conservative deployment default in this scoped change.
edit('internal/config/config.go','// Only health is exposed until phase 03. Do not accidentally deploy an open business API.','// External listener/TLS exposure remains with phase21; operational routes now require JWTs.')
edit('internal/config/config.go','// Auth configures trusted verification-key health, not an issuer or login service.','// Auth configures the Pengui verifier and its shared key cache, never a local issuer.')
# One actual verifier/cache is used by production readiness and request handling.
edit('internal/foundation/command.go','"github.com/hurtener/chartworks/internal/config"','"github.com/hurtener/chartworks/internal/config"\n"github.com/hurtener/chartworks/internal/auth"\n"github.com/hurtener/chartworks/internal/securityapi"')
edit('internal/foundation/command.go','keyProbe := NewKeyProbe(v.Auth, nil)','keyProbe,err := auth.New(v.Auth, nil,nil)\nif err!=nil{return err}\nservice,err:=securityapi.New(db)\nif err!=nil{return err}')
edit('internal/foundation/command.go','}, keyProbe.Check)', '}, keyProbe.Check, securityapi.Handler(keyProbe,service,r,v.Telemetry.Metrics))')
edit('internal/foundation/server.go','state       map[string]Dependency','state       map[string]Dependency\nprotected http.Handler')
edit('internal/foundation/server.go','store, keys Checker) (*Server, error)', 'store, keys Checker, protected ...http.Handler) (*Server, error)')
edit('internal/foundation/server.go','return &Server{values: cfg.Values(), reporter: r, store: store, keys: keys, state: map[string]Dependency{}}, nil', 'var h http.Handler\nif len(protected)>1{return nil,errors.New("foundation: one protected router required")}\nif len(protected)==1{h=protected[0]}\nreturn &Server{values: cfg.Values(), reporter: r, store: store, keys: keys, state: map[string]Dependency{},protected:h}, nil')
edit('internal/foundation/server.go','w.WriteHeader(http.StatusNotFound)\n\t\t\treturn','if s.protected!=nil{s.protected.ServeHTTP(w,r);return}\n\t\t\tw.WriteHeader(http.StatusNotFound)\n\t\t\treturn')
edit('internal/foundation/server.go','}{"01-02-foundation", []string{"configuration", "health", "postgresql_metadata"}, false, false}', '}{"01-04-authority", s.implemented(), false, s.protected!=nil}')
with Path('internal/foundation/server.go').open('a') as f:f.write('''
func(s *Server) implemented()[]string{out:=[]string{"configuration","health","postgresql_metadata"};if s.protected!=nil{out=append(out,"jwt_verification","signed_scope_enforcement","operational_api")};return out}
''')
# Wire lower-case public result contracts used by the real SDK.
edit('internal/securityapi/http.go','"github.com/hurtener/chartworks/internal/telemetry"','"github.com/hurtener/chartworks/internal/telemetry"\ncw "github.com/hurtener/chartworks/sdk/chartworks"')
edit('internal/securityapi/http.go','respond(w,p)','respond(w,cw.Policy{Revision:p.Revision,AuditDays:p.AuditDays,OperationHours:p.OperationHours})')
edit('internal/securityapi/http.go','if a==nil{a=[]store.Audit{}};respond(w,a)','out:=make([]cw.Audit,0,len(a));for _,v:=range a{out=append(out,cw.Audit{ID:v.ID,Actor:v.Actor,Action:v.Action,Resource:v.Resource,CreatedAt:v.CreatedAt})};respond(w,out)')
edit('internal/securityapi/http.go','respond(w,result)','respond(w,cw.Operation{ID:result.ID,Status:result.Status,PolicyRevision:result.PolicyRevision,Cutoff:result.Cutoff,Limit:result.Limit,DeletedEvents:result.DeletedEvents,DeletedOperations:result.DeletedOperations})')
# Drop an unused decoder; all authoritative strings use stringValue.
p=Path('internal/auth/verifier.go');s=p.read_text();start=s.index('func text(');end=s.index('func stringValue(',start);p.write_text(s[:start]+s[end:])
# All first consumers are implemented, not hidden behind planned skips.
p=Path('docs/plans/phase-registry.json');d=json.loads(p.read_text())
for phase in ('03','04'):
 d['phases'][phase]['status']='in_progress'
 path=next(Path('docs/plans').glob(f'phase-{phase}-*.md'));path.write_text(path.read_text().replace('Status: planned.','Status: in_progress.'))
p.write_text(json.dumps(d,indent=2)+'\n')
with Path('scripts/coverage-bands.conf').open('a') as f:f.write('\ninternal/identity 85\ninternal/auth 85\ninternal/access 85\ninternal/securityapi 85\nsdk/chartworks 80\n')
p=Path('go.mod');s=p.read_text();s=s.replace('require (','require (\n github.com/golang-jwt/jwt/v5 v5.3.1',1);p.write_text(s)
# Production is never compiled with the temporary editing program present after normalization.
