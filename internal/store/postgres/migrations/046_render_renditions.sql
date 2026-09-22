CREATE TABLE chartworks.render_renditions (
 tenant_id text NOT NULL,
 rendition_id text NOT NULL CHECK(rendition_id ~ '^rnd-[a-f0-9]{32}$'),
 run_id text NOT NULL CHECK(run_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 output_id text NOT NULL CHECK(length(output_id) BETWEEN 0 AND 128),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 private boolean NOT NULL,
 source_digest text NOT NULL CHECK(source_digest ~ '^[a-f0-9]{64}$'),
 renderer_version text NOT NULL CHECK(length(renderer_version) BETWEEN 1 AND 128),
 theme_version text NOT NULL CHECK(length(theme_version) BETWEEN 1 AND 128),
 format text NOT NULL CHECK(format IN('json','csv','html','svg')),
 request bytea NOT NULL CHECK(octet_length(request) BETWEEN 2 AND 1048576),
 rendition bytea NOT NULL CHECK(octet_length(rendition) BETWEEN 2 AND 67108864),
 content_digest text NOT NULL CHECK(content_digest ~ '^[a-f0-9]{64}$'),
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,rendition_id),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '90 days')
);
CREATE INDEX render_rendition_run ON chartworks.render_renditions(tenant_id,run_id,rendition_id);
CREATE INDEX render_rendition_expiry ON chartworks.render_renditions(tenant_id,expires_at,rendition_id);

CREATE FUNCTION chartworks.protect_render_rendition() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'rendition is immutable' USING ERRCODE='23514';
END $$;
CREATE TRIGGER render_rendition_immutable BEFORE UPDATE ON chartworks.render_renditions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_render_rendition();

DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''rendition.created'',''rendition.expired''))',previous);
END $$;
