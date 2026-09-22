-- New reporting captures persist complete per-topic template selection evidence.
-- Existing singular template records remain immutable compatibility input, but a
-- revision may never mix the legacy and current representations.
ALTER TABLE chartworks.block_revisions
 ADD CONSTRAINT block_template_selections_shape CHECK (
  NOT (definition ? 'template' AND definition ? 'templates') AND
  (NOT (definition ? 'templates') OR
   (jsonb_typeof(definition->'templates')='array' AND
    jsonb_array_length(definition->'templates') BETWEEN 1 AND 8 AND
    octet_length((definition->'templates')::text)<=32768))
 );
