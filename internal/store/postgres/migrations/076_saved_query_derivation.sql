-- Saved copies have an explicit origin and never impersonate a new direct
-- clarification/review submission. The original seals remain on their parent.
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_saved_derivation_shape CHECK (
 saved_copy_parent IS NULL OR (
  saved_copy_parent=parent_id AND generation_pending IS NULL AND
  generation_resolution IS NULL AND intent_review IS NULL AND
  plan_operation IS NULL AND plan_request_digest IS NULL));

CREATE FUNCTION chartworks.guard_saved_derivation_payload() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.saved_copy_parent IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM chartworks.nlq_queries p
  WHERE (p.tenant_id,p.actor_id,p.session_id,p.query_id)=
   (NEW.tenant_id,NEW.actor_id,NEW.session_id,NEW.saved_copy_parent)
  AND p.revision=NEW.parent_revision
  AND ROW(p.topic_id,p.topics,p.topic_versions,p.rule_versions,p.template_selections,
   p.example_selection,p.context_id,p.locale,p.question,p.route,p.generation,
   p.sql_text,p.parameters,p.receipt,p.assumptions,p.ambiguities,p.errors,
   p.validation_fixes,p.clarification,p.relation_scope,p.analytical_version,p.analytical)
  IS NOT DISTINCT FROM
   ROW(NEW.topic_id,NEW.topics,NEW.topic_versions,NEW.rule_versions,NEW.template_selections,
   NEW.example_selection,NEW.context_id,NEW.locale,NEW.question,NEW.route,NEW.generation,
   NEW.sql_text,NEW.parameters,NEW.receipt,NEW.assumptions,NEW.ambiguities,NEW.errors,
   NEW.validation_fixes,NEW.clarification,NEW.relation_scope,NEW.analytical_version,NEW.analytical)
 ) THEN
  RAISE EXCEPTION 'saved derivation payload mismatch' USING ERRCODE='23503';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER b_saved_derivation_payload BEFORE INSERT ON chartworks.nlq_queries
 FOR EACH ROW EXECUTE FUNCTION chartworks.guard_saved_derivation_payload();
