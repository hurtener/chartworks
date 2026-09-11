package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
)

// Planning uses the real source, validator, gateway and PostgreSQL. The runner
// is deliberately non-executable: these tests must never perform managed writes.
// TestPhase26 separately requires the pinned native executable and Linux sandbox.
func newPhase26PlanningFixture(t *testing.T) *phase26Fixture {
	t.Helper()
	provider := newGatewayFixture(t, nil)
	model := &phase26Model{Engine: provider.engine, fixture: provider}
	scratch := t.TempDir()
	runner := filepath.Join(scratch, "must-not-execute")
	artifact := []byte("Planning-only negative execution sentinel.\n")
	if err := os.WriteFile(runner, artifact, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(artifact)
	base := newEngineeringFixture(t, func(v *config.Values) {
		v.Pipelines.Enabled = true
		v.Pipelines.RunnerPath = runner
		v.Pipelines.RunnerSHA256 = hex.EncodeToString(digest[:])
		v.Pipelines.TempDir = scratch
	}, model)
	service, err := engineering.NewPipelineService(base.db, base.s, base.validator, model, base.values, base.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	return phase26FixtureFromPipeline(t, model, &pipelineFixture{engineeringFixture: base, pipelines: service})
}

func TestProposalRejectionAndRevisionFence(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	ctx := context.Background()
	p := f.propose(t)
	review := engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "reject", Reason: "Revise the reviewed projection before execution."}
	rejected, err := f.reviewer.ReviewEngineeringProposal(ctx, p.ID, review)
	if err != nil || rejected.State != "rejected" {
		t.Fatal("reject exact revision", rejected, err)
	}
	if _, err := f.client.ApplyEngineeringProposal(ctx, p.ID, phase26ApplyRequest(rejected)); err == nil {
		t.Fatal("rejected proposal was executable")
	}
	edit := engineering.AutopilotEditRequest{ExpectedVersion: rejected.Version, Definition: rejected.Material.Pipeline, Reason: "Resubmit the independently reviewed projection."}
	revised, err := f.client.EditEngineeringProposal(ctx, p.ID, edit)
	if err != nil || revised.State != "draft" || revised.Revision != rejected.Revision+1 || revised.Version <= rejected.Version {
		t.Fatal("rejection revision", revised, err)
	}
	if _, err := f.client.EditEngineeringProposal(ctx, p.ID, edit); err == nil {
		t.Fatal("stale edit overwrote current material")
	}
	review.Decision = "approve"
	if _, err := f.reviewer.ReviewEngineeringProposal(ctx, p.ID, review); err == nil {
		t.Fatal("old review approved new revision")
	}
	approved := f.approve(t, revised)
	if approved.Revision != revised.Revision || approved.Digest != revised.Digest {
		t.Fatal("approval changed material")
	}
}

func TestProposalConcurrentEditsPreserveOneRevision(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	p := f.propose(t)
	type result struct {
		proposal engineering.AutopilotProposal
		err      error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, reason := range []string{"First independently submitted correction.", "Second independently submitted correction."} {
		go func(reason string) {
			<-start
			proposal, err := f.client.EditEngineeringProposal(context.Background(), p.ID, engineering.AutopilotEditRequest{ExpectedVersion: p.Version, Definition: p.Material.Pipeline, Reason: reason})
			results <- result{proposal, err}
		}(reason)
	}
	close(start)
	var winner engineering.AutopilotProposal
	successes := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			successes++
			winner = result.proposal
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent CAS accepted %d edits", successes)
	}
	current, err := f.auto.Get(context.Background(), f.author, p.ID)
	if err != nil || current.Digest != winner.Digest || current.Version != p.Version+1 || current.Revision != p.Revision+1 || current.State != "draft" {
		t.Fatal("persisted winner is not the sole next revision", current, err)
	}
	f.approve(t, current)
}
