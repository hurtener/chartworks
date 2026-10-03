package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ nlqexec.ExampleRequalificationRepository = (*DB)(nil)

// RequalifyExample preserves the old immutable origin and creates only a new
// candidate. Origin/version CAS and current publication/source fences share the
// same transaction as insertion, including exact idempotent retry validation.
func (d *DB) RequalifyExample(ctx context.Context, scope store.Scope, x nlqexec.ExampleRecord) (out nlqexec.ExampleRecord, err error) {
	r := x.Origin.Requalification
	if checkScope(scope) != nil || r == nil || !r.Valid() || !nlqexec.ExampleParametersValid(x) || x.State != "candidate" || x.Version != 1 || x.ID == "" || x.Origin.SchemaVersion != 1 || x.PositiveEvidence < 1 || x.ReviewedAt != nil || x.ReviewedBy != "" || x.ReviewNote != "" {
		return out, store.ErrInvalid
	}
	origin, err := marshalNLQ(x.Origin)
	if err != nil {
		return out, err
	}
	var schema []byte
	if x.ParameterSchema != nil {
		schema, err = marshalNLQ(x.ParameterSchema)
		if err != nil {
			return out, err
		}
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := queryRetentionFence(ctx, tx, scope.Tenant()); err != nil {
			return err
		}
		const columns = `example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,uncertainty,positive_evidence,negative_evidence,origin,version,reviewed_by,review_note,reviewed_at,provenance,created_at,updated_at,parameter_schema`
		var old nlqexec.ExampleRecord
		if err := scanExample(tx.QueryRow(ctx, `SELECT `+columns+` FROM chartworks.nlq_examples WHERE tenant_id=$1 AND example_id=$2 FOR SHARE`, scope.Tenant(), r.ExampleID), &old); err != nil {
			return err
		}
		if old.Version != r.Version || old.Digest != r.Digest || exec.Hash(old.Origin) != r.OriginDigest || old.Topic != x.Topic || old.Question != x.Question || old.SQL != x.SQL || exec.Hash(old.ParameterSchema) != exec.Hash(x.ParameterSchema) || old.PositiveEvidence != x.PositiveEvidence || old.NegativeEvidence != x.NegativeEvidence || old.EvidenceCount != x.EvidenceCount {
			return store.ErrConflict
		}
		var version string
		if err := tx.QueryRow(ctx, `SELECT active_version FROM chartworks.topic_publication_heads WHERE tenant_id=$1 AND topic_id=$2 AND NOT archived FOR UPDATE`, scope.Tenant(), x.Topic).Scan(&version); err != nil {
			return err
		}
		if version != x.Origin.TopicVersion {
			return store.ErrConflict
		}
		var definition []byte
		if err := tx.QueryRow(ctx, `SELECT definition FROM chartworks.topic_published_versions WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3`, scope.Tenant(), x.Topic, version).Scan(&definition); err != nil {
			return err
		}
		var p topics.Definition
		if json.Unmarshal(definition, &p) != nil {
			return store.ErrMigration
		}
		for _, dataset := range p.Datasets {
			var revision int64
			if err := tx.QueryRow(ctx, `SELECT current_revision FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 AND NOT deleted FOR SHARE`, scope.Tenant(), dataset.Source.Source).Scan(&revision); err != nil {
				return err
			}
			if revision != dataset.Source.SourceRevision || dataset.Source.Context != x.Origin.Context {
				return exec.ErrBinding
			}
		}
		var rules string
		ruleErr := tx.QueryRow(ctx, `SELECT COALESCE(active_version,'') FROM chartworks.topic_rule_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR SHARE`, scope.Tenant(), x.Topic).Scan(&rules)
		if ruleErr != nil && !errors.Is(ruleErr, pgx.ErrNoRows) {
			return ruleErr
		}
		expectedRules := ""
		if len(x.Origin.RuleVersions) > 1 {
			return store.ErrInvalid
		}
		if len(x.Origin.RuleVersions) == 1 {
			expectedRules = x.Origin.RuleVersions[0]
		}
		if expectedRules != rules {
			return store.ErrConflict
		}
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.nlq_examples(tenant_id,example_id,topic_id,question,sql_text,digest,state,weight,evidence_count,provenance,created_at,updated_at,uncertainty,positive_evidence,negative_evidence,origin,version,parameter_schema) VALUES($1,$2,$3,$4,$5,$6,'candidate',$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,1,$16::jsonb) ON CONFLICT(tenant_id,topic_id,digest) DO NOTHING`, scope.Tenant(), x.ID, x.Topic, x.Question, x.SQL, x.Digest, x.Weight, x.EvidenceCount, x.Provenance, x.Created, x.Updated, x.Uncertainty, x.PositiveEvidence, x.NegativeEvidence, origin, schema)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			event, err := newID()
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,'nlq.example_changed',$4)`, scope.Tenant(), event, scope.Actor(), x.ID); err != nil {
				return err
			}
		}
		if err := scanExample(tx.QueryRow(ctx, `SELECT `+columns+` FROM chartworks.nlq_examples WHERE tenant_id=$1 AND topic_id=$2 AND digest=$3`, scope.Tenant(), x.Topic, x.Digest), &out); err != nil {
			return err
		}
		if exec.Hash(out.Origin) != exec.Hash(x.Origin) || exec.Hash(out.ParameterSchema) != exec.Hash(x.ParameterSchema) {
			return store.ErrConflict
		}
		if tag.RowsAffected() == 1 && old.ReviewedAt == nil {
			if _, err := tx.Exec(ctx, `INSERT INTO chartworks.nlq_example_contributions(tenant_id,example_id,feedback_id,actor_id,session_id,query_id) SELECT tenant_id,$3,feedback_id,actor_id,session_id,query_id FROM chartworks.nlq_example_contributions WHERE tenant_id=$1 AND example_id=$2`, scope.Tenant(), old.ID, out.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO chartworks.nlq_example_quarantine(tenant_id,example_id,reason) SELECT $1,$3,'legacy_lineage_unproved' WHERE EXISTS(SELECT 1 FROM chartworks.nlq_example_quarantine WHERE tenant_id=$1 AND example_id=$2) OR $4<>(SELECT count(*) FROM chartworks.nlq_example_contributions WHERE tenant_id=$1 AND example_id=$3)`, scope.Tenant(), old.ID, out.ID, out.EvidenceCount); err != nil {
				return err
			}
		}
		return nil
	})
	return
}
