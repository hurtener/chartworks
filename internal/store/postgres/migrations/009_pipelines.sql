CREATE TABLE chartworks.pipeline_heads (
 tenant_id text NOT NULL REFERENCES chartworks.policies(tenant_id),
 pipeline_id text NOT NULL CHECK(pipeline_id ~ '^[A-Za-z0-9_.:-]{1,48}$'),
 actor_id text NOT NULL, session_id text NOT NULL,
 draft_version bigint NOT NULL CHECK(draft_version>0), published_version bigint,
 PRIMARY KEY(tenant_id,pipeline_id), CHECK(published_version IS NULL OR published_version>0 AND published_version<=draft_version)
);
CREATE TABLE chartworks.pipeline_versions (
 tenant_id text NOT NULL, pipeline_id text NOT NULL, version bigint NOT NULL CHECK(version>0),
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object' AND octet_length(manifest::text)<=2097152),
 manifest_hash text NOT NULL CHECK(manifest_hash ~ '^[a-f0-9]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), published_at timestamptz,
 PRIMARY KEY(tenant_id,pipeline_id,version),
 FOREIGN KEY(tenant_id,pipeline_id) REFERENCES chartworks.pipeline_heads(tenant_id,pipeline_id)
);
CREATE FUNCTION chartworks.protect_pipeline_version() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.pipeline_id,NEW.version,NEW.manifest,NEW.manifest_hash,NEW.created_at) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.pipeline_id,OLD.version,OLD.manifest,OLD.manifest_hash,OLD.created_at)
 OR OLD.published_at IS NOT NULL AND NEW.published_at IS DISTINCT FROM OLD.published_at
 THEN RAISE EXCEPTION 'immutable pipeline version' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER pipeline_version_immutable BEFORE UPDATE ON chartworks.pipeline_versions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_pipeline_version();
ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN('pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted',
 'request.accepted','request.resumed','request.cancelled','request.succeeded',
 'upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased',
 'profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased'
));

ALTER TABLE chartworks.operations DROP CONSTRAINT operations_kind_check;
ALTER TABLE chartworks.operations ADD CONSTRAINT operations_kind_check CHECK(kind IN('retention.sweep','upload.load','upload.erase','profile.build','pipeline.run'));
ALTER TABLE chartworks.operations DROP CONSTRAINT request_manifest_shape;
ALTER TABLE chartworks.operations ADD CONSTRAINT request_manifest_shape CHECK(
 (dispatch_mode='request' AND kind IN('upload.load','upload.erase','profile.build','pipeline.run') AND
  binding_id IS NULL AND schedule_id IS NULL AND schedule_revision IS NULL AND
  initiator_id=actor_id AND initiator_session IS NOT NULL AND initiator_session ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
  manifest_hash IS NOT NULL AND manifest_hash ~ '^[a-f0-9]{64}$' AND
  request_manifest IS NOT NULL AND jsonb_typeof(request_manifest)='object' AND octet_length(request_manifest::text)<=4096 AND
  request_manifest->>'kind'=kind AND due_at=created_at AND window_start=created_at AND window_end=created_at)
 OR (dispatch_mode IN('inline','queued') AND kind='retention.sweep' AND request_manifest IS NULL));

CREATE TABLE chartworks.pipeline_runs (
 tenant_id text NOT NULL, operation_id text NOT NULL, pipeline_id text NOT NULL, version bigint NOT NULL,
 manifest_hash text NOT NULL CHECK(manifest_hash ~ '^[a-f0-9]{64}$'),
 state text NOT NULL CHECK(state IN('staged','uncertain','quality_failed','published')),
 PRIMARY KEY(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,pipeline_id,version) REFERENCES chartworks.pipeline_versions(tenant_id,pipeline_id,version)
);
CREATE TABLE chartworks.pipeline_stages (
 tenant_id text NOT NULL, operation_id text NOT NULL, step_id text NOT NULL,
 source_id text NOT NULL, source_revision bigint NOT NULL CHECK(source_revision>0),
 stage jsonb NOT NULL CHECK(jsonb_typeof(stage)='object' AND octet_length(stage::text)<=262144),
 PRIMARY KEY(tenant_id,operation_id,step_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.pipeline_runs(tenant_id,operation_id)
);
CREATE TABLE chartworks.pipeline_outputs (
 tenant_id text NOT NULL, pipeline_id text NOT NULL, step_id text NOT NULL,
 operation_id text NOT NULL, source_id text NOT NULL,
 PRIMARY KEY(tenant_id,pipeline_id,step_id),
 FOREIGN KEY(tenant_id,pipeline_id) REFERENCES chartworks.pipeline_heads(tenant_id,pipeline_id),
 FOREIGN KEY(tenant_id,operation_id,step_id) REFERENCES chartworks.pipeline_stages(tenant_id,operation_id,step_id),
 FOREIGN KEY(tenant_id,source_id) REFERENCES chartworks.sources(tenant_id,source_id)
);

ALTER TABLE chartworks.source_revisions ADD COLUMN pipeline jsonb CHECK(pipeline IS NULL OR jsonb_typeof(pipeline)='object' AND octet_length(pipeline::text)<=16384);
