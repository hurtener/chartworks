ALTER TABLE chartworks.nlq_queries
  ADD COLUMN parent_revision bigint,
  ADD COLUMN parent_digest text;

ALTER TABLE chartworks.nlq_queries
  ADD CONSTRAINT nlq_queries_parent_lineage_check CHECK (
    (parent_id IS NULL AND parent_revision IS NULL AND parent_digest IS NULL) OR
    (parent_id IS NOT NULL AND parent_revision IS NULL AND parent_digest IS NULL) OR
    (parent_id IS NOT NULL AND parent_revision > 0 AND parent_digest ~ '^[0-9a-f]{64}$')
  );

COMMENT ON COLUMN chartworks.nlq_queries.parent_revision IS
  'Exact parent revision observed before child planning; NULL on pre-049 legacy children.';
COMMENT ON COLUMN chartworks.nlq_queries.parent_digest IS
  'SHA-256 digest of the exact protected parent record observed before child planning; NULL on pre-049 legacy children.';

CREATE OR REPLACE FUNCTION chartworks.protect_nlq_query() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' AND chartworks.deleted_composition_operation(OLD.tenant_id,OLD.operation) THEN RETURN OLD; END IF;
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.actor_id<>OLD.actor_id OR NEW.session_id<>OLD.session_id OR NEW.query_id<>OLD.query_id OR
    NEW.parent_id IS DISTINCT FROM OLD.parent_id OR NEW.parent_revision IS DISTINCT FROM OLD.parent_revision OR NEW.parent_digest IS DISTINCT FROM OLD.parent_digest OR
    NEW.topic_id<>OLD.topic_id OR NEW.topics IS DISTINCT FROM OLD.topics OR NEW.topic_versions IS DISTINCT FROM OLD.topic_versions OR NEW.rule_versions IS DISTINCT FROM OLD.rule_versions OR
    NEW.template_selections IS DISTINCT FROM OLD.template_selections OR NEW.example_selection IS DISTINCT FROM OLD.example_selection OR
    NEW.context_id<>OLD.context_id OR NEW.locale<>OLD.locale OR NEW.question<>OLD.question OR NEW.route IS DISTINCT FROM OLD.route OR
    NEW.created_at<>OLD.created_at OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'nlq query immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
