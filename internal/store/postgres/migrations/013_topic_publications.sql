CREATE TABLE chartworks.topic_reviews (
 tenant_id text NOT NULL, review_id text NOT NULL CHECK(review_id ~ '^[a-f0-9]{32}$'),
 topic_id text NOT NULL, draft_revision bigint NOT NULL, digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 decision text NOT NULL CHECK(decision IN('approve','reject')),
 note text NOT NULL CHECK(octet_length(note) BETWEEN 1 AND 1024),
 actor_id text NOT NULL, session_id text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,review_id), UNIQUE(tenant_id,topic_id,review_id),
 FOREIGN KEY(tenant_id,topic_id,draft_revision) REFERENCES chartworks.topic_draft_versions(tenant_id,topic_id,revision)
);
CREATE TABLE chartworks.topic_publication_heads (
 tenant_id text NOT NULL, topic_id text NOT NULL,
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0 AND revision<4611686018427387904),
 active_version text, archived boolean NOT NULL DEFAULT false,
 PRIMARY KEY(tenant_id,topic_id),
 FOREIGN KEY(tenant_id,topic_id) REFERENCES chartworks.topic_draft_heads(tenant_id,topic_id)
);
CREATE TABLE chartworks.topic_published_versions (
 tenant_id text NOT NULL, topic_id text NOT NULL, version_id text NOT NULL,
 draft_revision bigint NOT NULL, review_id text NOT NULL,
 digest text NOT NULL CHECK(digest ~ '^[a-f0-9]{64}$'),
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object' AND octet_length(definition::text)<=2097152),
 receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=65536),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id) REFERENCES chartworks.topic_publication_heads(tenant_id,topic_id),
 FOREIGN KEY(tenant_id,topic_id,draft_revision) REFERENCES chartworks.topic_draft_versions(tenant_id,topic_id,revision),
 FOREIGN KEY(tenant_id,topic_id,review_id) REFERENCES chartworks.topic_reviews(tenant_id,topic_id,review_id)
);
ALTER TABLE chartworks.topic_publication_heads ADD FOREIGN KEY(tenant_id,topic_id,active_version)
 REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE chartworks.topic_published_dependencies (
 tenant_id text NOT NULL, topic_id text NOT NULL, version_id text NOT NULL,
 dataset_id text NOT NULL, source_id text NOT NULL, context_id text NOT NULL, source_revision bigint NOT NULL,
 PRIMARY KEY(tenant_id,topic_id,version_id,dataset_id),
 FOREIGN KEY(tenant_id,topic_id,version_id) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,source_id,source_revision) REFERENCES chartworks.source_revisions(tenant_id,source_id,revision)
);
CREATE TABLE chartworks.topic_published_generations (
 tenant_id text NOT NULL, topic_id text NOT NULL, version_id text NOT NULL,
 context_id text NOT NULL, generation_id text NOT NULL,
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object' AND octet_length(manifest::text)<=1048576),
 PRIMARY KEY(tenant_id,topic_id,version_id,context_id),
 FOREIGN KEY(tenant_id,topic_id,version_id) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id,context_id,generation_id) REFERENCES chartworks.vector_generations(tenant_id,topic_id,context_id,generation_id)
);
CREATE TABLE chartworks.topic_publication_events (
 tenant_id text NOT NULL, topic_id text NOT NULL, revision bigint NOT NULL,
 version_id text NOT NULL, archived boolean NOT NULL,
 kind text NOT NULL CHECK(kind IN('publish','rollback','archive')),
 actor_id text NOT NULL, session_id text NOT NULL,
 note text NOT NULL CHECK(octet_length(note) BETWEEN 1 AND 1024),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,topic_id,revision),
 FOREIGN KEY(tenant_id,topic_id,version_id) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id)
);
CREATE TRIGGER topic_review_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_reviews FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_published_version_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_published_versions FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_published_dependency_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_published_dependencies FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_published_generation_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_published_generations FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_publication_event_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_publication_events FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();

-- Final-state consistency also guards existing standalone vector operations.
-- A topic transition must change every affected pointer in the same transaction.
CREATE FUNCTION chartworks.check_topic_facet_publication() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_tenant text; target_topic text; head record;
BEGIN
 IF TG_OP='DELETE' THEN target_tenant=OLD.tenant_id;target_topic=OLD.topic_id;
 ELSE target_tenant=NEW.tenant_id;target_topic=NEW.topic_id; END IF;
 SELECT active_version,archived INTO head FROM chartworks.topic_publication_heads WHERE tenant_id=target_tenant AND topic_id=target_topic;
 IF NOT FOUND THEN RETURN NULL; END IF;
 IF EXISTS (
  SELECT 1 FROM chartworks.topic_published_generations m
  JOIN chartworks.vector_generations g ON(g.tenant_id,g.topic_id,g.context_id,g.generation_id)=(m.tenant_id,m.topic_id,m.context_id,m.generation_id)
  LEFT JOIN chartworks.vector_heads h ON(h.tenant_id,h.topic_id,h.context_id)=(m.tenant_id,m.topic_id,m.context_id)
  WHERE m.tenant_id=target_tenant AND m.topic_id=target_topic AND m.version_id=head.active_version
   AND (g.state<>'ready' OR g.version_id<>head.active_version OR h.active_generation IS DISTINCT FROM m.generation_id OR h.archived IS DISTINCT FROM head.archived)
 ) OR EXISTS (
  SELECT 1 FROM chartworks.vector_heads h WHERE h.tenant_id=target_tenant AND h.topic_id=target_topic
   AND NOT h.archived AND h.active_generation IS NOT NULL AND (head.archived OR NOT EXISTS (
    SELECT 1 FROM chartworks.topic_published_generations m WHERE m.tenant_id=target_tenant AND m.topic_id=target_topic
    AND m.version_id=head.active_version AND m.context_id=h.context_id AND m.generation_id=h.active_generation))
 ) OR (head.active_version IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM chartworks.topic_published_generations m WHERE m.tenant_id=target_tenant AND m.topic_id=target_topic AND m.version_id=head.active_version
 )) THEN RAISE EXCEPTION 'topic and facet publication must match' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER topic_facet_head_consistency AFTER INSERT OR UPDATE OR DELETE ON chartworks.topic_publication_heads DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION chartworks.check_topic_facet_publication();
CREATE CONSTRAINT TRIGGER vector_topic_head_consistency AFTER INSERT OR UPDATE OR DELETE ON chartworks.vector_heads DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION chartworks.check_topic_facet_publication();
CREATE CONSTRAINT TRIGGER topic_generation_consistency AFTER INSERT OR UPDATE OR DELETE ON chartworks.topic_published_generations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION chartworks.check_topic_facet_publication();

ALTER TABLE chartworks.audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE chartworks.audit_events ADD CONSTRAINT audit_events_action_check CHECK(action IN('topic.reviewed','topic.published','topic.rolled_back','topic.archived','topic.drafted','pipeline.drafted','pipeline.published','pipeline.staged','pipeline.effect','pipeline.activated','pipeline.reconciled',
 'retention_policy.updated','retention.sweep','job.accepted','job.cancelled','schedule.created','schedule.updated','schedule.fired',
 'facets.generation_staged','facets.generation_published','facets.archived','facets.erased','source.created','source.rotated',
 'read.accepted','read.cancel_requested','read.succeeded','read.empty','read.truncated','read.cancelled','read.timed_out','read.failed','read.uncertain','read.interrupted',
 'request.accepted','request.resumed','request.cancelled','request.succeeded',
 'upload.reserved','upload.staged','upload.activated','upload.erasure_requested','upload.erased',
 'profile.reserved','profile.checkpoint','profile.summary_started','profile.published','profile.dependency_registered','profile.health_changed','profile.erased'
));
