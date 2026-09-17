ALTER TABLE chartworks.nlq_queries ADD COLUMN clarification jsonb;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_clarification_shape CHECK (
    clarification IS NULL OR (
        jsonb_typeof(clarification) = 'object'
        AND clarification->>'schema_version' = '1'
        AND octet_length(clarification::text) <= 262144
    )
);
CREATE FUNCTION chartworks.keep_nlq_clarification_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.clarification IS DISTINCT FROM OLD.clarification THEN
        RAISE EXCEPTION 'immutable query clarification evidence';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER nlq_clarification_immutable BEFORE UPDATE ON chartworks.nlq_queries
    FOR EACH ROW EXECUTE FUNCTION chartworks.keep_nlq_clarification_immutable();
