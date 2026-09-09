# Temporary, guarded seam edits for this development branch only.
from pathlib import Path
import re

def replace_once(path, old, new):
    p=Path(path);s=p.read_text()
    if new in s:return
    if s.count(old)!=1:raise SystemExit(f'{path}: expected one guarded seam: {old[:80]}')
    p.write_text(s.replace(old,new,1))

def import_path(path, module):
    p=Path(path);s=p.read_text()
    if f'"{module}"' not in s:
        if 'import (\n' not in s:raise SystemExit(f'{path}: no grouped imports')
        p.write_text(s.replace('import (\n',f'import (\n\t"{module}"\n',1))

config=Path('internal/config/config.go');s=config.read_text()
if 'Reporting    Reporting' not in s and not re.search(r'\bReporting\s+Reporting\s+`json:"reporting"`',s):
    s=s.replace('type Values struct {','type Values struct {\n\tReporting Reporting `json:"reporting"`',1)
if 'Reporting: DefaultReporting(),' not in s and not re.search(r'\bReporting:\s+DefaultReporting\(\),',s):
    s,n=re.subn(r'(\bCharts:\s+DefaultCharts\(\),)',r'\1\n\t\tReporting: DefaultReporting(),',s,count=1)
    if n!=1:raise SystemExit('config defaults seam changed')
if 'v.Reporting.validate()' not in s:
    needle='\tif err := v.Charts.validate(); err != nil {\n\t\treturn err\n\t}'
    if needle not in s:raise SystemExit('config validation seam changed')
    s=s.replace(needle,needle+'\n\tif err := v.Reporting.validate(); err != nil { return err }',1)
config.write_text(s)

for module in ['github.com/hurtener/chartworks/internal/reporting','github.com/hurtener/chartworks/internal/reportingapi']:
    import_path('internal/foundation/work.go',module)
replace_once('internal/foundation/work.go','\tw.handler = chartapi.Handler(verifier, chartService, w.handler)','''\tw.handler = chartapi.Handler(verifier, chartService, w.handler)
 var capture reporting.QueryCapture
 if w.nlq != nil { capture = queryBlockCapture{query:w.nlq} }
 blockService, err := reporting.New(db,published,w.sourceService,validator,executor,capture,v.Reporting)
 if err != nil { w.close();return nil,err }
 blockRegistry, err := reportingapi.Registry(blockService.CanValidate(),blockService.CanCapture())
 if err != nil { w.close();return nil,err }
 w.handler = reportingapi.Handler(verifier,blockService,w.handler)''')
replace_once('internal/foundation/work.go','nlqExecutionRegistry, byoRegistry, chartRegistry)','nlqExecutionRegistry, byoRegistry, chartRegistry, blockRegistry)')

# One normalized read-to-chart conversion, shared by the SDK and block validator.
# Keep the SDK function/error public names backward compatible.
chart_path=Path('sdk/chartworks/charts.go')
s=chart_path.read_text()
if 'chartdata.FromReadResult' not in s:
    start=s.index('func ChartDataFromReadResult(')
    body=s[start:]
    if not body.rstrip().endswith('}'):raise SystemExit('unexpected chart adapter tail')
    replacements={'ChartDataFromReadResult':'FromReadResult','ReadResult':'exec.Result','ChartData':'charts.Data','ChartLimits':'charts.Limits','ChartColumn':'charts.Column','ChartCell':'charts.Cell','ChartCompleteness':'charts.Completeness','ChartColumnProvenance':'charts.Provenance','ErrChartResult':'ErrInvalid'}
    for old,new in replacements.items():body=re.sub(r'\b'+old+r'\b',new,body)
    Path('internal/chartdata').mkdir(exist_ok=True)
    Path('internal/chartdata/read.go').write_text('''// Package chartdata losslessly adapts common read results to caller-owned chart data.
// It infers no aggregation, units, semantic approval, source scope or authorization.
package chartdata
import("bytes";"context";"encoding/json";"errors";"strconv";"github.com/hurtener/chartworks/internal/charts";"github.com/hurtener/chartworks/internal/exec")
var ErrInvalid=errors.New("chartworks: invalid chart result")
// FromReadResult preserves exact scalar encodings and rejects invalid wire data.
'''+body)
    s=s[:start]+'''func ChartDataFromReadResult(ctx context.Context,result ReadResult,limits ChartLimits)(ChartData,error){return chartdata.FromReadResult(ctx,result,limits)}
'''
    s=s.replace('errors.New("chartworks: invalid chart result")','chartdata.ErrInvalid')
    for imp in ['bytes','encoding/json','errors','strconv']:s=s.replace('\t"'+imp+'"\n','')
    s=s.replace('import (\n','import (\n\t"github.com/hurtener/chartworks/internal/chartdata"\n',1)
    chart_path.write_text(s)

p=Path('internal/reporting/definition.go');s=p.read_text()
if 'chartdata.FromReadResult' not in s:
    start=s.index('func checkResult(')
    s=s[:start]+'''func checkResult(ctx context.Context,d Definition,result exec.Result,limits config.Reporting)error{
 if digest(result.Schema)!=digest(d.ExpectedSchema) || result.Bytes>limits.PreviewBytes{return ErrStale}
 normalized,err:=chartdata.FromReadResult(ctx,result,chartLimits(limits));if err!=nil{return ErrStale}
 positions:=map[string]int{};for i,f:=range result.Schema{positions[f.Name]=i}
 for _,output:=range d.Outputs{
  if output.Mapping==nil{continue}
  data:=charts.Data{Version:charts.Version,Columns:clone(output.Mapping.Columns),Rows:make([][]charts.Cell,len(normalized.Rows)),Completeness:normalized.Completeness}
  for rowIndex,row:=range normalized.Rows{
   data.Rows[rowIndex]=make([]charts.Cell,len(data.Columns))
   for i,column:=range data.Columns{index,ok:=positions[column.Name];if !ok{return ErrStale};data.Rows[rowIndex][i]=row[index]}
  }
  if err:=charts.ValidateMapping(ctx,data,*output.Mapping,chartLimits(limits));err!=nil{return ErrStale}
 }
 return ctx.Err()
}
'''
    s=s.replace('import (\n','import (\n\t"github.com/hurtener/chartworks/internal/chartdata"\n',1)
    p.write_text(s)

# Already applied type edits are idempotent after gofmt too.
p=Path('.phase27_apply.py');s=p.read_text();s=s.replace("if 'for _, pin := range d.Topics { add(\"topic\", \"read\", pin.Topic) }' not in s:","if 'add(\"topic\", \"read\", pin.Topic)' not in s:");p.write_text(s)

# An absent pin set is corruption, never evidence of current health.
p=Path('internal/store/postgres/blocks_read.go');s=p.read_text()
old='const blockCurrent = `NOT EXISTS ('
new='''const blockCurrent = `EXISTS(SELECT 1 FROM chartworks.block_source_pins present WHERE (present.tenant_id,present.block_id,present.revision)=(r.tenant_id,r.block_id,r.revision))
 AND EXISTS(SELECT 1 FROM chartworks.block_topic_pins present WHERE (present.tenant_id,present.block_id,present.revision)=(r.tenant_id,r.block_id,r.revision)) AND NOT EXISTS ('''
s=s.replace(old,new)
# Readers of shared history never receive a pointer into another actor's draft.
s=s.replace('if reporting.Require(e, id, reporting.Preview) != nil {\n\t\t\tout.State.DraftRevision = 0\n\t\t\tout.State.DraftState = ""\n\t\t}', 'out.State.DraftRevision = 0\n\t\tout.State.DraftState = ""')
# A stored healthy observation does not live forever after its evidence expires.
needle='\tif !out.Current {'
if 'Reason: "validation_expired"}\n\t}' not in s:
    s=s.replace(needle,'\tif out.Validation != nil && !time.Now().Before(out.Validation.Evidence.ExpiresAt) && out.Health.Status == "healthy" { out.Health = reporting.Health{Status:"stale",Reason:"validation_expired"} }\n'+needle,1)
# Coordinate validation is part of the read boundary, not an unused diagnostic.
if 'if err := blockConsistency(out); err != nil' not in s:
    s=s.replace('\tif !e.Valid() {\n\t\treturn reporting.Snapshot{}, access.ErrUnauthenticated','\tif err := blockConsistency(out); err != nil { return reporting.Snapshot{},err }\n\tif !e.Valid() {\n\t\treturn reporting.Snapshot{}, access.ErrUnauthenticated',1)
p.write_text(s)

# Require the new metadata relations in readiness validation, not merely a version row.
p=Path('internal/store/postgres/migrate.go');s=p.read_text()
if '"block_heads"' not in s:
    marker='"byo_context_bundles"'
    if marker not in s:
        marker='"topic_publication_heads"'
    if marker not in s:raise SystemExit('missing requiredRelations list marker')
    tables=['block_heads','block_revisions','block_revision_references','block_topic_pins','block_source_pins','block_validations','block_publications','block_attestations','block_withdrawals','block_events','block_health']
    s=s.replace(marker,marker+', '+', '.join('"'+x+'"' for x in tables),1)
p.write_text(s)

# Register, do not lower, the existing coverage bands.
p=Path('scripts/coverage-bands.conf');s=p.read_text()
for package in ['internal/reporting','internal/reportingapi','internal/chartdata']:
    if not re.search(r'^'+re.escape(package)+r'\s',s,re.M):s+='\n'+package+' 80\n'
p.write_text(s)
