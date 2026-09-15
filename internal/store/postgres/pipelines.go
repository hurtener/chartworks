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

func pipelineDefinitionValid(d engineering.PipelineDefinition) bool {
	l := config.DefaultPipelines()
	l.MaxSteps = 32
	l.MaxSQLBytes = 1 << 20
	return engineering.ValidatePipelineDefinition(d, l) == nil
}
func pipelineVersionTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, version int64, action, permission string) (out engineering.PipelineRecord, err error) {
	if !identity.Identifier(id) || version < 1 {
		return out, store.ErrInvalid
	}
	if err = access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: id}); err != nil {
		return out, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT h.tenant_id,h.actor_id,h.session_id,v.version,v.manifest,v.manifest_hash,v.created_at,v.published_at FROM chartworks.pipeline_heads h JOIN chartworks.pipeline_versions v USING(tenant_id,pipeline_id) WHERE h.tenant_id=$1 AND h.pipeline_id=$2 AND v.version=$3 AND NOT h.retired`, e.Tenant(), id, version).Scan(&out.Tenant, &out.Actor, &out.Session, &out.Version, &raw, &out.Digest, &out.Created, &out.Published)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(raw, &out.Definition) != nil || !pipelineDefinitionValid(out.Definition) || out.Definition.ID != id || readexec.Hash(out.Definition) != out.Digest {
		return engineering.PipelineRecord{}, store.ErrInvalid
	}
	out.State = "draft"
	if out.Published != nil {
		out.State = "published"
	}
	return out, out.Require(e, action, permission)
}

// SavePipeline appends an immutable draft under the expected head revision.
func (d *DB) SavePipeline(ctx context.Context, e identity.Envelope, def engineering.PipelineDefinition, expected int64) (out engineering.PipelineRecord, err error) {
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var inner error
		out, inner = savePipelineTx(ctx, tx, e, def, expected)
		return inner
	})
	if err != nil {
		return engineering.PipelineRecord{}, err
	}
	return out, nil
}

// ReadPipeline returns an immutable version after signed action and resource checks.
func (d *DB) ReadPipeline(ctx context.Context, e identity.Envelope, id string, version int64, action, permission string) (out engineering.PipelineRecord, err error) {
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var inner error
		out, inner = pipelineVersionTx(ctx, tx, e, id, version, action, permission)
		return inner
	})
	if err != nil {
		return engineering.PipelineRecord{}, err
	}
	return out, nil
}

// PublishPipeline atomically publishes the exact current draft version.
func (d *DB) PublishPipeline(ctx context.Context, e identity.Envelope, id string, version int64) (out engineering.PipelineRecord, err error) {
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var inner error
		out, inner = publishPipelineTx(ctx, tx, e, id, version)
		return inner
	})
	if err != nil {
		return engineering.PipelineRecord{}, err
	}
	return out, nil
}
