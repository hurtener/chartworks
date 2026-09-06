CREATE TABLE chartworks.profile_versions (
 tenant_id text NOT NULL,
 profile_id text NOT NULL CHECK(profile_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 source_id text NOT NULL,
 context_id text NOT NULL CHECK(context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 dataset_id text NOT NULL CHECK(dataset_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object' AND octet_length(manifest::text)<=262144),
 manifest_hash text NOT NULL CHECK(manifest_hash ~ '^[a-f0-9]{64}$'),
 state text NOT NULL DEFAULT 'reserved' CHECK(state IN('reserved','sampling','checkpoint','complete','erased')),
 operation_id text,
 created_at timestamptz NOT NULL,
 result jsonb CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=524288),
 deterministic_hash text CHECK(deterministic_hash ~ '^[a-f0-9]{64}$'),
 summary_started boolean NOT NULL DEFAULT false,
 last_read_operation text CHECK(last_read_operation ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 last_read_deadline timestamptz,
 changes jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(changes)='array' AND octet_length(changes::text)<=262144),
 PRIMARY KEY(tenant_id,profile_id),
 FOREIGN KEY(tenant_id,source_id) REFERENCES chartworks.sources(tenant_id,source_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id) ON DELETE SET NULL(operation_id),
 CHECK((result IS NULL)=(deterministic_hash IS NULL)),
 CHECK((state IN('checkpoint','complete'))=(result IS NOT NULL)),
 CHECK((last_read_operation IS NULL)=(last_read_deadline IS NULL))
);
CREATE INDEX profile_history ON chartworks.profile_versions(tenant_id,actor_id,session_id,source_id,context_id,dataset_id,created_at);
CREATE TABLE chartworks.profile_heads (
 tenant_id text NOT NULL,
 actor_id text NOT NULL,
 session_id text NOT NULL,
 source_id text NOT NULL,
 dataset_id text NOT NULL,
 profile_id text NOT NULL,
 PRIMARY KEY(tenant_id,actor_id,session_id,source_id,dataset_id),
 FOREIGN KEY(tenant_id,profile_id) REFERENCES chartworks.profile_versions(tenant_id,profile_id),
 FOREIGN KEY(tenant_id,source_id) REFERENCES chartworks.sources(tenant_id,source_id)
);
CREATE FUNCTION chartworks.protect_profile_version() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.profile_id,NEW.source_id,NEW.context_id,NEW.dataset_id,NEW.actor_id,NEW.session_id,NEW.manifest,NEW.manifest_hash,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.profile_id,OLD.source_id,OLD.context_id,OLD.dataset_id,OLD.actor_id,OLD.session_id,OLD.manifest,OLD.manifest_hash,OLD.created_at)
 OR OLD.state='erased' AND NEW.state<>'erased'
 OR OLD.state='complete' AND NEW.state<>'erased' AND ROW(NEW.state,NEW.result,NEW.deterministic_hash,NEW.summary_started,NEW.last_read_operation,NEW.last_read_deadline,NEW.changes) IS DISTINCT FROM ROW(OLD.state,OLD.result,OLD.deterministic_hash,OLD.summary_started,OLD.last_read_operation,OLD.last_read_deadline,OLD.changes)
 OR OLD.deterministic_hash IS NOT NULL AND NEW.state<>'erased' AND NEW.deterministic_hash IS DISTINCT FROM OLD.deterministic_hash
 OR OLD.summary_started AND NOT NEW.summary_started
 THEN RAISE EXCEPTION 'immutable profile evidence' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER profile_version_immutable BEFORE UPDATE ON chartworks.profile_versions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_profile_version();

CREATE TABLE chartworks.profile_dependencies (
 tenant_id text NOT NULL,
 actor_id text NOT NULL,
 session_id text NOT NULL,
 kind text NOT NULL CHECK(kind IN('topic','block','report','dashboard')),
 resource_id text NOT NULL CHECK(resource_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 definition_version text NOT NULL CHECK(definition_version ~ '^[a-f0-9]{64}$'),
 source_id text NOT NULL,
 dataset_id text NOT NULL,
 profile_id text NOT NULL,
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object' AND octet_length(manifest::text)<=32768),
 PRIMARY KEY(tenant_id,actor_id,session_id,kind,resource_id,definition_version),
 FOREIGN KEY(tenant_id,profile_id) REFERENCES chartworks.profile_versions(tenant_id,profile_id),
 FOREIGN KEY(tenant_id,source_id) REFERENCES chartworks.sources(tenant_id,source_id)
);
CREATE TABLE chartworks.profile_health_events (
 tenant_id text NOT NULL,
 event_id text NOT NULL CHECK(event_id ~ '^[a-f0-9]{64}$'),
 actor_id text NOT NULL,
 session_id text NOT NULL,
 profile_id text NOT NULL,
 kind text NOT NULL,
 resource_id text NOT NULL,
 definition_version text NOT NULL,
 event jsonb NOT NULL CHECK(jsonb_typeof(event)='object' AND octet_length(event::text)<=262144),
 PRIMARY KEY(tenant_id,event_id),
 FOREIGN KEY(tenant_id,profile_id) REFERENCES chartworks.profile_versions(tenant_id,profile_id),
 FOREIGN KEY(tenant_id,actor_id,session_id,kind,resource_id,definition_version) REFERENCES chartworks.profile_dependencies(tenant_id,actor_id,session_id,kind,resource_id,definition_version)
);
CREATE TRIGGER dependency_version_immutable BEFORE UPDATE ON chartworks.profile_dependencies FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();
CREATE TRIGGER profile_health_immutable BEFORE UPDATE ON chartworks.profile_health_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN(
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted',
 'request.accepted','request.resumed','request.cancelled','request.succeeded',
 'upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased',
 'profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased'
));
