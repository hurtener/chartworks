-- Governed block authoring only: no run cache, report scheduler, model client or
-- local identity policy. Every reference includes its tenant and exact revision.
CREATE TABLE chartworks.block_heads (
 tenant_id text NOT NULL CHECK(tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 block_id text NOT NULL CHECK(block_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 topic_id text NOT NULL,
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 4096),
 draft_revision bigint NOT NULL CHECK(draft_revision BETWEEN 1 AND 256),
 published_revision bigint,
 draft_state text NOT NULL CHECK(draft_state IN('draft','validated','published','rejected')),
 archived boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,block_id),
 FOREIGN KEY(tenant_id,topic_id) REFERENCES chartworks.topic_publication_heads(tenant_id,topic_id),
 CHECK(NOT archived OR published_revision IS NULL)
);
CREATE TABLE chartworks.block_revisions (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 256),
 revision_id text NOT NULL CHECK(revision_id ~ '^[a-f0-9]{32}$'),
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object' AND octet_length(definition::text)<=1048576),
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 execution_digest text NOT NULL CHECK(execution_digest ~ '^[a-f0-9]{64}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 provenance jsonb NOT NULL CHECK(jsonb_typeof(provenance)='object' AND octet_length(provenance::text)<=16384),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,block_id,revision),
 UNIQUE(tenant_id,block_id,revision_id),
 FOREIGN KEY(tenant_id,block_id) REFERENCES chartworks.block_heads(tenant_id,block_id)
);
ALTER TABLE chartworks.block_heads ADD CONSTRAINT block_draft_reference FOREIGN KEY(tenant_id,block_id,draft_revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE chartworks.block_revision_references (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 kind text NOT NULL,
 permission text NOT NULL,
 resource_id text NOT NULL CHECK(resource_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 PRIMARY KEY(tenant_id,block_id,revision,kind,permission,resource_id),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision),
 CHECK((kind,permission) IN(('topic','read'),('source','read'),('dataset','query'),('execution_context','use')))
);
CREATE TABLE chartworks.block_topic_pins (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 topic_id text NOT NULL,
 version_id text NOT NULL,
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(tenant_id,block_id,revision,topic_id),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision),
 FOREIGN KEY(tenant_id,topic_id,version_id) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id)
);
CREATE TABLE chartworks.block_source_pins (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 source_id text NOT NULL,
 source_revision bigint NOT NULL,
 context_id text NOT NULL,
 PRIMARY KEY(tenant_id,block_id,revision,source_id),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision),
 FOREIGN KEY(tenant_id,source_id,source_revision) REFERENCES chartworks.source_revisions(tenant_id,source_id,revision)
);
CREATE TABLE chartworks.block_validations (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 evidence_id text NOT NULL CHECK(evidence_id ~ '^[a-f0-9]{32}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 attempt_id text NOT NULL CHECK(attempt_id ~ '^[a-f0-9]{32}$'),
 record jsonb NOT NULL CHECK(jsonb_typeof(record)='object' AND octet_length(record::text)<=4194304),
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,block_id,revision,evidence_id),
 UNIQUE(tenant_id,block_id,evidence_id),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '7 days')
);
-- The common read journal has an independent retention policy. CommitBlock
-- verifies its successful immutable row under a shared lock before copying the
-- content-free receipt. A permanent FK would break the existing journal sweep.
CREATE INDEX block_validation_latest ON chartworks.block_validations(tenant_id,block_id,revision,created_at DESC,evidence_id);
CREATE TABLE chartworks.block_publications (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 evidence_id text NOT NULL,
 actor_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,block_id,revision),
 FOREIGN KEY(tenant_id,block_id,revision,evidence_id) REFERENCES chartworks.block_validations(tenant_id,block_id,revision,evidence_id)
);
ALTER TABLE chartworks.block_heads ADD CONSTRAINT block_publication_reference FOREIGN KEY(tenant_id,block_id,published_revision) REFERENCES chartworks.block_publications(tenant_id,block_id,revision) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE chartworks.block_attestations (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 attestation_id text NOT NULL CHECK(attestation_id ~ '^[a-f0-9]{32}$'),
 evidence_id text NOT NULL,
 attestation jsonb NOT NULL CHECK(jsonb_typeof(attestation)='object' AND octet_length(attestation::text)<=8192),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,block_id,revision,attestation_id),
 UNIQUE(tenant_id,block_id,attestation_id),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_publications(tenant_id,block_id,revision),
 FOREIGN KEY(tenant_id,block_id,revision,evidence_id) REFERENCES chartworks.block_validations(tenant_id,block_id,revision,evidence_id)
);
CREATE TABLE chartworks.block_withdrawals (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 attestation_id text NOT NULL,
 withdrawal jsonb NOT NULL CHECK(jsonb_typeof(withdrawal)='object' AND octet_length(withdrawal::text)<=8192),
 PRIMARY KEY(tenant_id,block_id,revision,attestation_id),
 FOREIGN KEY(tenant_id,block_id,revision,attestation_id) REFERENCES chartworks.block_attestations(tenant_id,block_id,revision,attestation_id)
);
CREATE TABLE chartworks.block_events (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 version bigint NOT NULL,
 revision bigint NOT NULL,
 kind text NOT NULL CHECK(kind IN('create','capture','edit','restore','rename','parameterize','validate','publish','certify','withdraw','reject','archive','health')),
 actor_id text NOT NULL,
 note text NOT NULL CHECK(octet_length(note)<=2048),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,block_id,version),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision)
);
CREATE TABLE chartworks.block_health (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 observation jsonb NOT NULL CHECK(jsonb_typeof(observation)='object' AND octet_length(observation::text)<=8192),
 PRIMARY KEY(tenant_id,block_id,revision),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision)
);
CREATE FUNCTION chartworks.block_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'block history is immutable' USING ERRCODE='23514';
END;
$$;
CREATE TRIGGER block_revision_immutable BEFORE UPDATE OR DELETE ON chartworks.block_revisions FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
CREATE TRIGGER block_refs_immutable BEFORE UPDATE OR DELETE ON chartworks.block_revision_references FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
CREATE TRIGGER block_topic_pins_immutable BEFORE UPDATE OR DELETE ON chartworks.block_topic_pins FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
CREATE TRIGGER block_source_pins_immutable BEFORE UPDATE OR DELETE ON chartworks.block_source_pins FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
CREATE TRIGGER block_validation_immutable BEFORE UPDATE OR DELETE ON chartworks.block_validations FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
CREATE TRIGGER block_publication_immutable BEFORE UPDATE OR DELETE ON chartworks.block_publications FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
CREATE TRIGGER block_attestation_immutable BEFORE UPDATE OR DELETE ON chartworks.block_attestations FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
CREATE TRIGGER block_withdrawal_immutable BEFORE UPDATE OR DELETE ON chartworks.block_withdrawals FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
CREATE TRIGGER block_event_immutable BEFORE UPDATE OR DELETE ON chartworks.block_events FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN('rules.drafted','rules.reviewed','rules.published','rules.retired','topic.reviewed','topic.published','topic.rolled_back','topic.archived','topic.drafted','topic.health_rechecked','pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired','facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated','read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted','request.accepted','request.resumed','request.cancelled','request.succeeded','upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased','profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased','nlq.session_created','nlq.query_planned','nlq.query_executed','nlq.feedback_recorded','nlq.example_changed','byo.context_created','byo.step_accepted','byo.step_finished',
 'block.created','block.captured','block.edited','block.restored','block.renamed','block.parameterized','block.validated','block.published','block.certified','block.withdrawn','block.rejected','block.archived','block.health_checked'));
