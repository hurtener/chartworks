-- Closed source-rejection diagnostics. Historic generic query_error receipts
-- keep their original interpretation. Neither SQLSTATE nor raw driver text is
-- retained. The existing immutable terminal/manifest trigger stays unchanged.
ALTER TABLE chartworks.read_attempts DROP CONSTRAINT read_attempts_code_check;
ALTER TABLE chartworks.read_attempts ADD CONSTRAINT read_attempts_code_check
 CHECK (code IN ('','source_unavailable','invalid_result','cancelled','timed_out',
 'result_type_unsupported','limit_exceeded','context_changed','unsupported',
 'remote_outcome_unknown','result_not_retained','query_error',
 'query_division_by_zero','query_numeric_range','query_invalid_text',
 'query_invalid_datetime','query_datetime_range','query_cardinality',
 'query_function_signature','query_type_mismatch','query_grouping','query_windowing'));
ALTER TABLE chartworks.read_attempts ADD CONSTRAINT read_query_diagnostic_outcome
 CHECK (code NOT IN ('query_division_by_zero','query_numeric_range',
 'query_invalid_text','query_invalid_datetime','query_datetime_range',
 'query_cardinality','query_function_signature','query_type_mismatch',
 'query_grouping','query_windowing') OR
 (status='failed' AND rows_returned=0 AND bytes_returned=0 AND finished_at IS NOT NULL
  AND remote_state IN ('stopped','not_issued')));
-- not_issued may record a local pre-issue rejection but never authorizes repair.
-- The NLQ consumer still requires a confirmed stopped receipt for a retry.
