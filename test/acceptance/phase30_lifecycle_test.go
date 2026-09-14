package acceptance

import (
	"errors"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func (f *phase30Fixture) republishBlock(t *testing.T, id string, definition reporting.Definition) {
	t.Helper()
	current, err := f.domain.blocks.Read(t.Context(), f.domain.blockAuthor, id, reporting.Reference{})
	if err != nil {
		t.Fatal(err)
	}
	edited, err := f.domain.blocks.Edit(t.Context(), f.domain.blockAuthor, id, reporting.EditRequest{ExpectedVersion: current.State.Version, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, f.domain.blocks, f.domain.blockAuthor, edited)
}

func testPhase30Lifecycle(t *testing.T) {
	t.Run("durable-cas-management-and-history", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-lifecycle", f.domain.base)
		request := phase30Manual(phase30Target("saved_sql", "p30-lifecycle"))
		schedule := f.schedule(t, "lifecycle", request)
		manager := f.manager(t)
		replay := f.schedule(t, "lifecycle", request)
		if replay.ID != schedule.ID {
			t.Fatal("schedule create replay duplicated identity")
		}
		job, err := f.queue.TestSchedule(t.Context(), manager, schedule.ID, "explicit-test", schedule.Revision)
		if err != nil || job.ScheduleRevision != 1 {
			t.Fatal("test did not use normal occurrence admission", job, err)
		}
		again, err := f.queue.TestSchedule(t.Context(), manager, schedule.ID, "explicit-test", schedule.Revision)
		if err != nil || again.ID != job.ID {
			t.Fatal("manual test was not idempotent", again, err)
		}

		var wg sync.WaitGroup
		results := make(chan error, 8)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.queue.SetSchedule(t.Context(), manager, schedule.ID, 1, false)
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		wins := 0
		for err := range results {
			if err == nil {
				wins++
			} else if !errors.Is(err, store.ErrConflict) {
				t.Fatal("unexpected concurrent pause failure", err)
			}
		}
		if wins != 1 {
			t.Fatal("pause CAS had multiple winners", wins)
		}
		schedule, err = f.queue.GetSchedule(t.Context(), f.controls(t), schedule.ID)
		if err != nil || schedule.Enabled || schedule.Revision != 2 {
			t.Fatal("pause not persisted", schedule, err)
		}
		if _, err := f.queue.TestSchedule(t.Context(), manager, schedule.ID, "paused-test", 2); !errors.Is(err, store.ErrConflict) {
			t.Fatal("test bypassed pause", err)
		}
		request.Target.Reporting.Budget.MaxRows = 500
		if _, err := f.queue.ReplaceSchedule(t.Context(), manager, schedule.ID, 1, "stale-edit", request); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale edit was accepted", err)
		}
		schedule, err = f.queue.ReplaceSchedule(t.Context(), manager, schedule.ID, 2, "replace-future", request)
		if err != nil || schedule.Revision != 3 || schedule.Enabled || schedule.Request.Target.Reporting.Budget.MaxRows != 500 {
			t.Fatal("replacement changed pause or lost target", schedule, err)
		}
		replaced, err := f.queue.ReplaceSchedule(t.Context(), manager, schedule.ID, 2, "replace-future", request)
		if err != nil || replaced.Revision != 3 {
			t.Fatal("lost replacement reply was not safely replayable", replaced, err)
		}
		schedule, err = f.queue.SetSchedule(t.Context(), manager, schedule.ID, 3, true)
		if err != nil || schedule.Revision != 4 || !schedule.Enabled {
			t.Fatal("resume", schedule, err)
		}
		second, err := f.queue.TestSchedule(t.Context(), manager, schedule.ID, "changed-test", 4)
		if err != nil || second.ID == job.ID || second.Reporting.Target.Budget.MaxRows != 500 {
			t.Fatal("test did not use explicitly accepted replacement", second, err)
		}
		schedule, err = f.queue.RetireSchedule(t.Context(), manager, schedule.ID, 4)
		if err != nil || !schedule.Retired || schedule.Enabled || schedule.Revision != 5 {
			t.Fatal("irreversible retirement", schedule, err)
		}
		if _, err = f.queue.SetSchedule(t.Context(), manager, schedule.ID, 5, true); !errors.Is(err, store.ErrConflict) {
			t.Fatal("retirement resurrected by resume", err)
		}
		if _, err = f.queue.ReplaceSchedule(t.Context(), manager, schedule.ID, 5, "resurrect", request); !errors.Is(err, store.ErrConflict) {
			t.Fatal("retirement resurrected by replacement", err)
		}
		if _, err = f.queue.TestSchedule(t.Context(), manager, schedule.ID, "retired-test", 5); !errors.Is(err, store.ErrConflict) {
			t.Fatal("retired definition executed", err)
		}
		for range 2 {
			if err = f.queue.RunOnce(t.Context()); err != nil {
				t.Fatal("retirement mutated already accepted work", err)
			}
		}
		old := f.get(t, job.ID)
		if old.State != "succeeded" || old.ManifestHash != job.ManifestHash || old.Reporting.Target.Budget.MaxRows != 1000 {
			t.Fatal("update repinned earlier accepted run", old)
		}
		if newer := f.get(t, second.ID); newer.State != "succeeded" || newer.ScheduleRevision != 4 {
			t.Fatal("accepted replacement run", newer)
		}

		first, err := f.queue.History(t.Context(), f.controls(t), schedule.ID, jobs.ScheduleHistoryRequest{Kind: "revisions", Limit: 2})
		if err != nil || len(first.Revisions) != 2 || first.Revisions[0].Revision != 5 || !first.Revisions[0].Retired || first.NextBeforeRevision != 4 || len(first.Occurrences) != 0 {
			t.Fatal("bounded immutable revision history", first, err)
		}
		rest, err := f.queue.History(t.Context(), f.controls(t), schedule.ID, jobs.ScheduleHistoryRequest{Kind: "revisions", Limit: 100, BeforeRevision: first.NextBeforeRevision})
		if err != nil || len(rest.Revisions) != 3 || rest.Revisions[2].Revision != 1 || rest.Revisions[2].Request.Target.Reporting.Budget.MaxRows != 1000 {
			t.Fatal("revision cursor lost original definition", rest, err)
		}
		occurrences, err := f.queue.History(t.Context(), f.controls(t), schedule.ID, jobs.ScheduleHistoryRequest{Kind: "occurrences", Limit: 1})
		if err != nil || len(occurrences.Occurrences) != 1 || occurrences.NextBeforeDue == nil {
			t.Fatal("occurrence history is not bounded", occurrences, err)
		}
		remaining, err := f.queue.History(t.Context(), f.controls(t), schedule.ID, jobs.ScheduleHistoryRequest{Kind: "occurrences", Limit: 1, BeforeDue: occurrences.NextBeforeDue})
		if err != nil || len(remaining.Occurrences) != 1 || remaining.Occurrences[0].JobID == occurrences.Occurrences[0].JobID || remaining.NextBeforeDue != nil {
			t.Fatal("occurrence cursor repeated or omitted history", remaining, err)
		}
		raw := support.Raw(t, f.domain.f.f.dsn)
		if _, err := raw.Exec(t.Context(), `UPDATE chartworks.job_schedule_revisions SET enabled=false WHERE tenant_id=$1 AND schedule_id=$2 AND revision=1`, f.actor.Tenant(), schedule.ID); err == nil {
			t.Fatal("immutable history was writable")
		}
		if _, err := raw.Exec(t.Context(), `UPDATE chartworks.job_schedules SET retired=false,revision=revision+1 WHERE tenant_id=$1 AND schedule_id=$2`, f.actor.Tenant(), schedule.ID); err == nil {
			t.Fatal("persistent retirement guard was bypassed")
		}
	})
	t.Run("latest-is-resolved-once-per-occurrence", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-pins", f.domain.base)
		target := phase30Target("saved_sql", "p30-pins")
		target.LatestPublished, target.Revision = true, 0
		first := f.submit(t, "accepted-before-publication", target)
		d := phase27Copy(t, f.domain.base)
		d.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= 2 ORDER BY id"
		f.republishBlock(t, target.ID, d)
		second := f.submit(t, "accepted-after-publication", target)
		replayed := f.submit(t, "accepted-before-publication", target)
		if first.Reporting.Revision != 1 || second.Reporting.Revision != 2 || replayed.ID != first.ID || replayed.ManifestHash != first.ManifestHash {
			t.Fatal("acceptance/replay floated latest publication", first, second, replayed)
		}
		before := f.domain.attemptCount(t)
		for range 2 {
			if err := f.queue.RunOnce(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		if f.domain.attemptCount(t) != before+2 {
			t.Fatal("pinned runs did not execute exactly once")
		}
		oldView, err := f.delivery.View(t.Context(), f.domain.execute, reporting.ReportingViewRequest{Kind: "block", Run: first.ID, Output: "table-main", Limit: 10})
		if err != nil || oldView.PageBounds.Total != 2 {
			t.Fatal("earlier run used newer SQL", oldView, err)
		}
		newView, err := f.delivery.View(t.Context(), f.domain.execute, reporting.ReportingViewRequest{Kind: "block", Run: second.ID, Output: "table-main", Limit: 10})
		if err != nil || newView.PageBounds.Total != 1 {
			t.Fatal("new run did not use accepted new revision", newView, err)
		}

		doc := phase29Text("Accepted report revision")
		doc.Widgets = append(doc.Widgets, phase29BlockWidget("frozen", target.ID, 0, "table-main"))
		state := f.domain.report(t, "p30-report-pins", doc, true)
		reportTarget := phase30Target("report", state.ID)
		reportTarget.LatestPublished, reportTarget.Revision = true, 0
		reportJob := f.submit(t, "report-before-change", reportTarget)
		d.SQL = f.domain.base.SQL
		f.republishBlock(t, target.ID, d)
		doc.Metadata[0].Title = "A later report publication"
		edited, err := f.domain.documents.Edit(t.Context(), f.domain.author, "report", state.ID, state.Version, reporting.DocumentReference{}, doc)
		if err != nil {
			t.Fatal(err)
		}
		phase29Publish(t, f.domain.documents, f.domain.author, edited)
		later := f.submit(t, "report-after-change", reportTarget)
		if reportJob.Reporting.Revision != 1 || len(reportJob.Reporting.Pins) != 1 || reportJob.Reporting.Pins[0].Revision != 2 || later.Reporting.Revision != 2 || len(later.Reporting.Pins) != 1 || later.Reporting.Pins[0].Revision != 3 {
			t.Fatal("report/dependency pins were not resolved atomically", reportJob, later)
		}
		for range 2 {
			if err := f.queue.RunOnce(t.Context()); err != nil {
				t.Fatal("pinned report consumer", err)
			}
		}
		oldReport, err := f.delivery.View(t.Context(), f.domain.execute, reporting.ReportingViewRequest{Kind: "report", Run: reportJob.ID, Page: "main", Widget: "frozen", Output: "table-main", Limit: 10})
		if err != nil || oldReport.PageBounds.Total != 1 {
			t.Fatal("accepted report dependency floated", oldReport, err)
		}
	})
}
