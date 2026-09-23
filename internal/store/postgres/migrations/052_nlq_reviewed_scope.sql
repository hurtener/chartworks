-- Older plans retain an empty scope and fail closed on revalidation. New plans
-- pin the exact reviewed dataset/column projection beside their SQL evidence.
ALTER TABLE chartworks.nlq_queries
 ADD COLUMN relation_scope jsonb NOT NULL DEFAULT '[]'::jsonb
 CHECK (jsonb_typeof(relation_scope)='array' AND jsonb_array_length(relation_scope)<=32 AND octet_length(relation_scope::text)<=262144);

CREATE FUNCTION chartworks.protect_nlq_relation_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.relation_scope IS DISTINCT FROM OLD.relation_scope THEN
  RAISE EXCEPTION 'nlq reviewed scope is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER nlq_relation_scope_immutable BEFORE UPDATE ON chartworks.nlq_queries
 FOR EACH ROW EXECUTE FUNCTION chartworks.protect_nlq_relation_scope();
