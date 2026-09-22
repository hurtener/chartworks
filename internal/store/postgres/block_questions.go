package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/jackc/pgx/v5"
)

// RecordQuestionAssessment persists only service-derived, permission-filtered
// evidence. The verified envelope supplies tenant, actor and session coordinates.
func (d *DB) RecordQuestionAssessment(ctx context.Context, e identity.Envelope, record reporting.QuestionAssessmentRecord) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	raw, err := json.Marshal(record)
	if err != nil || len(raw) > 256<<10 {
		return reporting.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return err
	}
	defer cancel()
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO chartworks.block_question_assessments
 (tenant_id,assessment_id,actor_id,session_id,request_digest,candidate_scope_digest,evidence_digest,authority_digest,question_threshold,record,created_at)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	ON CONFLICT(tenant_id,assessment_id) DO NOTHING`, e.Tenant(), record.ID, e.User(), e.Session(), record.RequestDigest, record.CandidateScopeDigest, record.EvidenceDigest, record.AuthorityDigest, record.Threshold, raw, record.CreatedAt)
		return err
	})
}
