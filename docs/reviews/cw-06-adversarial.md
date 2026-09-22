# CW-06 adversarial review

Date: 2026-09-22. Scope: RUL-01 and BLK-02 on
`codex/cw06-rules-snapshots`. This record distinguishes executed local checks
from manual final-gap qualification.

## Reviewed behavior

- Compound scopes require every exact target; legacy entity scopes remain
  any-target; template scopes use validated exact identifiers.
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
   replacement window.** Capture now additionally requires that exact publication
   to remain active, so replacement/retirement racing the handoff fails stale.

## Executed evidence

- `CGO_ENABLED=0 go test ./internal/semantics ./internal/semantics/rulesets ./internal/nlqroute ./internal/nlqexec ./internal/reporting ./internal/store/postgres`
- `CGO_ENABLED=0 go test ./test/acceptance -run '^$'` (acceptance package
  compilation only)
- `CGO_ENABLED=0 go test ./... -run '^$'` (all package compilation)
- `git diff --check`, AGENTS/CLAUDE mirror check and confidential-name scan
- `make planning-check` with a canonical macOS private temporary path

The focused domain tests and all-package compilation passed. The first
`make planning-check` invocation exposed a pre-existing macOS
`/var` versus `/private/var` temporary-path assertion in the coverage-gate
unit; rerunning with the canonical private temporary path lets that test exercise
its intended behavior.

## Remaining qualification

The real PostgreSQL acceptance case
`TestCW06RulesReporting` creates, validates, publishes and certifies a
rule-governed block, then replaces only the ruleset and checks stale health,
refused revalidation and immutable historical publication. It is registered and
compile-checked, but was not executed locally because the available Docker
PostgreSQL fixture was unhealthy and Docker Desktop returned storage I/O errors
when starting a replacement. No destructive Docker repair was attempted.

Run the D-074 manual final-gap workflow on the exact candidate head for the real
PostgreSQL case, publication/CAS races, race detector, full coverage/fuzz suites,
all named phase acceptance, source/native matrices and release gate. The
workstream does not claim live provider, warehouse, browser, stress or release
evidence.
