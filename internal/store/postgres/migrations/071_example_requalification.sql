-- A fresh qualification retains both review origins without changing an old row.
ALTER TABLE chartworks.nlq_examples ADD CONSTRAINT nlq_example_requalification_shape CHECK (
 NOT (origin ? 'requalification') OR COALESCE((
  jsonb_typeof(origin->'requalification')='object' AND
  origin->'requalification'->>'policy'='current-semantic-example-review-v1' AND
  origin->'requalification'->>'example_id' ~ '^[a-f0-9]{32}$' AND
  (origin->'requalification'->>'version')::bigint > 0 AND
  origin->'requalification'->>'digest' ~ '^[a-f0-9]{64}$' AND
  origin->'requalification'->>'origin_digest' ~ '^[a-f0-9]{64}$' AND
  origin->'requalification'->>'contract_digest' ~ '^[a-f0-9]{64}$'
 ),false));
