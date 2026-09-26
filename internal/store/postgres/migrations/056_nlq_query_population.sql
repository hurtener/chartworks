-- Forward-only: retained v0/v1/v2/v3 rows keep their original proof policy.
ALTER TABLE chartworks.nlq_queries
 DROP CONSTRAINT nlq_queries_analytical_version_check,
 DROP CONSTRAINT nlq_analytical_shape,
 ADD CONSTRAINT nlq_queries_analytical_version_check CHECK (analytical_version IN (0,1,2,3,4)),
 ADD CONSTRAINT nlq_analytical_shape CHECK (
  (analytical_version=0 AND analytical IS NULL) OR
  (analytical_version IN (1,2,3,4) AND (analytical IS NULL OR COALESCE(
   jsonb_typeof(analytical)='object' AND octet_length(analytical::text)<=16384
   AND analytical->>'contract' ~ '^[0-9a-f]{64}$'
   AND analytical->>'query' ~ '^[0-9a-f]{64}$'
   AND jsonb_typeof(analytical->'metrics')='array'
   AND jsonb_array_length(analytical->'metrics') BETWEEN 1 AND 32
   AND (NOT analytical ? 'query_population' OR
    (analytical_version=4 AND analytical->>'query_population'='owned-query-predicates-v1'))
   AND (
    (analytical_version=1 AND analytical->>'version'='analytical-metrics-v1'
     AND analytical->>'scope'='selected_metric_expression_and_population;single_base_relation'
     AND NOT analytical ? 'grouping')
    OR
    (analytical_version=2 AND analytical->>'version'='analytical-metrics-v2' AND (
     (analytical->>'scope'='selected_metric_expression_and_population;single_base_relation' AND NOT analytical ? 'grouping') OR
     (analytical->>'scope'='selected_metric_expression_population_and_grouping;single_base_relation'
      AND jsonb_typeof(analytical->'grouping')='array' AND jsonb_array_length(analytical->'grouping') BETWEEN 1 AND 16)))
    OR
    (analytical_version IN (3,4) AND analytical->>'version'=CASE analytical_version WHEN 3 THEN 'analytical-metrics-v3' ELSE 'analytical-metrics-v4' END AND (
     (analytical->>'scope'='selected_metric_expression_and_population;single_base_relation' AND NOT analytical ? 'grouping') OR
     (analytical->>'scope' IN ('selected_metric_expression_population_and_grouping;single_base_relation','selected_metric_expression_population_and_calendar_grouping;single_base_relation')
      AND jsonb_typeof(analytical->'grouping')='array' AND jsonb_array_length(analytical->'grouping') BETWEEN 1 AND 16)))
   ), false)))
 );
-- The existing immutable trigger also protects the optional query_population
-- marker; only a newly checked execution correction may refresh the query hash.
