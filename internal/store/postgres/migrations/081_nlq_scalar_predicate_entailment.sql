-- Fresh bounded scalar equality proof. Existing receipt predicates and rows keep
-- their original meaning; no historical query is relabeled or rebound.
ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT nlq_queries_analytical_version_check;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_queries_analytical_version_check
 CHECK (analytical_version IN (0,1,2,3,4,5,6,7,8,9,10,11,12,13));

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.nlq_queries'::regclass AND conname='nlq_analytical_shape';
 ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT nlq_analytical_shape;
 EXECUTE format('ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_analytical_shape CHECK ((%s) OR (
  analytical_version=13 AND analytical IS NOT NULL AND COALESCE(
   jsonb_typeof(analytical)=''object'' AND octet_length(analytical::text)<=16384
   AND analytical->>''version''=''analytical-metrics-v13''
   AND analytical->>''intent''=''reviewed-order-limit-v1''
   AND analytical->>''query_population''=''owned-query-predicates-v1''
   AND analytical->>''scalar_entailment''=''entailed-scalar-predicates-v1''
   AND analytical->>''scalar_entailment_coverage'' ~ ''^[0-9a-f]{64}$''
   AND analytical->>''contract'' ~ ''^[0-9a-f]{64}$''
   AND analytical->>''query'' ~ ''^[0-9a-f]{64}$''
   AND jsonb_typeof(analytical->''metrics'')=''array''
   AND jsonb_array_length(analytical->''metrics'') BETWEEN 1 AND 32
   AND jsonb_typeof(analytical->''outputs'')=''array''
   AND jsonb_array_length(analytical->''outputs'')=jsonb_array_length(analytical->''metrics'')
   AND NOT analytical ? ''grouping'' AND NOT analytical ? ''completeness''
   AND analytical->>''scope'' IN (
    ''selected_metric_expression_and_population;independent_entailed_scoped_singleton_populations'',
    ''selected_metric_expression_population_and_scalar_total;independent_entailed_scoped_singleton_populations''
   ),false)
 ))',previous);
END $$;

-- New analytical evidence and protected binder custody must agree at storage.
-- Detailed occurrence/effect equality is reconstructed by the service, never
-- established by a JSON digest or this structural database predicate alone.
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_scalar_entailment_binding_shape CHECK (
 CASE WHEN analytical_version=13 THEN COALESCE(
  clarification IS NOT NULL
  AND clarification->>'schema_version'='1'
  AND length(clarification->>'base_sql')>0
  AND COALESCE(clarification->'base_parameters','null'::jsonb) IN ('null'::jsonb,'[]'::jsonb)
  AND clarification#>>'{binding,schema_version}'='6'
  AND clarification#>>'{binding,population_policy}'='entailed-scalar-predicates-v1'
  AND clarification#>>'{binding,source_binding}' ~ '^[0-9a-f]{64}$'
  AND clarification#>>'{binding,constraints}' ~ '^[0-9a-f]{64}$'
  AND clarification#>>'{binding,statement}' ~ '^[0-9a-f]{64}$'
  AND clarification#>>'{binding,validation,validated}'='true'
  AND jsonb_typeof(clarification#>'{binding,bindings}')='array'
  AND jsonb_array_length(clarification#>'{binding,bindings}') BETWEEN 2 AND 4
  AND jsonb_typeof(clarification#>'{binding,entailments}')='array'
  AND jsonb_array_length(clarification#>'{binding,entailments}') BETWEEN 1 AND 64,
  false)
 ELSE COALESCE(clarification#>>'{binding,schema_version}'<>'6',true)
  AND NOT COALESCE((clarification->'binding') ? 'entailments',false)
  AND NOT COALESCE(analytical ?| ARRAY['scalar_entailment','scalar_entailment_coverage'],false)
 END
);
