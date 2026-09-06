# Read execution implementation decision

### D-065 — One bounded validated read core with explicit attempt uncertainty · accepted

Continue D-064 and the existing phase-09 validator; do not replace its opaque
plans or weaken any acceptance criterion. Phase 10 extends the concrete source
adapter with cursor execution and supplies the HTTP/SDK consumer, a bounded
content-free attempt journal, explicit cancel/reconciliation and exact typed data.
Pengui remains the sole issuer/policy owner. No new identity, query scheduling,
model or result-retention subsystem is introduced.

Use the actual PostgreSQL 17 read-only transaction and catalog proof, native
DECLARE/FETCH on unchanged SQL, server/client/JWT deadlines and actual ordered
result types. The source-revision fence must last through the bounded native
execution and cleanup, not inherit the five-second metadata default. Keep
ordinary metadata deadlines short and reserve journal/control connection capacity.

Persist cancellation intent and let only the live owner signal its original
connection, avoiding cross-request cancellation after PID reuse. Acknowledged
rollback or exact tagged backend observation proves termination; loss of the
response/socket alone does not. Reconcile uncertainty before an explicit numbered
retry, without reconstructing lost values. The final outcome/audit commit honors
late cancellation. Fixed result errors discard partial values rather than coercing
them or broadening an empty query.

Exact decimals/integers/money travel as strings, booleans/NULL retain their JSON
types, and structured JSON travels as text to preserve embedded exact numbers.
Row/serialized-byte ceilings apply to every consumer. Optimizer-unit admission is
not a scan-byte/billing guarantee; actual scan bytes remain unknown. This qualified
contract and its unsupported cases are documented in
[read-execution.md](../contracts/read-execution.md).

A 24-hour bounded content-free journal supports lost-response recovery and replay
protection. It is not the phase-28 retained result store or the phase-06 queue.
Reporting/scheduled consumers arrive with their actual fresh-authority targets in
later phases; no unimplemented target is advertised. Applied migrations 001–005
remain unchanged; new migration 006 accompanies these consumers and regressions.
