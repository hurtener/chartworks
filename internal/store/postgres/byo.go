package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ nlqbyo.Repository = (*DB)(nil)

// CreateBYOBundle serializes tenant quota admission with the same transaction as
// snapshot/audit commit. Retained expired evidence counts until bounded cleanup.
func (d *DB) CreateBYOBundle(ctx context.Context, scope store.Scope, record nlqbyo.Record, limits config.QueryBundles) error {
	if config.ValidateQueryBundles(limits) != nil || !nlqbyo.RecordValid(record, scope, limits) {
		return store.ErrInvalid
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Fixed namespace; hash collisions only serialize unrelated tenants, they
		// cannot bypass quotas or the explicit tenant predicate on every query.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,190021))`, scope.Tenant()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM chartworks.byo_context_bundles WHERE (tenant_id,actor_id,session_id,bundle_id) IN (SELECT tenant_id,actor_id,session_id,bundle_id FROM chartworks.byo_context_bundles WHERE tenant_id=$1 AND retain_until<=$2 ORDER BY retain_until,bundle_id LIMIT 64 FOR UPDATE SKIP LOCKED)`, scope.Tenant(), record.Bundle.CreatedAt); err != nil {
			return err
		}
		var tenantCount, sessionCount int
		if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE actor_id=$2 AND session_id=$3) FROM chartworks.byo_context_bundles WHERE tenant_id=$1`, scope.Tenant(), scope.Actor(), record.Session).Scan(&tenantCount, &sessionCount); err != nil {
			return err
		}
		if tenantCount >= limits.PerTenant || sessionCount >= limits.PerSession {
			return nlqbyo.ErrBudget
		}
		_, err := tx.Exec(ctx, `INSERT INTO chartworks.byo_context_bundles(tenant_id,actor_id,session_id,bundle_id,context_id,payload,digest,max_steps,created_at,expires_at,retain_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, scope.Tenant(), scope.Actor(), record.Session, record.Bundle.ID, record.Bundle.Reference.Context, raw, record.Digest, record.Bundle.MaxSteps, record.Bundle.CreatedAt, record.Bundle.ExpiresAt, record.RetainUntil)
		if err != nil {
			return err
		}
		return auditJob(ctx, tx, scope, "byo.context_created", record.Bundle.ID)
	})
}

func byoCoordinates(scope store.Scope, in nlqbyo.Reference, session string) bool {
	return scope.Valid() && nlqbyo.ReferenceValid(in) && identity.Identifier(session)
}

// ReadBYOBundle applies all caller/partition/expiry predicates before payload read.
func (d *DB) ReadBYOBundle(ctx context.Context, scope store.Scope, in nlqbyo.Reference, session string, now time.Time) (out nlqbyo.Record, err error) {
	if !byoCoordinates(scope, in, session) || now.IsZero() {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		var digest string
		if err := tx.QueryRow(ctx, `SELECT payload,digest FROM chartworks.byo_context_bundles WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND bundle_id=$4 AND context_id=$5 AND expires_at>$6`, scope.Tenant(), scope.Actor(), session, in.ID, in.Context, now).Scan(&raw, &digest); err != nil {
			return err
		}
		if len(raw) > 1<<20 || json.Unmarshal(raw, &out) != nil || exec.Hash(out.Bundle) != digest || out.Digest != digest || out.Session != session || out.Bundle.Reference != in {
			return store.ErrMigration
		}
		return nil
	})
	return out, err
}

// ReserveBYOStep admits at most one physical attempt per operation and serializes
// step budgets under the immutable parent row. No lease/second queue is created.
func (d *DB) ReserveBYOStep(ctx context.Context, scope store.Scope, in nlqbyo.Reference, session string, step nlqbyo.Step, now time.Time) (out nlqbyo.Step, fresh bool, err error) {
	if !byoCoordinates(scope, in, session) || !nlqbyo.StepValid(step) || step.Number != 0 || step.Status != "accepted" || step.Execution != nil || step.FinishedAt != nil || now.IsZero() {
		return out, false, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var maximum int
		var digest string
		var expires time.Time
		if err := tx.QueryRow(ctx, `SELECT max_steps,digest,expires_at FROM chartworks.byo_context_bundles WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND bundle_id=$4 AND context_id=$5 AND expires_at>$6 FOR UPDATE`, scope.Tenant(), scope.Actor(), session, in.ID, in.Context, now).Scan(&maximum, &digest, &expires); err != nil {
			return err
		}
		if step.BundleDigest != digest || step.Deadline.After(expires) {
			return store.ErrInvalid
		}
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT receipt FROM chartworks.byo_steps WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND bundle_id=$4 AND operation=$5`, scope.Tenant(), scope.Actor(), session, in.ID, step.Operation).Scan(&raw)
		if err == nil {
			if json.Unmarshal(raw, &out) != nil || !nlqbyo.StepValid(out) {
				return store.ErrMigration
			}
			if out.InputDigest != step.InputDigest || out.BundleDigest != digest {
				return store.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.byo_steps WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND bundle_id=$4`, scope.Tenant(), scope.Actor(), session, in.ID).Scan(&count); err != nil {
			return err
		}
		if count >= maximum {
			return nlqbyo.ErrBudget
		}
		step.Number = count + 1
		raw, err = json.Marshal(step)
		if err != nil {
			return store.ErrInvalid
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.byo_steps(tenant_id,actor_id,session_id,bundle_id,operation,step,input_digest,receipt) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb)`, scope.Tenant(), scope.Actor(), session, in.ID, step.Operation, step.Number, step.InputDigest, raw); err != nil {
			return err
		}
		if err = auditJob(ctx, tx, scope, "byo.step_accepted", in.ID); err != nil {
			return err
		}
		out, fresh = step, true
		return nil
	})
	if errors.Is(err, store.ErrNotFound) {
		err = nlqbyo.ErrReplan
	}
	return out, fresh, err
}

// FinishBYOStep seals content-free evidence once; database fences prohibit any
// request/provenance rewrite or a second terminal update, including concurrent ones.
func (d *DB) FinishBYOStep(ctx context.Context, scope store.Scope, in nlqbyo.Reference, session string, step nlqbyo.Step) error {
	if !byoCoordinates(scope, in, session) || !nlqbyo.StepValid(step) || step.Number < 1 || step.Status == "accepted" || step.FinishedAt == nil {
		return store.ErrInvalid
	}
	raw, err := json.Marshal(step)
	if err != nil {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chartworks.byo_steps s SET receipt=$6::jsonb,terminal=true WHERE s.tenant_id=$1 AND s.actor_id=$2 AND s.session_id=$3 AND s.bundle_id=$4 AND s.operation=$5 AND NOT s.terminal AND EXISTS(SELECT 1 FROM chartworks.byo_context_bundles b WHERE (b.tenant_id,b.actor_id,b.session_id,b.bundle_id)=(s.tenant_id,s.actor_id,s.session_id,s.bundle_id) AND b.context_id=$7)`, scope.Tenant(), scope.Actor(), session, in.ID, step.Operation, raw, in.Context)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return auditJob(ctx, tx, scope, "byo.step_finished", in.ID)
	})
}

// ReadBYOSteps returns only the requested caller's bounded evidence, ordered by step.
func (d *DB) ReadBYOSteps(ctx context.Context, scope store.Scope, in nlqbyo.Reference, session string) (out []nlqbyo.Step, err error) {
	if !byoCoordinates(scope, in, session) {
		return nil, store.ErrInvalid
	}
	out = []nlqbyo.Step{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT s.receipt FROM chartworks.byo_steps s JOIN chartworks.byo_context_bundles b USING(tenant_id,actor_id,session_id,bundle_id) WHERE s.tenant_id=$1 AND s.actor_id=$2 AND s.session_id=$3 AND s.bundle_id=$4 AND b.context_id=$5 ORDER BY s.step LIMIT 32`, scope.Tenant(), scope.Actor(), session, in.ID, in.Context)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var step nlqbyo.Step
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			if json.Unmarshal(raw, &step) != nil || !nlqbyo.StepValid(step) {
				return store.ErrMigration
			}
			out = append(out, step)
		}
		return rows.Err()
	})
	return out, err
}
