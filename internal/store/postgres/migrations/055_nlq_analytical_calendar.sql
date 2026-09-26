-- Keep all v0/v1/v2 records untouched. New authoring selects v3; retained replay
-- reconstructs the original policy, never a newer calendar interpretation.
ALTER TABLE chartworks.nlq_queries
 DROP CONSTRAINT nlq_queries_analytical_version_check,
 DROP CONSTRAINT nlq_analytical_shape,
 ADD CONSTRAINT nlq_queries_analytical_version_check CHECK (analytical_version IN (0,1,2,3)),
 ADD CONSTRAINT nlq_analytical_shape CHECK (
  (analytical_version=0 AND analytical IS NULL) OR
  (analytical_version IN (1,2,3) AND (analytical IS NULL OR COALESCE(
   jsonb_typeof(analytical)='object' AND octet_length(analytical::text)<=16384
   AND analytical->>'contract' ~ '^[0-9a-f]{64}$'
   AND analytical->>'query' ~ '^[0-9a-f]{64}$'
   AND jsonb_typeof(analytical->'metrics')='array'
   AND jsonb_array_length(analytical->'metrics') BETWEEN 1 AND 32
   AND (
    (analytical_version=1 AND analytical->>'version'='analytical-metrics-v1'
     AND analytical->>'scope'='selected_metric_expression_and_population;single_base_relation'
     AND NOT analytical ? 'grouping')
    OR
    (((analytical_version=2 AND analytical->>'version'='analytical-metrics-v2') OR (analytical_version=3 AND analytical->>'version'='analytical-metrics-v3')) AND (
     (analytical->>'scope'='selected_metric_expression_and_population;single_base_relation' AND NOT analytical ? 'grouping') OR
     (analytical->>'scope'='selected_metric_expression_population_and_grouping;single_base_relation'
      AND jsonb_typeof(analytical->'grouping')='array'
      AND jsonb_array_length(analytical->'grouping') BETWEEN 1 AND 16)
    )) OR
    (analytical_version=3 AND analytical->>'version'='analytical-metrics-v3'
     AND analytical->>'scope'='selected_metric_expression_population_and_calendar_grouping;single_base_relation'
     AND jsonb_typeof(analytical->'grouping')='array'
     AND jsonb_array_length(analytical->'grouping') BETWEEN 1 AND 16)
   ), false)))
 );
-- The existing nlq_analytical_immutable trigger still forbids changing the
-- version, scope, contract or grouping. Only the query hash can be refreshed
-- after an already allowed and newly checked execution correction.
