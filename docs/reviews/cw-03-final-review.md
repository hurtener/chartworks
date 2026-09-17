# CW-03 final review — September 17, 2026

Status: follow-up source corrections with regression tests. Local structural
checks pass; **runtime validation and CI remain pending**. This record does not
certify green CI, merge readiness, or closure of the findings.

Repository baseline: `2219fa29093253e0c51b94c4b9de3a4e52f19ee1` (`main`).
PR #24 was already merged on September 16. Its delivered head was
`f6bc39d170ab1850c9c4d1b89627c42ad2514375`; the merge has the same source tree.
This follow-up targets that main commit, not the already merged PR branch.

## Observed final CI

- Final PR CI [35120624081](https://github.com/hurtener/chartworks/actions/runs/35120624081)
  failed during `FuzzActualVerifier`: `context deadline exceeded`. Its prior
  race/coverage and strict phase acceptance steps passed. The subsequent full
  development preflight did not execute successfully in that run.
- Merged-main CI [35162354547](https://github.com/hurtener/chartworks/actions/runs/35162354547)
  failed at the coverage gate: PostgreSQL store 83.98% (5,758 / 6,856 statements),
  required 84%. This is distinct from the PR fuzz failure. Later steps were skipped.
- CW-03, Clients, Reporting contracts, Reports and dashboards, and MCP workflows
  passed at the delivered PR head. Those historical passes are not executions of
  the new source corrections in this follow-up.

## Findings and prepared corrections

### CW03-FR-01 — P1: pending composition retries can bypass current child caps

A child run may complete before its parent's group checkpoint fails. The child's
terminal `Run` returns retained evidence without reentering `continueFrozen`.
The parent previously consumed it without `CheckRuntimeResult`, so a newly lowered
row or byte cap could be bypassed. The related window occurs after the group has
been checkpointed but before the parent commits final completion: previously
completed groups were skipped without the current-cap check.

The correction reuses `CheckRuntimeResult` in both paths. The parent intersects
accepted artifact limits with the current execution limits; the child's immutable
accepted limits still apply. Already checkpointed values are never erased or
rewritten. A pending parent returns a typed budget error rather than completing
above the current ceiling. Terminal artifact reads remain pure retained reads.
Existing live invocation fences, signed child authority and context checks remain
in use. No new identity, authority grant or warehouse/model execution is added.

`TestCW03CompletedChildRetryUsesCurrentCaps` injects real PostgreSQL transaction
failures at group checkpoint and final completion. It covers unchanged, lowered
row, and lowered byte caps at both boundaries, exact manifest/selection retention,
unchanged child outputs and query attempts, and zero repeated warehouse/model work.
An already checkpointed parent can complete after an explicit retry under its
original allowable ceiling, without regenerating the child. These tests are
written, not claimed passing.

### CW03-FR-02 — P2: independent output locale is replaced by block fallback

Block description localized the block resource first and then passed that resolved
block locale to output localization. An English-only block could therefore force
English output labels despite independently authored Spanish output metadata and
a Spanish request.

Pass the original requested locale to `viewerChoices`. Exact-tag, same-language
and authored-first fallback remain the existing pure output-localization policy.
`TestCW03OutputLocaleIndependentOfBlockLocale` covers English-only block metadata,
Spanish output metadata, exact/language fallback, absent/unsupported locales, and
no source/model calls. Runtime/browser confirmation remains outstanding.

### CW03-FR-03 — verifier fuzz harness and a non-vacuous authority oracle

The PR failure has the signature of the upstream duration-based fuzz coordinator
issue [Go #75804](https://go.dev/issue/75804). Inspection of the pinned Go 1.26.4
coordinator found its cancellation suppression compares the child context error;
the log alone does not prove that race caused this individual failure.

The prepared harness runs this target for 256 executions with two workers, race
instrumentation and a three-minute process timeout. There is no success-on-error
retry, skipped input, failure suppression, or production timeout change. Each
worker additionally verifies its own valid signed control before the mutated
input, so an always-deny verifier cannot satisfy the fuzz oracle. The command still
requires execution on the pinned toolchain before this finding can be closed.

### CW03-FR-04 — real retained-catalog pagination coverage

Keep the existing 84% PostgreSQL-store coverage gate. Add
`TestCW03RetainedCatalogPagination` against the real store and three published,
executed retained runs. It checks keyset continuation and termination, no repeated
or skipped identities, same-tenant/different-context exclusion, invalid limits and
cursors, and zero warehouse/model work. It exercises a previously uncovered
continuation branch, rather than lowering the threshold or replacing runtime tests
with serializer checks. The new measured percentage is unavailable until CI runs.

## Validation and limits of this review

The patch's base files are matched against exact Git blob hashes returned for the
main commit. Available local checks cover Go syntax parsing/formatting, YAML syntax,
unified patch application and whitespace. They are not Go type checking, real
PostgreSQL execution, browser acceptance, race tests, fuzzing or coverage results.

The original preparation pass could not publish because repository writes were
unavailable. The publication pass has authenticated GitHub write operations and
rechecked that remote main still equals the exact baseline above. Local Git still
fails DNS resolution for github.com; the installed Go is 1.23.2, and an actual
retry to obtain the pinned 1.26.4 toolchain failed DNS resolution too. Publishing
through the repository API does not imply runtime validation. No merge, tag or
deployment is part of this follow-up.

Remaining completion work: run the new regressions on the pinned real boundaries,
run the verifier fuzz target, measure unchanged coverage gates, run strict phase
27–31 and applicable clients/browser regressions, run full CI, fix further findings,
attach exact passing source and run evidence to the follow-up PR. The branch
and pull request are publication artifacts, not evidence that CI has passed.
No green or ready-to-merge claim is made.

The existing [v2 contract](../contracts/reporting-output-intent-v2.md) remains the
behavioral requirement. These corrections continue BLK-01/BLK-07 and preserve the
BLK-05 egress boundary; they do not claim broader feature parity or independent
security, performance, live-provider, cloud-warehouse or release qualification.
