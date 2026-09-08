-- Migration 017 closed the NLQ status set before the executor's terminal
-- cancellation classifications were persisted. Keep the applied migration
-- immutable and widen it forward for all registered terminal statuses.
ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT IF EXISTS nlq_queries_status_check;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_queries_status_check CHECK(status IN('preflight','planned','running','succeeded','empty','truncated','failed','uncertain','cancelled','timed_out','interrupted'));

-- The executor's 16 MiB cap counts schema/row payload bytes before the
-- durable JSON envelope adds field names and receipt framing. Retain a small,
-- bounded envelope allowance while keeping the result far below unbounded
-- metadata storage.
ALTER TABLE chartworks.nlq_queries DROP CONSTRAINT IF EXISTS nlq_queries_result_check;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_queries_result_check CHECK(result IS NULL OR (jsonb_typeof(result)='object' AND octet_length(result::text)<=16908288));
