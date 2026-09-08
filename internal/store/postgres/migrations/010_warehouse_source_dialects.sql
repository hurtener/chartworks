ALTER TABLE chartworks.source_revisions DROP CONSTRAINT source_revisions_check1;
ALTER TABLE chartworks.source_revisions ADD CONSTRAINT source_revisions_identity_check CHECK(
 binding ?& ARRAY['tenant','source','context','dialect','revision']
 AND binding->>'tenant'=tenant_id
 AND binding->>'source'=source_id
 AND binding->>'context'=context_id
 AND binding->>'dialect' IN ('postgres','mysql','sqlserver','bigquery','snowflake','databricks')
 AND (binding->>'revision')::bigint=revision
);
