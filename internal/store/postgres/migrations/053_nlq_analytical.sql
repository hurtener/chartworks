-- Older query rows stay explicitly unmeasured. No old SQL is retroactively
-- labeled analytically correct. New service-created plans select version 1.
ALTER TABLE chartworks.nlq_queries
 ADD COLUMN analytical_version smallint NOT NULL DEFAULT 0 CHECK (analytical_version IN (0,1)),
 ADD COLUMN analytical jsonb,
 ADD CONSTRAINT nlq_analytical_shape CHECK (
  (analytical_version=0 AND analytical IS NULL) OR
  (analytical_version=1 AND (analytical IS NULL OR COALESCE(
   jsonb_typeof(analytical)='object' AND octet_length(analytical::text)<=16384
   AND analytical->>'version'='analytical-metrics-v1'
   AND analytical->>'scope'='selected_metric_expression_and_population;single_base_relation'
   AND analytical->>'contract' ~ '^[0-9a-f]{64}$'
   AND analytical->>'query' ~ '^[0-9a-f]{64}$'
   AND jsonb_typeof(analytical->'metrics')='array'
   AND jsonb_array_length(analytical->'metrics') BETWEEN 1 AND 32, false)))
 );

-- A service-validated, semantics-preserving execution correction may change only
-- the query digest. It cannot drop/swap the selected semantic proof or downgrade
-- a versioned plan into an unmeasured historical one.
CREATE FUNCTION chartworks.protect_nlq_analytical() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.analytical_version IS DISTINCT FROM OLD.analytical_version
  OR (NEW.analytical - 'query') IS DISTINCT FROM (OLD.analytical - 'query') THEN
  RAISE EXCEPTION 'nlq analytical contract is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_analytical_immutable BEFORE UPDATE ON chartworks.nlq_queries
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_analytical();
