-- Explicit non-sensitive business vocabulary is retained with the immutable
-- authoring checkpoint. It is caller input, not discovered warehouse membership.
ALTER TABLE chartworks.topic_generation_checkpoints
 ADD COLUMN vocabulary jsonb NOT NULL DEFAULT '[]'::jsonb
 CHECK(jsonb_typeof(vocabulary)='array' AND jsonb_array_length(vocabulary)<=32 AND octet_length(vocabulary::text)<=32768);
