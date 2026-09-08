-- Opaque references are private data, not signed capabilities. Composite keys
-- and immutable snapshots bind every step to its original tenant/actor/session.
CREATE TABLE chartworks.byo_context_bundles (
 tenant_id text NOT NULL CHECK(tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 bundle_id text NOT NULL CHECK(bundle_id ~ '^[0-9a-f]{64}$'),
 context_id text NOT NULL CHECK(context_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 payload bytea NOT NULL CHECK(octet_length(payload) BETWEEN 1 AND 1048576),
 digest text NOT NULL CHECK(digest ~ '^[0-9a-f]{64}$'),
 max_steps integer NOT NULL CHECK(max_steps BETWEEN 1 AND 32),
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 retain_until timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,actor_id,session_id,bundle_id),
 CHECK(expires_at > created_at AND expires_at <= created_at + interval '1 hour'),
 CHECK(retain_until >= expires_at AND retain_until <= created_at + interval '7 days')
);
CREATE INDEX byo_bundle_retention ON chartworks.byo_context_bundles(tenant_id,retain_until,bundle_id);

CREATE TABLE chartworks.byo_steps (
 tenant_id text NOT NULL,
 actor_id text NOT NULL,
 session_id text NOT NULL,
 bundle_id text NOT NULL,
 operation text NOT NULL CHECK(operation ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 step integer NOT NULL CHECK(step BETWEEN 1 AND 32),
 input_digest text NOT NULL CHECK(input_digest ~ '^[0-9a-f]{64}$'),
 receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=32768),
 terminal boolean NOT NULL DEFAULT false,
 PRIMARY KEY(tenant_id,actor_id,session_id,bundle_id,operation),
 UNIQUE(tenant_id,actor_id,session_id,bundle_id,step),
 FOREIGN KEY(tenant_id,actor_id,session_id,bundle_id) REFERENCES chartworks.byo_context_bundles(tenant_id,actor_id,session_id,bundle_id) ON DELETE CASCADE
);

CREATE FUNCTION chartworks.byo_bundle_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'BYO context snapshots are immutable' USING ERRCODE='23514';
END;
$$;
CREATE TRIGGER byo_bundle_immutable BEFORE UPDATE ON chartworks.byo_context_bundles FOR EACH ROW EXECUTE FUNCTION chartworks.byo_bundle_immutable();

CREATE FUNCTION chartworks.byo_step_fence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.terminal OR NOT NEW.terminal OR
    (NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.bundle_id,NEW.operation,NEW.step,NEW.input_digest) IS DISTINCT FROM
    (OLD.tenant_id,OLD.actor_id,OLD.session_id,OLD.bundle_id,OLD.operation,OLD.step,OLD.input_digest) OR
    (NEW.receipt - ARRAY['status','code','finished_at','execution']) IS DISTINCT FROM
    (OLD.receipt - ARRAY['status','code','finished_at','execution']) THEN
  RAISE EXCEPTION 'BYO step evidence fence' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER byo_step_fence BEFORE UPDATE ON chartworks.byo_steps FOR EACH ROW EXECUTE FUNCTION chartworks.byo_step_fence();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN('rules.drafted','rules.reviewed','rules.published','rules.retired','topic.reviewed','topic.published','topic.rolled_back','topic.archived','topic.drafted','topic.health_rechecked','pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired','facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated','read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted','request.accepted','request.resumed','request.cancelled','request.succeeded','upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased','profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased','nlq.session_created','nlq.query_planned','nlq.query_executed','nlq.feedback_recorded','nlq.example_changed',
 'byo.context_created','byo.step_accepted','byo.step_finished'));
