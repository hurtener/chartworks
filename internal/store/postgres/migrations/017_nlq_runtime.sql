-- Phase 18 keeps question state and learning evidence in Chartworks metadata.
-- SQL/corrections/results are protected domain metadata and are never returned
-- by a repository projection without the separate inspection authority.
CREATE TABLE chartworks.nlq_sessions (
 tenant_id text NOT NULL,
 actor_id text NOT NULL,
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 context_id text NOT NULL CHECK(context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 topics jsonb NOT NULL CHECK(jsonb_typeof(topics)='array' AND jsonb_array_length(topics) BETWEEN 1 AND 4 AND octet_length(topics::text)<=4096),
 locale text NOT NULL CHECK(locale IN('en','es')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,actor_id,session_id)
);

CREATE TABLE chartworks.nlq_queries (
 tenant_id text NOT NULL,
 actor_id text NOT NULL,
 session_id text NOT NULL,
 query_id text NOT NULL CHECK(query_id ~ '^[a-f0-9]{32}$'),
 parent_id text,
 operation text CHECK(operation IS NULL OR operation ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 topic_id text NOT NULL CHECK(topic_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 topics jsonb NOT NULL CHECK(jsonb_typeof(topics)='array' AND jsonb_array_length(topics) BETWEEN 1 AND 4 AND octet_length(topics::text)<=4096),
 topic_versions jsonb NOT NULL CHECK(jsonb_typeof(topic_versions)='array' AND jsonb_array_length(topic_versions) BETWEEN 1 AND 4 AND octet_length(topic_versions::text)<=4096),
 rule_versions jsonb NOT NULL CHECK(jsonb_typeof(rule_versions)='array' AND jsonb_array_length(rule_versions)<=4 AND octet_length(rule_versions::text)<=4096),
 context_id text NOT NULL CHECK(context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 locale text NOT NULL CHECK(locale IN('en','es')),
 question text NOT NULL CHECK(octet_length(question) BETWEEN 1 AND 16384),
 route jsonb NOT NULL CHECK(jsonb_typeof(route)='object' AND octet_length(route::text)<=1048576),
 generation jsonb NOT NULL CHECK(jsonb_typeof(generation)='object' AND octet_length(generation::text)<=1048576),
 sql_text text CHECK(sql_text IS NULL OR octet_length(sql_text) BETWEEN 1 AND 32768),
 parameters jsonb NOT NULL CHECK(jsonb_typeof(parameters)='array' AND jsonb_array_length(parameters)<=64 AND octet_length(parameters::text)<=524288),
 receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=1048576),
 status text NOT NULL CHECK(status IN('preflight','planned','running','succeeded','empty','truncated','failed','uncertain')),
 result jsonb CHECK(result IS NULL OR (jsonb_typeof(result)='object' AND octet_length(result::text)<=16777216)),
 assumptions jsonb NOT NULL CHECK(jsonb_typeof(assumptions)='array' AND jsonb_array_length(assumptions)<=32 AND octet_length(assumptions::text)<=131072),
 ambiguities jsonb NOT NULL CHECK(jsonb_typeof(ambiguities)='array' AND jsonb_array_length(ambiguities)<=32 AND octet_length(ambiguities::text)<=131072),
 errors jsonb NOT NULL CHECK(jsonb_typeof(errors)='array' AND jsonb_array_length(errors)<=32 AND octet_length(errors::text)<=131072),
 validation_fixes smallint NOT NULL CHECK(validation_fixes BETWEEN 0 AND 1),
 execution_fixes smallint NOT NULL CHECK(execution_fixes BETWEEN 0 AND 1),
 revision bigint NOT NULL CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,actor_id,session_id,query_id),
 FOREIGN KEY(tenant_id,actor_id,session_id) REFERENCES chartworks.nlq_sessions(tenant_id,actor_id,session_id)
);
CREATE UNIQUE INDEX nlq_query_operation ON chartworks.nlq_queries(tenant_id,actor_id,operation) WHERE operation IS NOT NULL;
CREATE INDEX nlq_queries_session ON chartworks.nlq_queries(tenant_id,actor_id,session_id,created_at,query_id);

CREATE TABLE chartworks.nlq_feedback (
 tenant_id text NOT NULL,
 actor_id text NOT NULL,
 session_id text NOT NULL,
 feedback_id text NOT NULL CHECK(feedback_id ~ '^[a-f0-9]{32}$'),
 query_id text NOT NULL,
 verdict text NOT NULL CHECK(verdict IN('positive','negative')),
 correction_sql text CHECK(correction_sql IS NULL OR octet_length(correction_sql) BETWEEN 1 AND 32768),
 note text CHECK(note IS NULL OR octet_length(note)<=4096),
 provenance text NOT NULL CHECK(octet_length(provenance) BETWEEN 1 AND 256),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,feedback_id),
 FOREIGN KEY(tenant_id,actor_id,session_id,query_id) REFERENCES chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id)
);
CREATE UNIQUE INDEX nlq_feedback_once ON chartworks.nlq_feedback(tenant_id,actor_id,session_id,query_id);

CREATE TABLE chartworks.nlq_examples (
 tenant_id text NOT NULL,
 example_id text NOT NULL CHECK(example_id ~ '^[a-f0-9]{32}$'),
 topic_id text NOT NULL CHECK(topic_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 question text NOT NULL CHECK(octet_length(question) BETWEEN 1 AND 16384),
 sql_text text NOT NULL CHECK(octet_length(sql_text) BETWEEN 1 AND 32768),
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 state text NOT NULL CHECK(state IN('candidate','active','retired')),
 weight double precision NOT NULL CHECK(weight>=0 AND weight<=1),
 evidence_count integer NOT NULL CHECK(evidence_count BETWEEN 1 AND 2147483647),
 provenance text NOT NULL CHECK(octet_length(provenance) BETWEEN 1 AND 512),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,example_id),
 UNIQUE(tenant_id,topic_id,digest)
);
CREATE INDEX nlq_examples_active ON chartworks.nlq_examples(tenant_id,topic_id,state,weight DESC,updated_at DESC);

CREATE FUNCTION chartworks.protect_nlq_session() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.context_id,NEW.topics,NEW.locale,NEW.created_at) IS DISTINCT FROM
  (OLD.tenant_id,OLD.actor_id,OLD.session_id,OLD.context_id,OLD.topics,OLD.locale,OLD.created_at) THEN
  RAISE EXCEPTION 'nlq session is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_session_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_sessions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_session();

CREATE FUNCTION chartworks.protect_nlq_query() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.actor_id<>OLD.actor_id OR NEW.session_id<>OLD.session_id OR NEW.query_id<>OLD.query_id OR
    NEW.parent_id IS DISTINCT FROM OLD.parent_id OR NEW.topic_id<>OLD.topic_id OR NEW.topics IS DISTINCT FROM OLD.topics OR
    NEW.topic_versions IS DISTINCT FROM OLD.topic_versions OR NEW.rule_versions IS DISTINCT FROM OLD.rule_versions OR NEW.context_id<>OLD.context_id OR
    NEW.locale<>OLD.locale OR NEW.question<>OLD.question OR NEW.route IS DISTINCT FROM OLD.route OR NEW.created_at<>OLD.created_at OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'nlq query immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_query_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_queries FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_query();

CREATE FUNCTION chartworks.protect_nlq_feedback() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'nlq feedback is immutable' USING ERRCODE='55000';
END $$;
CREATE TRIGGER nlq_feedback_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_feedback FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_feedback();

CREATE FUNCTION chartworks.protect_nlq_example() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.example_id<>OLD.example_id OR NEW.topic_id<>OLD.topic_id OR NEW.question<>OLD.question OR NEW.sql_text<>OLD.sql_text OR NEW.digest<>OLD.digest OR NEW.created_at<>OLD.created_at OR OLD.state='retired' OR (OLD.state='active' AND NEW.state='candidate') THEN
  RAISE EXCEPTION 'nlq example immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_example_immutable BEFORE UPDATE OR DELETE ON chartworks.nlq_examples FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_example();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN('rules.drafted','rules.reviewed','rules.published','rules.retired','topic.reviewed','topic.published','topic.rolled_back','topic.archived','topic.drafted','pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired','facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated','read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted','request.accepted','request.resumed','request.cancelled','request.succeeded','upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased','profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased','nlq.session_created','nlq.query_planned','nlq.query_executed','nlq.feedback_recorded','nlq.example_changed'));
