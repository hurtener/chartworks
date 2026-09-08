CREATE TABLE chartworks.topic_draft_heads (
 tenant_id text NOT NULL REFERENCES chartworks.policies(tenant_id),
 topic_id text NOT NULL CHECK(topic_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 current_revision bigint NOT NULL CHECK(current_revision BETWEEN 1 AND 128),
 PRIMARY KEY(tenant_id,topic_id)
);
CREATE TABLE chartworks.topic_draft_versions (
 tenant_id text NOT NULL, topic_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 128),
 version_id text NOT NULL CHECK(version_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object' AND octet_length(manifest::text)<=2097152),
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 change_note text NOT NULL CHECK(octet_length(change_note) BETWEEN 1 AND 1024),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,topic_id,revision), UNIQUE(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id) REFERENCES chartworks.topic_draft_heads(tenant_id,topic_id)
);
ALTER TABLE chartworks.topic_draft_heads ADD FOREIGN KEY(tenant_id,topic_id,current_revision)
 REFERENCES chartworks.topic_draft_versions(tenant_id,topic_id,revision) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE chartworks.topic_draft_dependencies (
 tenant_id text NOT NULL, topic_id text NOT NULL, revision bigint NOT NULL,
 dataset_id text NOT NULL, source_id text NOT NULL, context_id text NOT NULL,
 source_revision bigint NOT NULL, profile_id text NOT NULL,
 profile_digest text NOT NULL CHECK(profile_digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(tenant_id,topic_id,revision,dataset_id),
 FOREIGN KEY(tenant_id,topic_id,revision) REFERENCES chartworks.topic_draft_versions(tenant_id,topic_id,revision),
 FOREIGN KEY(tenant_id,source_id,source_revision) REFERENCES chartworks.source_revisions(tenant_id,source_id,revision),
 FOREIGN KEY(tenant_id,profile_id) REFERENCES chartworks.profile_versions(tenant_id,profile_id)
);
CREATE FUNCTION chartworks.protect_topic_draft() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'immutable topic draft evidence' USING ERRCODE='55000';
END $$;
CREATE TRIGGER topic_draft_version_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_draft_versions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_draft_dependency_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_draft_dependencies FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN('topic.drafted','pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted',
 'request.accepted','request.resumed','request.cancelled','request.succeeded',
 'upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased',
 'profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased'
));
