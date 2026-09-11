package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func proposalDependenciesExist(ctx context.Context, tx pgx.Tx, tenant, source, pipeline string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM chartworks.block_source_pins p JOIN chartworks.block_heads h ON(h.tenant_id,h.block_id,h.published_revision)=(p.tenant_id,p.block_id,p.revision) WHERE p.tenant_id=$1 AND p.source_id=$2 AND NOT h.archived)
 OR EXISTS(SELECT 1 FROM chartworks.topic_publication_heads h JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version),LATERAL jsonb_array_elements(v.definition->'datasets') ds WHERE h.tenant_id=$1 AND NOT h.archived AND ds#>>'{source,source}'=$2)
 OR EXISTS(SELECT 1 FROM chartworks.pipeline_heads h JOIN chartworks.pipeline_versions v ON(v.tenant_id,v.pipeline_id,v.version)=(h.tenant_id,h.pipeline_id,h.published_version),LATERAL jsonb_array_elements(v.manifest->'steps') s WHERE h.tenant_id=$1 AND h.pipeline_id<>$3 AND NOT h.retired AND s->>'source'=$2)`, tenant, source, pipeline).Scan(&exists)
	return exists, err
}

// CompensateAutopilot is invoked only while source.WithPipelineOutputs holds
// native ownership locks. It retires metadata visibility; it does NOT claim to
// drop warehouse tables or roll back an external transaction globally.
func (d *DB) CompensateAutopilot(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, x engineering.PipelineExecution) (out engineering.AutopilotProposal, err error) {
	v, err := proof.Checked(e, "engineering.autopilot.compensate")
	if err != nil {
		return out, err
	}
	if v.ExecutionDigest == "" || v.ExecutionDigest != readexec.Hash(x) {
		return out, engineering.ErrOwnership
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
		if p.State == "compensated" {
			out = p
			return nil
		}
		if p.State != "applied" || p.Version != v.ExpectedVersion || p.Version >= 4096 || p.Material.Request.ExpectedPipelineVersion != 0 || p.Operation != x.Operation.ID {
			return engineering.ErrCompensationBlocked
		}
		current, readErr := readPipelineExecutionTx(ctx, tx, e, p.Operation)
		if readErr != nil {
			return readErr
		}
		if readexec.Hash(current) != v.ExecutionDigest || current.State != "published" || current.Operation.State != "succeeded" {
			return engineering.ErrCompensationBlocked
		}
		if _, readErr = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061310))`, e.Tenant()+":"+p.Material.Pipeline.ID); readErr != nil {
			return readErr
		}
		if _, readErr = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061311))`, e.Tenant()+":"+p.Material.Pipeline.ID); readErr != nil {
			return readErr
		}
		var draft, published int64
		var retired bool
		if readErr = tx.QueryRow(ctx, `SELECT draft_version,COALESCE(published_version,0),retired FROM chartworks.pipeline_heads WHERE tenant_id=$1 AND pipeline_id=$2 FOR UPDATE`, e.Tenant(), p.Material.Pipeline.ID).Scan(&draft, &published, &retired); readErr != nil {
			return readErr
		}
		if retired || draft != 1 || published != 1 || current.Version != 1 || current.Digest != readexec.Hash(p.Material.Pipeline) {
			return engineering.ErrCompensationBlocked
		}
		var owned bool
		if readErr = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.engineering_proposal_pipeline_effects WHERE tenant_id=$1 AND proposal_id=$2 AND revision=$3 AND pipeline_id=$4 AND pipeline_version=1 AND pipeline_digest=$5)`, e.Tenant(), p.ID, p.Revision, p.Material.Pipeline.ID, current.Digest).Scan(&owned); readErr != nil {
			return readErr
		}
		if !owned {
			return engineering.ErrOwnership
		}
		stages := append([]engineering.PipelineStageState(nil), current.Stages...)
		sort.Slice(stages, func(i, j int) bool { return stages[i].Stage.Source < stages[j].Stage.Source })
		if len(stages) == 0 || len(stages) != len(p.Material.Pipeline.Steps) {
			return store.ErrInvalid
		}
		for _, s := range stages {
			if s.Previous != nil || s.State != "checked" || s.Stage.Revision != 1 || !s.Stage.Valid() || s.Stage.Pipeline != p.Material.Pipeline.ID || s.Stage.Operation != p.Operation {
				return engineering.ErrCompensationBlocked
			}
			if readErr = access.Require(e, "engineering.autopilot.compensate", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: s.Stage.Source}); readErr != nil {
				return readErr
			}
			var revision int64
			var deleted bool
			var currentOperation string
			if readErr = tx.QueryRow(ctx, `SELECT s.current_revision,s.deleted,o.operation_id FROM chartworks.sources s JOIN chartworks.pipeline_outputs o ON(o.tenant_id,o.source_id)=(s.tenant_id,s.source_id) WHERE s.tenant_id=$1 AND s.source_id=$2 AND o.pipeline_id=$3 AND o.step_id=$4 FOR UPDATE OF s,o`, e.Tenant(), s.Stage.Source, p.Material.Pipeline.ID, s.Stage.Step).Scan(&revision, &deleted, &currentOperation); readErr != nil {
				return readErr
			}
			if deleted || revision != s.Stage.Revision || currentOperation != p.Operation {
				return engineering.ErrCompensationBlocked
			}
			dependent, dependencyErr := proposalDependenciesExist(ctx, tx, e.Tenant(), s.Stage.Source, p.Material.Pipeline.ID)
			if dependencyErr != nil {
				return dependencyErr
			}
			if dependent {
				return engineering.ErrCompensationBlocked
			}
		}
		for _, s := range stages {
			if _, readErr = tx.Exec(ctx, `UPDATE chartworks.sources SET deleted=true WHERE tenant_id=$1 AND source_id=$2 AND current_revision=$3 AND NOT deleted`, e.Tenant(), s.Stage.Source, s.Stage.Revision); readErr != nil {
				return readErr
			}
			if _, readErr = tx.Exec(ctx, `DELETE FROM chartworks.pipeline_outputs WHERE tenant_id=$1 AND pipeline_id=$2 AND step_id=$3 AND operation_id=$4`, e.Tenant(), p.Material.Pipeline.ID, s.Stage.Step, p.Operation); readErr != nil {
				return readErr
			}
			if _, readErr = proposalEffectTx(ctx, tx, e, p, engineering.ProposalEffect{Kind: "compensation", Target: s.Stage.Source, State: "retired", Version: s.Stage.Revision, Digest: s.Stage.Digest, Operation: p.Operation, Code: "owned_generation_quarantined"}); readErr != nil {
				return readErr
			}
		}
		if _, readErr = tx.Exec(ctx, `UPDATE chartworks.pipeline_heads SET retired=true,published_version=NULL WHERE tenant_id=$1 AND pipeline_id=$2`, e.Tenant(), p.Material.Pipeline.ID); readErr != nil {
			return readErr
		}
		if _, readErr = tx.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET version=version+1,state='compensated',updated_at=clock_timestamp() WHERE tenant_id=$1 AND proposal_id=$2`, e.Tenant(), p.ID); readErr != nil {
			return readErr
		}
		if readErr = proposalEventTx(ctx, tx, e, p.ID, p.Version+1, p.Revision, "engineering.proposal_compensated", map[string]any{"operation": p.Operation, "disposition": "owned_generations_quarantined", "global_rollback": false}); readErr != nil {
			return readErr
		}
		out, readErr = proposalTx(ctx, tx, e, p.ID, "engineering.autopilot.compensate", false)
		return readErr
	})
	if err != nil {
		return engineering.AutopilotProposal{}, err
	}
	return out, nil
}

func proposalImpactsTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, p engineering.AutopilotProposal) ([]engineering.ProposalImpact, error) {
	grants, err := blockGrants(e)
	if err != nil {
		return nil, err
	}
	sources := []string{p.Material.Binding.Source}
	for _, s := range p.Material.Pipeline.Steps {
		sources = append(sources, p.Material.Pipeline.ID+"."+s.ID)
	}
	rows, err := tx.Query(ctx, `SELECT kind,id FROM (
 SELECT 'block'::text kind,h.block_id id FROM chartworks.block_heads h JOIN chartworks.block_source_pins b ON(b.tenant_id,b.block_id,b.revision)=(h.tenant_id,h.block_id,h.published_revision)
 WHERE h.tenant_id=$1 AND $4::boolean AND b.source_id=ANY($2::text[]) AND NOT h.archived
 AND EXISTS(SELECT 1 FROM chartworks.block_revision_references rr WHERE(rr.tenant_id,rr.block_id,rr.revision)=(h.tenant_id,h.block_id,h.published_revision))
 AND NOT EXISTS(SELECT 1 FROM chartworks.block_revision_references rr WHERE(rr.tenant_id,rr.block_id,rr.revision)=(h.tenant_id,h.block_id,h.published_revision)
  AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'=rr.kind AND g->>'permission'=rr.permission AND g->>'id' IN(rr.resource_id,'*')))
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='block' AND g->>'permission'='read' AND g->>'id' IN(h.block_id,'*'))
 UNION
 SELECT 'topic'::text kind,h.topic_id id FROM chartworks.topic_publication_heads h JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version),LATERAL jsonb_array_elements(v.definition->'datasets') ds
 WHERE h.tenant_id=$1 AND $5::boolean AND ds#>>'{source,source}'=ANY($2::text[]) AND NOT h.archived
 AND EXISTS(SELECT 1 FROM chartworks.topic_published_dependencies dep WHERE(dep.tenant_id,dep.topic_id,dep.version_id)=(v.tenant_id,v.topic_id,v.version_id))
 AND NOT EXISTS(SELECT 1 FROM chartworks.topic_published_dependencies dep WHERE(dep.tenant_id,dep.topic_id,dep.version_id)=(v.tenant_id,v.topic_id,v.version_id)
  AND (NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='source' AND g->>'permission'='read' AND g->>'id' IN(dep.source_id,'*'))
   OR NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='dataset' AND g->>'permission'='query' AND g->>'id' IN(dep.dataset_id,'*'))
   OR NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='execution_context' AND g->>'permission'='use' AND g->>'id' IN(dep.context_id,'*'))))
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'='topic' AND g->>'permission'='read' AND g->>'id' IN(h.topic_id,'*'))
 ) impacts ORDER BY kind,id LIMIT 101`, e.Tenant(), sources, grants, e.Has("reporting.read"), e.Has("topics.read"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []engineering.ProposalImpact{{Kind: "pipeline", ID: p.Material.Pipeline.ID, Reason: "review_required"}}
	for rows.Next() {
		var kind, id string
		if err = rows.Scan(&kind, &id); err != nil {
			return nil, err
		}
		out = append(out, engineering.ProposalImpact{Kind: kind, ID: id, Reason: "review_required"})
		if len(out) > 101 {
			return nil, engineering.ErrLimit
		}
	}
	return out, rows.Err()
}

// RecordAutopilotDrift retains one immutable reviewable amendment per unchanged
// failure evidence. Only authorized impact IDs are projected; no definition changes.
func (d *DB) RecordAutopilotDrift(ctx context.Context, e identity.Envelope, proof engineering.PreparedProposalApply, observation engineering.AutopilotDrift, interval time.Duration) (out engineering.AutopilotDrift, err error) {
	v, err := proof.Checked(e, "engineering.autopilot.drift")
	if err != nil {
		return out, err
	}
	if v.Drift == nil || readexec.Hash(*v.Drift) != readexec.Hash(observation) || interval < time.Minute || interval > 7*24*time.Hour {
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
		if p.State != "applied" && p.State != "applying" {
			return engineering.ErrState
		}
		if observation.Proposal != p.ID || observation.Revision != p.Revision || observation.PriorBindingDigest != readexec.Hash(p.Material.Binding) || observation.State != "proposed" || observation.ID != "" {
			return store.ErrInvalid
		}
		want := readexec.Hash([]any{engineering.AutopilotVersion, p.ID, p.Revision, p.Digest, observation.Kind, observation.PriorBindingDigest, observation.CurrentBindingDigest, p.Operation, p.Applied})
		if want != observation.EvidenceDigest {
			return store.ErrInvalid
		}
		var raw []byte
		readErr = tx.QueryRow(ctx, `SELECT observation FROM chartworks.engineering_amendments WHERE tenant_id=$1 AND proposal_id=$2 AND revision=$3 AND evidence_digest=$4`, e.Tenant(), p.ID, p.Revision, want).Scan(&raw)
		if readErr == nil {
			if json.Unmarshal(raw, &out) != nil {
				return store.ErrInvalid
			}
			// Recompute current projection rather than leaking old reader reach.
			out.Impacts, readErr = proposalImpactsTx(ctx, tx, e, p)
			return readErr
		}
		if !errors.Is(readErr, pgx.ErrNoRows) {
			return readErr
		}
		var recentlyChanged bool
		if readErr = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.engineering_amendments WHERE tenant_id=$1 AND proposal_id=$2 AND revision=$3 AND created_at>clock_timestamp()-$4::interval)`, e.Tenant(), p.ID, p.Revision, interval.String()).Scan(&recentlyChanged); readErr != nil {
			return readErr
		}
		if recentlyChanged {
			return engineering.ErrLimit
		}
		out = observation
		out.ID, readErr = newID()
		if readErr != nil {
			return readErr
		}
		out.Impacts = []engineering.ProposalImpact{}
		raw, readErr = json.Marshal(out)
		if readErr != nil {
			return readErr
		}
		if _, readErr = tx.Exec(ctx, `INSERT INTO chartworks.engineering_amendments(tenant_id,amendment_id,proposal_id,revision,kind,evidence_digest,observation) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), out.ID, p.ID, p.Revision, out.Kind, want, raw); readErr != nil {
			return readErr
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if readErr = auditJob(ctx, tx, scope, "engineering.amendment_created", out.ID); readErr != nil {
			return readErr
		}
		out.Impacts, readErr = proposalImpactsTx(ctx, tx, e, p)
		return readErr
	})
	if err != nil {
		return engineering.AutopilotDrift{}, err
	}
	return out, nil
}
