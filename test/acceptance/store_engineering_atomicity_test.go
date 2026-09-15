package acceptance

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

type storeProposalHooks struct {
	engineering.AutopilotRepository
	save       func(context.Context, identity.Envelope, engineering.PreparedProposal, int64, int) (engineering.AutopilotProposal, error)
	stage      func(context.Context, identity.Envelope, engineering.PreparedProposalApply, string) (engineering.AutopilotProposal, error)
	run        func(context.Context, identity.Envelope, engineering.PreparedProposalApply, engineering.PipelineRun) (engineering.AutopilotProposal, error)
	compensate func(context.Context, identity.Envelope, engineering.PreparedProposalApply, engineering.PipelineExecution) (engineering.AutopilotProposal, error)
	drift      func(context.Context, identity.Envelope, engineering.PreparedProposalApply, engineering.AutopilotDrift, time.Duration) (engineering.AutopilotDrift, error)
}

func (r *storeProposalHooks) SaveAutopilotProposal(ctx context.Context, e identity.Envelope, p engineering.PreparedProposal, expected int64, maximum int) (engineering.AutopilotProposal, error) {
	if r.save != nil {
		return r.save(ctx, e, p, expected, maximum)
	}
	return r.AutopilotRepository.SaveAutopilotProposal(ctx, e, p, expected, maximum)
}
func (r *storeProposalHooks) StageAutopilotPipeline(ctx context.Context, e identity.Envelope, p engineering.PreparedProposalApply, stage string) (engineering.AutopilotProposal, error) {
	if r.stage != nil {
		return r.stage(ctx, e, p, stage)
	}
	return r.AutopilotRepository.StageAutopilotPipeline(ctx, e, p, stage)
}
func (r *storeProposalHooks) RecordAutopilotRun(ctx context.Context, e identity.Envelope, p engineering.PreparedProposalApply, run engineering.PipelineRun) (engineering.AutopilotProposal, error) {
	if r.run != nil {
		return r.run(ctx, e, p, run)
	}
	return r.AutopilotRepository.RecordAutopilotRun(ctx, e, p, run)
}
func (r *storeProposalHooks) CompensateAutopilot(ctx context.Context, e identity.Envelope, p engineering.PreparedProposalApply, x engineering.PipelineExecution) (engineering.AutopilotProposal, error) {
	if r.compensate != nil {
		return r.compensate(ctx, e, p, x)
	}
	return r.AutopilotRepository.CompensateAutopilot(ctx, e, p, x)
}
func (r *storeProposalHooks) RecordAutopilotDrift(ctx context.Context, e identity.Envelope, p engineering.PreparedProposalApply, d engineering.AutopilotDrift, interval time.Duration) (engineering.AutopilotDrift, error) {
	if r.drift != nil {
		return r.drift(ctx, e, p, d, interval)
	}
	return r.AutopilotRepository.RecordAutopilotDrift(ctx, e, p, d, interval)
}

func proposalStoreSnapshot(t *testing.T, raw *pgx.Conn) string {
	t.Helper()
	return storeTableSnapshot(t, raw, "engineering_proposal_heads", "engineering_proposal_versions", "engineering_proposal_references", "engineering_proposal_reviews", "engineering_proposal_effects", "engineering_proposal_pipeline_effects", "engineering_proposal_events", "engineering_amendments", "pipeline_heads", "pipeline_versions", "pipeline_outputs", "sources")
}

func storeAutopilot(t *testing.T, f *phase26Fixture, repo engineering.AutopilotRepository) *engineering.Autopilot {
	t.Helper()
	auto, err := engineering.NewAutopilot(repo, f.pipelines, f.limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auto.Close)
	return auto
}

func storeApproveProposal(t *testing.T, f *phase26Fixture, p engineering.AutopilotProposal) engineering.AutopilotProposal {
	t.Helper()
	e := f.token.envelope(t, f.author.Tenant(), "store-independent-reviewer", phase26Scopes()...)
	out, err := f.auto.Review(context.Background(), e, p.ID, engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "approve", Reason: "Synthetic independent review of exact material"})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStoreProposalAuthoringRollback(t *testing.T) {
	f := newPhase26Fixture(t)
	raw := support.Raw(t, f.dsn)
	ctx := context.Background()
	for _, table := range []string{"engineering_proposal_heads", "engineering_proposal_versions", "engineering_proposal_references", "engineering_proposal_events", "audit_events"} {
		t.Run("create/"+table, func(t *testing.T) {
			before := proposalStoreSnapshot(t, raw)
			r := &storeProposalHooks{AutopilotRepository: f.db}
			r.save = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposal, expected int64, maximum int) (engineering.AutopilotProposal, error) {
				remove := storeWriteFault(t, raw, table, "INSERT", "")
				out, err := f.db.SaveAutopilotProposal(ctx, e, proof, expected, maximum)
				remove()
				return out, err
			}
			out, err := storeAutopilot(t, f, r).Propose(ctx, f.author, f.goal)
			if !errors.Is(err, store.ErrUnavailable) || out.ID != "" || proposalStoreSnapshot(t, raw) != before {
				t.Fatal("failed proposal creation left a head, material, reference or effect", err)
			}
		})
	}
	p, err := f.auto.Propose(ctx, f.author, f.goal)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ table, operation string }{{"engineering_proposal_versions", "INSERT"}, {"engineering_proposal_heads", "UPDATE"}, {"engineering_proposal_events", "INSERT"}} {
		t.Run("edit/"+tc.table, func(t *testing.T) {
			before := proposalStoreSnapshot(t, raw)
			remove := storeWriteFault(t, raw, tc.table, tc.operation, "")
			_, err := f.auto.Edit(ctx, f.author, p.ID, engineering.AutopilotEditRequest{ExpectedVersion: p.Version, Definition: p.Material.Pipeline, Reason: "Synthetic reviewed correction"})
			remove()
			if !errors.Is(err, store.ErrUnavailable) || proposalStoreSnapshot(t, raw) != before {
				t.Fatal("failed edit lost revision or review state", err)
			}
		})
	}
	reviewer := f.token.envelope(t, f.author.Tenant(), "store-reviewer", phase26Scopes()...)
	for _, tc := range []struct{ table, operation string }{{"engineering_proposal_reviews", "INSERT"}, {"engineering_proposal_heads", "UPDATE"}, {"engineering_proposal_events", "INSERT"}, {"audit_events", "INSERT"}} {
		t.Run("review/"+tc.table, func(t *testing.T) {
			before := proposalStoreSnapshot(t, raw)
			remove := storeWriteFault(t, raw, tc.table, tc.operation, "")
			out, err := f.auto.Review(ctx, reviewer, p.ID, engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "approve", Reason: "Synthetic independent review"})
			remove()
			if !errors.Is(err, store.ErrUnavailable) || out.Review != nil || proposalStoreSnapshot(t, raw) != before {
				t.Fatal("failed review installed approval or advanced its CAS", err)
			}
		})
	}
	approved := storeApproveProposal(t, f, p)
	if approved.Review == nil || approved.State != "approved" || approved.Version != p.Version+1 {
		t.Fatal("review after faults did not commit exactly once")
	}
}

func TestStoreProposalStageRollback(t *testing.T) {
	f := newPhase26Fixture(t)
	raw := support.Raw(t, f.dsn)
	ctx := context.Background()
	for index, tc := range []struct{ stage, table, operation, condition string }{
		{"draft", "pipeline_heads", "INSERT", ""},
		{"draft", "pipeline_versions", "INSERT", ""},
		{"draft", "audit_events", "INSERT", "NEW.action='pipeline.drafted'"},
		{"draft", "engineering_proposal_pipeline_effects", "INSERT", ""},
		{"draft", "engineering_proposal_effects", "INSERT", ""},
		{"draft", "engineering_proposal_heads", "UPDATE", ""},
		{"draft", "engineering_proposal_events", "INSERT", ""},
		{"publish", "pipeline_versions", "UPDATE", ""},
		{"publish", "pipeline_heads", "UPDATE", ""},
		{"publish", "audit_events", "INSERT", "NEW.action='pipeline.published'"},
		{"publish", "engineering_proposal_effects", "INSERT", ""},
	} {
		t.Run(tc.stage+"/"+tc.table, func(t *testing.T) {
			g := f.goal
			g.ID, g.Pipeline = fmt.Sprintf("store-stage-proposal-%d", index), fmt.Sprintf("store-stage-pipeline-%d", index)
			p, err := f.auto.Propose(ctx, f.author, g)
			if err != nil {
				t.Fatal(err)
			}
			p = storeApproveProposal(t, f, p)
			modelCalls := f.model.fixture.requests.Load()
			called := false
			r := &storeProposalHooks{AutopilotRepository: f.db}
			r.stage = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, stage string) (engineering.AutopilotProposal, error) {
				if stage != tc.stage {
					return f.db.StageAutopilotPipeline(ctx, e, proof, stage)
				}
				called = true
				before := proposalStoreSnapshot(t, raw)
				remove := storeWriteFault(t, raw, tc.table, tc.operation, tc.condition)
				out, err := f.db.StageAutopilotPipeline(ctx, e, proof, stage)
				remove()
				if proposalStoreSnapshot(t, raw) != before {
					t.Error("failed stage partially committed or erased an earlier completed stage")
				}
				return out, err
			}
			_, err = storeAutopilot(t, f, r).Apply(ctx, f.author, p.ID, phase26ApplyRequest(p))
			if !called || !errors.Is(err, store.ErrUnavailable) || f.model.fixture.requests.Load() != modelCalls {
				t.Fatal("apply did not stop at the injected stage, or repeated planning", called, err)
			}
			current, err := f.auto.Get(ctx, f.author, p.ID)
			if err != nil || current.Operation != "" || current.State == "applied" {
				t.Fatal("failed local stage started native execution", current.State, err)
			}
			wantEffects := 0
			if tc.stage == "publish" {
				wantEffects = 1 // The previously committed draft is real, not rolled back globally.
			}
			if len(current.Effects) != wantEffects {
				t.Fatal("actual completed effects were not preserved", len(current.Effects), wantEffects)
			}
		})
	}
}

func TestStoreProposalRunAndCompensationRollback(t *testing.T) {
	f := newPhase26Fixture(t)
	raw := support.Raw(t, f.dsn)
	ctx := context.Background()
	p, err := f.auto.Propose(ctx, f.author, f.goal)
	if err != nil {
		t.Fatal(err)
	}
	p = storeApproveProposal(t, f, p)
	r := &storeProposalHooks{AutopilotRepository: f.db}
	completed := false
	r.run = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, run engineering.PipelineRun) (engineering.AutopilotProposal, error) {
		if run.State != "published" {
			return f.db.RecordAutopilotRun(ctx, e, proof, run)
		}
		completed = true
		for _, tc := range []struct{ table, operation string }{{"engineering_proposal_effects", "INSERT"}, {"engineering_proposal_heads", "UPDATE"}, {"engineering_proposal_events", "INSERT"}, {"audit_events", "INSERT"}} {
			t.Run("run-receipt/"+tc.table, func(t *testing.T) {
				before := proposalStoreSnapshot(t, raw)
				remove := storeWriteFault(t, raw, tc.table, tc.operation, "")
				out, err := f.db.RecordAutopilotRun(ctx, e, proof, run)
				remove()
				if !errors.Is(err, store.ErrUnavailable) || out.State == "applied" || proposalStoreSnapshot(t, raw) != before {
					t.Fatal("failed effect receipt falsified managed completion", err)
				}
			})
		}
		return f.db.RecordAutopilotRun(ctx, e, proof, run)
	}
	auto := storeAutopilot(t, f, r)
	applied, err := auto.Apply(ctx, f.author, p.ID, phase26ApplyRequest(p))
	if err != nil || !completed || applied.State != "applied" {
		t.Fatal("real native effect or its final receipt failed", completed, applied.State, err)
	}
	t.Run("drift-amendments", func(t *testing.T) {
		storeDriftAmendmentPersistence(t, f, raw, applied)
	})
	r.compensate = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, x engineering.PipelineExecution) (engineering.AutopilotProposal, error) {
		// All attempts run while the production service holds native ownership locks.
		for _, tc := range []struct{ table, operation string }{{"sources", "UPDATE"}, {"pipeline_outputs", "DELETE"}, {"engineering_proposal_effects", "INSERT"}, {"pipeline_heads", "UPDATE"}, {"engineering_proposal_heads", "UPDATE"}, {"engineering_proposal_events", "INSERT"}, {"audit_events", "INSERT"}} {
			t.Run("compensation/"+tc.table, func(t *testing.T) {
				before := proposalStoreSnapshot(t, raw)
				remove := storeWriteFault(t, raw, tc.table, tc.operation, "")
				out, err := f.db.CompensateAutopilot(ctx, e, proof, x)
				remove()
				if !errors.Is(err, store.ErrUnavailable) || out.State == "compensated" || proposalStoreSnapshot(t, raw) != before {
					t.Fatal("failed compensation partially hid a managed generation", err)
				}
			})
		}
		return f.db.CompensateAutopilot(ctx, e, proof, x)
	}
	out, err := auto.Compensate(ctx, f.author, applied.ID, phase26ApplyRequest(applied))
	if err != nil || out.State != "compensated" {
		t.Fatal("bounded compensation after fault removal", out.State, err)
	}
	var baselineRows int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM analytics.sales`).Scan(&baselineRows); err != nil || baselineRows != 2 {
		t.Fatal("compensation changed the baseline warehouse", baselineRows, err)
	}
}
