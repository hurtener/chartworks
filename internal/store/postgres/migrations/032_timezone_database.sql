-- Existing accepted due/window instants are untouched. New workers agree on
-- the exact timezone archive before accepting or claiming more occurrences.
ALTER TABLE chartworks.queue_limits ADD COLUMN timezone_database text;
ALTER TABLE chartworks.queue_limits ADD CONSTRAINT queue_timezone_version CHECK
 (timezone_database IS NULL OR timezone_database ~ '^go[0-9]+\.[0-9]+\.[0-9]+-sha256:[a-f0-9]{64}$');
