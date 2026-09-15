package acceptance

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store/postgres"
)

// Interleave an actual authoring change immediately before the real final
// checkpoint transaction. All admission, ownership, persistence and publication
// still use the production repository and broker, not a success-returning fake.
type phase30CheckpointInterleave struct {
	*postgres.DB
	onComplete func()
}

func (r *phase30CheckpointInterleave) CheckpointComposition(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedCompositionWrite) (reporting.CompositionRecord, error) {
	write, err := proof.Checked(inv)
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	if write.Kind == "complete" && r.onComplete != nil {
		fn := r.onComplete
		r.onComplete = nil
		fn()
	}
	return r.DB.CheckpointComposition(ctx, inv, proof)
}

func TestPhase30AdversarialPublicationBoundary(t *testing.T) {
	for _, change := range []string{"unchanged", "archive-report", "withdraw-child-certification"} {
		t.Run(change, func(t *testing.T) {
			f := newPhase30Fixture(t, false)
			f.certify(t, "p30-checkpoint-child")
			d := phase29Text("Current eligibility at the publication transaction")
			widget := phase29BlockWidget("certified", "p30-checkpoint-child", 1, "table-main")
			widget.Block.Revision, widget.Block.Policy = 1, "certified_only"
			d.Widgets = append(d.Widgets, widget)
			state := f.domain.report(t, "p30-checkpoint-report", d, true)
			repo := &phase30CheckpointInterleave{DB: f.domain.f.f.db}
			interleaved := false
			repo.onComplete = func() {
				interleaved = true
				switch change {
				case "archive-report":
					if _, err := f.domain.documents.Transition(t.Context(), f.domain.author, "report", state.ID, state.Version, 1, "archive", "Withdraw before catalog transaction"); err != nil {
						t.Fatal(err)
					}
				case "withdraw-child-certification":
					block, err := f.domain.blocks.Read(t.Context(), f.domain.blockAuthor, "p30-checkpoint-child", reporting.Reference{})
					if err != nil || block.Trust.HistoricalAttestation == nil {
						t.Fatal("certification fixture", block, err)
					}
					if _, err := f.domain.blocks.Withdraw(t.Context(), f.domain.blockAuthor, "p30-checkpoint-child", reporting.WithdrawRequest{ExpectedVersion: block.State.Version, Revision: 1, Attestation: block.Trust.HistoricalAttestation.ID, Note: "Withdraw before catalog transaction"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			runner, err := jobs.NewRequestRunner(f.domain.f.f.db, f.limits)
			if err != nil {
				t.Fatal(err)
			}
			compositions, err := reporting.NewCompositions(f.domain.documents, repo, f.domain.runs, reporting.DocumentsFromQueries(f.domain.query), runner)
			if err != nil {
				t.Fatal(err)
			}
			delivery, err := reporting.NewDelivery(f.domain.blocks, f.domain.runs, f.domain.documents, compositions, f.domain.f.f.db, f.domain.limits.Viewer)
			if err != nil {
				t.Fatal(err)
			}
			scheduled, err := reporting.NewScheduled(delivery, f.domain.f.f.db)
			if err != nil {
				t.Fatal(err)
			}
			f.queue, err = jobs.NewWithReporting(f.domain.f.f.db, f.provider, f.limits, nil, scheduled)
			if err != nil {
				t.Fatal(err)
			}
			job := f.submit(t, "checkpoint-eligibility", phase30Target("report", state.ID))
			beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			err = f.queue.RunOnce(t.Context())
			current := f.get(t, job.ID)
			if !interleaved || current.ManifestHash != job.ManifestHash || f.domain.attemptCount(t) != beforeQueries+1 || f.domain.f.model.requests.Load() != beforeModels {
				t.Fatal("publication boundary was not exercised without regenerating work", interleaved, current, err)
			}
			if change != "unchanged" {
				if err == nil || current.State != "blocked" || current.Delivery == nil || current.Delivery.Catalog == "available" {
					t.Fatal("P1: current eligibility was not enforced atomically with publication", current, err)
				}
				return
			}
			if err != nil || current.State != "succeeded" || current.Delivery == nil || current.Delivery.Catalog != "available" {
				t.Fatal("eligible publication refused", current, err)
			}
			// A completed artifact is historical evidence, not a new execution.
			// Archiving its authoring object must not invoke execution eligibility
			// on the separate authorized retained-read path.
			if _, err := f.domain.documents.Transition(t.Context(), f.domain.author, "report", state.ID, state.Version, 1, "archive", "Archive after successful publication"); err != nil {
				t.Fatal(err)
			}
			view, err := delivery.View(t.Context(), f.domain.execute, reporting.DeliveryViewRequest{Kind: "report", Run: job.ID, Page: "main", Widget: "certified", Output: "table-main", Limit: 10})
			if err != nil || view.Output == nil || view.Summary.Run != job.ID || f.domain.attemptCount(t) != beforeQueries+1 || f.domain.f.model.requests.Load() != beforeModels {
				t.Fatal("historical retained read was confused with a new execution", view, err)
			}
		})
	}
}
