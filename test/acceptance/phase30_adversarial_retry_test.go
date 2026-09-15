package acceptance

import (
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

// A successful group is not permission to publish on a later attempt. These
// cases change real business eligibility after durable query checkpoints and
// before a failed catalog transaction is retried with fresh broker authority.
func TestPhase30AdversarialRetryEligibility(t *testing.T) {
	for _, change := range []string{"archive-report", "withdraw-child-certification"} {
		t.Run(change, func(t *testing.T) {
			f := newPhase30Fixture(t, false)
			f.certify(t, "p30-retry-certified")
			d := phase29Text("Recheck current eligibility before delivery")
			widget := phase29BlockWidget("certified", "p30-retry-certified", 1, "table-main")
			widget.Block.Revision = 1
			widget.Block.Policy = "certified_only"
			d.Widgets = append(d.Widgets, widget)
			state := f.domain.report(t, "p30-retry-report", d, true)
			job := f.submit(t, "retry-eligibility", phase30Target("report", state.ID))
			raw := support.Raw(t, f.domain.f.f.dsn)
			_, err := raw.Exec(t.Context(), `CREATE FUNCTION chartworks.p30_adversarial_delivery() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.catalog_state='available' THEN RAISE EXCEPTION 'synthetic catalog outage' USING ERRCODE='58000'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER p30_adversarial_delivery BEFORE UPDATE ON chartworks.reporting_occurrence_delivery FOR EACH ROW EXECUTE FUNCTION chartworks.p30_adversarial_delivery();`)
			if err != nil {
				t.Fatal(err)
			}
			beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			if err := f.queue.RunOnce(t.Context()); err == nil {
				t.Fatal("synthetic publication outage was not exercised")
			}
			current := f.get(t, job.ID)
			if current.State != "retry" || current.Delivery == nil || current.Delivery.Catalog != "pending" || f.domain.attemptCount(t) != beforeQueries+1 {
				t.Fatal("fixture did not reach a retryable post-query publication boundary", current)
			}
			var completed int
			if err := raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.composition_run_groups WHERE tenant_id=$1 AND operation_id=$2 AND result->>'state'='completed'`, f.actor.Tenant(), job.ID).Scan(&completed); err != nil || completed != 1 {
				t.Fatal("fixture lacks its durable successful group", completed, err)
			}
			if _, err := raw.Exec(t.Context(), `DROP TRIGGER p30_adversarial_delivery ON chartworks.reporting_occurrence_delivery; DROP FUNCTION chartworks.p30_adversarial_delivery();`); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "archive-report":
				if _, err := f.domain.documents.Transition(t.Context(), f.domain.author, "report", state.ID, state.Version, 1, "archive", "Withdraw report eligibility before retry"); err != nil {
					t.Fatal(err)
				}
			case "withdraw-child-certification":
				block, err := f.domain.blocks.Read(t.Context(), f.domain.blockAuthor, "p30-retry-certified", reporting.Reference{})
				if err != nil || block.Trust.HistoricalAttestation == nil {
					t.Fatal("certification fixture", block, err)
				}
				if _, err := f.domain.blocks.Withdraw(t.Context(), f.domain.blockAuthor, "p30-retry-certified", reporting.WithdrawRequest{ExpectedVersion: block.State.Version, Revision: 1, Attestation: block.Trust.HistoricalAttestation.ID, Note: "Withdraw child eligibility before retry"}); err != nil {
					t.Fatal(err)
				}
			}
			f.retryNow(t, job.ID)
			err = f.queue.RunOnce(t.Context())
			current = f.get(t, job.ID)
			if err == nil || current.State != "blocked" || current.Delivery.Catalog == "available" || current.ManifestHash != job.ManifestHash {
				t.Fatal("P1: retry published after current business eligibility was withdrawn", change, current, err)
			}
			if f.domain.attemptCount(t) != beforeQueries+1 || f.domain.f.model.requests.Load() != beforeModels {
				t.Fatal("eligibility recheck regenerated protected work")
			}
		})
	}
}
