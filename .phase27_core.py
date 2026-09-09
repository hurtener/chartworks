# Temporary, exact seam edits. Removed before read-only final verification.
from pathlib import Path
import re

def patch(path,old,new):
 p=Path(path);s=p.read_text()
 if new in s:return
 if s.count(old)!=1:raise SystemExit('guard failed '+path+': '+old[:100])
 p.write_text(s.replace(old,new,1))

def imp(path,module):
 p=Path(path);s=p.read_text()
 if '"'+module+'"' not in s:
  if 'import (\n' not in s:raise SystemExit('import seam '+path)
  p.write_text(s.replace('import (\n','import (\n\t"'+module+'"\n',1))

imp('sdk/chartworks/charts.go','github.com/hurtener/chartworks/internal/exec')
patch('sdk/chartworks/charts.go','return chartdata.FromReadResult(ctx, result, limits)','''normalized := exec.Result{Rows:result.Rows,Outcome:result.Outcome,Truncation:result.Truncation,Bytes:result.Bytes}
 normalized.Cost.PlannerUnits=result.Cost.PlannerUnits
 normalized.Cost.ScannedBytes=result.Cost.ScannedBytes
 if result.Schema!=nil { normalized.Schema=make([]exec.Field,len(result.Schema));for i,field:=range result.Schema{normalized.Schema[i]=exec.Field(field)} }
 return chartdata.FromReadResult(ctx,normalized,limits)''')
imp('test/acceptance/phase27_test.go','github.com/hurtener/chartworks/internal/chartdata')
patch('test/acceptance/phase27_test.go','sdk.ChartDataFromReadResult(ctx, *report.Result, charts.Defaults())','chartdata.FromReadResult(ctx, *report.Result, charts.Defaults())')

imp('internal/reporting/model.go','github.com/hurtener/chartworks/internal/sources')
p=Path('internal/reporting/model.go');s=p.read_text()
if not re.search(r'\bCatalog\s+sources.CatalogIdentity',s):
 s=s.replace('type ValidationRecord struct {','type ValidationRecord struct {\n Catalog sources.CatalogIdentity `json:"catalog_identity"`',1)
p.write_text(s)

p=Path('internal/reporting/validation.go');s=p.read_text()
needle='\tscope, err := validationScope(binding, definitions)'
if 'var catalog sources.CatalogIdentity' not in s:
 if s.count(needle)!=1:raise SystemExit('validation catalog seam')
 s=s.replace(needle,''' var catalog sources.CatalogIdentity
 if observer,ok:=s.sources.(CatalogObserver);ok {
  observed,err:=observer.ObserveCatalog(ctx,e,d.Source,d.Context)
  if err!=nil{return record,result,resolved,nil,err}
  if observed.BindingDigest!=exec.Hash(binding){return record,result,resolved,nil,ErrStale}
  catalog=observed.Identity
 }
'''+needle,1)
 s=s.replace('\trecord.Evidence.ResolvedAt = resolved.At','\trecord.Catalog=clone(catalog)\n\trecord.Evidence.ResolvedAt = resolved.At',1)
p.write_text(s)
imp('internal/reporting/validation.go','github.com/hurtener/chartworks/internal/sources')

patch('internal/store/postgres/blocks_write.go','if !snapshot.Current {','if !snapshot.Current && m.Revision==nil {')
p=Path('internal/store/postgres/blocks_write.go');s=p.read_text()
s=s.replace('r == nil || r.Actor != e.User()', 'r == nil || len(r.Definition.Topics)==0 || r.Actor != e.User()')
p.write_text(s)

# Extend the thin public registry/SDK generator rather than hand-writing a second
# consumer path for assisted period edits and native dependency impact.
p=Path('.phase27_api.py');s=p.read_text()
marker='operations = [\n'
entries=''' ('POST','/{id}/parameters/assist','Write','ParameterizeBlock','Parameterize','ParameterizeRequest','View','Append an AST-verified typed period amendment without publication','observe'),
 ('POST','/{id}/impact','Read','RecheckBlockImpact','RecheckImpact','ImpactRequest','Impact','Explicitly observe dependency impact without altering definitions','observe'),
 ('POST','/{id}/impact/apply','Write','ApplyBlockImpact','ApplyImpact','ApplyImpactRequest','View','Create a private draft for an exact current dependency proposal','observe'),
'''
if "'ParameterizeBlock'" not in s:s=s.replace(marker,marker+entries,1)
s=s.replace("aliases=['Localized'", "aliases=['ParameterizeRequest','Rename','ImpactRequest','Impact','ApplyImpactRequest','Localized'")
s=s.replace('func Registry(validation, capture bool)', 'func Registry(validation, capture, observe bool)')
s=s.replace('entry.feature=="capture" && !capture {continue}', 'entry.feature=="capture" && !capture || entry.feature=="observe" && !observe {continue}')
s=s.replace('Registry(service.CanValidate(),service.CanCapture())','Registry(service.CanValidate(),service.CanCapture(),service.CanObserve())')
s=s.replace('if !validQuery(r.URL.Query(),selected.Query)', 'parsedQuery,queryErr:=url.ParseQuery(r.URL.RawQuery)\n  if queryErr!=nil || !validQuery(parsedQuery,selected.Query)')
p.write_text(s)
p=Path('internal/foundation/work.go');s=p.read_text().replace('reportingapi.Registry(blockService.CanValidate(), blockService.CanCapture())','reportingapi.Registry(blockService.CanValidate(), blockService.CanCapture(),blockService.CanObserve())');p.write_text(s)
