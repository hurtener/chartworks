CREATE TABLE chartworks.policy_revisions (
 tenant_id text NOT NULL CHECK (tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 revision bigint NOT NULL CHECK (revision > 0),
 audit_days integer NOT NULL CHECK (audit_days BETWEEN 1 AND 3650),
 operation_hours integer NOT NULL CHECK (operation_hours BETWEEN 1 AND 8760),
 created_by text NOT NULL CHECK (created_by ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY (tenant_id, revision)
);
CREATE TABLE chartworks.policies (
 tenant_id text PRIMARY KEY CHECK (tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 current_revision bigint NOT NULL CHECK (current_revision > 0),
 FOREIGN KEY (tenant_id,current_revision) REFERENCES chartworks.policy_revisions(tenant_id,revision) DEFERRABLE INITIALLY DEFERRED
);
CREATE FUNCTION chartworks.reject_revision_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'immutable revision' USING ERRCODE = '55000'; END;
$$;
CREATE TRIGGER policy_revision_immutable BEFORE UPDATE ON chartworks.policy_revisions
 FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();
CREATE TABLE chartworks.audit_events (
 tenant_id text NOT NULL CHECK (tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 event_id text NOT NULL CHECK (event_id ~ '^[a-f0-9]{32}$'),
 actor_id text NOT NULL CHECK (actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 action text NOT NULL CHECK (action IN ('retention_policy.updated','retention.sweep')),
 resource_id text NOT NULL CHECK (resource_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY (tenant_id,event_id)
);
CREATE INDEX audit_retention ON chartworks.audit_events(tenant_id,created_at,event_id);
