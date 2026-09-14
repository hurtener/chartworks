"""Explicit test-harness corrections, without changing production authority or assertions."""
from pathlib import Path
import subprocess
changed=set()
def replace(path,old,new,count=1):
    p=Path(path);text=p.read_text()
    if new in text:return
    if text.count(old)!=count:raise RuntimeError(f'Unexpected anchor {path}: {old[:140]!r}')
    p.write_text(text.replace(old,new));changed.add(path)

p='test/acceptance/phase30_provenance_test.go'
replace(p,'''import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/reporting"
	"strings"
	"testing"
)''','''import (
    "encoding/json"
    "errors"
    "slices"
    "strings"
    "testing"

    "github.com/hurtener/chartworks/internal/reporting"
    "github.com/hurtener/chartworks/internal/store"
)''')
replace(p,'''job, err := f.queue.TestSchedule(t.Context(), f.manager(t), schedule.ID, "provenance-occurrence", schedule.Revision)''','''manager := f.manager(t)
            if kind == "report" {
                // Report publications explicitly retain dependency read reach.
                // Schedule permissions must not replace that signed consent.
                beforeQueries, beforeBroker := f.domain.attemptCount(t), f.calls.Load()
                if _, err := f.queue.TestSchedule(t.Context(),manager,schedule.ID,"missing-dependency-consent",schedule.Revision); !errors.Is(err,store.ErrNotFound) {
                    t.Fatal("schedule management bypassed the report dependency graph",err)
                }
                if beforeQueries!=f.domain.attemptCount(t) || beforeBroker!=f.calls.Load() {t.Fatal("rejected test performed protected work")}
                scopes := slices.DeleteFunc(phase30ManagementScopes(f),func(s string)bool{return s=="scheduling.cancel" || s=="cw.run.write:*"})
                scopes = append(scopes,"cw.block.read:p30-provenance-block")
                if len(scopes)>32 {t.Fatal("fixture enlarged issuer scope ceiling")}
                manager = phase27Actor(t,f.domain.f,f.actor.User(),scopes)
            }
            job, err := f.queue.TestSchedule(t.Context(), manager, schedule.ID, "provenance-occurrence", schedule.Revision)''')

p='web/report-viewer/component.test.mjs'
replace(p,'window.resultOverride=null;window.hostReady=false;', 'window.resultOverride=null;window.resultOverrideTool=null;window.hostReady=false;')
replace(p,'if(window.resultOverride){result=window.resultOverride;window.resultOverride=null;}', 'if(window.resultOverride&&(!window.resultOverrideTool||window.resultOverrideTool===m.params.name)){result=window.resultOverride;window.resultOverride=null;window.resultOverrideTool=null;}')
replace(p,"const restore=async()=>{await evaluate('show(fixture.view)');await waitTitle(fixtures.view.summary.target.id);};",'''let restoreSequence=0;
// Posting a notification is asynchronous. A unique rendered marker proves the
// resource accepted a new generation before installing the next hostile reply;
// merely waiting for the unchanged fixture title can race an old tool response.
const restore=async()=>{
  const marker='restore-'+(++restoreSequence);
  await evaluate(`(()=>{window.resultOverride=null;window.resultOverrideTool=null;const v=JSON.parse(JSON.stringify(fixture.view));v.summary.target.id=${JSON.stringify(marker)};show(v);})()`);
  await waitTitle(marker);
  await evaluate('show(fixture.view)');
  await waitTitle(fixtures.view.summary.target.id);
};''')
replace(p,'await evaluate(`const d=JSON.parse(JSON.stringify(fixture.description));','await evaluate(`(()=>{const d=JSON.parse(JSON.stringify(fixture.description));')
replace(p,"window.resultOverride={structuredContent:{result:d},content:[]};Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Run with these filters').click();`);", "window.resultOverride={structuredContent:{result:d},content:[]};window.resultOverrideTool='reporting_describe';Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Run with these filters').click();})()`);")
replace(p,'await restore();await evaluate(`const v=JSON.parse(JSON.stringify(fixture.view));v.selection.', 'await restore();await evaluate(`(()=>{const v=JSON.parse(JSON.stringify(fixture.view));v.selection.')
replace(p,"\"'different-run'\":\"'report'\"};show(v);`);", "\"'different-run'\":\"'report'\"};show(v);})()`);")
replace(p,"await restore();await evaluate(`const v=JSON.parse(JSON.stringify(fixture.view));v.summary.target.id='_valid-coordinate';v.filters[0].parameter.name='_valid_parameter';show(v);`);", "await restore();await evaluate(`(()=>{const v=JSON.parse(JSON.stringify(fixture.view));v.summary.target.id='_valid-coordinate';v.filters[0].parameter.name='_valid_parameter';show(v);})()`);")
subprocess.run(['gofmt','-w',*sorted(p for p in changed if p.endswith('.go'))],check=True)
subprocess.run(['node','--check','web/report-viewer/component.test.mjs'],check=True)
subprocess.run(['git','diff','--check'],check=True)
