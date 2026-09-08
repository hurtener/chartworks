package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ rulesets.Repository = (*DB)(nil)

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
		if err = tx.QueryRow(ctx, `SELECT revision FROM chartworks.topic_rule_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR UPDATE`, e.Tenant(), published.State.Topic).Scan(&current); err != nil {
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
		return tx.QueryRow(ctx, `SELECT v.version_id,v.topic_version,v.pack_digest FROM chartworks.topic_rule_publication_heads h JOIN chartworks.topic_rule_published_versions v ON(v.tenant_id,v.topic_id)=(h.tenant_id,h.topic_id) WHERE h.tenant_id=$1 AND h.topic_id=$2 AND v.version_id=CASE WHEN $3::text='' THEN h.active_version ELSE $3 END`, e.Tenant(), topic, version).Scan(&out.RuleVersion, &out.TopicVersion, &out.PackDigest)
	})
	return
}

func (d *DB) ReadPublishedRules(ctx context.Context, e identity.Envelope, topic, version string, access drafts.Access) (out rulesets.Published, err error) {
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
		if err := tx.QueryRow(ctx, `SELECT h.revision,COALESCE(h.active_version,''),v.version_id,v.digest,v.definition,v.published_at FROM chartworks.topic_rule_publication_heads h JOIN chartworks.topic_rule_published_versions v ON(v.tenant_id,v.topic_id)=(h.tenant_id,h.topic_id) WHERE h.tenant_id=$1 AND h.topic_id=$2 AND v.version_id=$3`, e.Tenant(), topic, version).Scan(&out.State.Revision, &active, &out.State.Version, &out.Digest, &raw, &out.PublishedAt); err != nil {
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
		if err := lockPublishedTopic(ctx, tx, e.Tenant(), published); err != nil {
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
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "rules.retired", published.State.Topic); err != nil {
			return err
		}
		out = rulesets.State{Topic: published.State.Topic, Revision: expected + 1, Version: *active, Retired: true}
		return nil
	})
	return
}
