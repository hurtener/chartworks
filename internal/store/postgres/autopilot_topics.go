package postgres

import (
	"context"
	"fmt"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// RecordAutopilotTopic verifies the ordinary private draft and managed execution
// before committing the final proposal effect. A caller-supplied receipt is not proof.
func (d *DB) RecordAutopilotTopic(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, result engineering.ProposalTopicResult) (out engineering.AutopilotProposal, err error) {
	v, err := proof.Checked(e, "engineering.autopilot.apply")
	if err != nil {
		return out, err
	}
	if v.Material.Topic == nil || v.Material.Request.Topic == nil {
		return out, store.ErrInvalid
	}
	if err = drafts.RequirePack(e, *v.Material.Topic, drafts.Write); err != nil {
		return out, err
	}
	model, err := semantics.Compile(*v.Material.Topic)
	if err != nil {
		return out, err
	}
	g := v.Material.Request.Topic
	if result.Topic != g.Topic || result.Revision != g.ExpectedRevision+1 || result.Digest != model.Digest() {
		return out, store.ErrInvalid
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, readErr := proposalApplyTx(ctx, tx, e, v)
		if readErr != nil {
			return readErr
		}
		if p.State != "applying" && p.State != "applied" || p.ApplyActor != e.User() || p.ApplySession != e.Session() {
			return store.ErrConflict
		}
		execution, readErr := readPipelineExecutionTx(ctx, tx, e, p.Operation)
		if readErr != nil {
			return readErr
		}
		if execution.State != "published" || execution.Operation.State != "succeeded" {
			return store.ErrConflict
		}
		var actualDigest, note string
		readErr = tx.QueryRow(ctx, `SELECT v.digest,v.change_note FROM chartworks.topic_draft_heads h JOIN chartworks.topic_draft_versions v ON(v.tenant_id,v.topic_id,v.revision)=(h.tenant_id,h.topic_id,h.current_revision) WHERE h.tenant_id=$1 AND h.topic_id=$2 AND h.current_revision=$3 AND h.actor_id=$4 AND h.session_id=$5 FOR SHARE OF h`, e.Tenant(), result.Topic, result.Revision, e.User(), e.Session()).Scan(&actualDigest, &note)
		if readErr != nil {
			return readErr
		}
		if actualDigest != result.Digest || note != fmt.Sprintf("Engineering proposal %s revision %d digest %s", p.ID, p.Revision, p.Digest) {
			return store.ErrConflict
		}
		changed, writeErr := proposalEffectTx(ctx, tx, e, p, engineering.ProposalEffect{Kind: "topic_draft", Target: result.Topic, State: "committed", Version: result.Revision, Digest: result.Digest})
		if writeErr != nil {
			return writeErr
		}
		state, action := "applied", "engineering.proposal_applied"
		if p.Material.Request.Schedule != nil {
			state, action = "applying", "engineering.proposal_effect"
		}
		if changed || p.State != state {
			if _, writeErr = tx.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET version=version+1,state=$3,applied_at=CASE WHEN $3='applied' THEN COALESCE(applied_at,clock_timestamp()) ELSE applied_at END,updated_at=clock_timestamp() WHERE tenant_id=$1 AND proposal_id=$2`, e.Tenant(), p.ID, state); writeErr != nil {
				return writeErr
			}
			if writeErr = proposalEventTx(ctx, tx, e, p.ID, p.Version+1, p.Revision, action, map[string]any{"topic": result.Topic, "topic_revision": result.Revision, "digest": result.Digest}); writeErr != nil {
				return writeErr
			}
		}
		out, readErr = proposalTx(ctx, tx, e, p.ID, "engineering.autopilot.apply", false)
		return readErr
	})
	if err != nil {
		return engineering.AutopilotProposal{}, err
	}
	return out, nil
}
