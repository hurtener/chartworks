-- Content-free synchronous read attempt journal. This is not a second queue,
-- result cache, identity registry or retained user-token store.
CREATE TABLE chartworks.read_attempts (
 tenant_id text NOT NULL,
 actor_id text NOT NULL CHECK (actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 attempt_id text NOT NULL CHECK (attempt_id ~ '^[a-f0-9]{32}$'),
 operation_id text NOT NULL CHECK (operation_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 attempt_number integer NOT NULL CHECK (attempt_number BETWEEN 1 AND 3),
 source_id text NOT NULL,
 context_id text NOT NULL CHECK (context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest)='object' AND octet_length(manifest::text)<=65536),
 manifest_hash text NOT NULL CHECK (manifest_hash ~ '^[a-f0-9]{64}$'),
 status text NOT NULL CHECK (status IN ('accepted','dispatching','running','succeeded','empty','truncated','cancelled','timed_out','failed','uncertain','interrupted')),
 remote_query jsonb CHECK (jsonb_typeof(remote_query)='object' AND octet_length(remote_query::text)<=1024),
 remote_state text NOT NULL CHECK (remote_state IN ('not_issued','running','stopped','unknown')),
 cancel_requested boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL,
 deadline timestamptz NOT NULL CHECK (deadline>created_at AND deadline<=created_at+interval '61 seconds'),
 finished_at timestamptz,
 rows_returned integer NOT NULL DEFAULT 0 CHECK (rows_returned BETWEEN 0 AND 100000),
 bytes_returned integer NOT NULL DEFAULT 0 CHECK (bytes_returned BETWEEN 0 AND 16777216),
 code text NOT NULL DEFAULT '' CHECK (code IN ('','source_unavailable','invalid_result','cancelled','timed_out','result_type_unsupported','limit_exceeded','context_changed','unsupported','remote_outcome_unknown','result_not_retained')),
 PRIMARY KEY (tenant_id,actor_id,attempt_id),
 UNIQUE (tenant_id,actor_id,operation_id,attempt_number),
 FOREIGN KEY (tenant_id,source_id) REFERENCES chartworks.sources(tenant_id,source_id),
 CHECK (status IN ('accepted','dispatching','running','uncertain') OR finished_at IS NOT NULL),
 CHECK (status NOT IN ('succeeded','empty','truncated') OR remote_state='stopped'),
 CHECK (status IN ('succeeded','empty','truncated') OR rows_returned=0 AND bytes_returned=0)
);
CREATE INDEX read_attempt_retention ON chartworks.read_attempts(tenant_id,created_at) WHERE finished_at IS NOT NULL AND status<>'uncertain';
CREATE FUNCTION chartworks.protect_read_manifest() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.tenant_id,NEW.actor_id,NEW.attempt_id,NEW.operation_id,NEW.attempt_number,NEW.source_id,NEW.context_id,NEW.manifest,NEW.manifest_hash,NEW.created_at,NEW.deadline)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.actor_id,OLD.attempt_id,OLD.operation_id,OLD.attempt_number,OLD.source_id,OLD.context_id,OLD.manifest,OLD.manifest_hash,OLD.created_at,OLD.deadline)
 OR OLD.remote_query IS NOT NULL AND NEW.remote_query IS DISTINCT FROM OLD.remote_query
 OR OLD.cancel_requested AND NOT NEW.cancel_requested
 OR OLD.finished_at IS NOT NULL AND OLD.status<>'uncertain' THEN
 RAISE EXCEPTION 'immutable read attempt' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_read_manifest BEFORE UPDATE ON chartworks.read_attempts FOR EACH ROW EXECUTE FUNCTION chartworks.protect_read_manifest();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted'
));
