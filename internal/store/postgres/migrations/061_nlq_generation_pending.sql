-- Pending generation is immutable protected query evidence, never SQL authority.
ALTER TABLE chartworks.nlq_queries ADD COLUMN generation_pending jsonb;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_generation_pending_shape CHECK (
 generation_pending IS NULL OR COALESCE(
  status='preflight' AND sql_text IS NULL AND result IS NULL
  AND jsonb_typeof(generation_pending)='object'
  AND octet_length(generation_pending::text)<=131072
  AND generation_pending->'problem'->>'query_id'=query_id
  AND generation_pending->'problem'->>'answer_context' ~ '^[0-9a-f]{64}$'
  AND (generation_pending->>'round')::integer BETWEEN 1 AND 3,
 false)
);

CREATE FUNCTION chartworks.protect_nlq_generation_pending() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.generation_pending IS DISTINCT FROM OLD.generation_pending THEN
  RAISE EXCEPTION 'nlq generation question is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_generation_pending_immutable BEFORE UPDATE ON chartworks.nlq_queries
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_generation_pending();

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action=''nlq.generation_pending'')',previous);
END $$;

ALTER TABLE chartworks.nlq_queries ADD COLUMN generation_resolution jsonb;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_generation_resolution_shape CHECK (
 generation_resolution IS NULL OR COALESCE(
 jsonb_typeof(generation_resolution)='object' AND octet_length(generation_resolution::text)<=512
 AND generation_resolution->>'query_id'=parent_id
 AND generation_resolution->>'answer_digest' ~ '^[0-9a-f]{64}$',false));
CREATE FUNCTION chartworks.protect_nlq_generation_resolution() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.generation_resolution IS DISTINCT FROM OLD.generation_resolution THEN
  RAISE EXCEPTION 'nlq generation resolution is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_generation_resolution_immutable BEFORE UPDATE ON chartworks.nlq_queries
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_generation_resolution();
