-- Phase 29: immutable report/dashboard revisions and independent lifecycle heads.
-- No identity, audience-grant, issuer or duplicated warehouse-execution tables.
CREATE TABLE chartworks.document_heads (
 tenant_id text NOT NULL CHECK(tenant_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 kind text NOT NULL CHECK(kind IN('report','dashboard')),
 document_id text NOT NULL CHECK(document_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 4096),
 latest_revision bigint NOT NULL CHECK(latest_revision BETWEEN 1 AND 256),
 draft_revision bigint,
 review_revision bigint,
 published_revision bigint,
 archived boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,kind,document_id),
 CHECK(draft_revision IS NULL OR draft_revision BETWEEN 1 AND latest_revision),
 CHECK(review_revision IS NULL OR review_revision BETWEEN 1 AND latest_revision),
 CHECK(published_revision IS NULL OR published_revision BETWEEN 1 AND latest_revision),
 CHECK(draft_revision IS NULL OR review_revision IS NULL OR draft_revision<>review_revision),
 CHECK(draft_revision IS NULL OR published_revision IS NULL OR draft_revision<>published_revision),
 CHECK(review_revision IS NULL OR published_revision IS NULL OR review_revision<>published_revision)
);
CREATE TABLE chartworks.document_revisions (
 tenant_id text NOT NULL,
 kind text NOT NULL,
 document_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 256),
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object' AND octet_length(definition::text)<=2097152),
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 session_id text NOT NULL CHECK(session_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 origins jsonb NOT NULL CHECK(jsonb_typeof(origins)='array' AND jsonb_array_length(origins)<=100 AND octet_length(origins::text)<=524288),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,kind,document_id,revision),
 FOREIGN KEY(tenant_id,kind,document_id) REFERENCES chartworks.document_heads(tenant_id,kind,document_id)
);
CREATE TABLE chartworks.document_publications (
 tenant_id text NOT NULL,
 kind text NOT NULL,
 document_id text NOT NULL,
 revision bigint NOT NULL,
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,kind,document_id,revision),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision)
);
ALTER TABLE chartworks.document_heads
 ADD FOREIGN KEY(tenant_id,kind,document_id,draft_revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision) DEFERRABLE INITIALLY DEFERRED,
 ADD FOREIGN KEY(tenant_id,kind,document_id,review_revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision) DEFERRABLE INITIALLY DEFERRED,
 ADD FOREIGN KEY(tenant_id,kind,document_id,published_revision) REFERENCES chartworks.document_publications(tenant_id,kind,document_id,revision) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE chartworks.document_references (
 tenant_id text NOT NULL,
 kind text NOT NULL CHECK(kind='report'),
 document_id text NOT NULL,
 revision bigint NOT NULL,
 resource_kind text NOT NULL,
 permission text NOT NULL,
 resource_id text NOT NULL CHECK(resource_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 PRIMARY KEY(tenant_id,kind,document_id,revision,resource_kind,permission,resource_id),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision),
 CHECK((resource_kind,permission) IN(('block','read'),('topic','read'),('source','read'),('dataset','query'),('execution_context','use')))
);
CREATE TABLE chartworks.document_block_refs (
 tenant_id text NOT NULL,
 kind text NOT NULL CHECK(kind='report'),
 document_id text NOT NULL,
 revision bigint NOT NULL,
 widget_id text NOT NULL CHECK(widget_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 block_id text NOT NULL,
 pinned_revision bigint,
 observed_revision bigint NOT NULL,
 PRIMARY KEY(tenant_id,kind,document_id,revision,widget_id),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision),
 FOREIGN KEY(tenant_id,block_id,observed_revision) REFERENCES chartworks.block_publications(tenant_id,block_id,revision),
 CHECK(pinned_revision IS NULL OR pinned_revision=observed_revision)
);
CREATE TABLE chartworks.document_topic_refs (
 tenant_id text NOT NULL,
 kind text NOT NULL CHECK(kind='report'),
 document_id text NOT NULL,
 revision bigint NOT NULL,
 widget_id text NOT NULL,
 topic_id text NOT NULL,
 version_id text NOT NULL,
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(tenant_id,kind,document_id,revision,widget_id,topic_id),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision),
 FOREIGN KEY(tenant_id,topic_id,version_id) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id)
);
CREATE TABLE chartworks.document_query_refs (
 tenant_id text NOT NULL,
 kind text NOT NULL CHECK(kind='report'),
 document_id text NOT NULL,
 revision bigint NOT NULL,
 widget_id text NOT NULL,
 actor_id text NOT NULL,
 session_id text NOT NULL,
 query_id text NOT NULL,
 query_digest text NOT NULL CHECK(query_digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(tenant_id,kind,document_id,revision,widget_id),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision),
 FOREIGN KEY(tenant_id,actor_id,session_id,query_id) REFERENCES chartworks.nlq_queries(tenant_id,actor_id,session_id,query_id)
);
CREATE TABLE chartworks.document_page_refs (
 tenant_id text NOT NULL,
 kind text NOT NULL CHECK(kind='dashboard'),
 document_id text NOT NULL,
 revision bigint NOT NULL,
 ordinal integer NOT NULL CHECK(ordinal BETWEEN 0 AND 99),
 page_id text NOT NULL CHECK(page_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 title text NOT NULL CHECK(octet_length(title)<=256),
 report_kind text NOT NULL DEFAULT 'report' CHECK(report_kind='report'),
 report_id text NOT NULL,
 report_revision bigint NOT NULL,
 PRIMARY KEY(tenant_id,kind,document_id,revision,ordinal),
 UNIQUE(tenant_id,kind,document_id,revision,page_id),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision),
 FOREIGN KEY(tenant_id,report_kind,report_id,report_revision) REFERENCES chartworks.document_publications(tenant_id,kind,document_id,revision)
);
CREATE TABLE chartworks.document_external_refs (
 tenant_id text NOT NULL,
 kind text NOT NULL,
 external_system text NOT NULL CHECK(external_system ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 external_id text NOT NULL CHECK(external_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 external_version text NOT NULL CHECK(external_version ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 document_id text NOT NULL,
 revision bigint NOT NULL,
 PRIMARY KEY(tenant_id,kind,external_system,external_id,external_version),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision)
);
CREATE TABLE chartworks.document_events (
 tenant_id text NOT NULL,
 kind text NOT NULL,
 document_id text NOT NULL,
 version bigint NOT NULL,
 revision bigint NOT NULL,
 operation text NOT NULL CHECK(operation IN('create','edit','review','publish','reject','archive','import')),
 actor_id text NOT NULL,
 note text NOT NULL CHECK(octet_length(note)<=2048),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,kind,document_id,version),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision)
);
CREATE TABLE chartworks.document_quarantine (
 tenant_id text NOT NULL,
 actor_id text NOT NULL CHECK(actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 quarantine_id text NOT NULL CHECK(quarantine_id ~ '^[a-f0-9]{32}$'),
 kind text NOT NULL CHECK(kind IN('report','dashboard')),
 target_id text NOT NULL CHECK(target_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 definition jsonb NOT NULL CHECK(octet_length(definition::text)<=2097152),
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 reason text NOT NULL CHECK(reason='unsupported_definition'),
 external_reference jsonb NOT NULL CHECK(jsonb_typeof(external_reference)='object' AND octet_length(external_reference::text)<=2048),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,actor_id,quarantine_id)
);

CREATE FUNCTION chartworks.immutable_document_row() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'document evidence is immutable' USING ERRCODE='55000';
END $$;
CREATE TRIGGER document_revision_immutable BEFORE UPDATE OR DELETE ON chartworks.document_revisions FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_publication_immutable BEFORE UPDATE OR DELETE ON chartworks.document_publications FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_reference_immutable BEFORE UPDATE OR DELETE ON chartworks.document_references FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_block_ref_immutable BEFORE UPDATE OR DELETE ON chartworks.document_block_refs FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_topic_ref_immutable BEFORE UPDATE OR DELETE ON chartworks.document_topic_refs FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_query_ref_immutable BEFORE UPDATE OR DELETE ON chartworks.document_query_refs FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_page_ref_immutable BEFORE UPDATE OR DELETE ON chartworks.document_page_refs FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_external_ref_immutable BEFORE UPDATE OR DELETE ON chartworks.document_external_refs FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_event_immutable BEFORE UPDATE OR DELETE ON chartworks.document_events FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE TRIGGER document_quarantine_immutable BEFORE UPDATE OR DELETE ON chartworks.document_quarantine FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();

CREATE FUNCTION chartworks.protect_document_head() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR ROW(NEW.tenant_id,NEW.kind,NEW.document_id,NEW.created_at) IS DISTINCT FROM ROW(OLD.tenant_id,OLD.kind,OLD.document_id,OLD.created_at)
 OR NEW.version<>OLD.version+1 OR NEW.latest_revision NOT BETWEEN OLD.latest_revision AND OLD.latest_revision+1 OR (OLD.archived AND NOT NEW.archived)
 THEN RAISE EXCEPTION 'invalid document head transition' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER document_head_guard BEFORE UPDATE OR DELETE ON chartworks.document_heads FOR EACH ROW EXECUTE FUNCTION chartworks.protect_document_head();

-- Extend the existing exact action constraint without replacing earlier actions.
DO $$ DECLARE previous text; BEGIN
 SELECT pg_get_expr(conbin,conrelid) INTO STRICT previous FROM pg_constraint
 WHERE conrelid='chartworks.audit_events'::regclass AND conname='audit_events_action_check';
 ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
 EXECUTE format('ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK ((%s) OR action IN (''document.created'',''document.edited'',''document.reviewed'',''document.published'',''document.rejected'',''document.archived'',''document.imported'',''document.quarantined''))', previous);
END $$;
