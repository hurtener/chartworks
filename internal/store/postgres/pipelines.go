package postgres

import (
	"context"
	"encoding/json"
	"errors"

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
	err = tx.QueryRow(ctx, `SELECT h.tenant_id,h.actor_id,h.session_id,v.version,v.manifest,v.manifest_hash,v.created_at,v.published_at FROM chartworks.pipeline_heads h JOIN chartworks.pipeline_versions v USING(tenant_id,pipeline_id) WHERE h.tenant_id=$1 AND h.pipeline_id=$2 AND v.version=$3`, e.Tenant(), id, version).Scan(&out.Tenant, &out.Actor, &out.Session, &out.Version, &raw, &out.Digest, &out.Created, &out.Published)
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
	if expected < 0 || expected >= 1<<62 || !pipelineDefinitionValid(def) {
		return out, engineering.ErrInvalid
	}
	if err = access.Require(e, "engineering.pipeline.write", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: def.ID}); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061310))`, e.Tenant()+":"+def.ID); err != nil {
			return err
		}
		var current int64
		var actor, session string
		err := tx.QueryRow(ctx, `SELECT draft_version,actor_id,session_id FROM chartworks.pipeline_heads WHERE tenant_id=$1 AND pipeline_id=$2 FOR UPDATE`, e.Tenant(), def.ID).Scan(&current, &actor, &session)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if expected != 0 {
				return store.ErrConflict
			}
			if _, err = tx.Exec(ctx, `INSERT INTO chartworks.pipeline_heads(tenant_id,pipeline_id,actor_id,session_id,draft_version) VALUES($1,$2,$3,$4,1)`, e.Tenant(), def.ID, e.User(), e.Session()); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if current != expected {
				return store.ErrConflict
			}
			if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_heads SET draft_version=$3 WHERE tenant_id=$1 AND pipeline_id=$2`, e.Tenant(), def.ID, expected+1); err != nil {
				return err
			}
		}
		raw, err := json.Marshal(def)
		if err != nil {
			return store.ErrInvalid
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.pipeline_versions(tenant_id,pipeline_id,version,manifest,manifest_hash) VALUES($1,$2,$3,$4,$5)`, e.Tenant(), def.ID, expected+1, raw, readexec.Hash(def)); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err = auditJob(ctx, tx, scope, "pipeline.drafted", def.ID); err != nil {
			return err
		}
		out, err = pipelineVersionTx(ctx, tx, e, def.ID, expected+1, "engineering.pipeline.write", "write")
		return err
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
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061310))`, e.Tenant()+":"+id); err != nil {
			return err
		}
		var err error
		out, err = pipelineVersionTx(ctx, tx, e, id, version, "engineering.pipeline.publish", "write")
		if err != nil {
			return err
		}
		if out.Published != nil {
			return nil
		}
		var current int64
		if err = tx.QueryRow(ctx, `SELECT draft_version FROM chartworks.pipeline_heads WHERE tenant_id=$1 AND pipeline_id=$2 FOR UPDATE`, e.Tenant(), id).Scan(&current); err != nil {
			return err
		}
		if current != version {
			return store.ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_versions SET published_at=clock_timestamp() WHERE tenant_id=$1 AND pipeline_id=$2 AND version=$3`, e.Tenant(), id, version); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_heads SET published_version=$3 WHERE tenant_id=$1 AND pipeline_id=$2`, e.Tenant(), id, version); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err = auditJob(ctx, tx, scope, "pipeline.published", id); err != nil {
			return err
		}
		out, err = pipelineVersionTx(ctx, tx, e, id, version, "engineering.pipeline.publish", "write")
		return err
	})
	if err != nil {
		return engineering.PipelineRecord{}, err
	}
	return out, nil
}
