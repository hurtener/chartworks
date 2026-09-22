-- Immutable reporting rule dependencies. These rows contain only reviewed
-- version/digest coordinates; rule text remains in the rule publication store.
CREATE TABLE chartworks.block_rule_pins (
 tenant_id text NOT NULL,
 block_id text NOT NULL,
 revision bigint NOT NULL,
 topic_id text NOT NULL,
 topic_version text NOT NULL,
 pack_digest text NOT NULL CHECK(pack_digest ~ '^[a-f0-9]{64}$'),
 rule_version text NOT NULL,
 rule_digest text NOT NULL CHECK(rule_digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(tenant_id,block_id,revision,topic_id),
 FOREIGN KEY(tenant_id,block_id,revision) REFERENCES chartworks.block_revisions(tenant_id,block_id,revision),
 FOREIGN KEY(tenant_id,topic_id,rule_version) REFERENCES chartworks.topic_rule_published_versions(tenant_id,topic_id,version_id),
 FOREIGN KEY(tenant_id,topic_id,topic_version) REFERENCES chartworks.topic_published_versions(tenant_id,topic_id,version_id)
);
CREATE INDEX block_rule_pins_invalidation ON chartworks.block_rule_pins(tenant_id,topic_id,rule_version,block_id,revision);
CREATE TRIGGER block_rule_pins_immutable BEFORE UPDATE OR DELETE ON chartworks.block_rule_pins FOR EACH ROW EXECUTE FUNCTION chartworks.block_immutable();
