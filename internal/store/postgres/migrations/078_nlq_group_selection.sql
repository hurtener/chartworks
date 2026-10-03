-- Separate v11 final-group-selection receipt shape; retain every prior predicate and row.
ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT nlq_queries_analytical_version_check;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_queries_analytical_version_check
 CHECK (analytical_version IN (0,1,2,3,4,5,6,7,8,9,10,11));

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.nlq_queries'::regclass AND conname='nlq_analytical_shape';
 ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT nlq_analytical_shape;
 EXECUTE format('ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_analytical_shape CHECK ((%s) OR (
  analytical_version=11 AND analytical IS NOT NULL AND COALESCE(
   jsonb_typeof(analytical)=''object'' AND octet_length(analytical::text)<=16384
   AND analytical->>''version''=''analytical-metrics-v11''
   AND analytical->>''intent''=''reviewed-order-limit-v1''
   AND analytical->>''query_population''=''owned-query-predicates-v1''
   AND analytical->>''contract'' ~ ''^[0-9a-f]{64}$''
   AND analytical->>''query'' ~ ''^[0-9a-f]{64}$''
   AND jsonb_typeof(analytical->''metrics'')=''array''
   AND jsonb_array_length(analytical->''metrics'') BETWEEN 1 AND 32
   AND jsonb_typeof(analytical->''outputs'')=''array''
   AND jsonb_array_length(analytical->''outputs'')=jsonb_array_length(analytical->''metrics'')
   AND jsonb_typeof(analytical->''grouping'')=''array''
   AND jsonb_array_length(analytical->''grouping'') BETWEEN 1 AND 16
   AND NOT analytical ? ''completeness''
   AND analytical->>''scope'' IN (
    ''selected_metric_expression_population_and_grouping;independent_selected_grouped_populations'',
    ''selected_metric_expression_population_and_calendar_grouping;independent_selected_grouped_populations''
   ),false)
 ))',previous);
END $$;
