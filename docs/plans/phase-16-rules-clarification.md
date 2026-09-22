# Phase 16 — rules-clarification

Status: shipped. Owner: internal/semantics, internal/nlq. Hard dependencies: 05, 15. Current cumulative evidence: [phases 15–18 and 21](../reviews/phase-15-18-current-evidence.md).

Proposed PR #11 delivery; this status becomes effective after every required hosted
check passes and the PR merges.

## Authority and design

RFC-001 §8/9, D-049, the typed [phase 16 integration contract](../contracts/semantic-foundation-v1.md#phase-16-integration-contract), and [COMMON.md](COMMON.md) apply. Separate hard execution constraints from advisory language. Rule evaluation is business reasoning under signed authority, not an access-policy issuer.

## Brief findings incorporated

Briefs 03, 05, 14: rule lifecycle, provenance, priority/conflicts, bounded context, clarification slots and rule replay/shadow analysis.

## Findings I'm departing from

Historical replay/shadow functionality is not automatically deferred. Mandatory constraints cannot disappear when prompt budget is exhausted; feedback cannot publish itself.

## Scope and implementation tasks

1. Implement typed business rules, scope/priority/conflict/provenance, proposed/active/retired lifecycle and explicit review.
2. Generate/edit ambiguity patterns and sensitive-literal labels; return named clarification slots before generation.
3. Preserve historical replay and shadow comparisons through the same query/evaluation core; separate executable constraints from optional prompt guidance.

## Non-goals

No autonomous rule approval, second query engine or unlimited replay under a broad service credential.

## Config and persistence

Rules advisory token budget, sensitivity policy and bounded replay case/call/time limits; no silent constraint truncation. Version rules and comparison inputs/results; production state remains unchanged by a shadow run. Register rule lifecycle, pattern editing and comparison operations with the shared HTTP/SDK surfaces.

## Bounded runtime stage

The [rule lifecycle v1 contract](../contracts/rule-lifecycle-v1.md) implements the
reviewed immutable lifecycle, deterministic evaluation of closed hard constraints,
detached pattern reads, exact replay/shadow comparison evidence, and ordered
publish/retire invalidation fences. It reuses the published topic dependency fence,
shared HTTP registry and SDK. The phase 17 routing consumer supplies the first real
required-slot and real-token advisory consumer over these seams. Phase 18 still owns
shared-reader query/evidence consumption of invalidation fences and downstream
generation/execution integration; those dependent gates are not circular prerequisites
for phase 16's own routing-rule acceptance.

## Acceptance criteria

1. **AC01** — Rules CRUD/activation honors signed scope and exact resource reach; no feedback or draft can self-approve.
2. **AC02** — Conflicting or unmarked sensitive rules fail with bounded explanations; mandatory execution restrictions cannot be dropped for prompt budget.
3. **AC03** — Ambiguous questions produce typed slots and stop unsupported generation until required choices are supplied.
4. **AC04** — Rule injection counts real tokens, reports omitted advisory context and preserves hard constraints/explicit user choices.
5. **AC05** — Replay/shadow runs pin rule/topic versions and current data authority; comparison evidence cannot mutate production rules.
6. **AC06** — Rule activation/retirement invalidates affected query/block evidence and caches without silently rewriting published blocks.

## Tests, coverage and smoke

`TestPhase16/AC01` through `TestPhase16/AC06` exercises signed lifecycle, conflict and
mandatory preservation, the real phase 17 clarification/token consumer, exact replay and
shadow pins, and immutable invalidation cursors against real PostgreSQL state. Query and
evidence consumption of those fences remains on the phase 18 shared reader; no separate
executor is introduced. COMMON.md sets coverage; `scripts/smoke/phase-16.sh` requires
all six results for full rule parity.

## Glossary, decisions and deviations

Advisory guidance, hard constraint and shadow comparison are distinct. D-049 restores the source continuity obligation. No runtime completion is claimed.


## CW-01 delivered clarification extension

The [conditional clarification contract](../contracts/conditional-clarification-v1.md)
extends this phase's existing core, without changing authority ownership, immutable
publication, source partitions or validated-read prerequisites. It supplies reviewed
conditional applicability and typed answer resolution, mandatory-group token/privacy
handling, source-bound parameter effects, session correction/removal and retained
consumer replay. Preview, replay/shadow and safe import dispositions use the existing
API/SDK surfaces rather than a new authoring application.

`TestCW01/AC01` through `TestCW01/AC10` add the clarification acceptance corpus;
the six existing `TestPhase16`, `TestPhase17` and `TestPhase18` criteria remain
unchanged and continue to run. [CW-01 delivery evidence](../reviews/cw-01-delivery-review.md)
records exact executed checks and review corrections. No CLAR-AC11 comprehension
study, live cloud/model quality measurement or general SQL-shape expansion is
claimed by these software tests.

## CW-06 rule-scope continuation

RUL-01 is implemented by the closed compound-AND and exact-template scope
contract in [rule lifecycle v1](../contracts/rule-lifecycle-v1.md). Template
selection is a server-verified publication pin propagated through the real NLQ
and saved/replay paths; omission or substitution fails before generation.
Selection is deterministic and replayable; distinct template scopes do not create false
compile-time conflicts, while potentially overlapping hard constraints still
fail closed. No executable SQL, regular-expression or authority mechanism was
added. Focused truth-table and conflict coverage supplements AC02/AC05/AC06;
full race, fuzz, PostgreSQL and release qualification remains in the manual final
gap workflow described by D-074.

## EXP-05 interaction and semantic-edit qualification

The deterministic runtime portion of EXP-05 is covered by public lifecycle and
real-PostgreSQL regressions. Simultaneous rule matches retain sorted selection,
required, excluded and violation evidence. Dependency conflicts (including an
exclusion beneath a pinned metric) reject before draft persistence. Template
scopes expose exact match/mismatch reasons and require canonical selection pins.
Compiler-level relationship removal and changed-digest regressions cannot
silently rebind an old rule definition; a new reviewed topic/digest pin is
required. Public lifecycle coverage separately proves exact topic/ruleset
replacement and rollback; relationship-specific publication remains final
qualification evidence. Historical
replay remains immutable across replacement, retirement and rollback while
rechecking the caller's current source/context reach. Reporting capture and
migration-040 regressions cover template provenance removal/recapture and
populated legacy revision shapes. The executed boundary and remaining human/live
calibration are recorded in [the EXP-05 review](../reviews/exp05-rule-interactions.md).
