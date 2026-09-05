# Phase 16 — rules-clarification

Status: planned. Owner: internal/semantics, internal/nlq. Hard dependencies: 05, 15.

## Authority and design

RFC-001 §8/9, D-049 and [COMMON.md](COMMON.md) apply. Separate hard execution constraints from advisory language. Rule evaluation is business reasoning under signed authority, not an access-policy issuer.

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

## Acceptance criteria

1. **AC01** — Rules CRUD/activation honors signed scope and exact resource reach; no feedback or draft can self-approve.
2. **AC02** — Conflicting or unmarked sensitive rules fail with bounded explanations; mandatory execution restrictions cannot be dropped for prompt budget.
3. **AC03** — Ambiguous questions produce typed slots and stop unsupported generation until required choices are supplied.
4. **AC04** — Rule injection counts real tokens, reports omitted advisory context and preserves hard constraints/explicit user choices.
5. **AC05** — Replay/shadow runs pin rule/topic versions and current data authority; comparison evidence cannot mutate production rules.
6. **AC06** — Rule activation/retirement invalidates affected query/block evidence and caches without silently rewriting published blocks.

## Tests, coverage and smoke

Implement `TestPhase16/AC01` through `TestPhase16/AC06` using rule/conflict/clarification goldens and real versioned state. Query/replay integration uses the shared reader once available; no separate executor. COMMON.md sets coverage; `scripts/smoke/phase-16.sh` requires all six results for full rule parity.

## Glossary, decisions and deviations

Advisory guidance, hard constraint and shadow comparison are distinct. D-049 restores the source continuity obligation. No runtime completion is claimed.
