-- Extend the existing operation ledger. This is not a second queue or an IAM database.
ALTER TABLE chartworks.operations
 ADD COLUMN dispatch_mode text NOT NULL DEFAULT 'inline' CHECK(dispatch_mode IN ('inline','queued')),
 ADD COLUMN binding_id text,
 ADD COLUMN initiator_id text,
 ADD COLUMN initiator_session text,
 ADD COLUMN due_at timestamptz,
 ADD COLUMN window_start timestamptz,
 ADD COLUMN window_end timestamptz,
 ADD COLUMN manifest_hash text,
 ADD COLUMN attempt_count integer NOT NULL DEFAULT 0 CHECK(attempt_count BETWEEN 0 AND 8),
 ADD COLUMN max_attempts integer NOT NULL DEFAULT 1 CHECK(max_attempts BETWEEN 1 AND 8),
 ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 ADD COLUMN error_code text NOT NULL DEFAULT '' CHECK(error_code IN ('','authority_blocked','attempt_failed','attempt_timeout','definition_changed','attempts_exhausted','operation_expired','cancelled')),
 ADD COLUMN schedule_id text,
 ADD COLUMN schedule_revision bigint;

-- Replace only checks that mention status; names assigned to old unnamed checks are not guessed.
DO $$ DECLARE c record; status_att smallint; BEGIN
 SELECT attnum INTO status_att FROM pg_catalog.pg_attribute WHERE attrelid='chartworks.operations'::regclass AND attname='status';
 FOR c IN SELECT conname FROM pg_catalog.pg_constraint WHERE conrelid='chartworks.operations'::regclass AND contype='c' AND status_att=ANY(conkey)
 LOOP EXECUTE format('ALTER TABLE chartworks.operations DROP CONSTRAINT %I',c.conname); END LOOP;
END $$;
ALTER TABLE chartworks.operations
 ADD CONSTRAINT operation_state CHECK(status IN ('pending','running','retry','succeeded','expired','failed','blocked','cancelled')),
 ADD CONSTRAINT operation_running_lease CHECK((status='running')=(lease_until IS NOT NULL)),
 ADD CONSTRAINT operation_terminal_time CHECK((status IN ('succeeded','expired','failed','blocked','cancelled'))=(finished_at IS NOT NULL)),
 ADD CONSTRAINT inline_operation_states CHECK(dispatch_mode='queued' OR status IN ('pending','running','succeeded','expired')),
 ADD CONSTRAINT queued_manifest_shape CHECK(dispatch_mode='inline' OR (
 binding_id IS NOT NULL AND binding_id ~ '^[A-Za-z0-9_.-]{1,64}$' AND
 initiator_id IS NOT NULL AND initiator_id ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
 initiator_session IS NOT NULL AND initiator_session ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
 actor_id='svc:chartworks:'||binding_id AND due_at IS NOT NULL AND window_start IS NOT NULL AND window_end IS NOT NULL AND window_end=due_at AND window_start<=window_end AND
 manifest_hash IS NOT NULL AND manifest_hash ~ '^[a-f0-9]{64}$' AND
 ((schedule_id IS NULL AND schedule_revision IS NULL) OR (schedule_id IS NOT NULL AND schedule_revision IS NOT NULL AND schedule_id ~ '^[a-f0-9]{32}$' AND schedule_revision>0))));

CREATE TABLE chartworks.queue_limits (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[a-f0-9]{64}$')
);
CREATE TABLE chartworks.operation_attempts (
 tenant_id text NOT NULL, operation_id text NOT NULL, fence bigint NOT NULL CHECK(fence>0),
 attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 8), owner_id text NOT NULL,
 state text NOT NULL CHECK(state IN ('acquiring','succeeded','retry','failed','blocked','cancelled','abandoned')),
 error_code text NOT NULL DEFAULT '' CHECK(error_code IN ('','authority_blocked','attempt_failed','attempt_timeout','definition_changed','cancelled','lease_lost')), started_at timestamptz NOT NULL DEFAULT clock_timestamp(), finished_at timestamptz,
 executor_id text,
 PRIMARY KEY(tenant_id,operation_id,fence), UNIQUE(tenant_id,operation_id,attempt),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id),
 CHECK((state='acquiring')=(finished_at IS NULL))
);
CREATE TABLE chartworks.job_schedules (
 tenant_id text NOT NULL REFERENCES chartworks.policies(tenant_id),
 schedule_id text NOT NULL CHECK(schedule_id ~ '^[a-f0-9]{32}$'),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), enabled boolean NOT NULL DEFAULT true,
 creator_id text NOT NULL CHECK(creator_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 creator_session text NOT NULL CHECK(creator_session ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 client_key text NOT NULL CHECK(client_key ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 request jsonb NOT NULL CHECK(jsonb_typeof(request)='object' AND octet_length(request::text)<=8192),
 previous_due timestamptz, next_due timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,schedule_id), UNIQUE(tenant_id,creator_id,client_key),
 CHECK(next_due IS NULL OR previous_due IS NOT NULL AND previous_due<next_due)
);
ALTER TABLE chartworks.operations ADD CONSTRAINT operation_schedule_ref FOREIGN KEY(tenant_id,schedule_id) REFERENCES chartworks.job_schedules(tenant_id,schedule_id);
CREATE TABLE chartworks.job_occurrences (
 tenant_id text NOT NULL, schedule_id text NOT NULL, due_at timestamptz NOT NULL,
 window_start timestamptz NOT NULL, window_end timestamptz NOT NULL,
 skipped_through timestamptz,
 disposition text NOT NULL CHECK(disposition IN ('queued','overlap_skipped','missed_skipped')),
 operation_id text,
 PRIMARY KEY(tenant_id,schedule_id,due_at),
 FOREIGN KEY(tenant_id,schedule_id) REFERENCES chartworks.job_schedules(tenant_id,schedule_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id),
 CHECK(window_start<=window_end),CHECK((disposition='queued')=(operation_id IS NOT NULL))
);
CREATE INDEX queued_eligible ON chartworks.operations(next_attempt_at,due_at,operation_id) WHERE dispatch_mode='queued' AND status IN ('pending','retry','running');
CREATE INDEX queued_tenant_active ON chartworks.operations(tenant_id,status,lease_until) WHERE dispatch_mode='queued';
CREATE INDEX scheduled_due ON chartworks.job_schedules(next_due) WHERE enabled AND next_due IS NOT NULL;

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN ('retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired'));

CREATE FUNCTION chartworks.protect_dispatch_manifest() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF OLD.dispatch_mode='queued' AND ROW(NEW.tenant_id,NEW.operation_id,NEW.actor_id,NEW.kind,NEW.client_key,NEW.request_hash,NEW.policy_revision,NEW.cutoff,NEW.batch_limit,NEW.dispatch_mode,NEW.binding_id,NEW.initiator_id,NEW.initiator_session,NEW.due_at,NEW.window_start,NEW.window_end,NEW.manifest_hash,NEW.max_attempts,NEW.schedule_id,NEW.schedule_revision,NEW.created_at,NEW.expires_at)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.actor_id,OLD.kind,OLD.client_key,OLD.request_hash,OLD.policy_revision,OLD.cutoff,OLD.batch_limit,OLD.dispatch_mode,OLD.binding_id,OLD.initiator_id,OLD.initiator_session,OLD.due_at,OLD.window_start,OLD.window_end,OLD.manifest_hash,OLD.max_attempts,OLD.schedule_id,OLD.schedule_revision,OLD.created_at,OLD.expires_at)
 THEN RAISE EXCEPTION 'immutable dispatch manifest' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER dispatch_manifest_immutable BEFORE UPDATE ON chartworks.operations FOR EACH ROW EXECUTE FUNCTION chartworks.protect_dispatch_manifest();
CREATE FUNCTION chartworks.protect_schedule_definition() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.schedule_id,NEW.creator_id,NEW.creator_session,NEW.client_key,NEW.request_hash,NEW.request,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.schedule_id,OLD.creator_id,OLD.creator_session,OLD.client_key,OLD.request_hash,OLD.request,OLD.created_at)
 THEN RAISE EXCEPTION 'immutable schedule definition' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER schedule_definition_immutable BEFORE UPDATE ON chartworks.job_schedules FOR EACH ROW EXECUTE FUNCTION chartworks.protect_schedule_definition();

CREATE UNIQUE INDEX queued_caller_idempotency ON chartworks.operations(tenant_id,initiator_id,client_key) WHERE dispatch_mode='queued';
