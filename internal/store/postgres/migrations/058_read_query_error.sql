-- A validated source can fail on row values (for example division by zero).
-- The native executor already emits this detail-free code. Preserve it in the
-- durable journal instead of turning successful failure-finalization into an
-- uncertain metadata write. No old attempt, SQL or diagnostic text is rewritten.
ALTER TABLE chartworks.read_attempts DROP CONSTRAINT read_attempts_code_check;
ALTER TABLE chartworks.read_attempts ADD CONSTRAINT read_attempts_code_check
 CHECK (code IN ('','source_unavailable','invalid_result','cancelled','timed_out',
 'result_type_unsupported','limit_exceeded','context_changed','unsupported',
 'remote_outcome_unknown','result_not_retained','query_error'));
-- The immutable manifest/terminal-attempt trigger and status/remote-state
-- constraints remain intact. Only a confirmed stopped failure is repairable.
