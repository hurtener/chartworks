# Phase 26/28 runtime completion evidence

Status: implementation and direct adversarial review in progress. No independent
subagents were used, as requested by the owner. This document is not a runtime
completion claim.

The starting branch `1d412d9` contained domain services, PostgreSQL migrations and
named acceptance tests, but lacked application wiring, registered HTTP operations
and SDK integration. The runtime completion adds those consumers and changes the
phase ledger to `in_progress` so cumulative preflight cannot skip them.

Initial real PostgreSQL Phase 28 execution found an output-order mismatch and a
narrative test fixture that put the same field in both allowed and redacted lists.
The frozen lane now seals explicit request output order without changing the
existing authoring-preview order contract. The narrative fixture uses disjoint
lists. HTTP testing also exposed empty enum values in default run requests;
optional policy fields now omit empty values so the core's defaults apply.

Verification environment: macOS arm64, Go 1.26.4, PostgreSQL 17.11 and pgvector
0.8.2, native pinned SQL parser. Docker returned a local storage I/O error and its
existing PostgreSQL fixture was unreachable, so an isolated native database was
created for read-path tests. The managed runner intentionally requires Linux
process isolation and executable tmpfs; its evidence must come from Linux CI.

Phase 28 local race acceptance passed all eight criteria after the fixes, including
HTTP/SDK admission, execution, paging, cancellation and three additional lost-reply
checkpoints. This was a working-tree run, not hosted exact-commit evidence.

Additional review fixes: PostgreSQL preserves the new safe typed domain errors;
drift can produce an independently reviewable amendment; physical source probes
detect unregistered schema changes; narrative arithmetic preserves scientific
notation and bounds exponent allocation. Regression tests accompany these changes.

Pending: Phase 26 Linux runtime acceptance, direct adversarial review completion,
coverage, full cumulative checks and exact committed-source hosted CI. Phase 25,
34, live provider quality and production deployment are not claimed.

## Continued scope and contract review

At `2903e5c`, the cumulative phase-21 protected-operation checks pass locally
with the new runtime registry, including both verifier and dispatch authentication
errors. The runtime operation manifest has a parity assertion against the actual
registration. Local lint reports zero issues. Phase-28 real-PostgreSQL acceptance
also exercises schema drift before physical execution, preview privacy after
publication, receipt/summary reads and closed HTTP request shapes.

Full phase-26 closure remains open: its task 1 includes proposed topic and schedule
changes, while the current proposal model and validator admit only one pipeline
and its managed dataset. The existing durable scheduling service currently admits
only the maintenance target; request-driven pipeline execution is functional but
is not an unattended schedule handler. This is a concrete runtime gap, not a
reason to mark the task complete based on the six named tests. Any added consumer
must preserve ordinary topic review/publication and Pengui execution authority,
and must not advertise a schedule target without its real handler.

The passing local unit plus `TestPhase21`, `TestPhase27`, and `TestPhase28`
race-instrumented runs cover 2,007/2,508 reporting statements (80.02%) and
323/390 reporting API statements (82.82%). These are combined statement unions
from the same production source, not sums of percentages or full CI results.
The new reporting model-policy configuration tests also pass. Hosted Linux
phase-26 acceptance and full repository coverage remain pending.

A further direct inspection found drift-impact discovery checked target read
reach without all dependencies. The fix filters every block reference and every
topic source/dataset/context in PostgreSQL and requires the corresponding read
action before returning impact IDs. AC05 now publishes a real two-context topic
and checks both authorized inclusion and exclusion under missing context/action,
including replay of the same deduplicated observation. The added acceptance
fixture compiles; local lint is clean. Its Linux execution is still pending.

## Topic integration continuation

L2 now accepts an optional profile-backed private topic change, retains it in the
reviewed material, supports explicit semantic edits and saves through the existing
draft service after successful managed execution. Migration 026 extends the
existing effect/reference ledger; final completion verifies the actual private
draft. Local PostgreSQL tests cover no-write preparation, exact evidence checks,
normal save, lost-reply reconciliation and refusal to adopt another proposal's
private effect. Full phase-26 topic apply is included in AC06 for Linux CI.

The first hosted Linux run at c06a0e4 passed AC01, AC02, AC04, AC05 and AC06.
AC03 failed because its SDK assertion expected an internal domain sentinel rather
than the actual HTTP 409; the assertion now checks the SDK status. This was a
failed CI run, not phase completion. Current-head acceptance and coverage remain
required. The focused runtime step now precedes the expensive reference-container
build, with all container and full-suite gates retained.

Follow-up checks found a stale migration inventory assertion (22 rather than 26);
it is updated to the actual final migration identity, and the affected store/API/
SDK race tests pass. AC08 now observes a real blocked warehouse relation before
cancelling through HTTP; the request terminates without result exposure. That
PostgreSQL test passes locally. Topic amendments now advance draft revision only
for a recorded committed topic effect and preserve reviewed names/entities in the
new independent material. Explicit fresh-profile selection updates only evidence
coordinates and still requires ordinary semantic/source validation.

Hosted CI run 34542894136 at 27f98f24092bb82eb8ffdd8a6daf7cfbd69bd653
completed the combined strict phase-26/28 runtime acceptance step successfully
(2026-09-10 23:51:03 UTC). This includes the first real Linux reviewed-topic apply.
The subsequent cancellation and topic-amendment fixes at 58e4bf5 have their own
pending CI run; neither result substitutes for final-head full coverage or the
remaining schedule integration.

Hosted CI run 34543896878 at 58e4bf5bf98bae94a979f6a60b1760bba9097b31
also passed the strict combined runtime acceptance step (2026-09-11 00:03:50
UTC), including the running-cancellation and topic-amendment additions. The
full build/container/coverage sequence was still running when inspected.

## Scheduling authority inspection

Pipeline occurrence timing, retries and execution belong to Chartworks. The
platform's prompt schedules are a separate consumer and do not implement this
requirement. The currently consumed execution-authority companion remains
unmerged and permits only retention maintenance. Inspection of the platform's
ordinary capability mint found a real-user scope projection and a separate
connection bootstrap identity; neither supplies the existing manifest-bound
execution proof required by this repository. Reusing a connection bearer or
persisting the initiating user's token would not satisfy the active contract.
No platform scheduling feature or authorization change has been implemented as
part of this inspection. Scheduled pipeline execution remains incomplete.


## Scheduled execution and reviewed recurrence continuation

The strict Linux phase-26/28 runtime acceptance step passed at c31118a6e726fefc9548cdb66fb1e2625063b2dc
in run 34546295009 (2026-09-11 00:38:27 UTC), including native scheduled pipeline
execution and exact occurrence replay. Its client job found the optional target's
request schema still using the pointer-free schema mode. That registration is
corrected; the actual assembly lifecycle passes locally. Two lint findings are
also corrected.

The following L2 stage adds reviewed schedule creation/replacement and final
actual-effect verification. Schedule edits retain the immutable initial request,
and amendments address the previously committed schedule revision. Local real-PG
race acceptance proves replacement retry reconciliation and preservation of
accepted occurrences; phase-21/28 regression acceptance and focused core tests
pass. Lint reports zero issues. The newly extended native L2 schedule test compiles
but requires its own hosted execution before completion can be claimed.

## Cumulative gates and admission deadline review

The full run at 58e4bf5 failed on an outdated table inventory and an unregistered
pipeline input fixture, then reached the cumulative ten-minute Go package timeout.
The corrected PostgreSQL inventory/atomicity tests pass with race instrumentation
at c697c07. The cumulative allowance is now twenty minutes; coverage thresholds,
test selection and failure handling are unchanged. The coverage parser's six
unit tests pass.

The 0955bbe client and MCP workflows pass. Its Linux runtime step rejects the new
reviewed-schedule fixture before apply because it requests more than 32 scopes.
The fixture now omits unrelated topic and compensation scopes; the production
bound is unchanged. Native execution of that corrected fixture remains required.

Direct authority review also found ordinary queue admission could wait beyond
bearer expiry before writing. Submit, schedule creation, replacement, pause/resume
and manual fire now carry the signed deadline through their complete call. The
real PostgreSQL regression observes admission waiting on the queue lock, releases
it after expiry, and verifies that neither jobs nor schedules were admitted.
That regression and existing Phase 06/replacement race acceptance pass locally.
