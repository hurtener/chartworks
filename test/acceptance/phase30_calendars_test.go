package acceptance

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

func phase30Time(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil { t.Fatal(err) }
	return at
}

// Management and execution are separately signed. This fixture combines only
// the existing target dependencies with the selected schedule permissions; it
// deliberately keeps the real issuer's 32-scope ceiling unchanged.
func (f *phase30Fixture) manager(t *testing.T) identity.Envelope {
	t.Helper()
	scopes := slices.Clone(f.admissionScopes)
	scopes = append(scopes, "scheduling.read", "scheduling.execute", "scheduling.cancel", "cw.schedule.read:*", "cw.schedule.write:*", "cw.schedule.execute:*", "cw.run.write:*")
	slices.Sort(scopes)
	scopes = slices.Compact(scopes)
	if len(scopes) > 32 { t.Fatalf("schedule fixture exceeds real signed scope ceiling: %d", len(scopes)) }
	return phase27Actor(t, f.domain.f, f.actor.User(), scopes)
}

func phase30Manual(target jobs.ReportingTarget) jobs.ScheduleRequest {
	return jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &target},
		Spec: jobs.Spec{Type: "manual", Timezone: target.Timezone, Missed: "skip", Overlap: "queue"}}
}

func (f *phase30Fixture) schedule(t *testing.T, key string, request jobs.ScheduleRequest) jobs.Schedule {
	t.Helper()
	s, err := f.queue.CreateSchedule(t.Context(), f.actor, key, request)
	if err != nil { t.Fatal("create reporting schedule", err) }
	return s
}

// cursorAt is an explicit database clock seam, not a fake scheduler or a
// modified accepted job. TickSchedules still computes and records real windows.
func (f *phase30Fixture) cursorAt(t *testing.T, schedule string, previous, next time.Time) {
	t.Helper()
	_, err := support.Raw(t, f.domain.f.f.dsn).Exec(t.Context(), `UPDATE chartworks.job_schedules SET previous_due=$3,next_due=$4 WHERE tenant_id=$1 AND schedule_id=$2`, f.actor.Tenant(), schedule, previous, next)
	if err != nil { t.Fatal(err) }
}

func (f *phase30Fixture) occurrence(t *testing.T, schedule string, due time.Time) jobs.Job {
	t.Helper()
	var id string
	if err := support.Raw(t, f.domain.f.f.dsn).QueryRow(t.Context(), `SELECT operation_id FROM chartworks.job_occurrences WHERE tenant_id=$1 AND schedule_id=$2 AND due_at=$3`, f.actor.Tenant(), schedule, due).Scan(&id); err != nil { t.Fatal(err) }
	return f.get(t, id)
}

func testPhase30Calendars(t *testing.T) {
	t.Run("real-cron-and-interval-calendar", func(t *testing.T) {
		for _, test := range []struct { name, cron, zone, after, first, second string }{
			{"spring-gap", "30 2 * * *", "America/New_York", "2024-03-10T06:00:00Z", "2024-03-11T06:30:00Z", "2024-03-12T06:30:00Z"},
			{"autumn-fold", "30 1 * * *", "America/New_York", "2024-11-03T04:00:00Z", "2024-11-03T05:30:00Z", "2024-11-03T06:30:00Z"},
			{"leap-day", "0 0 29 2 *", "UTC", "2023-03-01T00:00:00Z", "2024-02-29T00:00:00Z", "2028-02-29T00:00:00Z"},
		} {
			t.Run(test.name, func(t *testing.T) {
				spec := jobs.Spec{Type: "cron", Cron: test.cron, Timezone: test.zone, Missed: "catch_up", MaxCatchUp: 2, Overlap: "queue"}
				first, err := spec.Next(phase30Time(t, test.after))
				if err != nil || !first.Equal(phase30Time(t, test.first)) { t.Fatal("first calendar occurrence", first, err) }
				second, err := spec.Next(first)
				if err != nil || !second.Equal(phase30Time(t, test.second)) || !second.After(first) { t.Fatal("second calendar occurrence", second, err) }
				previous, err := spec.Previous(second.Add(-time.Microsecond))
				if err != nil || !previous.Equal(first) { t.Fatal("half-open calendar predecessor", previous, err) }
			})
		}
		spec := jobs.Spec{Type: "interval", IntervalSeconds: 3600, Anchor: phase30Time(t, "2024-03-10T06:30:00Z"), Timezone: "America/New_York", Missed: "skip", Overlap: "queue"}
		at, err := spec.Next(spec.Anchor)
		if err != nil || !at.Equal(phase30Time(t, "2024-03-10T07:30:00Z")) { t.Fatal("elapsed interval was treated as a civil hour", at, err) }
	})
	t.Run("parameter-core-calendar-contract", func(t *testing.T) {
		for _, test := range []struct { name, mode, unit, zone, at, start, end string; invalid bool }{
			{"leap-month", "previous", "month", "UTC", "2024-03-01T00:00:00Z", "2024-02-01T00:00:00Z", "2024-03-01T00:00:00Z", false},
			{"leap-year", "previous", "year", "UTC", "2025-01-01T00:00:00Z", "2024-01-01T00:00:00Z", "2025-01-01T00:00:00Z", false},
			{"spring-short-day", "previous", "day", "America/New_York", "2024-03-11T04:00:00Z", "2024-03-10T05:00:00Z", "2024-03-11T04:00:00Z", false},
			{"fall-long-day", "previous", "day", "America/New_York", "2024-11-04T05:00:00Z", "2024-11-03T04:00:00Z", "2024-11-04T05:00:00Z", false},
			{"clamped-month", "rolling", "month", "UTC", "2024-03-31T12:00:00Z", "2024-02-29T12:00:00Z", "2024-03-31T12:00:00Z", false},
			{"ambiguous-civil-subtraction", "rolling", "day", "America/New_York", "2024-11-04T06:30:00Z", "", "", true},
			{"nonexistent-civil-subtraction", "rolling", "day", "America/New_York", "2024-03-11T06:30:00Z", "", "", true},
		} {
			t.Run(test.name, func(t *testing.T) {
				period := reporting.Period{Mode: test.mode, Unit: test.unit, Count: 1, DSTPolicy: "reject", MonthPolicy: "clamp"}
				parameters := []reporting.Parameter{{Name: "period", Type: "relative_period", Required: true, Default: &reporting.Value{Period: &period}}}
				resolved, err := reporting.ResolveParameters(parameters, nil, reporting.Resolution{At: phase30Time(t, test.at), Timezone: test.zone})
				if test.invalid { if !errors.Is(err, reporting.ErrInvalid) { t.Fatal("ambiguous/gap policy ignored", err) }; return }
				if err != nil || len(resolved.Parameters) != 2 || len(resolved.Values) != 1 || resolved.Values[0].Window == nil { t.Fatal("period not bound by existing core", resolved, err) }
				w := resolved.Values[0].Window
				if !w.Start.Equal(phase30Time(t, test.start)) || !w.End.Equal(phase30Time(t, test.end)) { t.Fatal("calendar window", w) }
			})
		}
	})
	t.Run("accepted-window-survives-delay-retry-and-read", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		d := phase27Copy(t, f.domain.base)
		d.SQL = "SELECT id, amount FROM analytics.sales WHERE $1::timestamptz < $2::timestamptz ORDER BY id"
		period := reporting.Period{Mode: "previous", Unit: "day", Count: 1, DSTPolicy: "reject", MonthPolicy: "clamp"}
		d.Parameters = []reporting.Parameter{{Name: "window", Type: "relative_period", Required: true, Default: &reporting.Value{Period: &period}}}
		f.domain.block(t, "p30-window", d)
		target := phase30Target("saved_sql", "p30-window")
		target.Timezone = "America/New_York"
		target.Arguments = []jobs.ReportingArgument{{Name: "window", Value: jobs.ReportingValue{Period: &jobs.ReportingPeriod{Mode: "schedule_window", FirstOccurrence: "reject", DSTPolicy: "reject", MonthPolicy: "clamp"}}}}
		request := phase30Manual(target)
		request.Spec = jobs.Spec{Type: "cron", Cron: "0 0 * * *", Timezone: target.Timezone, Missed: "catch_up", MaxCatchUp: 1, Overlap: "queue"}
		schedule := f.schedule(t, "fixed-dst-window", request)
		start, due := phase30Time(t, "2024-03-10T05:00:00Z"), phase30Time(t, "2024-03-11T04:00:00Z")
		f.cursorAt(t, schedule.ID, start, due)
		admitted, err := f.domain.f.f.db.TickSchedules(t.Context(), f.limits)
		if err != nil || admitted != 1 { t.Fatal("real bounded catch-up", admitted, err) }
		job := f.occurrence(t, schedule.ID, due)
		if !job.WindowStart.Equal(start) || !job.WindowEnd.Equal(due) || job.WindowEnd.Sub(job.WindowStart) != 23*time.Hour { t.Fatal("accepted window lost civil DST semantics", job) }
		f.mode.Store(2)
		if err := f.queue.RunOnce(t.Context()); err == nil { t.Fatal("issuer outage was hidden") }
		f.retryNow(t, job.ID)
		f.mode.Store(0)
		if err := f.queue.RunOnce(t.Context()); err != nil { t.Fatal("delayed real report", err, f.get(t, job.ID)) }
		done := f.get(t, job.ID)
		if done.State != "succeeded" || done.ManifestHash != job.ManifestHash || !done.WindowStart.Equal(start) || !done.DueAt.Equal(due) { t.Fatal("retry floated the accepted window", done) }
		proof, err := f.provider.Acquire(t.Context(), job)
		if err != nil { t.Fatal(err) }
		record, err := f.domain.f.f.db.ReadFrozenRun(t.Context(), proof.Envelope(), job.ID, true)
		if err != nil || record.Manifest == nil || len(record.Manifest.Resolved.Values) != 1 || record.Manifest.Resolved.Values[0].Window == nil { t.Fatal("missing actual retained bind proof", err) }
		w := record.Manifest.Resolved.Values[0].Window
		if !w.Start.Equal(start) || !w.End.Equal(due) || !record.Manifest.Resolved.At.Equal(due) { t.Fatal("domain consumer re-resolved against retry time", w) }
		history, err := f.queue.History(t.Context(), f.controls(t), schedule.ID, jobs.ScheduleHistoryRequest{Kind: "occurrences", Limit: 100})
		if err != nil || len(history.Occurrences) < 2 { t.Fatal("missed range was not retained", history, err) }
		skipped := false
		for _, occurrence := range history.Occurrences { if occurrence.SkippedThrough != nil { skipped = true } }
		if !skipped { t.Fatal("catch-up truncation silently discarded missed time") }
	})
	t.Run("first-occurrence-is-explicit", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		d := phase27Copy(t, f.domain.base)
		d.SQL = "SELECT id, amount FROM analytics.sales WHERE $1::timestamptz < $2::timestamptz ORDER BY id"
		period := reporting.Period{Mode: "previous", Unit: "day", Count: 1, DSTPolicy: "reject", MonthPolicy: "clamp"}
		d.Parameters = []reporting.Parameter{{Name: "window", Type: "relative_period", Required: true, Default: &reporting.Value{Period: &period}}}
		f.domain.block(t, "p30-first", d)
		target := phase30Target("saved_sql", "p30-first")
		target.Arguments = []jobs.ReportingArgument{{Name: "window", Value: jobs.ReportingValue{Period: &jobs.ReportingPeriod{Mode: "schedule_window", FirstOccurrence: "reject", DSTPolicy: "reject", MonthPolicy: "clamp"}}}}
		before := f.domain.attemptCount(t)
		job := f.submit(t, "reject-no-prior-window", target)
		if err := f.queue.RunOnce(t.Context()); err == nil { t.Fatal("first window was invented") }
		if done := f.get(t, job.ID); done.State != "blocked" || done.ErrorCode != "reporting_attention" || f.domain.attemptCount(t) != before { t.Fatal("first policy reached SQL", done) }
		target.Arguments[0].Value.Period.FirstOccurrence = "previous"
		target.Arguments[0].Value.Period.Unit, target.Arguments[0].Value.Period.Count = "day", 1
		job = f.submit(t, "declared-first-window", target)
		if err := f.queue.RunOnce(t.Context()); err != nil { t.Fatal("explicit first-window execution", err) }
		if done := f.get(t, job.ID); done.State != "succeeded" || f.domain.attemptCount(t) != before+1 { t.Fatal("explicit fallback was not executed", done) }
	})
}
