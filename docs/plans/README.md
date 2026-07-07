# Chartworks — Master Phase Plan

> **Status:** accepted · 2026-07-06 · owner: orchestrator
> **Authority:** below `RFC-001-Chartworks.md`, above everything else (CLAUDE.md §2).
> Each phase gets its own `phase-NN-slug.md` (from `_template.md`) authored via the
> §16 workflow before implementation starts; this file is the index, the wave
> structure, and the cross-cutting conventions.

---

## Cross-cutting conventions (binding on every phase)

1. **Doneness** = CLAUDE.md §4.2: all acceptance criteria pass · coverage bands met
   (`make coverage`) · `scripts/smoke/phase-NN.sh` reports `OK ≥ count(criteria),
   FAIL = 0` · all prior smoke scripts still pass · preflight green. Preflight modes:
   `make preflight` (fast — changed phases only, pre-commit hook) and
   `make preflight-full` (every phase's smoke — CI and the §14 wave-end gate).
2. **Branching & ceremony.** Each phase implements on `feat/phase-NN-slug`. Mid-wave
   phases merge to the wave integration branch `wave-N` after orchestrator review
   (adversarial: reject on any criterion miss, scope creep, or drift). **Full CI/PR
   ceremony at wave ends only**: `wave-N` → `main` as one reviewed PR with the §14
   checklist, green CI, and the checkpoint audit's punch list resolved.
3. **Checkpoint audit** at every wave boundary (CLAUDE.md §17): read-only review of
   all shipped phases for wiring gaps, RFC drift, weak tests, hygiene regressions —
   punch list lands as `chore(checkpoint)` inside the wave PR.
4. **Coverage bands** (recorded in `scripts/coverage-bands.conf` in the same PR that
   creates a package): 85% `store` / `vindex` / `auth` / `identity` / `access` /
   `exec` and conformance-tested subsystems · 80% other new `internal/` packages ·
   70% `cmd/` / CLI / eval tooling.
5. **Standing test obligations** ride the phase that touches the area: the access
   adversarial set (cross-tenant probe from a mechanically derived route/tool
   registry — never a hand-maintained list — empty access set, forged header,
   fetch-then-filter regression guard) · write/DDL-injection + schema-escape probes
   once exec ships · fuzz on JWT parsing, NLQ payloads, and SQL-validation inputs ·
   `-race` concurrent-reuse tests on every shared artifact · golden tests on
   contracts (context bundles, chart specs, validation error codes) · integration
   tests with real drivers when a phase closes a seam another opened (Docker
   Postgres via `make pg-up`; the gateway `mock` driver is the one sanctioned
   boundary mock, paired with recorded-fixture tests).
6. **Vocabulary discipline** (P6) checked by `make drift-audit` on every commit; new
   terms land in `docs/glossary.md` in the same PR. The §2 forbidden list is binding.
7. **Model/agent policy** (build-time): exploration/low-difficulty → Sonnet; medium+
   or design-sensitive → Opus; plan authoring and checkpoint audits → Opus; the
   orchestrator reviews adversarially and independently re-runs every gate.
8. **Live verification** (D-010/D-031; manual, wave-end, never CI): real provider
   models + the sample warehouse via `.env`, `-count=1` always. **Soundings retro
   favors, applied from day one:** fresh-database proof for every migration-touching
   harness (no pre-migrated dev DB masking fail-loud guards) · the live gate stood up
   as soon as `.env` exists, not at the last wave · golangci-lint version pinned in
   CI from the first commit · external dependencies' packaging claims verified
   against real release assets before a plan bakes them in (pin exact tags).
9. **Hard gate (P1b):** no phase executes generated or submitted SQL against any
   real data source before phase 09 + 10 (the validation core and the read-only
   execution layer) are shipped and their adversarial obligations pass. Phases 12–13
   write only through the declared materialization path (P1c).
10. **Dockyard re-evaluation checkpoint** (D-019): at the Wave-5 → Wave-6 boundary,
    before phase 22 starts, re-check Dockyard's runtime for a global tool-middleware
    seam / production adopters; the result is a decision entry either way.

---

## Wave & phase index

| Wave | Phase | Slug | Owns | Depends on |
|---|---|---|---|---|
| **1 Foundations** | 01 | `binary-config-telemetry` | `cmd/chartworks`, `internal/config`, `internal/telemetry` | — |
| | 02 | `store-migrations` | `internal/store` + the §12 schema | 01 |
| | 03 | `auth-identity` | `internal/auth`, `internal/identity` | 01 |
| **2 Seams** | 04 | `access-grants` | `internal/access` (grants, resolver, scope-debug, decision telemetry) | 02, 03 |
| | 05 | `gateway` | `internal/gateway` (bifrost + mock, roles, metering) | 01 |
| | 06 | `jobs-scheduler` | `internal/jobs` (generic queue, handlers, dispatcher) | 02 |
| | 07 | `vindex` | `internal/vindex` (pgvector facets) | 02 |
| **3 Sources & SQL core** | 08 | `sources-core` | `internal/sources` (seam, registry, custody, postgres driver, null/mock, discovery) | 02, 04 |
| | 09 | `sql-validate-core` | `internal/exec` validation half (AST stages, `ValidatedSQL`, error vocabulary, dialects) | 01 |
| | 10 | `exec-read` | `internal/exec` execution half (read-only enforcement, caps, timeouts, result shaping) | 08, 09 |
| | 11 | `uploads-workspace` | upload parsing + workspace provisioning + dataset registration | 08 |
| **4 Engineering** | 12 | `engineering-profiling` | profiles, quality, freshness, type classification | 05, 06, 08 |
| | 13 | `engineering-pipelines` | pipeline defs, write-shape validation, materializer, runs, lineage, schedules | 06, 09, 10, 12 |
| | 14 | `warehouse-drivers` | `mysql`, `sqlserver`, `bigquery`, `snowflake`, `databricks` drivers + conformance (D-032) | 08, 10 |
| **5 Semantics & NLQ** | 15 | `topics-lifecycle` | `internal/semantics` (packs, lifecycle, facets, capability contracts, export/import) | 04, 05, 07, 12 |
| | 16 | `rules-clarification` | governed rules, clarification patterns, canonical registry | 15 |
| | 17 | `nlq-routing-context` | span hints, retrieval, routing, context assembly, budgets | 05, 07, 15, 16 |
| | 18 | `nlq-generation-execution` | internal generation, precedence, bounded repair, plan/run/preflight/refine, feedback + learning | 09, 10, 17 |
| | 19 | `byo-mode` | context bundle contract, `get_query_context`, `submit_sql`, parity proofs | 18 |
| | 20 | `charts-spec` | `internal/charts` (rules selector, declarative spec) | 10, 15 |
| **6 Surfaces** | 21 | `http-api` | `internal/api` (full §11.2) | 04, 08, 11, 13, 15–20 |
| | 22 | `mcp-server` | `internal/mcpserver` (10 tools, middleware gate, annotations test) | same as 21 |
| | 23 | `sdk-cli-parity` | `sdk/chartworks`, admin CLI, three-surface parity tests | 21, 22 |
| **7 Quality & release** | 24 | `eval` | `eval/`, `chartworks eval`, golden + red-team CI gates | 18, 19, 20 |
| | 25 | `e2e-release` | E2E both auth modes, Dockerfile, product README, CHANGELOG, v0.1.0 | all |

Within a wave, phases without mutual deps run in parallel (01 → 02∥03; 04∥05∥06∥07;
08∥09 → 10∥11; 12 → 13∥14; 15 → 16 → 17 → 18 → 19, with 20 parallel from 15;
21∥22 → 23). Phase 14 may slip into Wave 5 without blocking anything (it is
additive behind the adapter seam). Phase 20 may land in Wave 6 if Wave 5 runs long.

---

## Phase detail blocks

### Phase 01 — `binary-config-telemetry` (Wave 1)
**RFC:** §3.2–3.3, §14, §15. **Briefs:** 01. **Difficulty:** low.
The `chartworks` binary skeleton (`serve`/`mcp`/`admin`/`eval`/`version` stubs,
stdlib-flag dispatch, D-008), typed config (YAML + `env:` indirection, unknown keys
rejected, per-subsystem fail-loud validators, the §14 example config), telemetry
foundations (slog JSON/text, Prometheus registry + `/metrics`, request-id middleware
primitives, audit-emitter interface, the **telemetry conformance test** asserting
every registered metric exports — brief 01's dead-counter scar).
**Key criteria:** missing secret ⇒ refused boot with a typed error; unknown config
key rejected; a registered-but-unexported metric fails the conformance test; smoke
drives `--version`, a bad config, a good config.

### Phase 02 — `store-migrations` (Wave 1)
**RFC:** §12. **Briefs:** 02, 05. **Difficulty:** medium.
The `Store` seam (narrow per-domain interfaces, no god-interface), pgx/v5 driver,
forward-only migration runner, migrations for the **complete budgeted 25-table
inventory**, conformance suite against Docker Postgres, **fresh-DB harness from day
one** (convention 8). `tenant_id NOT NULL CHECK (<> '')` everywhere (the fork's
nullable regression is the named counterexample); every read/write method takes
non-optional scope parameters.
**Key criteria:** conformance green under `-race` on a fresh DB; a scope-less query
method cannot be expressed; migration runner idempotent; cross-tenant probe returns
nothing at the store layer.

### Phase 03 — `auth-identity` (Wave 1)
**RFC:** §4. **Briefs:** 04, 01. **Difficulty:** medium.
JWT validation (asymmetric-only, parser-level `HS*`/`none` rejection,
`iss`/`aud`/`exp`/skew, per-surface audiences, JWKS fetch + cache + fail-closed
staleness), self-issue (keypair, API-key exchange, constant-time compare), the
frozen `identity.Envelope`, `chartworks admin bootstrap` (local CLI first-admin,
D-030 — **no** header-exchange endpoint exists to test because it doesn't exist).
**Key criteria:** `HS256`/`none` rejected before claims parse; stale JWKS ⇒
not-ready + typed 401; HTTP-audience token rejected on MCP surface and vice versa;
forged-header attempt has no effect; `FuzzParseToken` with seed corpus; envelope
immutability API-enforced; missing signing key ⇒ refused boot (no silent generation
— brief 04's scar).

### Phase 04 — `access-grants` (Wave 2)
**RFC:** §5. **Briefs:** 04, 05, 01. **Difficulty:** medium-high.
The `grants` model (three grains, `read|query|manage`), tenant roles with the
explicit admin sentinel, the **one resolver** → `EffectiveAccess` on the envelope,
per-decision counters + content-free events, scope-debug (admin API + CLI), and the
**mechanically derived adversarial registry** (routes/tools enumerated from
registration tables, cross-tenant probed in CI).
**Key criteria:** no grant ⇒ typed `access.none`, no query issued (proven by store
call-count assertion); agent principals resolve their own grants, never their
owner's; empty-set short-circuit metric increments; scope-debug names the failed
predicate; adversarial suite green.

### Phase 05 — `gateway` (Wave 2)
**RFC:** §13. **Briefs:** 03, 01. **Difficulty:** medium.
The intelligence seam: role-based model config (`embedding`/`enhance`/`sqlgen`/
`sqlfix`/`clarify`/`pipeline_draft`/`profile_summary`), `bifrost` + `mock` drivers,
schema-constrained structured outputs (free-text JSON parse forbidden + lint),
per-call metering → `gateway_call_events` + metrics, embedding model/dims pinning
validated at boot, recorded-fixture tests per role.
**Key criteria:** a provider SDK import outside `internal/gateway` fails an
architecture test; schema violation ⇒ typed error, never partial parse; every call
metered; dims mismatch at boot ⇒ refused start.

### Phase 06 — `jobs-scheduler` (Wave 2)
**RFC:** §3.3, §7.7. **Briefs:** 01, 02. **Difficulty:** medium.
The single generic leased queue (`FOR UPDATE SKIP LOCKED`, lease + heartbeat +
reclaim, priorities, typed handlers registered by kind — D-025), the schedule
dispatcher (cron/interval, skip-if-running overlap policy) enqueuing into the same
queue, graceful-shutdown lease release.
**Key criteria:** a crashed worker's job reclaims after lease expiry (proven with a
killed goroutine); overlap policy skips + logs + metric; handlers run under `-race`
concurrently; schedule dispatch is idempotent per due-window.

### Phase 07 — `vindex` (Wave 2)
**RFC:** §3.2, D-029. **Briefs:** 03. **Difficulty:** low.
The vector seam + `pgvector` driver: facet upsert/delete/search scoped
`(tenant, topic, version)`, HNSW cosine, batch operations, tenant-scoped deletion
(the fork's tenant-embedding cleanup, carried).
**Key criteria:** cross-tenant search returns nothing (adversarial); delete-by-topic
/-version/-tenant cascades verified; conformance against Docker Postgres w/ pgvector.

### Phase 08 — `sources-core` (Wave 3)
**RFC:** §6. **Briefs:** 02, 05, 04. **Difficulty:** high.
The data-source adapter seam (capabilities gating, `ValidatedSQL`-only query
signature), the connections registry (status lifecycle, secret-free read shape),
credential custody (AES-256-GCM envelope encryption, key ring rotation, fail-closed
boot, content-free credential events), the `postgres` driver + `null` + `mock`,
schema discovery with dialect-agnostic `TypeCategory` classification (computed once
— brief 02's `money`-column bug).
**Key criteria:** registry read/list shapes have no secret field (type-level test);
decrypt tries all ring keys (rotation test); missing key ring with stored secrets ⇒
refused boot; `null` adapter fails loud on any operation; a raw-string execute
cannot be expressed against the seam; discovery conformance on Docker Postgres.

### Phase 09 — `sql-validate-core` (Wave 3)
**RFC:** §9.5 (stages 1–2 + the semantics-independent half of 3). **Briefs:** 02,
04, 03, 07, 08. **Difficulty:** high.
Parser selection (evaluated against real dialect fixtures — convention 8's
pin-and-verify), the three-stage skeleton: byte/encoding pre-parse (**never
duplicating parser judgment** — the CTE-regression rule + golden CTE fixture),
dialect-aware AST parse, whole-tree statement blocking (SELECT-family only, blocked
node classes incl. dialect escapes), single-statement enforcement, the typed error
vocabulary, and the unforgeable `ValidatedSQL` type. Topic-pack/grant allowlisting
lands in phase 18 (needs semantics); the write-shape variant (declared inputs/one
output) lands here for phase 13's use.
**Key criteria:** DDL/DML anywhere in the tree rejected across all V1 dialects
(table-driven corpus); CTE queries pass (golden fixture); multi-statement rejected;
`FuzzValidate` with seed corpus asserting "never panics, never passes a write";
`ValidatedSQL` unconstructible outside the package (compile-time proof).

### Phase 10 — `exec-read` (Wave 3)
**RFC:** §9.6. **Briefs:** 02, 04. **Difficulty:** high.
The read execution layer over the adapter seam: read-only transaction/session
enforcement per driver (documented per-engine), server-side statement timeouts +
context deadlines, cursor-level row caps (never LIMIT-wrapping — the `ORDER BY`
lesson as a standing exec rule + regression test), `QueryResult` → `ResultPreview`
shaping, idempotency-key support, execution metrics.
**Key criteria:** a write statement smuggled past a hypothetically-broken validator
still fails at the adapter (read-only session probe — the defense-in-depth proof);
a query exceeding the timeout stops server-side (verified via `pg_stat_activity`);
row cap clamps to ceiling regardless of caller input; `ORDER BY` preserved under
capping (regression).

### Phase 11 — `uploads-workspace` (Wave 3)
**RFC:** §7.4. **Briefs:** 02, 01. **Difficulty:** medium.
Upload parsing (CSV/XLSX/Parquet: header detection, type inference, cell
sanitization, size/row limits), per-tenant workspace provisioning (separate
database, reached via the standard `postgres` adapter — D-024), dataset
registration with origin `upload`, upload-file custody on disk under tenant-scoped
paths, erasure hooks.
**Key criteria:** an uploaded CSV becomes a queryable dataset through the same
adapter + grants path as a warehouse table (integration); a malformed/oversized
upload ⇒ typed error; workspace DB is unreachable through the `store` seam
(architecture test); tenant erase removes workspace tables + files.

### Phase 12 — `engineering-profiling` (Wave 4)
**RFC:** §7.1–7.2, §7.5. **Briefs:** 09, 12, 02. **Difficulty:** medium.
Selective dataset registration, the profile job (per-column stats, sampled +
sanitized values, six quality dimensions, value families, freshness classification,
large-table hints), profile versioning, schema-drift re-discovery diff marking
dependent datasets/topics, `profile_summary` gateway role (optional descriptions).
**Key criteria:** profile output matches the normalized golden shape; sampling
respects ceilings (no full-table scans — asserted via mock adapter call log);
freshness buckets correct across fixtures; drift diff flags dependents.

### Phase 13 — `engineering-pipelines` (Wave 4)
**RFC:** §7.3, §7.6–7.7. **Briefs:** 11, 10, 02. **Difficulty:** high.
Pipeline definitions (versioned, declared inputs/output/destination per step),
write-shape validation via phase 09, quality checks (fail-loud), the
`Materializer` interface (distinct from `Query` — P1c) with `create-or-replace` +
`full refresh` strategies on declared destinations only, runs + lineage + freshness
stamping, schedule attachment, LLM-assisted drafting (`pipeline_draft` role,
draft-only — publication is an explicit human gate), the canonical identity
registry.
**Key criteria:** a step writing outside its declared output is rejected at
validation; an undeclared destination cannot be materialized to (typed error); a
failed quality check fails the run loudly (status + metric + audit); NLQ-path code
cannot reach `Materializer` (architecture test — the P1c proof); lineage recorded
per materialization.

### Phase 14 — `warehouse-drivers` (Wave 4)
**RFC:** §6.1 (D-032). **Briefs:** 02, 05. **Difficulty:** medium-high.
`mysql`, `sqlserver`, `bigquery`, `snowflake`, `databricks` drivers on their
official pure-Go drivers/connectors (exact versions pinned + verified against
release assets — convention 8), each passing the adapter conformance suite
(discovery, typing, read-only posture, caps, timeouts) + recorded-fixture dialect
tests for the validator. **The self-hostable engines (mysql, sqlserver — joining
postgres from phase 08) run the conformance suite against dockerized instances
loaded with public datasets (Kaggle-class)** — docker-compose services + a dataset
seeding script are part of this phase's deliverable; the cloud trio validates via
recorded fixtures + the live gate.
**Key criteria:** conformance suite green per driver (dockerized for the
self-hostable class; live halves tagged for the cloud class); CGo stays disabled
(`CGO_ENABLED=0` build proof in CI); per-engine read-only enforcement documented +
tested (dockerized proof for postgres/mysql/sqlserver, live-gated for the rest);
the dataset seeding script is idempotent and licensing-clean.

### Phase 15 — `topics-lifecycle` (Wave 5)
**RFC:** §8.1–8.3. **Briefs:** 05 (binding state machine), 03, 07. **Difficulty:** high.
The pack model (validators, canonicalized ids, join-graph checks), the unified
lifecycle (draft/review/published/deprecated + active/archived, atomic publish
swap, rollback, discard guard, typed audit, transition-authority matrix via the
resolver), **source health as lifecycle input** (+ re-check source,
reference-rewriting `update_table`), facet decomposition → vindex on publish
(invalidation matrix binding), capability contracts, topic generation job
(`enhance` role), export/import with the P6 sanitizer.
**Key criteria:** exactly-one-active-version invariant (concurrency test);
unavailable table excluded from contracts (health gating proof); publish/rollback/
archive each carry their invalidation (matrix test); export contains no internal
ids (golden); the `repair` word appears on no surface (drift-audit).

### Phase 16 — `rules-clarification` (Wave 5)
**RFC:** §8.4. **Briefs:** 03, 05. **Difficulty:** medium.
Governed rules (scoped, categorized, prioritized, structurally validated, lifecycle
+ audit), the budgeted injection lane (dropped/contradictions as named fields),
sensitive-literal enforcement, topic-scoped clarification patterns (generated at
enhancement via `clarify` role + manual editing), preflight surfacing.
**Key criteria:** rules injection respects its independent budget (token-count
test); an unmarked sensitive literal is rejected; dropped rules are enumerated in
output, never silent; clarification slots produced for ambiguous fixtures.

### Phase 17 — `nlq-routing-context` (Wave 5)
**RFC:** §9.1–9.2, §8.3. **Briefs:** 03 (deepest), 05, 08. **Difficulty:** high.
Span hints (lexicon-light Go — D-028), facet retrieval (typed candidates,
k-per-type, tenant-scoped caches), routing decisions + calibrated confidence
(published ∩ healthy ∩ granted eligibility), the `ContextAssembler` (single pruning
owner: card caps → complexity-tier budgets, one tokenizer-backed currency,
never-mutate-source, reduction logging, provenance on filters, rules lane).
**Key criteria:** routing eligibility excludes unpublished/unhealthy/ungranted
topics (table-driven); budgets hold per tier (golden token counts); pruned context
never mutates the pack (immutability test); router safe under concurrent reuse
(`-race`); no-route and clarify decisions are typed, never empty results.

### Phase 18 — `nlq-generation-execution` (Wave 5)
**RFC:** §9.3, §9.5 (allowlist stage), §9.6–9.8. **Briefs:** 03, 02, 08.
**Difficulty:** high.
Internal generation (`sqlgen`, schema-constrained), the one precedence function
(edit-base > hints > examples > default, unit-tested), the semantics-aware
validation stage (topic pack ∩ grants allowlist + join reachability), bounded
repair (≤1 validation fix, ≤1 execution fix), plan/run/preflight/refine services,
feedback + learn-positive job (DB-first weights), examples lifecycle. **This phase
closes the P1b hard gate** — generated SQL executes only from here on.
**Key criteria:** the full plan→run round-trip on the mock stack (golden); an
ungranted-but-in-topic table is rejected (the D-021 intersection proof); repair
caps enforced with typed terminal failure; plan-scope caller can never reach
execution (scope test); learn-positive survives restart (DB-first proof);
adversarial: injection corpus + cross-tenant NLQ probes green.

### Phase 19 — `byo-mode` (Wave 5)
**RFC:** §9.4, D-022. **Briefs:** 03 (Q10), 08, 12. **Difficulty:** medium.
The versioned context-bundle contract (published schema, golden-tested),
`get_query_context` (constraints restated, provenance-labeled priors, clarification
slots), `submit_sql` through the identical core, provenance recording, the
**mode-parity proof** (every validation/execution check firing for mode (a) fires
for mode (b) — asserted mechanically, not by convention).
**Key criteria:** bundle schema is versioned + backward-compat checked (golden);
an adversarial submission corpus (write smuggling, out-of-bundle tables,
multi-statement) all rejected with typed errors; parity test enumerates checks from
the validator registry itself; `query.context` without `query.submit` cannot
execute.

### Phase 20 — `charts-spec` (Wave 5, may land Wave 6)
**RFC:** §10, D-026. **Briefs:** 06. **Difficulty:** medium.
`ColumnMetadata` derivation (from executed shape + pack definitions, no extra
I/O), the deterministic selector (14-kind catalog, slot binding with the
required-slot reclaim tiebreaker, weighted suitability + intent keywords, adaptive
alternatives, table fallback at score floor), the `ResultPresentation` envelope +
provenance, golden selection suite.
**Key criteria:** selection is deterministic and pure (property test: same input ⇒
same output, no I/O); table fallback never raises; golden suite over the brief-06
fixture shapes; no ECharts/renderer types anywhere in the package (architecture
test).

### Phase 21 — `http-api` (Wave 6)
**RFC:** §11.2. **Briefs:** 01, 04, 06. **Difficulty:** high.
The full §11.2 surface: routing, validation, typed error mapping (§9.5 vocabulary
→ HTTP), transport hardening (explicit timeouts/body limits/Origin+Content-Type,
bearer-only), uploads (multipart), audit coverage asserted mechanically against the
route table, `/metrics` operator-scoped, per-route decision telemetry.
**Key criteria:** every route registered ⇒ present in the adversarial registry +
audit assertion (mechanical); tenant-mismatch across param/token ⇒ typed 400;
denied resource reads as 404 (existence-hiding, brief 04 keeper); parity
skeleton with MCP verified for the Ask tier; smoke drives one endpoint per group.

### Phase 22 — `mcp-server` (Wave 6)
**RFC:** §11.1, D-019. **Briefs:** 13, 09, 08. **Difficulty:** medium.
The 10-tool surface on mcp-go (pinned, verified): global tool middleware (scope +
grant gate + panic recovery + typed error results), streamable-HTTP + stdio +
in-process, per-surface `aud` enforcement, the **fail-closed annotation allowlist
test** (every registered tool explicitly read-only/write), tool results on the
§9.7 envelope. Preceded by the convention-10 Dockyard re-check (decision entry
either way).
**Key criteria:** unannotated tool fails CI; a tool bypassing the middleware cannot
be registered (structural test); MCP-audience enforcement (HTTP token rejected);
in-process client round-trip green; stack traces never cross the boundary
(panic-injection test).

### Phase 23 — `sdk-cli-parity` (Wave 6)
**RFC:** §11.3. **Briefs:** 01. **Difficulty:** medium.
`sdk/chartworks` (HTTP + in-process), the admin CLI (`bootstrap`, `scope-debug`,
`keys`, `erase`), and the **three-surface parity suite** (every Discover/Ask/BYO/
Feedback capability through HTTP, MCP, and SDK in one table-driven suite).
**Key criteria:** parity suite enumerates capabilities from the registration
tables (a surface gap fails mechanically); SDK in-process mode shares the one core
stack (architecture test); CLI is `run(args, stdout, stderr) int` testable; erase
cascades verified end-to-end.

### Phase 24 — `eval` (Wave 7)
**RFC:** §16, D-031. **Briefs:** 03, 12, 08. **Difficulty:** medium-high.
`chartworks eval`: the five golden suites (routing, generation, validation incl.
CTE fixture, chart selection, context budgets), the red-team suite (six adversarial
categories), CI gating (0.85 + zero criticals, mock/fixture path),
grounded-accuracy benchmark harness (BIRD/Spider-informed categories, live-gated),
golden-case seeding from positive feedback.
**Key criteria:** eval runs green in CI on the mock path; a seeded regression trips
the gate (self-test); red-team suite covers all six categories with ≥N cases each;
accuracy harness runs against the sample warehouse under the live gate (`-count=1`).

### Phase 25 — `e2e-release` (Wave 7)
**RFC:** §17 + all. **Briefs:** all (cumulative audit). **Difficulty:** medium.
E2E suites in both auth modes (self-issue + external-issuer with a local JWKS
stub), the reference Dockerfile (static binary, `CGO_ENABLED=0` proof), ops docs,
the product README in the family voice, CHANGELOG + v0.1.0 tag procedure, and the
**final cumulative audit** (the Soundings lesson: the last full-system pass catches
what every per-phase gate missed).
**Key criteria:** E2E green in both modes against fresh Docker Postgres; image
builds + serves + passes smoke from scratch; `preflight-full` green; the cumulative
audit punch list resolved; live gate green as the release blocker.

---

## Risk register (standing)

| Risk | Phase(s) | Mitigation |
|---|---|---|
| Go SQL-parser dialect coverage falls short of the six V1 engines (D-032) | 09, 14 | Parser selected against a real per-dialect fixture corpus (incl. mysql + tsql) *before* the plan bakes it in (convention 8); the `ansi` sentinel + capability gating degrade unknown constructs to typed rejections, never silent passes |
| Read-only session enforcement differs materially per engine | 10, 14 | Documented per-driver posture; **dockerized real-engine probes for postgres/mysql/sqlserver** (D-032) + live-gated probes for the cloud trio; the validator remains the primary gate, the session mode is defense-in-depth — both must hold independently |
| Upload workspace provisioning (per-tenant DBs) complicates ops | 11 | Single-instance/two-database default for dev; provisioning behind one interface so a managed-DB driver can replace it without core surgery |
| Context-budget tuning regresses generation quality invisibly | 17, 18, 24 | Token-count goldens from day one; the eval gate runs from Wave 7 backward-applied to Wave-5 fixtures; live gate scores grounded accuracy each wave end |
| BYO bundle becomes a de-facto public API before it stabilizes | 19 | `bundle_version` from the first ship; backward-compat golden; the contract is explicitly marked pre-1.0 until phase 25 |
| The 13-role gateway config sprawls | 05+ | Roles are a closed enum in config; adding one is a decision entry |
| Wave 5 is the long pole (6 phases, chained) | 15–20 | 20 is explicitly slippable; 15→18 are the critical path — staff them Opus-first; checkpoint audit at the boundary before surfaces build on them |
| Predecessor scars re-enter via familiarity (repair vocab, header trust, flag-switched writes) | all | drift-audit forbidden-word scan; architecture tests for P1c/P5/P7 land with the phase that owns each seam, not at the end |

---

*Authored per the §16 workflow inputs: RFC-001 v1.0, decisions D-001…D-031, briefs
01–13. Wave-end PRs fill the §14 checklist from the orchestrator's own gate runs.*
