-- Repair admission capacity without rewriting any accepted operation or applied
-- migration. Nested work cannot be claimed through the ordinary root runner:
-- a pending/retry/unleased child can only execute under a live parent, whose
-- root slot is already counted. A cancelled/terminal parent must not leave an
-- unclaimable child consuming admission capacity indefinitely.
--
-- Do not infer that cancellation stopped external work. A child with a live
-- lease and no matching live parent fence remains an independent root until
-- that lease expires, exactly as in active_execution_roots. This also covers
-- a parent being retried/reclaimed while an old-fence child is still running.
-- Ordinary root accounting, tenant predicates, authority, manifests, attempt
-- history and the shared admission lock are unchanged.
CREATE OR REPLACE VIEW chartworks.pending_execution_roots AS
 SELECT o.tenant_id,o.operation_id FROM chartworks.operations o
 WHERE o.dispatch_mode IN('queued','request') AND o.status IN('pending','retry','running')
 AND (o.nested_parent IS NULL OR
  (o.status='running' AND o.lease_until>clock_timestamp() AND o.expires_at>clock_timestamp()
   AND NOT EXISTS(SELECT 1 FROM chartworks.operations p
    WHERE p.tenant_id=o.tenant_id AND p.operation_id=o.nested_parent
    AND p.nested_parent IS NULL AND p.status='running' AND p.fence=o.nested_fence
    AND p.lease_until>clock_timestamp() AND p.expires_at>clock_timestamp())));
