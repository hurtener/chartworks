package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// savePipelineTx is the single draft effect used by the ordinary service and
// reviewed proposal attribution. Both the target and attribution may share one
// local transaction; neither path can infer execution/publication authority.
func savePipelineTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, def engineering.PipelineDefinition, expected int64) (engineering.PipelineRecord, error) {
	if expected < 0 || expected >= 1<<62 || !pipelineDefinitionValid(def) {
		return engineering.PipelineRecord{}, engineering.ErrInvalid
	}
	if err := access.Require(e, "engineering.pipeline.write", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: def.ID}); err != nil {
		return engineering.PipelineRecord{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061310))`, e.Tenant()+":"+def.ID); err != nil {
		return engineering.PipelineRecord{}, err
	}
	var current int64
	var retired bool
	err := tx.QueryRow(ctx, `SELECT draft_version,retired FROM chartworks.pipeline_heads WHERE tenant_id=$1 AND pipeline_id=$2 FOR UPDATE`, e.Tenant(), def.ID).Scan(&current, &retired)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if expected != 0 {
			return engineering.PipelineRecord{}, store.ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.pipeline_heads(tenant_id,pipeline_id,actor_id,session_id,draft_version) VALUES($1,$2,$3,$4,1)`, e.Tenant(), def.ID, e.User(), e.Session()); err != nil {
			return engineering.PipelineRecord{}, err
		}
	case err != nil:
		return engineering.PipelineRecord{}, err
	default:
		if retired || current != expected {
			return engineering.PipelineRecord{}, store.ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_heads SET draft_version=$3 WHERE tenant_id=$1 AND pipeline_id=$2`, e.Tenant(), def.ID, expected+1); err != nil {
			return engineering.PipelineRecord{}, err
		}
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return engineering.PipelineRecord{}, store.ErrInvalid
	}
	if _, err = tx.Exec(ctx, `INSERT INTO chartworks.pipeline_versions(tenant_id,pipeline_id,version,manifest,manifest_hash) VALUES($1,$2,$3,$4,$5)`, e.Tenant(), def.ID, expected+1, raw, readexec.Hash(def)); err != nil {
		return engineering.PipelineRecord{}, err
	}
	scope, _ := store.NewScope(e.Tenant(), e.User())
	if err = auditJob(ctx, tx, scope, "pipeline.drafted", def.ID); err != nil {
		return engineering.PipelineRecord{}, err
	}
	return pipelineVersionTx(ctx, tx, e, def.ID, expected+1, "engineering.pipeline.write", "write")
}

// pipelinePublicationInputs fences current source coordinates through metadata
// publication. The same source head locks serialize publication against retiring
// an owned output; an invisible/deleted generation cannot acquire a new dependent.
func pipelinePublicationInputs(ctx context.Context, tx pgx.Tx, e identity.Envelope, def engineering.PipelineDefinition) error {
	contexts := map[string]string{}
	for _, step := range def.Steps {
		if len(step.FromSteps) > 0 {
			continue
		}
		if prior, ok := contexts[step.Source]; ok && prior != step.Context {
			return readexec.ErrBinding
		}
		contexts[step.Source] = step.Context
	}
	ids := make([]string, 0, len(contexts))
	for id := range contexts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var actual string
		if err := tx.QueryRow(ctx, `SELECT r.context_id FROM chartworks.sources s JOIN chartworks.source_revisions r ON(r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision) WHERE s.tenant_id=$1 AND s.source_id=$2 AND NOT s.deleted FOR SHARE OF s`, e.Tenant(), id).Scan(&actual); err != nil {
			return err
		}
		if actual != contexts[id] {
			return readexec.ErrBinding
		}
	}
	return nil
}

// publishPipelineTx is ordinary exact-draft publication, reused for attributed
// L2 effects. No proposal review grants its signed publish/source permissions.
func publishPipelineTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, version int64) (engineering.PipelineRecord, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061310))`, e.Tenant()+":"+id); err != nil {
		return engineering.PipelineRecord{}, err
	}
	out, err := pipelineVersionTx(ctx, tx, e, id, version, "engineering.pipeline.publish", "write")
	if err != nil {
		return engineering.PipelineRecord{}, err
	}
	if out.Published != nil {
		return out, nil
	}
	var current int64
	var retired bool
	if err = tx.QueryRow(ctx, `SELECT draft_version,retired FROM chartworks.pipeline_heads WHERE tenant_id=$1 AND pipeline_id=$2 FOR UPDATE`, e.Tenant(), id).Scan(&current, &retired); err != nil {
		return engineering.PipelineRecord{}, err
	}
	if retired || current != version {
		return engineering.PipelineRecord{}, store.ErrConflict
	}
	if err = pipelinePublicationInputs(ctx, tx, e, out.Definition); err != nil {
		return engineering.PipelineRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_versions SET published_at=clock_timestamp() WHERE tenant_id=$1 AND pipeline_id=$2 AND version=$3`, e.Tenant(), id, version); err != nil {
		return engineering.PipelineRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_heads SET published_version=$3 WHERE tenant_id=$1 AND pipeline_id=$2`, e.Tenant(), id, version); err != nil {
		return engineering.PipelineRecord{}, err
	}
	scope, _ := store.NewScope(e.Tenant(), e.User())
	if err = auditJob(ctx, tx, scope, "pipeline.published", id); err != nil {
		return engineering.PipelineRecord{}, err
	}
	return pipelineVersionTx(ctx, tx, e, id, version, "engineering.pipeline.publish", "write")
}
