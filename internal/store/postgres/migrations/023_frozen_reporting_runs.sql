-- Frozen reporting is a consumer of the existing request operation ledger.
-- No additional scheduler, authorization registry or bearer storage is created.
ALTER TABLE chartworks.operations DROP CONSTRAINT operations_kind_check;
ALTER TABLE chartworks.operations ADD CONSTRAINT operations_kind_check CHECK(kind IN('retention.sweep','upload.load','upload.erase','profile.build','pipeline.run','reporting.run'));
ALTER TABLE chartworks.operations DROP CONSTRAINT request_manifest_shape;
ALTER TABLE chartworks.operations ADD CONSTRAINT request_manifest_shape CHECK(
 (dispatch_mode='request' AND kind IN('upload.load','upload.erase','profile.build','pipeline.run','reporting.run') AND
  binding_id IS NULL AND schedule_id IS NULL AND schedule_revision IS NULL AND
  initiator_id=actor_id AND initiator_session IS NOT NULL AND initiator_session ~ '^[A-Za-z0-9_.:-]{1,128}$' AND
  manifest_hash IS NOT NULL AND manifest_hash ~ '^[a-f0-9]{64}$' AND
  request_manifest IS NOT NULL AND jsonb_typeof(request_manifest)='object' AND octet_length(request_manifest::text)<=4096 AND
  request_manifest->>'kind'=kind AND due_at=created_at AND window_start=created_at AND window_end=created_at)
 OR (dispatch_mode IN('inline','queued') AND kind='retention.sweep' AND request_manifest IS NULL));

CREATE TABLE chartworks.frozen_runs (
 tenant_id text NOT NULL,
 operation_id text NOT NULL,
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 block_id text NOT NULL,
 revision bigint NOT NULL,
 revision_digest text NOT NULL CHECK(revision_digest ~ '^[a-f0-9]{64}$'),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 task_hash text NOT NULL CHECK(task_hash ~ '^[a-f0-9]{64}$'),
 manifest_digest text NOT NULL CHECK(manifest_digest ~ '^[a-f0-9]{64}$'),
 reuse_key text NOT NULL CHECK(reuse_key ~ '^[a-f0-9]{64}$'),
 private boolean NOT NULL,
 source_id text NOT NULL CHECK(source_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 context_id text NOT NULL CHECK(context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 partition_digest text NOT NULL CHECK(partition_digest ~ '^[a-f0-9]{64}$'),
 locale text NOT NULL CHECK(length(locale) BETWEEN 1 AND 64),
 timezone text NOT NULL CHECK(length(timezone) BETWEEN 1 AND 256),
 frozen_version text NOT NULL CHECK(frozen_version='frozen-block-run-v1'),
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 observed_at timestamptz,
 finished_at timestamptz,
 state text NOT NULL DEFAULT 'sealed' CHECK(state IN('sealed','normalized','succeeded','partial','failed','expired')),
 code text NOT NULL DEFAULT '' CHECK(code IN('','output_failed','retention_expired')),
 reused_from text,
 retained_bytes bigint NOT NULL CHECK(retained_bytes BETWEEN 0 AND 67108864),
 reserved_bytes bigint NOT NULL CHECK(reserved_bytes BETWEEN 0 AND 67108864),
 max_artifact_bytes bigint NOT NULL CHECK(max_artifact_bytes BETWEEN 1024 AND 67108864),
 reserved_calls integer NOT NULL DEFAULT 0 CHECK(reserved_calls BETWEEN 0 AND 8),
 reserved_tokens integer NOT NULL DEFAULT 0 CHECK(reserved_tokens BETWEEN 0 AND 131072),
 PRIMARY KEY(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision),
 FOREIGN KEY(tenant_id,reused_from) REFERENCES chartworks.frozen_runs(tenant_id,operation_id),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '90 days'),
 CHECK(NOT private OR expires_at<=created_at+interval '7 days'),
 CHECK(retained_bytes<=max_artifact_bytes AND reserved_bytes<=max_artifact_bytes),
 CHECK(state<>'expired' OR (retained_bytes=0 AND reserved_bytes=0)),
 CHECK(reused_from IS NULL OR reused_from<>operation_id)
);
CREATE INDEX frozen_run_expiry ON chartworks.frozen_runs(tenant_id,expires_at,operation_id) WHERE state<>'expired';
CREATE INDEX frozen_run_reuse ON chartworks.frozen_runs(tenant_id,reuse_key,observed_at DESC,operation_id) WHERE state='succeeded';

-- Exact encoded bytes make hashes and byte budgets independent of jsonb's
-- whitespace/canonicalization. Eligibility metadata is queried before payloads.
CREATE TABLE chartworks.frozen_run_payloads (
 tenant_id text NOT NULL, operation_id text NOT NULL,
 manifest bytea NOT NULL CHECK(octet_length(manifest) BETWEEN 1 AND 4194304),
 result bytea CHECK(octet_length(result) BETWEEN 1 AND 4194304),
 result_digest text CHECK(result_digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.frozen_runs(tenant_id,operation_id),
 CHECK((result IS NULL)=(result_digest IS NULL))
);
CREATE TABLE chartworks.frozen_run_outputs (
 tenant_id text NOT NULL, operation_id text NOT NULL,
 output_id text NOT NULL CHECK(output_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 ordinal integer NOT NULL CHECK(ordinal BETWEEN 0 AND 31),
 kind text NOT NULL CHECK(length(kind) BETWEEN 1 AND 32),
 state text NOT NULL CHECK(state IN('indeterminate','succeeded','failed')),
 payload bytea NOT NULL CHECK(octet_length(payload) BETWEEN 1 AND 67108864),
 PRIMARY KEY(tenant_id,operation_id,output_id),
 UNIQUE(tenant_id,operation_id,ordinal),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.frozen_runs(tenant_id,operation_id)
);
-- Copies of actual content-free journal receipts outlive the read journal's
-- independent retention. There is deliberately no permanent read_attempts FK.
CREATE TABLE chartworks.frozen_run_attempts (
 tenant_id text NOT NULL, operation_id text NOT NULL,
 attempt_number integer NOT NULL CHECK(attempt_number BETWEEN 1 AND 3),
 receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=131072),
 PRIMARY KEY(tenant_id,operation_id,attempt_number),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.frozen_runs(tenant_id,operation_id)
);

CREATE FUNCTION chartworks.protect_frozen_run() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.operation_id,NEW.actor_id,NEW.session_id,NEW.block_id,NEW.revision,NEW.revision_digest,NEW.request_hash,NEW.task_hash,NEW.manifest_digest,NEW.reuse_key,NEW.private,NEW.source_id,NEW.context_id,NEW.partition_digest,NEW.locale,NEW.timezone,NEW.frozen_version,NEW.created_at,NEW.expires_at,NEW.max_artifact_bytes)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.actor_id,OLD.session_id,OLD.block_id,OLD.revision,OLD.revision_digest,OLD.request_hash,OLD.task_hash,OLD.manifest_digest,OLD.reuse_key,OLD.private,OLD.source_id,OLD.context_id,OLD.partition_digest,OLD.locale,OLD.timezone,OLD.frozen_version,OLD.created_at,OLD.expires_at,OLD.max_artifact_bytes)
 THEN RAISE EXCEPTION 'frozen run manifest is immutable' USING ERRCODE='23514'; END IF;
 IF OLD.state IN('succeeded','partial','failed','expired') AND NEW.state<>OLD.state AND NEW.state<>'expired'
 THEN RAISE EXCEPTION 'terminal frozen run cannot resume' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER frozen_run_immutable BEFORE UPDATE ON chartworks.frozen_runs FOR EACH ROW EXECUTE FUNCTION chartworks.protect_frozen_run();
CREATE FUNCTION chartworks.protect_frozen_payload() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.operation_id,NEW.manifest) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.manifest)
 OR (OLD.result IS NOT NULL AND ROW(NEW.result,NEW.result_digest) IS DISTINCT FROM ROW(OLD.result,OLD.result_digest))
 THEN RAISE EXCEPTION 'frozen payload is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER frozen_payload_immutable BEFORE UPDATE ON chartworks.frozen_run_payloads FOR EACH ROW EXECUTE FUNCTION chartworks.protect_frozen_payload();
CREATE FUNCTION chartworks.protect_frozen_output() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.operation_id,NEW.output_id,NEW.ordinal,NEW.kind) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.output_id,OLD.ordinal,OLD.kind)
 OR (OLD.state<>'indeterminate' AND NEW.payload IS DISTINCT FROM OLD.payload)
 OR (OLD.state='indeterminate' AND (OLD.kind<>'narrative' OR NEW.state NOT IN('succeeded','failed')))
 THEN RAISE EXCEPTION 'frozen output is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER frozen_output_immutable BEFORE UPDATE ON chartworks.frozen_run_outputs FOR EACH ROW EXECUTE FUNCTION chartworks.protect_frozen_output();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN(
 'rules.drafted','rules.reviewed','rules.published','rules.retired','topic.reviewed','topic.published','topic.rolled_back','topic.archived','topic.drafted','topic.health_rechecked','pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired','facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated','read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted','request.accepted','request.resumed','request.cancelled','request.succeeded','upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased','profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased','nlq.session_created','nlq.query_planned','nlq.query_executed','nlq.feedback_recorded','nlq.example_changed','byo.context_created','byo.step_accepted','byo.step_finished',
 'block.created','block.captured','block.edited','block.restored','block.renamed','block.parameterized','block.validated','block.published','block.certified','block.withdrawn','block.rejected','block.archived','block.health_checked',
 'reporting.run_sealed','reporting.query_checkpoint','reporting.output_started','reporting.output_checkpoint','reporting.run_completed','reporting.run_reused','reporting.run_cancelled','reporting.artifact_expired'));
