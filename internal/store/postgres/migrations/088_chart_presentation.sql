-- Optional display overrides are independent of mapping generation. Existing
-- definitions without the extension remain byte-for-byte unchanged. This is a
-- closed structural fence; native validation owns exact renderer roles and
-- canonical-equivalent normalization.
CREATE FUNCTION chartworks.reporting_presentation_valid(d jsonb)
RETURNS boolean LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE AS $$
DECLARE
 output jsonb;
 mapping jsonb;
 presentation jsonb;
 item jsonb;
 canonical jsonb;
 seen text[];
 position bigint;
 previous_position bigint;
BEGIN
 IF jsonb_typeof(d->'outputs') IS DISTINCT FROM 'array' THEN RETURN false; END IF;
 FOR output IN SELECT value FROM jsonb_array_elements(d->'outputs') LOOP
  mapping := output->'mapping';
  IF mapping IS NULL OR NOT (mapping ? 'presentation') THEN CONTINUE; END IF;
  presentation := mapping->'presentation';
  IF jsonb_typeof(mapping) IS DISTINCT FROM 'object'
     OR NOT COALESCE(mapping->'version' IN ('1'::jsonb,'2'::jsonb,'3'::jsonb),false)
     OR jsonb_typeof(presentation) IS DISTINCT FROM 'object'
     OR presentation - ARRAY['version','columns'] <> '{}'::jsonb
     OR presentation->'version' IS DISTINCT FROM '1'::jsonb
     OR jsonb_typeof(presentation->'columns') IS DISTINCT FROM 'array'
     OR jsonb_typeof(mapping->'columns') IS DISTINCT FROM 'array' THEN RETURN false; END IF;
  IF jsonb_array_length(presentation->'columns') NOT BETWEEN 1 AND 256
     OR jsonb_array_length(mapping->'columns') NOT BETWEEN 1 AND 256 THEN RETURN false; END IF;
  seen := ARRAY[]::text[];
  previous_position := 0;
  FOR item IN SELECT value FROM jsonb_array_elements(presentation->'columns') LOOP
   IF jsonb_typeof(item) IS DISTINCT FROM 'object'
      OR item - ARRAY['column','display_label','fraction_digits'] <> '{}'::jsonb
      OR jsonb_typeof(item->'column') IS DISTINCT FROM 'string'
      OR item->>'column' !~ '^[A-Za-z0-9_.:-]{1,128}$'
      OR item->>'column' = ANY(seen)
      OR NOT (item ? 'display_label' OR item ? 'fraction_digits') THEN RETURN false; END IF;
   IF (SELECT count(*) FROM jsonb_array_elements(mapping->'columns')
       WHERE value->>'id'=item->>'column') <> 1 THEN RETURN false; END IF;
   SELECT value, ordinality INTO canonical, position
    FROM jsonb_array_elements(mapping->'columns') WITH ORDINALITY
    WHERE value->>'id'=item->>'column';
   IF position<=previous_position THEN RETURN false; END IF;
   previous_position := position;
   seen := array_append(seen,item->>'column');
   -- Every table override addresses a bound, visible column. Other display
   -- roles remain authoritative in the native mapping validator.
   IF mapping->>'kind'='table' THEN
    IF jsonb_typeof(mapping->'bindings'->'columns') IS DISTINCT FROM 'array' THEN RETURN false; END IF;
    IF NOT (mapping->'bindings'->'columns' @> jsonb_build_array(item->'column')) THEN RETURN false; END IF;
    IF mapping->'version'='3'::jsonb THEN
     IF jsonb_typeof(mapping->'table'->'columns') IS DISTINCT FROM 'array' THEN RETURN false; END IF;
     IF NOT EXISTS (SELECT 1 FROM jsonb_array_elements(mapping->'table'->'columns') c
                    WHERE c->'column'=item->'column' AND c->'visible'='true'::jsonb) THEN RETURN false; END IF;
    END IF;
   END IF;
   IF item ? 'display_label' AND
      (jsonb_typeof(item->'display_label') IS DISTINCT FROM 'string'
       OR octet_length(item->>'display_label')>256
       OR mapping->>'kind' IS DISTINCT FROM 'table') THEN RETURN false; END IF;
   IF item ? 'fraction_digits' THEN
    IF jsonb_typeof(item->'fraction_digits') IS DISTINCT FROM 'number'
       OR item->>'fraction_digits' !~ '^[0-9]+$'
       OR NOT COALESCE(canonical->>'type' IN ('integer','decimal','number'),false)
       OR canonical->'format'->>'percent' IS DISTINCT FROM '' THEN RETURN false; END IF;
    IF (item->>'fraction_digits')::numeric NOT BETWEEN 0 AND 20 THEN RETURN false; END IF;
   END IF;
  END LOOP;
 END LOOP;
 RETURN true;
END;
$$;

ALTER TABLE chartworks.block_revisions ADD CONSTRAINT block_presentation_check
 CHECK(chartworks.reporting_presentation_valid(definition));
