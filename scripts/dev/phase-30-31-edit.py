"""Temporary branch-only patch; remove before final review."""
from pathlib import Path
import subprocess

changed = set()
def replace(path, old, new):
    p = Path(path); text = p.read_text()
    if new in text: return
    if text.count(old) != 1: raise RuntimeError(f'Expected one anchor in {path}: {old[:120]!r}')
    p.write_text(text.replace(old, new)); changed.add(path)

p = 'internal/jobs/jobs.go'
replace(p, '\tPipeline  *PipelineTarget `json:"pipeline,omitempty"`', '\tPipeline  *PipelineTarget `json:"pipeline,omitempty"`\n\tReporting *ReportingTarget `json:"reporting,omitempty"`')
replace(p, 'if s.Pipeline == nil {', 'if s.Pipeline == nil && s.Reporting == nil {')
replace(p, 'if s.Pipeline != nil && s.Pipeline.Valid() {', 'if s.Pipeline != nil && s.Pipeline.Valid() && s.Reporting == nil {')
replace(p, '\tcase PipelineKind:\n', '\tcase ReportingKind:\n\t\tif s.Pipeline == nil && s.Reporting != nil && s.Reporting.Valid() { return nil }\n\tcase PipelineKind:\n')
replace(p, '\tPipeline          *PipelineTarget `json:"pipeline,omitempty"`', '\tPipeline          *PipelineTarget `json:"pipeline,omitempty"`\n\tReporting         *ReportingDispatch `json:"reporting,omitempty"`')
replace(p, '\tb, _ := json.Marshal(parts)', '\tif j.Reporting != nil { parts = append(parts, j.Reporting) }\n\tb, _ := json.Marshal(parts)')
replace(p, '((j.Kind == MaintenanceKind && j.Pipeline == nil) || (j.Kind == PipelineKind && j.Pipeline != nil && j.Pipeline.Valid()))', '((j.Kind == MaintenanceKind && j.Pipeline == nil && j.Reporting == nil) || (j.Kind == PipelineKind && j.Pipeline != nil && j.Pipeline.Valid() && j.Reporting == nil) || (j.Kind == ReportingKind && j.Pipeline == nil && j.Reporting != nil && j.Reporting.Valid()))')
replace(p, '\tif j.Kind == PipelineKind {', '''\tif j.Kind == ReportingKind {
        if j.Reporting.Target.Require(e) != nil || access.Require(e, "reporting.execute",
            access.Resource{Tenant: j.Tenant, Kind: "execution_binding", Permission: "use", ID: j.BindingID},
            access.Resource{Tenant: j.Tenant, Kind: "run", Permission: "execute", ID: j.ID}) != nil { return ErrAuthority }
        return nil
    }
\tif j.Kind == PipelineKind {''')
p = 'internal/jobs/service.go'
replace(p, '\tpipeline  PipelineExecutor', '\tpipeline  PipelineExecutor\n\treporting ReportingExecutor')
replace(p, '\tif request.Kind == PipelineKind {', '''\tif request.Kind == ReportingKind {
        if s.reporting == nil { return store.Scope{}, ErrTransient }
        if err := request.Reporting.Require(e); err != nil { return store.Scope{}, err }
        if err := s.reporting.ValidateScheduledReporting(ctx, e, *request.Reporting); err != nil { return store.Scope{}, err }
        return store.NewScope(e.Tenant(), e.User())
    }
\tif request.Kind == PipelineKind {''')
replace(p, '\t\t} else {\n\t\t\t_, err = s.repo.CompleteJob(effect, lease, envelope)', '''\t\t} else if lease.Job.Kind == ReportingKind {
            if s.reporting == nil { err = ErrAuthority } else { err = s.reporting.ExecuteScheduledReporting(effect, lease, envelope) }
\t\t} else {
\t\t\t_, err = s.repo.CompleteJob(effect, lease, envelope)''')
# RequestTask dispatch validation is extended in a dedicated small helper.
p = 'internal/jobs/requests.go'
text = Path(p).read_text()
start = text.index('\tif t.Dispatch != nil {', text.index('func (t RequestTask) Valid() bool'))
end = text.index('\n\t}', start) + len('\n\t}')
old = text[start:end]
new = '\tif t.Dispatch != nil {\n\t\treturn validDispatchedRequest(t)\n\t}'
if old != new:
    Path(p).write_text(text[:start] + new + text[end:]); changed.add(p)
helper = Path('internal/jobs/reporting_request.go')
helper.write_text('''package jobs

// validDispatchedRequest preserves the accepted queue manifest across both
// production target families. Neither a session label nor a request body can
// convert ordinary request work into broker-authorized queued work.
func validDispatchedRequest(t RequestTask) bool {
    j := t.Dispatch
    if j == nil || !j.Valid() || t.ID != j.ID || t.Tenant != j.Tenant || t.Actor != j.Executor ||
        t.Session != j.ID || t.MaxAttempts != j.MaxAttempts || t.ManifestHash != j.ManifestHash ||
        t.Created.IsZero() || !t.Expires.After(t.Created) || t.ManifestHash != t.Digest() { return false }
    if j.Kind == PipelineKind {
        return t.Input == (RequestInput{Kind: PipelineKind, Target: j.Pipeline.ID, InputHash: j.Pipeline.Digest})
    }
    return j.Kind == ReportingKind && j.Reporting != nil && t.Input == j.Reporting.Input
}
''')
subprocess.run(['git', 'add', '-N', '--', str(helper)], check=True)
changed.add(str(helper))
changed.add('internal/jobs/reporting_target.go')
p = 'internal/store/postgres/request_jobs.go'
replace(p, "dispatch_mode='queued' AND kind='pipeline.run' AND operation_id=$3", "dispatch_mode='queued' AND kind IN('pipeline.run','reporting.scheduled') AND operation_id=$3")
p = 'internal/store/postgres/jobs.go'
replace(p, '\t\tj.Pipeline = accepted.Pipeline', '\t\tj.Pipeline = accepted.Pipeline\n\t\tj.Reporting = accepted.Reporting')
subprocess.run(['gofmt', '-w', *sorted(changed)], check=True)
subprocess.run(['git', 'diff', '--check'], check=True)
