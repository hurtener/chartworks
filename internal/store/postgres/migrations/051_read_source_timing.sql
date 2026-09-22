-- Physical-source timing belongs to one read attempt. Prior attempts remain
-- unknown; neither journal wall time nor a legacy timestamp span is backfilled.
ALTER TABLE chartworks.read_attempts
 ADD COLUMN source_duration_ns bigint
 CHECK (source_duration_ns IS NULL OR source_duration_ns BETWEEN 1 AND 300000000000);
