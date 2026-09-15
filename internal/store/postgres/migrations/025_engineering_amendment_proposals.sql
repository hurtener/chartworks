-- Drift evidence and a new independently reviewed proposal have distinct identity.
CREATE TABLE chartworks.engineering_amendment_proposals (
 tenant_id text NOT NULL,
 amendment_id text NOT NULL,
 proposal_id text NOT NULL,
 parent_id text NOT NULL,
 parent_revision bigint NOT NULL,
 parent_digest text NOT NULL,
 PRIMARY KEY(tenant_id,amendment_id),
 UNIQUE(tenant_id,proposal_id),
 FOREIGN KEY(tenant_id,amendment_id) REFERENCES chartworks.engineering_amendments(tenant_id,amendment_id),
 FOREIGN KEY(tenant_id,proposal_id) REFERENCES chartworks.engineering_proposal_heads(tenant_id,proposal_id),
 FOREIGN KEY(tenant_id,parent_id,parent_revision,parent_digest) REFERENCES chartworks.engineering_proposal_versions(tenant_id,proposal_id,revision,digest),
 CHECK(proposal_id<>parent_id)
);
CREATE TRIGGER engineering_amendment_proposal_immutable BEFORE UPDATE ON chartworks.engineering_amendment_proposals FOR EACH ROW EXECUTE FUNCTION chartworks.reject_revision_update();
