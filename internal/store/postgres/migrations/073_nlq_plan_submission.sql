-- Retain Plan identity separately from mutable Run attempt operation keys.
ALTER TABLE chartworks.nlq_queries ADD COLUMN plan_operation text;
ALTER TABLE chartworks.nlq_queries ADD COLUMN plan_request_digest text;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_plan_submission_shape CHECK (
 (plan_operation IS NULL AND plan_request_digest IS NULL) OR
 COALESCE(plan_operation ~ '^[A-Za-z0-9_.:-]{1,128}$' AND plan_operation NOT LIKE 'resume:%' AND
 plan_request_digest ~ '^[0-9a-f]{64}$' AND sql_text IS NOT NULL AND generation_pending IS NULL,false));
CREATE UNIQUE INDEX nlq_plan_submission_operation ON chartworks.nlq_queries(tenant_id,actor_id,plan_operation) WHERE plan_operation IS NOT NULL;
CREATE FUNCTION chartworks.protect_nlq_plan_submission() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.plan_operation IS DISTINCT FROM OLD.plan_operation OR NEW.plan_request_digest IS DISTINCT FROM OLD.plan_request_digest THEN
  RAISE EXCEPTION 'nlq plan submission immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_plan_submission_immutable BEFORE UPDATE ON chartworks.nlq_queries
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_plan_submission();

-- Plan and Run keys share one actor namespace, including concurrent writers.
-- This transaction lock is distinct from the service's session-level Plan lock.
CREATE FUNCTION chartworks.fence_nlq_operation_namespace() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE key text;
BEGIN
 FOR key IN SELECT DISTINCT value FROM unnest(ARRAY[NEW.operation,NEW.plan_operation]) AS value WHERE value IS NOT NULL ORDER BY value LOOP
  PERFORM pg_advisory_xact_lock(hashtextextended(jsonb_build_array(NEW.tenant_id,NEW.actor_id,key)::text,7214061011));
  IF EXISTS(SELECT 1 FROM chartworks.nlq_queries q WHERE q.tenant_id=NEW.tenant_id AND q.actor_id=NEW.actor_id AND q.query_id<>NEW.query_id AND (q.operation=key OR q.plan_operation=key)) THEN
   RAISE EXCEPTION 'nlq operation already reserved' USING ERRCODE='23505';
  END IF;
 END LOOP;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_operation_namespace BEFORE INSERT OR UPDATE OF operation,plan_operation ON chartworks.nlq_queries
 FOR EACH ROW EXECUTE FUNCTION chartworks.fence_nlq_operation_namespace();
