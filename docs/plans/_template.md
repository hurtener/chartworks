# Phase NN — slug

Status: planned. Owner: owning packages. Hard dependencies: registered phase IDs or none.

## Authority and design

Cite current RFC sections and the relevant appended decisions. [COMMON.md](COMMON.md) is part of this plan. Specify interfaces/data flow and how the phase ships its first concrete consumer without a dependency cycle.

## Brief findings incorporated

Name informing briefs and feature IDs. Distinguish inspected source, documentation, tests and inventory from runtime evidence.

## Findings I'm departing from

State deliberate departures and equivalent behavior. Pengui alone issues authority; host Apps compatibility is established. No silent source-feature deferral.

## Scope and implementation tasks

Describe ordered, concrete package/schema/API/SDK/config changes and consuming operations. Avoid a list of speculative interfaces without implementations.

## Non-goals

Name boundaries that prevent adjacent-platform or authentication overbuild.

## Config and persistence

List exact new settings or the explicitly assigned schema implementation task, defaults/bounds/secret classifications and table/migration ownership. No IAM state.

## Acceptance criteria

1. **AC01** — A concrete observable behavior, inputs, failure/positive assertions and real boundary requirements.

## Tests, coverage and smoke

Implement TestPhaseNN with every ACxx subtest. Pair scripts/smoke/phase-NN.sh, update phase-registry.json and coverage.json, and follow COMMON.md. Missing/skipped runtime tests are never passing implementation.

## Glossary, decisions and deviations

Name vocabulary/decision changes and record implementation deviations with preserved source behavior/evidence. No completion is claimed at plan-authoring time.
