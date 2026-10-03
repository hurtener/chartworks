-- Protected saved copies retain every semantic field. Only authenticated
-- producer/query/ancestor resealing may differ from the exact parent route.
-- The private Go INSERT guard additionally checks the canonical parent witness
-- hash; JSONB text is not that canonical encoding and is never used as a substitute.
CREATE FUNCTION chartworks.protected_saved_route_equal(parent_route jsonb, child_route jsonb, parent_id text, child_id text)
RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
 WHEN parent_route->'clarification_applicability' IS NULL OR parent_route->'clarification_applicability'='null'::jsonb
 THEN parent_route IS NOT DISTINCT FROM child_route
 ELSE COALESCE(
  jsonb_typeof(parent_route->'clarification_applicability')='object'
  AND jsonb_typeof(child_route->'clarification_applicability')='object'
  AND parent_route#>'{clarification_applicability,version}'='1'::jsonb
  AND jsonb_typeof(parent_route#>'{clarification_applicability,query}')='string'
  AND jsonb_typeof(child_route#>'{clarification_applicability,query}')='string'
  AND jsonb_typeof(child_route#>'{clarification_applicability,prior_query}')='string'
  AND parent_route#>>'{clarification_applicability,query}'=parent_id
  AND child_route#>>'{clarification_applicability,query}'=child_id
  AND child_route#>>'{clarification_applicability,prior_query}'=parent_id
  AND child_id<>parent_id
  AND jsonb_typeof(child_route#>'{clarification_applicability,prior_digest}')='string'
  AND child_route#>>'{clarification_applicability,prior_digest}' ~ '^[0-9a-f]{64}$'
  AND (parent_route #- '{clarification_applicability,query}' #- '{clarification_applicability,prior_query}' #- '{clarification_applicability,prior_digest}')
   IS NOT DISTINCT FROM
   (child_route #- '{clarification_applicability,query}' #- '{clarification_applicability,prior_query}' #- '{clarification_applicability,prior_digest}'), false)
 END
$$;

CREATE OR REPLACE FUNCTION chartworks.register_nlq_query_origin() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE root text; root_count integer;
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.tenant_id,721415));
 IF EXISTS(SELECT 1 FROM chartworks.nlq_query_origins o WHERE (o.tenant_id,o.actor_id,o.session_id,o.query_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id) AND o.erased_at IS NOT NULL) THEN
 RAISE EXCEPTION 'query origin erased' USING ERRCODE='23503'; END IF;
 IF TG_OP='UPDATE' AND NEW.saved_copy_parent IS DISTINCT FROM OLD.saved_copy_parent THEN RAISE EXCEPTION 'saved copy origin immutable' USING ERRCODE='55000'; END IF;
 IF TG_OP='INSERT' AND NEW.saved_copy_parent IS NOT NULL THEN
  IF NEW.saved_copy_parent IS DISTINCT FROM NEW.parent_id OR NOT EXISTS(SELECT 1 FROM chartworks.nlq_queries p
   WHERE (p.tenant_id,p.actor_id,p.session_id,p.query_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.saved_copy_parent)
   AND ROW(p.sql_text,p.parameters,p.generation,p.topic_versions,p.rule_versions,p.context_id) IS NOT DISTINCT FROM ROW(NEW.sql_text,NEW.parameters,NEW.generation,NEW.topic_versions,NEW.rule_versions,NEW.context_id)
   AND chartworks.protected_saved_route_equal(p.route,NEW.route,p.query_id,NEW.query_id)
   AND p.revision=NEW.parent_revision AND NEW.status='planned' AND NEW.result IS NULL AND NEW.plan_operation IS NULL) THEN
   RAISE EXCEPTION 'invalid saved copy origin' USING ERRCODE='23503'; END IF;
 END IF;
 INSERT INTO chartworks.nlq_query_origins(tenant_id,actor_id,session_id,query_id,saved_copy_parent) VALUES(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,NEW.saved_copy_parent) ON CONFLICT DO NOTHING;
 IF TG_OP='INSERT' THEN
 SELECT count(DISTINCT c.operation_id) INTO root_count FROM chartworks.composition_runs c JOIN chartworks.composition_run_groups g USING(tenant_id,operation_id)
 WHERE (c.tenant_id,c.actor_id,c.session_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id) AND g.kind='query' AND ('composition:'||c.operation_id||':'||g.group_id) IN(NEW.plan_operation,NEW.operation);
 IF root_count>1 THEN RAISE EXCEPTION 'ambiguous composition origin' USING ERRCODE='23503'; END IF;
 FOR root IN SELECT DISTINCT c.operation_id FROM chartworks.composition_runs c JOIN chartworks.composition_run_groups g USING(tenant_id,operation_id)
 WHERE (c.tenant_id,c.actor_id,c.session_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id) AND g.kind='query' AND ('composition:'||c.operation_id||':'||g.group_id) IN(NEW.plan_operation,NEW.operation) LOOP
  IF chartworks.exact_deleted_composition_root(NEW.tenant_id,root) OR EXISTS(SELECT 1 FROM chartworks.composition_origin_erasure x WHERE x.tenant_id=NEW.tenant_id AND x.operation_id=root) THEN RAISE EXCEPTION 'composition origin erased' USING ERRCODE='23503'; END IF;
  INSERT INTO chartworks.nlq_query_owners VALUES(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,root) ON CONFLICT DO NOTHING;
 END LOOP;
 IF NEW.saved_copy_parent IS NOT NULL THEN
 INSERT INTO chartworks.nlq_query_owners SELECT NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,o.composition_root FROM chartworks.nlq_query_owners o WHERE (o.tenant_id,o.actor_id,o.session_id,o.query_id)=(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.saved_copy_parent) ON CONFLICT DO NOTHING;
 END IF;
 END IF; -- ownership is immutable; later Run keys add history only
 IF NEW.operation IS NOT NULL THEN INSERT INTO chartworks.nlq_query_operations VALUES(NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.query_id,NEW.operation) ON CONFLICT DO NOTHING; END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION chartworks.guard_saved_derivation_payload() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.saved_copy_parent IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM chartworks.nlq_queries p
  WHERE (p.tenant_id,p.actor_id,p.session_id,p.query_id)=
   (NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.saved_copy_parent)
  AND p.revision=NEW.parent_revision
  AND chartworks.protected_saved_route_equal(p.route,NEW.route,p.query_id,NEW.query_id)
  AND ROW(p.topic_id,p.topics,p.topic_versions,p.rule_versions,p.template_selections,
   p.example_selection,p.context_id,p.locale,p.question,p.generation,
   p.sql_text,p.parameters,p.receipt,p.assumptions,p.ambiguities,p.errors,
   p.validation_fixes,p.clarification,p.relation_scope,p.analytical_version,p.analytical)
  IS NOT DISTINCT FROM
   ROW(NEW.topic_id,NEW.topics,NEW.topic_versions,NEW.rule_versions,NEW.template_selections,
   NEW.example_selection,NEW.context_id,NEW.locale,NEW.question,NEW.generation,
   NEW.sql_text,NEW.parameters,NEW.receipt,NEW.assumptions,NEW.ambiguities,NEW.errors,
   NEW.validation_fixes,NEW.clarification,NEW.relation_scope,NEW.analytical_version,NEW.analytical)
 ) THEN
  RAISE EXCEPTION 'saved derivation payload mismatch' USING ERRCODE='23503';
 END IF;
 RETURN NEW;
END $$;
