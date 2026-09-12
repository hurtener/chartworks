"""Temporary branch-only editing aid; remove before final review."""
from pathlib import Path
import subprocess
changed = set()
def replace(path, old, new):
    p = Path(path); text = p.read_text()
    if new in text: return
    if text.count(old) != 1: raise RuntimeError(f'Expected one anchor in {path}: {old[:120]!r}')
    p.write_text(text.replace(old, new)); changed.add(path)

p = 'test/acceptance/phase31_execution_parity_test.go'
text = Path(p).read_text()
lines = []
for line in text.splitlines(keepends=True):
    if 'callProtected(' in line:
        if 'phase31Wire(t,' in line:
            line = line.replace('phase31Wire(t,', 'string(phase31Wire(t,').replace(', nil)', '), nil)')
        line = line.replace('[]byte(body)', 'body').replace('[]byte(strings.Repeat(" ", 17<<20)+"{}")', 'strings.Repeat(" ", 17<<20)+"{}"')
    lines.append(line)
Path(p).write_text(''.join(lines)); changed.add(p)

p = 'internal/jobs/requests.go'
replace(p, '\tif t.Dispatch != nil {\n\t\treturn validDispatchedRequest(t)\n\t}', '\tif t.Dispatch != nil && !validDispatchedRequest(t) {\n\t\treturn false\n\t}')
replace(p, '\towned     bool\n}', '\towned     bool\n\tparent    *Invocation\n}')
replace(p, '\treturn i.owned && i.lease.Task.Valid()', '''\tif i.parent != nil {
        p := i.parent
        if p.parent != nil || !p.Valid() || (p.lease.Task.Input.Kind != "report.run" && p.lease.Task.Input.Kind != "dashboard.run") ||
            i.lease.Task.Input.Kind != "reporting.run" || i.lease.Task.Tenant != p.lease.Task.Tenant ||
            i.lease.Task.Actor != p.lease.Task.Actor || i.lease.Task.Session != p.lease.Task.Session || i.lease.Task.ID == p.lease.Task.ID { return false }
    }
\treturn i.owned && i.lease.Task.Valid()''')
replace(p, '\tif err := i.lease.Task.Require(i.authority); err != nil {', '''\tif i.parent != nil {
        p := i.parent.Lease().Task
        if _, err := i.parent.Current(p.Input.Kind, p.Input.Target, p.Input.InputHash); err != nil { return identity.Envelope{}, err }
    }
\tif err := i.lease.Task.Require(i.authority); err != nil {''')
signature = 'func (r *RequestRunner) Run(ctx context.Context, e identity.Envelope, task RequestTask, timeout time.Duration, handler func(context.Context, Invocation) error) (RequestTask, error) {'
replace(p, signature, signature + '\n\treturn r.run(ctx, e, task, timeout, handler, nil)\n}\n\nfunc (r *RequestRunner) run(ctx context.Context, e identity.Envelope, task RequestTask, timeout time.Duration, handler func(context.Context, Invocation) error, parent *Invocation) (RequestTask, error) {')
replace(p, '\tlease, err := r.repo.ClaimRequest(ctx, e, task.ID, hex.EncodeToString(id[:]), r.limits)', '''\tvar lease RequestLease
    var err error
    if parent == nil {
        lease, err = r.repo.ClaimRequest(ctx, e, task.ID, hex.EncodeToString(id[:]), r.limits)
    } else {
        if _, err = nestedAuthority(*parent, task.Input); err != nil { return RequestTask{}, err }
        repo, ok := r.repo.(NestedRequestRepository)
        if !ok { return RequestTask{}, ErrInvalid }
        lease, err = repo.ClaimNestedRequest(ctx, *parent, task.ID, hex.EncodeToString(id[:]), r.limits)
    }''')
replace(p, 'invocation := Invocation{lease: lease, authority: e, owned: true}', 'invocation := Invocation{lease: lease, authority: e, owned: true, parent: parent}')
changed.add('internal/jobs/nested_requests.go')
subprocess.run(['gofmt', '-w', *sorted(changed)], check=True)
subprocess.run(['git', 'diff', '--check'], check=True)
