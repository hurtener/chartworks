package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ engineering.AutopilotRepository = (*DB)(nil)

func proposalReadArgs(e identity.Envelope, id, action string) ([]any, error) {
	if !e.Valid() {
		return nil, access.ErrUnauthenticated
	}
	if !identity.Identifier(id) {
		return nil, store.ErrInvalid
	}
	switch action {
	case "engineering.autopilot.read", "engineering.autopilot.propose", "engineering.autopilot.review", "engineering.autopilot.apply", "engineering.autopilot.compensate", "engineering.autopilot.drift":
	default:
		return nil, access.ErrForbidden
	}
	if !e.Has(action) {
		return nil, access.ErrForbidden
	}
	grants, err := blockGrants(e)
	if err != nil {
		return nil, err
	}
	permission := "write"
	if action == "engineering.autopilot.read" {
		permission = "read"
	}
	return []any{e.Tenant(), id, grants, permission}, nil
}

const proposalEligibility = `h.tenant_id=$1 AND h.proposal_id=$2
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='source' AND g->>'permission'=$4 AND g->>'id' IN(h.pipeline_id,'*'))
 AND EXISTS(SELECT 1 FROM chartworks.engineering_proposal_references rr WHERE(rr.tenant_id,rr.proposal_id,rr.revision)=(v.tenant_id,v.proposal_id,v.revision))
 AND NOT EXISTS(SELECT 1 FROM chartworks.engineering_proposal_references rr WHERE(rr.tenant_id,rr.proposal_id,rr.revision)=(v.tenant_id,v.proposal_id,v.revision)
  AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'=rr.kind AND g->>'permission'=rr.permission AND g->>'id' IN(rr.resource_id,'*')))`

func proposalHardLimits() config.Autopilot {
	l := config.DefaultAutopilot()
	l.MaxSteps = 16
	l.MaxCalls = 8
	l.MaxTokens = 128 << 10
	l.MaxProposalBytes = 1 << 20
	l.MaxEvidenceBytes = 128 << 10
	return l
}

func proposalTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id, action string, lock bool) (out engineering.AutopilotProposal, err error) {
	args, err := proposalReadArgs(e, id, action)
	if err != nil {
		return out, err
	}
	query := `SELECT h.proposal_id,h.tenant_id,h.version,h.revision,v.digest,h.state,h.origin_author,v.material,h.created_at,h.updated_at,h.applied_at,COALESCE(h.operation_id,''),COALESCE(h.apply_actor,''),COALESCE(h.apply_session,''),
 CASE WHEN r.version IS NULL THEN NULL ELSE jsonb_build_object('revision',r.revision,'digest',r.digest,'decision',r.decision,'reason',r.reason,'actor',r.actor_id,'session',r.session_id,'created_at',r.created_at) END
 FROM chartworks.engineering_proposal_heads h
 JOIN chartworks.engineering_proposal_versions v ON(v.tenant_id,v.proposal_id,v.revision)=(h.tenant_id,h.proposal_id,h.revision)
 LEFT JOIN chartworks.engineering_proposal_reviews r ON(r.tenant_id,r.proposal_id,r.version)=(h.tenant_id,h.proposal_id,h.review_version)
 WHERE ` + proposalEligibility
	if lock {
		query += ` FOR UPDATE OF h`
	}
	var material, review []byte
	err = tx.QueryRow(ctx, query, args...).Scan(&out.ID, &out.Tenant, &out.Version, &out.Revision, &out.Digest, &out.State, &out.OriginAuthor, &material, &out.Created, &out.Updated, &out.Applied, &out.Operation, &out.ApplyActor, &out.ApplySession, &review)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(material, &out.Material) != nil || out.Material.Digest() != out.Digest || out.Material.Request.ID != out.ID || out.Material.Binding.Tenant != out.Tenant || engineering.ValidateAutopilotMaterial(out.Material, proposalHardLimits()) != nil {
		return engineering.AutopilotProposal{}, store.ErrInvalid
	}
	if review != nil {
		if json.Unmarshal(review, &out.Review) != nil || out.Review == nil || out.Review.Revision != out.Revision || out.Review.Digest != out.Digest {
			return engineering.AutopilotProposal{}, store.ErrInvalid
		}
		if out.Review.Decision == "approve" && (out.Review.Actor == out.OriginAuthor || out.Review.Actor == out.Material.Author) {
			return engineering.AutopilotProposal{}, store.ErrInvalid
		}
	}
	if err = engineering.RequireAutopilotMaterial(e, out.Material, action); err != nil {
		return engineering.AutopilotProposal{}, err
	}
	out.Effects = []engineering.ProposalEffect{}
	rows, err := tx.Query(ctx, `SELECT evidence FROM chartworks.engineering_proposal_effects WHERE tenant_id=$1 AND proposal_id=$2 AND revision=$3 ORDER BY observed_order,kind,target_id`, e.Tenant(), id, out.Revision)
	if err != nil {
		return engineering.AutopilotProposal{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var effect engineering.ProposalEffect
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &effect) != nil || !identity.Identifier(effect.Target) || effect.Observed.IsZero() {
			return engineering.AutopilotProposal{}, store.ErrInvalid
		}
		out.Effects = append(out.Effects, effect)
		if len(out.Effects) > 40 {
			return engineering.AutopilotProposal{}, store.ErrInvalid
		}
	}
	if err = rows.Err(); err != nil {
		return engineering.AutopilotProposal{}, err
	}
	if !e.Valid() {
		return engineering.AutopilotProposal{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

// ReadAutopilotProposal never loads proposal/model/SQL material until the entire
// signed metadata selection has succeeded inside PostgreSQL.
func (d *DB) ReadAutopilotProposal(ctx context.Context, e identity.Envelope, id, action string) (out engineering.AutopilotProposal, err error) {
	if _, err = proposalReadArgs(e, id, action); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var readErr error
		out, readErr = proposalTx(ctx, tx, e, id, action, false)
		return readErr
	})
	if err != nil {
		return engineering.AutopilotProposal{}, err
	}
	return out, nil
}

func proposalSourceFence(ctx context.Context, tx pgx.Tx, e identity.Envelope, m engineering.ProposalMaterial) error {
	var revision int64
	var contextID string
	err := tx.QueryRow(ctx, `SELECT s.current_revision,r.context_id FROM chartworks.sources s JOIN chartworks.source_revisions r ON(r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision) WHERE s.tenant_id=$1 AND s.source_id=$2 AND NOT s.deleted FOR SHARE OF s`, e.Tenant(), m.Binding.Source).Scan(&revision, &contextID)
	if err != nil {
		return err
	}
	if revision != m.Binding.Revision || contextID != m.Binding.Context {
		return engineering.ErrProposalDrift
	}
	return nil
}

func proposalApplyTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, v engineering.ProposalApply) (engineering.AutopilotProposal, error) {
	p, err := proposalTx(ctx, tx, e, v.ID, v.Action, true)
	if err != nil {
		return p, err
	}
	if p.Revision != v.Revision || p.Digest != v.Digest || readexec.Hash(p.Material) != readexec.Hash(v.Material) {
		return engineering.AutopilotProposal{}, store.ErrConflict
	}
	if v.Action != "engineering.autopilot.drift" {
		if p.Review == nil || p.Review.Decision != "approve" || p.Review.Actor == p.OriginAuthor || p.Review.Actor == p.Material.Author {
			return engineering.AutopilotProposal{}, engineering.ErrProposalReview
		}
		if p.ApplyActor != "" && (p.ApplyActor != e.User() || p.ApplySession != e.Session()) {
			return engineering.AutopilotProposal{}, access.ErrForbidden
		}
	}
	return p, nil
}
