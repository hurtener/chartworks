CREATE TABLE chartworks.canonical_entity_heads (
 tenant_id text NOT NULL REFERENCES chartworks.policies(tenant_id),
 entity_id text NOT NULL CHECK(entity_id ~ '^[A-Za-z0-9_.:-]{1,128}$'),
 current_revision bigint NOT NULL CHECK(current_revision>0 AND current_revision<4611686018427387904),
 PRIMARY KEY(tenant_id,entity_id)
);
CREATE TABLE chartworks.canonical_entity_revisions (
 tenant_id text NOT NULL, entity_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision>0 AND revision<4611686018427387904),
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 meaning jsonb NOT NULL CHECK(jsonb_typeof(meaning)='object' AND octet_length(meaning::text)<=65536),
 approving_topic_id text NOT NULL, approving_version_id text NOT NULL, review_id text NOT NULL,
 actor_id text NOT NULL, session_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,entity_id,revision),
 UNIQUE(tenant_id,entity_id,revision,digest),
 FOREIGN KEY(tenant_id,entity_id) REFERENCES chartworks.canonical_entity_heads(tenant_id,entity_id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(tenant_id,approving_topic_id,approving_version_id) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,approving_topic_id,review_id) REFERENCES chartworks.topic_reviews(tenant_id,topic_id,review_id)
);
ALTER TABLE chartworks.canonical_entity_heads ADD CONSTRAINT canonical_entity_current_revision_fk
 FOREIGN KEY(tenant_id,entity_id,current_revision) REFERENCES chartworks.canonical_entity_revisions(tenant_id,entity_id,revision)
 DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE chartworks.canonical_entity_terms (
 tenant_id text NOT NULL, normalized_term text NOT NULL CHECK(octet_length(normalized_term) BETWEEN 1 AND 1024),
 entity_id text NOT NULL, first_revision bigint NOT NULL,
 PRIMARY KEY(tenant_id,normalized_term),
 FOREIGN KEY(tenant_id,entity_id,first_revision) REFERENCES chartworks.canonical_entity_revisions(tenant_id,entity_id,revision)
);
CREATE TABLE chartworks.topic_published_canonical_refs (
 tenant_id text NOT NULL, topic_id text NOT NULL, version_id text NOT NULL,
 entity_id text NOT NULL, revision bigint NOT NULL, digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(tenant_id,topic_id,version_id,entity_id),
 FOREIGN KEY(tenant_id,topic_id,version_id) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,entity_id,revision,digest) REFERENCES chartworks.canonical_entity_revisions(tenant_id,entity_id,revision,digest)
);

CREATE FUNCTION chartworks.protect_canonical_head() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' OR (TG_OP='INSERT' AND NEW.current_revision<>1) OR
    (TG_OP='UPDATE' AND (NEW.tenant_id,NEW.entity_id)!=(OLD.tenant_id,OLD.entity_id)) OR
    (TG_OP='UPDATE' AND NEW.current_revision<>OLD.current_revision+1)
 THEN RAISE EXCEPTION 'canonical entity head must advance exactly once' USING ERRCODE='55000'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER canonical_entity_head_fence BEFORE INSERT OR UPDATE OR DELETE ON chartworks.canonical_entity_heads FOR EACH ROW EXECUTE FUNCTION chartworks.protect_canonical_head();
CREATE TRIGGER canonical_entity_revision_immutable BEFORE UPDATE OR DELETE ON chartworks.canonical_entity_revisions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER canonical_entity_term_immutable BEFORE UPDATE OR DELETE ON chartworks.canonical_entity_terms FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_published_canonical_ref_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_published_canonical_refs FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
