package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func readPipelineExecutionTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, operation string) (out engineering.PipelineExecution, err error) {
	task, err := readRequestTx(ctx, tx, e, operation, false)
	if err != nil {
		return out, err
	}
	if task.Input.Kind != "pipeline.run" {
		return out, store.ErrNotFound
	}
	out.Operation = task
	err = tx.QueryRow(ctx, `SELECT pipeline_id,version,manifest_hash,state FROM chartworks.pipeline_runs WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), operation).Scan(&out.Pipeline, &out.Version, &out.Digest, &out.State)
	if err != nil {
		return out, err
	}
	if out.Pipeline != task.Input.Target || out.Digest != task.Input.InputHash {
		return engineering.PipelineExecution{}, store.ErrInvalid
	}
	rows, err := tx.Query(ctx, `SELECT stage FROM chartworks.pipeline_stages WHERE tenant_id=$1 AND operation_id=$2 ORDER BY step_id`, e.Tenant(), operation)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.Stages = []engineering.PipelineStageState{}
	for rows.Next() {
		var raw []byte
		var s engineering.PipelineStageState
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &s) != nil || !s.Valid() || s.Stage.Tenant != e.Tenant() || s.Stage.Operation != operation || s.Stage.Pipeline != out.Pipeline || s.Stage.Version != out.Version {
			return engineering.PipelineExecution{}, store.ErrInvalid
		}
		out.Stages = append(out.Stages, s)
		if s.State == "dispatching" && task.State != "running" {
			out.State = "uncertain"
		}
	}
	if err = rows.Err(); err != nil {
		return engineering.PipelineExecution{}, err
	}
	return out, nil
}

// ReadPipelineExecution returns an authorized actor/session-private operation receipt.
func (d *DB) ReadPipelineExecution(ctx context.Context, e identity.Envelope, operation string) (out engineering.PipelineExecution, err error) {
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var x error
		out, x = readPipelineExecutionTx(ctx, tx, e, operation)
		return x
	})
	if err != nil {
		return engineering.PipelineExecution{}, err
	}
	return out, nil
}

// ReservePipelineExecution records the immutable accepted manifest and prepared stages.
func (d *DB) ReservePipelineExecution(ctx context.Context, e identity.Envelope, r engineering.PipelineRecord, task jobs.RequestTask, schema string) (out engineering.PipelineExecution, err error) {
	if task.Require(e) != nil || r.Published == nil || r.Digest != task.Input.InputHash || r.Definition.ID != task.Input.Target || task.Input.Kind != "pipeline.run" || !readexec.SQLIdentifier(schema) {
		return out, jobs.ErrAuthority
	}
	if err = r.Require(e, "engineering.pipeline.run", "write"); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		actual, err := readRequestTx(ctx, tx, e, task.ID, true)
		if err != nil {
			return err
		}
		if actual.ManifestHash != task.ManifestHash {
			return store.ErrConflict
		}
		out, err = readPipelineExecutionTx(ctx, tx, e, task.ID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061311))`, e.Tenant()+":"+r.Definition.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.pipeline_runs(tenant_id,operation_id,pipeline_id,version,manifest_hash,state) VALUES($1,$2,$3,$4,$5,'staged')`, e.Tenant(), task.ID, r.Definition.ID, r.Version, r.Digest); err != nil {
			return err
		}
		for _, step := range r.Definition.Steps {
			source := r.Definition.ID + "." + step.ID
			var prior engineering.PipelineStageState
			var priorRaw []byte
			err = tx.QueryRow(ctx, `SELECT s.stage FROM chartworks.pipeline_outputs p JOIN chartworks.pipeline_stages s USING(tenant_id,operation_id,step_id) WHERE p.tenant_id=$1 AND p.pipeline_id=$2 AND p.step_id=$3`, e.Tenant(), r.Definition.ID, step.ID).Scan(&priorRaw)
			var previous *sources.PipelineStage
			revision := int64(1)
			if err == nil {
				if json.Unmarshal(priorRaw, &prior) != nil || !prior.Stage.Valid() {
					return store.ErrInvalid
				}
				previous = &prior.Stage
				revision = prior.Stage.Revision + 1
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			columns := []string{}
			for _, col := range step.Columns {
				columns = append(columns, col.Name)
			}
			if step.Strategy == "scd2" {
				columns = append([]string{"_valid_from"}, columns...)
				columns = append(columns, "_valid_until", "_is_current")
			}
			stage := sources.PipelineStage{Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Pipeline: r.Definition.ID, Version: r.Version, Revision: revision, Operation: task.ID, Step: step.ID, Alias: r.Definition.Connection, Schema: schema, Table: "cw_p_" + readexec.Hash([]string{e.Tenant(), task.ID, step.ID})[:32], Columns: columns, Source: source, Context: source + ":v" + strconv.FormatInt(revision, 10), State: "prepared"}
			state := engineering.PipelineStageState{Stage: stage, Previous: previous, State: "prepared"}
			raw, _ := json.Marshal(state)
			if _, err = tx.Exec(ctx, `INSERT INTO chartworks.pipeline_stages(tenant_id,operation_id,step_id,source_id,source_revision,stage) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), task.ID, step.ID, source, revision, raw); err != nil {
				return err
			}
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err = auditJob(ctx, tx, scope, "pipeline.staged", task.ID); err != nil {
			return err
		}
		out, err = readPipelineExecutionTx(ctx, tx, e, task.ID)
		return err
	})
	if err != nil {
		return engineering.PipelineExecution{}, err
	}
	return out, nil
}

// MutatePipelineStage persists stage evidence under the live operation fence.
func (d *DB) MutatePipelineStage(ctx context.Context, i jobs.Invocation, r engineering.PipelineRecord, next engineering.PipelineStageState) (out engineering.PipelineExecution, err error) {
	e, err := i.Current("pipeline.run", r.Definition.ID, r.Digest)
	if err != nil {
		return out, err
	}
	if err = r.Require(e, "engineering.pipeline.run", "write"); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := requestFenceTx(ctx, tx, i); err != nil {
			return err
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT stage FROM chartworks.pipeline_stages WHERE tenant_id=$1 AND operation_id=$2 AND step_id=$3 FOR UPDATE`, e.Tenant(), i.Lease().Task.ID, next.Stage.Step).Scan(&raw); err != nil {
			return err
		}
		var prior engineering.PipelineStageState
		if json.Unmarshal(raw, &prior) != nil {
			return store.ErrInvalid
		}
		a, b := prior.Stage, next.Stage
		a.OID = 0
		b.OID = 0
		a.State = ""
		b.State = ""
		a.Digest = ""
		b.Digest = ""
		if readexec.Hash(a) != readexec.Hash(b) || readexec.Hash(prior.Previous) != readexec.Hash(next.Previous) {
			return store.ErrConflict
		}
		if prior.State == "checked" {
			if readexec.Hash(prior) == readexec.Hash(next) {
				out, err = readPipelineExecutionTx(ctx, tx, e, i.Lease().Task.ID)
				return err
			}
			return store.ErrConflict
		}
		switch next.State {
		case "dispatching":
			if next.Fence != i.Lease().Fence || prior.State == "dispatching" && prior.Fence >= next.Fence {
				return store.ErrConflict
			}
		case "applied", "checked", "quality_failed":
			if prior.Fence != i.Lease().Fence || next.Fence != prior.Fence || next.Stage.OID <= 0 {
				return store.ErrConflict
			}
		default:
			return store.ErrInvalid
		}
		if !next.Valid() {
			return store.ErrInvalid
		}
		raw, _ = json.Marshal(next)
		if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_stages SET stage=$4 WHERE tenant_id=$1 AND operation_id=$2 AND step_id=$3`, e.Tenant(), i.Lease().Task.ID, next.Stage.Step, raw); err != nil {
			return err
		}
		if next.State == "quality_failed" {
			if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_runs SET state='quality_failed' WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), i.Lease().Task.ID); err != nil {
				return err
			}
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if next.Compensated && (next.State == "checked" || next.State == "quality_failed") {
			if err = auditJob(ctx, tx, scope, "pipeline.reconciled", i.Lease().Task.ID); err != nil {
				return err
			}
		}
		if err = auditJob(ctx, tx, scope, "pipeline.effect", i.Lease().Task.ID); err != nil {
			return err
		}
		out, err = readPipelineExecutionTx(ctx, tx, e, i.Lease().Task.ID)
		return err
	})
	if err != nil {
		return engineering.PipelineExecution{}, err
	}
	return out, nil
}

// ReadPipelineStage returns an authorized checked stage for a private pipeline input.
func (d *DB) ReadPipelineStage(ctx context.Context, e identity.Envelope, operation, step string) (sources.PipelineStage, error) {
	x, err := d.ReadPipelineExecution(ctx, e, operation)
	if err != nil {
		return sources.PipelineStage{}, err
	}
	for _, s := range x.Stages {
		if s.Stage.Step == step && s.State == "checked" && s.Stage.Valid() {
			return s.Stage, nil
		}
	}
	return sources.PipelineStage{}, store.ErrNotFound
}

// CompletePipelineExecution atomically activates the complete manifest under the live fence.
func (d *DB) CompletePipelineExecution(ctx context.Context, i jobs.Invocation, r engineering.PipelineRecord, records []sources.Record) error {
	e, err := i.Current("pipeline.run", r.Definition.ID, r.Digest)
	if err != nil {
		return err
	}
	if err = r.Require(e, "engineering.pipeline.run", "write"); err != nil {
		return err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return err
	}
	defer stop()
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := requestFenceTx(ctx, tx, i); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061311))`, e.Tenant()+":"+r.Definition.ID); err != nil {
			return err
		}
		current, err := readPipelineExecutionTx(ctx, tx, e, i.Lease().Task.ID)
		if err != nil {
			return err
		}
		if current.State == "published" {
			return store.ErrConflict
		}
		if len(records) != len(current.Stages) {
			return store.ErrInvalid
		}
		byID := map[string]sources.Record{}
		for _, record := range records {
			if !record.Valid() || byID[record.Source.ID].Source.ID != "" {
				return store.ErrInvalid
			}
			byID[record.Source.ID] = record
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		for _, s := range current.Stages {
			record, ok := byID[s.Stage.Source]
			if !ok || s.State != "checked" || !s.Stage.Valid() || record.Pipeline == nil || *record.Pipeline != s.Stage.Location() || record.Source.Revision != s.Stage.Revision {
				return store.ErrInvalid
			}
			if err = access.Require(e, "engineering.pipeline.run", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: s.Stage.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: s.Stage.Context}); err != nil {
				return err
			}
			if err = putSourceTx(ctx, tx, scope, s.Stage.Revision-1, record, true); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO chartworks.pipeline_outputs(tenant_id,pipeline_id,step_id,operation_id,source_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,pipeline_id,step_id) DO UPDATE SET operation_id=EXCLUDED.operation_id,source_id=EXCLUDED.source_id`, e.Tenant(), r.Definition.ID, s.Stage.Step, i.Lease().Task.ID, s.Stage.Source); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.pipeline_runs SET state='published' WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), i.Lease().Task.ID); err != nil {
			return err
		}
		if err = auditJob(ctx, tx, scope, "pipeline.activated", i.Lease().Task.ID); err != nil {
			return err
		}
		return completeRequestTx(ctx, tx, i)
	})
}
