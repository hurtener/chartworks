-- Reviewed L2 domain metadata. Identity and grants remain signed Pengui input;
-- managed execution continues to use the existing operation and pipeline ledgers.
CREATE TABLE chartworks.engineering_proposal_heads (
 tenant_id text NOT NULL CHECK(tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 proposal_id text NOT NULL CHECK(proposal_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 pipeline_id text NOT NULL CHECK(pipeline_id ~ '^[A-Za-z0-9_.:-]{1,48}$'),
 origin_author text NOT NULL CHECK(origin_author ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 4096),
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 256),
 state text NOT NULL CHECK(state IN('draft','approved','rejected','applying','applied','compensated')),
 review_version bigint,
 operation_id text,
 apply_actor text CHECK(apply_actor ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 apply_session text CHECK(apply_session ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 applied_at timestamptz,
 PRIMARY KEY(tenant_id,proposal_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id),
 CHECK((apply_actor IS NULL)=(apply_session IS NULL)),
 CHECK(state NOT IN('approved','applying','applied','compensated') OR review_version IS NOT NULL),
 CHECK(state NOT IN('applied','compensated') OR (operation_id IS NOT NULL AND applied_at IS NOT NULL))
);
CREATE TABLE chartworks.engineering_proposal_versions (
 tenant_id text NOT NULL, proposal_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 256),
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 source_id text NOT NULL, source_revision bigint NOT NULL, context_id text NOT NULL,
 author_id text NOT NULL CHECK(author_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 material bytea NOT NULL CHECK(octet_length(material) BETWEEN 1 AND 1048576),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,proposal_id,revision),
 UNIQUE(tenant_id,proposal_id,revision,digest),
 FOREIGN KEY(tenant_id,proposal_id) REFERENCES chartworks.engineering_proposal_heads(tenant_id,proposal_id),
 FOREIGN KEY(tenant_id,source_id,source_revision) REFERENCES chartworks.source_revisions(tenant_id,source_id,revision)
);
ALTER TABLE chartworks.engineering_proposal_heads ADD CONSTRAINT engineering_proposal_current FOREIGN KEY(tenant_id,proposal_id,revision) REFERENCES chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER engineering_proposal_material_immutable BEFORE UPDATE ON chartworks.engineering_proposal_versions FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();
CREATE TABLE chartworks.engineering_proposal_references (
 tenant_id text NOT NULL, proposal_id text NOT NULL, revision bigint NOT NULL,
 kind text NOT NULL, permission text NOT NULL,
 resource_id text NOT NULL CHECK(resource_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 PRIMARY KEY(tenant_id,proposal_id,revision,kind,permission,resource_id),
 FOREIGN KEY(tenant_id,proposal_id,revision) REFERENCES chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision),
 CHECK((kind,permission) IN(('source','query'),('dataset','query'),('execution_context','use')))
);
CREATE TRIGGER engineering_proposal_references_immutable BEFORE UPDATE ON chartworks.engineering_proposal_references FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();
CREATE TABLE chartworks.engineering_proposal_reviews (
 tenant_id text NOT NULL, proposal_id text NOT NULL,
 version bigint NOT NULL, revision bigint NOT NULL,
 digest text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL,
 decision text NOT NULL CHECK(decision IN('approve','reject')),
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 2048),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,proposal_id,version),
 FOREIGN KEY(tenant_id,proposal_id,revision,digest) REFERENCES chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision,digest)
);
ALTER TABLE chartworks.engineering_proposal_heads ADD CONSTRAINT engineering_proposal_review FOREIGN KEY(tenant_id,proposal_id,review_version) REFERENCES chartworks.engineering_proposal_reviews(tenant_id,proposal_id,version) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER engineering_proposal_review_immutable BEFORE UPDATE ON chartworks.engineering_proposal_reviews FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();

-- This uniqueness proves which proposal created a pipeline revision. Matching a
-- coincidentally identical draft is not ownership and cannot justify compensation.
CREATE TABLE chartworks.engineering_proposal_pipeline_effects (
 tenant_id text NOT NULL, proposal_id text NOT NULL, revision bigint NOT NULL,
 pipeline_id text NOT NULL, pipeline_version bigint NOT NULL, pipeline_digest text NOT NULL,
 PRIMARY KEY(tenant_id,proposal_id,revision),
 UNIQUE(tenant_id,pipeline_id,pipeline_version),
 FOREIGN KEY(tenant_id,proposal_id,revision) REFERENCES chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision),
 FOREIGN KEY(tenant_id,pipeline_id,pipeline_version) REFERENCES chartworks.pipeline_versions(tenant_id,pipeline_id,version),
 CHECK(pipeline_digest ~ '^[a-f0-9]{64}$')
);
CREATE TRIGGER engineering_proposal_pipeline_effect_immutable BEFORE UPDATE ON chartworks.engineering_proposal_pipeline_effects FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();
CREATE TABLE chartworks.engineering_proposal_effects (
 tenant_id text NOT NULL, proposal_id text NOT NULL, revision bigint NOT NULL,
 observed_order integer NOT NULL CHECK(observed_order BETWEEN 0 AND 4),
 kind text NOT NULL CHECK(kind IN('pipeline_draft','pipeline_publication','pipeline_run','managed_step','compensation')),
 target_id text NOT NULL CHECK(target_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=16384),
 PRIMARY KEY(tenant_id,proposal_id,revision,kind,target_id),
 FOREIGN KEY(tenant_id,proposal_id,revision) REFERENCES chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision)
);
CREATE TABLE chartworks.engineering_proposal_events (
 tenant_id text NOT NULL, proposal_id text NOT NULL, version bigint NOT NULL,
 revision bigint NOT NULL, action text NOT NULL,
 actor_id text NOT NULL, session_id text NOT NULL,
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=131072),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,proposal_id,version),
 FOREIGN KEY(tenant_id,proposal_id,revision) REFERENCES chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision)
);
CREATE TRIGGER engineering_proposal_event_immutable BEFORE UPDATE ON chartworks.engineering_proposal_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();

CREATE TABLE chartworks.engineering_amendments (
 tenant_id text NOT NULL, amendment_id text NOT NULL CHECK(amendment_id ~ '^[a-f0-9]{32}$'),
 proposal_id text NOT NULL, revision bigint NOT NULL,
 kind text NOT NULL CHECK(kind IN('schema_changed','freshness_expired','quality_failed')),
 evidence_digest text NOT NULL CHECK(evidence_digest ~ '^[a-f0-9]{64}$'),
 observation jsonb NOT NULL CHECK(jsonb_typeof(observation)='object' AND octet_length(observation::text)<=65536),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,amendment_id),
 UNIQUE(tenant_id,proposal_id,revision,evidence_digest),
 FOREIGN KEY(tenant_id,proposal_id,revision) REFERENCES chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision)
);
CREATE TRIGGER engineering_amendment_evidence_immutable BEFORE UPDATE ON chartworks.engineering_amendments FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();

ALTER TABLE chartworks.pipeline_heads ADD COLUMN retired boolean NOT NULL DEFAULT false;

-- Extend the preceding exact allowlist, preserving its complete predicate.
DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_constraintdef(oid) INTO previous FROM pg_constraint WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 IF previous IS NULL OR left(previous,7)<>'CHECK (' THEN RAISE EXCEPTION 'missing audit action constraint'; END IF;
 previous:=substring(previous from 8 for length(previous)-8);
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE 'ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK (('||previous||') OR action IN(''engineering.proposal_created'',''engineering.proposal_edited'',''engineering.proposal_reviewed'',''engineering.proposal_effect'',''engineering.proposal_applied'',''engineering.proposal_compensated'',''engineering.amendment_created''))';
END $$;
