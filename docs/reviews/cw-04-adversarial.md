# CW-04 adversarial fix review

Date: 2026-09-22. Scope: PR #28 rich semantics and metric dependency context.

The consolidated dual review found five reachable contract failures. The fix round
kept the fast pull-request/manual final-gap CI split unchanged.

1. Direct-endpoint join selection omitted confirmed bridge joins. Routing now uses
   the confirmed join bridge forest, includes a unique connecting subgraph and
   returns `nlqroute.ErrMetricContext` for disconnected or competing paths.
2. Governed values were unique only by ID and aliases only within one value.
   Compilation now rejects normalized primary/alias collisions across the complete
   dimension vocabulary, including Unicode-equivalent spellings.
3. Profile rebind discarded sensitivity. It now preserves classification only for
   the same source/context/physical column; otherwise it clears classification and
   atomically rejects any dependent governed values or literal filters.
4. Enhancement did not tell the gateway which deterministic generated metric IDs
   were legal. The sealed request now carries a bounded deterministic catalog;
   output validation requires current-page IDs to be actual measure results.
5. Relationship endpoints could escape the supplied page, while KPI/relationship
   retries blindly appended. Endpoints are page-local; exact retries and KPI repeats
   are no-ops, cross-page relationship evidence rejects, and same-ID changes fail.

Focused tests cover a three-dataset bridge, disconnected and competing graphs,
cycles/order, mandatory budget behavior, all primary/alias collision classes,
rebind preservation and rollback, same-step KPI creation, unknown/future IDs,
pagination/catalog budget, page-local relationships and retry conflicts. Native
parser, real PostgreSQL, race, fuzz, full coverage and release qualification remain
owned by the manual final-gap workflow.
