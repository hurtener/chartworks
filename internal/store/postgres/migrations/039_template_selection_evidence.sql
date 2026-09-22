ALTER TABLE chartworks.nlq_queries
 ADD COLUMN template_selections jsonb NOT NULL DEFAULT '[]'::jsonb
 CHECK(jsonb_typeof(template_selections)='array' AND jsonb_array_length(template_selections)<=4 AND octet_length(template_selections::text)<=16384);
ALTER TABLE chartworks.nlq_queries ALTER COLUMN template_selections DROP DEFAULT;

CREATE OR REPLACE FUNCTION chartworks.protect_nlq_query() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR NEW.tenant_id<>OLD.tenant_id OR NEW.actor_id<>OLD.actor_id OR NEW.session_id<>OLD.session_id OR NEW.query_id<>OLD.query_id OR
    NEW.parent_id IS DISTINCT FROM OLD.parent_id OR NEW.topic_id<>OLD.topic_id OR NEW.topics IS DISTINCT FROM OLD.topics OR
    NEW.topic_versions IS DISTINCT FROM OLD.topic_versions OR NEW.rule_versions IS DISTINCT FROM OLD.rule_versions OR
    NEW.template_selections IS DISTINCT FROM OLD.template_selections OR NEW.context_id<>OLD.context_id OR
    NEW.locale<>OLD.locale OR NEW.question<>OLD.question OR NEW.route IS DISTINCT FROM OLD.route OR NEW.created_at<>OLD.created_at OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'nlq query immutable fields changed' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;

ALTER TABLE chartworks.topic_rule_comparison_evidence
 ADD COLUMN template_selection jsonb
 CHECK(template_selection IS NULL OR (jsonb_typeof(template_selection)='object' AND octet_length(template_selection::text)<=16384));
