package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

func TestStoreProposalProofAndReviewFences(t *testing.T) {
	// An absent caller context is invalid input, not a background authority fallback.
	var missingContext context.Context
	f := newPhase26Fixture(t)
	raw := support.Raw(t, f.dsn)
	ctx := context.Background()
	hooks := &storeProposalHooks{AutopilotRepository: f.db}
	var sealed engineering.PreparedProposal
	hooks.save = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposal, expected int64, maximum int) (engineering.AutopilotProposal, error) {
		sealed = proof
		for _, tc := range []struct {
			name     string
			ctx      context.Context
			expected int64
			maximum  int
			want     error
		}{
			{"context", nil, 0, maximum, access.ErrUnauthenticated},
			{"negative_revision", ctx, -1, maximum, store.ErrInvalid},
			{"revision_ceiling", ctx, 4096, maximum, store.ErrInvalid},
			{"zero_limit", ctx, 0, 0, store.ErrInvalid},
			{"excessive_limit", ctx, 0, 100001, store.ErrInvalid},
			{"missing_expected_head", ctx, 1, maximum, store.ErrConflict},
		} {
			t.Run("save/"+tc.name, func(t *testing.T) {
				before := proposalStoreSnapshot(t, raw)
				out, err := f.db.SaveAutopilotProposal(tc.ctx, e, proof, tc.expected, tc.maximum)
				if !errors.Is(err, tc.want) || out.ID != "" || proposalStoreSnapshot(t, raw) != before {
					t.Fatal("invalid persistence admission changed proposal", err)
				}
			})
		}
		return f.db.SaveAutopilotProposal(ctx, e, proof, expected, maximum)
	}
	auto := storeAutopilot(t, f, hooks)
	p, err := auto.Propose(ctx, f.author, f.goal)
	if err != nil {
		t.Fatal(err)
	}
	before := proposalStoreSnapshot(t, raw)
	replay, err := f.db.SaveAutopilotProposal(ctx, f.author, sealed, 0, f.limits.MaxProposals)
	if err != nil || replay.Digest != p.Digest || proposalStoreSnapshot(t, raw) != before {
		t.Fatal("same proposal proof did not replay exactly", err)
	}
	if _, err = f.db.SaveAutopilotProposal(ctx, f.author, sealed, 999, f.limits.MaxProposals); !errors.Is(err, store.ErrConflict) {
		t.Fatal("obsolete CAS accepted", err)
	}
	if _, err = f.db.ReadAutopilotProposal(missingContext, f.author, p.ID, "engineering.autopilot.read"); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err = f.db.ReadAutopilotAmendment(missingContext, f.author, "amendment"); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err = f.db.ReadAutopilotAmendment(ctx, f.author, "missing-amendment"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	for _, table := range []string{"engineering_proposal_heads", "engineering_proposal_effects"} {
		t.Run("read_unavailable/"+table, func(t *testing.T) {
			restore := storeHideTable(t, raw, table)
			out, err := f.db.ReadAutopilotProposal(ctx, f.author, p.ID, "engineering.autopilot.read")
			restore()
			if !errors.Is(err, store.ErrUnavailable) || out.Material.Author != "" {
				t.Fatal("partial private proposal returned on read failure", err)
			}
		})
	}
	reviewer := f.token.envelope(t, f.author.Tenant(), "store-independent-reviewer", phase26Scopes()...)
	review := engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "approve", Reason: "Synthetic independent review"}
	if _, err = f.db.ReviewAutopilotProposal(missingContext, reviewer, p.ID, review); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	p = storeApproveProposal(t, f, p)
	// Capture a real apply proof, withdraw its approval through the ordinary
	// independent-review operation, then attempt to use that now-obsolete proof.
	hooks.stage = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, stage string) (engineering.AutopilotProposal, error) {
		if stage != "draft" {
			t.Fatal("unapproved proposal reached publication")
		}
		for _, tc := range []struct {
			name string
			call func() error
			want error
		}{
			{"context", func() error { _, err := f.db.StageAutopilotPipeline(missingContext, e, proof, "draft"); return err }, access.ErrUnauthenticated},
			{"unknown_stage", func() error { _, err := f.db.StageAutopilotPipeline(ctx, e, proof, "execute"); return err }, store.ErrInvalid},
			{"publish_before_owned_draft", func() error { _, err := f.db.StageAutopilotPipeline(ctx, e, proof, "publish"); return err }, store.ErrConflict},
			{"empty_operation_receipt", func() error { _, err := f.db.RecordAutopilotRun(ctx, e, proof, engineering.PipelineRun{}); return err }, store.ErrInvalid},
			{"run_receipt_context", func() error {
				_, err := f.db.RecordAutopilotRun(missingContext, e, proof, engineering.PipelineRun{Operation: jobs.RequestTask{ID: "missing"}})
				return err
			}, access.ErrUnauthenticated},
			{"run_before_application", func() error {
				_, err := f.db.RecordAutopilotRun(ctx, e, proof, engineering.PipelineRun{Operation: jobs.RequestTask{ID: "missing"}})
				return err
			}, store.ErrConflict},
			{"unproposed_topic", func() error {
				_, err := f.db.RecordAutopilotTopic(ctx, e, proof, engineering.ProposalTopicResult{})
				return err
			}, store.ErrInvalid},
			{"unproposed_schedule", func() error {
				_, err := f.db.RecordAutopilotSchedule(ctx, e, proof, jobs.Schedule{ID: "missing"})
				return err
			}, store.ErrInvalid},
		} {
			t.Run("apply/"+tc.name, func(t *testing.T) {
				before := proposalStoreSnapshot(t, raw)
				if err := tc.call(); !errors.Is(err, tc.want) || proposalStoreSnapshot(t, raw) != before {
					t.Fatal("invalid effect altered proposal or pipeline", err)
				}
			})
		}
		rejected, err := f.db.ReviewAutopilotProposal(ctx, reviewer, p.ID, engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "reject", Reason: "Withdraw approval before any managed effect"})
		if err != nil || rejected.State != "rejected" {
			t.Fatal(err)
		}
		before := proposalStoreSnapshot(t, raw)
		out, err := f.db.StageAutopilotPipeline(ctx, e, proof, stage)
		if !errors.Is(err, engineering.ErrProposalReview) || proposalStoreSnapshot(t, raw) != before {
			t.Fatal("old proof bypassed the current review decision", err)
		}
		return out, err
	}
	if _, err = auto.Apply(ctx, f.author, p.ID, phase26ApplyRequest(p)); !errors.Is(err, engineering.ErrProposalReview) {
		t.Fatal(err)
	}
	current, err := f.auto.Get(ctx, f.author, p.ID)
	if err != nil || current.State != "rejected" || len(current.Effects) != 0 || current.Operation != "" {
		t.Fatal("withdrawn review acquired effects", err)
	}
	// The number of stored proposals is enforced at the store, before creating a
	// second head; rejected proposals do not silently disappear from the limit.
	hooks.save = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposal, _ int64, _ int) (engineering.AutopilotProposal, error) {
		return f.db.SaveAutopilotProposal(ctx, e, proof, 0, 1)
	}
	second := f.goal
	second.ID = "over-limit-proposal"
	second.Pipeline = "over-limit-pipeline"
	if _, err = auto.Propose(ctx, f.author, second); !errors.Is(err, engineering.ErrLimit) {
		t.Fatal("proposal storage limit ignored", err)
	}
}

// storeDriftAmendmentPersistence extends the already committed native generation.
// Reusing that real lifecycle avoids a second identical managed execution while
// still exercising source observation and every amendment write on PostgreSQL.
func storeDriftAmendmentPersistence(t *testing.T, f *phase26Fixture, raw *pgx.Conn, p engineering.AutopilotProposal) {
	t.Helper()
	ctx := context.Background()
	var err error
	// Age the synthetic completed observation, not immutable proposal material.
	// The service must derive freshness evidence rather than trusting a request flag.
	if _, err = raw.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET applied_at=clock_timestamp()-interval '2 minutes' WHERE tenant_id=$1 AND proposal_id=$2`, f.author.Tenant(), p.ID); err != nil {
		t.Fatal(err)
	}
	hooks := &storeProposalHooks{AutopilotRepository: f.db}
	snapshot := func() string {
		return storeTableSnapshot(t, raw, "engineering_amendments", "engineering_proposal_heads", "engineering_proposal_versions", "audit_events")
	}
	called := false
	hooks.drift = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, observation engineering.AutopilotDrift, interval time.Duration) (engineering.AutopilotDrift, error) {
		called = true
		for _, tc := range []struct {
			name     string
			ctx      context.Context
			interval time.Duration
			want     error
		}{
			{"context", nil, interval, access.ErrUnauthenticated},
			{"unbounded_dedup", ctx, 0, store.ErrInvalid},
			{"excessive_dedup", ctx, 8 * 24 * time.Hour, store.ErrInvalid},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before := snapshot()
				_, err := f.db.RecordAutopilotDrift(tc.ctx, e, proof, observation, tc.interval)
				if !errors.Is(err, tc.want) || snapshot() != before {
					t.Fatal("invalid drift admission changed state", err)
				}
			})
		}
		altered := observation
		altered.ID = "caller-selected-id"
		if _, err := f.db.RecordAutopilotDrift(ctx, e, proof, altered, interval); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("detached observation replaced sealed evidence", err)
		}
		for _, table := range []string{"engineering_proposal_heads", "engineering_amendments", "block_source_pins"} {
			t.Run("read_failure/"+table, func(t *testing.T) {
				before := snapshot()
				restore := storeHideTable(t, raw, table)
				out, err := f.db.RecordAutopilotDrift(ctx, e, proof, observation, interval)
				restore()
				if !errors.Is(err, store.ErrUnavailable) || out.ID != "" || snapshot() != before {
					t.Fatal("missing drift prerequisite published partial evidence", err)
				}
			})
		}
		for _, table := range []string{"engineering_amendments", "audit_events"} {
			t.Run("write_failure/"+table, func(t *testing.T) {
				before := snapshot()
				remove := storeWriteFault(t, raw, table, "INSERT", "")
				out, err := f.db.RecordAutopilotDrift(ctx, e, proof, observation, interval)
				remove()
				if !errors.Is(err, store.ErrUnavailable) || out.ID != "" || snapshot() != before {
					t.Fatal("failed drift write published partial amendment", err)
				}
			})
		}
		out, err := f.db.RecordAutopilotDrift(ctx, e, proof, observation, interval)
		if err != nil {
			return out, err
		}
		before := snapshot()
		duplicate, err := f.db.RecordAutopilotDrift(ctx, e, proof, observation, interval)
		if err != nil || duplicate.ID != out.ID || snapshot() != before {
			t.Fatal("duplicate drift was not idempotent", err)
		}
		return out, nil
	}
	auto := storeAutopilot(t, f, hooks)
	drift, err := auto.DetectDrift(ctx, f.author, p.ID)
	if err != nil || !called || drift.Kind != "freshness_expired" {
		t.Fatal("real drift did not reach persistence", called, err)
	}

	// A drift identifier is not enough to create a child proposal: the parent,
	// source pin, pipeline head and amendment link must all survive admission.
	hooks.drift = nil
	hooks.save = func(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposal, expected int64, maximum int) (engineering.AutopilotProposal, error) {
		material, err := proof.Checked(e)
		if err != nil || material.Origin == nil {
			t.Fatal("amendment proof missing its parent", err)
		}
		amendmentSnapshot := func() string {
			return proposalStoreSnapshot(t, raw) + storeTableSnapshot(t, raw, "engineering_amendment_proposals")
		}
		for _, table := range []string{"engineering_amendments", "pipeline_heads"} {
			t.Run("amendment/read_failure/"+table, func(t *testing.T) {
				before := amendmentSnapshot()
				restore := storeHideTable(t, raw, table)
				out, err := f.db.SaveAutopilotProposal(ctx, e, proof, expected, maximum)
				restore()
				if !errors.Is(err, store.ErrUnavailable) || out.ID != "" || amendmentSnapshot() != before {
					t.Fatal("amendment ignored missing parent evidence", err)
				}
			})
		}
		t.Run("amendment/link_write_failure", func(t *testing.T) {
			before := amendmentSnapshot()
			remove := storeWriteFault(t, raw, "engineering_amendment_proposals", "INSERT", "")
			out, err := f.db.SaveAutopilotProposal(ctx, e, proof, expected, maximum)
			remove()
			if !errors.Is(err, store.ErrUnavailable) || out.ID != "" || amendmentSnapshot() != before {
				t.Fatal("failed link left a detached amendment proposal", err)
			}
		})
		for _, state := range []string{"draft", "applying"} {
			t.Run("amendment/parent_"+state, func(t *testing.T) {
				if _, err := raw.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET state=$3 WHERE tenant_id=$1 AND proposal_id=$2`, e.Tenant(), p.ID, state); err != nil {
					t.Fatal(err)
				}
				before := amendmentSnapshot()
				out, err := f.db.SaveAutopilotProposal(ctx, e, proof, expected, maximum)
				unchanged := amendmentSnapshot() == before
				if _, restoreErr := raw.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET state='applied' WHERE tenant_id=$1 AND proposal_id=$2`, e.Tenant(), p.ID); restoreErr != nil {
					t.Fatal(restoreErr)
				}
				if !errors.Is(err, engineering.ErrState) || out.ID != "" || !unchanged {
					t.Fatal("amendment accepted an unfinished non-quality-failed parent", err)
				}
			})
		}
		return f.db.SaveAutopilotProposal(ctx, e, proof, expected, maximum)
	}
	amendment, err := auto.Amend(ctx, f.author, p.ID, engineering.AutopilotAmendRequest{Drift: drift.ID})
	if err != nil || amendment.Material.Origin == nil || amendment.AmendmentOf != p.ID || amendment.Review != nil {
		t.Fatal("valid amendment lost parent evidence or inherited approval", err)
	}
	reread, err := f.db.ReadAutopilotAmendment(ctx, f.author, drift.ID)
	if err != nil || reread.ID != amendment.ID {
		t.Fatal("persisted amendment link did not resolve", err)
	}
	if _, err = raw.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET applied_at=clock_timestamp()-interval '3 minutes' WHERE tenant_id=$1 AND proposal_id=$2`, f.author.Tenant(), p.ID); err != nil {
		t.Fatal(err)
	}
	before := snapshot()
	if _, err = f.auto.DetectDrift(ctx, f.author, p.ID); !errors.Is(err, engineering.ErrLimit) || snapshot() != before {
		t.Fatal("changed evidence evaded bounded dedup interval", err)
	}
	current, err := f.auto.Get(ctx, f.author, p.ID)
	if err != nil || current.Digest != p.Digest || current.State != "applied" || current.Revision != p.Revision {
		t.Fatal("drift rewrote reviewed definitions", err)
	}
}
