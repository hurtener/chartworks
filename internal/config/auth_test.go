package config
import("testing";"time")
func TestAuthConfigurationBounds(t *testing.T){
 base:=Defaults().Auth;base.Issuer="https://issuer.example";base.JWKSURL="https://issuer.example/keys";base.Audience="chartworks"
 if err:=ValidateAuth(base);err!=nil{t.Fatal(err)}
 changes:=[]func(*Auth){func(a *Auth){a.Issuer="http://issuer.example"},func(a *Auth){a.JWKSURL="https://user:secret@host/keys"},func(a *Auth){a.Audience=""},func(a *Auth){a.Audience="a b"},func(a *Auth){a.Audiences.HTTP="other"},func(a *Auth){a.Algorithms=nil},func(a *Auth){a.Algorithms=[]string{"HS256"}},func(a *Auth){a.Algorithms=[]string{"ES256","ES256"}},func(a *Auth){a.RequestTimeout=0},func(a *Auth){a.RefreshInterval=a.JWKSMaxStale},func(a *Auth){a.JWKSMaxStale=Duration(2*time.Hour)},func(a *Auth){a.ClockSkew=-1},func(a *Auth){a.MaxTokenLifetime=0},func(a *Auth){a.MaxTokenBytes=0},func(a *Auth){a.MaxTokenBytes=65537},func(a *Auth){a.MaxClaimBytes=a.MaxTokenBytes+1},func(a *Auth){a.MaxScopes=33},func(a *Auth){a.MaxScopeBytes=4097}}
 for i,change:=range changes{a:=base;change(&a);if ValidateAuth(a)==nil{t.Fatal("invalid auth config",i)}}
 base.Audience="";base.Audiences=Audiences{HTTP:"chartworks:http",MCP:"chartworks:mcp"};if ValidateAuth(base)!=nil||base.HTTPAudience()==base.MCPAudience(){t.Fatal("per-surface audience contract")}
}
