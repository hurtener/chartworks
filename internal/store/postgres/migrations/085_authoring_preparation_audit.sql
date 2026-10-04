-- Preserve every existing audit action while admitting only the closed,
-- content-free preparation reservation and terminal transition events.
DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''authoring.preparation_reserved'',''authoring.preparation_prepared'',''authoring.preparation_failed'',''authoring.preparation_uncertain''))',previous);
END $$;
