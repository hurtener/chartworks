package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ evaluation.Repository = (*DB)(nil)
var _ evaluation.LiveInputResolver = (*DB)(nil)

// SaveInput stores protected live material under its canonical digest.
func (d *DB) SaveInput(ctx context.Context, scope store.Scope, ref evaluation.ProtectedRef, in evaluation.LiveInput) error {
	if checkScope(scope) != nil {
		return store.ErrInvalid
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return store.ErrInvalid
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != ref.Digest {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_inputs(tenant_id,input_digest,material) VALUES($1,$2,$3::jsonb) ON CONFLICT DO NOTHING`, scope.Tenant(), ref.Digest, raw)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// ResolveEvaluationInput reads protected material only after current signed tenant authority.
func (d *DB) ResolveEvaluationInput(ctx context.Context, e identity.Envelope, ref evaluation.ProtectedRef) (out evaluation.LiveInput, err error) {
	if !e.Valid() || ref.Digest == "" {
		return out, store.ErrInvalid
	}
	if err = access.Require(e, "ops.write", access.Tenant(e, "write")); err != nil {
		return out, err
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT material FROM chartworks.evaluation_inputs WHERE tenant_id=$1 AND input_digest=$2`, e.Tenant(), ref.Digest).Scan(&raw); err != nil {
			return err
		}
		if json.Unmarshal(raw, &out) != nil {
			return store.ErrMigration
		}
		return nil
	})
	return out, err
}

// CreateSuite stores one immutable draft revision.
func (d *DB) CreateSuite(ctx context.Context, scope store.Scope, r evaluation.SuiteRecord) error {
	if checkScope(scope) != nil || r.Suite.Validate() != nil || r.State != evaluation.Draft || r.Author != scope.Actor() {
		return store.ErrInvalid
	}
	want, _ := r.Suite.Digest()
	if want != r.Digest {
		return store.ErrInvalid
	}
	raw, _ := json.Marshal(r.Suite)
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_suites(tenant_id,suite_id,revision,manifest_digest,manifest,state,author_id,created_at) VALUES($1,$2,$3,$4,$5::jsonb,'draft',$6,$7) ON CONFLICT DO NOTHING`, scope.Tenant(), r.Suite.ID, r.Suite.Revision, r.Digest, raw, r.Author, r.CreatedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return store.ErrConflict
		}
		return nil
	})
}

// ReviewSuite atomically records one distinct exact-revision decision.
func (d *DB) ReviewSuite(ctx context.Context, scope store.Scope, r evaluation.SuiteReview) (out evaluation.SuiteRecord, err error) {
	if checkScope(scope) != nil || r.Reviewer != scope.Actor() {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		var author, state, dig string
		if err := tx.QueryRow(ctx, `SELECT manifest,author_id,state,manifest_digest FROM chartworks.evaluation_suites WHERE tenant_id=$1 AND suite_id=$2 AND revision=$3 FOR UPDATE`, scope.Tenant(), r.SuiteID, r.Revision).Scan(&raw, &author, &state, &dig); err != nil {
			return err
		}
		if state != "draft" || dig != r.Digest || author == r.Reviewer {
			return store.ErrConflict
		}
		reviewRaw, _ := json.Marshal(r)
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_suites SET state=$5,review=$6::jsonb WHERE tenant_id=$1 AND suite_id=$2 AND revision=$3 AND manifest_digest=$4 AND state='draft'`, scope.Tenant(), r.SuiteID, r.Revision, r.Digest, r.Decision, reviewRaw)
		if err != nil || tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		if json.Unmarshal(raw, &out.Suite) != nil {
			return store.ErrMigration
		}
		out = evaluation.SuiteRecord{Suite: out.Suite, Digest: dig, State: r.Decision, Author: author, Review: &r}
		return nil
	})
	return out, err
}

// AcceptedSuite reads only an exact accepted suite revision.
func (d *DB) AcceptedSuite(ctx context.Context, scope store.Scope, id string, rev int64, dig string) (out evaluation.SuiteRecord, err error) {
	if checkScope(scope) != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw, review []byte
		var author, state string
		var created time.Time
		if err := tx.QueryRow(ctx, `SELECT manifest,author_id,state,review,created_at FROM chartworks.evaluation_suites WHERE tenant_id=$1 AND suite_id=$2 AND revision=$3 AND manifest_digest=$4 AND state='accepted'`, scope.Tenant(), id, rev, dig).Scan(&raw, &author, &state, &review, &created); err != nil {
			return err
		}
		if json.Unmarshal(raw, &out.Suite) != nil || json.Unmarshal(review, &out.Review) != nil {
			return store.ErrMigration
		}
		out.Digest, out.State, out.Author, out.CreatedAt = dig, evaluation.Lifecycle(state), author, created
		return nil
	})
	return out, err
}

// BeginRun durably admits a run before delegated work.
func (d *DB) BeginRun(ctx context.Context, scope store.Scope, in evaluation.RunRequest) error {
	if checkScope(scope) != nil {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_runs(tenant_id,actor_id,run_id,suite_id,suite_revision,suite_digest,status) VALUES($1,$2,$3,$4,$5,$6,'running') ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), in.RunID, in.SuiteID, in.SuiteRevision, in.SuiteDigest)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// SaveReport commits terminal evidence for an admitted run.
func (d *DB) SaveReport(ctx context.Context, scope store.Scope, r evaluation.Report) error {
	if checkScope(scope) != nil || r.Validate() != nil {
		return store.ErrInvalid
	}
	raw, _ := json.Marshal(r)
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_runs SET evidence_hash=$4,mode=$5,gate_passed=$6,report=$7::jsonb,status=$8,completed_at=$9 WHERE tenant_id=$1 AND actor_id=$2 AND run_id=$3 AND status='running'`, scope.Tenant(), scope.Actor(), r.RunID, r.EvidenceHash, r.Mode, r.GatePassed, raw, r.Status, r.CompletedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// ReadReport reads tenant-scoped terminal evidence.
func (d *DB) ReadReport(ctx context.Context, scope store.Scope, id string) (out evaluation.Report, err error) {
	if checkScope(scope) != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT report FROM chartworks.evaluation_runs WHERE tenant_id=$1 AND run_id=$2 AND report IS NOT NULL`, scope.Tenant(), id).Scan(&raw); err != nil {
			return err
		}
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return store.ErrMigration
		}
		return nil
	})
	return out, err
}

// RequestCancel persists cancellation intent for an active run.
func (d *DB) RequestCancel(ctx context.Context, scope store.Scope, id string) error {
	if checkScope(scope) != nil {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_runs SET cancel_requested=true WHERE tenant_id=$1 AND run_id=$2 AND status='running'`, scope.Tenant(), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrNotFound
		}
		return nil
	})
}

// RecoverRun converts one abandoned admitted run to immutable typed evidence.
func (d *DB) RecoverRun(ctx context.Context, scope store.Scope, id string, completed time.Time) (out evaluation.Report, err error) {
	if checkScope(scope) != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		var started time.Time
		if err := tx.QueryRow(ctx, `SELECT s.manifest,r.created_at FROM chartworks.evaluation_runs r JOIN chartworks.evaluation_suites s ON s.tenant_id=r.tenant_id AND s.suite_id=r.suite_id AND s.revision=r.suite_revision AND s.manifest_digest=r.suite_digest WHERE r.tenant_id=$1 AND r.run_id=$2 AND r.status='running' FOR UPDATE OF r`, scope.Tenant(), id).Scan(&raw, &started); err != nil {
			return err
		}
		var suite evaluation.Suite
		if json.Unmarshal(raw, &suite) != nil {
			return store.ErrMigration
		}
		report, makeErr := evaluation.FailureReport(id, suite, "dependency_failed", "crash_recovered", started, completed)
		if makeErr != nil {
			return store.ErrMigration
		}
		reportRaw, _ := json.Marshal(report)
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_runs SET evidence_hash=$3,mode=$4,gate_passed=false,report=$5::jsonb,status='dependency_failed',completed_at=$6 WHERE tenant_id=$1 AND run_id=$2 AND status='running'`, scope.Tenant(), id, report.EvidenceHash, report.Mode, reportRaw, report.CompletedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		out = report
		return nil
	})
	return out, err
}

// SaveFeedbackExport stores one immutable training candidate ledger.
func (d *DB) SaveFeedbackExport(ctx context.Context, scope store.Scope, x evaluation.CandidateExport) error {
	if checkScope(scope) != nil {
		return store.ErrInvalid
	}
	raw, _ := json.Marshal(x)
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_feedback_exports(tenant_id,actor_id,export_id,evidence_hash,split,manifest,created_at) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7) ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), x.ID, x.EvidenceHash, x.Split, raw, x.CreatedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// SaveProposal stores one immutable optimization candidate.
func (d *DB) SaveProposal(ctx context.Context, scope store.Scope, p evaluation.OptimizationProposal) error {
	if checkScope(scope) != nil || p.Validate() != nil {
		return store.ErrInvalid
	}
	raw, _ := json.Marshal(p)
	dig, _ := digestJSON(p)
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_proposals(tenant_id,proposal_id,author_id,proposal_digest,proposal,state,created_at) VALUES($1,$2,$3,$4,$5::jsonb,'candidate',$6) ON CONFLICT DO NOTHING`, scope.Tenant(), p.ID, scope.Actor(), dig, raw, p.CreatedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// ReadProposal reads a tenant-scoped optimization candidate.
func (d *DB) ReadProposal(ctx context.Context, scope store.Scope, id string) (out evaluation.OptimizationProposal, err error) {
	if checkScope(scope) != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT proposal FROM chartworks.evaluation_proposals WHERE tenant_id=$1 AND proposal_id=$2`, scope.Tenant(), id).Scan(&raw); err != nil {
			return err
		}
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return store.ErrMigration
		}
		return nil
	})
	return out, err
}

// ReviewProposal records a distinct exact-proposal decision.
func (d *DB) ReviewProposal(ctx context.Context, scope store.Scope, r evaluation.ReviewReceipt) error {
	if checkScope(scope) != nil || r.Reviewer != scope.Actor() {
		return store.ErrInvalid
	}
	raw, _ := json.Marshal(r)
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_proposals SET state=$4,review=$5::jsonb WHERE tenant_id=$1 AND proposal_id=$2 AND proposal_digest=$3 AND state='candidate' AND author_id<>$6`, scope.Tenant(), r.ProposalID, r.ProposalDigest, r.Decision, raw, r.Reviewer)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// SelectPack advances the approved pack pointer by CAS.
func (d *DB) SelectPack(ctx context.Context, scope store.Scope, x evaluation.PackSelection, expected int64) (out evaluation.PackSelection, err error) {
	if checkScope(scope) != nil || x.Actor != scope.Actor() {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var approved bool
		if err := tx.QueryRow(ctx, `SELECT state='approve' FROM chartworks.evaluation_proposals WHERE tenant_id=$1 AND proposal_id=$2 AND (proposal->'candidate'->>'pack_digest')=$3`, scope.Tenant(), x.ProposalID, x.PackDigest).Scan(&approved); err != nil || !approved {
			if err != nil {
				return err
			}
			return store.ErrConflict
		}
		x.Revision = expected + 1
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_pack_selection(tenant_id,revision,pack_digest,proposal_id,actor_id,selected_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id) DO UPDATE SET revision=EXCLUDED.revision,pack_digest=EXCLUDED.pack_digest,proposal_id=EXCLUDED.proposal_id,actor_id=EXCLUDED.actor_id,selected_at=EXCLUDED.selected_at WHERE chartworks.evaluation_pack_selection.revision=$7`, scope.Tenant(), x.Revision, x.PackDigest, x.ProposalID, x.Actor, x.SelectedAt, expected)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		out = x
		return nil
	})
	return out, err
}

func digestJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
