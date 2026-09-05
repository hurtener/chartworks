from pathlib import Path
import re

def replace(path,old,new):
 p=Path(path);s=p.read_text();assert old in s,(path,old);p.write_text(s.replace(old,new))
for name in ('phase04_test.go','security_negative_test.go'):
 replace('test/acceptance/'+name,'telemetry.New(io.Discard, "json")','telemetry.New(io.Discard, "json", true)')
# Cross-package structs use named fields so changes to their contract cannot silently reorder tests.
p=Path('test/acceptance/phase04_test.go');s=p.read_text();s=re.sub(r'\{\s*"([^"]*)",\s*"([^"]*)",\s*"([^"]*)",\s*"([^"]*)"\s*\}',lambda m:'{Tenant: "'+m[1]+'", Kind: "'+m[2]+'", Permission: "'+m[3]+'", ID: "'+m[4]+'"}',s);p.write_text(s)
# Keep in-process selections bounded by the same immutable token snapshot.
p=Path('internal/access/access.go');s=p.read_text().replace('type Selection struct {','type Selection struct {\n authority identity.Envelope');s=s.replace('func (s Selection) Tenant() string { return s.tenant }','func (s Selection) Tenant() string { if !s.authority.Valid(){return ""};return s.tenant }');s=s.replace('func (s Selection) IDs() []string { return append([]string(nil), s.ids...) }','func (s Selection) IDs() []string {if !s.authority.Valid(){return nil};return append([]string(nil), s.ids...) }');s=s.replace('return s.all && s.tenant != ""','return s.authority.Valid() && s.all && s.tenant != ""');s=s.replace('if s.tenant == "" || tenant != s.tenant','if !s.authority.Valid() || s.tenant == "" || tenant != s.tenant');s=s.replace('Selection{tenant: e.Tenant()}','Selection{tenant: e.Tenant(),authority:e}');p.write_text(s)
# Prepared SDK performs bounded reads of metrics without changing the JSON wire type of domain results.
p=Path('sdk/chartworks/client.go');s=p.read_text();old='if err != nil || len(data) > 1<<20 || json.Unmarshal(data, out) != nil {';assert old in s;s=s.replace(old,'if err == nil && len(data)<=1<<20 { if text,ok:=out.(*string);ok { *text=string(data);return nil } }\n'+old);p.write_text(s)
# Body-reading errors keep the connection closed rather than consuming an unbounded remainder.
replace('internal/securityapi/http.go','if err != nil || len(b) > 4096 {','if err != nil || len(b) > 4096 {\n w.Header().Set("Connection","close")')
# No successful empty response can be introduced by registry drift.
p=Path('internal/securityapi/http.go');s=p.read_text();needle='respond(w, cw.Operation{ID: result.ID, Status: result.Status, PolicyRevision: result.PolicyRevision, Cutoff: result.Cutoff, Limit: result.Limit, DeletedEvents: result.DeletedEvents, DeletedOperations: result.DeletedOperations})';assert needle in s;s=s.replace(needle,needle+'\n default: failure(w,access.ErrNotFound)');p.write_text(s)
# Actual command now has mandatory JWT verification for operational endpoints.
p=Path('scripts/smoke_foundation.py');s=p.read_text().replace('body.get("authentication") is not False','body.get("authentication") is not True').replace('unimplemented business/auth capability advertised','incorrect implemented authority capability').replace(')[0] != 404:',')[0] != 401:');p.write_text(s)
# Remove unused helper (all actual tests use bounded SDK/recorder reads).
p=Path('test/acceptance/security_support_test.go');s=p.read_text();start=s.find('func bodyText(')
if start>=0:s=s[:start];s=s.replace('\n\t"io"','')
p.write_text(s)
