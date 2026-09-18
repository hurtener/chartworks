# CW-01 native timestamp follow-up

This is a narrow follow-up to the existing temporal-identity finding, not a new
routing or warehouse-support workstream. The inspected candidate was
`5cadf97a98db56390cb84372143e570195c5018c` on PR #23. The ordinary validated-read,
source-context, authority and publication gates are unchanged.

## Finding and correction

The native compatibility check still admitted MySQL `timestamp` as the reviewed
wall-clock `timestamp` type. The resulting predicate cast its date bounds to
`DATETIME`. MySQL `TIMESTAMP` is session-zone-sensitive whereas `DATETIME` is not;
treating them as interchangeable could shift a reviewed business window. The
[MySQL 8.4 time-zone contract](https://dev.mysql.com/doc/refman/8.4/en/time-zone-support.html)
describes conversion between session time and UTC for native `TIMESTAMP` values.

The current scalar binder does not implement an instant-aware MySQL `TIMESTAMP`
path. Its native timestamp family now returns `unsupported_constraint_type` from
the pre-provider validation boundary, regardless of whether the supplied policy
claims wall-clock or instant semantics. This is explicit insufficiency, not a
fallback to a different time meaning. MySQL `DATE` and `DATETIME` remain supported;
queries with no scalar clarification keep their existing behavior.

## Bounded regression and evidence

`TestBusinessMySQLTimestampRejectedBeforeBinding` exercises lower/uppercase and
fractional-precision native spellings, both declared temporal meanings, typed
pre-provider rejection, no partial SQL/parameters/receipt, and unchanged queries
without scalar constraints. `TestBusinessMySQLWallClockAndDatePreserved` checks
exact bounds and native casts for the supported date and wall-clock families.
These are synthetic adapter tests and do not claim new live-engine qualification.

The regression tests are part of the unchanged affected-package race and global
coverage jobs. Their actual result must be recorded against the resulting commit
and test-merge SHA in PR #23; previous successful runs are not evidence for this
follow-up. No fixture, lint rule, coverage threshold or execution gate is relaxed.

The remaining delivered scope and explicit limitations are in the
[clarification contract](../contracts/conditional-clarification-v1.md) and
[bounded delivery review](cw-01-delivery-review.md). No merge is performed here.
