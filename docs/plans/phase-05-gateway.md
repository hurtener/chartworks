# Phase 05 — `gateway` (the intelligence seam)

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** phase-01-binary-config-telemetry

---

## RFC / request sections

- **RFC-001-Chartworks.md §13** (Gateway seam — P5; D-003) — the authoritative
  spec: drivers `bifrost` + `mock`, the seven model roles, schema-constrained
  outputs, per-call metering to `gateway_call_events` + Prometheus, embedding
  model/dims pinned per index, recorded-fixture test per role.
- **RFC-001-Chartworks.md §14** (Config surface) — the `gateway` config domain:
  driver, per-role model configs, embedding model + dims (pinned).
- **RFC-001-Chartworks.md §15** (Telemetry) — "every counter is exported"; the
  telemetry conformance test; `gateway_call_events` audit-adjacent metering.
- **RFC-001-Chartworks.md §12** — the `gateway_call_events` budgeted table
  (`ts, tenant_id, stage, model, tokens_in/out, cost, latency_ms`).
- Decisions: **D-003** (one intelligence seam, Bifrost driver), **D-029**
  (embedding model + dims pinned per index, validated at boot), **D-010**
  (live-verification gate), **D-031** (eval — schema-constrained generation is a
  gated golden suite downstream).

## Depends on

- **phase-01-binary-config-telemetry** — the typed-config loader + `env:`
  indirection + per-subsystem fail-loud validators (this phase registers the
  `gateway` config domain and its boot validator into that machinery); the
  Prometheus registry + the telemetry conformance test (the gateway's per-role
  token/cost/latency counters register here and must export); the slog handler.

No other hard dependency. The metering **persistence** target
(`gateway_call_events`) is a store table shipped by phase-02, but this phase does
**not** import `internal/store`: it defines a narrow `MeterSink` interface and
consumes it, so its only compile-time dependency stays phase-01 (matching the
master-plan dependency column). The store-backed `MeterSink` is wired in
`cmd/chartworks` once both packages exist; the gateway's own tests use an
in-memory capturing sink.

## Informing briefs

- `docs/research/03-predecessor-nlq-pipeline.md` — schema-constrained generation
  (typed input/output signature, not free-text completion); the shared-generator
  concurrency lock; the LLM response cache; DSPy/GEPA prompt-pack machinery.
- `docs/research/01-predecessor-architecture.md` — the **dead-metrics scar** (an
  in-process counter registry never exported); scattered model access via
  DSPy + LiteLLM; per-stage token/batch budgets and the `model_pricing.yaml`
  cost table.

## Brief findings incorporated

- **Schema-constrained generation is the only structured-output path**
  (brief 03 §3). Every non-embedding role call carries a mandatory JSON schema
  that both constrains the provider call and re-validates the returned object; a
  violation is a typed error, never a free-text or partial parse. This
  generalizes the predecessors' "typed input/output signature" into an explicit
  seam contract and is lint-enforced (no `json.Unmarshal` of raw model text
  anywhere in the tree — RFC §13 "free-text JSON parsing … forbidden and
  lint-checked").
- **Metering must actually export** (brief 01 observability scar). The
  predecessors' counters accumulated in-process with no `/metrics` path. Here
  every call emits exactly one `gateway_call_events` row **and** increments
  Prometheus counters (`tokens_in`/`tokens_out`/`cost`/`latency` labelled by
  role); the telemetry conformance test (phase-01) fails if any registered
  gateway counter is not exported. Metering is a wrapper the drivers cannot
  bypass — it lives in the seam, not in each driver.
- **The seam consolidates all model access** (brief 01: DSPy + LiteLLM scattered
  across services). `internal/gateway` is the only package that imports a
  provider SDK or builds a provider request (P5); an architecture test fails the
  build on any such import elsewhere.
- **Reusable-artifact concurrency safety** (brief 03 §3: the shared generator's
  demo-set swap guarded by a lock). The `Gateway` is constructed once and reused
  across concurrent requests; per-call state (role, schema, inputs) rides
  parameters, never receiver fields. A `-race` concurrent-reuse test is a
  standing obligation.
- **Per-role model config + a cost table** (brief 01: per-stage token/batch
  budgets, `config/model_pricing.yaml`). Each of the seven roles carries its own
  model id, temperature, and token budget; an optional pricing map turns
  tokens into a `cost` for metering (unpriced model ⇒ `cost=0`, still metered).
- **Embedding model + dims pinned, validated at boot** (D-029; brief 03's
  retrieval layer). The configured `embedding.dims` is checked against the
  driver-reported dimension for the configured embedding model at construction;
  a mismatch refuses boot with a typed error (never a silent truncate/pad).

## Findings I'm departing from

- **The LLM response cache** (brief 03 §3) — not built into the gateway seam in
  this phase. Caching identical calls is a call-site concern (the NLQ generation
  path, phase-18) with its own invalidation semantics; baking it into the seam
  would couple metering ("every call metered") to cache hits and blur what a
  "call" is. Deferred to the consuming phase; noted as a non-goal.
- **DSPy-style predictor runtime / GEPA prompt-pack optimizer** (brief 03 §3).
  Chartworks does not embed a DSPy runtime or a genetic prompt optimizer; it
  keeps the *property* (schema-constrained, typed I/O) via JSON-schema-constrained
  Bifrost calls. Prompt-pack tuning, if it returns, is a post-V1 `semantics`/`nlq`
  concern, not a seam feature.
- **A `rerank` role.** D-003's prose mentions rerank among gateway call kinds,
  but RFC §13's role enum is exactly the seven below — rerank is not a V1 role
  and is not carried speculatively. Adding it later is a decision entry
  (risk-register: roles are a closed enum).

## Scope

Delivers `internal/gateway`:

- The **`Gateway` seam** (interface + factory + driver registration via `init()`
  blank-import, per §4.4).
- The **`Role` closed enum** — exactly seven roles:
  `embedding`, `enhance`, `sqlgen`, `sqlfix`, `clarify`, `pipeline_draft`,
  `profile_summary` — each independently configured.
- Two drivers: **`bifrost`** (wrapping `github.com/maximhq/bifrost/core`) and
  **`mock`** (backs every test; deterministic; configurable dims + canned
  responses/fixtures).
- The **schema-constrained call path**: `Complete(ctx, role, SchemaRequest)`
  constrains the provider call to a JSON schema and re-validates the returned
  object, returning a typed result or `ErrSchemaViolation`.
- The **embedding call path**: `Embed(ctx, EmbedRequest)` returning vectors whose
  dimension is validated against the boot-pinned dims.
- **Metering**: a seam-level wrapper that, after every call, emits one
  `gateway_call_events`-shaped record through the `MeterSink` interface and
  increments the Prometheus token/cost/latency counters (registered against the
  phase-01 registry). The `stage` column carries the role name.
- The **`gateway` config domain** + its fail-loud boot validator (registered into
  phase-01's config machinery), including the pinned-dims boot check.
- The **free-text-JSON lint** (architecture test) and the **provider-SDK-import
  architecture test**.
- **Recorded-fixture tests**, ≥1 per role, replaying a secret-scrubbed real
  Bifrost wire response to validate the mapping without a live paid call.

## Non-goals

- **The per-role output schemas themselves** (the sqlgen SQL-decision schema, the
  routing schema, the chart-spec schema, …). The seam ships the schema-constrained
  *mechanism*; each consuming phase (17/18/20/…) supplies and golden-tests its own
  schema. This phase golden-tests the mechanism with representative fixture
  schemas only.
- **LLM response caching** (deferred — see departures; a phase-18 concern).
- **Prompt-pack / GEPA optimization**, shadow evaluation, autopilot promotion.
- **A `rerank` role or any eighth role** (decision entry required to add one).
- **The store-backed `MeterSink` implementation** and the actual
  `gateway_call_events` migration (phase-02 owns the table; the store-backed sink
  is wired in `cmd/chartworks`). This phase defines the interface and the row
  shape only.
- **OTel span export** (telemetry-seam adapter, off by default — phase-01/§15).

## Design

### The seam

```go
type Role string

const (
    RoleEmbedding      Role = "embedding"
    RoleEnhance        Role = "enhance"
    RoleSQLGen         Role = "sqlgen"
    RoleSQLFix         Role = "sqlfix"
    RoleClarify        Role = "clarify"
    RolePipelineDraft  Role = "pipeline_draft"
    RoleProfileSummary Role = "profile_summary"
)

// Gateway is constructed once and reused concurrently. Immutable after New.
type Gateway interface {
    // Embed uses the embedding role; the returned vectors' dimension is
    // guaranteed to equal the boot-pinned dims (validated at construction).
    Embed(ctx context.Context, req EmbedRequest) (*EmbedResult, error)

    // Complete runs a schema-constrained structured generation for a non-embedding
    // role. The returned object is validated against req.Schema; a violation
    // yields ErrSchemaViolation (typed) and a nil result — never a partial parse.
    Complete(ctx context.Context, role Role, req SchemaRequest) (*SchemaResult, error)
}
```

- `EmbedRequest{ Tenant, Inputs []string }` → `EmbedResult{ Vectors [][]float32, Dims int }`.
- `SchemaRequest{ Tenant, Messages, Schema (JSON Schema), MaxTokens? }` →
  `SchemaResult{ Object json.RawMessage /* schema-valid */, Usage }`.
- Every request carries the caller `tenant` (P3) so the metering row is
  tenant-attributed; no identity/access recomputation happens here — the frozen
  envelope (phase-03) is the source, this seam only reads the tenant tag it is
  handed.

### Drivers & factory

`New(cfg Config, sink MeterSink, reg *prometheus.Registry) (Gateway, error)`
selects the driver by `cfg.Driver`, wraps it in the metering decorator, and runs
the boot validations (dims pin, per-role config presence). Drivers register via
`init()` blank-import (`_ "…/gateway/bifrost"`). `bifrost` is the only file that
imports `github.com/maximhq/bifrost/core`. `mock` is deterministic: it reports a
configurable embedding dimension and returns canned/fixture objects, and is the
sanctioned boundary mock (paired with the recorded-fixture tests below).

### Schema-constrained path (P5)

`Complete` passes `req.Schema` to the provider as a structured-output constraint
**and** re-validates the returned object against the same schema before returning.
Failure modes (missing required field, wrong type, trailing/garbage bytes,
non-JSON) all collapse to `ErrSchemaViolation` with the offending validator
detail — a typed error (P4), never a best-effort partial object. There is exactly
one JSON decoder for model output, inside this path; a repo-wide lint/architecture
test forbids `encoding/json` decoding of raw model text anywhere else.

### Metering (the anti dead-metric)

The metering decorator wraps every driver call: on return (success or typed
failure) it records `{ts, tenant_id, stage=role, model, tokens_in, tokens_out,
cost, latency_ms}` to the `MeterSink` and increments Prometheus counters
`gateway_tokens_total{role,direction}`, `gateway_cost_total{role,model}`,
`gateway_call_latency_seconds{role}`, `gateway_calls_total{role,outcome}`. Cost is
computed from `gateway.pricing[model]` (unpriced ⇒ 0, still recorded). Because the
wrapper is in the seam, no driver can emit a call that isn't metered — proven by a
capturing-sink test that asserts exactly one row + one counter tick per call.

```go
type MeterSink interface {
    Record(ctx context.Context, ev CallEvent) // CallEvent mirrors gateway_call_events
}
```

### Boot validation (fail-loud, P4)

`New` refuses to return a `Gateway` when: (a) `cfg.Driver` is unknown; (b) any of
the seven roles lacks a model config; (c) a config key names a role outside the
closed enum; (d) `driver=bifrost` but the provider key (`env:`) is unset; (e) the
driver-reported embedding dimension for the configured embedding model ≠
`cfg.Embedding.Dims`. Each is a typed error surfaced through phase-01's fail-loud
config path; readiness (`/readyz`, §15) gates on the pinned-dims validation.

### Upholding the invariants

- **P5** — the only provider-SDK importer; schema-constrained outputs; free-text
  JSON parse forbidden + lint; every call metered.
- **P4** — schema violation, dims mismatch, unknown role, missing key all fail
  loud with typed errors + metrics; no silent degrade or empty result.
- **P3** — every call is tenant-tagged for metering; no cross-tenant leakage of
  cached state (no cache here).
- **P7** — one seam, two thin drivers; metering/validation live in the core so no
  driver or caller can omit them.
- **§5 concurrency** — `Gateway` immutable after `New`; per-call state in
  parameters; `-race` concurrent-reuse test.

## Config keys added

All under the `gateway` domain (RFC §14). Documented here, in the example config,
and smoke-checked (a good gateway block boots; a bad one is refused).

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `gateway.driver` | string | — | yes | one of `{bifrost, mock}`; unknown ⇒ refused boot |
| `gateway.embedding.dims` | int | — | yes | pinned dims; validated against the driver-reported dimension at boot (D-029); mismatch ⇒ refused boot |
| `gateway.roles.<role>.model` | string | — | yes (per role) | model id for each of the seven roles; a missing role ⇒ refused boot |
| `gateway.roles.<role>.temperature` | float | `0.0` | no | sampling temperature per role |
| `gateway.roles.<role>.max_tokens` | int | per-role | no | per-call token budget |
| `gateway.bifrost.api_key` | string (`env:`) | — | when `driver=bifrost` | provider key via `env:` indirection; never logged |
| `gateway.bifrost.base_url` | string | provider default | no | optional endpoint override |
| `gateway.pricing.<model>.input_per_1k` | float | `0` | no | cost table for metering (echoes the predecessor `model_pricing.yaml`) |
| `gateway.pricing.<model>.output_per_1k` | float | `0` | no | as above; unpriced model ⇒ `cost=0`, still metered |

A config key naming a role outside the closed seven, or an unknown `gateway.*`
key, is rejected by the loader (phase-01 unknown-key rejection) — proving the
enum is closed.

## Acceptance criteria

1. **No provider SDK outside the seam.** An architecture test
   (`TestArch_NoProviderSDKOutsideGateway`) enumerates module imports and fails
   the build if `github.com/maximhq/bifrost` (or any provider SDK) is imported by
   any package other than `internal/gateway` (P5).
2. **Schema violation ⇒ typed error, never partial parse.** A `mock` driver
   returning an object that violates the request schema (missing required field /
   wrong type / trailing bytes) makes `Complete` return `ErrSchemaViolation`
   (`errors.Is`-matchable) and a **nil** result — asserted no partially-populated
   object escapes (`TestSchemaViolationTypedError`).
3. **Every call is metered.** With a capturing `MeterSink` and a snapshot of the
   Prometheus registry, every `Embed`/`Complete` call emits exactly one
   `gateway_call_events`-shaped row **and** one increment of the token/cost/latency
   counters; a registered gateway counter absent from `/metrics` fails the
   phase-01 telemetry conformance test (`TestEveryCallMetered`).
4. **Dims mismatch ⇒ refused boot.** `New` with `embedding.dims` ≠ the
   driver-reported embedding dimension returns a typed error and no `Gateway`
   (`TestEmbeddingDimsMismatchRefusesBoot`).
5. **Free-text JSON parse forbidden (lint).** An architecture/lint test
   (`TestArch_NoFreeTextJSONParse`) asserts model output is decoded only inside
   the schema-constrained path — no `encoding/json` decode of raw provider text
   elsewhere (RFC §13 lint requirement).
6. **Closed role enum.** A config naming a role outside the seven, or omitting a
   model for any of the seven, is rejected at load/boot with a typed error
   (`TestUnknownRoleRejectedAtConfig`); the enum has exactly seven members.
7. **Concurrent-reuse safe.** The `Gateway` serves concurrent `Embed`/`Complete`
   calls with no data race and no cross-call state bleed under `-race`
   (`TestGatewayConcurrentReuse`).
8. **Recorded-fixture per role.** Each of the seven roles has ≥1 test replaying a
   secret-scrubbed real Bifrost wire response and asserting the mapping, with no
   live API call (`TestRecordedFixturePerRole`).
9. **`mock` driver backs tests and resolves every role.** `driver=mock` builds a
   `Gateway` in which all seven roles resolve and both call paths work
   deterministically (`TestMockDriverAllRolesResolvable`).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven config-validation cases (unknown driver / missing role /
  extra role / missing bifrost key / dims mismatch); schema-constraint
  success + each violation class; metering row + counter assertions; the role
  enum membership test.
- **Integration:** the metering decorator against the **real** `MeterSink`
  contract via an in-memory capturing sink (the store-backed sink is exercised
  in phase-02/`cmd` wiring, not here). The gateway `mock` driver is the sanctioned
  boundary mock, **paired** with the per-role recorded-fixture tests against the
  real Bifrost wire format (criterion 8) — the §11 requirement. No live call in
  CI; the live path is the D-010 wave-end gate.
- **Adversarial:** n/a — this phase touches no ACL/auth/tenant-scope query path
  (it tags calls with the caller tenant for metering but issues no data query).
  The security-critical properties here (no SDK leak, schema constraint, metered)
  are covered by criteria 1–5.
- **Fuzz:** `FuzzSchemaConstrain` over the schema-validation/decode surface of
  `Complete` (raw candidate bytes → invariant: never panics, never returns a
  non-schema-valid object, never a partial parse). Seed corpus of valid + each
  violation class; runs as an ordinary CI test.
- **Bench:** `BenchmarkComplete_MockSchemaConstrain` and `BenchmarkEmbed_Mock`
  on the hot reusable path (baseline, not a CI gate).

## Coverage targets

**80%** for `internal/gateway`. Justification: it is a new `internal/` package,
not one of the 85%-band security-critical paths (store / vindex / auth / identity
/ access / exec). Its recorded-fixture tests validate wire-format *mapping*, not a
store-style cross-driver conformance suite, so they do not elevate it to the 85%
"conformance-tested subsystem" band; the load-bearing safety properties (P5 SDK
isolation, schema constraint, metering, dims pin) are proven by the architecture
and behavior tests (criteria 1–6), which the coverage percentage does not
substitute for. The `bifrost` driver's live-only branches are exercised under the
D-010 gate, not hermetically — counted as a documented non-hermetic surface, not
a silent lowering.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/gateway` | 80% | new `internal/` package; not an 85%-band auth/store/access path (default) |

`scripts/coverage-bands.conf` entry added in this PR: `internal/gateway 80`.

## Smoke checks

`scripts/smoke/phase-05.sh` SKIPs the whole script until `internal/gateway`
exists, then maps each criterion to a Go test via `run_group` (§lib.bash).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestArch_NoProviderSDKOutsideGateway` PASS |
| 2 | `TestSchemaViolationTypedError` PASS |
| 3 | `TestEveryCallMetered` PASS |
| 4 | `TestEmbeddingDimsMismatchRefusesBoot` PASS |
| 5 | `TestArch_NoFreeTextJSONParse` PASS |
| 6 | `TestUnknownRoleRejectedAtConfig` PASS |
| 7 | `TestGatewayConcurrentReuse` PASS (run under `-race`) |
| 8 | `TestRecordedFixturePerRole` PASS |
| 9 | `TestMockDriverAllRolesResolvable` PASS |

## Glossary additions

Pre-written for `docs/glossary.md` (landed in the same PR). The existing
`gateway` seam / Schema-constrained generation / Recorded-fixture test entries are
**not** re-added; only genuinely new terms:

- **Model role** — one of the seven closed gateway call purposes
  (`embedding`, `enhance`, `sqlgen`, `sqlfix`, `clarify`, `pipeline_draft`,
  `profile_summary`), each carrying its own model config (model id, temperature,
  token budget). The set is a closed enum; adding a role is a decision entry
  (RFC §13, D-003).
- **Metered call** — every gateway call records `{tenant, role, model, tokens_in,
  tokens_out, cost, latency}` to `gateway_call_events` and to the exported
  Prometheus counters; metering lives in the seam so no driver can bypass it (the
  predecessors' dead-in-process-counter scar is the counterexample — brief 01).

## Decisions filed

No new `D-NNN` entry. This phase implements existing decisions:

- **D-003** — one intelligence seam, `bifrost` + `mock` drivers, schema-constrained
  outputs, free-text JSON parse forbidden.
- **D-029** — embedding model + dims pinned per index, validated at boot.
- **D-010** — the live-verification gate exercises the real `bifrost` path at wave
  end (never CI).
- **D-031** — schema-constrained generation is what the downstream golden suites
  gate.

Adding an eighth role (e.g. `rerank`) would require a **new** decision entry
(risk-register: gateway roles are a closed enum) — explicitly out of scope here.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3). Empty at authoring time. -->
