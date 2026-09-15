-- Reports and dashboards consume the existing request-operation lease engine.
-- Preserve the already shipped queued-pipeline/retention admission predicates.
DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.operations'::regclass AND conname='operations_kind_check';
 ALTER TABLE chartworks.operations DROP CONSTRAINT operations_kind_check;
 EXECUTE format('ALTER TABLE chartworks.operations ADD CONSTRAINT operations_kind_check CHECK ((%s) OR kind IN (''report.run'',''dashboard.run''))', previous);
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.operations'::regclass AND conname='request_manifest_shape';
 ALTER TABLE chartworks.operations DROP CONSTRAINT request_manifest_shape;
 EXECUTE format($shape$ALTER TABLE chartworks.operations ADD CONSTRAINT request_manifest_shape CHECK ((%s) OR
 (dispatch_mode='request' AND kind IN('report.run','dashboard.run') AND binding_id IS NULL AND schedule_id IS NULL AND schedule_revision IS NULL
 AND initiator_id=actor_id AND initiator_session IS NOT NULL AND initiator_session ~ '^[A-Za-z0-9_.:-]{1,128}$'
 AND manifest_hash IS NOT NULL AND manifest_hash ~ '^[a-f0-9]{64}$'
 AND request_manifest IS NOT NULL AND jsonb_typeof(request_manifest)='object' AND octet_length(request_manifest::text)<=4096
 AND request_manifest->>'kind'=kind AND due_at=created_at AND window_start=created_at AND window_end=created_at))$shape$, previous);
END $$;

CREATE TABLE chartworks.composition_runs (
 tenant_id text NOT NULL,
 operation_id text NOT NULL,
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 kind text NOT NULL CHECK(kind IN('report','dashboard')),
 document_id text NOT NULL,
 revision bigint NOT NULL,
 definition_digest text NOT NULL CHECK(definition_digest ~ '^[a-f0-9]{64}$'),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 task_hash text NOT NULL CHECK(task_hash ~ '^[a-f0-9]{64}$'),
 manifest_digest text NOT NULL CHECK(manifest_digest ~ '^[a-f0-9]{64}$'),
 private boolean NOT NULL,
 partial_policy text NOT NULL CHECK(partial_policy IN('fail_closed','allow_partial')),
 redacted boolean NOT NULL,
 total_pages integer NOT NULL CHECK(total_pages BETWEEN 0 AND 100),
 query_groups integer NOT NULL CHECK(query_groups BETWEEN 0 AND 100),
 state text NOT NULL DEFAULT 'sealed' CHECK(state IN('sealed','completed','partial','failed','cancelled','expired')),
 code text NOT NULL DEFAULT '' CHECK(code IN('','partial_report','cancelled','retention_expired')),
 complete boolean NOT NULL DEFAULT false,
 mixed_freshness boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 finished_at timestamptz,
 retained_bytes bigint NOT NULL CHECK(retained_bytes BETWEEN 0 AND 16777216),
 reserved_bytes bigint NOT NULL CHECK(reserved_bytes BETWEEN 0 AND 16777216),
 max_bytes bigint NOT NULL CHECK(max_bytes BETWEEN 1024 AND 16777216),
 PRIMARY KEY(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.operations(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '90 days'),
 CHECK(NOT private OR expires_at<=created_at+interval '7 days'),
 CHECK(retained_bytes<=max_bytes AND reserved_bytes<=max_bytes),
 CHECK(NOT complete OR state='completed'),
 CHECK(state<>'expired' OR (retained_bytes=0 AND reserved_bytes=0 AND NOT complete))
);
CREATE INDEX composition_run_expiry ON chartworks.composition_runs(tenant_id,expires_at,operation_id) WHERE state<>'expired';

-- Opaque encoded payloads are separate from authorization and summary indexes.
CREATE TABLE chartworks.composition_run_payloads (
 tenant_id text NOT NULL, operation_id text NOT NULL,
 manifest bytea NOT NULL CHECK(octet_length(manifest) BETWEEN 1 AND 16777216),
 PRIMARY KEY(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.composition_runs(tenant_id,operation_id)
);
CREATE TABLE chartworks.composition_run_groups (
 tenant_id text NOT NULL, operation_id text NOT NULL,
 group_id text NOT NULL CHECK(group_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 ordinal integer NOT NULL CHECK(ordinal BETWEEN 0 AND 99),
 kind text NOT NULL CHECK(kind IN('block','query')),
 started boolean NOT NULL DEFAULT false,
 plan bytea CHECK(plan IS NULL OR octet_length(plan) BETWEEN 1 AND 4096),
 result bytea CHECK(result IS NULL OR octet_length(result) BETWEEN 1 AND 16777216),
 result_digest text CHECK(result_digest IS NULL OR result_digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(tenant_id,operation_id,group_id),
 UNIQUE(tenant_id,operation_id,ordinal),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.composition_runs(tenant_id,operation_id),
 CHECK((result IS NULL)=(result_digest IS NULL)),
 CHECK(plan IS NULL OR (kind='query' AND started))
);
CREATE TABLE chartworks.composition_run_pages (
 tenant_id text NOT NULL, operation_id text NOT NULL,
 page_id text NOT NULL CHECK(page_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 ordinal integer NOT NULL CHECK(ordinal BETWEEN 0 AND 99),
 report_kind text NOT NULL DEFAULT 'report' CHECK(report_kind='report'),
 report_id text NOT NULL,
 revision bigint NOT NULL,
 private boolean NOT NULL,
 summary jsonb NOT NULL CHECK(jsonb_typeof(summary)='object' AND octet_length(summary::text)<=1048576),
 PRIMARY KEY(tenant_id,operation_id,page_id),
 UNIQUE(tenant_id,operation_id,ordinal),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.composition_runs(tenant_id,operation_id),
 FOREIGN KEY(tenant_id,report_kind,report_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision)
);
CREATE TABLE chartworks.composition_run_widgets (
 tenant_id text NOT NULL, operation_id text NOT NULL, page_id text NOT NULL,
 widget_id text NOT NULL CHECK(widget_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 group_id text,
 selected_outputs jsonb NOT NULL CHECK(jsonb_typeof(selected_outputs)='array' AND jsonb_array_length(selected_outputs)<=64 AND octet_length(selected_outputs::text)<=16384),
 static_payload bytea CHECK(static_payload IS NULL OR octet_length(static_payload) BETWEEN 1 AND 1048576),
 PRIMARY KEY(tenant_id,operation_id,page_id,widget_id),
 FOREIGN KEY(tenant_id,operation_id,page_id) REFERENCES chartworks.composition_run_pages(tenant_id,operation_id,page_id),
 FOREIGN KEY(tenant_id,operation_id,group_id) REFERENCES chartworks.composition_run_groups(tenant_id,operation_id,group_id)
);

-- These rows describe required signed reach, not local grants. The immutable
-- service-issued manifest supplies the requirements; only a fresh JWT satisfies them.
CREATE TABLE chartworks.composition_run_references (
 tenant_id text NOT NULL, operation_id text NOT NULL,
 page_id text NOT NULL,
 action text NOT NULL CHECK(action IN('reporting.read','reporting.execute','reporting.preview','sources.query','query.execute')),
 kind text NOT NULL CHECK(kind IN('report','dashboard','block','topic','source','dataset','execution_context')),
 permission text NOT NULL CHECK(permission IN('read','execute','preview','query','use')),
 resource_id text NOT NULL CHECK(resource_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 PRIMARY KEY(tenant_id,operation_id,page_id,action,kind,permission,resource_id),
 FOREIGN KEY(tenant_id,operation_id) REFERENCES chartworks.composition_runs(tenant_id,operation_id),
 CHECK(action<>'reporting.read' OR (kind='execution_context' AND permission='use'))
);

CREATE FUNCTION chartworks.protect_composition_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR ROW(NEW.tenant_id,NEW.operation_id,NEW.actor_id,NEW.session_id,NEW.kind,NEW.document_id,NEW.revision,NEW.definition_digest,NEW.request_hash,NEW.task_hash,NEW.manifest_digest,NEW.private,NEW.partial_policy,NEW.redacted,NEW.total_pages,NEW.query_groups,NEW.created_at,NEW.expires_at,NEW.max_bytes)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.actor_id,OLD.session_id,OLD.kind,OLD.document_id,OLD.revision,OLD.definition_digest,OLD.request_hash,OLD.task_hash,OLD.manifest_digest,OLD.private,OLD.partial_policy,OLD.redacted,OLD.total_pages,OLD.query_groups,OLD.created_at,OLD.expires_at,OLD.max_bytes)
 OR (OLD.state<>'sealed' AND NEW.state NOT IN(OLD.state,'expired')) OR (OLD.state='expired' AND NEW IS DISTINCT FROM OLD)
 THEN RAISE EXCEPTION 'immutable composition identity' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER composition_head_guard BEFORE UPDATE OR DELETE ON chartworks.composition_runs FOR EACH ROW EXECUTE FUNCTION chartworks.protect_composition_head();

CREATE FUNCTION chartworks.protect_composition_payload() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND EXISTS(SELECT 1 FROM chartworks.composition_runs h WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id AND h.expires_at<=clock_timestamp()) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'immutable composition payload' USING ERRCODE='55000';
END $$;
CREATE TRIGGER composition_payload_guard BEFORE UPDATE OR DELETE ON chartworks.composition_run_payloads FOR EACH ROW EXECUTE FUNCTION chartworks.protect_composition_payload();
CREATE TRIGGER composition_widget_guard BEFORE UPDATE OR DELETE ON chartworks.composition_run_widgets FOR EACH ROW EXECUTE FUNCTION chartworks.protect_composition_payload();
CREATE TRIGGER composition_reference_guard BEFORE UPDATE OR DELETE ON chartworks.composition_run_references FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();

CREATE FUNCTION chartworks.protect_composition_group() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
  IF EXISTS(SELECT 1 FROM chartworks.composition_runs h WHERE h.tenant_id=OLD.tenant_id AND h.operation_id=OLD.operation_id AND h.expires_at<=clock_timestamp()) THEN RETURN OLD; END IF;
  RAISE EXCEPTION 'composition group requires retention expiry' USING ERRCODE='55000';
 END IF;
 IF ROW(NEW.tenant_id,NEW.operation_id,NEW.group_id,NEW.ordinal,NEW.kind) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.group_id,OLD.ordinal,OLD.kind)
 OR (OLD.started AND NOT NEW.started) OR (OLD.plan IS NOT NULL AND NEW.plan IS DISTINCT FROM OLD.plan)
 OR (OLD.result IS NOT NULL AND ROW(NEW.result,NEW.result_digest) IS DISTINCT FROM ROW(OLD.result,OLD.result_digest))
 THEN RAISE EXCEPTION 'immutable composition checkpoint' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER composition_group_guard BEFORE UPDATE OR DELETE ON chartworks.composition_run_groups FOR EACH ROW EXECUTE FUNCTION chartworks.protect_composition_group();
CREATE FUNCTION chartworks.protect_composition_page() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR ROW(NEW.tenant_id,NEW.operation_id,NEW.page_id,NEW.ordinal,NEW.report_kind,NEW.report_id,NEW.revision,NEW.private)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.operation_id,OLD.page_id,OLD.ordinal,OLD.report_kind,OLD.report_id,OLD.revision,OLD.private)
 THEN RAISE EXCEPTION 'immutable composition page' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER composition_page_guard BEFORE UPDATE OR DELETE ON chartworks.composition_run_pages FOR EACH ROW EXECUTE FUNCTION chartworks.protect_composition_page();

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''composition.sealed'',''composition.query_started'',''composition.plan_checkpoint'',''composition.group_checkpoint'',''composition.completed'',''composition.cancelled'',''composition.expired''))', previous);
END $$;
