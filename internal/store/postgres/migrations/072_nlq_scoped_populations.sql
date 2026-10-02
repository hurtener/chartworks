-- V9 adds explicit fact-owned singleton placement and reviewed ordinary
-- known-amount completeness with proof-issued output ordinals. V0-v8 receipt
-- predicates are retained verbatim through the current constraint expression.
ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT nlq_queries_analytical_version_check;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_queries_analytical_version_check
 CHECK (analytical_version IN (0,1,2,3,4,5,6,7,8,9));

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.nlq_queries'::regclass AND conname='nlq_analytical_shape';
 ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT nlq_analytical_shape;
 EXECUTE format('ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_analytical_shape CHECK ((%s) OR (
  analytical_version=9 AND (analytical IS NULL OR COALESCE(
   jsonb_typeof(analytical)=''object'' AND octet_length(analytical::text)<=16384
   AND analytical->>''version''=''analytical-metrics-v9''
   AND analytical->>''intent''=''reviewed-order-limit-v1''
   AND analytical->>''query_population''=''owned-query-predicates-v1''
   AND analytical->>''contract'' ~ ''^[0-9a-f]{64}$''
   AND analytical->>''query'' ~ ''^[0-9a-f]{64}$''
   AND jsonb_typeof(analytical->''metrics'')=''array''
   AND jsonb_array_length(analytical->''metrics'') BETWEEN 1 AND 32
   AND jsonb_typeof(analytical->''outputs'')=''array''
   AND jsonb_array_length(analytical->''outputs'')=jsonb_array_length(analytical->''metrics'')
   AND ((NOT analytical ? ''grouping'' AND NOT analytical ? ''completeness''
   AND analytical->>''scope'' IN (
    ''selected_metric_expression_and_population;independent_scoped_singleton_populations'',
    ''selected_metric_expression_population_and_scalar_total;independent_scoped_singleton_populations''
   )) OR (
    jsonb_typeof(analytical->''completeness'')=''object''
    AND analytical->''completeness''->>''policy''=''reviewed-known-amount-outputs-v1''
    AND jsonb_typeof(analytical->''completeness''->''obligations'')=''array''
    AND jsonb_array_length(analytical->''completeness''->''obligations'') BETWEEN 1 AND 32
    AND analytical->>''scope'' IN (
     ''selected_metric_expression_and_population;single_base_relation;reviewed_known_amount_completeness'',
     ''selected_metric_expression_population_and_scalar_total;single_base_relation;reviewed_known_amount_completeness'',
     ''selected_metric_expression_population_and_grouping;single_base_relation;reviewed_known_amount_completeness'',
     ''selected_metric_expression_population_and_calendar_grouping;single_base_relation;reviewed_known_amount_completeness'',
     ''selected_metric_expression_and_population;physically_unique_reviewed_joins;reviewed_known_amount_completeness'',
     ''selected_metric_expression_population_and_scalar_total;physically_unique_reviewed_joins;reviewed_known_amount_completeness'',
     ''selected_metric_expression_population_and_grouping;physically_unique_reviewed_joins;reviewed_known_amount_completeness'',
     ''selected_metric_expression_population_and_calendar_grouping;physically_unique_reviewed_joins;reviewed_known_amount_completeness''
    )
   )),false))
 ))',previous);
END $$;
