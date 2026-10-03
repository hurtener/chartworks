-- Explicit semantic proposals retain protected origins; application only creates
-- a private draft. Publication remains the existing separately reviewed flow.
CREATE TABLE chartworks.topic_feedback_proposals (
 tenant_id text NOT NULL,
 proposal_id text NOT NULL CHECK(proposal_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 actor_id text NOT NULL,
 session_id text NOT NULL,
 topic_id text NOT NULL,
 feedback_id text NOT NULL,
 draft_revision bigint NOT NULL,
 query_revision bigint NOT NULL CHECK(query_revision>0),
 request_digest text NOT NULL CHECK(request_digest ~ '^[a-f0-9]{64}$'),
 proposal_digest text NOT NULL CHECK(proposal_digest ~ '^[a-f0-9]{64}$'),
 candidate_digest text NOT NULL CHECK(candidate_digest ~ '^[a-f0-9]{64}$'),
 proposal jsonb NOT NULL CHECK(jsonb_typeof(proposal)='object' AND octet_length(proposal::text)<=262144),
 candidate jsonb NOT NULL CHECK(jsonb_typeof(candidate)='object' AND octet_length(candidate::text)<=2097152),
 applied_revision bigint,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,proposal_id),
 FOREIGN KEY(tenant_id,topic_id,draft_revision) REFERENCES chartworks.topic_draft_versions(tenant_id,topic_id,revision),
 FOREIGN KEY(tenant_id,topic_id,applied_revision) REFERENCES chartworks.topic_draft_versions(tenant_id,topic_id,revision),
 CHECK(applied_revision IS NULL OR applied_revision=draft_revision+1)
);
CREATE FUNCTION chartworks.protect_topic_feedback_proposal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-'applied_revision') IS DISTINCT FROM (to_jsonb(OLD)-'applied_revision') OR OLD.applied_revision IS NOT NULL OR NEW.applied_revision IS NULL THEN
 RAISE EXCEPTION 'immutable topic proposal evidence' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER topic_feedback_proposal_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_feedback_proposals FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_feedback_proposal();

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action=''topic.feedback_proposed'')',previous);
END $$;


-- A content-free tombstone detaches separately authored proposals from report
-- retention. The proposal never stores historical SQL, notes, rows or literals.
CREATE TABLE chartworks.topic_feedback_origin_tombstones (
 tenant_id text NOT NULL,
 proposal_id text NOT NULL,
 reason text NOT NULL CHECK(reason='document_deleted'),
 erased_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,proposal_id),
 FOREIGN KEY(tenant_id,proposal_id) REFERENCES chartworks.topic_feedback_proposals(tenant_id,proposal_id)
);
CREATE TRIGGER topic_feedback_origin_tombstone_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_feedback_origin_tombstones FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
