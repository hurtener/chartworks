# CW-01 native-type and publication follow-up

This record describes the scoped source correction and its regression checks.
It is not a claim that a workflow ran: exact-head Actions results on PR #23 are
execution evidence. No merge, human approval or live cloud qualification is implied.

## Native SQL Server identity

SQL Server `timestamp` is a synonym for binary `rowversion`, not a calendar value.
The scalar admission path now rejects both native identities for every calendar
constraint before model work and again before SQL binding. An advertised temporal
category does not override native identity. Rejection is the existing typed
`unsupported_constraint_type` failure on `target` and returns a completely empty
bound-query object: no partial SQL, parameters, provenance or read proof.

`TestBusinessSQLServerRowversionRejectedBeforeBinding` covers both native spellings,
case normalization, all three temporal declarations and preservation of ordinary
queries without scalar constraints. `TestBusinessSQLServerCalendarTypesPreserved`
checks the existing DATE, DATETIME, DATETIME2 and DATETIMEOFFSET paths, exact bounds,
parameter positions and provenance without fabricated validator proof.
`TestBusinessPostgresNativeTimestampStillSupported` guards against accidentally
rejecting the PostgreSQL type globally. Existing MySQL instant/wall-clock tests
remain unchanged.

Primary native-type reference:
https://learn.microsoft.com/en-us/sql/t-sql/data-types/rowversion-transact-sql

## Protected publication export

The failed `TestPhase15/AC02` assertion in run 35286593649 still classified Export
as an invalid publication-access enumeration. CW-01 added a protected published
export consumer. Keep that supported path and its existing action/resource and
source-partition enforcement, rather than removing export to satisfy a stale test.

The lifecycle regression now rejects genuinely invalid access values (0 and 255),
checks that an unauthorized exporter is denied, and preserves not-found behavior
for an authorized export before publication. After publication it checks the exact
retained version and independently denies a missing export action or topic-export
permission. No production access check is removed or broadened by this correction.

## Cumulative coverage execution capacity

The same failed run subsequently reached the acceptance package's 20-minute
process timeout while `TestTopicDraftCommitFencesAndErasure` had just started
(reported elapsed 0s). This was cumulative race/coverpkg execution, not evidence
that the newly starting test had hung. That interrupted profile is not valid
passing coverage evidence.

Allow the full package 30 minutes, retaining the existing 60-minute aggregate
coverage command and 90-minute CI job bounds. Every package and test remains
selected; race instrumentation, atomic coverage, exact package inventory, all
coverage percentages, failure propagation and the later acceptance/security/
preflight checks are unchanged. A timeout or failed test still fails the gate.

Source preparation and diagnostics stay on a separate tooling branch. They are
not in the proposed PR tree, and final CI tests committed source with read-only
repository permissions. The executable mode of the existing coverage script is
preserved.
