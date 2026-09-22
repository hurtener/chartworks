CREATE TABLE chartworks.migration_batches (
 tenant_id text NOT NULL,
 batch_id text NOT NULL,
 cohort_id text NOT NULL,
 manifest_digest text NOT NULL CHECK(length(manifest_digest)=64),
 manifest jsonb NOT NULL,
 plan jsonb NOT NULL,
 state text NOT NULL CHECK(state IN ('importing','complete','erased')),
 revision bigint NOT NULL CHECK(revision>0),
 applied integer NOT NULL DEFAULT 0 CHECK(applied>=0),
 quarantined integer NOT NULL DEFAULT 0 CHECK(quarantined>=0),
 total integer NOT NULL CHECK(total>0),
 next_ref text NOT NULL DEFAULT '',
 actor_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,batch_id),
 UNIQUE(tenant_id,cohort_id,manifest_digest)
);

CREATE TABLE chartworks.migration_checkpoints (
 tenant_id text NOT NULL,
 batch_id text NOT NULL,
 sequence integer NOT NULL CHECK(sequence>=0),
 external_ref text NOT NULL,
 kind text NOT NULL,
 action text NOT NULL,
 destination_result text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,batch_id,external_ref),
 UNIQUE(tenant_id,batch_id,sequence),
 FOREIGN KEY(tenant_id,batch_id) REFERENCES chartworks.migration_batches(tenant_id,batch_id) ON DELETE CASCADE
);

CREATE TABLE chartworks.migration_external_refs (
 tenant_id text NOT NULL,
 kind text NOT NULL,
 external_ref text NOT NULL,
 manifest_digest text NOT NULL CHECK(length(manifest_digest)=64),
 source_revision bigint NOT NULL CHECK(source_revision>0),
 object_digest text NOT NULL CHECK(length(object_digest)=64),
 destination_result text NOT NULL,
 tombstoned boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,kind,external_ref)
);

-- Reserve stable source coordinates before any owning adapter is invoked. The
-- committed reservation also fences concurrent batches with different mappings.
CREATE TABLE chartworks.migration_ref_reservations (
 tenant_id text NOT NULL,
 kind text NOT NULL,
 external_ref text NOT NULL,
 manifest_digest text NOT NULL CHECK(length(manifest_digest)=64),
 source_revision bigint NOT NULL CHECK(source_revision>0),
 object_digest text NOT NULL CHECK(length(object_digest)=64),
 destination_mapping text NOT NULL,
 tombstoned boolean NOT NULL DEFAULT false,
 PRIMARY KEY(tenant_id,kind,external_ref)
);

CREATE TABLE chartworks.migration_cutovers (
 tenant_id text NOT NULL,
 cohort_id text NOT NULL,
 batch_id text NOT NULL,
 route text NOT NULL,
 previous_route text NOT NULL DEFAULT '',
 state text NOT NULL CHECK(state IN ('active','rolled_back')),
 generation bigint NOT NULL CHECK(generation>0),
 boundary jsonb NOT NULL,
 irreversible_effects jsonb NOT NULL DEFAULT '[]'::jsonb,
 operator_reference text NOT NULL,
 actor_id text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,cohort_id),
 FOREIGN KEY(tenant_id,batch_id) REFERENCES chartworks.migration_batches(tenant_id,batch_id)
);

CREATE TABLE chartworks.migration_cutover_events (
 tenant_id text NOT NULL,
 cohort_id text NOT NULL,
 generation bigint NOT NULL,
 event text NOT NULL CHECK(event IN ('cutover','rollback')),
 record jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,cohort_id,generation)
);

CREATE TABLE chartworks.migration_schedule_routes (
 tenant_id text NOT NULL,
 cohort_id text NOT NULL,
 stream_id text NOT NULL,
 route text NOT NULL,
 schedule_id text NOT NULL,
 schedule_revision bigint NOT NULL CHECK(schedule_revision>0),
 PRIMARY KEY(tenant_id,cohort_id,route),
 UNIQUE(tenant_id,schedule_id),
 FOREIGN KEY(tenant_id,cohort_id) REFERENCES chartworks.migration_cutovers(tenant_id,cohort_id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,schedule_id) REFERENCES chartworks.job_schedules(tenant_id,schedule_id)
);

CREATE TABLE chartworks.migration_occurrence_admissions (
 tenant_id text NOT NULL,
 stream_id text NOT NULL,
 due_at timestamptz NOT NULL,
 cohort_id text NOT NULL,
 generation bigint NOT NULL CHECK(generation>0),
 schedule_id text NOT NULL,
 PRIMARY KEY(tenant_id,stream_id,due_at)
);

CREATE FUNCTION chartworks.immutable_migration_manifest() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.manifest IS DISTINCT FROM OLD.manifest OR NEW.manifest_digest IS DISTINCT FROM OLD.manifest_digest OR NEW.plan IS DISTINCT FROM OLD.plan OR NEW.cohort_id IS DISTINCT FROM OLD.cohort_id OR NEW.total IS DISTINCT FROM OLD.total THEN
  RAISE EXCEPTION 'immutable migration manifest' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

CREATE TRIGGER migration_manifest_immutable BEFORE UPDATE ON chartworks.migration_batches
FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_migration_manifest();

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''migration.started'',''migration.checkpointed'',''migration.cutover'',''migration.rolled_back'',''migration.erased''))',previous);
END $$;

REVOKE ALL ON chartworks.migration_batches,chartworks.migration_checkpoints,chartworks.migration_external_refs,chartworks.migration_ref_reservations,chartworks.migration_cutovers,chartworks.migration_cutover_events,chartworks.migration_schedule_routes,chartworks.migration_occurrence_admissions FROM PUBLIC;

-- The reporting target guard applies only to reporting schedules. SQL NULL from
-- maintenance/pipeline payloads must take the non-reporting branch explicitly.
CREATE OR REPLACE FUNCTION chartworks.protect_reporting_schedule_target() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE target_id text; target_type text; BEGIN
 target_type := NEW.request->'target'->'reporting'->>'type';
 IF target_type IS NULL OR target_type NOT IN('report','saved_question') THEN RETURN NEW; END IF;
 target_id := NEW.request->'target'->'reporting'->>'id';
 IF NOT EXISTS(SELECT 1 FROM chartworks.document_heads h WHERE h.tenant_id=NEW.tenant_id
  AND h.kind='report' AND h.document_id=target_id AND NOT h.archived AND NOT h.deleted FOR KEY SHARE OF h)
 THEN RAISE EXCEPTION 'reporting schedule target unavailable' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
