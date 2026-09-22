# Evaluation and calibration v1

Status: implemented for review, 2026-09-22. Owns Phase 24 and the Phase 24 portions of EVAL-01 and EXP-01/03/05/09/10/11.

An evaluation suite is an immutable, versioned manifest with a nonzero deterministic seed, explicit fixture or live mode, protected input references, documented equivalent semantic outcomes, reviewed thresholds, hard case/call/token/retry/time ceilings and exact implementation/configuration/semantic/rule/template/source/dialect provenance. Cases compare semantic evidence digests and typed outcomes. They never treat SQL text similarity as result correctness. English and Spanish are explicit case attributes.

Security-critical cases cover signed identity and data scope, injection, dialect escape, resource exhaustion, BYO safety and frozen-report invariants. Their tolerated failure count is always zero. A quality threshold is valid only when marked reviewed. Owner calibration may instead be `unknown`; such a manifest can record observations but cannot produce a passing gate. The evaluator contains a deliberately failing regression path so a green gate cannot be a no-op.

Fixture observations are embedded only in fixture suites. Live suites reject fixture observations and require an injected authority-bound runner. Reports preserve the mode and separately record service, source and model duration, calls, tokens, retries and cost. Missing cost or usage stays absent rather than becoming zero. Dialect entries independently say measured, unsupported or unknown; a fixture result cannot claim live engine evidence or public benchmark equivalence.

The runtime service reuses the registered operator `ops.write`/`ops.read` actions plus signed tenant write/read/export reach before storing suites/runs or exporting reviewed feedback; it invents no issuer scope. PostgreSQL stores tenant-and-actor partitioned immutable suite revisions and run evidence. Ordinary reports and logs contain hashes, bounded outcomes and measurements, never bearers, prompts, SQL or result rows. Feedback export creates protected, held-out `candidate` cases only.

Replay and shadow use the same evaluator. Optimization compares held-out evidence under the suite budgets and can only create a candidate proposal. Promotion needs an immutable human approve/reject receipt; no score changes a prompt, example, rule or semantic publication. Rollback is selection of an earlier reviewed external pack revision, whose digest is part of the next suite manifest.

The operator CLI runs or inspects explicit local fixture manifests. Live execution remains in the authority-bound runtime, where the existing single Bifrost gateway and source adapters are injected. Phase 34 consumes the suite/run hashes in its cutover ledger. Phase 25 still owns final live calibrated workloads and full release qualification.
