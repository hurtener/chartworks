-- Forward-only: existing v1 rows/digests/origins are unchanged. New v2 schemas
-- contain only native-proved input domains and dense kinds; no values/defaults.
CREATE OR REPLACE FUNCTION chartworks.valid_example_parameters(value jsonb) RETURNS boolean
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE slot jsonb; slot_number integer := 0; version text; has_domain boolean := false;
BEGIN
 IF value IS NULL THEN RETURN true; END IF;
 IF jsonb_typeof(value) IS DISTINCT FROM 'object' OR octet_length(value::text)>16384
  OR value->>'version' NOT IN ('example-parameters-v1','example-parameters-v2')
  OR jsonb_typeof(value->'version') IS DISTINCT FROM 'string'
  OR jsonb_typeof(value->'slots') IS DISTINCT FROM 'array'
  OR value - 'version' - 'slots' <> '{}'::jsonb THEN RETURN false; END IF;
 version := value->>'version';
 IF jsonb_array_length(value->'slots') NOT BETWEEN 1 AND 64 THEN RETURN false; END IF;
 FOR slot IN SELECT jsonb_array_elements(value->'slots') LOOP
  slot_number := slot_number+1;
  IF jsonb_typeof(slot) IS DISTINCT FROM 'object'
   OR jsonb_typeof(slot->'position') IS DISTINCT FROM 'number'
   OR slot->>'position' IS DISTINCT FROM slot_number::text
   OR jsonb_typeof(slot->'kind') IS DISTINCT FROM 'string'
   OR slot->>'kind' NOT IN ('text','integer','number','boolean','null')
   OR slot - 'position' - 'kind' - 'domain' <> '{}'::jsonb THEN RETURN false; END IF;
  IF slot ? 'domain' THEN
   IF version <> 'example-parameters-v2' OR slot->>'kind' <> 'text'
    OR jsonb_typeof(slot->'domain') IS DISTINCT FROM 'string'
    OR slot->>'domain' NOT IN ('date','timestamp','timestamptz','uuid','time','timetz','interval','json','jsonb') THEN RETURN false; END IF;
   has_domain := true;
  END IF;
 END LOOP;
 RETURN version = 'example-parameters-v1' OR has_domain;
END $$;
-- The existing immutable schema and origin triggers remain in force.
