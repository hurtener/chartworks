CREATE TABLE chartworks.topic_health (
 tenant_id text NOT NULL, topic_id text NOT NULL,
 publication_revision bigint NOT NULL CHECK(publication_revision>0 AND publication_revision<4611686018427387904),
 version_id text NOT NULL, healthy boolean NOT NULL,
 issues jsonb NOT NULL CHECK(jsonb_typeof(issues)='array' AND octet_length(issues::text)<=262144),
 actor_id text NOT NULL, session_id text NOT NULL, observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,topic_id),
 FOREIGN KEY(tenant_id,topic_id) REFERENCES chartworks.topic_publication_heads(tenant_id,topic_id),
 FOREIGN KEY(tenant_id,topic_id,version_id) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id)
);

CREATE TABLE chartworks.topic_generation_checkpoints (
 tenant_id text NOT NULL, topic_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL,
 draft_revision bigint NOT NULL, cursor integer NOT NULL CHECK(cursor>0 AND cursor<=8192),
 complete boolean NOT NULL, receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=65536),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,topic_id,actor_id,session_id,draft_revision),
 FOREIGN KEY(tenant_id,topic_id,draft_revision)
  REFERENCES chartworks.topic_draft_versions(tenant_id,topic_id,revision)
);

CREATE TRIGGER topic_generation_checkpoint_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_generation_checkpoints FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN('topic.reviewed','topic.published','topic.rolled_back','topic.archived','topic.drafted','topic.health_rechecked','pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'rules.drafted','rules.reviewed','rules.published','rules.retired',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted',
 'request.accepted','request.resumed','request.cancelled','request.succeeded',
 'upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased',
 'profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased'));
