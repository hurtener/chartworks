-- Source-backed report blocks are distinct from reviewed topic definitions.
-- Existing topic records, definition bytes and custody hashes are unchanged.
ALTER TABLE chartworks.block_heads ALTER COLUMN topic_id DROP NOT NULL;
ALTER TABLE chartworks.block_heads ADD COLUMN source_parent text;
ALTER TABLE chartworks.block_heads ADD CONSTRAINT block_origin_exclusive CHECK ((topic_id IS NULL) <> (source_parent IS NULL));
ALTER TABLE chartworks.block_heads ADD CONSTRAINT block_source_parent FOREIGN KEY (tenant_id,source_parent) REFERENCES chartworks.sources(tenant_id,source_id);
CREATE FUNCTION chartworks.protect_block_origin() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF ROW(NEW.topic_id,NEW.source_parent) IS DISTINCT FROM ROW(OLD.topic_id,OLD.source_parent) THEN
  RAISE EXCEPTION 'immutable block origin' USING ERRCODE='23514';
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER immutable_block_origin BEFORE UPDATE ON chartworks.block_heads FOR EACH ROW EXECUTE FUNCTION chartworks.protect_block_origin();

ALTER TABLE chartworks.authoring_preparations ALTER COLUMN topic_id DROP NOT NULL;
ALTER TABLE chartworks.authoring_preparations ALTER COLUMN topic_version DROP NOT NULL;
ALTER TABLE chartworks.authoring_preparations ADD CONSTRAINT preparation_origin_exclusive CHECK (COALESCE(
 (topic_id IS NOT NULL AND topic_version IS NOT NULL AND record#>'{request,intent,source_dataset}' IS NULL)
 OR (topic_id IS NULL AND topic_version IS NULL AND jsonb_typeof(record#>'{request,intent,source_dataset}')='object'
  AND record#>>'{request,intent,source_dataset,source}'=source_id
  AND (record#>>'{request,intent,source_dataset,source_revision}')::bigint=source_revision
  AND record#>>'{request,intent,source_dataset,context}'=record#>>'{binding,context}'
  AND record#>>'{request,intent,source_dataset,dataset}'=record#>>'{request,intent,dataset}'
  AND record#>>'{request,intent,source_dataset,schema_digest}' ~ '^[a-f0-9]{64}$'
  AND record->'topics' IN ('null'::jsonb,'[]'::jsonb)),false));
ALTER TABLE chartworks.authoring_preparation_consumed ALTER COLUMN topic_id DROP NOT NULL;

-- The same guarded receipt and full binding fence protect native admission.
-- A registered table has no semantic publication or rule head to lock.
CREATE OR REPLACE FUNCTION chartworks.guard_authoring_preparation_read() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE p chartworks.authoring_preparations%ROWTYPE; current_topic record; current_binding jsonb; BEGIN
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
 IF p.topic_id IS NOT NULL THEN
 SELECT h.archived,h.active_version,v.digest INTO current_topic FROM chartworks.topic_publication_heads h
 LEFT JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version)
 WHERE h.tenant_id=p.tenant_id AND h.topic_id=p.topic_id FOR UPDATE OF h NOWAIT;
 IF NOT FOUND OR current_topic.archived OR current_topic.active_version IS DISTINCT FROM p.topic_version
 OR current_topic.digest IS DISTINCT FROM p.record#>>'{topics,0,digest}'
 OR EXISTS(SELECT 1 FROM chartworks.topic_rule_publication_heads h WHERE h.tenant_id=p.tenant_id AND h.topic_id=p.topic_id AND h.active_version IS NOT NULL) THEN
  RAISE EXCEPTION 'preparation semantics changed' USING ERRCODE='23514';
 END IF;
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

-- Preserve compact source origin after SQL-bearing preparation cleanup.
CREATE OR REPLACE FUNCTION chartworks.preparation_consumed_record(p chartworks.authoring_preparations) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('id',p.preparation_id,'tenant',p.tenant_id,'actor',p.actor_id,'session',p.session_id,
 'target',p.target_id,'operation',p.operation_id,'input_digest',p.input_digest,'source_operation',p.source_operation,
 'digest',p.record->>'digest','source',p.source_id,'source_revision',p.source_revision,'context',p.record#>>'{binding,context}',
 'dataset',p.record#>>'{request,intent,dataset}','topic',p.record#>'{topics,0}','references',p.record->'references',
 'revision',1,'revision_digest',p.record#>>'{revision,digest}','execution_digest',p.record#>>'{revision,execution_digest}',
 'expires_at',p.expires_at,'settlement',p.settlement,'settled_at',p.settled_at,'consumed_at',p.consumed_at) || CASE WHEN p.record#>'{request,intent,source_dataset}' IS NULL THEN '{}'::jsonb ELSE jsonb_build_object('source_dataset',p.record#>'{request,intent,source_dataset}') END
$$;
