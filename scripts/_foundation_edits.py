from pathlib import Path

def change(path,old,new,count=1):
 p=Path(path);s=p.read_text()
 if s.count(old)!=count: raise SystemExit('expected exact edit count: '+path+' '+old[:80])
 p.write_text(s.replace(old,new))

p='internal/config/config.go'
change(p,'func (d *Duration) UnmarshalJSON','// UnmarshalJSON accepts explicit duration strings only.\nfunc (d *Duration) UnmarshalJSON')
change(p,'func (d Duration) MarshalJSON','// MarshalJSON preserves explicit duration units.\nfunc (d Duration) MarshalJSON')
change(p,'func (c Config) GoString','// GoString prevents detailed formatting from printing a resolved credential.\nfunc (c Config) GoString')
change(p,'func (c Config) MarshalJSON','// MarshalJSON emits detached values and secret references, never credential bytes.\nfunc (c Config) MarshalJSON')
change(p,"if !((c >= 'A' && c <= 'Z') || c == '_' || (i > 0 && c >= '0' && c <= '9')) {\n\t\t\treturn \"\", invalid(\"secret\", \"invalid reference\")\n\t\t}","if (c >= 'A' && c <= 'Z') || c == '_' || (i > 0 && c >= '0' && c <= '9') { continue }; return \"\", invalid(\"secret\", \"invalid reference\")")
change(p,"if !((c >= 'a' && c <= 'z') || c == '_' || c == '.') {\n\t\t\treturn \"document\"\n\t\t}","if (c >= 'a' && c <= 'z') || c == '_' || c == '.' { continue }; return \"document\"")
change(p,"if delim == '{' {","switch {\n case delim == '{':")
change(p,"} else if delim == '[' && depth > 0 {","case delim == '[' && depth > 0:")
change(p,'} else {\n\t\treturn invalid("document", "object required")\n\t}', 'default:\n\t\treturn invalid("document", "object required")\n\t}')
change(p,'if v.Store.MaxConns < 1 || v.Store.MaxConns > 100 {','if v.Store.TransactionTimeout < Duration(time.Millisecond) { return invalid("store.transaction_timeout", "minimum duration is one millisecond") }\n if v.Store.MaxConns < 1 || v.Store.MaxConns > 100 {')
p='internal/store/store.go'
change(p,"if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == ':' || c == '.') {\n\t\t\treturn false\n\t\t}","if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == ':' || c == '.' { continue }; return false")
p='internal/foundation/server.go'
change(p,'if !ok {\n\t\t\tstate = "starting"\n\t\t} else if !v.ValidUntil.IsZero() && !now.Before(v.ValidUntil) {\n\t\t\tstate = "stale"\n\t\t} else if v.Ready {\n\t\t\tstate = "ready"\n\t\t}', 'switch { case !ok: state="starting"; case !v.ValidUntil.IsZero() && !now.Before(v.ValidUntil): state="stale"; case v.Ready: state="ready" }')
p='internal/foundation/keys.go'
change(p,'"crypto/elliptic"','"crypto/ecdh"')
change(p,'var curve elliptic.Curve','var curve ecdh.Curve\n var size int')
change(p,'curve = elliptic.P256()','curve = ecdh.P256(); size=32')
change(p,'curve = elliptic.P384()','curve = ecdh.P384(); size=48')
change(p,'curve = elliptic.P521()','curve = ecdh.P521(); size=66')
change(p,'size := (curve.Params().BitSize + 7) / 8\n\t\t\tif len(x) != size || len(y) != size || !curve.IsOnCurve(new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)) {\n\t\t\t\treturn false\n\t\t\t}', 'if len(x)!=size || len(y)!=size{return false}; point:=append([]byte{4},x...);point=append(point,y...);if _,e:=curve.NewPublicKey(point);e!=nil{return false}')
change('internal/foundation/keys_test.go','http.Redirect(w, r, "https://other.example", 302)','http.Redirect(w, r, "https://other.example", http.StatusFound)')
change('test/acceptance/phase02_test.go','cmd := exec.CommandContext(commandCtx, "python3", args...)','// #nosec G204 -- fixed tool and test-owned temporary paths; no shell or caller input.\n cmd := exec.CommandContext(commandCtx, "python3", args...)')
# Stop capability/history drift from silently keeping readiness green.
p='internal/store/postgres/migrate.go'
change(p,'\t\treturn nil\n\t})','\t\treturn requiredRelations(ctx,tx)\n\t})',count=2)
with Path(p).open('a') as f:f.write('''
// Required relation presence complements history checks, without claiming a superuser-tamper sandbox.
func requiredRelations(ctx context.Context,tx pgx.Tx)error{
 var count int
 if e:=tx.QueryRow(ctx,`SELECT count(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='chartworks' AND c.relkind='r' AND c.relname IN ('schema_migrations','policies','policy_revisions','audit_events','operations')`).Scan(&count);e!=nil{return e};if count!=5{return store.ErrMigration};return nil
}
''')
# Apply documentation orientation without changing the historical source corpus.
p=Path('docs/plans/README.md');s=p.read_text();s=s.replace('All phases are initially **planned**.','Phases **01 and 02 are in implementation verification**; the other 32 remain planned.');p.write_text(s)
p=Path('README.md');s=p.read_text();s+='\n## Phase 01–02 foundation\n\nThe first Go foundation now has strict configuration, lifecycle/health, PostgreSQL metadata migrations and real-store acceptance tests. Business JWT enforcement, analytics and reporting remain later phases. Start with [GETTING-STARTED.md](GETTING-STARTED.md); the [adversarial review](docs/reviews/phase-01-02-adversarial.md) records failure probes and corrections.\n';p.write_text(s)
for n in ('01','02'):
 p=next(Path('docs/plans').glob('phase-'+n+'-*.md'));s=p.read_text();s+='\n## Implementation record — 2026-09-05\n\nThe criterion-to-test mapping is implemented in `test/acceptance/phase'+n+'_test.go`, with shared adversarial cases and real PostgreSQL fixtures. The first foundation is intentionally loopback health-only before phases 03/04; storage scopes are isolation coordinates, not authentication. See D-056, D-057 and D-058, [operator instructions](../../GETTING-STARTED.md), [configuration reference](../configuration.md) and [self-review](../reviews/phase-01-02-adversarial.md). Package coverage uses full-suite cross-package instrumentation at unchanged thresholds. All six criteria must pass without skips before this phase is marked shipped.\n';p.write_text(s)
# The final CI will be read-only; temporarily stage exact document edits during normalization.
p=Path('.github/workflows/foundation-branch.yml');s=p.read_text().replace('git add go.mod go.sum cmd internal test docs/plans','git add go.mod go.sum cmd internal test docs/plans README.md');s=s.replace('go tool cover -func=/tmp/foundation-coverage.out\n          python3','go tool cover -func=/tmp/foundation-coverage.out\n          make coverage\n          make planning-check\n          python3');p.write_text(s)
