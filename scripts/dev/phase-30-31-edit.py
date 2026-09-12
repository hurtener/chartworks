"""Temporary branch-only editing aid; remove before final review."""
from pathlib import Path
import subprocess
changed=set()
def replace(path,old,new):
 p=Path(path);text=p.read_text()
 if new in text:return
 if text.count(old)!=1:raise RuntimeError(f'Expected one anchor in {path}: {old[:180]!r}')
 p.write_text(text.replace(old,new));changed.add(path)

p='test/acceptance/phase31_test.go'
replace(p,'''\tif block != "" {
\t\tscopes = append(scopes, "cw.block.read:"+block)
\t}''','''\tif block != "" {
        // The real catalog requires signed parent/dependency reach in addition
        // to the block ID. Derive only this synthetic block's exact references;
        // no execution, SQL-read, model, authoring or wildcard grant is added.
        snapshot,err:=f.domain.f.f.db.ReadBlock(t.Context(),f.domain.execute,block,reporting.Reference{},reporting.Read)
        if err!=nil{t.Fatal(err)}
        scopes=append(scopes,"cw.block.read:"+block,"cw.topic.read:"+snapshot.State.Topic)
        for _,ref:=range snapshot.References {
            scope:="cw."+ref.Kind+"."+ref.Permission+":"+ref.ID
            if !slices.Contains(scopes,scope){scopes=append(scopes,scope)}
        }
\t}''')
p='test/acceptance/phase31_providers_test.go'
replace(p,'\tsearch, err := f.service.Search(t.Context(), reader, reporting.ReportingSearchRequest{Kind: "block", Limit: 100})','''    missingDependencies:=phase27Actor(t,f.domain.f,"missing-dependency-reader",[]string{"reporting.read","cw.block.read:p31-visible","cw.execution_context.use:"+d.Context})
    deniedSearch,err:=f.service.Search(t.Context(),missingDependencies,reporting.ReportingSearchRequest{Kind:"block",Limit:100})
    if err!=nil || len(deniedSearch.Items)!=0{t.Fatal("metadata catalog widened missing dependency authority",deniedSearch,err)}
\tsearch, err := f.service.Search(t.Context(), reader, reporting.ReportingSearchRequest{Kind: "block", Limit: 100})''')
replace(p,'''\t// Move only the test fixture's retention deadline into the past. The real
\t// metadata-only reader must return a tombstone without loading old payloads.''','''\t// Install the production retention tombstone while deliberately retaining
    // the synthetic old payload rows as adversarial input. Do not rewrite the
    // immutable accepted deadline. The actual provider must not fetch those
    // values merely because stale payload bytes still exist in storage.''')
replace(p,"UPDATE chartworks.composition_runs SET expires_at=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2", "UPDATE chartworks.composition_runs SET state='expired',code='retention_expired',complete=false,retained_bytes=0,reserved_bytes=0 WHERE tenant_id=$1 AND operation_id=$2")
p='web/report-viewer/app.js'
replace(p,"  if (columns.length > 256 || rows.length > 1000) throw fail('limit_exceeded');", "  if (columns.length > 256 || rows.length > 1000) throw fail('limit_exceeded');\n  if (rows.length === 0) { const notice = element('p',w.empty,'notice'); notice.setAttribute('role','status'); parent.append(notice); }")
subprocess.run(['gofmt','-w',*sorted(p for p in changed if p.endswith('.go'))],check=True)
subprocess.run(['node','--check','web/report-viewer/app.js'],check=True)
subprocess.run(['git','diff','--check'],check=True)
