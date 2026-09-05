# Phase NN — <slug / short title>

> **Status:** draft | in-review | in-progress | shipped
> **Owner:** <name / handle>
> **Depends on:** <phase-NN-slug, or "none">

Copy this file per CLAUDE.md §16 step 6:

```bash
cp docs/plans/_template.md docs/plans/phase-NN-slug.md
cp scripts/smoke/_template.sh scripts/smoke/phase-NN.sh
```

Fill every section below — do not delete a section for being inapplicable;
write "n/a" and say why. `make drift-audit` mechanically checks that this
file contains the "Brief findings incorporated" and "Acceptance criteria"
headings verbatim, and that `scripts/smoke/phase-NN.sh` exists.

---

## RFC / request sections

<!-- Cite the RFC-001-Chartworks.md section(s) this phase implements. Until
     the RFC lands, cite the consumer request doc (if one lands at kickoff) §X and/or
     docs/decisions.md D-NNN entries instead. -->

## Depends on

<!-- Other phases this one requires to already be shipped, and why. "none"
     if this is a foundational phase. -->

## Informing briefs

<!-- Per docs/research/INDEX.md, list the research brief(s) that inform this
     phase. A phase plan that cites no informing brief is a drift signal —
     explain why if genuinely none exists yet. -->

## Brief findings incorporated

<!-- What did the cited brief(s) recommend that this plan adopts? Be
     specific — this section (and the next) are forcing functions that make
     inheritance from research visible, not a formality. -->

## Findings I'm departing from

<!-- What did a brief (or the request doc) recommend that this plan does
     NOT do, and why? "none" is a valid answer but should be a deliberate
     one, not an oversight. -->

## Scope

<!-- What this phase delivers. Be concrete: packages touched, endpoints/
     tools added, schema changes. -->

## Non-goals

<!-- What this phase explicitly does NOT deliver, to prevent scope creep
     and to give the next phase a clean starting line. -->

## Design

<!-- The approach: data flow, key types/interfaces, how it fits the seams
     in CLAUDE.md §4.4 (gateway/store/vindex/auth/extraction/telemetry),
     and how it upholds P1–P7 (CLAUDE.md §1) where relevant. -->

## Config keys added

<!-- Every new config key: name, type, default, and where it's documented
     (CLAUDE.md §4.2: "a new config key ⇒ documented in the plan, the
     example config, and a smoke check"). "none" if this phase adds none. -->

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| | | | | |

## Acceptance criteria

<!-- Numbered, mechanically checkable. Each one should map to a smoke check
     below. CLAUDE.md §4.2: a phase is done only when every criterion here
     passes. -->

1.
2.
3.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** <!-- table-driven where it fits -->
- **Integration:** <!-- required whenever Deps name another subsystem's
  shipped phase, or this phase closes a seam another phase opened, or
  introduces a public interface other phases build on (§17). Real drivers
  (Docker Postgres, a real token) — the gateway `mock` driver is the one
  sanctioned boundary mock, paired with a recorded-fixture test. -->
- **Adversarial:** <!-- required if this phase touches an ACL/auth path:
  cross-tenant probe, empty access set, forged-header attempt,
  fetch-then-filter regression guard. "n/a" otherwise. -->
- **Fuzz:** <!-- required for any parse/decode surface (JWT, ingest
  payloads): FuzzXxx + seed corpus + asserted invariant. "n/a" otherwise. -->
- **Bench:** <!-- required for any hot reusable artifact: BenchmarkXxx.
  "n/a" otherwise. -->

## Coverage targets

<!-- Per CLAUDE.md §11 defaults (override here with a reason if different):
     80% new packages; 85% store/vindex drivers, auth/acl, conformance-tested
     subsystems; 70% CLI/tooling. List the exact scripts/coverage-bands.conf
     entries this phase adds. -->

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| | | |

## Smoke checks

<!-- Map each acceptance criterion above to an assertion in
     scripts/smoke/phase-NN.sh. -->

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | |
| 2 | |
| 3 | |

## Glossary additions

<!-- New terms this phase introduces, pre-written for docs/glossary.md
     (landed in the same PR per CLAUDE.md §14). "none" if this phase
     introduces no new vocabulary. -->

## Decisions filed

<!-- New docs/decisions.md D-NNN entries this phase files, or references to
     existing ones it relies on. -->

## Deviation log

<!-- Filled DURING implementation, not at authoring time. Every reasonable
     deviation from this plan discovered while building it (CLAUDE.md §4.3):
     what changed, why, and confirmation this file was updated in the same
     PR. -->
