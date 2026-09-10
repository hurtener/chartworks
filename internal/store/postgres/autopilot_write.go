package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func proposalEventTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, version, revision int64, action string, evidence any) error {
	raw, err := json.Marshal(evidence)
	if err != nil || len(raw) > 131072 {
		return store.ErrInvalid
	}
	if _, err = tx.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_events(tenant_id,proposal_id,version,revision,action,actor_id,session_id,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, e.Tenant(), id, version, revision, action, e.User(), e.Session(), raw); err != nil {
		return err
	}
	scope, _ := store.NewScope(e.Tenant(), e.User())
	return auditJob(ctx, tx, scope, action, id)
}

func insertProposalMaterial(ctx context.Context, tx pgx.Tx, e identity.Envelope, m engineering.ProposalMaterial, revision int64) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return store.ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision,digest,request_hash,source_id,source_revision,context_id,author_id,session_id,material,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, e.Tenant(), m.Request.ID, revision, m.Digest(), readexec.Hash(m.Request), m.Binding.Source, m.Binding.Revision, m.Binding.Context, e.User(), e.Session(), raw, m.Created)
	if err != nil {
		return err
	}
	for _, r := range m.References {
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_references(tenant_id,proposal_id,revision,kind,permission,resource_id) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), m.Request.ID, revision, r.Kind, r.Permission, r.ID); err != nil {
			return err
		}
	}
	return nil
}

// SaveAutopilotProposal appends material; it does not create or publish a pipeline.
func (d *DB) SaveAutopilotProposal(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposal, expected int64, maximum int) (out engineering.AutopilotProposal, err error) {
	m, err := proof.Checked(e)
	if err != nil {
		return out, err
	}
	if expected < 0 || expected > 4095 || maximum < 1 || maximum > 100000 {
		return out, store.ErrInvalid
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, writeErr := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214062601))`, e.Tenant()); writeErr != nil {
			return writeErr
		}
		var exists bool
		if queryErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.engineering_proposal_heads WHERE tenant_id=$1 AND proposal_id=$2)`, e.Tenant(), m.Request.ID).Scan(&exists); queryErr != nil {
			return queryErr
		}
		var old engineering.AutopilotProposal
		revision, version := int64(1), int64(1)
		action := "engineering.proposal_created"
		if exists {
			var readErr error
			old, readErr = proposalTx(ctx, tx, e, m.Request.ID, "engineering.autopilot.propose", true)
			if readErr != nil {
				return readErr
			}
			if expected == 0 {
				if readexec.Hash(old.Material.Request) != readexec.Hash(m.Request) || readexec.Hash(old.Material.Origin) != readexec.Hash(m.Origin) {
					return store.ErrConflict
				}
				out = old
				return nil
			}
			if old.Version != expected || old.State == "applying" || old.State == "applied" || old.State == "compensated" || old.Revision >= 256 {
				return store.ErrConflict
			}
			if readexec.Hash(old.Material.Request) != readexec.Hash(m.Request) || readexec.Hash(old.Material.Origin) != readexec.Hash(m.Origin) || readexec.Hash(old.Material.Binding) != readexec.Hash(m.Binding) {
				return engineering.ErrProposalDrift
			}
			revision, version, action = old.Revision+1, old.Version+1, "engineering.proposal_edited"
		} else {
			if expected != 0 {
				return store.ErrConflict
			}
			var total int
			if queryErr := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.engineering_proposal_heads WHERE tenant_id=$1`, e.Tenant()).Scan(&total); queryErr != nil {
				return queryErr
			}
			if total >= maximum {
				return engineering.ErrLimit
			}
		}
		if !exists && m.Origin != nil {
			if originErr := proposalOriginTx(ctx, tx, e, m); originErr != nil {
				return originErr
			}
		}
		if writeErr := proposalSourceFence(ctx, tx, e, m); writeErr != nil {
			return writeErr
		}
		if !exists {
			if _, writeErr := tx.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_heads(tenant_id,proposal_id,pipeline_id,origin_author,version,revision,state) VALUES($1,$2,$3,$4,1,1,'draft')`, e.Tenant(), m.Request.ID, m.Pipeline.ID, e.User()); writeErr != nil {
				return writeErr
			}
		}
		if writeErr := insertProposalMaterial(ctx, tx, e, m, revision); writeErr != nil {
			return writeErr
		}
		if !exists && m.Origin != nil {
			if _, writeErr := tx.Exec(ctx, `INSERT INTO chartworks.engineering_amendment_proposals(tenant_id,amendment_id,proposal_id,parent_id,parent_revision,parent_digest) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), m.Origin.Drift, m.Request.ID, m.Origin.Proposal, m.Origin.Revision, m.Origin.Digest); writeErr != nil {
				return writeErr
			}
		}
		if exists {
			if _, writeErr := tx.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET version=$3,revision=$4,state='draft',review_version=NULL,updated_at=clock_timestamp() WHERE tenant_id=$1 AND proposal_id=$2 AND version=$5`, e.Tenant(), m.Request.ID, version, revision, expected); writeErr != nil {
				return writeErr
			}
		}
		if writeErr := proposalEventTx(ctx, tx, e, m.Request.ID, version, revision, action, map[string]any{"digest": m.Digest(), "model_version": m.ModelVersion, "prompt_version": m.PromptVersion}); writeErr != nil {
			return writeErr
		}
		var readErr error
		out, readErr = proposalTx(ctx, tx, e, m.Request.ID, "engineering.autopilot.propose", false)
		return readErr
	})
	if err != nil {
		return engineering.AutopilotProposal{}, err
	}
	return out, nil
}

// ReviewAutopilotProposal serializes competing reviewers and binds their signed
// identity to the exact revision. The submitter cannot self-approve via a field.
func (d *DB) ReviewAutopilotProposal(ctx context.Context, e identity.Envelope, id string, r engineering.AutopilotReviewRequest) (out engineering.AutopilotProposal, err error) {
	if r.ExpectedVersion < 1 || r.Revision < 1 || len(r.Reason) < 1 || len(r.Reason) > 2048 || (r.Decision != "approve" && r.Decision != "reject") {
		return out, store.ErrInvalid
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, readErr := proposalTx(ctx, tx, e, id, "engineering.autopilot.review", true)
		if readErr != nil {
			return readErr
		}
		if p.Version != r.ExpectedVersion || p.Revision != r.Revision || p.Digest != r.Digest || p.Version >= 4096 || (p.State != "draft" && p.State != "approved") {
			return store.ErrConflict
		}
		if r.Decision == "approve" && (e.User() == p.OriginAuthor || e.User() == p.Material.Author) {
			return engineering.ErrProposalReview
		}
		if r.Decision == "approve" {
			if readErr = proposalSourceFence(ctx, tx, e, p.Material); readErr != nil {
				return readErr
			}
		}
		version := p.Version + 1
		if _, readErr = tx.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_reviews(tenant_id,proposal_id,version,revision,digest,actor_id,session_id,decision,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, e.Tenant(), id, version, p.Revision, p.Digest, e.User(), e.Session(), r.Decision, r.Reason); readErr != nil {
			return readErr
		}
		state := "rejected"
		if r.Decision == "approve" {
			state = "approved"
		}
		if _, readErr = tx.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET version=$3,review_version=$3,state=$4,updated_at=clock_timestamp() WHERE tenant_id=$1 AND proposal_id=$2 AND version=$5`, e.Tenant(), id, version, state, p.Version); readErr != nil {
			return readErr
		}
		if readErr = proposalEventTx(ctx, tx, e, id, version, p.Revision, "engineering.proposal_reviewed", map[string]any{"decision": r.Decision, "digest": p.Digest}); readErr != nil {
			return readErr
		}
		out, readErr = proposalTx(ctx, tx, e, id, "engineering.autopilot.review", false)
		return readErr
	})
	if err != nil {
		return engineering.AutopilotProposal{}, err
	}
	return out, nil
}

func proposalEffectTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, p engineering.AutopilotProposal, effect engineering.ProposalEffect) (bool, error) {
	var previous []byte
	err := tx.QueryRow(ctx, `SELECT evidence FROM chartworks.engineering_proposal_effects WHERE tenant_id=$1 AND proposal_id=$2 AND revision=$3 AND kind=$4 AND target_id=$5 FOR UPDATE`, e.Tenant(), p.ID, p.Revision, effect.Kind, effect.Target).Scan(&previous)
	if err == nil {
		var prior engineering.ProposalEffect
		if json.Unmarshal(previous, &prior) != nil {
			return false, store.ErrInvalid
		}
		prior.Observed = time.Time{}
		effect.Observed = time.Time{}
		if readexec.Hash(prior) == readexec.Hash(effect) {
			return false, nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	effect.Observed = time.Now().UTC().Truncate(time.Microsecond)
	raw, err := json.Marshal(effect)
	if err != nil {
		return false, store.ErrInvalid
	}
	order, ok := map[string]int{"pipeline_draft": 0, "pipeline_publication": 1, "pipeline_run": 2, "managed_step": 3, "compensation": 4}[effect.Kind]
	if !ok {
		return false, store.ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_effects(tenant_id,proposal_id,revision,kind,target_id,evidence,observed_order) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,proposal_id,revision,kind,target_id) DO UPDATE SET evidence=EXCLUDED.evidence`, e.Tenant(), p.ID, p.Revision, effect.Kind, effect.Target, raw, order)
	return err == nil, err
}

// StageAutopilotPipeline uses the SAME transaction helpers as ordinary pipeline
// authoring. The created revision and its proposal attribution commit together.
func (d *DB) StageAutopilotPipeline(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, stage string) (out engineering.AutopilotProposal, err error) {
	v, err := proof.Checked(e, "engineering.autopilot.apply")
	if err != nil {
		return out, err
	}
	if stage != "draft" && stage != "publish" {
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
		if p.State != "approved" && p.State != "applying" {
			return engineering.ErrProposalReview
		}
		if p.Version >= 4096 {
			return store.ErrConflict
		}
		if readErr = proposalSourceFence(ctx, tx, e, p.Material); readErr != nil {
			return readErr
		}
		def := p.Material.Pipeline
		targetVersion := p.Material.Request.ExpectedPipelineVersion + 1
		var ownedVersion int64
		var ownedDigest string
		readErr = tx.QueryRow(ctx, `SELECT pipeline_version,pipeline_digest FROM chartworks.engineering_proposal_pipeline_effects WHERE tenant_id=$1 AND proposal_id=$2 AND revision=$3`, e.Tenant(), p.ID, p.Revision).Scan(&ownedVersion, &ownedDigest)
		owned := readErr == nil
		if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
			return readErr
		}
		if owned && (ownedVersion != targetVersion || ownedDigest != readexec.Hash(def)) {
			return store.ErrConflict
		}
		kind := "pipeline_draft"
		if stage == "draft" {
			if !owned {
				if _, readErr = savePipelineTx(ctx, tx, e, def, p.Material.Request.ExpectedPipelineVersion); readErr != nil {
					return readErr
				}
				if _, readErr = tx.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_pipeline_effects(tenant_id,proposal_id,revision,pipeline_id,pipeline_version,pipeline_digest) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), p.ID, p.Revision, def.ID, targetVersion, readexec.Hash(def)); readErr != nil {
					return readErr
				}
			} else {
				if _, readErr = pipelineVersionTx(ctx, tx, e, def.ID, targetVersion, "engineering.pipeline.write", "write"); readErr != nil {
					return readErr
				}
			}
		} else {
			if !owned {
				return store.ErrConflict
			}
			if _, readErr = publishPipelineTx(ctx, tx, e, def.ID, targetVersion); readErr != nil {
				return readErr
			}
			kind = "pipeline_publication"
		}
		changed, writeErr := proposalEffectTx(ctx, tx, e, p, engineering.ProposalEffect{Kind: kind, Target: def.ID, State: "committed", Version: targetVersion, Digest: readexec.Hash(def)})
		if writeErr != nil {
			return writeErr
		}
		if changed {
			if p.State == "approved" && p.Version != v.ExpectedVersion {
				return store.ErrConflict
			}
			if _, writeErr = tx.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET version=version+1,state='applying',apply_actor=$3,apply_session=$4,updated_at=clock_timestamp() WHERE tenant_id=$1 AND proposal_id=$2`, e.Tenant(), p.ID, e.User(), e.Session()); writeErr != nil {
				return writeErr
			}
			if writeErr = proposalEventTx(ctx, tx, e, p.ID, p.Version+1, p.Revision, "engineering.proposal_effect", map[string]any{"kind": kind, "target": def.ID, "pipeline_version": targetVersion, "digest": readexec.Hash(def)}); writeErr != nil {
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

// RecordAutopilotRun observes the real private operation/stages, not a client
// claim of success. Admission is recorded before the service starts native work.
func (d *DB) RecordAutopilotRun(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, run engineering.PipelineRun) (out engineering.AutopilotProposal, err error) {
	v, err := proof.Checked(e, "engineering.autopilot.apply")
	if err != nil {
		return out, err
	}
	if !identity.Identifier(run.Operation.ID) {
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
		if p.State != "applying" && p.State != "applied" || p.ApplyActor != e.User() || p.ApplySession != e.Session() || p.Version >= 4096 {
			return store.ErrConflict
		}
		if p.Operation != "" && p.Operation != run.Operation.ID {
			return store.ErrConflict
		}
		x, readErr := readPipelineExecutionTx(ctx, tx, e, run.Operation.ID)
		if readErr != nil {
			return readErr
		}
		if x.Pipeline != p.Material.Pipeline.ID || x.Version != p.Material.Request.ExpectedPipelineVersion+1 || x.Digest != readexec.Hash(p.Material.Pipeline) {
			return store.ErrInvalid
		}
		changed, writeErr := proposalEffectTx(ctx, tx, e, p, engineering.ProposalEffect{Kind: "pipeline_run", Target: x.Pipeline, State: x.State, Version: x.Version, Digest: x.Digest, Operation: x.Operation.ID, Code: x.Operation.Code})
		if writeErr != nil {
			return writeErr
		}
		for _, stage := range x.Stages {
			if !stage.Valid() {
				return store.ErrInvalid
			}
			yes, effectErr := proposalEffectTx(ctx, tx, e, p, engineering.ProposalEffect{Kind: "managed_step", Target: stage.Stage.Source, State: stage.State, Version: stage.Stage.Revision, Digest: stage.Stage.Digest, Operation: x.Operation.ID, Code: stage.Code})
			if effectErr != nil {
				return effectErr
			}
			changed = changed || yes
		}
		state, action := "applying", "engineering.proposal_effect"
		if x.State == "published" && x.Operation.State == "succeeded" {
			if len(x.Stages) != len(p.Material.Pipeline.Steps) {
				return store.ErrInvalid
			}
			for _, stage := range x.Stages {
				if stage.State != "checked" || !stage.Stage.Valid() {
					return store.ErrInvalid
				}
			}
			state, action = "applied", "engineering.proposal_applied"
		}
		if p.State == "applied" && state != "applied" {
			return store.ErrConflict
		}
		if changed || p.Operation == "" || p.State != state {
			if _, writeErr = tx.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET version=version+1,state=$3,operation_id=$4,applied_at=CASE WHEN $3='applied' THEN COALESCE(applied_at,clock_timestamp()) ELSE applied_at END,updated_at=clock_timestamp() WHERE tenant_id=$1 AND proposal_id=$2`, e.Tenant(), p.ID, state, x.Operation.ID); writeErr != nil {
				return writeErr
			}
			if writeErr = proposalEventTx(ctx, tx, e, p.ID, p.Version+1, p.Revision, action, map[string]any{"operation": x.Operation.ID, "state": state, "physical_queue_attempts": x.Operation.Attempts}); writeErr != nil {
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

func proposalOriginTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, m engineering.ProposalMaterial) error {
	o := m.Origin
	p, err := proposalTx(ctx, tx, e, o.Proposal, "engineering.autopilot.propose", true)
	if err != nil {
		return err
	}
	if p.Revision != o.Revision || p.Digest != o.Digest || p.Material.Pipeline.ID != m.Pipeline.ID || p.Material.Binding.Source != m.Binding.Source || m.Request.ExpectedPipelineVersion != p.Material.Request.ExpectedPipelineVersion+1 {
		return engineering.ErrProposalDrift
	}
	if p.State != "applied" {
		if p.State != "applying" || p.Operation == "" {
			return engineering.ErrState
		}
		x, readErr := readPipelineExecutionTx(ctx, tx, e, p.Operation)
		if readErr != nil {
			return readErr
		}
		if x.State != "quality_failed" {
			return engineering.ErrState
		}
	}
	var evidence string
	if err = tx.QueryRow(ctx, `SELECT evidence_digest FROM chartworks.engineering_amendments WHERE tenant_id=$1 AND amendment_id=$2 AND proposal_id=$3 AND revision=$4 FOR SHARE`, e.Tenant(), o.Drift, p.ID, p.Revision).Scan(&evidence); err != nil {
		return err
	}
	if evidence != o.EvidenceDigest {
		return engineering.ErrProposalDrift
	}
	var version int64
	if err = tx.QueryRow(ctx, `SELECT draft_version FROM chartworks.pipeline_heads WHERE tenant_id=$1 AND pipeline_id=$2 AND NOT retired FOR SHARE`, e.Tenant(), m.Pipeline.ID).Scan(&version); err != nil {
		return err
	}
	if version != m.Request.ExpectedPipelineVersion {
		return engineering.ErrProposalConflict
	}
	return nil
}
