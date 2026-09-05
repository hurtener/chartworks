-- Technical source/context metadata only. Warehouse secrets remain operator references.
CREATE TABLE chartworks.sources (
 tenant_id text NOT NULL CHECK(tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 source_id text NOT NULL CHECK(source_id ~ '^[A-Za-z0-9_.:-]{1,80}$'),
 current_revision bigint NOT NULL CHECK(current_revision>0 AND current_revision<4611686018427387904),
 PRIMARY KEY(tenant_id,source_id)
);
CREATE TABLE chartworks.source_revisions (
 tenant_id text NOT NULL,
 source_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision>0 AND revision<4611686018427387904),
 context_id text NOT NULL CHECK(context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 name text NOT NULL CHECK(octet_length(name) BETWEEN 1 AND 128),
 connection_alias text NOT NULL CHECK(connection_alias ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 binding jsonb NOT NULL CHECK(jsonb_typeof(binding)='object' AND octet_length(binding::text)<=1048576),
 created_by text NOT NULL CHECK(created_by ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,source_id,revision),
 UNIQUE(tenant_id,context_id),
 CHECK(context_id=source_id||':v'||revision::text),
 CHECK(binding->>'tenant'=tenant_id AND binding->>'source'=source_id AND binding->>'context'=context_id AND binding->>'dialect'='postgres' AND (binding->>'revision')::bigint=revision)
);
ALTER TABLE chartworks.sources ADD CONSTRAINT source_revision_reference FOREIGN KEY(tenant_id,source_id,current_revision) REFERENCES chartworks.source_revisions(tenant_id,source_id,revision) DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER immutable_source_revision BEFORE UPDATE ON chartworks.source_revisions FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN (
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased',
 'source.created','source.rotated'
));
