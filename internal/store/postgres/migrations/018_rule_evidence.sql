CREATE TABLE chartworks.topic_rule_comparison_evidence (
 tenant_id text NOT NULL,
 comparison_id text NOT NULL CHECK(comparison_id ~ '^[a-f0-9]{32}$'),
 actor_id text NOT NULL,
 session_id text NOT NULL,
 topic_id text NOT NULL,
 mode text NOT NULL CHECK(mode IN('replay','shadow')),
 topic_version text NOT NULL,
 pack_digest text NOT NULL CHECK(pack_digest ~ '^[0-9a-f]{64}$'),
 references_json jsonb NOT NULL CHECK(jsonb_typeof(references_json)='array' AND jsonb_array_length(references_json) BETWEEN 1 AND 256 AND octet_length(references_json::text)<=262144),
 baseline_rule_version text NOT NULL,
 baseline_result jsonb NOT NULL CHECK(jsonb_typeof(baseline_result)='object' AND octet_length(baseline_result::text)<=262144),
 candidate_rule_version text,
 candidate_result jsonb CHECK(candidate_result IS NULL OR (jsonb_typeof(candidate_result)='object' AND octet_length(candidate_result::text)<=262144)),
 changed boolean NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,comparison_id),
 FOREIGN KEY(tenant_id,topic_id,topic_version) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id,baseline_rule_version) REFERENCES chartworks.topic_rule_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id,candidate_rule_version) REFERENCES chartworks.topic_rule_published_versions(tenant_id,topic_id,version_id),
 CHECK((mode='replay' AND candidate_rule_version IS NULL AND candidate_result IS NULL AND NOT changed) OR
       (mode='shadow' AND candidate_rule_version IS NOT NULL AND candidate_result IS NOT NULL))
);
CREATE INDEX topic_rule_comparison_topic ON chartworks.topic_rule_comparison_evidence(tenant_id,topic_id,created_at,comparison_id);

CREATE TABLE chartworks.topic_rule_evidence_invalidations (
 tenant_id text NOT NULL,
 invalidation_id text NOT NULL CHECK(invalidation_id ~ '^[a-f0-9]{32}$'),
 topic_id text NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 kind text NOT NULL CHECK(kind IN('publish','retire')),
 old_rule_version text,
 new_rule_version text,
 topic_version text NOT NULL,
 pack_digest text NOT NULL CHECK(pack_digest ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,invalidation_id),
 UNIQUE(tenant_id,topic_id,revision),
 FOREIGN KEY(tenant_id,topic_id,topic_version) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id,old_rule_version) REFERENCES chartworks.topic_rule_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id,new_rule_version) REFERENCES chartworks.topic_rule_published_versions(tenant_id,topic_id,version_id),
 CHECK((kind='publish' AND new_rule_version IS NOT NULL) OR (kind='retire' AND new_rule_version IS NULL))
);
CREATE INDEX topic_rule_invalidations_topic ON chartworks.topic_rule_evidence_invalidations(tenant_id,topic_id,revision);

CREATE TRIGGER topic_rule_comparison_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_rule_comparison_evidence FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
CREATE TRIGGER topic_rule_invalidation_immutable BEFORE UPDATE OR DELETE ON chartworks.topic_rule_evidence_invalidations FOR EACH ROW EXECUTE FUNCTION chartworks.protect_topic_draft();
