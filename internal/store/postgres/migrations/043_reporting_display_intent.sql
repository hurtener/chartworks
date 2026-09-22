-- Version-three chart mappings remain inside immutable block definition JSON.
-- This adds structural storage defense without rewriting prior definitions,
-- publications, manifests, outputs or content digests. The database check is a
-- second line of defense for the closed shape; Go remains the semantic validator.
CREATE FUNCTION chartworks.reporting_display_intent_valid(d jsonb)
RETURNS boolean LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE AS $$
DECLARE
 output jsonb;
 mapping jsonb;
 bindings jsonb;
 display_options jsonb;
 column_value jsonb;
 format_value jsonb;
 threshold_value jsonb;
 actual_ids jsonb;
 expected_ids jsonb;
 ordinal bigint;
BEGIN
 FOR output IN SELECT value FROM jsonb_array_elements(d->'outputs') LOOP
  mapping := output->'mapping';
  IF mapping IS NULL OR mapping->>'version'<>'3' THEN CONTINUE; END IF;
  bindings := mapping->'bindings';
  IF output->>'kind' NOT IN ('kpi','table')
     OR mapping->>'kind' IS DISTINCT FROM output->>'kind'
     OR jsonb_typeof(bindings) IS DISTINCT FROM 'object'
     OR jsonb_typeof(mapping->'columns') IS DISTINCT FROM 'array'
     OR jsonb_array_length(mapping->'columns') NOT BETWEEN 1 AND 256 THEN RETURN false; END IF;

  actual_ids := '[]'::jsonb;
  FOR column_value, ordinal IN SELECT value, ordinality FROM jsonb_array_elements(mapping->'columns') WITH ORDINALITY LOOP
   IF jsonb_typeof(column_value) IS DISTINCT FROM 'object'
      OR jsonb_typeof(column_value->'id') IS DISTINCT FROM 'string'
      OR column_value->>'id' !~ '^[A-Za-z0-9_.:-]{1,128}$'
      OR jsonb_typeof(column_value->'name') IS DISTINCT FROM 'string'
      OR octet_length(column_value->>'name') NOT BETWEEN 1 AND 1024
      OR jsonb_typeof(column_value->'type') IS DISTINCT FROM 'string'
      OR column_value->>'type' NOT IN ('integer','decimal','number','text','temporal','boolean','binary','structured')
      OR jsonb_typeof(column_value->'role') IS DISTINCT FROM 'string'
      OR column_value->>'role' NOT IN ('unknown','identifier','dimension','time','measure','kpi')
      OR jsonb_typeof(column_value->'grain') IS DISTINCT FROM 'string'
      OR column_value->>'grain' NOT IN ('','second','minute','hour','day','week','month','quarter','year')
      OR jsonb_typeof(column_value->'aggregation') IS DISTINCT FROM 'string'
      OR column_value->>'aggregation' NOT IN ('','sum','count','average','minimum','maximum','distinct_count') THEN RETURN false; END IF;
   IF column_value ? 'display_label' AND
      (jsonb_typeof(column_value->'display_label') IS DISTINCT FROM 'string' OR octet_length(column_value->>'display_label')>256) THEN RETURN false; END IF;

   format_value := column_value->'format';
   IF jsonb_typeof(format_value) IS DISTINCT FROM 'object'
      OR jsonb_typeof(format_value->'unit') IS DISTINCT FROM 'string' OR octet_length(format_value->>'unit')>64
      OR jsonb_typeof(format_value->'currency') IS DISTINCT FROM 'string'
      OR (format_value->>'currency' <> '' AND format_value->>'currency' !~ '^[A-Z]{3}$')
      OR jsonb_typeof(format_value->'percent') IS DISTINCT FROM 'string' OR format_value->>'percent' NOT IN ('','fraction','whole')
      OR (format_value->>'currency' <> '' AND format_value->>'percent' <> '')
      OR jsonb_typeof(format_value->'fraction_digits') IS DISTINCT FROM 'number'
      OR format_value->>'fraction_digits' !~ '^[0-9]+$'
      OR (format_value->>'fraction_digits')::numeric NOT BETWEEN 0 AND 20 THEN RETURN false; END IF;
   IF format_value ? 'currency_symbol' AND
      (jsonb_typeof(format_value->'currency_symbol') IS DISTINCT FROM 'string' OR octet_length(format_value->>'currency_symbol')>8
       OR (format_value->>'currency_symbol'<>'' AND format_value->>'currency'='')) THEN RETURN false; END IF;
   IF format_value ? 'locale' AND
      (jsonb_typeof(format_value->'locale') IS DISTINCT FROM 'string'
       OR (format_value->>'locale' <> '' AND format_value->>'locale' !~ '^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8})*$')
       OR octet_length(format_value->>'locale')>35) THEN RETURN false; END IF;
   IF format_value ? 'date_pattern' AND
      (jsonb_typeof(format_value->'date_pattern') IS DISTINCT FROM 'string'
       OR format_value->>'date_pattern' NOT IN ('','date_short','date_medium','date_long','datetime_short','year_month')
       OR (format_value->>'date_pattern' <> '' AND column_value->>'type'<>'temporal')) THEN RETURN false; END IF;
   IF column_value->>'type' NOT IN ('integer','decimal','number') AND
      (column_value->>'role' IN ('measure','kpi') OR column_value->>'aggregation'<>'' OR format_value->>'currency'<>'' OR format_value->>'percent'<>'' OR format_value->>'fraction_digits'<>'0') THEN RETURN false; END IF;
   IF (column_value->>'role'='time' OR column_value->>'grain'<>'') AND column_value->>'type'<>'temporal' THEN RETURN false; END IF;
   actual_ids := actual_ids || jsonb_build_array(column_value->'id');
  END LOOP;
  IF jsonb_array_length(actual_ids) <> (SELECT count(DISTINCT value) FROM jsonb_array_elements(actual_ids)) THEN RETURN false; END IF;

  IF output->>'kind'='kpi' THEN
   display_options := mapping->'kpi';
   IF jsonb_typeof(display_options) IS DISTINCT FROM 'object' OR mapping ? 'table'
      OR bindings - ARRAY['category','value','comparison','target'] <> '{}'::jsonb
      OR jsonb_typeof(bindings->'value') IS DISTINCT FROM 'string' OR bindings->>'value' !~ '^[A-Za-z0-9_.:-]{1,128}$'
      OR jsonb_typeof(display_options->'value_row') IS DISTINCT FROM 'string'
      OR display_options->>'value_row' NOT IN ('first','last')
      OR jsonb_typeof(display_options->'comparison_mode') IS DISTINCT FROM 'string'
      OR display_options->>'comparison_mode' NOT IN ('none','previous_row','comparison_column')
      OR jsonb_typeof(display_options->'show_delta') IS DISTINCT FROM 'boolean'
      OR jsonb_typeof(display_options->'show_percent_delta') IS DISTINCT FROM 'boolean'
      OR jsonb_typeof(display_options->'show_target_difference') IS DISTINCT FROM 'boolean'
      OR jsonb_typeof(display_options->'sparkline') IS DISTINCT FROM 'boolean'
      OR jsonb_typeof(display_options->'thresholds') IS DISTINCT FROM 'array'
      OR jsonb_array_length(display_options->'thresholds')>16 THEN RETURN false; END IF;
   IF (bindings ? 'category' AND (jsonb_typeof(bindings->'category') IS DISTINCT FROM 'string' OR bindings->>'category' !~ '^[A-Za-z0-9_.:-]{1,128}$'))
      OR (bindings ? 'comparison' AND (jsonb_typeof(bindings->'comparison') IS DISTINCT FROM 'string' OR bindings->>'comparison' !~ '^[A-Za-z0-9_.:-]{1,128}$'))
      OR (bindings ? 'target' AND (jsonb_typeof(bindings->'target') IS DISTINCT FROM 'string' OR bindings->>'target' !~ '^[A-Za-z0-9_.:-]{1,128}$')) THEN RETURN false; END IF;
   IF (display_options->>'comparison_mode'='comparison_column') IS DISTINCT FROM (bindings ? 'comparison')
      OR ((display_options->>'show_target_difference')::boolean) IS DISTINCT FROM (bindings ? 'target')
      OR (display_options->>'sparkline')::boolean AND NOT (bindings ? 'category') THEN RETURN false; END IF;
   expected_ids := '[]'::jsonb;
   IF bindings ? 'category' THEN expected_ids := expected_ids || jsonb_build_array(bindings->'category'); END IF;
   expected_ids := expected_ids || jsonb_build_array(bindings->'value');
   IF bindings ? 'comparison' THEN expected_ids := expected_ids || jsonb_build_array(bindings->'comparison'); END IF;
   IF bindings ? 'target' THEN expected_ids := expected_ids || jsonb_build_array(bindings->'target'); END IF;
   IF actual_ids <> expected_ids THEN RETURN false; END IF;
   FOR threshold_value IN SELECT value FROM jsonb_array_elements(display_options->'thresholds') LOOP
    IF jsonb_typeof(threshold_value) IS DISTINCT FROM 'object'
       OR jsonb_typeof(threshold_value->'operator') IS DISTINCT FROM 'string'
       OR threshold_value->>'operator' NOT IN ('lt','lte','gt','gte')
       OR jsonb_typeof(threshold_value->'value') IS DISTINCT FROM 'string'
       OR threshold_value->>'value' !~ '^[+-]?[0-9]+([.][0-9]+)?$'
       OR jsonb_typeof(threshold_value->'state') IS DISTINCT FROM 'string'
       OR threshold_value->>'state' !~ '^[A-Za-z0-9_.:-]{1,128}$'
       OR (threshold_value ? 'label' AND (jsonb_typeof(threshold_value->'label') IS DISTINCT FROM 'string' OR octet_length(threshold_value->>'label')>512)) THEN RETURN false; END IF;
   END LOOP;
  ELSE
   display_options := mapping->'table';
   IF jsonb_typeof(display_options) IS DISTINCT FROM 'object' OR mapping ? 'kpi'
      OR bindings - ARRAY['columns'] <> '{}'::jsonb
      OR jsonb_typeof(bindings->'columns') IS DISTINCT FROM 'array'
      OR jsonb_array_length(bindings->'columns')<>jsonb_array_length(mapping->'columns')
      OR jsonb_typeof(display_options->'columns') IS DISTINCT FROM 'array'
      OR jsonb_array_length(display_options->'columns')<>jsonb_array_length(mapping->'columns')
      OR jsonb_typeof(display_options->'page_size') IS DISTINCT FROM 'number'
      OR display_options->>'page_size' !~ '^[0-9]+$'
      OR (display_options->>'page_size')::numeric NOT BETWEEN 1 AND 1000
      OR jsonb_typeof(display_options->'show_totals') IS DISTINCT FROM 'boolean' THEN RETURN false; END IF;
   FOR column_value, ordinal IN SELECT value, ordinality FROM jsonb_array_elements(display_options->'columns') WITH ORDINALITY LOOP
    IF jsonb_typeof(column_value) IS DISTINCT FROM 'object'
       OR jsonb_typeof(column_value->'column') IS DISTINCT FROM 'string'
       OR jsonb_typeof(column_value->'visible') IS DISTINCT FROM 'boolean'
       OR jsonb_typeof((bindings->'columns')->((ordinal-1)::integer)) IS DISTINCT FROM 'string'
       OR column_value->>'column' IS DISTINCT FROM (bindings->'columns')->>((ordinal-1)::integer)
       OR column_value->>'column' IS DISTINCT FROM (mapping->'columns'->((ordinal-1)::integer))->>'id' THEN RETURN false; END IF;
   END LOOP;
   IF NOT EXISTS (SELECT 1 FROM jsonb_array_elements(display_options->'columns') item WHERE item->>'visible'='true') THEN RETURN false; END IF;
  END IF;
 END LOOP;
 RETURN true;
END;
$$;

ALTER TABLE chartworks.block_revisions ADD CONSTRAINT block_display_intent_check
 CHECK(chartworks.reporting_display_intent_valid(definition));
