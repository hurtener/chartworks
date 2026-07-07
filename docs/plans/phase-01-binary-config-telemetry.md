# Phase 01 — `binary-config-telemetry`

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** none (Wave 1 foundational phase)

---

## RFC / request sections

- **RFC-001 §3.2** — the package inventory: this phase creates `internal/config`,
  `internal/telemetry`, and the `cmd/chartworks` binary skeleton.
- **RFC-001 §3.3** — runtime shape and the **fail-loud boot sequence** (config
  validation is the first gate; store reachability / migrations / JWKS / embedding-pin
  gates are added by their owning phases 02/03/05). "One process" scaffolding — the
  shared-core construction point in `cmd/chartworks`.
- **RFC-001 §14** — the config surface: typed YAML + `env:` indirection for every
  secret, **unknown keys rejected**, per-subsystem fail-loud validators, **one naming
  convention from day one** (no flat/nested dual regime), and an example config that
  ships and is smoke-checked.
- **RFC-001 §15** — telemetry/audit/health rules: Prometheus registry with **every
  counter exported** (the dead-registry counterexample) asserted by a telemetry
  conformance test; content-free audit events.
- **Decisions:** D-002 (module path placeholder), D-005 (CGo-free static binary),
  D-008 (stdlib `flag` CLI, `run(args, stdout, stderr) int` dispatch), D-009 (preflight
  / coverage-band / drift-audit machinery this phase wires into).

## Depends on

none. This is the root of Wave 1: it stands up the module (`go.mod`), the binary
entry point, the config loader, and the telemetry primitives that phases 02 (store),
03 (auth/identity), and every later subsystem consume. It names no other subsystem's
shipped phase, so it opens seams rather than closing them.

## Informing briefs

- **Brief 01** — `docs/research/01-predecessor-architecture.md` (primary): the config
  system, the **dead in-process metrics registry scar**, the two-config-naming-regime
  scar, the fail-fast-inconsistency scar, and the single-process runtime shape.
- **Brief 04** — `docs/research/04-predecessor-security-tenancy.md` (secondary, for
  the config↔identity boundary): the header-trust scars that make "request-id is
  correlation-only, never identity" a binding rule for this phase's middleware.

## Brief findings incorporated

- **The dead-metrics scar (brief 01, "Observability" / "Scars" / Open question 5).**
  The predecessors' in-process `Counter` registry was called everywhere but
  `snapshot_counters` was imported nowhere outside its own package — no `/metrics`, no
  exporter, operationally inert. This phase closes it structurally: metrics are created
  **only** through `telemetry.Telemetry.Counter/Gauge/Histogram`, which both registers
  with the one Prometheus registry **and** records the metric name in an in-process
  manifest; a **telemetry conformance test** scrapes `MetricsHandler()` and fails if any
  manifested name is absent from the exposition. "The counters exist" and "the counters
  are observable" cannot silently diverge (brief 01 Open question 5, verbatim intent).
- **One config naming convention (brief 01, "Configuration" / Scar "two independent
  config-naming regimes").** The predecessors carried a ~120-entry
  `_FLAT_ENVIRONMENT_MAP` bridging a legacy-flat and a nested name for the same value,
  so every setting's canonical name depended on when it was added. This phase adopts a
  **single** convention: nested typed YAML keys, secrets via a uniform `env:NAME`
  value indirection — no flat-alias map, decided once.
- **Fail-loud boot, resolved consistently (brief 01, Scar "startup fail-fast is
  inconsistent").** The predecessors hard-failed on router-cache refresh but only logged
  embedding-model warmup failures. Config validation here is uniformly fail-loud (P4):
  the aggregate validator reports **all** failures at once (`errors.Join`), and a
  missing required secret or unknown key **refuses boot** with a typed error — never a
  degraded start.
- **Request-id is correlation, not identity (brief 04, header-trust scars).** The
  predecessors' `_platform_auto_auth` / `resolve_dataset_principal` treated `X-*`
  headers as the identity source of truth. This phase's request-id middleware may read
  `X-Request-Id` / `X-Correlation-Id` **only** for log/audit correlation, generating one
  when absent; it never derives identity, tenant, or access from a header (P2).
- **Content-free audit shape (brief 01, "Audit").** The predecessors' `@audited`
  decorator emitted actor/target/result with no request body. This phase provides the
  `AuditEvent` type with **no free-form data field** (ids + principal + decision +
  request-id only), so a data row or token byte is structurally impossible to log.

## Findings I'm departing from

- **Opt-in `@audited` decorator as the coverage mechanism (brief 01, "Audit" keeper).**
  The predecessors' audit fired only on explicitly decorated routes; RFC §15 requires
  **mechanical** audited-route coverage asserted against the route/tool tables. Phase 01
  ships only the emitter *interface* and a slog-backed default; the mechanical coverage
  assertion lands with the surfaces that own route tables (phases 21/22). This phase
  deliberately does not adopt opt-in decoration as the audit-coverage strategy.
- **The mlflow / experiment-tracker telemetry sink (brief 01, "Tracing / experiment
  tracking").** The predecessors bridged flow events into both OTel and an ML-experiment
  tracker, a double sink whose audit/business separation was unclear. This phase carries
  **only** an OTel adapter behind the telemetry seam, off by default (RFC §15) — no
  second experiment-tracking sink.
- **Platform-coupled bootstrap in the app factory (brief 01, Scar
  `_ensure_databricks_api_key`).** No deploy-target-specific code enters
  `cmd/chartworks`; any such concern would live behind a driver seam (CLAUDE.md §4.4),
  not the binary. None is introduced this phase.

## Scope

Packages created: `internal/config`, `internal/telemetry`, `cmd/chartworks`; plus the
module root `go.mod` (`module github.com/hurtener/chartworks`, `go 1.26` — D-002/D-005)
and the shipped `config.example.yaml`.

- **`cmd/chartworks`** — the binary skeleton on stdlib `flag` (D-008):
  `main()` → `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))`; `run(args, stdout,
  stderr) int` dispatches the closed subcommand set `serve | mcp | admin | eval |
  version`. `version` (and the `--version` flag) is **real** — it prints the
  ldflags-injected `version`/`commit`/`buildDate`. `serve`/`mcp`/`admin`/`eval` are
  stubs that load+validate config and report "not yet implemented" (no servers bound).
  `serve --config <path> --check` loads and validates config, prints `OK` or the typed
  error, and exits **without binding any socket** — the config-validation smoke handle.
- **`internal/config`** — `Load(path, lookupEnv) (*Config, error)`: typed YAML decode
  with **unknown-key rejection**, uniform `env:NAME` secret indirection, and an
  **aggregate fail-loud validator** (`errors.Join` over each sub-config's `Validate()`).
  Typed sentinel errors: `ErrUnknownKey`, `ErrMissingSecret`, `ErrInvalidValue`. This
  phase defines the top-level `env`, the `telemetry` domain, and the `store.dsn`
  required-secret archetype (see Design); later phases extend `Config` with their own
  domains additively.
- **`internal/telemetry`** — `New(cfg) (*Telemetry, error)`: a `log/slog` handler
  (JSON in prod / text in dev, selected by `telemetry.log_format`), the one Prometheus
  registry with manifest-tracked metric constructors, `MetricsHandler() http.Handler`
  (promhttp over the one registry), the request-id middleware + context helpers
  (correlation-only), and the `AuditEmitter` interface + `AuditEvent` (content-free) with
  a slog-backed default emitter.

## Non-goals

- **The HTTP/MCP servers and their `server` config domain** — `serve`/`mcp` are stubs;
  socket binding, transport hardening, `/healthz`+`/readyz`, and the operator-scoped
  `/metrics` route land in phases 21/22, which introduce the `server` config keys.
- **The `auth`, `sources`, `workspace`, `exec`, `nlq`, `gateway`, `jobs` config
  domains** — each owning phase extends `Config` with its section, validator,
  example-config block, keys table, and smoke check in its own PR (additive config
  growth; §14 discipline).
- **Store connectivity / migrations** — phase 01 owns only the `store.dsn` *config key*
  (presence + env-indirection); pgx wiring, pool sizing, migration policy, and the
  `/readyz` store-reachability gate are phase 02.
- **The store-backed audit emitter and the mechanical audited-route assertion** — phase
  01 ships the interface + a slog default only; the durable emitter (phase 02) and the
  route-table coverage assertion (phases 21/22) come later.
- **OTel export wiring** — the adapter seam is reserved (off by default); actual span
  export is deferred until a later phase needs it.

## Design

**Data flow.** `main` → `run(args, stdout, stderr)` parses the subcommand; every
non-`version` command calls `config.Load(path, os.LookupEnv)` → constructs
`telemetry.New(cfg.Telemetry)` → (in later phases) the shared core → the requested
surface. Phase 01 stops at "config loaded + telemetry constructed" and reports the stub.

**`internal/config` — one loader, fail-loud (P4, P7).**

```
type Config struct {
    Env       string          `yaml:"env"`                 // dev | prod (required)
    Telemetry TelemetryConfig `yaml:"telemetry"`
    Store     StoreConfig     `yaml:"store"`
}
type TelemetryConfig struct {
    LogFormat      string `yaml:"log_format"`              // json | text
    MetricsEnabled bool   `yaml:"metrics_enabled"`
    OTelEndpoint   string `yaml:"otel_endpoint"`           // "" = off; may be "env:CHARTWORKS_OTEL_ENDPOINT"
}
type StoreConfig struct {
    DSN string `yaml:"dsn" required:"true"`                // secret, typically "env:CHARTWORKS_STORE_DSN"
}
type Validator interface{ Validate() error }              // each sub-config implements it
func Load(path string, lookupEnv func(string) (string, bool)) (*Config, error)
```

- **Unknown-key rejection**: decode via `gopkg.in/yaml.v3` with `Decoder.KnownFields(true)`;
  an unexpected key yields `ErrUnknownKey` naming the offending path. This is the single
  naming regime — no flat-alias map (brief 01 departure).
- **`env:` indirection**: any string field whose value is `env:NAME` is resolved through
  `lookupEnv`. A field tagged `required:"true"` that resolves to empty (or references an
  unset var) yields `ErrMissingSecret` naming the field + var. `store.dsn` is the
  archetype that exercises this end-to-end at the binary (RFC §3.3 reads the store DSN
  first at config-validation time); its *use* is phase 02.
- **Aggregate validation (fail-loud, P4)**: `Load` runs every sub-config `Validate()`
  and returns `errors.Join(...)` so **all** problems surface at once — never first-error
  short-circuit, never a degraded default. `lookupEnv` is injected (not `os.Getenv`
  directly) so tests are hermetic and the missing-secret path is deterministic.

**`internal/telemetry` — the counters cannot go dark (P4; brief 01 scar).**

```
type Telemetry struct { Logger *slog.Logger; reg *prometheus.Registry; manifest []string }
func New(cfg config.TelemetryConfig) (*Telemetry, error)
func (t *Telemetry) Counter(name, help string, labels ...string) *prometheus.CounterVec
func (t *Telemetry) MetricsHandler() http.Handler        // promhttp.HandlerFor(t.reg, …)
func (t *Telemetry) RegisteredMetricNames() []string     // the manifest
// correlation-only request id (never identity — P2)
func WithRequestID(next http.Handler) http.Handler
func RequestID(ctx context.Context) string
// content-free audit (P4; brief 01 Audit shape)
type AuditEvent struct {
    TS       time.Time; Tenant, Principal, Action, ResourceType, ResourceID,
    Decision, RequestID string
}                                                        // no free-form data/body field
type AuditEmitter interface{ Emit(ctx context.Context, e AuditEvent) }
```

- **Manifest-tracked metrics**: every `Counter/Gauge/Histogram` registers with the one
  `reg` **and** appends its name to `manifest`. The **telemetry conformance test**
  scrapes `MetricsHandler()` and asserts `manifest ⊆ exposition`; a metric registered to
  a private/un-scraped registry (the predecessor's dead-end) is absent from the manifest
  *and* the handler, and an accompanying architecture test asserts metrics are only
  created via `t.Counter/…`, so a bypass fails CI. This is the structural close of the
  dead-metrics scar.
- **slog handler**: JSON (prod) / text (dev), format from config; a redaction rule keeps
  secrets/token bytes out of records (CLAUDE.md §5). One logger, injected downstream.
- **Request-id middleware**: reads `X-Request-Id`/`X-Correlation-Id` for correlation
  only, generating a fresh id when absent, storing it in `ctx`; it never reads a header
  for identity/tenant/access (P2). The `X-Request-Id` used here is a correlation label,
  not an identity input — the brief-04 distinction made explicit.
- **AuditEmitter**: interface + slog-backed default; `AuditEvent` has no data field, so a
  content-full log is impossible by construction. Tenant rides the event for future
  isolation (P3) but no query exists yet.

**Property mapping.** P2 — no identity from headers (request-id is correlation-only,
audit takes principal as a caller-supplied param). P4 — fail-loud config + fail-loud
metrics conformance + `errors.Join` completeness. P6 — no plumbing nouns on any
log/wire string (drift-audit vocab scan covers `cmd/`). P7 — one config loader, one
telemetry constructor, one CLI dispatch: no parallel paths. P1/P3/P5 — n/a at this phase
(no queries, no gateway); the `AuditEvent.Tenant` field reserves the isolation seam.

## Config keys added

Documented here, in `config.example.yaml`, and smoke-checked (CLAUDE.md §4.2). Later
phases add their own domains; phase 01 introduces only the framework plus these.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `env` | string (`dev`\|`prod`) | `dev` | yes | Deployment environment; selects the default slog handler and downstream defaults. |
| `telemetry.log_format` | string (`json`\|`text`) | `json` (prod) / `text` (dev) | no | slog handler selector; invalid value ⇒ `ErrInvalidValue`. |
| `telemetry.metrics_enabled` | bool | `true` | no | Gates the Prometheus registry + `MetricsHandler`. |
| `telemetry.otel_endpoint` | string | `""` (off) | no | OTel adapter target; `""` = off (RFC §15). May use `env:` indirection. |
| `store.dsn` | string (secret) | — | yes | Postgres DSN; the `env:`-indirected required-secret archetype (`env:CHARTWORKS_STORE_DSN`). Presence only this phase; connectivity is phase 02. |

## Acceptance criteria

1. `chartworks version` and `chartworks --version` print the ldflags-injected
   `version`, `commit`, and `buildDate`, and exit 0.
2. `chartworks serve --config config.example.yaml --check` loads and validates the
   shipped example config, prints `OK`, and exits 0 **without binding any socket**.
3. A config carrying an **unknown key** ⇒ `serve --check` fails with `ErrUnknownKey`
   (naming the key) on stderr and a non-zero exit; no surface starts.
4. A **required `env:`-indirected secret** (`store.dsn`) whose referenced env var is
   unset ⇒ refused boot with `ErrMissingSecret` (naming the field/var) and a non-zero
   exit.
5. `env:NAME` indirection resolves a **set** env var into the config value at load
   (round-trip unit test through injected `lookupEnv`).
6. Aggregate validation reports **all** failures at once via `errors.Join` (a config
   with ≥2 defects surfaces ≥2 typed errors), never first-error short-circuit (P4).
7. `telemetry.New` emits **JSON** logs when `log_format=json` and **text** when
   `=text`; no secret or token bytes appear in a record (redaction unit test).
8. The **telemetry conformance test** passes when every metric created via
   `t.Counter/…` appears on `MetricsHandler()`, and **fails** when a metric is
   registered but not exported (dead-counter scar, brief 01).
9. The request-id middleware attaches a generated id to `ctx` when no header is present
   and reuses `X-Request-Id` for correlation only; it derives **no** identity/tenant
   from any header (P2 unit test). `AuditEvent` has no free-form data field
   (compile-time shape assertion).
10. `run(args, stdout, stderr) int` is the sole dispatch (D-008); an **unknown
    subcommand** ⇒ non-zero exit with usage on stderr, and no panic crosses `main`.
11. `CGO_ENABLED=0 go build ./cmd/chartworks` succeeds — the static-binary posture
    (D-005) holds.
12. `config.example.yaml` round-trips through `Load` with **zero** unknown keys and a
    clean aggregate validation (the shipped example is itself valid).

## Test obligations

Per CLAUDE.md §11:

- **Unit:** table-driven `config.Load` cases (good / unknown-key / missing-secret /
  invalid-value / multi-defect `errors.Join` / env round-trip); `telemetry` handler
  format selection + redaction; request-id generate-vs-reuse; metric manifest tracking;
  `run()` dispatch table (version / --version / unknown subcommand / serve --check
  good+bad). The **telemetry conformance test** (manifest ⊆ exposition) and the
  **architecture test** (metrics only via `t.Counter`) are standing unit obligations.
- **Integration:** n/a — no cross-subsystem seam with a real driver (no Postgres, no
  token, no gateway) at this foundational phase. The public interfaces this phase opens
  (`config.Config`, `telemetry.Telemetry`/`AuditEmitter`, `WithRequestID`, CLI `run()`)
  are covered by unit + the conformance/architecture tests; downstream phases add the
  Docker-Postgres/real-token integration tests when they *consume* these (§17).
- **Adversarial:** n/a — no ACL/auth/query path in this phase (grants are phase 04, JWT
  is phase 03). The one adjacent obligation is P2-shaped and covered by criterion 9's
  "no identity from headers" unit test.
- **Fuzz:** `FuzzConfigLoad` over arbitrary YAML + env fixtures, asserting the invariant
  "never panics; an unknown key is always rejected; a resolved secret never leaks into an
  error string." (Config is a decode surface; not one of §11's named prime surfaces —
  JWT/NLQ/SQL — but a fuzz guard is cheap insurance here.)
- **Bench:** `BenchmarkRequestIDMiddleware` (the per-request hot middleware). `Load` is
  boot-time, not hot — no benchmark.

## Coverage targets

Per CLAUDE.md §11 convention 4 defaults (added to `scripts/coverage-bands.conf` in this
PR):

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/config` | 80 | New `internal/` package (default). |
| `internal/telemetry` | 80 | New `internal/` package (default). |
| `cmd/chartworks` | 70 | CLI/tooling band (default). |

`scripts/coverage-bands.conf` entries added:

```
internal/config      80
internal/telemetry   80
cmd/chartworks       70
```

## Smoke checks

Each criterion maps to one assertion in `scripts/smoke/phase-01.sh` (binary-driven where
the surface is the CLI, `go test -run` via `lib.bash run_group` where the assertion is
in-package). The script SKIPs whole-cloth until `bin/chartworks` exists.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `bin/chartworks version` and `bin/chartworks --version` print non-empty `version`/`commit`/`buildDate`; exit 0. |
| 2 | `bin/chartworks serve --config config.example.yaml --check` prints `OK`, exits 0, binds no socket. |
| 3 | `serve --check` on a fixture with an unknown key exits non-zero and stderr matches `ErrUnknownKey`/the key name. |
| 4 | `serve --check` with `CHARTWORKS_STORE_DSN` unset exits non-zero and stderr matches `ErrMissingSecret`. |
| 5 | `run_group internal/config TestLoad_EnvIndirection` — env round-trip resolves. |
| 6 | `run_group internal/config TestLoad_AggregateErrors` — ≥2 defects ⇒ ≥2 joined errors. |
| 7 | `run_group internal/telemetry TestNew_LogFormat` + `TestRedaction`. |
| 8 | `run_group internal/telemetry TestMetricsConformance` (pass) + `TestMetricsConformance_DetectsUnexported`. |
| 9 | `run_group internal/telemetry TestRequestID_NoIdentityFromHeader` + `TestAuditEvent_ContentFreeShape`. |
| 10 | `bin/chartworks bogus-subcommand` exits non-zero with usage on stderr; `run_group cmd/chartworks TestRun_Dispatch`. |
| 11 | `CGO_ENABLED=0 go build -o /dev/null ./cmd/chartworks` exits 0. |
| 12 | `run_group internal/config TestExampleConfig_Valid` — `config.example.yaml` loads with zero unknown keys and clean validation. |

## Glossary additions

New terms this phase introduces (pre-written for `docs/glossary.md`, landed in the same
implementation PR per CLAUDE.md §14 — not edited here):

- **Telemetry conformance test** — the standing test that scrapes the one Prometheus
  registry's `MetricsHandler` and fails if any metric created through the telemetry
  constructor is not exported; the structural close of the predecessors' dead in-process
  counter registry (brief 01, RFC §15).
- **Audit event (content-free)** — the `telemetry.AuditEvent`: ids + principal + action
  + resource type/id + decision + request-id, with **no** free-form data or request-body
  field, so a data row or token byte cannot be logged (RFC §15, CLAUDE.md §7).
- **`env:` indirection** — the single config convention for secrets: a string value
  `env:NAME` is resolved from the environment at load; a required field resolving to
  empty refuses boot with a typed error (RFC §14).

## Decisions filed

No new decisions. This phase relies on existing entries: **D-002** (module path
`github.com/hurtener/chartworks`, placeholder), **D-005** (CGo-free, `CGO_ENABLED=0`
static binary), **D-008** (stdlib `flag` CLI, `run(args, stdout, stderr) int`), **D-009**
(preflight / coverage-band / drift-audit machinery). A departure from this plan during
implementation is logged below and, if it changes a settled decision, filed as a new
`D-NNN` (CLAUDE.md §15) — never a silent edit.

## Deviation log

<!-- Filled DURING implementation. Every reasonable deviation (CLAUDE.md §4.3): what
     changed, why, and confirmation this file was updated in the same PR. -->

none yet — authoring-time plan.
