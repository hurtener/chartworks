-- Historical examples remain unchanged and have no parameter schema. Versioned
-- schemas retain types/positions only, never previous source or private values.
CREATE FUNCTION chartworks.valid_example_parameters(value jsonb) RETURNS boolean
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE slot jsonb; slot_number integer := 0;
BEGIN
 IF value IS NULL THEN RETURN true; END IF;
 IF jsonb_typeof(value) IS DISTINCT FROM 'object' OR octet_length(value::text)>16384
  OR value->>'version' IS DISTINCT FROM 'example-parameters-v1'
  OR jsonb_typeof(value->'slots') IS DISTINCT FROM 'array'
  OR value - 'version' - 'slots' <> '{}'::jsonb THEN RETURN false; END IF;
 IF jsonb_array_length(value->'slots') NOT BETWEEN 1 AND 64 THEN RETURN false; END IF;
 FOR slot IN SELECT jsonb_array_elements(value->'slots') LOOP
  slot_number := slot_number+1;
  IF jsonb_typeof(slot) IS DISTINCT FROM 'object'
   OR jsonb_typeof(slot->'position') IS DISTINCT FROM 'number'
   OR slot->>'position' IS DISTINCT FROM slot_number::text
   OR jsonb_typeof(slot->'kind') IS DISTINCT FROM 'string'
   OR slot->>'kind' NOT IN ('text','integer','number','boolean','null')
   OR slot - 'position' - 'kind' <> '{}'::jsonb THEN RETURN false; END IF;
 END LOOP;
 RETURN true;
END $$;

ALTER TABLE chartworks.nlq_examples ADD COLUMN parameter_schema jsonb
 CHECK(chartworks.valid_example_parameters(parameter_schema));

-- A reviewed schema cannot be relabeled, removed or gain private default values.
-- The existing trigger still guards SQL/question/digest/origin and review state.
CREATE FUNCTION chartworks.protect_example_parameters() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.parameter_schema IS DISTINCT FROM OLD.parameter_schema THEN
  RAISE EXCEPTION 'example parameter schema is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_example_parameters_immutable BEFORE UPDATE ON chartworks.nlq_examples
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_example_parameters();
