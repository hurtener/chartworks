-- Content-free logical lookup admission. read_attempts remains the only
-- physical source journal; no option rows, search text, SQL or token are kept.
CREATE TABLE chartworks.authoring_option_operations (
 tenant_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL,
 operation_id text NOT NULL CHECK(operation_id ~ '^option:[1-9][0-9]{0,11}:[a-f0-9]{32}$'),
 target_digest text NOT NULL CHECK(target_digest ~ '^[a-f0-9]{64}$'),
 namespace_digest text NOT NULL CHECK(namespace_digest ~ '^[a-f0-9]{64}$'),
 input_digest text NOT NULL CHECK(input_digest ~ '^[a-f0-9]{64}$'),
 source_operation text NOT NULL CHECK(source_operation ~ '^authoring-option:[a-f0-9]{64}$'),
 record jsonb NOT NULL CHECK(jsonb_typeof(record)='object' AND octet_length(record::text)<=262144),
 status text NOT NULL CHECK(status IN('accepted','completed','failed','uncertain')),
 created_at timestamptz NOT NULL,
 deadline timestamptz NOT NULL CHECK(deadline>created_at AND deadline<=created_at+interval '61 seconds'),
 source_id text GENERATED ALWAYS AS(record->>'source') STORED NOT NULL,
 source_revision bigint GENERATED ALWAYS AS((record->>'source_revision')::bigint) STORED NOT NULL,
 topic_id text GENERATED ALWAYS AS(record#>>'{topic,topic}') STORED NOT NULL,
 topic_version text GENERATED ALWAYS AS(record#>>'{topic,version}') STORED NOT NULL,
 report_kind text GENERATED ALWAYS AS(CASE WHEN record#>>'{target,report,report}' IS NOT NULL THEN 'report' ELSE NULL END) STORED,
 report_id text GENERATED ALWAYS AS(record#>>'{target,report,report}') STORED,
 report_revision bigint GENERATED ALWAYS AS((record#>>'{target,report,revision}')::bigint) STORED,
 PRIMARY KEY(tenant_id,actor_id,session_id,operation_id),
 UNIQUE(tenant_id,actor_id,source_operation),
 UNIQUE(tenant_id,actor_id,operation_id),
 FOREIGN KEY(tenant_id,source_id,source_revision) REFERENCES chartworks.source_revisions(tenant_id,source_id,revision),
 FOREIGN KEY(tenant_id,topic_id,topic_version) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,report_kind,report_id,report_revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision),
 CHECK(record->>'tenant'=tenant_id AND record->>'actor'=actor_id AND record->>'session'=session_id AND record->>'operation'=operation_id AND record->>'input_digest'=input_digest AND record->>'source_operation'=source_operation AND record->>'status'=status)
);
CREATE INDEX authoring_option_liability ON chartworks.authoring_option_operations(tenant_id,actor_id,namespace_digest,status);
CREATE TABLE chartworks.authoring_option_block_refs (
 tenant_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL, operation_id text NOT NULL,
 block_id text NOT NULL, block_revision bigint NOT NULL, policy text NOT NULL CHECK(policy IN('private_preview','published')),
 PRIMARY KEY(tenant_id,actor_id,session_id,operation_id,block_id,block_revision,policy),
 FOREIGN KEY(tenant_id,actor_id,session_id,operation_id) REFERENCES chartworks.authoring_option_operations(tenant_id,actor_id,session_id,operation_id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,block_id,block_revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision)
);
CREATE FUNCTION chartworks.protect_authoring_option() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.operation_id,NEW.target_digest,NEW.namespace_digest,NEW.input_digest,NEW.source_operation,NEW.created_at,NEW.deadline)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.actor_id,OLD.session_id,OLD.operation_id,OLD.target_digest,OLD.namespace_digest,OLD.input_digest,OLD.source_operation,OLD.created_at,OLD.deadline)
 OR (NEW.record-ARRAY['status','code','execution_status','remote_state','cancel_requested','receipt']) IS DISTINCT FROM (OLD.record-ARRAY['status','code','execution_status','remote_state','cancel_requested','receipt'])
 OR (OLD.record->'receipt' IS NOT NULL AND NEW.record->'receipt' IS DISTINCT FROM OLD.record->'receipt')
 OR OLD.status IN('completed','failed')
 OR (OLD.record->>'cancel_requested')::boolean AND NOT (NEW.record->>'cancel_requested')::boolean THEN
  RAISE EXCEPTION 'immutable option operation' USING ERRCODE='23514';
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER immutable_authoring_option BEFORE UPDATE ON chartworks.authoring_option_operations FOR EACH ROW EXECUTE FUNCTION chartworks.protect_authoring_option();

-- Serialize native attempt admission with explicit reservation cancellation.
-- Other execution consumers and the existing published options path are intact.
CREATE FUNCTION chartworks.guard_authoring_option_read() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE r chartworks.authoring_option_operations%ROWTYPE; current_version text; current_digest text; current_archived boolean; pin jsonb; current_target record; BEGIN
 IF NEW.operation_id NOT LIKE 'authoring-option:%' THEN RETURN NEW; END IF;
 SELECT * INTO r FROM chartworks.authoring_option_operations p WHERE p.tenant_id=NEW.tenant_id AND p.actor_id=NEW.actor_id AND p.source_operation=NEW.operation_id FOR UPDATE;
 IF NOT FOUND OR r.session_id<>NEW.manifest->>'session' OR r.status<>'accepted'
 OR (r.record->>'cancel_requested')::boolean OR r.deadline<=clock_timestamp()
 OR to_timestamp(split_part(r.operation_id,':',2)::double precision)<clock_timestamp()-interval '5 minutes'
 OR NEW.attempt_number<>1 OR NEW.source_id<>r.source_id OR NEW.context_id<>r.record->>'context'
 OR r.record->'receipt' IS NULL OR NEW.deadline>r.deadline OR NEW.manifest->'validation' IS DISTINCT FROM r.record->'receipt'
 OR NOT (NEW.manifest->>'preview')::boolean
 OR (NEW.manifest#>>'{limits,rows}')::integer<>(r.record->>'rows')::integer
 OR (NEW.manifest#>>'{limits,rows}')::integer>200
 OR (NEW.manifest#>>'{limits,bytes}')::integer>524288 THEN
  RAISE EXCEPTION 'option admission unavailable' USING ERRCODE='23514';
 END IF;
 IF r.report_id IS NOT NULL THEN
  SELECT h.archived,h.deleted,COALESCE(h.draft_revision,0) AS draft_revision,COALESCE(h.published_revision,0) AS published_revision,v.digest,v.actor_id INTO current_target
  FROM chartworks.document_heads h JOIN chartworks.document_revisions v ON(v.tenant_id,v.kind,v.document_id)=(h.tenant_id,h.kind,h.document_id) AND v.revision=r.report_revision
  WHERE h.tenant_id=r.tenant_id AND h.kind='report' AND h.document_id=r.report_id FOR SHARE OF h;
  IF NOT FOUND OR current_target.archived OR current_target.deleted OR current_target.digest<>r.record#>>'{target,report,digest}'
   OR r.record#>>'{target,report,policy}'='private_preview' AND (current_target.actor_id<>r.actor_id OR current_target.draft_revision<>r.report_revision)
   OR r.record#>>'{target,report,policy}'='published' AND current_target.published_revision<>r.report_revision THEN
   RAISE EXCEPTION 'option report changed' USING ERRCODE='23514';
  END IF;
 ELSE
  IF EXISTS(SELECT 1 FROM chartworks.block_heads h WHERE h.tenant_id=r.tenant_id AND h.block_id=r.record#>>'{target,dataset,new_block}') THEN
   RAISE EXCEPTION 'option target changed' USING ERRCODE='23514';
  END IF;
 END IF;
 FOR pin IN SELECT value FROM jsonb_array_elements(r.record->'blocks') ORDER BY value->>'block' LOOP
  SELECT h.archived,v.digest,v.actor_id,EXISTS(SELECT 1 FROM chartworks.block_publications p WHERE(p.tenant_id,p.block_id,p.revision)=(v.tenant_id,v.block_id,v.revision)) AS published INTO current_target
  FROM chartworks.block_heads h JOIN chartworks.block_revisions v ON(v.tenant_id,v.block_id)=(h.tenant_id,h.block_id) AND v.revision=(pin->>'revision')::bigint
  WHERE h.tenant_id=r.tenant_id AND h.block_id=pin->>'block' FOR SHARE OF h;
  IF NOT FOUND OR current_target.archived OR current_target.digest<>pin->>'digest'
   OR pin->>'policy'='private_preview' AND current_target.actor_id<>r.actor_id
   OR pin->>'policy'='published' AND NOT current_target.published THEN
   RAISE EXCEPTION 'option block changed' USING ERRCODE='23514';
  END IF;
 END LOOP;
 SELECT COALESCE(h.active_version,''),h.archived,COALESCE(v.digest,'') INTO current_version,current_archived,current_digest
 FROM chartworks.topic_publication_heads h LEFT JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version)
 WHERE h.tenant_id=r.tenant_id AND h.topic_id=r.topic_id FOR UPDATE OF h;
 IF NOT FOUND OR current_archived OR current_version<>r.topic_version OR current_digest<>r.record#>>'{topic,digest}'
 OR EXISTS(SELECT 1 FROM chartworks.topic_rule_publication_heads h WHERE h.tenant_id=r.tenant_id AND h.topic_id=r.topic_id AND h.active_version IS NOT NULL) THEN
  RAISE EXCEPTION 'option semantics changed' USING ERRCODE='23514';
 END IF;
 SELECT h.current_revision,v.context_id INTO current_target FROM chartworks.sources h JOIN chartworks.source_revisions v ON(v.tenant_id,v.source_id,v.revision)=(h.tenant_id,h.source_id,h.current_revision)
 WHERE h.tenant_id=r.tenant_id AND h.source_id=r.source_id AND NOT h.deleted FOR SHARE OF h;
 IF NOT FOUND OR current_target.current_revision<>r.source_revision OR current_target.context_id<>r.record->>'context' THEN
  RAISE EXCEPTION 'option source changed' USING ERRCODE='23514';
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER authoring_option_read_start BEFORE INSERT ON chartworks.read_attempts FOR EACH ROW EXECUTE FUNCTION chartworks.guard_authoring_option_read();

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''authoring.option_reserved'',''authoring.option_finished'',''authoring.option_control''))',previous);
END $$;
