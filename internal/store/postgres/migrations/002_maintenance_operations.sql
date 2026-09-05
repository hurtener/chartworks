CREATE TABLE chartworks.operations (
 tenant_id text NOT NULL CHECK (tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 operation_id text NOT NULL CHECK (operation_id ~ '^[a-f0-9]{32}$'),
 actor_id text NOT NULL CHECK (actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 kind text NOT NULL CHECK (kind = 'retention.sweep'),
 client_key text NOT NULL CHECK (client_key ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 request_hash text NOT NULL CHECK (request_hash ~ '^[a-f0-9]{64}$'),
 policy_revision bigint NOT NULL,
 cutoff timestamptz NOT NULL,
 batch_limit integer NOT NULL CHECK (batch_limit BETWEEN 1 AND 1000),
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded')),
 fence bigint NOT NULL DEFAULT 0 CHECK (fence >= 0),
 lease_owner text CHECK (lease_owner ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 lease_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL,
 finished_at timestamptz,
 deleted_events bigint NOT NULL DEFAULT 0 CHECK (deleted_events >= 0),
 deleted_operations bigint NOT NULL DEFAULT 0 CHECK (deleted_operations >= 0),
 PRIMARY KEY (tenant_id,operation_id),
 UNIQUE (tenant_id,actor_id,kind,client_key),
 UNIQUE (tenant_id,actor_id,operation_id),
 FOREIGN KEY (tenant_id,policy_revision) REFERENCES chartworks.policy_revisions(tenant_id,revision),
 CHECK ((lease_owner IS NULL) = (lease_until IS NULL)),
 CHECK ((status = 'running') = (lease_until IS NOT NULL)),
 CHECK ((status = 'succeeded') = (finished_at IS NOT NULL)),
 CHECK (expires_at > created_at)
);
ALTER TABLE chartworks.audit_events ADD COLUMN operation_id text;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_operation_ref
 FOREIGN KEY (tenant_id,actor_id,operation_id) REFERENCES chartworks.operations(tenant_id,actor_id,operation_id) ON DELETE RESTRICT;
CREATE INDEX operation_retention ON chartworks.operations(tenant_id,expires_at) WHERE status='succeeded';
