-- Physical option lookup keeps the same custody and native read journal.
-- Existing reviewed admissions retain their bytes, keys, foreign keys and fences.
ALTER TABLE chartworks.authoring_option_operations
 ALTER COLUMN topic_id DROP NOT NULL,
 ALTER COLUMN topic_version DROP NOT NULL;
-- PostgreSQL 17 is the qualified metadata-store baseline.
ALTER TABLE chartworks.authoring_option_operations
 ALTER COLUMN topic_id SET EXPRESSION AS (NULLIF(record#>>'{topic,topic}','')),
 ALTER COLUMN topic_version SET EXPRESSION AS (NULLIF(record#>>'{topic,version}',''));
ALTER TABLE chartworks.authoring_option_operations ADD CONSTRAINT option_origin_exclusive CHECK (COALESCE(
 (topic_id IS NOT NULL AND topic_version IS NOT NULL AND record#>'{target,dataset,source_dataset}' IS NULL)
 OR (topic_id IS NULL AND topic_version IS NULL AND jsonb_typeof(record->'column')='object'
  AND record->'topic'='{"topic":"","version":"","digest":""}'::jsonb),false));
ALTER TABLE chartworks.authoring_option_operations ADD CONSTRAINT option_column_coordinates CHECK (record->'column' IS NULL OR COALESCE(
 jsonb_typeof(record->'column')='object' AND COALESCE(record->>'dimension','')=''
 AND record#>>'{column,type}' IN ('text','identifier','integer','number','boolean')
 AND record#>>'{column,source_dataset,source}'=source_id
 AND record#>>'{column,source_dataset,context}'=record->>'context'
 AND record#>>'{column,source_dataset,dataset}'=record->>'dataset'
 AND (record#>>'{column,source_dataset,source_revision}')::bigint=source_revision
 AND record#>>'{column,source_dataset,schema_digest}' ~ '^[a-f0-9]{64}$'
 AND (record#>'{target,dataset}' IS NULL OR (
  COALESCE(record#>>'{target,dataset,column}','')<>'' AND COALESCE(record#>>'{target,dataset,dimension}','')=''
  AND (topic_id IS NOT NULL OR record#>'{target,dataset,source_dataset}'=record#>'{column,source_dataset}'))),false));

CREATE OR REPLACE FUNCTION chartworks.guard_authoring_option_read() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE r chartworks.authoring_option_operations%ROWTYPE; current_version text; current_digest text; current_archived boolean; pin jsonb; current_target record; BEGIN
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
  SELECT h.archived,v.digest,v.actor_id,v.definition->'source_dataset' AS source_dataset,EXISTS(SELECT 1 FROM chartworks.block_publications p WHERE(p.tenant_id,p.block_id,p.revision)=(v.tenant_id,v.block_id,v.revision)) AS published INTO current_target
  FROM chartworks.block_heads h JOIN chartworks.block_revisions v ON(v.tenant_id,v.block_id)=(h.tenant_id,h.block_id) AND v.revision=(pin->>'revision')::bigint
  WHERE h.tenant_id=r.tenant_id AND h.block_id=pin->>'block' FOR SHARE OF h;
  IF NOT FOUND OR current_target.archived OR current_target.digest<>pin->>'digest'
   OR pin->>'policy'='private_preview' AND current_target.actor_id<>r.actor_id
   OR pin->>'policy'='published' AND NOT current_target.published
   OR r.topic_id IS NULL AND current_target.source_dataset IS DISTINCT FROM r.record#>'{column,source_dataset}' THEN
   RAISE EXCEPTION 'option block changed' USING ERRCODE='23514';
  END IF;
 END LOOP;
 IF r.topic_id IS NOT NULL THEN
 SELECT COALESCE(h.active_version,''),h.archived,COALESCE(v.digest,'') INTO current_version,current_archived,current_digest
 FROM chartworks.topic_publication_heads h LEFT JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version)
 WHERE h.tenant_id=r.tenant_id AND h.topic_id=r.topic_id FOR UPDATE OF h;
 IF NOT FOUND OR current_archived OR current_version<>r.topic_version OR current_digest<>r.record#>>'{topic,digest}'
 OR EXISTS(SELECT 1 FROM chartworks.topic_rule_publication_heads h WHERE h.tenant_id=r.tenant_id AND h.topic_id=r.topic_id AND h.active_version IS NOT NULL) THEN
  RAISE EXCEPTION 'option semantics changed' USING ERRCODE='23514';
 END IF;
 END IF;
 SELECT h.current_revision,v.context_id INTO current_target FROM chartworks.sources h JOIN chartworks.source_revisions v ON(v.tenant_id,v.source_id,v.revision)=(h.tenant_id,h.source_id,h.current_revision)
 WHERE h.tenant_id=r.tenant_id AND h.source_id=r.source_id AND NOT h.deleted FOR SHARE OF h;
 IF NOT FOUND OR current_target.current_revision<>r.source_revision OR current_target.context_id<>r.record->>'context' THEN
  RAISE EXCEPTION 'option source changed' USING ERRCODE='23514';
 END IF; RETURN NEW;
END $$;
