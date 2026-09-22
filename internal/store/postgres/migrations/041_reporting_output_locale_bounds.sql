-- Align persisted locale bytes with the existing domain locale contract.
-- No revision, publication, accepted manifest, receipt or payload is rewritten.
-- Migration 035 and its checksum remain unchanged.
CREATE OR REPLACE FUNCTION chartworks.reporting_output_intents_valid(d jsonb)
RETURNS boolean LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE AS $$
DECLARE
 output jsonb;
 intent jsonb;
 label jsonb;
 ids text[] := ARRAY[]::text[];
 orders integer[] := ARRAY[]::integer[];
 locales text[];
 position integer;
BEGIN
 IF jsonb_typeof(d->'schema_version') IS DISTINCT FROM 'number'
    OR (d->>'schema_version') NOT IN ('1','2')
    OR jsonb_typeof(d->'outputs') IS DISTINCT FROM 'array'
    OR jsonb_array_length(d->'outputs') NOT BETWEEN 1 AND 64 THEN
  RETURN false;
 END IF;
 FOR output IN SELECT value FROM jsonb_array_elements(d->'outputs') LOOP
  IF jsonb_typeof(output->'id') IS DISTINCT FROM 'string'
     OR length(output->>'id') NOT BETWEEN 1 AND 128
     OR (output->>'id')=ANY(ids) THEN RETURN false; END IF;
  ids := array_append(ids, output->>'id');
  IF d->>'schema_version'='1' THEN
   -- New intent cannot be hidden under legacy selection semantics.
   IF output ? 'intent' THEN RETURN false; END IF;
   CONTINUE;
  END IF;
  intent := output->'intent';
  IF jsonb_typeof(intent) IS DISTINCT FROM 'object'
     OR jsonb_typeof(intent->'enabled') IS DISTINCT FROM 'boolean'
     OR jsonb_typeof(intent->'default_selected') IS DISTINCT FROM 'boolean'
     OR jsonb_typeof(intent->'display_order') IS DISTINCT FROM 'number'
     OR (intent->>'display_order') !~ '^[0-9]{1,2}$'
     OR jsonb_typeof(intent->'metadata') IS DISTINCT FROM 'array'
     OR jsonb_array_length(intent->'metadata') NOT BETWEEN 1 AND 32 THEN
   RETURN false;
  END IF;
  position := (intent->>'display_order')::integer;
  IF position NOT BETWEEN 0 AND 63 OR position=ANY(orders) THEN RETURN false; END IF;
  orders := array_append(orders,position);
  locales := ARRAY[]::text[];
  FOR label IN SELECT value FROM jsonb_array_elements(intent->'metadata') LOOP
   IF jsonb_typeof(label->'locale') IS DISTINCT FROM 'string'
      OR octet_length(label->>'locale') NOT BETWEEN 2 AND 64
      OR (label->>'locale')=ANY(locales)
      OR jsonb_typeof(label->'display_name') IS DISTINCT FROM 'string'
      OR length(btrim(label->>'display_name'))=0
      OR octet_length(label->>'display_name')>256
      OR jsonb_typeof(label->'description') IS DISTINCT FROM 'string'
      OR octet_length(label->>'description')>4096 THEN RETURN false; END IF;
   locales := array_append(locales,label->>'locale');
  END LOOP;
 END LOOP;
 RETURN true;
END;
$$;
