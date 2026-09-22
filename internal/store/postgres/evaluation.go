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

// CreateRuntimePack persists one immutable server-owned runtime configuration draft.
func (d *DB) CreateRuntimePack(ctx context.Context, scope store.Scope, r evaluation.RuntimePackRecord) error {
	if checkScope(scope) != nil || r.Validate() != nil || r.State != evaluation.Draft || r.Author != scope.Actor() || r.Review != nil {
		return store.ErrInvalid
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_runtime_packs(tenant_id,pack_digest,configuration_digest,runtime_digest,material,state,author_id,created_at) VALUES($1,$2,$3,$4,$5::jsonb,'draft',$6,$7) ON CONFLICT DO NOTHING`, scope.Tenant(), r.Pack.Digest, r.Pack.ConfigurationDigest, r.Digest, raw, r.Author, r.CreatedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// ReviewRuntimePack atomically verifies the reviewer-visible effective routing and cost.
func (d *DB) ReviewRuntimePack(ctx context.Context, scope store.Scope, review evaluation.RuntimePackReview) (out evaluation.RuntimePackRecord, err error) {
	if checkScope(scope) != nil || review.Reviewer != scope.Actor() {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		var author, state, runtimeDigest, configDigest string
		if err := tx.QueryRow(ctx, `SELECT material,author_id,state,runtime_digest,configuration_digest FROM chartworks.evaluation_runtime_packs WHERE tenant_id=$1 AND pack_digest=$2 FOR UPDATE`, scope.Tenant(), review.PackDigest).Scan(&raw, &author, &state, &runtimeDigest, &configDigest); err != nil {
			return err
		}
		if state != "draft" || author == review.Reviewer || runtimeDigest != review.RuntimeDigest || configDigest != review.ConfigurationDigest || json.Unmarshal(raw, &out) != nil {
			return store.ErrConflict
		}
		if out.Pack.ID != review.PackID || out.Pack.Revision != review.PackRevision || out.Pack.Digest != review.PackDigest || out.Pack.ConfigurationDigest != review.ConfigurationDigest || out.Pack.Model != review.Model || out.Config.Model != review.Model || out.Config.SystemInstruction != review.SystemInstruction || out.Config.AttemptCostUSD != review.MaxAttemptCostUSD || len(out.Config.Models) != len(review.Models) {
			return store.ErrConflict
		}
		for i := range out.Config.Models {
			if out.Config.Models[i] != review.Models[i] {
				return store.ErrConflict
			}
		}
		reviewRaw, _ := json.Marshal(review)
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_runtime_packs SET state=$4,review=$5::jsonb WHERE tenant_id=$1 AND pack_digest=$2 AND runtime_digest=$3 AND state='draft'`, scope.Tenant(), review.PackDigest, review.RuntimeDigest, review.Decision, reviewRaw)
		if err != nil || tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		out.State, out.Review = review.Decision, &review
		return nil
	})
	return out, err
}

// AcceptedRuntimePack resolves only an accepted exact pack/configuration binding.
func (d *DB) AcceptedRuntimePack(ctx context.Context, scope store.Scope, packDigest, configDigest string) (out evaluation.RuntimePackRecord, err error) {
	if checkScope(scope) != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw, reviewRaw []byte
		var state string
		if err := tx.QueryRow(ctx, `SELECT material,state,review FROM chartworks.evaluation_runtime_packs WHERE tenant_id=$1 AND pack_digest=$2 AND configuration_digest=$3 AND state='accepted'`, scope.Tenant(), packDigest, configDigest).Scan(&raw, &state, &reviewRaw); err != nil {
			return err
		}
		if json.Unmarshal(raw, &out) != nil || json.Unmarshal(reviewRaw, &out.Review) != nil {
			return store.ErrMigration
		}
		out.State = evaluation.Lifecycle(state)
		if out.Validate() != nil {
			return store.ErrMigration
		}
		return nil
	})
	return out, err
}

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
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_inputs(tenant_id,actor_id,input_digest,material) VALUES($1,$2,$3,$4::jsonb) ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), ref.Digest, raw)
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
		if err := tx.QueryRow(ctx, `SELECT material FROM chartworks.evaluation_inputs WHERE tenant_id=$1 AND actor_id=$2 AND input_digest=$3`, e.Tenant(), e.User(), ref.Digest).Scan(&raw); err != nil {
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
		var suite evaluation.Suite
		if json.Unmarshal(raw, &suite) != nil || validateHeldoutTx(ctx, tx, scope.Tenant(), suite.Cases) != nil {
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
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_runs(tenant_id,actor_id,run_id,suite_id,suite_revision,suite_digest,pack_digest,status) VALUES($1,$2,$3,$4,$5,$6,$7,'running') ON CONFLICT DO NOTHING`, scope.Tenant(), scope.Actor(), in.RunID, in.SuiteID, in.SuiteRevision, in.SuiteDigest, in.PackDigest)
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
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_runs SET evidence_hash=$4,mode=$5,gate_passed=$6,report=$7::jsonb,status=$8,completed_at=$9 WHERE tenant_id=$1 AND actor_id=$2 AND run_id=$3 AND status='running' AND pack_digest=$10`, scope.Tenant(), scope.Actor(), r.RunID, r.EvidenceHash, r.Mode, r.GatePassed, raw, r.Status, r.CompletedAt, r.Pack.Digest)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return nil
	})
}

// ReadReport reads actor-scoped terminal evidence.
func (d *DB) ReadReport(ctx context.Context, scope store.Scope, id string) (out evaluation.Report, err error) {
	if checkScope(scope) != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT report FROM chartworks.evaluation_runs WHERE tenant_id=$1 AND actor_id=$2 AND run_id=$3 AND report IS NOT NULL`, scope.Tenant(), scope.Actor(), id).Scan(&raw); err != nil {
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
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_runs SET cancel_requested=true WHERE tenant_id=$1 AND actor_id=$2 AND run_id=$3 AND status='running'`, scope.Tenant(), scope.Actor(), id)
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
		var packDigest string
		if err := tx.QueryRow(ctx, `SELECT s.manifest,r.created_at,r.pack_digest FROM chartworks.evaluation_runs r JOIN chartworks.evaluation_suites s ON s.tenant_id=r.tenant_id AND s.suite_id=r.suite_id AND s.revision=r.suite_revision AND s.manifest_digest=r.suite_digest WHERE r.tenant_id=$1 AND r.actor_id=$2 AND r.run_id=$3 AND r.status='running' FOR UPDATE OF r`, scope.Tenant(), scope.Actor(), id).Scan(&raw, &started, &packDigest); err != nil {
			return err
		}
		var suite evaluation.Suite
		if json.Unmarshal(raw, &suite) != nil {
			return store.ErrMigration
		}
		pack, ok := suite.PackByDigest(packDigest)
		if !ok {
			return store.ErrMigration
		}
		report, makeErr := evaluation.FailureReportWithPack(id, suite, pack, "dependency_failed", "crash_recovered", started, completed)
		if makeErr != nil {
			return store.ErrMigration
		}
		reportRaw, _ := json.Marshal(report)
		tag, err := tx.Exec(ctx, `UPDATE chartworks.evaluation_runs SET evidence_hash=$4,mode=$5,gate_passed=false,report=$6::jsonb,status='dependency_failed',completed_at=$7 WHERE tenant_id=$1 AND actor_id=$2 AND run_id=$3 AND status='running'`, scope.Tenant(), scope.Actor(), id, report.EvidenceHash, report.Mode, reportRaw, report.CompletedAt)
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

// SaveFeedbackExport stores one immutable unassigned candidate ledger.
func (d *DB) SaveFeedbackExport(ctx context.Context, scope store.Scope, x evaluation.CandidateExport) error {
	if checkScope(scope) != nil || x.ValidateExport() != nil || x.Split != "candidate" || x.Author != scope.Actor() {
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

// ReadFeedbackExport reads one exact protected split ledger.
func (d *DB) ReadFeedbackExport(ctx context.Context, scope store.Scope, id string) (out evaluation.CandidateExport, err error) {
	if checkScope(scope) != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT manifest FROM chartworks.evaluation_feedback_exports WHERE tenant_id=$1 AND export_id=$2`, scope.Tenant(), id).Scan(&raw); err != nil {
			return err
		}
		if json.Unmarshal(raw, &out) != nil {
			return store.ErrMigration
		}
		return nil
	})
	return out, err
}

// ReviewFeedbackSplit atomically creates disjoint training and heldout children from an unexposed candidate.
func (d *DB) ReviewFeedbackSplit(ctx context.Context, scope store.Scope, candidateID, candidateDigest string, out evaluation.FeedbackSplit) error {
	if checkScope(scope) != nil || out.ValidateSplit() != nil || out.Training.Reviewer != scope.Actor() || out.CandidateDigest != candidateDigest || out.CandidateID != candidateID {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var author string
		var candidateRaw []byte
		if err := tx.QueryRow(ctx, `SELECT actor_id,manifest FROM chartworks.evaluation_feedback_exports WHERE tenant_id=$1 AND export_id=$2 AND evidence_hash=$3 AND split='candidate' FOR UPDATE`, scope.Tenant(), candidateID, candidateDigest).Scan(&author, &candidateRaw); err != nil {
			return err
		}
		if author == scope.Actor() {
			return store.ErrConflict
		}
		if out.Training.Author != author || out.Heldout.Author != author {
			return store.ErrConflict
		}
		var candidate evaluation.CandidateExport
		if json.Unmarshal(candidateRaw, &candidate) != nil || candidate.ValidateExport() != nil {
			return store.ErrMigration
		}
		want := map[string]string{}
		for _, c := range candidate.Cases {
			want[c.ID] = c.Input.Digest
		}
		seen := map[string]bool{}
		for _, child := range []evaluation.CandidateExport{out.Training, out.Heldout} {
			for _, c := range child.Cases {
				if seen[c.ID] || want[c.ID] != c.Input.Digest {
					return store.ErrConflict
				}
				seen[c.ID] = true
			}
		}
		if len(seen) != len(want) {
			return store.ErrConflict
		}
		for _, child := range []evaluation.CandidateExport{out.Training, out.Heldout} {
			raw, _ := json.Marshal(child)
			tag, err := tx.Exec(ctx, `INSERT INTO chartworks.evaluation_feedback_exports(tenant_id,actor_id,export_id,evidence_hash,split,parent_digest,reviewer_id,reviewed_at,manifest,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10) ON CONFLICT DO NOTHING`, scope.Tenant(), author, child.ID, child.EvidenceHash, child.Split, candidateDigest, scope.Actor(), child.ReviewedAt, raw, child.CreatedAt)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return store.ErrConflict
			}
		}
		return nil
	})
}

// ValidateHeldoutCases rejects training-origin material without a reviewed heldout child.
func (d *DB) ValidateHeldoutCases(ctx context.Context, scope store.Scope, cases []evaluation.Case) error {
	if checkScope(scope) != nil {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return validateHeldoutTx(ctx, tx, scope.Tenant(), cases)
	})
}

// ValidateOptimizationHeldout requires every optimized case to originate in one independently reviewed heldout ledger.
func (d *DB) ValidateOptimizationHeldout(ctx context.Context, scope store.Scope, suite evaluation.Suite) error {
	if checkScope(scope) != nil || suite.Mode != evaluation.Live || suite.HeldoutLineageDigest == "" {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT manifest FROM chartworks.evaluation_feedback_exports WHERE tenant_id=$1 AND evidence_hash=$2 AND split='heldout'`, scope.Tenant(), suite.HeldoutLineageDigest).Scan(&raw); err != nil {
			return err
		}
		var export evaluation.CandidateExport
		if json.Unmarshal(raw, &export) != nil || export.ValidateExport() != nil {
			return store.ErrMigration
		}
		allowed := map[string]bool{}
		for _, c := range export.Cases {
			allowed[c.Input.Digest] = true
		}
		count := 0
		for _, c := range suite.Cases {
			if c.HeldOut {
				count++
				if !allowed[c.Input.Digest] {
					return store.ErrConflict
				}
			}
		}
		if count == 0 {
			return store.ErrConflict
		}
		return nil
	})
}

func validateHeldoutTx(ctx context.Context, tx pgx.Tx, tenant string, cases []evaluation.Case) error {
	rows, err := tx.Query(ctx, `SELECT manifest FROM chartworks.evaluation_feedback_exports WHERE tenant_id=$1`, tenant)
	if err != nil {
		return err
	}
	defer rows.Close()
	candidate, training, heldout := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var raw []byte
		var x evaluation.CandidateExport
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &x) != nil {
			return store.ErrMigration
		}
		for _, c := range x.Cases {
			switch x.Split {
			case "candidate":
				candidate[c.Input.Digest] = true
			case "training":
				training[c.Input.Digest] = true
			case "heldout":
				heldout[c.Input.Digest] = true
			}
		}
	}
	for _, c := range cases {
		if c.HeldOut && (candidate[c.Input.Digest] || training[c.Input.Digest]) && !heldout[c.Input.Digest] {
			return store.ErrConflict
		}
		if training[c.Input.Digest] && heldout[c.Input.Digest] {
			return store.ErrMigration
		}
	}
	return rows.Err()
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

// SelectedPack returns the current approved production configuration pointer.
func (d *DB) SelectedPack(ctx context.Context, scope store.Scope) (out evaluation.PackSelection, err error) {
	if checkScope(scope) != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT revision,pack_digest,proposal_id,actor_id,selected_at FROM chartworks.evaluation_pack_selection WHERE tenant_id=$1`, scope.Tenant()).Scan(&out.Revision, &out.PackDigest, &out.ProposalID, &out.Actor, &out.SelectedAt)
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
