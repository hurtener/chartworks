-- Cloud APIs disclose their final native identifier only after dispatch. Keep
-- every prior coordinate immutable and permit exactly that missing field once.
CREATE OR REPLACE FUNCTION chartworks.protect_read_manifest() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 acknowledgment_path text[];
BEGIN
 IF ROW(NEW.tenant_id,NEW.actor_id,NEW.attempt_id,NEW.operation_id,NEW.attempt_number,NEW.source_id,NEW.context_id,NEW.manifest,NEW.manifest_hash,NEW.created_at,NEW.deadline)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.actor_id,OLD.attempt_id,OLD.operation_id,OLD.attempt_number,OLD.source_id,OLD.context_id,OLD.manifest,OLD.manifest_hash,OLD.created_at,OLD.deadline)
 OR OLD.cancel_requested AND NOT NEW.cancel_requested
 OR OLD.finished_at IS NOT NULL AND OLD.status<>'uncertain' THEN
 RAISE EXCEPTION 'immutable read attempt' USING ERRCODE='23514';
 END IF;

 IF OLD.remote_query IS NOT NULL AND NEW.remote_query IS DISTINCT FROM OLD.remote_query THEN
 acknowledgment_path := CASE OLD.remote_query->>'driver'
  WHEN 'snowflake' THEN ARRAY['snowflake','query_id']
  WHEN 'databricks' THEN ARRAY['databricks','statement_id']
 END;
 IF acknowledgment_path IS NULL
 OR OLD.status<>'dispatching' OR NEW.status<>'running' OR NEW.remote_state<>'running'
 OR OLD.cancel_requested OR NEW.cancel_requested
 OR OLD.manifest#>>'{validation,dialect}' IS DISTINCT FROM OLD.remote_query->>'driver'
 OR OLD.remote_query#>acknowledgment_path IS NOT NULL
 OR jsonb_typeof(NEW.remote_query#>acknowledgment_path) IS DISTINCT FROM 'string'
 OR NOT COALESCE(NEW.remote_query#>>acknowledgment_path ~ '^[A-Za-z0-9_.:-]{1,128}$',false)
 OR (NEW.remote_query #- acknowledgment_path) IS DISTINCT FROM OLD.remote_query THEN
 RAISE EXCEPTION 'immutable read attempt' USING ERRCODE='23514';
 END IF;
 END IF;
 RETURN NEW;
END $$;
