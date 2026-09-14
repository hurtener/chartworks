"""Explicit final boundary edits. Remove this aid before the reviewed PR."""
from pathlib import Path
import subprocess
changed=set()
def replace(path,old,new,count=1):
    p=Path(path); text=p.read_text()
    if new in text:return
    if text.count(old)!=count:raise RuntimeError(f'Unexpected anchor in {path}: {old[:140]!r}')
    p.write_text(text.replace(old,new));changed.add(path)
def write(path,content):
    p=Path(path)
    if p.exists():
        if p.read_text()!=content:raise RuntimeError(f'Refusing unexpected overwrite {path}')
        return
    p.write_text(content);subprocess.run(['git','add','-N','--',path],check=True);changed.add(path)

# Preserve every old migration identity, and explicitly test the new migrations.
replace('internal/store/postgres/errors_test.go','SchemaVersion() != "30" || len(manifest) != 30','SchemaVersion() != "33" || len(manifest) != 33')
replace('internal/store/postgres/errors_test.go','{29, "migrations/030_report_composition_runs.sql", "composition_group_guard"},','''{29, "migrations/030_report_composition_runs.sql", "composition_group_guard"},
        {30, "migrations/031_nested_report_leases.sql", "nested_child_shape"},
        {31, "migrations/032_timezone_database.sql", "timezone_database_version"},
        {32, "migrations/033_reporting_occurrences.sql", "reporting_occurrence_delivery"},''')

# Storage-outage fixtures must model a storage failure, not a violated invariant.
# Corrupted retained manifests remain attention/blocked, not transient retries.
replace('internal/reporting/scheduled.go','''	// A rejected checkpoint transaction is not a changed business definition.
	// Retrying preserves the accepted manifest and lets the existing consumer
	// reconcile retained evidence; an uncheckpointed query remains incomplete.
	if errors.Is(err, store.ErrInvalid) {
		return errors.Join(jobs.ErrTransient, err)
	}
''','''    // Storage outages may retry; invalid retained evidence or violated domain
    // invariants require attention. Never retry by replacing accepted pins.
''')
replace('internal/reporting/scheduled_errors_test.go','{store.ErrInvalid, jobs.ErrTransient}','{store.ErrInvalid, jobs.ErrReportingAttention}')
for path,kind in [('test/acceptance/phase30_operations_test.go','normalized checkpoint'),('test/acceptance/phase30_delivery_test.go','catalog publication')]:
    replace(path,f"synthetic {kind} failure' USING ERRCODE='55000'",f"synthetic {kind} failure' USING ERRCODE='58000'")

# Call identity stays opaque, but reservations cannot be borrowed by another
# actor/session in the same tenant. This does not construct or expand authority.
replace('internal/gateway/gateway.go','func (c Call) Tenant() string { return c.envelope.Tenant() }','''func (c Call) Tenant() string { return c.envelope.Tenant() }

// MatchesIdentity binds a durable reservation to the current verified principal.
// It exposes no authority bytes and grants no additional resource permissions.
func (c Call) MatchesIdentity(e identity.Envelope) bool {
    return c.Valid() && e.Valid() && c.envelope.Tenant()==e.Tenant() &&
        c.envelope.User()==e.User() && c.envelope.Session()==e.Session()
}''')
replace('internal/reporting/scheduled.go','if !call.Valid() || call.Tenant() != j.Tenant {','if !call.MatchesIdentity(e) {')
write('internal/gateway/reservation_identity_test.go','''package gateway

import (
    "testing"
    "time"
    "github.com/hurtener/chartworks/internal/access"
    "github.com/hurtener/chartworks/internal/identity"
)

func TestReservationMatchesExactCurrentIdentity(t *testing.T) {
    now:=time.Now()
    envelope:=func(tenant,user,session string)identity.Envelope{
        t.Helper()
        e,err:=identity.FromVerified(tenant,user,session,[]string{"reporting.execute","cw.block.execute:block"},now.Add(time.Minute),func()time.Time{return now})
        if err!=nil{t.Fatal(err)}
        return e
    }
    original:=envelope("tenant","svc:reporting","occurrence")
    call,err:=Authorize(original,"reporting.execute","partition",access.Resource{Tenant:"tenant",Kind:"block",Permission:"execute",ID:"block"})
    if err!=nil{t.Fatal(err)}
    if !call.MatchesIdentity(original)||!call.MatchesIdentity(envelope("tenant","svc:reporting","occurrence")){t.Fatal("exact current principal rejected")}
    for _,other:=range []identity.Envelope{envelope("other","svc:reporting","occurrence"),envelope("tenant","other","occurrence"),envelope("tenant","svc:reporting","other"),{}}{
        if call.MatchesIdentity(other){t.Fatal("reservation identity widened")}
    }
    if (Call{}).MatchesIdentity(original){t.Fatal("zero call accepted")}
    now=now.Add(2*time.Minute)
    if call.MatchesIdentity(original){t.Fatal("expired principal accepted")}
}
''')

# Bound both catalog implementations; no partial metadata accompanies refusal.
replace('internal/reporting/delivery.go','''	if in.Kind != "block" {
		return s.catalog.ListCompositionArtifacts(ctx, e, in.Kind, in.Resource, in.After, in.Limit)
	}''','''	if in.Kind != "block" {
        result, err := s.catalog.ListCompositionArtifacts(ctx,e,in.Kind,in.Resource,in.After,in.Limit)
        if err != nil { return ReportingRunsResult{},err }
        if err := s.bound(result); err != nil { return ReportingRunsResult{},err }
        return result,ctx.Err()
    }''')
replace('internal/reporting/delivery.go','''			out.Items = append(out.Items, blockRunSummary(v))
		}
	}
	return out, ctx.Err()''','''			out.Items = append(out.Items, blockRunSummary(v))
		}
	}
    if err := s.bound(out); err != nil { return ReportingRunsResult{},err }
	return out, ctx.Err()''')
write('test/acceptance/phase31_catalog_bounds_test.go','''package acceptance

import (
    "encoding/json"
    "errors"
    "fmt"
    "strings"
    "testing"
    "github.com/hurtener/chartworks/internal/config"
    "github.com/hurtener/chartworks/internal/reporting"
)

func testPhase31CatalogBounds(t *testing.T) {
    for _,kind:=range []string{"block","report"}{
        t.Run(kind,func(t *testing.T){
            f:=newPhase31Fixture(t,false)
            id:="p31-catalog-"+strings.Repeat("x",108)
            if kind=="block"{f.domain.block(t,id,f.domain.base)}else{f.domain.report(t,id,phase29Text("Bounded retained catalog"),true)}
            for n:=0;n<64;n++{f.run(t,kind,id,fmt.Sprintf("catalog-%02d",n))}
            limits:=config.DefaultReportingViewer();limits.MaxMessageBytes=16384
            bounded,err:=reporting.NewDelivery(f.domain.blocks,f.domain.runs,f.domain.documents,f.domain.compositions,f.domain.f.f.db,limits)
            if err!=nil{t.Fatal(err)}
            beforeQueries,beforeModels:=f.domain.attemptCount(t),f.domain.f.model.requests.Load()
            request:=reporting.ReportingRunsRequest{Kind:kind,Resource:id,Limit:64}
            baseline,err:=f.service.Runs(t.Context(),f.domain.execute,request)
            wire,encodeErr:=json.Marshal(baseline)
            if err!=nil||encodeErr!=nil||len(baseline.Items)!=64||len(wire)<=limits.MaxMessageBytes{t.Fatal("fixture did not exceed the actual serialized bound",len(wire),len(baseline.Items),err,encodeErr)}
            refused,err:=bounded.Runs(t.Context(),f.domain.execute,request)
            if !errors.Is(err,reporting.ErrBudget)||len(refused.Items)!=0||refused.Next!=""{t.Fatal("catalog byte limit was advisory or leaked partial metadata",refused,err)}
            request.Limit=1
            page,err:=bounded.Runs(t.Context(),f.domain.execute,request)
            if err!=nil||len(page.Items)!=1||page.Next==""{t.Fatal("bounded continuation failed",page,err)}
            if beforeQueries!=f.domain.attemptCount(t)||beforeModels!=f.domain.f.model.requests.Load(){t.Fatal("catalog paging executed source/model")}
        })
    }
}
''')
replace('test/acceptance/phase31_test.go','t.Run("provider-boundaries", testPhase31ProviderBounds)','''t.Run("provider-boundaries", testPhase31ProviderBounds)
        t.Run("retained-catalog-byte-ceiling",testPhase31CatalogBounds)''')

# An explicit filter rerun preserves the original admitted block trust policy,
# rather than silently turning certified-only work into published-only work.
replace('internal/reporting/runs_model.go','type RunView struct {','type RunView struct {\n    Policy string `json:"policy,omitempty"`')
replace('internal/reporting/delivery_model.go','type ReportingViewResult struct {','type ReportingViewResult struct {\n    Policy string `json:"policy,omitempty"`')
replace('internal/store/postgres/frozen_runs_read.go','out.View.Trust = m.Trust','out.View.Trust = m.Trust\n    out.View.Policy = m.Policy')
replace('internal/reporting/delivery_view.go','out.Summary = blockRunSummary(v)','out.Summary = blockRunSummary(v)\n    out.Policy = v.Policy')
replace('web/report-viewer/app.js',"policy:'',", "policy:v.summary.kind==='block'?v.policy:'',")
replace('web/report-viewer/app.js',"if(!id(description.resource?.target?.id)","if(v.summary.kind==='block'&&!['published','certified_only'].includes(v.policy))throw fail('stale_validation');\n        if(!id(description.resource?.target?.id)")
replace('test/viewerfixtures/fixtures.go','out.View = reporting.ReportingViewResult{','out.View = reporting.ReportingViewResult{\n        Policy: "certified_only",')
replace('test/viewerfixtures/fixtures.go','Certification: "uncertified",','Certification: "certified",')
replace('web/report-viewer/component.test.mjs',"  if(suite==='all'||suite==='security'){",'''  if(suite==='all'||suite==='security'){
    await restore();const policyRunCount=await evaluate("calls.filter(c=>c.name==='reporting_run').length");
    await evaluate(`Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Run with these filters').click();`);
    await until(()=>evaluate(`calls.filter(c=>c.name==='reporting_run').length===${policyRunCount+1}`),'certified filter run was not emitted');
    await check("calls.filter(c=>c.name==='reporting_run').at(-1).arguments.policy==='certified_only'",'filter rerun preserves the admitted trust requirement');
''')
# Existing provenance test now covers the actual certified path too, with no
# fake publication or attestation object.
replace('test/acceptance/phase30_provenance_test.go','[]string{"saved_sql", "report"}','[]string{"saved_sql", "block", "report"}')
replace('test/acceptance/phase30_provenance_test.go','f.domain.block(t, "p30-provenance-block", f.domain.base)','''if kind=="block" { f.certify(t,"p30-provenance-block") } else { f.domain.block(t, "p30-provenance-block", f.domain.base) }''')
replace('test/acceptance/phase30_provenance_test.go','p := view.Summary.Scheduled','''if resource=="block" {
                wantPolicy:="published";if kind=="block"{wantPolicy="certified_only"}
                if view.Policy!=wantPolicy{t.Fatal("viewer lost the admitted trust policy",view.Policy)}
            }
            p := view.Summary.Scheduled''')

# Include all newly shipped production packages in the existing strict gate.
# Existing thresholds are untouched, including the owner-approved store band.
replace('scripts/coverage_gate.py','(?:internal|cmd|sdk|eval)(?:/[A-Za-z0-9_-]+)+','(?:internal|cmd|sdk|eval|web)(?:/[A-Za-z0-9_-]+)+')
replace('scripts/coverage_gate.py','("internal", "cmd", "sdk", "eval")','("internal", "cmd", "sdk", "eval", "web")')
replace('scripts/coverage-bands.conf','internal/chartdata 80','internal/chartdata 80\ninternal/calendars 80\nweb/report-viewer 80')
replace('scripts/test_coverage_gate.py','    def test_decimal_bands_are_exact_basis_points(self):','''    def test_viewer_is_a_production_coverage_band(self):
        self.assertEqual(bands("web/report-viewer 80"), {"web/report-viewer": 8000})
        self.assertEqual(measure("mode: atomic\\nexample/web/report-viewer/resource.go:1.1,2.1 2 1\\n", "example", {"web/report-viewer"}), {"web/report-viewer": (2, 2)})

    def test_decimal_bands_are_exact_basis_points(self):''')
replace('internal/config/jobs.go','''	"github.com/hurtener/chartworks/internal/calendars"
	"net/url"
	"strings"
	"time"''','''	"net/url"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/calendars"''')
if changed:subprocess.run(['gofmt','-w',*sorted(p for p in changed if p.endswith('.go'))],check=True)
subprocess.run(['node','--check','web/report-viewer/app.js'],check=True)
subprocess.run(['node','--check','web/report-viewer/component.test.mjs'],check=True)
subprocess.run(['python3','-m','unittest','discover','-s','scripts','-p','test_coverage_gate.py'],check=True)
subprocess.run(['git','diff','--check'],check=True)
