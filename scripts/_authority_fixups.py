from pathlib import Path
# Align with the actual inspected Pengui minter's provider-scope limits.
p=Path('internal/identity/identity.go');s=p.read_text().replace('len(scopes)>128','len(scopes)>32').replace('total>8192','total>4096');p.write_text(s)
p=Path('internal/config/config.go');s=p.read_text().replace('MaxScopes:128,MaxScopeBytes:8192','MaxScopes:32,MaxScopeBytes:4096');p.write_text(s)
p=Path('internal/config/auth.go');s=p.read_text().replace('a.MaxScopes>128','a.MaxScopes>32').replace('a.MaxScopeBytes>8192','a.MaxScopeBytes>4096');p.write_text(s)
p=Path('test/acceptance/security_http_helpers_test.go');s=p.read_text().replace('import("net/http";"net/http/httptest")','import("net/http";"net/http/httptest";"testing";"github.com/hurtener/chartworks/internal/auth")');p.write_text(s)
p=Path('test/acceptance/phase03_test.go');s=p.read_text().replace(' "time"\n','').replace('  _ = time.Second\n','');p.write_text(s)
p=Path('test/acceptance/phase04_test.go');s=p.read_text().replace(' "fmt"\n','').replace('  _ = fmt.Sprint\n','');p.write_text(s)
p=Path('test/acceptance/security_negative_test.go');s=p.read_text().replace(' "context"\n','').replace(' ctx,cancel:=context.WithCancel(context.Background());cancel();_ = ctx\n','');p.write_text(s)
# Bound in-process operations too, not just HTTP middleware requests.
p=Path('internal/securityapi/service.go');s=p.read_text();s=s.replace(';return s.repo.Policy(ctx,scope)',';ctx,cancel:=context.WithDeadline(ctx,e.Deadline());defer cancel();return s.repo.Policy(ctx,scope)');s=s.replace(';return s.maintenance.Configure(ctx,scope,expected,p)',';ctx,cancel:=context.WithDeadline(ctx,e.Deadline());defer cancel();return s.maintenance.Configure(ctx,scope,expected,p)');s=s.replace(';return s.repo.Audits(ctx,scope,limit)',';ctx,cancel:=context.WithDeadline(ctx,e.Deadline());defer cancel();return s.repo.Audits(ctx,scope,limit)');s=s.replace(';return s.maintenance.Sweep(ctx,scope,key)',';ctx,cancel:=context.WithDeadline(ctx,e.Deadline());defer cancel();return s.maintenance.Sweep(ctx,scope,key)');p.write_text(s)
# Require the library's positive Valid result as well as its nil error.
p=Path('internal/auth/verifier.go');s=p.read_text().replace('_,err=jwt.Parse(token,','verified,err:=jwt.Parse(token,').replace('if err!=nil||ctx.Err()!=nil{return fail()}','if err!=nil||verified==nil||!verified.Valid||ctx.Err()!=nil{return fail()}');p.write_text(s)
# The shared key parser must accept only kid forms the verifier can actually address.
p=Path('internal/auth/key_validation.go');s=p.read_text().replace('"strings")','"strings";"github.com/hurtener/chartworks/internal/identity")');s=s.replace('len(kid) == 0 || len(kid) > 128 || strings.TrimSpace(kid) != kid','!identity.Identifier(kid)');p.write_text(s)
