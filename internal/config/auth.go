package config

import(
 "strings"
 "time"
)
// Audiences pins the intended resource on each transport; values are never taken from a token.
type Audiences struct{HTTP string `json:"http"`;MCP string `json:"mcp"`}
// HTTPAudience resolves the explicit per-surface setting or the existing single-audience shorthand.
func(a Auth) HTTPAudience()string{if a.Audiences.HTTP!=""{return a.Audiences.HTTP};return a.Audience}
// MCPAudience resolves the explicit MCP audience, not the HTTP value implicitly.
func(a Auth) MCPAudience()string{if a.Audiences.MCP!=""{return a.Audiences.MCP};return a.Audience}
// ValidateAuth is also used by direct verifier construction so no caller can skip its bounds.
func ValidateAuth(a Auth)error{
 if !secureURL(a.Issuer)||!secureURL(a.JWKSURL){return invalid("auth","trusted HTTPS issuer and key URL required")}
 if a.Audience!=""&&(a.Audiences.HTTP!=""||a.Audiences.MCP!=""){return invalid("auth.audiences","cannot mix shorthand and per-surface audiences")}
 for _,s:=range []string{a.HTTPAudience(),a.MCPAudience()}{if len(s)==0||len(s)>512||strings.ContainsAny(s," \r\n\t"){return invalid("auth.audiences","both bounded intended audiences are required")}}
 if len(a.Algorithms)==0||len(a.Algorithms)>6{return invalid("auth.algorithms","asymmetric allowlist required")}
 seen:=map[string]bool{};for _,s:=range a.Algorithms{switch s{case "RS256","RS384","RS512","ES256","ES384","ES512":default:return invalid("auth.algorithms","unsupported algorithm")};if seen[s]{return invalid("auth.algorithms","duplicate algorithm")};seen[s]=true}
 if a.RequestTimeout<=0||a.RequestTimeout>Duration(time.Minute)||a.RefreshInterval<=0||a.RefreshInterval>=a.JWKSMaxStale||a.JWKSMaxStale>Duration(time.Hour){return invalid("auth","invalid key refresh bounds")}
 if a.ClockSkew<0||a.ClockSkew>Duration(time.Minute)||a.MaxTokenLifetime<Duration(time.Second)||a.MaxTokenLifetime>Duration(24*time.Hour){return invalid("auth","invalid temporal bounds")}
 if a.MaxTokenBytes<1024||a.MaxTokenBytes>65536||a.MaxClaimBytes<512||a.MaxClaimBytes>a.MaxTokenBytes||a.MaxScopes<1||a.MaxScopes>128||a.MaxScopeBytes<1||a.MaxScopeBytes>8192{return invalid("auth","invalid claim size bounds")}
 return nil
}
