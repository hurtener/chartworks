# Phase 09 — sql-validate-core

Status: shipped. Owner: internal/exec. Hard dependencies: 03, 04.

## Authority and design

RFC-001 §9, D-045/D-051 and [COMMON.md](COMMON.md) apply. Define the read adapter interface here, implement concrete drivers in phase 08, and inject them at the composition root. This phase closes validation mechanics; real execution closes in phase 10.

## Brief findings incorporated

Briefs 02, 03, 04, 07, 08: native dialects, layered validation, relation/function boundaries, CTE preservation, typed errors and independent read credentials.

## Findings I'm departing from

A SELECT prefix, EXPLAIN result or parse success is not a security proof. No ANSI-only shortcut, keyword-only ban or bypass for unproven dialects.

## Scope and implementation tasks

1. Define ReadAdapter/validated-plan contracts in exec, parser drivers and positive statement/relation/function checks; concrete sources depend on the interface, not vice versa.
2. Build a dialect corpus and typed validation errors; retain native dialect generation and proven parser choices, not an ANSI-only shortcut.
3. Resolve CTE/aliases/nesting and actual semantic/authority/context dependencies; delegate native dry-plan checks under constrained read credentials.

## Non-goals

No raw-string execution API, skip-validation flag, local authorization policy or EXPLAIN ANALYZE as a harmless validator.

## Config and persistence

Exec SQL byte/AST-depth/parameter limits; parser driver pins and declared dialect capabilities; no skip_validation option. The validated plan binds source, execution context, semantic version, resolved dependencies and parameter shape. Its zero value is invalid. Diagnostic SQL remains protected domain evidence, not logs.

## Acceptance criteria

1. **AC01** — Only the validator constructs a nonzero executable plan; its source/context/authority/parameter binding is checked again at execution.
2. **AC02** — Whole-tree writes/DDL/stacked statements/side effects/external access and relation/column escapes are rejected per dialect.
3. **AC03** — Valid CTEs/window/set operations and scoped aliases survive; unproven dependency/function visibility fails closed rather than being guessed.
4. **AC04** — Native planning uses no EXPLAIN ANALYZE or write-enabled path; parsing/planning success alone never grants read authority.
5. **AC05** — Unsupported dialect/parser capability returns a truthful typed result; a driver is executable only after its full safety contract passes.
6. **AC06** — Fuzz corpus, parser/version pins and adversarial typed-error goldens are reproducible; no regex-only security guarantee is claimed.

## Tests, coverage and smoke

Implement `TestPhase09/AC01` through `TestPhase09/AC06`. Use dialect-positive and adversarial fixtures, fuzz parse/resolve surfaces and unforgeability/zero-value tests. Real native-plan probes arrive with drivers and cannot be replaced by a fake parser pass. COMMON.md requires 85% exec/conformance coverage; `scripts/smoke/phase-09.sh` requires all six results.

## Glossary, decisions and deviations

Validated plan, dependency set and execution context are shared contract terms. D-051 corrects the old universal safety assumptions. Runtime implementation and named acceptance are supplied here; no production deployment is claimed.

## Implemented evidence and qualified scope

D-064 and [the vector/source/read contract](../contracts/vector-sources-validation.md) pin the qualified scope. See the [adversarial review](../reviews/phase-07-08-adversarial.md). Existing hard dependencies and all six numbered criteria remain unchanged. Real PostgreSQL/pgvector and native-parser fixtures provide runtime evidence; final exact-head CI must pass before merge. The full execution product remains phase 10, additional engines phase 14 and the semantic generation consumer phase 15.

## Phase-10 read consumer

The [D-065 read execution contract](../contracts/read-execution.md) extends the
existing validated plan and source adapter without changing their authority or
qualification boundaries. Its [adversarial review](../reviews/phase-09-10-adversarial.md)
records the actual cursor, attempt, cancellation and exact-result regressions.
Earlier statements assigning execution to phase 10 are now realized by that
consumer; other-engine and retained reporting deliverables remain separately owned.
