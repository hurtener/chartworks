# CW-06 adversarial review

Date: 2026-09-22. Scope: RUL-01 and BLK-02 on
`codex/cw06-rules-snapshots`. This record distinguishes executed local checks
from manual final-gap qualification.

## Reviewed behavior

- Compound scopes require every exact target; legacy entity scopes remain
  any-target; template scopes use canonical reviewed topic/ruleset pins.
- Compilation rejects mixed scope shapes and overlapping contradictory hard
  constraints while allowing contradictory rules in distinct exact templates.
- Evaluation/replay/shadow retain deterministic selection reasons and never
  execute SQL, regex, source or model work.
- Query capture transfers exact retained rule versions/digests from the completed
  query. It fails closed if the query's rule vector is malformed, the rule reader
  is absent, or the retained rule/topic pin does not match.
- Block validation, certification health, impact checks, frozen manifests, reuse,
  compositions and scheduled admission retain rule pins. Replacement/retirement
  invalidates health without mutating publication or historical evidence.
- Tenant/source/context authority continues through existing verified-envelope
  rule/topic/source readers. Pins are evidence and never grants.

## Concrete findings and fixes

1. **Malformed template selectors initially behaved as an ordinary mismatch.**
   The evaluator now validates every non-empty selector with the canonical
   identifier grammar and the truth-table test covers rejection.
2. **The first implementation covered manually authored blocks but omitted
   query-capture transfer.** Completed-query evidence now carries the aligned
   rule-version vector; the capture adapter resolves exact retained publications
   under current authority and creates definition-v2 pins. Missing support or a
   mismatched vector fails explicitly.
3. **The first shape required a rule pin for every topic when any pin existed.**
   That incorrectly excluded a valid multi-topic query where only some topics
   had active reviewed rules. Validation now permits at most one sorted pin per
   pinned topic and still rejects duplicates, unknown topics and digest drift.
4. **Documentation initially described per-PR broad qualification.** The phase
   plans now preserve D-074: focused implementation checks run locally and on
   pull requests; race/fuzz/full coverage/PostgreSQL matrix/release qualification
   remains in the manual final workflow.
5. **Composition admission initially retained rule pins without comparing them
   to the current block validation record.** The shared admission/execution guard
   now rejects manifest pin tampering before authority or payload work.
6. **Query capture could read a formerly exact rule version across a concurrent
   replacement window.** Capture now locks each current rule publication head in
   the block commit transaction. The publisher takes the conflicting head lock
   before pointer advancement; a replacement that wins the race returns typed
   stale and rolls back every block/revision/pin write.
7. **A raw optional template string let callers omit or substitute applicability.**
   Routing now accepts only a structured reviewed selection pinned to the current
   topic/ruleset publication, rebuilds canonical coordinates server-side and
   returns `reviewed_template_required` before any model call when a selection is
   required. Stale coordinates or an unreviewed identifier fail closed.
8. **The first rule-scope implementation stopped at routing.** Preflight, planning,
   persisted query evidence, refinement/reconstruction, saved selections,
   completed-query capture and reporting provenance now retain the same sealed
   selection. Refinement cannot substitute another otherwise-valid template.
9. **Replay/shadow retained only output selection reasons.** Comparison evidence
   now also stores and projects the canonical input selection. Migration 039 adds
   bounded immutable query/comparison fields; two comparisons with identical
   semantic references but different reviewed templates remain distinguishable.
10. **Shadow equality ignored selection-only changes.** The comparator now includes
    sorted rule-selection evidence, so changed advisory applicability or an
    equivalent hard-constraint outcome cannot be reported unchanged.
11. **Current main assigned D-075 while this branch was under review.** Main's rich
    semantic decision remains D-075; this workstream is now uniquely D-076 and
    the active plan/gap links point to the correct decision.
12. **Frozen sealing omitted block coordinates from its rule-head fence.** The
    seal now locks the exact manifest revision's rule heads in the shared
    topic/rule/source order. A real PostgreSQL regression delays an ordinary
    publisher after it owns the rule head and requires the waiting seal to fail
    stale without a frozen manifest.
13. **A block edit could retain template provenance while replacing rule pins.**
    New captures persist a bounded topic-ordered selection set with complete
    topic/ruleset coordinates. Authoring, validation, certification and frozen
    gates require exact rule-pin matches; clearing provenance is allowed, while
    restoring it requires recapture. Retained singular records require one
    exactly matching rule pin, and migration 040 prevents mixed storage.

## Executed evidence

- `CGO_ENABLED=0 go test ./internal/semantics ./internal/semantics/rulesets ./internal/nlqroute ./internal/nlqexec ./internal/reporting ./internal/store/postgres`
- `CGO_ENABLED=0 go test ./internal/topicapi ./internal/nlqapi ./internal/reportingapi ./sdk/chartworks`
- `CGO_ENABLED=0 go test ./test/acceptance -run '^$'` (acceptance package
  compilation only)
- `CGO_ENABLED=0 go test ./... -run '^$'` (all package compilation)
- `git diff --check`, AGENTS/CLAUDE mirror check and confidential-name scan
- `make planning-check` with a canonical macOS private temporary path

The fix round reran the focused ruleset/router/NLQ/reporting/store/API/SDK tests
and acceptance-package compilation successfully after merging current main.
Formatting, vet, all-package compilation, planning coherence, mirror and diff
checks also passed on the integrated candidate.

## Remaining qualification

The real PostgreSQL acceptance case
`TestCW06RulesReporting` creates, validates, publishes and certifies a
rule-governed block; proves canonical template propagation through preflight,
plan, refinement, saved preparation, execution and reporting capture; and pauses
a real completed-query capture while the ordinary publisher replaces its rules.
It requires stale failure with zero block/revision/pin side effects. The case is
registered and compile-checked, but local execution reported
`CHARTWORKS_TEST_STORE_URL required`; no database result is claimed.

Run the D-074 manual final-gap workflow on the exact candidate head for the real
PostgreSQL case, publication/CAS races, race detector, full coverage/fuzz suites,
all named phase acceptance, source/native matrices and release gate. The
workstream does not claim live provider, warehouse, browser, stress or release
evidence.
