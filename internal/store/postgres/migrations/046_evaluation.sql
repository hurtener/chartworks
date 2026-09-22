CREATE TABLE chartworks.evaluation_suites (
    tenant_id text NOT NULL, actor_id text NOT NULL, suite_id text NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    manifest_digest text NOT NULL CHECK (length(manifest_digest) = 64),
    manifest jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, actor_id, suite_id, revision),
    UNIQUE (tenant_id, actor_id, suite_id, revision, manifest_digest)
);

CREATE TABLE chartworks.evaluation_runs (
    tenant_id text NOT NULL, actor_id text NOT NULL, run_id text NOT NULL,
    suite_id text NOT NULL, suite_revision bigint NOT NULL CHECK (suite_revision > 0),
    suite_digest text NOT NULL CHECK (length(suite_digest) = 64),
    evidence_hash text NOT NULL CHECK (length(evidence_hash) = 64),
    mode text NOT NULL CHECK (mode IN ('fixture','live')), gate_passed boolean NOT NULL,
    report jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, actor_id, run_id),
    FOREIGN KEY (tenant_id, actor_id, suite_id, suite_revision, suite_digest)
        REFERENCES chartworks.evaluation_suites(tenant_id, actor_id, suite_id, revision, manifest_digest)
);
