# Phase NN — slug

Status: planned. Owner: internal/package. Hard dependencies: none.

## Authority and design

Cite current RFC sections, owner decisions and [COMMON.md](COMMON.md). Pengui alone owns authority; production models use the Bifrost SDK and remote providers. No new local IAM, learned model runtime or established-host qualification.

## Brief findings incorporated

List the actual informing briefs and behavior retained, plus coverage.json IDs. Evidence depth is explicit; a file inventory is not a tested feature.

## Findings I'm departing from

Name deliberately superseded source behavior and its approved equivalent; do not remove required features silently.

## Scope and implementation tasks

Name the actual service/driver, first consumer, public operations, migration and limits. Pair a seam with its first real implementation. Follow the phase dependency graph, not numeric order.

## Non-goals

State concrete boundaries and adjacent phase ownership. Do not claim a needed consumer is complete elsewhere unless its dependency and test exist.

## Config and persistence

Exact keys/defaults/bounds/secret classification, example, migration tables/constraints, retention and positive/negative tests. Reuse existing authority, queue and SDK gateway seams.

## Acceptance criteria

1. **AC01** — Replace with an observable success and failure requirement.
2. **AC02** — Replace with another real requirement.

## Tests, coverage and smoke

Implement TestPhaseNN/AC01 and AC02 (extend the list and matching registry count as needed). Real I/O boundaries, applicable race/fuzz/adversarial cases and COMMON.md coverage apply. The phase smoke invokes scripts/run_phase_acceptance.py; missing/skipped/empty tests cannot pass. Add actual feature/gate references to coverage.json.

## Glossary, decisions and deviations

Add vocabulary and append decisions where necessary. Record implementation deviations with equivalent behavior and actual evidence; statuses remain planned until implementation is submitted and tests run.
