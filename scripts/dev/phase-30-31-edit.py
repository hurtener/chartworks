"""Explicit branch-only edits; removed before the completed PR."""
from pathlib import Path
import subprocess

changed = set()
def replace(path, old, new):
    p = Path(path)
    text = p.read_text()
    if new in text:
        return
    if text.count(old) != 1:
        raise RuntimeError(f'Expected one anchor in {path}: {old[:140]!r}')
    p.write_text(text.replace(old, new))
    changed.add(path)
def write(path, content):
    p = Path(path)
    if p.exists():
        if p.read_text() != content:
            raise RuntimeError(f'Refusing to overwrite unexpected file {path}')
        return
    p.write_text(content)
    subprocess.run(['git', 'add', '-N', '--', path], check=True)
    changed.add(path)

replace('internal/store/postgres/postgres.go',
        'jobs.ErrInvalid, jobs.ErrBusy, jobs.ErrEmpty, jobs.ErrAuthority, jobs.ErrTransient,',
        'jobs.ErrInvalid, jobs.ErrBusy, jobs.ErrEmpty, jobs.ErrAuthority, jobs.ErrReportingBudget, jobs.ErrReportingAttention, jobs.ErrTransient,')
replace('internal/reporting/scheduled.go',
        'func scheduledError(err error) error {\n',
        '''func scheduledError(err error) error {
    // A rejected checkpoint transaction is not a changed business definition.
    // Retrying preserves the accepted manifest and lets the existing consumer
    // reconcile retained evidence; an uncheckpointed query remains incomplete.
    if errors.Is(err, store.ErrInvalid) { return errors.Join(jobs.ErrTransient, err) }
''')
replace('internal/store/postgres/reporting_jobs.go',
        '''		if state == "normalized" {
			out.Query = "succeeded"
		}''',
        '''		if out.Kind == "block" && (state == "normalized" || state == "succeeded") {
            out.Query = "succeeded"
        }
        // Domain completion and catalog publication commit separately. Report
        // only the artifact evidence already retained, even if publication is
        // still pending after a storage failure. Never invent a notification.
        if state == "succeeded" || state == "completed" || state == "partial" {
            out.Artifact = "retained"
            out.Query = "succeeded"
            if state == "partial" { out.Query = "partial" }
        }''')
replace('internal/workapi/schedules.go',
        'reflect.TypeFor[jobs.ScheduleHistoryRequest](), false)',
        'reflect.TypeFor[jobs.ScheduleHistoryRequest](), false, api.NullableCollections)')
replace('test/acceptance/phase30_lifecycle_test.go',
        'phase29BlockWidget("frozen", target.ID, 0, "table-main")',
        'phase29BlockWidget("frozen", target.ID, 1, "table-main")')
replace('test/acceptance/phase30_delivery_test.go',
        'SET archived=true,version=version+1 WHERE',
        'SET archived=true,published_revision=NULL,version=version+1 WHERE')

write('internal/store/postgres/reporting_errors_test.go', '''package postgres

import (
    "errors"
    "fmt"
    "testing"
    "github.com/hurtener/chartworks/internal/jobs"
    "github.com/hurtener/chartworks/internal/store"
)

func TestReportingErrorsRetainPublicClass(t *testing.T) {
    for _, sentinel := range []error{jobs.ErrReportingBudget, jobs.ErrReportingAttention, jobs.ErrTransient} {
        t.Run(sentinel.Error(), func(t *testing.T) {
            for _, input := range []error{sentinel, fmt.Errorf("sensitive fixture: %w", sentinel), errors.Join(errors.New("private driver detail"), sentinel)} {
                got := safe(input)
                if got != sentinel { t.Fatalf("public error class lost: %v", got) }
            }
        })
    }
    if got := safe(errors.New("unknown private SQL or token detail")); got != store.ErrUnavailable {
        t.Fatal("unknown storage error leaked", got)
    }
}
''')
write('internal/reporting/scheduled_errors_test.go', '''package reporting

import (
    "context"
    "errors"
    "testing"
    "github.com/hurtener/chartworks/internal/access"
    "github.com/hurtener/chartworks/internal/gateway"
    "github.com/hurtener/chartworks/internal/jobs"
    "github.com/hurtener/chartworks/internal/store"
)

func TestScheduledFailureClassification(t *testing.T) {
    for _, tc := range []struct{ input, want error }{
        {nil, nil}, {context.Canceled, context.Canceled},
        {context.DeadlineExceeded, context.DeadlineExceeded},
        {store.ErrUnavailable, store.ErrUnavailable}, {store.ErrInvalid, jobs.ErrTransient},
        {ErrInvalid, jobs.ErrReportingAttention}, {ErrStale, jobs.ErrReportingAttention},
        {ErrIncomplete, jobs.ErrReportingAttention},
        {ErrBudget, jobs.ErrReportingBudget}, {gateway.ErrBudget, jobs.ErrReportingBudget},
        {jobs.ErrReportingBudget, jobs.ErrReportingBudget}, {access.ErrForbidden, jobs.ErrAuthority},
    } {
        got := scheduledError(tc.input)
        if !errors.Is(got, tc.want) { t.Fatalf("%v: got %v, want %v", tc.input, got, tc.want) }
    }
}
''')
if changed:
    subprocess.run(['gofmt', '-w', *sorted(p for p in changed if p.endswith('.go'))], check=True)
subprocess.run(['git', 'diff', '--check'], check=True)
