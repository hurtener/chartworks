"""Temporary branch-only editing aid; remove before final review."""
from pathlib import Path
import subprocess
changed=set()
def replace(path,old,new,marker):
 p=Path(path);text=p.read_text()
 if marker in text:return
 if text.count(old)!=1:raise RuntimeError(f'Expected one anchor in {path}: {old[:180]!r}')
 p.write_text(text.replace(old,new));changed.add(path)

p='test/acceptance/phase31_test.go'
replace(p,'''\tif block != "" {
\t\tscopes = append(scopes, "cw.block.read:"+block)
\t}''','''\tif block != "" {
        // Derive this fixture's exact metadata dependencies; no execution grant
        // or wildcard is added to the retained-result reader.
        snapshot,err:=f.domain.f.f.db.ReadBlock(t.Context(),f.domain.execute,block,reporting.Reference{},reporting.Read)
        if err!=nil{t.Fatal(err)}
        scopes=append(scopes,"cw.block.read:"+block,"cw.topic.read:"+snapshot.State.Topic)
        for _,ref:=range snapshot.References {
            scope:="cw."+ref.Kind+"."+ref.Permission+":"+ref.ID
            if !slices.Contains(scopes,scope){scopes=append(scopes,scope)}
        }
\t}''','snapshot.References')
p='test/acceptance/phase31_providers_test.go'
replace(p,'\tsearch, err := f.service.Search(t.Context(), reader, reporting.ReportingSearchRequest{Kind: "block", Limit: 100})','''    missingDependencies:=phase27Actor(t,f.domain.f,"missing-dependency-reader",[]string{"reporting.read","cw.block.read:p31-visible","cw.execution_context.use:"+d.Context})
    deniedSearch,err:=f.service.Search(t.Context(),missingDependencies,reporting.ReportingSearchRequest{Kind:"block",Limit:100})
    if err!=nil || len(deniedSearch.Items)!=0{t.Fatal("metadata catalog widened missing dependency authority",deniedSearch,err)}
\tsearch, err := f.service.Search(t.Context(), reader, reporting.ReportingSearchRequest{Kind: "block", Limit: 100})''','missing-dependency-reader')
replace(p,'''\t// Move only the test fixture's retention deadline into the past. The real
\t// metadata-only reader must return a tombstone without loading old payloads.''','''\t// Apply a retention tombstone, preserving the immutable accepted deadline.
    // Old synthetic payload rows deliberately remain as adversarial input:
    // the provider must not load values belonging to an expired artifact.''','Old synthetic payload rows')
replace(p,"UPDATE chartworks.composition_runs SET expires_at=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2", "UPDATE chartworks.composition_runs SET state='expired',code='retention_expired',complete=false,retained_bytes=0,reserved_bytes=0 WHERE tenant_id=$1 AND operation_id=$2", "SET state='expired',code='retention_expired',complete=false,retained_bytes=0,reserved_bytes=0")
p='web/report-viewer/app.js'
replace(p,"  if (columns.length > 256 || rows.length > 1000) throw fail('limit_exceeded');", "  if (columns.length > 256 || rows.length > 1000) throw fail('limit_exceeded');\n  if (rows.length === 0) { const notice = element('p',w.empty,'notice'); notice.setAttribute('role','status'); parent.append(notice); }", "rows.length === 0")
go=sorted(p for p in changed if p.endswith('.go'))
if go:subprocess.run(['gofmt','-w',*go],check=True)
subprocess.run(['node','--check','web/report-viewer/app.js'],check=True)
subprocess.run(['git','diff','--check'],check=True)
