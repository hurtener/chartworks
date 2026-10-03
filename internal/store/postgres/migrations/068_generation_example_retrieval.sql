-- Query-aware retrieval must apply current eligibility before bounded selection.
-- Existing example contents, review state and immutable origins are unchanged.
CREATE INDEX nlq_examples_generation_origin ON chartworks.nlq_examples
 (tenant_id,topic_id,(origin->>'context'),(origin->>'topic_version'),(origin->>'source_binding_digest'),weight DESC,example_id)
 WHERE state='active';
CREATE INDEX nlq_examples_generation_search ON chartworks.nlq_examples
 USING gin(to_tsvector('simple',question)) WHERE state='active';
