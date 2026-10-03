-- Retained authoring advisories are private evidence, never publication approval.
-- Existing manual/legacy drafts keep their explicit human review policy.
ALTER TABLE chartworks.topic_generation_checkpoints
 ADD COLUMN authoring_digest text NOT NULL DEFAULT '' CHECK(authoring_digest='' OR authoring_digest ~ '^[0-9a-f]{64}$'),
 ADD COLUMN quality_review jsonb CHECK(quality_review IS NULL OR (complete AND jsonb_typeof(quality_review)='object' AND octet_length(quality_review::text)<=262144)),
 ADD CONSTRAINT topic_authoring_quality_complete CHECK(authoring_digest='' OR NOT complete OR quality_review IS NOT NULL);
