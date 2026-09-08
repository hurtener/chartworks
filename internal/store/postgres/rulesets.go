package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ rulesets.Repository = (*DB)(nil)
var _ rulesets.EvidenceRepository = (*DB)(nil)

func requireRuleAccess(e identity.Envelope, topic string, access drafts.Access) error {
	return drafts.Require(e, topic, access)
}

func lockPublishedTopic(ctx context.Context, tx pgx.Tx, tenant string, expected topics.Published) error {
	var revision int64
	var version, digest string
	var archived bool
	err := tx.QueryRow(ctx, `SELECT h.revision,h.active_version,h.archived,v.digest FROM chartworks.topic_publication_heads h JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version) WHERE h.tenant_id=$1 AND h.topic_id=$2 FOR SHARE OF h`, tenant, expected.State.Topic).Scan(&revision, &version, &archived, &digest)
	if err != nil {
		return err
	}
	if revision != expected.State.Revision || version != expected.State.Version || archived || digest != expected.Digest {
		return store.ErrConflict
	}
	return nil
}

func lockRetainedTopic(ctx context.Context, tx pgx.Tx, tenant string, expected topics.Published) error {
	var digest string
	err := tx.QueryRow(ctx, `SELECT v.digest FROM chartworks.topic_publication_heads h JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id)=(h.tenant_id,h.topic_id) WHERE h.tenant_id=$1 AND h.topic_id=$2 AND v.version_id=$3 FOR SHARE OF h`, tenant, expected.State.Topic, expected.State.Version).Scan(&digest)
	if err != nil {
		return err
	}
	if digest != expected.Digest {
		return store.ErrConflict
	}
	return nil
}

// SaveRuleDraft persists a rule draft after rechecking the published topic pin.
func (d *DB) SaveRuleDraft(ctx context.Context, e identity.Envelope, published topics.Published, model semantics.RuleModel, expected int64, change string) (out rulesets.Draft, err error) {
	definition := model.Definition()
	if expected < 0 || expected >= 1<<62 || definition.Topic != published.State.Topic || definition.TopicVersion != published.State.Version || definition.PackDigest != published.Digest {
		return out, store.ErrInvalid
	}
	if err = requireRuleAccess(e, definition.Topic, drafts.Write); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	raw, _ := json.Marshal(definition)
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockPublishedTopic(ctx, tx, e.Tenant(), published); err != nil {
			return err
		}
		next := expected + 1
		if expected == 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO chartworks.topic_rule_draft_heads(tenant_id,topic_id,actor_id,session_id,current_revision) VALUES($1,$2,$3,$4,1)`, e.Tenant(), definition.Topic, e.User(), e.Session()); err != nil {
				return err
			}
		} else {
			command, err := tx.Exec(ctx, `UPDATE chartworks.topic_rule_draft_heads SET current_revision=$5 WHERE tenant_id=$1 AND topic_id=$2 AND actor_id=$3 AND session_id=$4 AND current_revision=$6`, e.Tenant(), definition.Topic, e.User(), e.Session(), next, expected)
			if err != nil {
				return err
			}
			if command.RowsAffected() != 1 {
				return store.ErrConflict
			}
		}
		out = rulesets.Draft{Topic: definition.Topic, Revision: next, Digest: model.Digest(), Definition: definition, Change: change}
		if err := tx.QueryRow(ctx, `INSERT INTO chartworks.topic_rule_draft_versions(tenant_id,topic_id,actor_id,session_id,revision,ruleset_id,version_id,topic_version,pack_digest,digest,definition,change_note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING created_at`, e.Tenant(), definition.Topic, e.User(), e.Session(), next, definition.ID, definition.Version, definition.TopicVersion, definition.PackDigest, model.Digest(), raw, change).Scan(&out.CreatedAt); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		return auditJob(ctx, tx, scope, "rules.drafted", definition.Topic)
	})
	return
}

// ReviewRuleDraft persists a review decision for a version-pinned rule draft.
func (d *DB) ReviewRuleDraft(ctx context.Context, e identity.Envelope, published topics.Published, topic string, in rulesets.ReviewRequest) (out rulesets.Review, err error) {
	if err = requireRuleAccess(e, topic, drafts.Review); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockPublishedTopic(ctx, tx, e.Tenant(), published); err != nil {
			return err
		}
		var digest, topicVersion, packDigest string
		if err := tx.QueryRow(ctx, `SELECT digest,topic_version,pack_digest FROM chartworks.topic_rule_draft_versions WHERE tenant_id=$1 AND topic_id=$2 AND actor_id=$3 AND session_id=$4 AND revision=$5`, e.Tenant(), topic, e.User(), e.Session(), in.DraftRevision).Scan(&digest, &topicVersion, &packDigest); err != nil {
			return err
		}
		if digest != in.Digest || topicVersion != published.State.Version || packDigest != published.Digest {
			return store.ErrConflict
		}
		reviewID, err := newID()
		if err != nil {
			return err
		}
		out = rulesets.Review{ID: reviewID, Topic: topic, DraftRevision: in.DraftRevision, Digest: in.Digest, Decision: in.Decision, Note: in.Note}
		if err = tx.QueryRow(ctx, `INSERT INTO chartworks.topic_rule_reviews(tenant_id,review_id,topic_id,actor_id,session_id,draft_revision,digest,decision,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING created_at`, e.Tenant(), reviewID, topic, e.User(), e.Session(), in.DraftRevision, in.Digest, in.Decision, in.Note).Scan(&out.CreatedAt); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		return auditJob(ctx, tx, scope, "rules.reviewed", topic)
	})
	return
}

// PublishRules atomically publishes an approved rule version and advances its head.
func (d *DB) PublishRules(ctx context.Context, e identity.Envelope, published topics.Published, reviewID string, expected int64) (out rulesets.Published, err error) {
	if err = requireRuleAccess(e, published.State.Topic, drafts.Publish); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockPublishedTopic(ctx, tx, e.Tenant(), published); err != nil {
			return err
		}
		var review rulesets.Review
		var raw []byte
		var rulesetID, version, topicVersion, packDigest, draftDigest string
		err := tx.QueryRow(ctx, `SELECT r.review_id,r.topic_id,r.draft_revision,r.digest,r.decision,r.note,r.created_at,d.ruleset_id,d.version_id,d.topic_version,d.pack_digest,d.digest,d.definition FROM chartworks.topic_rule_reviews r JOIN chartworks.topic_rule_draft_versions d ON(d.tenant_id,d.topic_id,d.actor_id,d.session_id,d.revision)=(r.tenant_id,r.topic_id,r.actor_id,r.session_id,r.draft_revision) WHERE r.tenant_id=$1 AND r.topic_id=$2 AND r.review_id=$3 AND r.actor_id=$4 AND r.session_id=$5`, e.Tenant(), published.State.Topic, reviewID, e.User(), e.Session()).Scan(&review.ID, &review.Topic, &review.DraftRevision, &review.Digest, &review.Decision, &review.Note, &review.CreatedAt, &rulesetID, &version, &topicVersion, &packDigest, &draftDigest, &raw)
		if err != nil {
			return err
		}
		if review.Decision != "approve" || review.Digest != draftDigest || topicVersion != published.State.Version || packDigest != published.Digest {
			return store.ErrConflict
		}
		var definition semantics.RuleSetDefinition
		if json.Unmarshal(raw, &definition) != nil || definition.ID != rulesetID || definition.Version != version || definition.Topic != published.State.Topic || definition.TopicVersion != topicVersion || definition.PackDigest != packDigest {
			return store.ErrInvalid
		}
		if expected == 0 {
			if _, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_rule_publication_heads(tenant_id,topic_id,revision) VALUES($1,$2,0)`, e.Tenant(), published.State.Topic); err != nil {
				return err
			}
		}
		var current int64
		var oldVersion *string
		if err = tx.QueryRow(ctx, `SELECT revision,active_version FROM chartworks.topic_rule_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR UPDATE`, e.Tenant(), published.State.Topic).Scan(&current, &oldVersion); err != nil {
			return err
		}
		if current != expected {
			return store.ErrConflict
		}
		if err = tx.QueryRow(ctx, `INSERT INTO chartworks.topic_rule_published_versions(tenant_id,topic_id,version_id,ruleset_id,topic_version,pack_digest,digest,definition,review_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING published_at`, e.Tenant(), published.State.Topic, version, rulesetID, topicVersion, packDigest, draftDigest, raw, review.ID).Scan(&out.PublishedAt); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.topic_rule_publication_heads SET revision=$3,active_version=$4 WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), published.State.Topic, expected+1, version); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_rule_publication_events(tenant_id,topic_id,revision,version_id,kind,actor_id,session_id,note) VALUES($1,$2,$3,$4,'publish',$5,$6,$7)`, e.Tenant(), published.State.Topic, expected+1, version, e.User(), e.Session(), review.Note); err != nil {
			return err
		}
		invalidationID, err := newID()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_rule_evidence_invalidations(tenant_id,invalidation_id,topic_id,revision,kind,old_rule_version,new_rule_version,topic_version,pack_digest) VALUES($1,$2,$3,$4,'publish',$5,$6,$7,$8)`, e.Tenant(), invalidationID, published.State.Topic, expected+1, oldVersion, version, published.State.Version, published.Digest); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err = auditJob(ctx, tx, scope, "rules.published", published.State.Topic); err != nil {
			return err
		}
		out.State = rulesets.State{Topic: published.State.Topic, Revision: expected + 1, Version: version, Active: true}
		out.Definition, out.Digest = definition, draftDigest
		return nil
	})
	return
}

// RuleVersionPin reads the exact rule/topic pin for a current or retained version.
func (d *DB) RuleVersionPin(ctx context.Context, e identity.Envelope, topic, version string, access drafts.Access) (out rulesets.Pin, err error) {
	if err = requireRuleAccess(e, topic, access); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		if version == "" {
			// Distinguish a topic with no ruleset head from an existing head
			// whose active pointer was retired. The router treats the former as
			// an optional no-rule topic, while the latter is a lifecycle conflict.
			var active *string
			if err := tx.QueryRow(ctx, `SELECT active_version FROM chartworks.topic_rule_publication_heads WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), topic).Scan(&active); err != nil {
				return err
			}
			if active == nil || *active == "" {
				return store.ErrConflict
			}
			return tx.QueryRow(ctx, `SELECT version_id,topic_version,pack_digest FROM chartworks.topic_rule_published_versions WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3`, e.Tenant(), topic, *active).Scan(&out.RuleVersion, &out.TopicVersion, &out.PackDigest)
		}
		return tx.QueryRow(ctx, `SELECT v.version_id,v.topic_version,v.pack_digest FROM chartworks.topic_rule_publication_heads h JOIN chartworks.topic_rule_published_versions v ON(v.tenant_id,v.topic_id)=(h.tenant_id,h.topic_id) WHERE h.tenant_id=$1 AND h.topic_id=$2 AND v.version_id=CASE WHEN $3::text='' THEN h.active_version ELSE $3 END`, e.Tenant(), topic, version).Scan(&out.RuleVersion, &out.TopicVersion, &out.PackDigest)
	})
	return
}

// ReadPublishedRules reads a current or explicitly retained immutable rule version.
func (d *DB) ReadPublishedRules(ctx context.Context, e identity.Envelope, topic, version string, access drafts.Access, current bool) (out rulesets.Published, err error) {
	if err = requireRuleAccess(e, topic, access); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		var active string
		err := tx.QueryRow(ctx, `SELECT h.revision,COALESCE(h.active_version,''),v.version_id,v.digest,v.definition,v.published_at FROM chartworks.topic_rule_publication_heads h JOIN chartworks.topic_rule_published_versions v ON(v.tenant_id,v.topic_id)=(h.tenant_id,h.topic_id) JOIN chartworks.topic_publication_heads th ON(th.tenant_id,th.topic_id)=(h.tenant_id,h.topic_id) WHERE h.tenant_id=$1 AND h.topic_id=$2 AND v.version_id=$3 AND (NOT $4 OR (h.active_version=v.version_id AND NOT th.archived AND th.active_version=v.topic_version))`, e.Tenant(), topic, version, current).Scan(&out.State.Revision, &active, &out.State.Version, &out.Digest, &raw, &out.PublishedAt)
		if err != nil {
			if current && errors.Is(err, pgx.ErrNoRows) {
				return store.ErrConflict
			}
			return err
		}
		if json.Unmarshal(raw, &out.Definition) != nil || out.Definition.Topic != topic || out.Definition.Version != out.State.Version || out.Digest == "" {
			return store.ErrInvalid
		}
		out.State.Topic = topic
		out.State.Active = active == version
		out.State.Retired = !out.State.Active
		return nil
	})
	return
}

// RetireRules clears the active rule pointer after rechecking the retained topic.
func (d *DB) RetireRules(ctx context.Context, e identity.Envelope, published topics.Published, note string, expected int64) (out rulesets.State, err error) {
	if err = requireRuleAccess(e, published.State.Topic, drafts.Publish); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockRetainedTopic(ctx, tx, e.Tenant(), published); err != nil {
			return err
		}
		var revision int64
		var active *string
		if err := tx.QueryRow(ctx, `SELECT revision,active_version FROM chartworks.topic_rule_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR UPDATE`, e.Tenant(), published.State.Topic).Scan(&revision, &active); err != nil {
			return err
		}
		if revision != expected || active == nil {
			return store.ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE chartworks.topic_rule_publication_heads SET revision=$3,active_version=NULL WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), published.State.Topic, expected+1); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.topic_rule_publication_events(tenant_id,topic_id,revision,version_id,kind,actor_id,session_id,note) VALUES($1,$2,$3,$4,'retire',$5,$6,$7)`, e.Tenant(), published.State.Topic, expected+1, *active, e.User(), e.Session(), note); err != nil {
			return err
		}
		invalidationID, err := newID()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_rule_evidence_invalidations(tenant_id,invalidation_id,topic_id,revision,kind,old_rule_version,new_rule_version,topic_version,pack_digest) VALUES($1,$2,$3,$4,'retire',$5,NULL,$6,$7)`, e.Tenant(), invalidationID, published.State.Topic, expected+1, *active, published.State.Version, published.Digest); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "rules.retired", published.State.Topic); err != nil {
			return err
		}
		out = rulesets.State{Topic: published.State.Topic, Revision: expected + 1, Version: *active, Retired: true}
		return nil
	})
	return
}

func (d *DB) RecordComparison(ctx context.Context, e identity.Envelope, comparison rulesets.Comparison) (rulesets.Comparison, error) {
	if !identity.Identifier(comparison.ID) || (comparison.Mode != "replay" && comparison.Mode != "shadow") || !identity.Identifier(comparison.Topic) || len(comparison.References) < 1 || len(comparison.References) > 256 || !identity.Identifier(comparison.Baseline.RuleVersion) || !identity.Identifier(comparison.Baseline.TopicVersion) {
		return rulesets.Comparison{}, store.ErrInvalid
	}
	if comparison.Mode == "replay" && comparison.Candidate != nil || comparison.Mode == "shadow" && (comparison.Candidate == nil || !identity.Identifier(comparison.Candidate.RuleVersion)) {
		return rulesets.Comparison{}, store.ErrInvalid
	}
	if err := requireRuleAccess(e, comparison.Topic, drafts.Read); err != nil {
		return rulesets.Comparison{}, err
	}
	references, err := json.Marshal(comparison.References)
	if err != nil {
		return rulesets.Comparison{}, store.ErrInvalid
	}
	baseline, err := json.Marshal(comparison.Baseline.Result)
	if err != nil {
		return rulesets.Comparison{}, store.ErrInvalid
	}
	var candidateVersion, candidateResult any
	if comparison.Candidate != nil {
		candidateVersion = comparison.Candidate.RuleVersion
		candidateResult, err = json.Marshal(comparison.Candidate.Result)
		if err != nil {
			return rulesets.Comparison{}, store.ErrInvalid
		}
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return rulesets.Comparison{}, err
	}
	defer cancel()
	created := comparison.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO chartworks.topic_rule_comparison_evidence(tenant_id,comparison_id,actor_id,session_id,topic_id,mode,topic_version,pack_digest,references_json,baseline_rule_version,baseline_result,candidate_rule_version,candidate_result,changed,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11::jsonb,$12,$13::jsonb,$14,$15)`, e.Tenant(), comparison.ID, e.User(), e.Session(), comparison.Topic, comparison.Mode, comparison.Baseline.TopicVersion, comparison.Baseline.PackDigest, references, comparison.Baseline.RuleVersion, baseline, candidateVersion, candidateResult, comparison.Changed, created)
		return err
	})
	if err != nil {
		return rulesets.Comparison{}, safe(err)
	}
	comparison.CreatedAt = created
	return comparison, nil
}

func (d *DB) ReadInvalidations(ctx context.Context, e identity.Envelope, topic string, after int64, limit int) (out []rulesets.Invalidation, err error) {
	if !identity.Identifier(topic) || after < 0 || limit < 1 || limit > 128 {
		return nil, store.ErrInvalid
	}
	if err = requireRuleAccess(e, topic, drafts.Read); err != nil {
		return nil, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return nil, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT invalidation_id,topic_id,revision,kind,COALESCE(old_rule_version,''),COALESCE(new_rule_version,''),topic_version,pack_digest,created_at FROM chartworks.topic_rule_evidence_invalidations WHERE tenant_id=$1 AND topic_id=$2 AND revision>$3 ORDER BY revision,invalidation_id LIMIT $4`, e.Tenant(), topic, after, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item rulesets.Invalidation
			if err = rows.Scan(&item.ID, &item.Topic, &item.Revision, &item.Kind, &item.OldRuleVersion, &item.NewRuleVersion, &item.TopicVersion, &item.PackDigest, &item.CreatedAt); err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, safe(err)
	}
	return out, nil
}
