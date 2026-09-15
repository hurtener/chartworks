-- Pipeline occurrences share the existing operation, attempt and schedule tables.
ALTER TABLE chartworks.operations
 ADD COLUMN dispatch_manifest jsonb,
 ADD COLUMN scheduled_pipeline_id text GENERATED ALWAYS AS (dispatch_manifest->'pipeline'->>'id') STORED,
 ADD COLUMN scheduled_pipeline_version bigint GENERATED ALWAYS AS ((dispatch_manifest->'pipeline'->>'version')::bigint) STORED,
 ADD CONSTRAINT scheduled_pipeline_version_ref FOREIGN KEY(tenant_id,scheduled_pipeline_id,scheduled_pipeline_version)
 REFERENCES chartworks.pipeline_versions(tenant_id,pipeline_id,version);

ALTER TABLE chartworks.operations DROP CONSTRAINT request_manifest_shape;
ALTER TABLE chartworks.operations ADD CONSTRAINT request_manifest_shape CHECK(
 (dispatch_mode='request' AND dispatch_manifest IS NULL AND kind IN('upload.load','upload.erase','profile.build','pipeline.run','reporting.run') AND
  binding_id IS NULL AND schedule_id IS NULL AND schedule_revision IS NULL AND
  initiator_id=actor_id AND initiator_session IS NOT NULL AND initiator_session ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
  manifest_hash IS NOT NULL AND manifest_hash ~ '^[a-f0-9]{64}$' AND
  request_manifest IS NOT NULL AND jsonb_typeof(request_manifest)='object' AND octet_length(request_manifest::text)<=4096 AND
  request_manifest->>'kind'=kind AND due_at=created_at AND window_start=created_at AND window_end=created_at)
 OR (dispatch_mode IN('inline','queued') AND kind='retention.sweep' AND request_manifest IS NULL AND dispatch_manifest IS NULL)
 OR (dispatch_mode='queued' AND kind='pipeline.run' AND COALESCE(
  jsonb_typeof(dispatch_manifest)='object' AND octet_length(dispatch_manifest::text)<=8192 AND
  jsonb_typeof(request_manifest)='object' AND octet_length(request_manifest::text)<=4096 AND
  dispatch_manifest->>'id'=operation_id AND dispatch_manifest->>'tenant'=tenant_id AND
  dispatch_manifest->>'kind'=kind AND dispatch_manifest->>'binding_id'=binding_id AND
  dispatch_manifest->>'executor'=actor_id AND dispatch_manifest->>'manifest_hash'=manifest_hash AND
  request_manifest->>'kind'=kind AND request_manifest->>'target'=scheduled_pipeline_id AND
  request_manifest->>'input_hash'=dispatch_manifest->'pipeline'->>'digest' AND
  scheduled_pipeline_id IS NOT NULL AND scheduled_pipeline_version>0, false)));

CREATE FUNCTION chartworks.protect_pipeline_dispatch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.dispatch_manifest IS DISTINCT FROM OLD.dispatch_manifest OR
 (OLD.dispatch_manifest IS NOT NULL AND NEW.request_manifest IS DISTINCT FROM OLD.request_manifest)
 THEN RAISE EXCEPTION 'immutable pipeline occurrence' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER pipeline_dispatch_immutable BEFORE UPDATE ON chartworks.operations
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_pipeline_dispatch();
