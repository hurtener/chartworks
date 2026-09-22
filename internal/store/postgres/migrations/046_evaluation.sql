CREATE TABLE chartworks.evaluation_suites (
    tenant_id text NOT NULL, suite_id text NOT NULL, revision bigint NOT NULL CHECK (revision > 0),
    manifest_digest text NOT NULL CHECK (length(manifest_digest) = 64), manifest jsonb NOT NULL,
    state text NOT NULL CHECK (state IN ('draft','accepted','rejected')),
    author_id text NOT NULL, review jsonb, created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, suite_id, revision),
    UNIQUE (tenant_id, suite_id, revision, manifest_digest),
    CHECK ((state = 'draft' AND review IS NULL) OR (state <> 'draft' AND review IS NOT NULL))
);
CREATE TABLE chartworks.evaluation_inputs (
    tenant_id text NOT NULL, input_digest text NOT NULL CHECK(length(input_digest)=64),
    material jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(tenant_id,input_digest)
);

CREATE TABLE chartworks.evaluation_runs (
    tenant_id text NOT NULL, actor_id text NOT NULL, run_id text NOT NULL,
    suite_id text NOT NULL, suite_revision bigint NOT NULL CHECK (suite_revision > 0),
    suite_digest text NOT NULL CHECK (length(suite_digest) = 64),
    evidence_hash text CHECK (evidence_hash IS NULL OR length(evidence_hash) = 64),
    mode text CHECK (mode IS NULL OR mode IN ('fixture','live')), gate_passed boolean,
    status text NOT NULL CHECK (status IN ('running','passed','failed','cancelled','timed_out','budget_exhausted','dependency_failed')),
    cancel_requested boolean NOT NULL DEFAULT false, report jsonb, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), completed_at timestamptz,
    PRIMARY KEY (tenant_id, run_id),
    FOREIGN KEY (tenant_id, suite_id, suite_revision, suite_digest)
        REFERENCES chartworks.evaluation_suites(tenant_id, suite_id, revision, manifest_digest),
    CHECK ((status='running' AND report IS NULL AND evidence_hash IS NULL AND completed_at IS NULL) OR
           (status<>'running' AND report IS NOT NULL AND evidence_hash IS NOT NULL AND completed_at IS NOT NULL))
);

CREATE TABLE chartworks.evaluation_feedback_exports (
    tenant_id text NOT NULL, actor_id text NOT NULL, export_id text NOT NULL,
    evidence_hash text NOT NULL CHECK (length(evidence_hash)=64),
    split text NOT NULL CHECK (split='training'), manifest jsonb NOT NULL, created_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, export_id)
);

CREATE TABLE chartworks.evaluation_proposals (
    tenant_id text NOT NULL, proposal_id text NOT NULL, author_id text NOT NULL,
    proposal_digest text NOT NULL CHECK(length(proposal_digest)=64), proposal jsonb NOT NULL,
    state text NOT NULL DEFAULT 'candidate' CHECK(state IN ('candidate','approve','reject')),
    review jsonb, created_at timestamptz NOT NULL,
    PRIMARY KEY(tenant_id,proposal_id), UNIQUE(tenant_id,proposal_id,proposal_digest),
    CHECK((state='candidate' AND review IS NULL) OR (state<>'candidate' AND review IS NOT NULL))
);
CREATE TABLE chartworks.evaluation_pack_selection (
    tenant_id text PRIMARY KEY, revision bigint NOT NULL CHECK(revision>0), pack_digest text NOT NULL CHECK(length(pack_digest)=64),
    proposal_id text NOT NULL, actor_id text NOT NULL, selected_at timestamptz NOT NULL,
    FOREIGN KEY(tenant_id,proposal_id) REFERENCES chartworks.evaluation_proposals(tenant_id,proposal_id)
);
