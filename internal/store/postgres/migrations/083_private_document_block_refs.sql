-- Private block references are separate immutable custody. Existing published
-- document_block_refs and their block_publications foreign key stay unchanged.
CREATE TABLE chartworks.document_private_block_refs (
 tenant_id text NOT NULL,
 kind text NOT NULL CHECK(kind='report'),
 document_id text NOT NULL,
 revision bigint NOT NULL,
 widget_id text NOT NULL CHECK(widget_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 block_id text NOT NULL,
 block_revision bigint NOT NULL CHECK(block_revision BETWEEN 1 AND 256),
 definition_digest text NOT NULL CHECK(definition_digest ~ '^[a-f0-9]{64}$'),
 block_actor_id text NOT NULL CHECK(block_actor_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 PRIMARY KEY(tenant_id,kind,document_id,revision,widget_id),
 FOREIGN KEY(tenant_id,kind,document_id,revision) REFERENCES chartworks.document_revisions(tenant_id,kind,document_id,revision),
 FOREIGN KEY(tenant_id,block_id,block_revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision)
);
CREATE TRIGGER document_private_block_ref_immutable BEFORE UPDATE OR DELETE ON chartworks.document_private_block_refs
 FOR EACH ROW EXECUTE FUNCTION chartworks.immutable_document_row();
CREATE FUNCTION chartworks.reject_private_document_publication() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF EXISTS(SELECT 1 FROM chartworks.document_private_block_refs p
  WHERE (p.tenant_id,p.kind,p.document_id,p.revision)=(NEW.tenant_id,NEW.kind,NEW.document_id,NEW.revision))
 THEN RAISE EXCEPTION 'private block reference cannot be published' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER document_private_publication_guard BEFORE INSERT ON chartworks.document_publications
 FOR EACH ROW EXECUTE FUNCTION chartworks.reject_private_document_publication();
CREATE TRIGGER document_private_review_guard BEFORE INSERT ON chartworks.document_events
 FOR EACH ROW WHEN (NEW.operation='review') EXECUTE FUNCTION chartworks.reject_private_document_publication();
