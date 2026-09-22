CREATE TABLE chartworks.onboarding_runs (
 tenant_id text NOT NULL,
 onboarding_id text NOT NULL CHECK(onboarding_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 operation_key text NOT NULL CHECK(operation_key ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 4096),
 stage text NOT NULL CHECK(stage IN('connect','inspect','profile','semantic_draft','review_publish','query_block_report_proposals','complete')),
 state text NOT NULL CHECK(state IN('ready','running','attention','cancelled','complete','failed')),
 source_id text NOT NULL CHECK(source_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 context_id text NOT NULL CHECK(context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object' AND octet_length(payload::text)<=1048576),
 payload_digest text NOT NULL CHECK(payload_digest ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 deadline timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,onboarding_id),
 UNIQUE(tenant_id,actor_id,session_id,operation_key),
 CHECK(updated_at>=created_at AND deadline>created_at)
);
CREATE INDEX onboarding_private_progress ON chartworks.onboarding_runs(tenant_id,actor_id,session_id,updated_at,onboarding_id);

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''onboarding.created'',''onboarding.progressed'',''onboarding.cancelled''))',previous);
END $$;
