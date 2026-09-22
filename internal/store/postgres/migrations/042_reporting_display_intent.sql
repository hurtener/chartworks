-- Version-three chart mappings remain inside immutable block definition JSON.
-- This adds structural storage defense without rewriting prior definitions,
-- publications, manifests, outputs or content digests.
CREATE FUNCTION chartworks.reporting_display_intent_valid(d jsonb)
RETURNS boolean LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE AS $$
DECLARE
 output jsonb;
 mapping jsonb;
 column_value jsonb;
 format_value jsonb;
BEGIN
 FOR output IN SELECT value FROM jsonb_array_elements(d->'outputs') LOOP
  mapping := output->'mapping';
  IF mapping IS NULL OR mapping->>'version'<>'3' THEN CONTINUE; END IF;
  IF output->>'kind' NOT IN ('kpi','table')
     OR mapping->>'kind' IS DISTINCT FROM output->>'kind'
     OR jsonb_typeof(mapping->'columns') IS DISTINCT FROM 'array'
     OR jsonb_array_length(mapping->'columns') NOT BETWEEN 1 AND 256 THEN RETURN false; END IF;
  IF output->>'kind'='kpi' THEN
   IF jsonb_typeof(mapping->'kpi') IS DISTINCT FROM 'object' OR mapping ? 'table' THEN RETURN false; END IF;
  ELSE
   IF jsonb_typeof(mapping->'table') IS DISTINCT FROM 'object' OR mapping ? 'kpi' THEN RETURN false; END IF;
  END IF;
  FOR column_value IN SELECT value FROM jsonb_array_elements(mapping->'columns') LOOP
   IF column_value ? 'display_label' AND
      (jsonb_typeof(column_value->'display_label') IS DISTINCT FROM 'string' OR octet_length(column_value->>'display_label')>256) THEN RETURN false; END IF;
   format_value := column_value->'format';
   IF jsonb_typeof(format_value) IS DISTINCT FROM 'object' THEN RETURN false; END IF;
   IF format_value ? 'locale' AND (jsonb_typeof(format_value->'locale') IS DISTINCT FROM 'string' OR octet_length(format_value->>'locale') NOT BETWEEN 0 AND 35) THEN RETURN false; END IF;
   IF format_value ? 'date_pattern' AND (format_value->>'date_pattern') NOT IN ('','date_short','date_medium','date_long','datetime_short','year_month') THEN RETURN false; END IF;
   IF format_value ? 'currency_symbol' AND (jsonb_typeof(format_value->'currency_symbol') IS DISTINCT FROM 'string' OR octet_length(format_value->>'currency_symbol')>8) THEN RETURN false; END IF;
  END LOOP;
 END LOOP;
 RETURN true;
END;
$$;

ALTER TABLE chartworks.block_revisions ADD CONSTRAINT block_display_intent_check
 CHECK(chartworks.reporting_display_intent_valid(definition));
