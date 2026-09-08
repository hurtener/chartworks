CREATE TABLE chartworks.topic_rule_draft_heads (
 tenant_id text NOT NULL, topic_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL,
 current_revision bigint NOT NULL CHECK(current_revision>0),
 PRIMARY KEY(tenant_id,topic_id,actor_id,session_id),
 FOREIGN KEY(tenant_id,topic_id) REFERENCES chartworks.topic_publication_heads(tenant_id,topic_id)
);
CREATE TABLE chartworks.topic_rule_draft_versions (
 tenant_id text NOT NULL, topic_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision>0), ruleset_id text NOT NULL, version_id text NOT NULL,
 topic_version text NOT NULL, pack_digest text NOT NULL CHECK(pack_digest ~ '^[0-9a-f]{64}$'),
 digest text NOT NULL CHECK(digest ~ '^[0-9a-f]{64}$'), definition jsonb NOT NULL,
 change_note text NOT NULL CHECK(octet_length(change_note) BETWEEN 1 AND 1024),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,topic_id,actor_id,session_id,revision),
 FOREIGN KEY(tenant_id,topic_id,actor_id,session_id) REFERENCES chartworks.topic_rule_draft_heads(tenant_id,topic_id,actor_id,session_id),
 FOREIGN KEY(tenant_id,topic_id,topic_version) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 CHECK(jsonb_typeof(definition)='object' AND octet_length(definition::text)<=1048576)
);
CREATE TABLE chartworks.topic_rule_reviews (
 tenant_id text NOT NULL, review_id text NOT NULL, topic_id text NOT NULL,
 actor_id text NOT NULL, session_id text NOT NULL, draft_revision bigint NOT NULL,
 digest text NOT NULL CHECK(digest ~ '^[0-9a-f]{64}$'),
 decision text NOT NULL CHECK(decision IN('approve','reject')),
 note text NOT NULL CHECK(octet_length(note) BETWEEN 1 AND 1024),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,review_id),
 FOREIGN KEY(tenant_id,topic_id,actor_id,session_id,draft_revision) REFERENCES chartworks.topic_rule_draft_versions(tenant_id,topic_id,actor_id,session_id,revision)
);
CREATE TABLE chartworks.topic_rule_publication_heads (
 tenant_id text NOT NULL, topic_id text NOT NULL, revision bigint NOT NULL DEFAULT 0,
 active_version text,
 PRIMARY KEY(tenant_id,topic_id),
 FOREIGN KEY(tenant_id,topic_id) REFERENCES chartworks.topic_publication_heads(tenant_id,topic_id)
);
CREATE TABLE chartworks.topic_rule_published_versions (
 tenant_id text NOT NULL, topic_id text NOT NULL, version_id text NOT NULL,
 ruleset_id text NOT NULL, topic_version text NOT NULL,
 pack_digest text NOT NULL CHECK(pack_digest ~ '^[0-9a-f]{64}$'),
 digest text NOT NULL CHECK(digest ~ '^[0-9a-f]{64}$'), definition jsonb NOT NULL,
 review_id text NOT NULL, published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id) REFERENCES chartworks.topic_rule_publication_heads(tenant_id,topic_id),
 FOREIGN KEY(tenant_id,topic_id,topic_version) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,review_id) REFERENCES chartworks.topic_rule_reviews(tenant_id,review_id),
 CHECK(jsonb_typeof(definition)='object' AND octet_length(definition::text)<=1048576)
);
ALTER TABLE chartworks.topic_rule_publication_heads ADD CONSTRAINT topic_rule_active_version_fk
 FOREIGN KEY(tenant_id,topic_id,active_version) REFERENCES chartworks.topic_rule_published_versions(tenant_id,topic_id,version_id)
 DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE chartworks.topic_rule_publication_events (
 tenant_id text NOT NULL, topic_id text NOT NULL, revision bigint NOT NULL,
 version_id text NOT NULL, kind text NOT NULL CHECK(kind IN('publish','retire')),
 actor_id text NOT NULL, session_id text NOT NULL,
 note text NOT NULL CHECK(octet_length(note) BETWEEN 1 AND 1024),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,topic_id,revision),
 FOREIGN KEY(tenant_id,topic_id,version_id) REFERENCES chartworks.topic_rule_published_versions(tenant_id,topic_id,version_id)
);
CREATE TRIGGER topic_rule_draft_version_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_rule_draft_versions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_rule_review_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_rule_reviews FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_rule_published_version_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_rule_published_versions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_rule_publication_event_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_rule_publication_events FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN('rules.drafted','rules.reviewed','rules.published','rules.retired','topic.reviewed','topic.published','topic.rolled_back','topic.archived','topic.drafted','pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted',
 'request.accepted','request.resumed','request.cancelled','request.succeeded',
 'upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased',
 'profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased'
));
