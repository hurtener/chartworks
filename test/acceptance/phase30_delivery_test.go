package acceptance

import (
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

func testPhase30Delivery(t *testing.T) {
	t.Run("ordinary-catalog-and-viewer-scheduled-provenance", testPhase30CatalogProvenance)
	t.Run("query-success-is-not-catalog-delivery", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-delivery", f.domain.base)
		target := phase30Target("saved_sql", "p30-delivery")
		target.Recipients = []string{"a-descriptive-recipient"}
		job := f.submit(t, "catalog-publication-failure", target)
		raw := support.Raw(t, f.domain.f.f.dsn)
		_, err := raw.Exec(t.Context(), `CREATE FUNCTION chartworks.p30_reject_delivery() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.catalog_state='available' THEN RAISE EXCEPTION 'synthetic catalog publication failure' USING ERRCODE='55000'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER p30_reject_delivery BEFORE UPDATE ON chartworks.reporting_occurrence_delivery FOR EACH ROW EXECUTE FUNCTION chartworks.p30_reject_delivery();`)
		if err != nil {
			t.Fatal(err)
		}
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		if err := f.queue.RunOnce(t.Context()); err == nil {
			t.Fatal("catalog publication failure hidden")
		}
		current := f.get(t, job.ID)
		if current.State != "retry" || current.Delivery == nil || current.Delivery.Query != "succeeded" || current.Delivery.Catalog != "pending" || current.Delivery.Notification != "not_requested" || f.domain.attemptCount(t) != beforeQueries+1 {
			t.Fatal("successful query falsely reported as delivered", current)
		}
		var catalog string
		var published *time.Time
		if err := raw.QueryRow(t.Context(), `SELECT catalog_state,published_at FROM chartworks.reporting_occurrence_delivery WHERE tenant_id=$1 AND operation_id=$2`, f.actor.Tenant(), job.ID).Scan(&catalog, &published); err != nil || catalog != "pending" || published != nil {
			t.Fatal("delivery transaction partially committed", catalog, published, err)
		}
		if _, err := raw.Exec(t.Context(), `DROP TRIGGER p30_reject_delivery ON chartworks.reporting_occurrence_delivery; DROP FUNCTION chartworks.p30_reject_delivery();`); err != nil {
			t.Fatal(err)
		}
		f.retryNow(t, job.ID)
		if err := f.queue.RunOnce(t.Context()); err != nil {
			t.Fatal("publication recovery", err)
		}
		current = f.get(t, job.ID)
		if current.State != "succeeded" || current.Attempts != 2 || current.Delivery.Artifact != "retained" || current.Delivery.Catalog != "available" || current.Delivery.Notification != "not_requested" || current.Delivery.Reserved.Queries != 1 {
			t.Fatal("recovered delivery has fabricated stage/usage evidence", current)
		}
		if f.domain.attemptCount(t) != beforeQueries+1 || f.domain.f.model.requests.Load() != beforeModels {
			t.Fatal("delivery retry reran the query or regenerated output")
		}
	})
	t.Run("recipient-and-creator-labels-grant-no-read-authority", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-recipient", f.domain.base)
		target := phase30Target("saved_sql", "p30-recipient")
		target.Recipients = []string{"recipient-only", "svc:untrusted"}
		job := f.submit(t, "recipient-labels", target)
		if err := f.queue.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
		selection := reporting.ReportingViewRequest{Kind: "block", Run: job.ID, Output: "table-main", Limit: 1}
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		for _, user := range []string{"recipient-only", "svc:untrusted", f.actor.User()} {
			reader := phase27Actor(t, f.domain.f, user, []string{"reporting.read", "cw.block.read:p30-recipient"})
			view, err := f.delivery.View(t.Context(), reader, selection)
			if err == nil || view.Output != nil || view.Text != nil {
				t.Fatal("descriptive identity bypassed current context reach", user, view, err)
			}
		}
		reader := phase27Actor(t, f.domain.f, "authorized-non-recipient", []string{"reporting.read", "cw.block.read:p30-recipient", "cw.execution_context.use:" + f.domain.base.Context})
		view, err := f.delivery.View(t.Context(), reader, selection)
		if err != nil || view.Output == nil {
			t.Fatal("independently authorized retained reader was rejected", view, err)
		}
		if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
			t.Fatal("recipient read performed protected execution")
		}
		if current := f.get(t, job.ID); current.Delivery.Notification != "not_requested" {
			t.Fatal("recipient metadata fabricated a notification receipt", current)
		}
	})
}

func testPhase30ChangedDependencies(t *testing.T) {
	t.Run("withdrawn-certification-blocks-already-accepted-run", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.certify(t, "p30-withdrawn")
		job := f.submit(t, "approved-then-withdrawn", phase30Target("block", "p30-withdrawn"))
		current, err := f.domain.blocks.Read(t.Context(), f.domain.blockAuthor, "p30-withdrawn", reporting.Reference{})
		if err != nil || current.Trust.HistoricalAttestation == nil {
			t.Fatal("actual certification fixture", current, err)
		}
		_, err = f.domain.blocks.Withdraw(t.Context(), f.domain.blockAuthor, "p30-withdrawn", reporting.WithdrawRequest{ExpectedVersion: current.State.Version, Revision: 1, Attestation: current.Trust.HistoricalAttestation.ID, Note: "Approval explicitly withdrawn before dispatch"})
		if err != nil {
			t.Fatal(err)
		}
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		if err := f.queue.RunOnce(t.Context()); err == nil {
			t.Fatal("withdrawn approval was treated as current")
		}
		if done := f.get(t, job.ID); done.State != "blocked" || done.ErrorCode != "reporting_attention" || done.ManifestHash != job.ManifestHash || done.Delivery.Catalog == "available" {
			t.Fatal("withdrawal was silently bypassed", done)
		}
		if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
			t.Fatal("withdrawn approval reached warehouse/model")
		}
	})
	t.Run("future-latest-publication-with-missing-output-needs-attention", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-removed-output", f.domain.base)
		target := phase30Target("saved_sql", "p30-removed-output")
		target.LatestPublished, target.Revision = true, 0
		request := phase30Manual(target)
		request.Spec = jobs.Spec{Type: "interval", IntervalSeconds: 60, Anchor: phase30Time(t, "2026-01-01T00:00:00Z"), Timezone: "UTC", Missed: "catch_up", MaxCatchUp: 1, Overlap: "queue"}
		schedule := f.schedule(t, "published-output-contract", request)
		d := phase27Copy(t, f.domain.base)
		d.Outputs = d.Outputs[:1]
		f.republishBlock(t, target.ID, d)
		due := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
		f.cursorAt(t, schedule.ID, due.Add(-time.Minute), due)
		if n, err := f.domain.f.f.db.TickSchedules(t.Context(), f.limits); err != nil || n != 1 {
			t.Fatal("real future occurrence admission", n, err)
		}
		job := f.occurrence(t, schedule.ID, due)
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		if err := f.queue.RunOnce(t.Context()); err == nil {
			t.Fatal("missing output was silently substituted")
		}
		done := f.get(t, job.ID)
		if done.State != "blocked" || done.ErrorCode != "reporting_attention" || done.Reporting.Revision != 2 || done.ManifestHash != job.ManifestHash || done.Delivery.Catalog == "available" {
			t.Fatal("missing output did not preserve its accepted failure", done)
		}
		if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
			t.Fatal("missing output caused query fallback")
		}
	})
	t.Run("unavailable-target-keeps-an-explicit-blocked-occurrence", func(t *testing.T) {
		f := newPhase30Fixture(t, false)
		f.domain.block(t, "p30-archived", f.domain.base)
		target := phase30Target("saved_sql", "p30-archived")
		request := phase30Manual(target)
		request.Spec = jobs.Spec{Type: "interval", IntervalSeconds: 60, Anchor: phase30Time(t, "2026-01-01T00:00:00Z"), Timezone: "UTC", Missed: "catch_up", MaxCatchUp: 1, Overlap: "queue"}
		schedule := f.schedule(t, "unavailable-target", request)
		// This fixture changes the real persisted availability state before the
		// next occurrence, without modifying any accepted manifest or authority.
		if _, err := support.Raw(t, f.domain.f.f.dsn).Exec(t.Context(), `UPDATE chartworks.block_heads SET archived=true,published_revision=NULL,version=version+1 WHERE tenant_id=$1 AND block_id=$2`, f.actor.Tenant(), target.ID); err != nil {
			t.Fatal(err)
		}
		due := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
		f.cursorAt(t, schedule.ID, due.Add(-time.Minute), due)
		if n, err := f.domain.f.f.db.TickSchedules(t.Context(), f.limits); err != nil || n != 1 {
			t.Fatal("unavailable target disappeared from schedule history", n, err)
		}
		job := f.occurrence(t, schedule.ID, due)
		if job.Reporting.Blocked != "dependency_unavailable" || job.Reporting.Revision != 0 {
			t.Fatal("unavailable publication acquired invented pins", job)
		}
		beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
		if err := f.queue.RunOnce(t.Context()); err == nil {
			t.Fatal("unavailable publication executed")
		}
		if done := f.get(t, job.ID); done.State != "blocked" || done.ManifestHash != job.ManifestHash || done.Delivery.Catalog == "available" {
			t.Fatal("unavailable target fell back to ambient work", done)
		}
		if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
			t.Fatal("unavailable target reached protected execution")
		}
	})
}
