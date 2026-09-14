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

for p in ['internal/jobs/schedule.go', 'sdk/chartworks/work.go']:
    replace(p, 'type Schedule struct {\n', 'type Schedule struct {\n\tRetired bool `json:"retired"`\n')
p = 'sdk/chartworks/work.go'
replace(p, 'type JobTarget struct {\n', 'type JobTarget struct {\n\tReporting *ScheduledReportingTarget `json:"reporting,omitempty"`\n')
replace(p, 'type Job struct {\n', 'type Job struct {\n\tReporting *ScheduledReportingDispatch `json:"reporting,omitempty"`\n\tDelivery *ScheduledReportingReceipt `json:"delivery,omitempty"`\n')
p = 'internal/store/postgres/schedules.go'
replace(p, 'const scheduleColumns = `schedule_id,revision,enabled,tenant_id,creator_id,creator_session,request,next_due,previous_due`', 'const scheduleColumns = `schedule_id,revision,enabled,tenant_id,creator_id,creator_session,request,next_due,previous_due,retired`')
replace(p, '&s.NextDue, &s.PreviousDue)', '&s.NextDue, &s.PreviousDue, &s.Retired)')
replace(p, 'if out.Revision != expected {', 'if out.Revision != expected || out.Retired {')
replace(p, 'if !schedule.Enabled || schedule.Revision != expected {', 'if !schedule.Enabled || schedule.Retired || schedule.Revision != expected {')
p = 'internal/store/postgres/schedule_replace.go'
replace(p, '\t\tif current.Revision == expected+1 {', '\t\tif current.Retired { return store.ErrConflict }\n\t\tif current.Revision == expected+1 {')
for path, typ in [('internal/jobs/jobs.go', 'Submission'), ('internal/jobs/schedule.go', 'ScheduleRequest')]:
    p = Path(path); s = p.read_text()
    pattern = r'(func \((\w+) ' + typ + r'\) Validate\(\) error \{\n)'
    marker = '\twire, marshalErr := json.Marshal('
    if marker not in s:
        s, n = re.subn(pattern, lambda m: m[1] + marker + m[2] + ')\n\tif marshalErr != nil || len(wire) > 8192 { return ErrInvalid }\n', s)
        if n != 1: raise RuntimeError('Missing validation anchor: '+path)
        p.write_text(s); changed.add(path)

p = 'internal/workapi/http.go'
replace(p, '\t\t\tOperation{Method: "POST", Path: "/v1/schedules/{id}/runs",', '\t\t\tOperation{Method: "PUT", Path: "/v1/schedules/{id}", Action: "scheduling.write", Effect: "schedule_state"},\n\t\t\tOperation{Method: "POST", Path: "/v1/schedules/{id}/retire", Action: "scheduling.write", Effect: "schedule_state"},\n\t\t\tOperation{Method: "POST", Path: "/v1/schedules/{id}/history", Action: "scheduling.read", Effect: "metadata_read"},\n\t\t\tOperation{Method: "POST", Path: "/v1/schedules/{id}/test", Action: "scheduling.execute", Effect: "durable_admission"},\n\t\t\tOperation{Method: "POST", Path: "/v1/schedules/{id}/runs",')
replace(p, 'op.Effect == "schedule_state" {', '(op.Effect == "schedule_state" && op.Path != "/v1/schedules/{id}") {')
replace(p, '\t\tcase "/v1/schedules/{id}":\n\t\t\tout, err = queue.GetSchedule(r.Context(), e, id)', '''\t\tcase "/v1/schedules/{id}":
            if r.Method == http.MethodGet { out, err = queue.GetSchedule(r.Context(), e, id) } else {
                var input ScheduleReplaceRequest
                key, keyErr := idempotency(r)
                if keyErr != nil { err = keyErr } else if err = body(w, r, &input); err == nil {
                    out, err = queue.ReplaceSchedule(r.Context(), e, id, input.Expected, key, input.Request)
                }
            }
        case "/v1/schedules/{id}/retire":
            var input ScheduleRevisionRequest
            if err = body(w, r, &input); err == nil { out, err = queue.RetireSchedule(r.Context(), e, id, input.Expected) }
        case "/v1/schedules/{id}/history":
            var input jobs.ScheduleHistoryRequest
            if err = body(w, r, &input); err == nil { out, err = queue.History(r.Context(), e, id, input) }
        case "/v1/schedules/{id}/test":
            var input ScheduleRevisionRequest
            key, keyErr := idempotency(r)
            if keyErr != nil { err = keyErr } else if err = body(w, r, &input); err == nil {
                out, err = queue.TestSchedule(r.Context(), e, id, key, input.Expected)
            }''')
replace(p, '\tif typ.Kind() != reflect.Struct || typ.PkgPath() == "time" {', '''\tfor typ.Kind() == reflect.Pointer {
        if strings.TrimSpace(string(data)) == "null" { return nil }
        typ = typ.Elem()
    }
    if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
        var items []json.RawMessage
        if json.Unmarshal(data, &items) != nil { return jobs.ErrInvalid }
        for _, item := range items { if closed(item, typ.Elem()) != nil { return jobs.ErrInvalid } }
        return nil
    }
\tif typ.Kind() != reflect.Struct || typ.PkgPath() == "time" {''')
p = 'internal/workapi/registry.go'
replace(p, '\tif len(definitions) == 0 {\n\t\treturn nil, nil\n\t}\n\treturn api.New(definitions)', '\tlifecycle, err := scheduleLifecycleDefinitions(dispatch)\n\tif err != nil { return nil, err }\n\tdefinitions = append(definitions, lifecycle...)\n\tif len(definitions) == 0 {\n\t\treturn nil, nil\n\t}\n\treturn api.New(definitions)')
p = 'test/acceptance/phase30_fixture_test.go'
replace(p, '\tif kind == "saved_question" {', '\tif t.ResourceKind() == "report" { t.Locale = "en-US" }\n\tif kind == "saved_question" {')
p = 'test/acceptance/phase30_test.go'
replace(p, '!errors.Is(err, access.ErrForbidden)', '!errors.Is(err, access.ErrNotFound)')
for p in ['internal/jobs/schedule_lifecycle.go', 'internal/store/postgres/schedule_lifecycle.go', 'internal/workapi/schedules.go', 'sdk/chartworks/schedules.go']:
    changed.add(p)
subprocess.run(['gofmt', '-w', *sorted(changed)], check=True)
subprocess.run(['git', 'diff', '--check'], check=True)
