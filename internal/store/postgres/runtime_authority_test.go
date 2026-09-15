package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestRuntimePersistenceRequiresAuthorityBeforeStorage(t *testing.T) {
	ctx := context.Background()
	// An absent database makes any accidental storage access fail immediately.
	// Public domain coordinates and zero-value proofs must never authorize it.
	var db *DB
	e := identity.Envelope{}
	checks := map[string]func() error{
		"proposal read": func() error {
			_, err := db.ReadAutopilotProposal(ctx, e, "proposal", "engineering.autopilot.read")
			return err
		},
		"amendment read": func() error {
			_, err := db.ReadAutopilotAmendment(ctx, e, "drift")
			return err
		},
		"proposal save": func() error {
			_, err := db.SaveAutopilotProposal(ctx, e, engineering.PreparedProposal{}, 0, 100)
			return err
		},
		"proposal review": func() error {
			_, err := db.ReviewAutopilotProposal(ctx, e, "proposal", engineering.AutopilotReviewRequest{ExpectedVersion: 1, Revision: 1, Decision: "approve", Reason: "Synthetic review"})
			return err
		},
		"pipeline stage": func() error {
			_, err := db.StageAutopilotPipeline(ctx, e, engineering.PreparedProposalApply{}, "publish")
			return err
		},
		"pipeline receipt": func() error {
			_, err := db.RecordAutopilotRun(ctx, e, engineering.PreparedProposalApply{}, engineering.PipelineRun{})
			return err
		},
		"topic receipt": func() error {
			_, err := db.RecordAutopilotTopic(ctx, e, engineering.PreparedProposalApply{}, engineering.ProposalTopicResult{})
			return err
		},
		"schedule receipt": func() error {
			_, err := db.RecordAutopilotSchedule(ctx, e, engineering.PreparedProposalApply{}, jobs.Schedule{ID: "schedule", Revision: 1})
			return err
		},
		"compensation": func() error {
			_, err := db.CompensateAutopilot(ctx, e, engineering.PreparedProposalApply{}, engineering.PipelineExecution{})
			return err
		},
		"drift": func() error {
			_, err := db.RecordAutopilotDrift(ctx, e, engineering.PreparedProposalApply{}, engineering.AutopilotDrift{Proposal: "proposal"}, time.Hour)
			return err
		},
		"run seal": func() error {
			_, err := db.SealFrozenRun(ctx, e, jobs.RequestTask{}, reporting.PreparedRun{})
			return err
		},
		"run checkpoint": func() error {
			_, err := db.CheckpointFrozenRun(ctx, jobs.Invocation{}, reporting.PreparedRunWrite{})
			return err
		},
		"run reuse": func() error {
			_, _, err := db.ReuseFrozenRun(ctx, jobs.Invocation{}, "run")
			return err
		},
		"run read": func() error {
			_, err := db.ReadFrozenRun(ctx, e, "run", false)
			return err
		},
		"run list": func() error {
			_, err := db.ListFrozenArtifacts(ctx, e, "", 10)
			return err
		},
		"run cancel": func() error {
			_, err := db.CancelFrozenRun(ctx, e, "run")
			return err
		},
		"retention": func() error {
			_, err := db.ExpireFrozenArtifacts(ctx, e, 10)
			return err
		},
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			if err := check(); err == nil {
				t.Fatal("unverified runtime operation accepted")
			}
		})
	}
}
