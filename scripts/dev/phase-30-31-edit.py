"""Explicit adversarial fixes; remove this branch-only aid before final PR."""
from pathlib import Path
import subprocess
changed = set()
def replace(path, old, new, count=1):
    p=Path(path); text=p.read_text()
    if new in text: return
    if text.count(old)!=count: raise RuntimeError(f'Unexpected anchor in {path}: {old[:140]!r}')
    p.write_text(text.replace(old,new)); changed.add(path)
def write(path, content):
    p=Path(path)
    if p.exists():
        if p.read_text()!=content: raise RuntimeError(f'Refuse unexpected overwrite: {path}')
        return
    p.write_text(content); subprocess.run(['git','add','-N','--',path],check=True); changed.add(path)

p='web/report-viewer/app.js'
replace(p,"/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/", "/^[A-Za-z0-9_.:-]{1,128}$/")
replace(p,"if(!integer(v.page_bounds?.offset,0,100000)", "if(v.selection.run!==v.summary.run||v.selection.kind!==v.summary.kind||v.summary.target?.kind!==v.summary.kind||!id(v.summary.target?.id)||!integer(v.summary.target?.revision,1,256))throw fail('invalid_request');\n    if(!integer(v.page_bounds?.offset,0,100000)")
replace(p,"if(!id(description.resource?.target?.id)||description.resource.target.revision!==v.summary.target.revision)throw fail('stale_validation');", "if(!id(description.resource?.target?.id)||description.resource.target.id!==v.summary.target.id||description.resource.target.kind!==v.summary.target.kind||description.resource.target.revision!==v.summary.target.revision)throw fail('stale_validation');")
replace(p,"if(v.summary.code)meta.append(element('p',text(v.summary.code)));this.root.append(meta);", "if(v.summary.scheduled){const s=v.summary.scheduled;meta.append(element('p',`${text(s.schedule_id)} · ${text(s.due_at)} · [${text(s.window_start)}, ${text(s.window_end)})`),element('p',`query: ${text(s.query)} · artifact: ${text(s.artifact)} · catalog: ${text(s.catalog)} · notification: ${text(s.notification)}`));}\n    if(v.summary.code)meta.append(element('p',text(v.summary.code)));this.root.append(meta);")
p='web/report-viewer/component.test.mjs'
replace(p,"  if(suite==='all'||suite==='security'){", """  if(suite==='all'||suite==='security'){
    // Corrupted/misrouted provider responses must not change the resource that
    // the user explicitly selected. These run in the actual iframe component.
    await restore();const initialRuns=await evaluate("calls.filter(c=>c.name==='reporting_run').length");
    for(const field of ['id','kind']){
      await restore();
      await evaluate(`const d=JSON.parse(JSON.stringify(fixture.description));d.resource.target.${field}=${field==='id'?"'different-resource'":"'report'"};window.resultOverride={structuredContent:{result:d},content:[]};Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Run with these filters').click();`);
      await until(()=>evaluate(`${body}.textContent.includes('stale_validation')`),'mismatched describe response was accepted');
      await check(`calls.filter(c=>c.name==='reporting_run').length===${initialRuns}`,'description identity cannot redirect a mutation');
      await check(`${body}.querySelector('table,svg')===null`,'mismatched description clears analytical values');
    }
    for(const field of ['run','kind']){
      await restore();await evaluate(`const v=JSON.parse(JSON.stringify(fixture.view));v.selection.${field}=${field==='run'?"'different-run'":"'report'"};show(v);`);
      await until(()=>evaluate(`${body}.textContent.includes('invalid_request')`),'inconsistent artifact identity rendered');
      await check(`${body}.querySelector('table,svg')===null`,'selection and summary must identify the same artifact');
    }
    await restore();await evaluate(`const v=JSON.parse(JSON.stringify(fixture.view));v.summary.target.id='_valid-coordinate';v.filters[0].parameter.name='_valid_parameter';show(v);`);await waitTitle('_valid-coordinate');
    await check(`${body}.querySelector('fieldset')!==null`,'viewer preserves the canonical opaque identifier grammar');
""")

write('internal/reporting/scheduled_provenance.go', '''package reporting

import "time"

// ScheduledProvenance is content-free metadata from the accepted occurrence.
// It exposes neither issuer bindings, recipients, actor identities, parameters,
// SQL nor tokens. Delivery stages are independent: retained query values do not
// imply catalog publication or any outbound notification. This is not a grant.
type ScheduledProvenance struct {
    ScheduleID string `json:"schedule_id,omitempty"`
    ScheduleRevision int64 `json:"schedule_revision,omitempty"`
    DueAt time.Time `json:"due_at"`
    WindowStart time.Time `json:"window_start"`
    WindowEnd time.Time `json:"window_end"`
    Execution string `json:"execution"`
    Query string `json:"query"`
    Artifact string `json:"artifact"`
    Catalog string `json:"catalog"`
    Notification string `json:"notification"`
    PublishedAt *time.Time `json:"published_at,omitempty"`
}
''')
for path, name in [('internal/reporting/runs_model.go','RunView'),('internal/reporting/compositions_model.go','CompositionView'),('internal/reporting/delivery_model.go','ReportingRunSummary')]:
    replace(path, 'type '+name+' struct {', 'type '+name+' struct {\n    Scheduled *ScheduledProvenance `json:"scheduled,omitempty"`')
replace('internal/reporting/delivery.go', 'Created: v.Created, Expires: v.Expires}', 'Created: v.Created, Expires: v.Expires, Scheduled: clone(v.Scheduled)}', count=2)
write('internal/store/postgres/reporting_provenance.go', '''package postgres

import (
    "context"
    "errors"
    "time"
    "github.com/hurtener/chartworks/internal/reporting"
    "github.com/jackc/pgx/v5"
)

// scheduledProvenanceTx is called only after the existing artifact/read-context
// checks in frozenReadTx or the non-redacted compositionViewTx. A schedule ID is
// never accepted as a substitute for artifact permission. No payload is read.
func scheduledProvenanceTx(ctx context.Context, tx pgx.Tx, tenant, id, artifactState string, expires time.Time) (*reporting.ScheduledProvenance, error) {
    out := new(reporting.ScheduledProvenance)
    err := tx.QueryRow(ctx, `SELECT COALESCE(o.schedule_id,''),COALESCE(o.schedule_revision,0),o.due_at,o.window_start,o.window_end,o.status,
      d.query_state,d.artifact_state,d.catalog_state,d.notification_state,d.published_at
      FROM chartworks.operations o JOIN chartworks.reporting_occurrence_delivery d USING(tenant_id,operation_id)
      WHERE o.tenant_id=$1 AND o.operation_id=$2 AND o.kind='reporting.scheduled' AND o.dispatch_mode='queued'`, tenant,id).Scan(
      &out.ScheduleID,&out.ScheduleRevision,&out.DueAt,&out.WindowStart,&out.WindowEnd,&out.Execution,
      &out.Query,&out.Artifact,&out.Catalog,&out.Notification,&out.PublishedAt)
    if errors.Is(err,pgx.ErrNoRows) { return nil,nil }
    if err!=nil { return nil,err }
    switch artifactState {
    case "normalized": out.Query="succeeded"
    case "succeeded","completed": out.Query,out.Artifact="succeeded","retained"
    case "partial": out.Query,out.Artifact="partial","retained"
    }
    if out.Catalog=="pending" && (out.Execution=="blocked" || out.Execution=="failed" || out.Execution=="cancelled" || out.Execution=="expired") {
        out.Catalog="unavailable"
    }
    if artifactState=="expired" || !time.Now().Before(expires) { out.Artifact,out.Catalog="expired","expired" }
    return out,nil
}
''')
replace('internal/store/postgres/frozen_runs_read.go',
        '\tout.View.QueryAttempts, err = frozenAttemptsTx(ctx, tx, e.Tenant(), h)',
        '''\tout.View.Scheduled, err = scheduledProvenanceTx(ctx, tx, e.Tenant(), id, h.view.State, h.view.Expires)
    if err != nil { return reporting.RunRecord{}, err }
\tout.View.QueryAttempts, err = frozenAttemptsTx(ctx, tx, e.Tenant(), h)''')
replace('internal/store/postgres/compositions_read.go',
        '''\tif !e.Valid() {
\t\treturn reporting.CompositionView{}, access.ErrUnauthenticated
\t}''',
        '''    rows.Close()
    if !v.Redacted {
        v.Scheduled, err = scheduledProvenanceTx(ctx, tx, e.Tenant(), v.ID, v.State, v.Expires)
        if err != nil { return reporting.CompositionView{}, err }
    }
\tif !e.Valid() {
\t\treturn reporting.CompositionView{}, access.ErrUnauthenticated
\t}''')
write('test/acceptance/phase30_provenance_test.go', '''package acceptance

import (
    "encoding/json"
    "strings"
    "testing"
    "github.com/hurtener/chartworks/internal/reporting"
)

func testPhase30CatalogProvenance(t *testing.T) {
    for _, kind := range []string{"saved_sql","report"} {
        t.Run(kind,func(t *testing.T) {
            f:=newPhase30Fixture(t,false)
            f.domain.block(t,"p30-provenance-block",f.domain.base)
            id:="p30-provenance-block"
            resource:="block"
            if kind=="report" {
                doc:=phase29Text("Public occurrence provenance")
                doc.Widgets=append(doc.Widgets,phase29BlockWidget("frozen",id,1,"table-main"))
                id="p30-provenance-report"
                f.domain.report(t,id,doc,true)
                resource="report"
            }
            target:=phase30Target(kind,id)
            target.Recipients=[]string{"PRIVATE_RECIPIENT_CANARY"}
            schedule:=f.schedule(t,"provenance-schedule",phase30Manual(target))
            job,err:=f.queue.TestSchedule(t.Context(),f.manager(t),schedule.ID,"provenance-occurrence",schedule.Revision)
            if err!=nil { t.Fatal(err) }
            if err=f.queue.RunOnce(t.Context());err!=nil {t.Fatal(err)}
            beforeQueries,beforeModels:=f.domain.attemptCount(t),f.domain.f.model.requests.Load()
            selection:=reporting.ReportingViewRequest{Kind:resource,Run:job.ID,Output:"table-main",Limit:10}
            if resource=="report" {selection.Page,selection.Widget="main","frozen"}
            view,err:=f.delivery.View(t.Context(),f.domain.execute,selection)
            if err!=nil || view.Output==nil || view.Summary.Scheduled==nil {t.Fatal("ordinary viewer lost scheduled provenance",view,err)}
            p:=view.Summary.Scheduled
            if p.ScheduleID!=schedule.ID || p.ScheduleRevision!=schedule.Revision || !p.DueAt.Equal(job.DueAt) || !p.WindowStart.Equal(job.WindowStart) || !p.WindowEnd.Equal(job.WindowEnd) ||
                p.Execution!="succeeded" || p.Query!="succeeded" || p.Artifact!="retained" || p.Catalog!="available" || p.Notification!="not_requested" || p.PublishedAt==nil {
                t.Fatal("occurrence/delivery evidence changed",p)
            }
            page,err:=f.delivery.Runs(t.Context(),f.domain.execute,reporting.ReportingRunsRequest{Kind:resource,Resource:id,Limit:10})
            if err!=nil || len(page.Items)!=1 || page.Items[0].Run!=job.ID || page.Items[0].Scheduled==nil {t.Fatal("ordinary catalog lost scheduled provenance",page,err)}
            want,_:=json.Marshal(p); got,_:=json.Marshal(page.Items[0].Scheduled)
            if string(want)!=string(got) {t.Fatal("viewer/catalog occurrence evidence diverged")}
            wire,_:=json.Marshal(view)
            for _,secret:=range []string{"PRIVATE_RECIPIENT_CANARY",`"binding_id"`,`"executor"`,`"authorization"`} {
                if strings.Contains(string(wire),secret) {t.Fatal("scheduling authority/recipient details entered the viewer")}
            }
            reader:=phase27Actor(t,f.domain.f,"without-source-context",[]string{"reporting.read","cw."+resource+".read:"+id})
            hidden,err:=f.delivery.View(t.Context(),reader,selection)
            if err==nil && (hidden.Output!=nil || hidden.Summary.Scheduled!=nil) {t.Fatal("hidden context exposed values or occurrence-derived status",hidden)}
            if beforeQueries!=f.domain.attemptCount(t) || beforeModels!=f.domain.f.model.requests.Load() {t.Fatal("provenance read executed source/model")}
        })
    }
}
''')
replace('test/acceptance/phase30_delivery_test.go', 'func testPhase30Delivery(t *testing.T) {', 'func testPhase30Delivery(t *testing.T) {\n\tt.Run("ordinary-catalog-and-viewer-scheduled-provenance",testPhase30CatalogProvenance)')
if changed: subprocess.run(['gofmt','-w',*sorted(p for p in changed if p.endswith('.go'))],check=True)
subprocess.run(['node','--check','web/report-viewer/app.js'],check=True)
subprocess.run(['node','--check','web/report-viewer/component.test.mjs'],check=True)
subprocess.run(['git','diff','--check'],check=True)
