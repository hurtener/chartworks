# Phase 19 — `byo-mode`

> **Status:** draft
> **Owner:** orchestrator (Wave 5)
> **Depends on:** phase-18-nlq-generation-execution

Authored per CLAUDE.md §16. Copies of the templates:

```bash
cp docs/plans/_template.md docs/plans/phase-19-byo-mode.md
cp scripts/smoke/_template.sh scripts/smoke/phase-19.sh
```

---

## RFC / request sections

- **RFC-001-Chartworks.md §9.4** — BYO-agent mode (mode b): `get_query_context`,
  `submit_sql`, one validation/execution core, provenance recording. **This phase's
  primary contract.**
- **RFC-001 §9.5 / §9.6** — the *identical* validation + execution core both modes
  converge on. This phase adds **no** new validation or execution logic; it wires the
  BYO surface into the core phases 09/10/18 already built.
- **RFC-001 §8.3** — the capability contract this phase projects into the bundle.
- **RFC-001 §9.7** — the normalized answer envelope `submit_sql` returns.
- **RFC-001 §11.1** — the two BYO tools (`get_query_context` scope `query.context`,
  `submit_sql` scope `query.submit`) and the fail-closed annotation posture.
- **RFC-001 §12** — the `queries` row (`mode ∈ internal|byo`, `bundle_version?`); no new
  table (the bundle is stateless — see Design).
- **RFC-001 §14** — the `nlq` config domain the two new keys join.
- **D-014** — dual generation modes, one validation/execution core.
- **D-022** — BYO-agent mode: published, versioned context bundle + `submit_sql` through
  the identical core; the standing parity test.
- **D-020** — the grants primitive + capability scopes the two BYO scopes plug into.
- **D-021** — the SQL-safety mechanism (`ValidatedSQL` as the only executable type) that
  makes mode-(b) unbypassability structural.
- **D-006** — asymmetric-only signing posture the `bundle_ref` handle inherits.

## Depends on

- **phase-18-nlq-generation-execution** — ships the semantics-aware validation stage
  (topic pack ∩ grants allowlist, join reachability), the `plan/run` services, and the
  **validator check registry** this phase's parity proof enumerates. Phase 18 closes the
  P1b hard gate; BYO submission executes generated SQL, so it cannot precede it.
- **phase-17-nlq-routing-context** — supplies the `ContextAssembler` / capability
  contract projection the bundle is built from (transitive via 18).
- **phase-16-rules-clarification** — supplies the governed rules + clarification slots the
  bundle **restates explicitly** (transitive via 18).
- **phase-15-topics-lifecycle** — supplies the published/healthy/granted topic-version
  eligibility the bundle pins and the submit path re-resolves.
- **phase-09 / phase-10** — the validation core (`ValidatedSQL`, typed error vocabulary)
  and read-only execution the submit path reuses unchanged.

This phase **closes no new seam of its own** — it is a thin BYO surface over the phase-18
core (P7). It does introduce one public shape (the versioned context bundle) other
surfaces (phase 21 HTTP, phase 22 MCP) build on, so it ships integration + parity tests
(§17).

## Informing briefs

Per `docs/research/INDEX.md` (BYO-agent mode → primary **03**, secondary **08**, **12**,
13):

- **Brief 03 §Q10** — the backbone: the handoff contract must be a *published, versioned,
  stable schema*; the validator is the *sole trust boundary* against an adversarial
  submitter; governance restated inside the payload; prior SQL attached as
  provenance-labeled *guidance*, not machinery.
- **Brief 08 §6** — the curated read-only tool subset for external agents; the
  URL-path-scope **anti-pattern** (scope from the token/envelope, never a caller-suppliable
  handle).
- **Brief 12 (Teramot)** — the context-handoff pattern D-014 generalizes; confirms the
  "hand context, agent generates, we validate+execute" shape as one instance of BYO mode.

## Brief findings incorporated

- **03 Q10a — the stricter justification bar.** Every bundle field is justified as "helps
  an *arbitrary* external agent produce compliant SQL," not "helps our predictor." The
  bundle therefore **enumerates** allowlisted tables/columns, dialect, and the
  single-statement/SELECT-family requirement as explicit machine-readable fields, rather
  than leaving them implicit in an internal call signature.
- **03 Q10b — validator as sole trust boundary.** `submit_sql` treats the submitter as
  adversarial and runs the *identical* §9.5/§9.6 core. No BYO-only fast path, no
  "cooperative generator" assumption. Unbypassability is structural: `ValidatedSQL` (D-021)
  is the only executable type, so the submit path *cannot* reach execution without passing
  every check.
- **03 Q10c — restated governance.** Governed rules and clarification slots (kept from
  phase 16) run before handoff and are **restated as explicit constraints inside the
  bundle** — never by topic-id reference an external agent can't dereference.
- **03 Q10d — provenance-labeled priors.** Known-good prior SQL is attached as *optional,
  provenance-and-confidence-labeled guidance*; the agent weighs it, it never silently
  constrains generation and is never treated as a validation input.
- **08 §6 — curated read-only subset + URL-scope anti-pattern.** BYO agents see only the
  read-only discovery/context/submit tools; scope for `submit_sql` derives entirely from
  the validated envelope (P2/P3), and `bundle_ref` is **not** a scope carrier — access is
  re-resolved live (see the `bundle_ref` design below). This is the direct application of
  the anti-pattern warning.
- **12 (Teramot) — pattern confirmation.** The bundle-then-submit shape is the
  Teramot-class flow; no Teramot-specific machinery is adopted (the brief is directional).

## Findings I'm departing from

- **03 Q10d — template precedence carried into the handoff.** The brief notes example
  precedence ("hints disable demos") mostly stops applying once generation is externalized.
  This plan **departs by omission**: the bundle carries *no* precedence directives at all —
  only provenance-labeled priors as flat guidance. The precedence function (§9.3) stays an
  internal-generation concern (phase 18); exporting it would leak machinery an arbitrary
  agent can't honor.
- **08 — a distinctly-scoped separate MCP tool surface for BYO** (brief 08 open question
  4). This plan keeps BYO tools in the **same** `internal/mcpserver` registration list and
  the same one core (P7); the separation is by *capability scope* (`query.context` /
  `query.submit`), not by a parallel surface. A second surface would violate P7.

## Scope

- The **versioned context-bundle contract** — a published, `bundle_version`-stamped,
  golden-tested Go type + JSON schema. Fields (all §9.4-mandated):
  `bundle_version`, `bundle_ref`, routing result, capability-contract slice, governed-rule
  constraints **restated explicitly**, dialect + SQL requirements (single statement,
  SELECT-family only, **enumerated** allowlisted tables/columns), clarification slots,
  optional provenance-labeled prior SQL.
- The **`get_query_context`** service (scope `query.context`): route → assemble → project
  the bundle → mint a `bundle_ref`. Read-only; issues **no** query.
- The **`submit_sql`** service (scope `query.submit`): `(bundle_ref, sql)` → resolve +
  verify the ref → run the *identical* §9.5 validation + §9.6 execution core → normalized
  §9.7 envelope with `generator: byo` provenance.
- **`bundle_ref` semantics** — a stateless, asymmetric-signed, TTL-bounded,
  principal-bound context handle (no new store table; see Design).
- **Provenance plumbing** — `mode/generator = byo` recorded on the `queries` row and in the
  result envelope, end to end.
- The **mode-parity proof** — a standing test that enumerates checks from the validator's
  own registry (phase 18) and asserts every one fires for a `byo` submission exactly as for
  an `internal` one.
- Packages touched: `internal/nlq` (a `byo` service area — bundle projection + the two
  services), `internal/exec` (consumed unchanged), `internal/mcpserver` (two tool
  registrations land in phase 22; this phase ships the core + HTTP-agnostic services and
  the parity/adversarial tests). Two `nlq` config keys.

## Non-goals

- **No new validation or execution logic.** Any new check would be a P7 violation — a check
  BYO runs that internal doesn't (or vice versa). All checks live in the phase-09/18 core.
- **No new store table.** The bundle is stateless-signed; §12's 25-table budget is
  untouched (`queries.bundle_version` already exists).
- **No BYO-side SQL generation, drafting, or repair.** The agent generates; Chartworks
  validates + executes. Execution-time self-repair (§9.6) is an internal-generation
  affordance and is **not** offered on the submit path (a repair would substitute machinery
  for the agent's SQL — out of the handoff contract).
- **No management-plane BYO tools.** BYO agents ask; they never administer (§11.1).
- **No bundle persistence / history API, no bundle caching across principals.**
- **MCP tool wiring + annotations** land in **phase 22** (with the fail-closed annotation
  test); HTTP `/query:context` + `/query:submit-sql` land in **phase 21**. This phase ships
  the core services those surfaces call and the parity contract they inherit.

## Design

### The context bundle

`get_query_context` reuses the phase-17 router + `ContextAssembler` to produce the internal
query context, then **projects** it into the published bundle. Projection is lossy on
purpose — internal-only machinery (precedence directives, raw pack internals, internal ids)
is dropped; every surviving field is justified against Q10a's "arbitrary agent" bar.

```
BundleV1 {
  bundle_version   string        // "v1" — the published schema tag (Risk-register: from day one)
  bundle_ref       string        // opaque signed handle (see below)
  routing {                      // §9.1 result, provenance-safe
    decision       enum          // single_topic | multi_topic | clarify | no_route
    topics []      { display_name, confidence }   // display labels only, never internal ids (§9.7)
    time_window?   string
  }
  contract         ContractSlice // §8.3 capability-contract projection (measures/dims/joins, prompt-safe)
  governance {                   // Q10c — RESTATED, never a topic-id pointer
    rules []       { statement, category, priority }   // the injected rules, verbatim as constraints
    dropped_rules? []{ statement, reason }              // budget-dropped rules named, never silent (P4)
  }
  sql_requirements {             // Q10a — enumerated, machine-readable
    dialect        string        // target adapter dialect
    single_statement   true
    statement_family   "SELECT"  // SELECT/WITH/set-ops only
    allowlisted_tables  []string // enumerated (topic pack ∩ caller grants), the intersection (D-021)
    allowlisted_columns map[table][]string
    row_cap        int           // the ceiling the submit path will clamp to (§9.6)
  }
  clarifications?  []Slot        // §8.4 slots surfaced before generation
  prior_sql?       []{ sql, provenance, confidence }   // Q10d — GUIDANCE only, provenance-labeled
}
```

`bundle_version` is a package constant (not config); a bump is a schema-evolution event
guarded by the backward-compat golden. `dropped_rules` makes the budget lane's drops
explicit (P4 — never a silent narrowing of governance the agent can't see).

### `bundle_ref` semantics (ambiguity resolved)

D-022 states `submit_sql` accepts `(bundle_ref, sql)` but leaves the ref's lifecycle open.
Resolved here:

- **Stateless & signed, not stored.** `bundle_ref` is an opaque token whose payload
  (issuing `tenant`/`principal`/`session`, pinned topic `version_id`(s), routing decision,
  target dialect, `bundle_version`, `issued_at`, `expires_at`) is **asymmetric-signed with
  the self-issue keypair** (D-006 posture — never `HS*`, never `none`) and base64-wrapped.
  This adds **no store table** (§12 budget preserved) and gives integrity + expiry for
  free. The agent treats it as opaque.
- **Principal-bound.** The ref is redeemable **only** by the same
  `(tenant, principal)` that `get_query_context` issued it to, read from the *frozen
  per-request envelope* at submit time (P2/P3). A ref presented under a different token is a
  typed rejection **before** any SQL is parsed. This is the direct answer to brief 08's
  URL-scope anti-pattern: the ref pins *which context*, never *whose access*.
- **TTL-bounded, reusable within the window.** The ref is valid until `expires_at`
  (`nlq.bundle_ttl`, default 15m) and is **not single-use**: a BYO agent commonly drafts →
  submits → refines → resubmits, and forcing a fresh routing pass per attempt is worse UX
  than internal mode's bounded repair for no safety gain. An expired ref ⇒ typed
  `bundle.expired`; a tampered/forged ref ⇒ signature-failure typed error.
- **Pins context, re-resolves capability.** The critical safety property: the ref pins the
  *routing/topic/dialect context* the agent was handed, but **access and topic
  health/publish state are re-resolved live at every submit** from the current envelope —
  never frozen into the ref. The §9.5 allowlist stage intersects the ref's pinned topic
  version schema with the caller's **live** dataset grants (D-020/D-021). A grant revoked
  between context and submit therefore shrinks the intersection and the offending table
  rejects `table.not_granted` (fail-loud). A topic that left `published`/healthy between
  context and submit rejects with a typed stale-bundle error. The ref is a *context
  contextualizer*, not a *capability token* — it can never widen access.

### `submit_sql` through the identical core

```
submit_sql(ctx, bundle_ref, sql):
  env      := frozen envelope (P2)                       // scope query.submit enforced by MCP middleware / HTTP gate
  ref      := verifyBundleRef(bundle_ref, env)           // signature + expiry + principal binding → typed error on fail
  access   := resolver.EffectiveAccess(env)              // LIVE, not from the ref (D-020)
  vsql     := exec.Validate(ctx, sql, ref.topicVersion, access, ref.dialect)   // the IDENTICAL §9.5 core; typed error codes
  result   := exec.Query(ctx, vsql, opts)                // the IDENTICAL §9.6 read-only core
  persist  query row { mode: byo, bundle_version, generator: byo, validation_json, exec_stats }
  return   §9.7 envelope { provenance.generator = byo, validation report, preview, chart spec, warnings }
```

`exec.Validate` / `exec.Query` are **the same functions** internal generation (§9.3) calls
— the only difference is the SQL's origin and the recorded provenance. `ValidatedSQL`
(D-021) being unconstructible outside the exec package is the compile-time guarantee that
mode (b) reaches execution only through the full validator.

### The mode-parity proof (mechanical, not a hand list)

Phase 18's validator exposes its checks as an **enumerable registry** — `[]CheckID` with
stable ids (`statement.blocked`, `table.not_granted`, `table.not_in_topic`,
`column.unknown`, `join.unreachable`, `single_statement`, …). The parity test:

1. reads the check registry from the validator itself (`exec.RegisteredChecks()`), never a
   list maintained in the test;
2. drives a corpus through both an `internal`-provenance and a `byo`-provenance validation,
   instrumenting which check ids fired;
3. asserts the set of checks exercised on the `byo` path **equals** the set on the
   `internal` path **and equals** the full registry (no check is skipped for BYO, none is
   BYO-only).

Because the registry is the source of truth, a new check added in a later phase is
*automatically* in scope — the parity proof cannot silently rot (the master-plan
requirement).

### Fitting the seams (CLAUDE.md §4.4 / P1–P7)

- **P1a/P1b/P3:** allowlist intersects the pinned topic schema with **live** grants inside
  the query path; empty effective access short-circuits (no query); tenant predicate rides
  every read. No fetch-then-filter.
- **P4:** every ref-verification failure, every validation rejection, every dropped rule is
  a typed error/named field + a metric — never a silent skip-to-execute, never an empty
  result reading as "nothing found."
- **P5:** `get_query_context` may route via the gateway (embedding retrieval) through the
  existing seam only; `submit_sql` makes **no** gateway call (the agent generated the SQL).
- **P7:** one core, thin BYO surface; parity test is the enforcement.

## Config keys added

Under the `nlq` config domain (§14). Both smoke-checked via the example config.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `nlq.bundle_ttl` | duration | `15m` | no | `bundle_ref` validity window; expiry ⇒ typed `bundle.expired`. |
| `nlq.byo_prior_sql_max` | int | `3` | no | Cap on provenance-labeled prior-SQL guidance entries embedded in a bundle (bounded, guidance-only; ≤ `nlq` example cap of 7). |

## Acceptance criteria

1. **Versioned bundle + backward-compat golden.** `get_query_context` returns a bundle
   carrying `bundle_version`; a golden fixture pins the published v1 schema, and a
   backward-compat golden proves a prior-`bundle_version` payload still parses (schema
   evolution guard — Risk register "BYO bundle becomes a de-facto public API").
2. **Governance restated in-bundle.** The bundle's `governance.rules` carry the active
   governed rules **as verbatim constraints** (not topic-id references); budget-dropped
   rules appear in `dropped_rules`, never silently. (Golden field-presence assertion.)
3. **SQL requirements enumerated.** `sql_requirements` enumerates `allowlisted_tables` /
   `allowlisted_columns`, `dialect`, `single_statement`, and `statement_family = SELECT`
   as explicit fields (golden). The allowlist is the topic-pack ∩ live-grants intersection.
4. **Priors are provenance-labeled guidance.** When priors exist they carry
   `provenance` + `confidence` and are capped at `nlq.byo_prior_sql_max`; they are never a
   validation input (absent-when-no-examples + field-shape test).
5. **Adversarial submission corpus all typed-rejected.** A corpus of write smuggling
   (INSERT/UPDATE/DELETE/DDL anywhere in the tree), out-of-bundle / out-of-grant tables,
   multi-statement, and dialect-escape submissions is each rejected with the §9.5 typed
   error code — zero reach execution (table-driven).
6. **Mechanical mode-parity proof.** The parity test enumerates checks from
   `exec.RegisteredChecks()` (the validator's own registry) and asserts the check set fired
   for a `byo` submission equals the set for an `internal` one and equals the full registry
   — not a hand-maintained list.
7. **Provenance recorded end to end.** A BYO submission writes a `queries` row with
   `mode = byo` / `generator = byo` and returns `provenance.generator = byo` in the
   envelope; an internal run records `internal` (differential test).
8. **Context without submit cannot execute.** A principal holding `query.context` but not
   `query.submit` is denied at the scope gate on `submit_sql` **before** validation; and
   `get_query_context` itself issues no query (store call-count assertion = 0).
9. **`bundle_ref` binding.** A ref redeemed under a different `(tenant, principal)` than
   issued is rejected before parse (cross-principal probe); an expired ref ⇒ `bundle.expired`;
   a tampered ref ⇒ signature-failure typed error.
10. **Live re-resolution (context ≠ capability).** A grant revoked between
    `get_query_context` and `submit_sql` causes the previously-allowlisted table to reject
    `table.not_granted` at submit — the ref pins context, never capability (fail-loud
    regression guard).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven bundle projection (field presence/omission), `bundle_ref`
  sign/verify round-trip, provenance recording, the two config validators. Golden tests on
  the published bundle schema + the backward-compat fixture (§11 contract-golden rule).
- **Integration:** required — this phase consumes phase-18's validator/execution core and
  introduces the bundle other surfaces build on (§17). `submit_sql` end to end against
  **Docker Postgres** with a real token and the `mock` gateway driver: route →
  `get_query_context` → submit a valid SQL → executed read-only → `byo` provenance
  persisted. Covers ≥1 failure mode (a revoked-grant resubmission) and runs under `-race`.
- **Adversarial:** required (this phase touches the ACL/SQL-safety path). The submission
  corpus (criterion 5), the cross-principal `bundle_ref` probe + empty-access short-circuit
  (criteria 8–9), the fetch-then-filter regression guard (the allowlist is intersected
  inside the query, criterion 10). Cross-tenant probe derived from the mechanical
  route/tool registry (master-plan convention 5).
- **Fuzz:** `FuzzBundleRef` over the ref decode/verify surface (asserted invariant: never
  panics, never verifies a tampered/expired/cross-principal ref). The generated-SQL
  validation fuzz target itself lives in phase 09/18; BYO reuses it unchanged.
- **Bench:** `BenchmarkBundleProjection` and `BenchmarkVerifyBundleRef` (hot per-request
  artifacts). Baseline only, not a CI gate.

## Coverage targets

Per CLAUDE.md §11 default (80% for new `internal/` packages). The BYO services and bundle
projection are new code in `internal/nlq`; validation/execution reuse is covered by phases
09/10/18. Entry added to `scripts/coverage-bands.conf` in this PR.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/nlq` (byo services + bundle) | 80% | default new-package band |
| `internal/exec` | (unchanged) | reused, not extended; its 85% band is phases 09/10's |

## Smoke checks

Each acceptance criterion maps to one assertion in `scripts/smoke/phase-19.sh` (via the
`run_group` helper in `scripts/smoke/lib.bash`). The script SKIPs cleanly until the
`internal/nlq` BYO surface exists.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestBundleVersionGolden` / `TestBundleBackwardCompatGolden` present and PASS |
| 2 | `TestBundleGovernanceRestated` PASS (rules verbatim + `dropped_rules` present) |
| 3 | `TestBundleSQLRequirementsEnumerated` PASS |
| 4 | `TestBundlePriorSQLProvenanceLabeled` PASS (cap + shape) |
| 5 | `TestSubmitAdversarialCorpusRejected` PASS (all typed-rejected) |
| 6 | `TestModeParityFromRegistry` PASS (registry-enumerated) |
| 7 | `TestByoProvenanceRecorded` PASS (row + envelope) |
| 8 | `TestContextScopeCannotSubmit` PASS + `TestGetContextIssuesNoQuery` PASS |
| 9 | `TestBundleRefBinding` PASS (cross-principal + expired + tampered) |
| 10 | `TestRevokedGrantRejectedAtSubmit` PASS |

## Glossary additions

The **Context bundle** term already exists in `docs/glossary.md` (seeded by RFC §9.4).
New terms this phase introduces, to land in `docs/glossary.md` in the same PR:

- **`bundle_ref`** — the opaque, asymmetric-signed, TTL-bounded, principal-bound context
  handle `get_query_context` mints and `submit_sql` redeems. Pins the routing/topic/dialect
  context the agent was handed; **never** carries access — grants and topic health are
  re-resolved live at submit (RFC §9.4, D-022; this plan).
- **Mode parity (BYO)** — the standing guarantee, proven by a registry-enumerated test,
  that every validation/execution check firing for internal generation also fires for a BYO
  `submit_sql` (P7; D-014/D-022).
- **Submission provenance** — the `generator ∈ internal | byo` label recorded on the
  `queries` row and in the answer envelope, distinguishing internally-generated from
  BYO-submitted SQL end to end (RFC §9.4).

## Decisions filed

References existing decisions. The concrete **`bundle_ref` semantics** (stateless signed,
TTL-bounded, principal-bound, context-not-capability) resolved an ambiguity D-022 left
open and were **ratified by the orchestrator as D-034** during the planning review; this
plan implements D-034 as specified there.

- **D-014** — dual generation modes, one core (the property this phase realizes).
- **D-022** — the versioned bundle + `submit_sql` through the identical core + the standing
  parity test (this phase's charter).
- **D-020 / D-021** — the grants primitive + `ValidatedSQL`-only execution that make BYO
  unbypassability structural.
- **D-006** — the asymmetric-only signing posture `bundle_ref` inherits.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring time. -->

- (none yet)
