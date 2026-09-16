-- Existing definitions, hashes, publications and manifests remain immutable.
-- V1 defaults are projected at read time; upgrades create ordinary reviewed
-- draft revisions. No UPDATE rewrites published JSON or accepted snapshots.
ALTER TABLE chartworks.block_revisions
 ADD CONSTRAINT block_definition_version CHECK ((definition->>'schema_version')::integer IN (1,2));
ALTER TABLE chartworks.frozen_runs DROP CONSTRAINT frozen_runs_frozen_version_check;
ALTER TABLE chartworks.frozen_runs
 ADD CONSTRAINT frozen_runs_frozen_version_check CHECK (frozen_version IN ('frozen-block-run-v1','frozen-block-run-v2'));
