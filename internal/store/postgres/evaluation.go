package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ evaluation.Repository = (*DB)(nil)

// SaveSuite stores one immutable tenant-and-actor-scoped suite revision.
func (d *DB) SaveSuite(ctx context.Context, scope store.Scope, suite evaluation.Suite, manifestDigest string) error {
	want, digestErr := suite.Digest()
	if checkScope(scope) != nil || digestErr != nil || manifestDigest != want {
		return store.ErrInvalid
	}
	raw, err := json.Marshal(suite)
	if err != nil {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_suites(tenant_id,actor_id,suite_id,revision,manifest_digest,manifest) VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), suite.ID, suite.Revision, manifestDigest, raw)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var got string
			if err = tx.QueryRow(ctx, `SELECT manifest_digest FROM chartworks.evaluation_suites WHERE tenant_id=$1 AND actor_id=$2 AND suite_id=$3 AND revision=$4`, scope.Tenant(), scope.Actor(), suite.ID, suite.Revision).Scan(&got); err != nil {
				return err
			}
			if got != manifestDigest {
				return store.ErrConflict
			}
		}
		return nil
	})
}

// SaveReport stores one immutable content-addressed evaluation run.
func (d *DB) SaveReport(ctx context.Context, scope store.Scope, report evaluation.Report) error {
	if checkScope(scope) != nil || report.Validate() != nil {
		return store.ErrInvalid
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_runs(tenant_id,actor_id,run_id,suite_id,suite_revision,suite_digest,evidence_hash,mode,gate_passed,report) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb) ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), report.RunID, report.SuiteID, report.SuiteRevision, report.SuiteDigest, report.EvidenceHash, report.Mode, report.GatePassed, raw)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// ReadReport reads only the exact tenant-and-actor-scoped run.
func (d *DB) ReadReport(ctx context.Context, scope store.Scope, runID string) (out evaluation.Report, err error) {
	if checkScope(scope) != nil || runID == "" {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT report FROM chartworks.evaluation_runs WHERE tenant_id=$1 AND actor_id=$2 AND run_id=$3`, scope.Tenant(), scope.Actor(), runID).Scan(&raw); err != nil {
			return err
		}
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return store.ErrMigration
		}
		return nil
	})
	return out, err
}
