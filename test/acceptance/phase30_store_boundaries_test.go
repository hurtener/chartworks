package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func testPhase30StoreBoundaries(t *testing.T) {
	t.Run("partial-artifact-is-not-complete-delivery", testPhase30PartialDelivery)
	t.Run("retirement-is-atomic-with-history-and-audit", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-atomic-retire", f.domain.base)
		s := f.schedule(t, "atomic-retire", phase30Manual(phase30Target("saved_sql", "p30-atomic-retire")))
		raw := support.Raw(t, f.domain.f.f.dsn)
		execSQL := func(sql string) {
			t.Helper()
			if _, err := raw.Exec(t.Context(), sql); err != nil {
				t.Fatal(err)
			}
		}
		for _, table := range []string{"job_schedules", "audit_events"} {
			// Both are fixed schema names, never request-derived SQL.
			execSQL(`CREATE FUNCTION chartworks.p30_storage_fault() RETURNS trigger LANGUAGE plpgsql AS $$
                BEGIN RAISE EXCEPTION 'synthetic retirement storage fault' USING ERRCODE='58000'; END; $$;
                CREATE TRIGGER p30_storage_fault BEFORE INSERT OR UPDATE ON chartworks.` + table + `
                FOR EACH ROW EXECUTE FUNCTION chartworks.p30_storage_fault();`)
			out, err := f.queue.RetireSchedule(t.Context(), f.manager(t), s.ID, 1)
			execSQL(`DROP TRIGGER p30_storage_fault ON chartworks.` + table + `;DROP FUNCTION chartworks.p30_storage_fault();`)
			if !errors.Is(err, store.ErrUnavailable) || out.ID != "" {
				t.Fatal("failed retirement disclosed or committed a result", table, out, err)
			}
			current, err := f.queue.GetSchedule(t.Context(), f.controls(t), s.ID)
			if err != nil || current.Revision != 1 || current.Retired || !current.Enabled {
				t.Fatal("failed retirement altered current state", current, err)
			}
			history, err := f.queue.History(t.Context(), f.controls(t), s.ID, jobs.ScheduleHistoryRequest{Kind: "revisions", Limit: 10})
			if err != nil || len(history.Revisions) != 1 || history.Revisions[0].Retired {
				t.Fatal("failed retirement appended history", history, err)
			}
		}
		if _, err := f.queue.RetireSchedule(t.Context(), f.manager(t), "absent-schedule", 1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("absent retirement", err)
		}
		if _, err := f.queue.RetireSchedule(t.Context(), f.manager(t), s.ID, 2); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale retirement", err)
		}
		retired, err := f.queue.RetireSchedule(t.Context(), f.manager(t), s.ID, 1)
		if err != nil || !retired.Retired || retired.Revision != 2 {
			t.Fatal("recovery after rollback", retired, err)
		}
		if _, err := f.queue.RetireSchedule(t.Context(), f.manager(t), s.ID, 2); !errors.Is(err, store.ErrConflict) {
			t.Fatal("duplicate retirement mutated history", err)
		}
	})
	t.Run("history-does-not-return-partial-metadata-on-storage-failure", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-history-errors", f.domain.base)
		s := f.schedule(t, "history-errors", phase30Manual(phase30Target("saved_sql", "p30-history-errors")))
		raw := support.Raw(t, f.domain.f.f.dsn)
		request := jobs.ScheduleHistoryRequest{Kind: "revisions", Limit: 10}
		if out, err := f.queue.History(t.Context(), f.controls(t), "absent", request); !errors.Is(err, store.ErrNotFound) || out.ScheduleID != "" {
			t.Fatal("absent history", out, err)
		}
		for _, test := range []struct{ kind, table string }{{"revisions", "job_schedule_revisions"}, {"occurrences", "job_occurrences"}} {
			if _, err := raw.Exec(t.Context(), `ALTER TABLE chartworks.`+test.table+` RENAME TO p30_offline`); err != nil {
				t.Fatal(err)
			}
			request.Kind = test.kind
			out, err := f.queue.History(t.Context(), f.controls(t), s.ID, request)
			if _, restoreErr := raw.Exec(t.Context(), `ALTER TABLE chartworks.p30_offline RENAME TO `+test.table); restoreErr != nil {
				t.Fatal(restoreErr)
			}
			if err == nil || out.ScheduleID != "" || out.Revisions != nil || out.Occurrences != nil {
				t.Fatal("failed history query returned metadata", out, err)
			}
		}
		// Model externally corrupted retained metadata explicitly. Production
		// writes cannot do this: the immutable-history trigger remains installed.
		if _, err := raw.Exec(t.Context(), `ALTER TABLE chartworks.job_schedule_revisions DISABLE TRIGGER schedule_history_immutable;
            UPDATE chartworks.job_schedule_revisions SET request='{}';
            ALTER TABLE chartworks.job_schedule_revisions ENABLE TRIGGER schedule_history_immutable`); err != nil {
			t.Fatal(err)
		}
		request.Kind = "revisions"
		if out, err := f.queue.History(t.Context(), f.controls(t), s.ID, request); !errors.Is(err, store.ErrUnavailable) || out.ScheduleID != "" || out.Revisions != nil {
			t.Fatal("corrupt history silently interpreted", out, err)
		}
	})
	t.Run("nested-children-cannot-borrow-or-rewrite-root-ownership", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.report(t, "p30-nested-owner", phase29Text("Owned report"), true)
		job := f.submit(t, "nested-parent", phase30Target("report", "p30-nested-owner"))
		_, proof, inv := f.claim(t, job.ID, "nested-parent-owner")
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		db := f.domain.f.f.db
		input := jobs.RequestInput{Kind: "reporting.run", Target: "child-block", InputHash: strings.Repeat("a", 64)}
		child, err := db.AdmitNestedRequest(t.Context(), inv, "one-child", input, f.limits)
		if err != nil {
			t.Fatal("real child admission", err)
		}
		// Replaying through the root API may not erase the immutable parent.
		if _, err := db.AdmitRequest(t.Context(), proof.Envelope(), "one-child", input, f.limits); !errors.Is(err, store.ErrConflict) {
			t.Fatal("child replay became independent work", err)
		}
		if _, err := db.ClaimRequest(t.Context(), proof.Envelope(), child.ID, "independent-owner", f.limits); !errors.Is(err, store.ErrConflict) {
			t.Fatal("root runner stole child ownership", err)
		}
		ordinary, err := db.AdmitRequest(t.Context(), proof.Envelope(), "ordinary-child-shape", input, f.limits)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ClaimNestedRequest(t.Context(), inv, ordinary.ID, "borrowed-owner", f.limits); !errors.Is(err, store.ErrConflict) {
			t.Fatal("ordinary root stole parent's execution slot", err)
		}
		if _, err := db.ClaimNestedRequest(t.Context(), inv, "absent-child", "owner", f.limits); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("absent nested claim", err)
		}
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := db.ClaimNestedRequest(cancelled, inv, child.ID, "owner", f.limits); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled nested claim", err)
		}
		lease, err := db.ClaimNestedRequest(t.Context(), inv, child.ID, "child-owner", f.limits)
		if err != nil || lease.Task.ID != child.ID || lease.Attempt != 1 {
			t.Fatal("legitimate owned child could not claim", lease, err)
		}
		if _, err := db.ClaimNestedRequest(t.Context(), inv, child.ID, "concurrent-owner", f.limits); !errors.Is(err, store.ErrConflict) {
			t.Fatal("second live child owner", err)
		}
		if out, err := db.SealScheduledComposition(t.Context(), inv, reporting.PreparedComposition{}); err == nil || out.Manifest.ID != "" {
			t.Fatal("unprepared composition sealed", out, err)
		}
		raw := support.Raw(t, f.domain.f.f.dsn)
		if _, err := raw.Exec(t.Context(), `UPDATE chartworks.operations SET lease_until=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2`, job.Tenant, job.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.AdmitNestedRequest(t.Context(), inv, "stale-parent-child", input, f.limits); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale parent admitted a new child", err)
		}
		if _, err := db.ClaimNestedRequest(t.Context(), inv, child.ID, "stale-parent-owner", f.limits); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale parent claimed a child", err)
		}
		if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
			t.Fatal("ownership metadata checks invoked source/model")
		}
	})
	t.Run("scheduled-seals-and-reservations-reject-inappropriate-ownership", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-seal-authority", f.domain.base)
		job := f.submit(t, "seal-authority", phase30Target("saved_sql", "p30-seal-authority"))
		_, _, inv := f.claim(t, job.ID, "seal-owner")
		db := f.domain.f.f.db
		if out, err := db.SealScheduledFrozenRun(t.Context(), inv, reporting.PreparedRun{}); err == nil || out.Manifest != nil {
			t.Fatal("unprepared frozen proof was sealed", out, err)
		}
		if out, err := db.SealScheduledComposition(t.Context(), inv, reporting.PreparedComposition{}); !errors.Is(err, jobs.ErrAuthority) || out.Manifest.ID != "" {
			t.Fatal("block occurrence became a report", out, err)
		}
		for _, charge := range []jobs.ReportingCharge{{}, {Queries: -1}, {Queries: 2}, {ModelTokens: 100}, {Queries: 1, ModelCalls: 1, ModelTokens: 100}} {
			if err := db.ReserveReportingUsage(t.Context(), inv, charge); !errors.Is(err, jobs.ErrInvalid) {
				t.Fatal("malformed reservation accepted", charge, err)
			}
		}
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		if err := db.ReserveReportingUsage(cancelled, inv, jobs.ReportingCharge{Queries: 1}); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled reservation", err)
		}
		raw := support.Raw(t, f.domain.f.f.dsn)
		if _, err := raw.Exec(t.Context(), `CREATE FUNCTION chartworks.p30_usage_fault() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN RAISE EXCEPTION 'synthetic reservation storage fault' USING ERRCODE='58000'; END; $$;
            CREATE TRIGGER p30_usage_fault BEFORE UPDATE ON chartworks.reporting_occurrence_delivery FOR EACH ROW EXECUTE FUNCTION chartworks.p30_usage_fault()`); err != nil {
			t.Fatal(err)
		}
		err := db.ReserveReportingUsage(t.Context(), inv, jobs.ReportingCharge{Queries: 1})
		if _, restoreErr := raw.Exec(t.Context(), `DROP TRIGGER p30_usage_fault ON chartworks.reporting_occurrence_delivery;DROP FUNCTION chartworks.p30_usage_fault()`); restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if !errors.Is(err, store.ErrUnavailable) {
			t.Fatal("reservation outage lost its public class", err)
		}
		if current := f.get(t, job.ID); current.Delivery.Reserved.Queries != 0 || current.Delivery.Catalog != "pending" {
			t.Fatal("failed reservation partially committed", current)
		}
		if err := db.ReserveReportingUsage(t.Context(), inv, jobs.ReportingCharge{Queries: 1}); err != nil {
			t.Fatal("reservation after rollback", err)
		}
		if current := f.get(t, job.ID); current.Delivery.Reserved.Queries != 1 {
			t.Fatal("successful reservation not persisted", current)
		}
	})
}

func testPhase30PartialDelivery(t *testing.T) {
	f := newPhase30Fixture(t, false)
	definition := phase27Definition(t, f.domain.f, f.domain.blockAuthor, f.domain.base.SQL)
	f.domain.block(t, "p30-output-failure", definition)
	for _, policy := range []string{"fail_closed", "allow_partial"} {
		doc := phase29Text("Independent table and unavailable narrative")
		doc.PartialFailure = policy
		table := phase29BlockWidget("table", "p30-output-failure", 1, "table-main")
		narrative := phase29BlockWidget("narrative", "p30-output-failure", 2, "narrative-main")
		narrative.Block.Narrative = true
		doc.Widgets = append(doc.Widgets, table, narrative)
		id := "p30-partial-" + policy
		f.domain.report(t, id, doc, true)
		target := phase30Target("report", id)
		target.Narrative = true
		target.Budget.ModelCalls, target.Budget.ModelTokens = 8, 128<<10
		target.PartialFailure = policy
		job := f.submit(t, "partial-"+policy, target)
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		runErr := f.queue.RunOnce(t.Context())
		got := f.get(t, job.ID)
		if got.Delivery == nil || got.Delivery.Notification != "not_requested" || got.ManifestHash != job.ManifestHash {
			t.Fatal("failure lost accepted provenance", got)
		}
		if f.domain.attemptCount(t) != beforeQueries+1 || f.domain.f.model.requests.Load() != beforeModels {
			t.Fatal("output failure duplicated data/model work")
		}
		if policy == "fail_closed" {
			// A successfully processed worker attempt can persist a blocked business
			// outcome. The durable receipt, not RunOnce's transport error, is authoritative.
			if runErr != nil || got.State != "blocked" || got.ErrorCode != "reporting_attention" ||
				got.Delivery.Query != "failed" || got.Delivery.Catalog != "unavailable" {
				t.Fatal("strict report claimed complete delivery", got, runErr)
			}
		} else {
			if runErr != nil || got.State != "succeeded" || got.Delivery.Query != "partial" || got.Delivery.Artifact != "retained" || got.Delivery.Catalog != "available" {
				t.Fatal("partial receipt lost successful independent outputs", got, runErr)
			}
			view, err := f.delivery.View(t.Context(), f.domain.execute, reporting.DeliveryViewRequest{Kind: "report", Run: job.ID, Page: "main", Widget: "table", Output: "table-main", Limit: 1})
			if err != nil || view.Output == nil || view.Summary.State != "partial" || view.Summary.Scheduled == nil || view.Summary.Scheduled.Query != "partial" {
				t.Fatal("ordinary viewer fabricated complete scheduled output", view, err)
			}
		}
	}
}
