"""Temporary branch-only integration aid; removed before final review."""
from pathlib import Path
import re
import subprocess
changed = set()
def replace(path, old, new):
    p = Path(path); s = p.read_text()
    if new in s: return
    if s.count(old) != 1: raise RuntimeError(f'Expected one anchor in {path}: {old[:120]!r}')
    p.write_text(s.replace(old, new)); changed.add(path)

p = 'test/acceptance/phase30_test.go'
replace(p, '\tt.Run("AC02", testPhase30Authority)\n', '\tt.Run("AC02", testPhase30Authority)\n\tt.Run("AC03", testPhase30Calendars)\n\tt.Run("AC04", testPhase30Lifecycle)\n\tt.Run("AC05", testPhase30Operations)\n\tt.Run("AC06", testPhase30Delivery)\n\tt.Run("AC07", testPhase30ChangedDependencies)\n\tt.Run("AC08", testPhase30Transport)\n')
p = Path('test/acceptance/phase30_calendars_test.go')
s = p.read_text()
start = s.index('\tscopes := slices.Clone(f.admissionScopes)', s.index('func (f *phase30Fixture) manager'))
end = s.index('\tif len(scopes) > 32', start)
s = s[:start] + '\tscopes := phase30ManagementScopes(f)\n' + s[end:]
s = s.replace('\n\t"slices"\n', '\n')
p.write_text(s); changed.add(str(p))

p = 'internal/reporting/scheduled.go'
replace(p, 'func (s *Scheduled) ValidateScheduledReporting(ctx context.Context, e identity.Envelope, target jobs.ReportingTarget) error {', '''func (s *Scheduled) ValidateScheduledReporting(ctx context.Context, e identity.Envelope, target jobs.ReportingTarget) (err error) {
    // The scheduling transport consumes this domain seam without importing the
    // reporting implementation. Preserve public error classes, not source or
    // model details, so a malformed target is not mislabeled an infrastructure
    // outage and stale publication consent remains a revision conflict.
    defer func() {
        switch {
        case errors.Is(err, ErrInvalid): err = errors.Join(jobs.ErrInvalid, err)
        case errors.Is(err, ErrStale): err = errors.Join(store.ErrConflict, err)
        case errors.Is(err, ErrBudget): err = errors.Join(jobs.ErrReportingBudget, err)
        case errors.Is(err, ErrUnavailable): err = errors.Join(jobs.ErrTransient, err)
        }
    }()''')
p = 'internal/workapi/http.go'
replace(p, 'case errors.Is(err, gateway.ErrBudget):', 'case errors.Is(err, gateway.ErrBudget), errors.Is(err, jobs.ErrReportingBudget):')
for path, field, jsonkey, enums in [
    ('internal/jobs/schedule.go', 'Type', 'type', 'enum=manual,enum=interval,enum=cron'),
    ('internal/jobs/jobs.go', 'Kind', 'kind', 'enum=retention.sweep,enum=pipeline.run,enum=reporting.scheduled')]:
    p = Path(path); s = p.read_text()
    pattern = r'(' + field + r'\s+string\s+`json:"' + jsonkey + r'")`'
    s, count = re.subn(pattern, lambda m: m[1] + ' jsonschema:"' + enums + '"`', s, count=1)
    if count != 1: raise RuntimeError('Expected closed kind schema: '+path)
    p.write_text(s); changed.add(path)
# A redundant state request under a matching CAS is a no-op, not an invented
# revision whose immutable history would disagree with the stored definition.
p = 'internal/store/postgres/schedules.go'
replace(p, '\t\tprevious, next := out.PreviousDue, out.NextDue', '\t\tif out.Enabled == enabled { return nil }\n\t\tprevious, next := out.PreviousDue, out.NextDue')
changed.update(str(p) for p in Path('test/acceptance').glob('phase30*.go'))
subprocess.run(['gofmt', '-w', *sorted(changed)], check=True)
subprocess.run(['git', 'diff', '--check'], check=True)
