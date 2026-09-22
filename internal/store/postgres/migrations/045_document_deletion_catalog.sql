-- Explicit live-data deletion and catalog presentation support. Pengui remains
-- the only identity/authority owner; this stores tombstones and audit IDs only.
ALTER TABLE chartworks.document_heads
 ADD COLUMN deleted boolean NOT NULL DEFAULT false,
 ADD COLUMN deleted_at timestamptz,
 ADD CONSTRAINT document_deleted_shape CHECK((NOT deleted AND deleted_at IS NULL) OR (deleted AND deleted_at IS NOT NULL AND archived));

CREATE TABLE chartworks.document_deletion_tombstones (
 tenant_id text NOT NULL,
 kind text NOT NULL CHECK(kind IN('report','dashboard')),
 document_id text NOT NULL,
 deletion_key text NOT NULL CHECK(deletion_key ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 expected_version bigint NOT NULL CHECK(expected_version BETWEEN 1 AND 4096),
 deleted_version bigint NOT NULL CHECK(deleted_version=expected_version+1),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 2048),
 erased_revisions integer NOT NULL CHECK(erased_revisions BETWEEN 1 AND 256),
 erased_runs integer NOT NULL CHECK(erased_runs BETWEEN 0 AND 100000),
 erased_child_runs integer NOT NULL CHECK(erased_child_runs BETWEEN 0 AND 100000),
 erased_queries integer NOT NULL CHECK(erased_queries BETWEEN 0 AND 100000),
 retired_schedules jsonb NOT NULL CHECK(jsonb_typeof(retired_schedules)='array' AND jsonb_array_length(retired_schedules)<=1000),
 deleted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,kind,document_id),
 UNIQUE(tenant_id,actor_id,deletion_key),
 FOREIGN KEY(tenant_id,kind,document_id) REFERENCES chartworks.document_heads(tenant_id,kind,document_id)
);

-- Schedule creation/replacement and report deletion serialize on the report
-- head. A schedule cannot appear after the delete transaction has enumerated
-- and retired every matching target.
CREATE FUNCTION chartworks.protect_reporting_schedule_target() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE target_id text; target_type text; BEGIN
 target_type := NEW.request->'target'->'reporting'->>'type';
 IF target_type NOT IN('report','saved_question') THEN RETURN NEW; END IF;
 target_id := NEW.request->'target'->'reporting'->>'id';
 IF NOT EXISTS(SELECT 1 FROM chartworks.document_heads h WHERE h.tenant_id=NEW.tenant_id
  AND h.kind='report' AND h.document_id=target_id AND NOT h.archived AND NOT h.deleted FOR KEY SHARE OF h)
 THEN RAISE EXCEPTION 'reporting schedule target unavailable' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER reporting_schedule_target_guard BEFORE INSERT OR UPDATE OF request ON chartworks.job_schedules
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_reporting_schedule_target();

CREATE FUNCTION chartworks.protect_document_page_target() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM chartworks.document_heads h WHERE h.tenant_id=NEW.tenant_id
  AND h.kind='report' AND h.document_id=NEW.report_id AND NOT h.archived AND NOT h.deleted FOR KEY SHARE OF h)
 THEN RAISE EXCEPTION 'dashboard report target unavailable' USING ERRCODE='23503'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER document_page_target_guard BEFORE INSERT ON chartworks.document_page_refs
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_document_page_target();

-- Deleted document payloads may be scrubbed exactly once. Other immutable rows
-- retain their original protection.
DROP TRIGGER document_revision_immutable ON chartworks.document_revisions;
CREATE FUNCTION chartworks.protect_document_revision_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' AND EXISTS(SELECT 1 FROM chartworks.document_deletion_tombstones t
  WHERE (t.tenant_id,t.kind,t.document_id)=(OLD.tenant_id,OLD.kind,OLD.document_id))
  AND ROW(NEW.tenant_id,NEW.kind,NEW.document_id,NEW.revision,NEW.digest,NEW.actor_id,NEW.session_id,NEW.created_at)
   IS NOT DISTINCT FROM ROW(OLD.tenant_id,OLD.kind,OLD.document_id,OLD.revision,OLD.digest,OLD.actor_id,OLD.session_id,OLD.created_at)
  AND NEW.definition='{"deleted":true}'::jsonb AND NEW.origins='[]'::jsonb THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'document evidence is immutable' USING ERRCODE='55000';
END $$;
CREATE TRIGGER document_revision_immutable BEFORE UPDATE OR DELETE ON chartworks.document_revisions
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_document_revision_delete();

DROP TRIGGER document_external_ref_immutable ON chartworks.document_external_refs;
CREATE FUNCTION chartworks.protect_document_external_ref_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND EXISTS(SELECT 1 FROM chartworks.document_deletion_tombstones t
  WHERE (t.tenant_id,t.kind,t.document_id)=(OLD.tenant_id,OLD.kind,OLD.document_id)) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'document evidence is immutable' USING ERRCODE='55000';
END $$;
CREATE TRIGGER document_external_ref_immutable BEFORE UPDATE OR DELETE ON chartworks.document_external_refs
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_document_external_ref_delete();

CREATE OR REPLACE FUNCTION chartworks.protect_document_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR ROW(NEW.tenant_id,NEW.kind,NEW.document_id,NEW.created_at) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.kind,OLD.document_id,OLD.created_at)
 OR NEW.version<>OLD.version+1 OR NEW.latest_revision NOT BETWEEN OLD.latest_revision AND OLD.latest_revision+1 OR (OLD.archived AND NOT NEW.archived)
 OR OLD.deleted
 OR (NEW.deleted AND (OLD.deleted OR NOT NEW.archived OR NEW.draft_revision IS NOT NULL OR NEW.review_revision IS NOT NULL OR NEW.published_revision IS NOT NULL))
 THEN RAISE EXCEPTION 'invalid document head transition' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;

-- Retention guards also permit erasure after an exact document tombstone. The
-- parent run row remains as a bounded expired receipt and stale workers fail.
CREATE OR REPLACE FUNCTION chartworks.protect_composition_payload() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND (EXISTS(SELECT 1 FROM chartworks.composition_runs h WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id AND h.expires_at<=clock_timestamp())
 OR EXISTS(SELECT 1 FROM chartworks.composition_runs h JOIN chartworks.document_deletion_tombstones t
  ON(t.tenant_id,t.kind,t.document_id)=(h.tenant_id,h.kind,h.document_id)
  WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id)
 OR EXISTS(SELECT 1 FROM chartworks.composition_run_pages p JOIN chartworks.document_deletion_tombstones t
  ON(t.tenant_id,t.kind,t.document_id)=(p.tenant_id,'report',p.report_id)
  WHERE p.tenant_id=OLD.tenant_id AND p.operation_id=OLD.operation_id)) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'immutable composition payload' USING ERRCODE='55000';
END $$;

CREATE OR REPLACE FUNCTION chartworks.protect_composition_group() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
  IF EXISTS(SELECT 1 FROM chartworks.composition_runs h WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id AND h.expires_at<=clock_timestamp())
  OR EXISTS(SELECT 1 FROM chartworks.composition_runs h JOIN chartworks.document_deletion_tombstones t
   ON(t.tenant_id,t.kind,t.document_id)=(h.tenant_id,h.kind,h.document_id)
   WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id)
  OR EXISTS(SELECT 1 FROM chartworks.composition_run_pages p JOIN chartworks.document_deletion_tombstones t
   ON(t.tenant_id,t.kind,t.document_id)=(p.tenant_id,'report',p.report_id)
   WHERE p.tenant_id=OLD.tenant_id AND p.operation_id=OLD.operation_id) THEN RETURN OLD; END IF;
  RAISE EXCEPTION 'composition group requires retention expiry' USING ERRCODE='55000';
 END IF;
 IF ROW(NEW.tenant_id,NEW.operation_id,NEW.group_id,NEW.ordinal,NEW.kind) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.group_id,OLD.ordinal,OLD.kind)
 OR (OLD.started AND NOT NEW.started) OR (OLD.plan IS NOT NULL AND NEW.plan IS DISTINCT FROM OLD.plan)
 OR (OLD.result IS NOT NULL AND ROW(NEW.result,NEW.result_digest) IS DISTINCT FROM ROW(OLD.result,OLD.result_digest))
 THEN RAISE EXCEPTION 'immutable composition checkpoint' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;

-- A nested frozen run or dynamic query is document-owned only when its exact
-- composition root is owned by the deleted document (including a dashboard
-- root containing a deleted report page). Independently admitted runs and
-- authoring queries never match this predicate and retain their own lifecycle.
CREATE FUNCTION chartworks.deleted_composition_operation(p_tenant text, p_operation text) RETURNS boolean
LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
  SELECT 1 FROM chartworks.composition_runs c
  WHERE c.tenant_id=p_tenant AND p_operation LIKE 'composition:'||c.operation_id||':%'
  AND (EXISTS(SELECT 1 FROM chartworks.document_deletion_tombstones t
       WHERE (t.tenant_id,t.kind,t.document_id)=(c.tenant_id,c.kind,c.document_id))
   OR EXISTS(SELECT 1 FROM chartworks.composition_run_pages p JOIN chartworks.document_deletion_tombstones t
       ON(t.tenant_id,t.kind,t.document_id)=(p.tenant_id,'report',p.report_id)
       WHERE (p.tenant_id,p.operation_id)=(c.tenant_id,c.operation_id)))
 )
$$;

CREATE OR REPLACE FUNCTION chartworks.protect_nlq_query() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND chartworks.deleted_composition_operation(OLD.tenant_id,OLD.operation) THEN RETURN OLD; END IF;
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.actor_id<>OLD.actor_id OR NEW.session_id<>OLD.session_id OR NEW.query_id<>OLD.query_id OR
    NEW.parent_id IS DISTINCT FROM OLD.parent_id OR NEW.topic_id<>OLD.topic_id OR NEW.topics IS DISTINCT FROM OLD.topics OR
    NEW.topic_versions IS DISTINCT FROM OLD.topic_versions OR NEW.rule_versions IS DISTINCT FROM OLD.rule_versions OR
    NEW.template_selections IS DISTINCT FROM OLD.template_selections OR NEW.example_selection IS DISTINCT FROM OLD.example_selection OR
    NEW.context_id<>OLD.context_id OR NEW.locale<>OLD.locale OR NEW.question<>OLD.question OR NEW.route IS DISTINCT FROM OLD.route OR
    NEW.created_at<>OLD.created_at OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'nlq query immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION chartworks.protect_nlq_feedback() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND EXISTS(SELECT 1 FROM chartworks.nlq_queries q
  WHERE (q.tenant_id,q.actor_id,q.session_id,q.query_id)=(OLD.tenant_id,OLD.actor_id,OLD.session_id,OLD.query_id)
  AND chartworks.deleted_composition_operation(q.tenant_id,q.operation)) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'nlq feedback is immutable' USING ERRCODE='55000';
END $$;

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action=''document.deleted'')',previous);
END $$;
