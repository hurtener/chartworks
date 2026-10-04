-- Fresh preparation admission is versioned. Existing custody and hashes are not
-- rewritten; ambiguous legacy liability remains retained until positively proved.
ALTER TABLE chartworks.authoring_preparations
 ADD COLUMN admission_guarded boolean NOT NULL DEFAULT false,
 ADD COLUMN read_receipt jsonb,
 ADD COLUMN admitted_attempt_id text,
 ADD COLUMN admitted_manifest jsonb,
 ADD COLUMN settlement jsonb,
 ADD COLUMN settled_at timestamptz,
 ADD COLUMN consumed_at timestamptz;
ALTER TABLE chartworks.authoring_preparations ADD CONSTRAINT preparation_settlement_shape CHECK (
 (settlement IS NULL)=(settled_at IS NULL)
 AND (settlement IS NULL OR jsonb_typeof(settlement)='object' AND octet_length(settlement::text)<=4096
  AND settlement ?& ARRAY['kind','status','remote_state','finished_at']
  AND settlement->>'kind' IN('attempt','not_issued') AND settlement->>'remote_state' IN('stopped','not_issued')
  AND settlement->>'status' IN('succeeded','empty','truncated','cancelled','timed_out','failed','interrupted')
  AND (settlement->>'kind'<>'attempt' OR settlement ?& ARRAY['attempt','manifest']))
 AND (read_receipt IS NULL OR jsonb_typeof(read_receipt)='object' AND octet_length(read_receipt::text)<=65536)
 AND (admitted_manifest IS NULL OR jsonb_typeof(admitted_manifest)='object' AND octet_length(admitted_manifest::text)<=65536)
 AND (admitted_attempt_id IS NULL)=(admitted_manifest IS NULL));
CREATE INDEX preparation_retention_candidates ON chartworks.authoring_preparations(tenant_id,actor_id,settled_at,preparation_id)
 WHERE status IN('prepared','failed','consumed') AND settlement IS NOT NULL;
CREATE INDEX preparation_target_liability ON chartworks.authoring_preparations(tenant_id,actor_id,target_id);

CREATE FUNCTION chartworks.preparation_key_time(operation text) RETURNS timestamptz LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE WHEN operation ~ '^prepare:[1-9][0-9]{0,11}:[a-f0-9]{32}$' THEN to_timestamp(split_part(operation,':',2)::double precision) END
$$;
CREATE FUNCTION chartworks.admit_authoring_preparation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.record#>>'{request,operation_version}' IS DISTINCT FROM 'prepare-v1'
 OR chartworks.preparation_key_time(NEW.operation_id) IS NULL THEN
  RAISE EXCEPTION 'preparation contract unavailable' USING ERRCODE='CW001';
 END IF;
 IF chartworks.preparation_key_time(NEW.operation_id)<clock_timestamp()-interval '5 minutes'
 OR chartworks.preparation_key_time(NEW.operation_id)>clock_timestamp()+interval '30 seconds' THEN
  RAISE EXCEPTION 'preparation operation expired' USING ERRCODE='CW002';
 END IF;
 IF NEW.status<>'accepted' OR NEW.settlement IS NOT NULL OR NEW.settled_at IS NOT NULL OR NEW.consumed_at IS NOT NULL
 OR NEW.read_receipt IS NOT NULL OR NEW.admitted_attempt_id IS NOT NULL OR NEW.admitted_manifest IS NOT NULL THEN
  RAISE EXCEPTION 'preparation admission invalid' USING ERRCODE='23514';
 END IF;
 IF EXISTS(SELECT 1 FROM chartworks.authoring_preparation_consumed c WHERE c.tenant_id=NEW.tenant_id AND c.actor_id=NEW.actor_id AND c.session_id=NEW.session_id AND c.operation_id=NEW.operation_id) THEN
  RAISE EXCEPTION 'preparation operation already consumed' USING ERRCODE='23505';
 END IF;
 NEW.admission_guarded=true; RETURN NEW;
END $$;
CREATE TRIGGER authoring_preparation_admission BEFORE INSERT ON chartworks.authoring_preparations FOR EACH ROW EXECUTE FUNCTION chartworks.admit_authoring_preparation();

CREATE OR REPLACE FUNCTION chartworks.protect_authoring_preparation() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE a chartworks.read_attempts%ROWTYPE; expected_manifest jsonb; BEGIN
 IF ROW(NEW.tenant_id,NEW.preparation_id,NEW.actor_id,NEW.session_id,NEW.target_id,NEW.operation_id,NEW.input_digest,NEW.source_operation,NEW.created_at,NEW.deadline,NEW.expires_at,NEW.admission_guarded)
 IS DISTINCT FROM ROW(OLD.tenant_id,OLD.preparation_id,OLD.actor_id,OLD.session_id,OLD.target_id,OLD.operation_id,OLD.input_digest,OLD.source_operation,OLD.created_at,OLD.deadline,OLD.expires_at,OLD.admission_guarded)
 OR (NEW.record-ARRAY['status','code','digest','revision','attempt']) IS DISTINCT FROM (OLD.record-ARRAY['status','code','digest','revision','attempt'])
 OR NOT (OLD.status=NEW.status OR OLD.status='accepted' AND NEW.status IN('prepared','failed','uncertain') OR OLD.status='uncertain' AND NEW.status='failed' OR OLD.status='prepared' AND NEW.status='consumed')
 OR OLD.status IN('prepared','consumed','failed') AND (NEW.record-'status') IS DISTINCT FROM (OLD.record-'status')
 OR OLD.read_receipt IS NOT NULL AND NEW.read_receipt IS DISTINCT FROM OLD.read_receipt
 OR OLD.admitted_attempt_id IS NOT NULL AND ROW(NEW.admitted_attempt_id,NEW.admitted_manifest) IS DISTINCT FROM ROW(OLD.admitted_attempt_id,OLD.admitted_manifest)
 OR OLD.settlement IS NOT NULL AND ROW(NEW.settlement,NEW.settled_at) IS DISTINCT FROM ROW(OLD.settlement,OLD.settled_at)
 OR OLD.consumed_at IS NOT NULL AND NEW.consumed_at IS DISTINCT FROM OLD.consumed_at THEN
  RAISE EXCEPTION 'immutable chart preparation' USING ERRCODE='23514';
 END IF;
 IF OLD.settlement IS NULL AND NEW.settlement IS NOT NULL THEN
  IF NEW.settlement->>'kind'='not_issued' THEN
   IF NOT OLD.admission_guarded OR OLD.admitted_attempt_id IS NOT NULL OR NEW.status<>'failed'
    OR NEW.settlement->>'status'<>'failed' OR NEW.settlement->>'remote_state'<>'not_issued'
    OR EXISTS(SELECT 1 FROM chartworks.read_attempts pending WHERE pending.tenant_id=OLD.tenant_id AND pending.actor_id=OLD.actor_id AND pending.operation_id=OLD.source_operation) THEN
    RAISE EXCEPTION 'preparation dispatch not disproved' USING ERRCODE='23514';
   END IF;
  ELSIF NEW.settlement->>'kind'='attempt' THEN
   expected_manifest=COALESCE(OLD.admitted_manifest,OLD.record#>'{attempt,manifest}');
   SELECT * INTO a FROM chartworks.read_attempts WHERE tenant_id=OLD.tenant_id AND actor_id=OLD.actor_id
    AND attempt_id=NEW.settlement->>'attempt' AND operation_id=OLD.source_operation AND attempt_number=1;
   IF NOT FOUND OR expected_manifest IS NULL OR a.manifest IS DISTINCT FROM expected_manifest
    OR a.manifest_hash IS DISTINCT FROM NEW.settlement->>'manifest'
    OR a.manifest->>'session' IS DISTINCT FROM OLD.session_id OR a.source_id<>OLD.source_id OR a.context_id<>OLD.record#>>'{binding,context}'
    OR a.status NOT IN('succeeded','empty','truncated','cancelled','timed_out','failed','interrupted')
    OR a.finished_at IS NULL OR a.remote_state NOT IN('stopped','not_issued')
    OR a.status IS DISTINCT FROM NEW.settlement->>'status' OR a.remote_state IS DISTINCT FROM NEW.settlement->>'remote_state'
    OR a.finished_at IS DISTINCT FROM (NEW.settlement->>'finished_at')::timestamptz
    OR a.remote_state='not_issued' AND a.remote_query IS NOT NULL THEN
    RAISE EXCEPTION 'preparation settlement not proved' USING ERRCODE='23514';
   END IF;
  ELSE RAISE EXCEPTION 'invalid preparation settlement' USING ERRCODE='23514';
  END IF;
  NEW.settled_at=clock_timestamp();
 END IF;
 IF NEW.status='prepared' AND OLD.status<>'prepared' AND (NEW.settlement IS NULL OR NEW.settlement->>'kind'<>'attempt' OR NEW.settlement->>'status' NOT IN('succeeded','empty') OR NEW.settlement->>'remote_state'<>'stopped') THEN
  RAISE EXCEPTION 'prepared execution not settled' USING ERRCODE='23514';
 END IF;
 IF NEW.status='consumed' AND (OLD.status<>'consumed' OR OLD.consumed_at IS NULL AND OLD.settlement IS NULL AND NEW.settlement IS NOT NULL) THEN NEW.consumed_at=clock_timestamp(); END IF;
 RETURN NEW;
END $$;

-- One lock serializes native admission with a durable no-dispatch settlement.
-- The admitted manifest survives independent native journal retention.
CREATE FUNCTION chartworks.guard_authoring_preparation_read() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE p chartworks.authoring_preparations%ROWTYPE; current_topic record; current_binding jsonb; BEGIN
 IF NEW.operation_id NOT LIKE 'chart-prepare:%' THEN RETURN NEW; END IF;
 SELECT * INTO p FROM chartworks.authoring_preparations WHERE tenant_id=NEW.tenant_id AND actor_id=NEW.actor_id AND source_operation=NEW.operation_id FOR UPDATE NOWAIT;
 IF NOT FOUND OR NOT p.admission_guarded OR p.status<>'accepted' OR p.settlement IS NOT NULL
 OR p.deadline<=clock_timestamp() OR p.admitted_attempt_id IS NOT NULL
 OR p.session_id IS DISTINCT FROM NEW.manifest->>'session' OR NEW.attempt_number<>1
 OR NEW.source_id<>p.source_id OR NEW.context_id IS DISTINCT FROM p.record#>>'{binding,context}'
 OR p.read_receipt IS NULL OR NEW.manifest->'validation' IS DISTINCT FROM p.read_receipt
 OR NEW.deadline>p.deadline OR NEW.manifest->>'preview' IS DISTINCT FROM 'true' THEN
  RAISE EXCEPTION 'preparation admission unavailable' USING ERRCODE='23514';
 END IF;
 SELECT h.archived,h.active_version,v.digest INTO current_topic FROM chartworks.topic_publication_heads h
 LEFT JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version)
 WHERE h.tenant_id=p.tenant_id AND h.topic_id=p.topic_id FOR UPDATE OF h NOWAIT;
 IF NOT FOUND OR current_topic.archived OR current_topic.active_version IS DISTINCT FROM p.topic_version
 OR current_topic.digest IS DISTINCT FROM p.record#>>'{topics,0,digest}'
 OR EXISTS(SELECT 1 FROM chartworks.topic_rule_publication_heads h WHERE h.tenant_id=p.tenant_id AND h.topic_id=p.topic_id AND h.active_version IS NOT NULL) THEN
  RAISE EXCEPTION 'preparation semantics changed' USING ERRCODE='23514';
 END IF;
 SELECT v.binding INTO current_binding FROM chartworks.sources h JOIN chartworks.source_revisions v ON(v.tenant_id,v.source_id,v.revision)=(h.tenant_id,h.source_id,h.current_revision)
 WHERE h.tenant_id=p.tenant_id AND h.source_id=p.source_id AND NOT h.deleted FOR SHARE OF h NOWAIT;
 IF NOT FOUND OR current_binding IS DISTINCT FROM p.record->'binding' THEN
  RAISE EXCEPTION 'preparation source changed' USING ERRCODE='23514';
 END IF;
 UPDATE chartworks.authoring_preparations SET admitted_attempt_id=NEW.attempt_id,admitted_manifest=NEW.manifest WHERE tenant_id=p.tenant_id AND preparation_id=p.preparation_id;
 RETURN NEW;
EXCEPTION WHEN lock_not_available THEN
 RAISE EXCEPTION 'preparation admission busy' USING ERRCODE='CW003';
END $$;
CREATE TRIGGER authoring_preparation_read_start BEFORE INSERT ON chartworks.read_attempts FOR EACH ROW EXECUTE FUNCTION chartworks.guard_authoring_preparation_read();

CREATE TABLE chartworks.authoring_preparation_consumed (
 tenant_id text NOT NULL, preparation_id text NOT NULL, actor_id text NOT NULL, session_id text NOT NULL,
 target_id text NOT NULL, operation_id text NOT NULL, input_digest text NOT NULL,
 source_id text NOT NULL, topic_id text NOT NULL, revision bigint NOT NULL CHECK(revision=1),
 record jsonb NOT NULL CHECK(jsonb_typeof(record)='object' AND octet_length(record::text)<=65536),
 PRIMARY KEY(tenant_id,preparation_id), UNIQUE(tenant_id,actor_id,session_id,operation_id), UNIQUE(tenant_id,target_id),
 FOREIGN KEY(tenant_id,target_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision) ON DELETE CASCADE
);
-- Closed database-generated representation, independently checked at insertion.
CREATE FUNCTION chartworks.preparation_consumed_record(p chartworks.authoring_preparations) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('id',p.preparation_id,'tenant',p.tenant_id,'actor',p.actor_id,'session',p.session_id,
 'target',p.target_id,'operation',p.operation_id,'input_digest',p.input_digest,'source_operation',p.source_operation,
 'digest',p.record->>'digest','source',p.source_id,'source_revision',p.source_revision,'context',p.record#>>'{binding,context}',
 'dataset',p.record#>>'{request,intent,dataset}','topic',p.record#>'{topics,0}','references',p.record->'references',
 'revision',1,'revision_digest',p.record#>>'{revision,digest}','execution_digest',p.record#>>'{revision,execution_digest}',
 'expires_at',p.expires_at,'settlement',p.settlement,'settled_at',p.settled_at,'consumed_at',p.consumed_at)
$$;
CREATE FUNCTION chartworks.protect_preparation_consumed() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE p chartworks.authoring_preparations%ROWTYPE; BEGIN
 IF TG_OP='DELETE' THEN
  IF EXISTS(SELECT 1 FROM chartworks.block_revisions b WHERE b.tenant_id=OLD.tenant_id AND b.block_id=OLD.target_id AND b.revision=OLD.revision) THEN
   RAISE EXCEPTION 'retained preparation receipt' USING ERRCODE='23514';
  END IF; RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'immutable preparation receipt' USING ERRCODE='23514'; END IF;
 SELECT * INTO p FROM chartworks.authoring_preparations WHERE tenant_id=NEW.tenant_id AND preparation_id=NEW.preparation_id;
 IF NOT FOUND OR p.status<>'consumed' OR p.settlement IS NULL OR p.consumed_at IS NULL
 OR ROW(NEW.actor_id,NEW.session_id,NEW.target_id,NEW.operation_id,NEW.input_digest,NEW.source_id,NEW.topic_id)
 IS DISTINCT FROM ROW(p.actor_id,p.session_id,p.target_id,p.operation_id,p.input_digest,p.source_id,p.topic_id)
 OR NEW.record IS DISTINCT FROM chartworks.preparation_consumed_record(p)
 OR NOT EXISTS(SELECT 1 FROM chartworks.block_revisions b WHERE b.tenant_id=p.tenant_id AND b.block_id=p.target_id AND b.revision=1 AND b.digest=p.record#>>'{revision,digest}' AND b.execution_digest=p.record#>>'{revision,execution_digest}') THEN
  RAISE EXCEPTION 'invalid preparation receipt' USING ERRCODE='23514';
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER immutable_preparation_consumed BEFORE INSERT OR UPDATE OR DELETE ON chartworks.authoring_preparation_consumed FOR EACH ROW EXECUTE FUNCTION chartworks.protect_preparation_consumed();

-- One positive-liability predicate owns replacement, reservation accounting and
-- cleanup. Contradictory legacy native evidence is retained and fully charged.
CREATE FUNCTION chartworks.preparation_has_liability(p chartworks.authoring_preparations) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT p.settlement IS NULL OR EXISTS(SELECT 1 FROM chartworks.read_attempts a WHERE a.tenant_id=p.tenant_id AND a.actor_id=p.actor_id AND a.operation_id=p.source_operation
  AND (p.settlement->>'kind'<>'attempt' OR a.attempt_id IS DISTINCT FROM p.settlement->>'attempt' OR a.attempt_number<>1
   OR a.manifest_hash IS DISTINCT FROM p.settlement->>'manifest' OR a.manifest IS DISTINCT FROM COALESCE(p.admitted_manifest,p.record#>'{attempt,manifest}')
   OR a.source_id IS DISTINCT FROM p.source_id OR a.context_id IS DISTINCT FROM p.record#>>'{binding,context}' OR a.manifest->>'session' IS DISTINCT FROM p.session_id
   OR a.remote_state='not_issued' AND a.remote_query IS NOT NULL OR a.status IS DISTINCT FROM p.settlement->>'status'
   OR a.remote_state IS DISTINCT FROM p.settlement->>'remote_state' OR a.finished_at IS DISTINCT FROM (p.settlement->>'finished_at')::timestamptz
   OR a.status NOT IN('succeeded','empty','truncated','cancelled','timed_out','failed','interrupted') OR a.remote_state NOT IN('stopped','not_issued') OR a.finished_at IS NULL))
$$;
CREATE FUNCTION chartworks.preparation_cleanup_eligible(p chartworks.authoring_preparations) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT p.status IN('prepared','failed','consumed') AND p.settlement IS NOT NULL
 AND p.expires_at<=statement_timestamp() AND p.settled_at<=statement_timestamp()-interval '24 hours'
 AND (p.status<>'consumed' OR p.consumed_at<=statement_timestamp()-interval '24 hours')
 AND (p.status='consumed' OR chartworks.preparation_key_time(p.operation_id) IS NULL OR chartworks.preparation_key_time(p.operation_id)<statement_timestamp()-interval '5 minutes')
 AND NOT chartworks.preparation_has_liability(p)
$$;
CREATE FUNCTION chartworks.protect_preparation_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF chartworks.preparation_cleanup_eligible(OLD) IS DISTINCT FROM true OR OLD.status='consumed' AND NOT EXISTS(
  SELECT 1 FROM chartworks.authoring_preparation_consumed c WHERE c.tenant_id=OLD.tenant_id AND c.preparation_id=OLD.preparation_id AND c.record=chartworks.preparation_consumed_record(OLD)) THEN
  RAISE EXCEPTION 'preparation retention not settled' USING ERRCODE='23514';
 END IF; RETURN OLD;
END $$;
CREATE TRIGGER authoring_preparation_delete_guard BEFORE DELETE ON chartworks.authoring_preparations FOR EACH ROW EXECUTE FUNCTION chartworks.protect_preparation_delete();
DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''authoring.preparation_settled'',''authoring.preparation_pruned''))',previous);
END $$;
