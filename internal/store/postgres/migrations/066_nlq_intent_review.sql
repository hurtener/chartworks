ALTER TABLE chartworks.nlq_queries ADD COLUMN intent_review jsonb;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_intent_review_shape CHECK (
 intent_review IS NULL OR COALESCE(
  jsonb_typeof(intent_review)='object' AND octet_length(intent_review::text)<=2048 AND parent_id IS NOT NULL AND
  (analytical_version>=7 OR generation_pending IS NOT NULL) AND
  jsonb_typeof(intent_review->'legacy')='object' AND
  jsonb_typeof(intent_review->'preflight')='object' AND
  intent_review->'legacy'->>'query_id' ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
  intent_review->'preflight'->>'query_id' ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
  intent_review->'legacy'->>'revision' ~ '^[1-9][0-9]*$' AND
  intent_review->'preflight'->>'revision' ~ '^[1-9][0-9]*$' AND
  intent_review->'legacy'->>'digest' ~ '^[0-9a-f]{64}$' AND
  intent_review->'preflight'->>'digest' ~ '^[0-9a-f]{64}$' AND
  intent_review->>'answer_digest' ~ '^[0-9a-f]{64}$' AND
  (NOT (intent_review ? 'selection_digest') OR intent_review->>'selection_digest' ~ '^[0-9a-f]{64}$') AND
  intent_review->'legacy'->>'query_id' <> intent_review->'preflight'->>'query_id' AND
  intent_review->'legacy'->>'query_id' <> query_id AND
  intent_review->'preflight'->>'query_id' <> query_id,false));
CREATE FUNCTION chartworks.protect_nlq_intent_review() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.intent_review IS DISTINCT FROM OLD.intent_review THEN
  RAISE EXCEPTION 'nlq intent review immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_intent_review_immutable BEFORE UPDATE ON chartworks.nlq_queries
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_intent_review();

CREATE UNIQUE INDEX nlq_intent_review_initial ON chartworks.nlq_queries
 (tenant_id,actor_id,(intent_review->'legacy'->>'query_id'),(intent_review->'preflight'->>'query_id'))
 WHERE intent_review IS NOT NULL AND generation_resolution IS NULL;

ALTER TABLE chartworks.nlq_queries
 ADD COLUMN intent_review_legacy_id text GENERATED ALWAYS AS (intent_review->'legacy'->>'query_id') STORED,
 ADD COLUMN intent_review_preflight_id text GENERATED ALWAYS AS (intent_review->'preflight'->>'query_id') STORED,
 ADD CONSTRAINT nlq_intent_review_legacy_origin FOREIGN KEY(tenant_id,actor_id,session_id,intent_review_legacy_id)
  REFERENCES chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id),
 ADD CONSTRAINT nlq_intent_review_preflight_origin FOREIGN KEY(tenant_id,actor_id,session_id,intent_review_preflight_id)
  REFERENCES chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id);
