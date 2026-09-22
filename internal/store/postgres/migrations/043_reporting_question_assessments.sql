-- Bounded duplicate-assessment evidence. Candidate text is retained only inside
-- protected evidence; no SQL, result values, prompts or credentials are stored.
CREATE TABLE chartworks.block_question_assessments (
 tenant_id text NOT NULL CHECK(tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 assessment_id text NOT NULL CHECK(assessment_id ~ '^[a-f0-9]{32}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 request_digest text NOT NULL CHECK(request_digest ~ '^[a-f0-9]{64}$'),
 candidate_scope_digest text NOT NULL CHECK(candidate_scope_digest ~ '^[a-f0-9]{64}$'),
 evidence_digest text NOT NULL CHECK(evidence_digest ~ '^[a-f0-9]{64}$'),
 record jsonb NOT NULL CHECK(jsonb_typeof(record)='object' AND octet_length(record::text)<=262144),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,assessment_id)
);
CREATE INDEX block_question_assessment_actor ON chartworks.block_question_assessments(tenant_id,actor_id,created_at DESC);
CREATE TRIGGER block_question_assessment_immutable BEFORE UPDATE OR DELETE ON chartworks.block_question_assessments FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
