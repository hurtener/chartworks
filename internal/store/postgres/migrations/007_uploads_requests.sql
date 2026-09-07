-- Add real request-driven targets to the existing operation and attempt engine.
-- Warehouse bytes remain in a separate managed workspace, never this metadata DB.
ALTER TABLE chartworks.operations DROP CONSTRAINT operations_kind_check;
ALTER TABLE chartworks.operations ADD CONSTRAINT operations_kind_check CHECK(kind IN('retention.sweep','upload.load','upload.erase','profile.build'));
ALTER TABLE chartworks.operations DROP CONSTRAINT operations_dispatch_mode_check;
ALTER TABLE chartworks.operations ADD CONSTRAINT operations_dispatch_mode_check CHECK(dispatch_mode IN('inline','queued','request'));
ALTER TABLE chartworks.operations ADD COLUMN request_manifest jsonb;
ALTER TABLE chartworks.operations DROP CONSTRAINT inline_operation_states;
ALTER TABLE chartworks.operations ADD CONSTRAINT inline_operation_states CHECK(dispatch_mode<>'inline' OR status IN('pending','running','succeeded','expired'));
ALTER TABLE chartworks.operations DROP CONSTRAINT queued_manifest_shape;
ALTER TABLE chartworks.operations ADD CONSTRAINT queued_manifest_shape CHECK(dispatch_mode<>'queued' OR (
 binding_id IS NOT NULL AND binding_id ~ '^[A-Za-z0-9_.-]{1,64}$' AND
 initiator_id IS NOT NULL AND initiator_id ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
 initiator_session IS NOT NULL AND initiator_session ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
 actor_id='svc:chartworks:'||binding_id AND due_at IS NOT NULL AND window_start IS NOT NULL AND window_end IS NOT NULL AND window_end=due_at AND window_start<=window_end AND
 manifest_hash IS NOT NULL AND manifest_hash ~ '^[a-f0-9]{64}$' AND
 ((schedule_id IS NULL AND schedule_revision IS NULL) OR (schedule_id IS NOT NULL AND schedule_revision IS NOT NULL AND schedule_id ~ '^[a-f0-9]{32}$' AND schedule_revision>0))));
ALTER TABLE chartworks.operations ADD CONSTRAINT request_manifest_shape CHECK(
 (dispatch_mode='request' AND kind IN('upload.load','upload.erase','profile.build') AND
  binding_id IS NULL AND schedule_id IS NULL AND schedule_revision IS NULL AND
  initiator_id=actor_id AND initiator_session IS NOT NULL AND initiator_session ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
  manifest_hash IS NOT NULL AND manifest_hash ~ '^[a-f0-9]{64}$' AND
  request_manifest IS NOT NULL AND jsonb_typeof(request_manifest)='object' AND octet_length(request_manifest::text)<=4096 AND
  request_manifest->>'kind'=kind AND due_at=created_at AND window_start=created_at AND window_end=created_at)
 OR (dispatch_mode IN('inline','queued') AND kind='retention.sweep' AND request_manifest IS NULL));
CREATE INDEX request_operation_lookup ON chartworks.operations(tenant_id,actor_id,operation_id) WHERE dispatch_mode='request';
CREATE INDEX shared_operation_active ON chartworks.operations(tenant_id,status,lease_until) WHERE dispatch_mode IN('queued','request');
CREATE FUNCTION chartworks.protect_request_manifest() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.dispatch_mode IS DISTINCT FROM OLD.dispatch_mode OR OLD.dispatch_mode='request' AND
 ROW(NEW.tenant_id,NEW.operation_id,NEW.actor_id,NEW.kind,NEW.client_key,NEW.request_hash,NEW.policy_revision,NEW.cutoff,NEW.batch_limit,NEW.binding_id,NEW.initiator_id,NEW.initiator_session,NEW.due_at,NEW.window_start,NEW.window_end,NEW.manifest_hash,NEW.max_attempts,NEW.schedule_id,NEW.schedule_revision,NEW.created_at,NEW.expires_at,NEW.request_manifest)
 IS DISTINCT FROM
 ROW(OLD.tenant_id,OLD.operation_id,OLD.actor_id,OLD.kind,OLD.client_key,OLD.request_hash,OLD.policy_revision,OLD.cutoff,OLD.batch_limit,OLD.binding_id,OLD.initiator_id,OLD.initiator_session,OLD.due_at,OLD.window_start,OLD.window_end,OLD.manifest_hash,OLD.max_attempts,OLD.schedule_id,OLD.schedule_revision,OLD.created_at,OLD.expires_at,OLD.request_manifest)
 THEN RAISE EXCEPTION 'immutable request operation' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER request_manifest_immutable BEFORE UPDATE ON chartworks.operations FOR EACH ROW EXECUTE FUNCTION chartworks.protect_request_manifest();

ALTER TABLE chartworks.sources ADD COLUMN deleted boolean NOT NULL DEFAULT false;
CREATE TABLE chartworks.uploads (
 tenant_id text NOT NULL REFERENCES chartworks.policies(tenant_id),
 source_id text NOT NULL CHECK(source_id ~ '^[A-Za-z0-9_.:-]{1,80}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 spec jsonb NOT NULL CHECK(jsonb_typeof(spec)='object' AND octet_length(spec::text)<=65536),
 spec_hash text NOT NULL CHECK(spec_hash ~ '^[a-f0-9]{64}$'),
 state text NOT NULL DEFAULT 'awaiting_data' CHECK(state IN('awaiting_data','staged','active','deleting','erased')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL CHECK(expires_at>created_at AND expires_at<=created_at+interval '7 days'),
 accounted_bytes bigint NOT NULL CHECK(accounted_bytes BETWEEN 0 AND 268435456),
 operation_id text,
 receipt jsonb CHECK(jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=8192),
 PRIMARY KEY(tenant_id,source_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id) ON DELETE SET NULL(operation_id),
 CHECK(state<>'active' OR receipt IS NOT NULL)
);
CREATE INDEX upload_expiration ON chartworks.uploads(tenant_id,expires_at) WHERE state IN('awaiting_data','staged','deleting');
CREATE FUNCTION chartworks.protect_upload_spec() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.source_id,NEW.actor_id,NEW.session_id,NEW.spec,NEW.spec_hash,NEW.created_at,NEW.expires_at)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.source_id,OLD.actor_id,OLD.session_id,OLD.spec,OLD.spec_hash,OLD.created_at,OLD.expires_at)
 OR OLD.state='erased' AND NEW.state<>'erased'
 THEN RAISE EXCEPTION 'immutable upload manifest' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER upload_spec_immutable BEFORE UPDATE ON chartworks.uploads FOR EACH ROW EXECUTE FUNCTION chartworks.protect_upload_spec();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN(
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted',
 'request.accepted','request.resumed','request.cancelled','request.succeeded',
 'upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased'
));
