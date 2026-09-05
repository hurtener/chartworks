# Phase 25 — `e2e-release` (Wave 7)

> **Status:** draft
> **Owner:** orchestrator
> **Depends on:** all shipped phases (01–24)

The release phase. It ships **no new product capability**: it proves the whole
system end to end in both auth modes, packages the **deployment unit — the reference
image carrying the `chartworks` binary plus the pinned `bruin` executor**
(D-036/D-037), writes the operator- and consumer-facing docs, runs the **final
cumulative audit** (the Soundings lesson — the last full-system pass catches what
every per-phase gate missed), and cuts **v0.1.0**.

---

## RFC / request sections

- **RFC §17 (amended)** (Operational shape) — **the container is the deployment unit
  (D-037)**: the reference image (glibc base — debian-slim class, never bare musl)
  carries the `chartworks` binary (CGo-free today; CGo permissible per-dependency by
  decision entry) **plus the pinned `bruin` binary (v0.11.666, glibc-dynamic, telemetry
  disabled — D-036)**. Bare-metal remains supported; pipeline execution without `bruin`
  on PATH degrades to a typed "pipeline execution unavailable" (P4). The `docker-compose`
  dev stack (Postgres 16 + pgvector on 5434 + the upload-workspace database), the
  reference `Dockerfile` shipping "at the release wave", graceful shutdown (servers
  drain, job leases release, in-flight Bruin runs awaited or lease-reclaimed with status
  checkpointed).
- **D-036** (Bruin as the DE write-path executor: a CLI subprocess behind the
  `PipelineRunner` seam; **SQL-only assets — never Python/R assets, never `ingestr`**;
  credentials via `${ENV}` interpolation or a custody-rendered tmpfs config — plaintext
  never on persistent disk) and **D-037** (supersedes D-005: single-static-binary demoted
  to a *preference for the `chartworks` binary itself*).
- **RFC §3.1** (the pipeline) — the exact ordering the E2E walks: `sources → engineering
  → semantics → nlq → exec → charts`.
- **RFC §3.3 / §15** (boot + health) — fail-loud readiness (`/readyz` = store reachable
  + migrations current + JWKS fresh in external mode + embedding pin validated); this is
  the container's healthcheck target.
- **RFC §4.1** (two issuers, one validation path) — the two auth modes the E2E exercises.
- **RFC §16 / D-031** — the eval gate (phase 24) and the **live gate** (D-010) this phase
  stands up as the release blocker.
- **RFC §11** — the surfaces the cumulative audit re-verifies for parity (HTTP / MCP /
  SDK / CLI).
- **CLAUDE.md §17** (E2E + wave-end checkpoint audit), **§14** (pre-merge checklist),
  **§4.1** (preflight gate).
- Master plan **conventions 1, 2, 3, 8** (doneness, wave-end ceremony, checkpoint audit,
  the Soundings retro favors) and the **Phase 25 detail block**.

## Depends on

**Every prior phase (01–24), shipped.** This phase is the terminal integration: the E2E
suite drives the full pipeline through the real surfaces built in 21–23, over the real
store/vindex/sources/exec/semantics/nlq/charts cores (02–20), authenticated by 03/04,
observed by 01. It cannot begin until Wave 6 (surfaces) and phase 24 (eval) are green,
because it consumes all of them. It closes **no new seam** — it exercises seams every
prior phase opened.

## Informing briefs

**Cumulatively, all of 01–13** (the Phase 25 detail block: "Briefs: all (cumulative
audit)"). The E2E and the cumulative audit exist to regression-guard the specific
predecessor scars each brief named, end to end, where per-phase gates saw only their
slice:

- `docs/research/01-predecessor-architecture.md` — the dead-metrics scar; the 13
  poll-worker sprawl (now one leased queue, D-025); the flat/nested config dual regime;
  Databricks-Apps platform coupling (RFC §17's named counterexample for a
  platform-agnostic image).
- `docs/research/02-predecessor-data-and-execution.md` — the ~50-table sprawl (audit
  asserts the 25-table budget holds); the single-gate read-only validator scar
  (defense-in-depth re-proven end to end); the encrypted Connections registry.
- `docs/research/03-predecessor-nlq-pipeline.md` — the lean context-engineering crown
  jewel; template precedence; three-stage validation; the BYO handoff (mode-parity
  re-proven through the real surfaces).
- `docs/research/04-predecessor-security-tenancy.md` — HS256/header-trust authn (the E2E
  forged-header + cross-tenant probes run through the *whole* stack, not a unit); the
  adversarial test registry.
- `docs/research/05-predecessor-diff.md` — the unified topic lifecycle (the E2E walks
  draft → publish; the audit checks the exactly-one-active invariant survives real
  concurrency).
- `docs/research/06-predecessor-frontend-charts.md` — the live P6 "repair" violation (the
  audit's forbidden-word sweep is the standing guard); the declarative chart-spec slot the
  E2E asserts is returned.
- `docs/research/07-wrenai-ideas.md`, `docs/research/08-datus-agent-ideas.md`,
  `docs/research/09-agents-repo-ideas.md` — the dry-plan→dry-run→query ladder, the layered
  SQL-safety gates, the fail-closed tool-annotation allowlist (the E2E round-trips the MCP
  tools; the audit re-runs the allowlist test).
- `docs/research/10-bruin-engine-evaluation.md` — the library-import collisions that
  D-023 rejected and D-036 dissolved by narrowing to a pinned CLI subprocess (SQL-only, no
  `ingestr`/Python, telemetry disabled, glibc base). The E2E's in-container pipeline run
  and the audit's no-Python-assets / no-plaintext-secrets sweeps are the standing guards;
  the `chartworks` binary itself stays CGo-free (`CGO_ENABLED=0` build proof, D-037
  preference).
- `docs/research/11-ssr-de-pipeline-draft.md` — the DE pipeline shape the E2E's
  upload→profile→(topic) leg exercises.
- `docs/research/12-genbi-landscape.md` — the Teramot capability bar and the BIRD/Spider
  caveat (the live-gate accuracy loop is scored as *grounded*, never marketed as
  BIRD-comparable).
- `docs/research/13-dockyard-mcp-surface.md` — the mcp-go-now decision (D-019); the audit
  confirms the Dockyard re-evaluation entry (Wave-5) exists.

## Brief findings incorporated

- **The last full-system pass is not optional (the Soundings lesson, master-plan
  convention 8 / §17).** Per-phase gates each verify one slice; wiring gaps and drift live
  *between* slices. This phase's cumulative audit is a single read-only sweep over every
  shipped phase with a fixed checklist (below), landing its punch list as one
  `chore(checkpoint)` inside the release PR.
- **Fresh-database proof, no pre-migrated dev DB (convention 8).** The E2E stack
  (`docker-compose.e2e.yml`) starts an *empty* Postgres and migrates on boot, so a
  fail-loud boot guard that a long-lived dev DB would mask is exercised every run.
- **Defense-in-depth re-proven at the top (brief 02/04).** The predecessors' validator was
  strong but was the *entire* guarantee, reachable-around. The E2E runs the injection /
  DDL-smuggle / cross-tenant / forged-header probes through the real HTTP + MCP surfaces,
  not just the `exec` unit tests — proving the gate holds where a caller actually stands.
- **Platform-agnostic image (brief 01).** The Dockerfile has no platform-coupled bootstrap
  (the Databricks-Apps coupling is the named counterexample); it is two pinned binaries +
  a `/readyz` healthcheck on a plain glibc base, runnable anywhere.
- **The subprocess boundary is verified where it deploys (brief 10, D-036).** Bruin's
  objections dissolved only in the narrowed shape — so the E2E proves exactly that shape
  *inside the container*: pinned version, telemetry disabled, SQL-only pipeline run
  through the `PipelineRunner` seam, no plaintext credentials on persistent disk, and the
  bare-metal degraded mode failing loud (P4), never silently.
- **The live gate is load-bearing for a model-driven product (D-010, brief 12).** Chartworks'
  routing and SQL generation are model-driven, so a real-provider run via `.env` blocks the
  release exactly as it blocks a wave — the "every gate green, production broken" failure P4
  exists to prevent.
- **Domain vocabulary is a standing sweep (brief 06, P6).** The "repair" word (the
  predecessors' live P6 violation) is re-scanned across every surface string in the audit;
  the domain-clean names are `re-check source` / `revalidate` (glossary).

## Findings I'm departing from

**None.** This phase authors no new design; it verifies inherited ones. The only
deliberate *scoping* choice — deferring charts rendering and any frontend (D-013) — was
settled at kickoff, so the E2E asserts a **chart spec** is returned (the reserved slot),
never a rendered chart. That is adherence to D-013, not a departure.

## Scope

Docs, tests, and packaging only — no `internal/` capability code.

1. **E2E suites (`test/e2e/`, run by `make e2e`, `-race -p 1 -count=1`):**
   - `TestE2ESelfIssue` — the full walk in **self-issue** mode against a fresh Docker
     Postgres: `admin bootstrap` (first admin, D-030) → mint an API key → exchange for a
     short-lived JWT → register a `postgres` data source **and** upload a CSV → the upload
     workspace registers a dataset (D-024) → run the profile job to `completed` → author a
     topic and **publish** it (lifecycle) → `preflight` → `plan` → `run` a question on the
     `mock` gateway → assert a normalized **chart spec** rides the result.
   - `TestE2EExternalIssuer` — the Discover→Ask walk in **external-issuer** mode: a **local
     JWKS stub** (`test/e2e/jwksstub`, an httptest server serving a generated RS256 keypair)
     mints a Pengui-shaped token (`tenant`, prefixed `sub`, per-surface `aud`, `scopes`,
     grants); Chartworks validates it fail-closed and serves the same flow — proving the
     validation path is identical for both issuers (RFC §4.1). Runs against its **own** fresh
     database.
   - `TestE2EPipelineRun` — the **in-container DE write-path walk** (D-036): against the
     compose stack, define a SQL-only pipeline with a dockerized-Postgres destination,
     run it through the `PipelineRunner` seam (Bruin subprocess inside the container),
     and assert the materialization lands, lineage is recorded, and **no plaintext
     connection secret touches persistent disk** during the run.
   - `TestE2EDegradedNoBruin` — the **bare-metal degraded mode** (D-037, P4): with no
     `bruin` on PATH, a pipeline run returns the typed "pipeline execution unavailable"
     error (+ metric) — never a silent skip; NLQ reads keep working (they never route
     through Bruin, D-036).
   - All suites carry the through-the-stack adversarial obligations (§11): a cross-tenant
     probe, a forged-`X-*`-header attempt, a DDL/injection submission — each rejected with a
     typed error at the surface, not the unit.
2. **The reference `Dockerfile`** (multi-stage: `golang:1.26` builder with `CGO_ENABLED=0`
   for the `chartworks` binary + a pinned, checksum-verified `bruin` v0.11.666 fetch stage →
   a **glibc-class runtime** (`debian:stable-slim`; never distroless-static/musl — Bruin is
   glibc-dynamic, D-036/D-037)), running as a **non-root** user, Bruin telemetry disabled in
   the image, with a `HEALTHCHECK` against `/readyz`, and **`docker-compose.e2e.yml`** (fresh
   Postgres 16 + pgvector, the upload-workspace database, a dockerized-Postgres pipeline
   destination, the `mock` gateway — secret-free).
3. **Ops docs** — `docs/ops.md` (the operator runbook: config surface pointer, boot/health,
   auth-mode setup, backup/restore of the store, `admin` CLI reference including
   `bootstrap`/`keys`/`scope-debug`/`erase`, graceful shutdown, the live-gate `.env`).
4. **The product `README.md`** rewritten in the family voice (outline below).
5. **`CHANGELOG.md`** — a `[0.1.0]` section (Keep a Changelog) and the documented
   **v0.1.0 tag procedure**.
6. **The cumulative audit** — `docs/audit/wave-7-cumulative.md` (the checklist + findings +
   resolution status) and `scripts/audit-cumulative.sh` (the mechanical half, exits 0 only
   when every machine-checkable item passes).

## Non-goals

- **No new capability, endpoint, MCP tool, config key, migration, or schema change.** A
  bug the E2E surfaces is fixed in the phase that owns it (§17), in this PR, but the fix is
  that phase's code, not new Phase-25 surface.
- **No frontend / chart rendering** (D-013) — the E2E asserts the chart *spec*, nothing
  renders it.
- **No CI-required live/provider run** — the live gate is manual and `.env`-driven (D-010);
  CI runs the `mock`/fixture path only.
- **No `docker compose up` inside `make preflight`** — the smoke script does cheap
  mechanical checks and gates the heavy E2E/build/live runs behind `E2E=1` / `LIVE=1`
  (they run in `make e2e`, `make preflight-full`'s CI job, and the release checklist), so
  the commit loop stays fast.
- **No release automation** (a publishing pipeline) — the tag procedure is documented and
  run by hand for v0.1.0.

## Design

### The E2E suite shape

One Go test binary under `test/e2e/`, using the **real drivers on every seam** (Docker
Postgres for `store` + `vindex` + the upload workspace; the gateway **`mock`** driver — the
one sanctioned boundary mock, §17 — so no provider key is needed in CI). Each top-level test
provisions its **own fresh database** (a uniquely-named DB created on the shared instance,
migrated on boot, dropped on teardown) so the two modes never share state and convention 8's
fresh-DB proof holds per run. `-p 1` serializes the suite (shared Postgres instance);
`-count=1` defeats caching.

**Self-issue walk (`TestE2ESelfIssue`) — one assertion per pipeline stage (RFC §3.1):**

| Step | Surface | Proves |
|---|---|---|
| `chartworks admin bootstrap --tenant demo` | CLI | first-admin provisioning without a header-exchange endpoint (D-030) |
| `admin keys create … --scopes …` | CLI | API key minted once, never recoverable (constant-time compare path) |
| `POST /v1/token` (api_key → JWT) | HTTP | self-issue mint, per-surface `aud` (D-006) |
| `POST /v1/sources` + `:test` | HTTP | data-source registration, secret-free read shape, status lifecycle |
| `POST /v1/uploads` (CSV) | HTTP | upload → workspace dataset via the standard adapter (D-024) |
| profile job → `completed` | HTTP poll | the leased queue (D-025) + profiling (RFC §7.2) |
| topic create → `:publish` | HTTP | the unified lifecycle + facet decomposition to `vindex` (RFC §8.2) |
| `POST /query:preflight` / `:plan` / `:run` | HTTP + MCP | routing → context → generation (mock) → validation → read-only exec |
| chart spec on the result | HTTP | the D-013 reserved slot is populated (`ColumnMetadata[]` + `ChartRecipe`) |

**External-issuer walk (`TestE2EExternalIssuer`):** the `jwksstub` package stands an
`httptest` server exposing `/.well-known/jwks.json` for a freshly generated RS256 keypair;
Chartworks boots with `auth.mode=external_issuer`, `auth.jwks_url` → the stub, `iss`/`aud`
pinned. The test mints a token with the stub's private key (tenant + prefixed `sub` + the
per-surface `aud` + `scopes` + grants), then walks Discover (`list_topics` /
`describe_dataset`) → Ask (`preflight` / `plan` / `run`). It additionally proves the
fail-closed paths: an `HS256` token rejected at the parser, a stale-JWKS window ⇒ `not-ready`
+ typed 401, and a wrong-`aud` (HTTP token on the MCP surface) rejected — the D-006 invariants
exercised through the real server, not a unit.

Both walks close with the standing adversarial set run **through the surface**: a
second-tenant token seeing nothing of tenant `demo` (cross-tenant), a forged
`X-Tenant`/`X-User` header having no effect (P2), and a `DROP TABLE`/`UNION`-smuggle
submission (mode-b `submit_sql`) rejected with a typed `validation.*` error (P1b).

**In-container pipeline run (`TestE2EPipelineRun`, D-036):** drives the *containerized*
server (the compose stack — the only place `bruin` is guaranteed present): create a
SQL-only pipeline whose declared destination is the dockerized-Postgres destination DB →
`POST /pipelines/{id}:run` → poll the run to `completed`. Asserts: the materialization
exists in the destination; lineage + freshness are stamped; the run is audited/metered by
Chartworks (Bruin is the executor, not the designer); and — by inspecting the container's
writable layer and volumes during/after the run — **no plaintext connection secret is
present on persistent disk** (credentials ride `${ENV}` interpolation or a
custody-rendered tmpfs config).

**Degraded mode (`TestE2EDegradedNoBruin`, D-037/P4):** boots the server bare-metal with
`bruin` absent from PATH. A pipeline run request returns the typed
"pipeline execution unavailable" error and increments its metric — never a silent skip,
never a queued-forever run — while Discover/Ask keep working (NLQ reads never route
through Bruin, D-038 posture per D-036).

### The reference Dockerfile (two binaries, glibc base — D-036/D-037)

The container is the deployment unit (D-037): the image carries the `chartworks` binary
**and** the pinned `bruin` executor. Bruin's prebuilt binary is **glibc-dynamic** (needs
`libc`, `libstdc++`, `libgcc_s`), so the runtime base is **glibc-class debian-slim** —
never `distroless/static`, `scratch`, or musl/Alpine (D-036's hard rule).

```dockerfile
# chartworks builder — pinned Go; CGO_ENABLED=0 remains the preference for the
# chartworks binary itself (D-037 — the binary is CGo-free today).
FROM golang:1.26 AS build
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o /out/chartworks ./cmd/chartworks

# bruin fetch — pinned v0.11.666 (D-036/D-035), checksum-verified against the
# release asset; the pin is a build ARG so a bump is one reviewed line + a
# conformance re-run, never a floating "latest".
FROM debian:stable-slim AS bruin
ARG BRUIN_VERSION=v0.11.666
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
 && curl -fsSLo /tmp/bruin.tar.gz "<pinned release asset for ${BRUIN_VERSION}>" \
 && echo "<pinned sha256>  /tmp/bruin.tar.gz" | sha256sum -c - \
 && tar -xzf /tmp/bruin.tar.gz -C /out

# runtime — glibc base (bruin is glibc-dynamic: libc/libstdc++/libgcc_s),
# non-root, telemetry disabled, self-describing health.
FROM debian:stable-slim
RUN useradd --system --uid 65532 --home /nonexistent --shell /usr/sbin/nologin chartworks
COPY --from=build /out/chartworks /usr/local/bin/chartworks
COPY --from=bruin /out/bruin /usr/local/bin/bruin
# Bruin telemetry OFF, baked into the image (D-036; the exact knob is the one
# phase 13 confirmed as its implementation blocker).
ENV <BRUIN_TELEMETRY_DISABLE_KNOB>=1
USER chartworks
EXPOSE 8080 8081 9090
HEALTHCHECK --interval=10s --timeout=3s --retries=5 \
  CMD ["/usr/local/bin/chartworks", "healthcheck", "--url", "http://localhost:8080/readyz"]
ENTRYPOINT ["/usr/local/bin/chartworks"]
CMD ["serve"]
```

Load-bearing properties, each a regression guard the smoke asserts mechanically:

- **`CGO_ENABLED=0` for the `chartworks` binary** — the D-037 preference; the E2E build
  runs `file` on it and asserts *statically linked* (the per-dependency-CGo rule means any
  future dynamic linkage of *chartworks itself* must trace to a decision entry, never drift).
- **Glibc-class base, never musl** — `bruin` must load; the E2E asserts
  `bruin --version` inside the container reports exactly the pinned **v0.11.666**.
- **Bruin telemetry disabled** — the disable mechanism (confirmed by phase 13, a D-036
  implementation blocker) is baked in as image ENV and verified by the E2E (no telemetry
  egress observed / the CLI reports telemetry off).
- **Non-root `USER`** — an unprivileged system UID; retained from the original design.
- **`HEALTHCHECK` → `/readyz`** — Docker/orchestrator liveness rides the same fail-loud
  readiness gate the ecosystem derives "healthy" from (RFC §15); if migrations aren't current
  or JWKS is stale, the container reports unhealthy, never "up but degraded."
- **Credential hygiene at run time** — pipeline runs render Bruin connection config via
  `${ENV}` interpolation or a custody-rendered **tmpfs** file (D-036); the E2E pipeline run
  asserts no plaintext connection secret lands on a persistent volume/layer.

`docker-compose.e2e.yml` composes this image with a fresh Postgres 16 + pgvector, a
dockerized-Postgres pipeline destination, and the `mock` gateway; Chartworks migrates on
boot; the whole thing is **secret-free** (no provider key) so
`docker compose -f docker-compose.e2e.yml up --build` is the from-scratch smoke.

### Ops docs inventory (`docs/ops.md`)

1. **Run it** — the reference image + compose, the three ports (HTTP 8080 / MCP 8081 /
   metrics 9090), the config surface pointer (RFC §14) and the example config.
2. **Boot & health** — the fail-loud boot sequence (RFC §3.3), `/healthz` vs `/readyz`,
   what makes readiness flip, graceful shutdown behavior (§17).
3. **Auth modes** — self-issue setup (keypair, API keys, `/v1/token`) and external-issuer
   setup (issuer, JWKS URL, `jwks_max_stale`, the dual per-surface audiences), D-006/D-030.
4. **The `admin` CLI** — `bootstrap` / `keys` / `scope-debug` / `erase`, with the erasure
   contract (hard-delete across store rows + facet vectors + workspace + uploaded files,
   loud on partial failure, RFC §15).
5. **Data-source credentials** — the key ring (`env: CHARTWORKS_SOURCE_KEYS`), rotation,
   the never-logged/never-echoed rule (§7, D-016).
6. **Observability** — the metrics families (RFC §15), the scope-debug diagnostic as the
   sanctioned "why no data / why no route" answer (never a user-facing repair surface).
7. **The live gate** — the `.env` shape (a real provider key through `bifrost`) and how to
   run the manual accuracy loop + release blocker (D-010, D-031).
8. **Pipeline execution & Bruin** — the container as the deployment unit (D-037); the
   pinned `bruin` v0.11.666 (bump = one reviewed pin + conformance re-run, D-035
   discipline); telemetry-disable verification; credential hygiene (`${ENV}` / tmpfs —
   never plaintext on persistent disk); the **bare-metal degraded mode** (no `bruin` on
   PATH ⇒ typed "pipeline execution unavailable"; NLQ unaffected).

### The product README outline (family voice)

Matching the sibling cadence (short-declarative tagline → why-it-exists → the-call-is-the-
pitch → core ideas → where-it-fits with scope negations → quickstart). **No centered logo
block yet** — no `chartworks-logo-full.png` asset exists; the block lands the day an asset
does (the sibling READMEs gate the logo on the asset, so its absence is not drift).

1. **Title + one-line tagline** — "A Go-native Explorer. Engineers structured data.
   Models it into topics. Answers questions as validated, read-only SQL. Deny-by-default
   grants inside every query. One binary, two surfaces."
2. **Nav line** — Quickstart · Surfaces · Auth & access · Architecture · Operations
   (`docs/ops.md`) · RFC.
3. **What Chartworks is** — the dual-surface, standalone-and-ecosystem, swappable Explorer
   seat (RFC §1); status **v0.1.0**, pointing at the RFC / decisions / CLAUDE.md.
4. **Why Chartworks exists** — structured-data analytics needs more than a text-to-SQL
   endpoint: someone has to connect and profile the sources, model them into governed
   topics, route a question to the right one, generate SQL that is *validated read-only*
   before it ever touches data, and refuse a row the caller has no grant for — computed
   *inside* the query. The clean-room-rewrite paragraph (good bones kept: the lean
   context-engineering layer; scars shed: id sprawl, the "repair" button, header-trust
   identity, the reach-around-the-validator execute path).
5. **The ask call, in full** — a copy-pasteable `curl` of `POST /v1/query:run` returning a
   normalized result: routing evidence + the validated SQL + a result preview + the chart
   **spec** (evidence and a spec, not a rendered chart — the calling surface renders).
6. **Core ideas** — deny-by-default grants inside the query (P1a) · SQL-safety: validated,
   read-only, schema-allowlisted, injection-guarded (P1b) · the governed write path is a
   *separate* interface NLQ can't reach (P1c) · identity in the signed token, never a
   header (P2) · tenant isolation at the storage layer (P3) · fail loud (P4) · one
   intelligence seam (P5) · domain vocabulary, not plumbing — `re-check source`, never
   "repair" (P6) · one core, thin surfaces (P7) · two SQL-generation modes, one validation
   core (D-014).
7. **Where Chartworks fits** — the family table (Portico / Harbor / Dockyard / Stowage /
   Soundings / **Chartworks**) and the **scope negations**: *not* unstructured documents
   (that's Soundings) · *not* identity or sharing policy (that's Pengui's) · *not* a chart
   renderer or frontend (deferred, D-013) · *not* answer composition (the calling agent's) ·
   *not* the MCP gateway (Portico's).
8. **Quickstart (Docker, secret-free)** — the `docker-compose.e2e.yml` stack + the self-issue
   walk (bootstrap → key → token → source/upload → publish → ask), mirroring §5's E2E.
9. **Surfaces** — the MCP 10-tool table (RFC §11.1) and the HTTP group table (§11.2), each
   with scope + effect.

### CHANGELOG + v0.1.0 tag procedure

`CHANGELOG.md` gains a `## [0.1.0] — 2026-…` section under Keep-a-Changelog headings
(`Added` — the full V1 capability inventory: DE stage, semantic model + lifecycle, dual-mode
NLQ-to-SQL, chart spec, dual surfaces, dual auth; and the standing non-goals as a note). The
documented, hand-run procedure:

1. Land every Wave-7 change (E2E, image, docs, audit punch list) on `main` via the reviewed
   wave PR; CI green; `make preflight-full` green.
2. Move the `[Unreleased]` entries under `[0.1.0]`; set the date; add the compare links.
3. `git tag -a v0.1.0 -m "Chartworks v0.1.0"` on the release commit (**unsigned** —
   `commit.gpgsign=false`, CLAUDE.md §12); `git push origin v0.1.0`.
4. The tag points at the commit whose tree passes the full §14 checklist and the live gate.

### The final cumulative audit — method + checklist

A single **read-only** full-system pass (master-plan convention 8; §17 wave-end checkpoint,
here at the terminal wave). Findings and status live in `docs/audit/wave-7-cumulative.md`;
`scripts/audit-cumulative.sh` runs the mechanical half and exits non-zero on any machine-
checkable miss. The punch list lands as one `chore(checkpoint)` inside the release PR — a
bug is fixed in the owning phase's code, in the same PR (§17). **Checklist:**

*Invariants (P1–P7), re-verified end to end, not per-slice:*
1. **P1a** — every store/warehouse query path takes a non-optional scope; an empty
   effective-access set short-circuits with no query issued (store call-count assertion,
   sampled across the surfaces).
2. **P1b** — no path executes generated/submitted SQL that isn't `ValidatedSQL`;
   `ValidatedSQL` is unconstructible outside `internal/exec` (compile-time proof still holds).
3. **P1c** — no NLQ/BYO code path can reach `sources.Materializer` (the write-path
   architecture test still holds).
4. **P2** — no `X-*` header is consulted for identity/access/tenant anywhere; the E2E
   forged-header probe is green.
5. **P3** — the tenant predicate is inescapable on every read/write; the E2E cross-tenant
   probe returns nothing at every surface.
6. **P4** — no silent degrade: readiness fails loud, a denied scope/dead dependency yields a
   typed error + metric (grep for empty-catch / silent-fallback patterns).
7. **P5** — no provider SDK/HTTP outside `internal/gateway` (architecture test); every
   gateway role has ≥1 recorded-fixture test.
8. **P6** — the forbidden-word sweep is clean; `repair` appears on no wire/UI/error/log
   surface; new vocabulary is in the glossary.
9. **P7** — every capability is on all its surfaces (the three-surface parity suite from
   phase 23 enumerates from the registration tables and is green).

*Hygiene + budget:*
10. The store schema equals the **25-table** RFC §12 inventory — no table/column drift, no
    id duplicated across tables (brief 02 sprawl guard).
11. Every route + MCP tool is present in the mechanically-derived adversarial registry **and**
    the audit-coverage assertion (§15).
12. Coverage bands: every package has a configured band, no band silently lowered, no
    unbanded package (convention 4).
13. Migrations are forward-only; none edited after merge.
14. Every config key is documented in a plan + the example config + a smoke check
    (convention: `make preflight-full` proves the smokes).
15. Every `D-NNN` / `RFC §X` / `brief NN` cross-reference resolves (`make drift-audit`).
16. The Wave-5 Dockyard re-evaluation decision entry exists (D-019 obligation).
17. The fuzz corpora (JWT, NLQ payloads, SQL-validation) run as ordinary CI tests; the
    adversarial obligations (injection, schema-escape, cross-tenant, resource exhaustion,
    BYO) are green (phase 24 red-team + the E2E through-surface probes).
18. **No plaintext connection secret on persistent disk during a pipeline run** (D-036) —
    credentials reach Bruin only via `${ENV}` interpolation or a custody-rendered tmpfs
    config; the E2E pipeline run's disk sweep is green and the rendering code path has no
    persistent-write branch (code inspection + the mechanical sweep).
19. **No `ingestr`/Python assets reachable in V1** (D-036's hard rule) — the pipeline
    definition validator rejects non-SQL asset kinds (typed error); no `ingestr` binary or
    Python runtime exists in the reference image; no code path invokes `bruin` with an
    ingestion/Python asset (grep + image inspection + a rejection test).

## Config keys added

**None.** This phase adds no config surface (RFC §14 is complete). The E2E stack sets
existing keys (`auth.mode`, `store` DSN, `workspace` DSN, `gateway.driver=mock`, the
per-surface audiences) via `docker-compose.e2e.yml` env and an example config; the live gate
sets `gateway.driver=bifrost` + a provider key from `.env` (D-010) — again existing keys.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| — | — | — | — | none added |

## Acceptance criteria

1. **`TestE2ESelfIssue` green** — the full self-issue walk (bootstrap → key → token →
   source + upload → profile → topic publish → preflight/plan/run → chart spec) passes under
   `-race -p 1 -count=1` against a **fresh** Docker Postgres, and its through-surface
   adversarial probes (cross-tenant, forged-header, DDL-smuggle) all reject with typed errors.
2. **`TestE2EExternalIssuer` green** — the external-issuer Discover→Ask walk passes against a
   **fresh** DB via the local JWKS stub, including the fail-closed rejections (`HS256` at the
   parser, stale-JWKS ⇒ not-ready 401, wrong-`aud` cross-surface).
3. **The `chartworks` binary builds `CGO_ENABLED=0` on a glibc-class base** — the build
   stage sets `CGO_ENABLED=0` (`file` reports *statically linked* — the D-037 preference
   proof for the binary itself), and the runtime base is glibc-class debian-slim (never
   `distroless/static` / `scratch` / musl — Bruin is glibc-dynamic, D-036).
4. **The image is non-root with a `/readyz` healthcheck** — the Dockerfile declares a
   non-root `USER` and a `HEALTHCHECK` targeting `/readyz`.
5. **`docker compose -f docker-compose.e2e.yml up --build` serves and smokes from scratch** —
   the container migrates on boot and `/readyz` returns ready with no pre-seeded DB and no
   provider key.
6. **`make preflight-full` is green** — build + every phase's smoke (each OK ≥ its criteria,
   FAIL = 0) + `drift-audit`.
7. **The cumulative audit punch list is resolved** — `scripts/audit-cumulative.sh` exits 0,
   and `docs/audit/wave-7-cumulative.md` records every checklist item as resolved (no open
   finding).
8. **The product `README.md` is in the family voice** — it contains the required sections
   (why-it-exists, the-ask-call-in-full, core ideas, where-it-fits with scope negations,
   quickstart) and points at the RFC / decisions / CLAUDE.md.
9. **`CHANGELOG.md` carries a `[0.1.0]` section** and the documented tag procedure; the
   annotated, unsigned `v0.1.0` tag points at the release commit.
10. **`docs/ops.md` exists** with the operator runbook inventory (run/health/auth/admin-CLI/
    credentials/observability/live-gate sections).
11. **The live gate is green as the release blocker (D-010)** — the real-provider E2E via
    `.env` passes `-count=1` (manual; recorded in the release checklist; never CI-required).
12. **`bruin` is present in the image at the pinned version** — `bruin --version` inside
    the container reports exactly **v0.11.666** (D-036/D-035; the fetch stage is
    checksum-verified against the release asset).
13. **Bruin telemetry is disabled in the image** — the phase-13-confirmed disable
    mechanism is baked in as image ENV and verified inside the container (the CLI reports
    telemetry off / no telemetry egress observed during the pipeline run).
14. **`TestE2EPipelineRun` green in-container** — a SQL-only pipeline with a
    dockerized-Postgres destination runs to `completed` through the `PipelineRunner` seam
    inside the compose stack; the materialization + lineage are asserted, and the disk
    sweep finds **no plaintext connection secret on persistent disk** during the run.
15. **`TestE2EDegradedNoBruin` green** — bare-metal with no `bruin` on PATH, a pipeline
    run returns the typed "pipeline execution unavailable" error + metric (P4, never
    silent), while Discover/Ask continue to work.

## Test obligations

Per CLAUDE.md §11:

- **Unit:** the JWKS stub (`test/e2e/jwksstub`) and the E2E harness helpers (fresh-DB
  provisioning, token minting, poll-until) carry table-driven unit tests for their own
  logic (keypair generation, JWKS JSON shape, teardown idempotency).
- **Integration / E2E:** the E2E suites (auth-mode walks + the in-container pipeline run +
  the degraded mode) **are** the integration tests for this phase —
  real drivers on every seam (Docker Postgres for store/vindex/workspace; the gateway
  `mock` driver, the one sanctioned boundary mock), identity/scope propagation proven across
  the whole pipeline, ≥1 failure mode per mode (fail-closed JWKS; typed rejection on an
  adversarial submission), `-race`. They live in `test/e2e/` (the pipeline, not one package,
  is the boundary).
- **Adversarial:** required and central — cross-tenant probe, forged-`X-*`-header attempt,
  empty-access short-circuit, DDL/injection-smuggle, wrong-`aud` replay — each run **through
  the real surface** (the E2E's whole point vs the per-phase units). The cumulative audit
  re-runs phase 24's red-team suite as a gate.
- **Fuzz:** n/a for new code — this phase adds no parse/decode surface; the audit *verifies*
  the standing fuzz targets (JWT, NLQ, SQL-validation) exist and run in CI.
- **Bench:** n/a — no new hot reusable artifact; the audit verifies the standing benchmarks
  still run under `make bench`.

## Coverage targets

Docs, the Dockerfile, compose, and the audit manifest are not Go and carry **no coverage
band** (n/a — they are exercised by the E2E run and the smoke's mechanical checks, not by
unit coverage). The new Go helper code (the JWKS stub + E2E harness utilities) is tooling
and takes the **70%** band. The E2E suites themselves drive product code across packages;
their attribution is not a per-package band (they run under `make e2e`, not the coverage
gate) — recorded here so the band gate has no unbanded-package failure.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `test/e2e` (harness + `jwksstub`) | 70% | E2E/tooling band (convention 4; §11 CLI/tooling default) |
| `docs/*`, `Dockerfile`, `docker-compose.e2e.yml`, `docs/audit/*` | n/a | non-Go; verified by the E2E run + mechanical smoke, not unit coverage |

## Smoke checks

`scripts/smoke/phase-25.sh` runs cheap **mechanical** assertions in `make preflight` and gates
the heavy `docker`/E2E/live runs behind `E2E=1` / `LIVE=1` (so the commit loop stays fast; the
full runs execute under `make e2e`, the CI `preflight-full` job, and the release checklist).
The script SKIPs entirely until the release artifacts exist.

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 | `TestE2ESelfIssue` exists in `test/e2e/` (`go test -list`); under `E2E=1`, `make e2e` runs it green |
| 2 | `TestE2EExternalIssuer` + the `jwksstub` package exist; under `E2E=1` it runs green |
| 3 | `Dockerfile` contains `CGO_ENABLED=0`; under `E2E=1`, the built binary is *statically linked* (`file`) |
| 4 | `Dockerfile` declares a non-root `USER` and a `HEALTHCHECK` targeting `/readyz` |
| 5 | `docker-compose.e2e.yml` exists and references a fresh Postgres + the built image; under `E2E=1`, `up --build` reaches `/readyz` ready |
| 6 | `make preflight-full` is invocable (the CI job runs it; the smoke asserts the target + every `scripts/smoke/phase-*.sh` present) |
| 7 | `scripts/audit-cumulative.sh` exists and exits 0; `docs/audit/wave-7-cumulative.md` has no `- [ ]` open item |
| 8 | `README.md` contains the required section headings (why · the ask call · core ideas · where it fits · quickstart) |
| 9 | `CHANGELOG.md` contains a `## [0.1.0]` heading; the tag procedure text is present |
| 10 | `docs/ops.md` exists with the runbook section headings |
| 11 | the live-gate target/recipe exists (`.env.example` + the documented `LIVE=1` recipe); under `LIVE=1` it runs `-count=1` green (manual, never CI) |
| 12 | `Dockerfile` pins `BRUIN_VERSION=v0.11.666` + a sha256 check; under `E2E=1`, `docker run … bruin --version` reports the pin |
| 13 | `Dockerfile` bakes the Bruin telemetry-disable ENV; under `E2E=1` the in-container check confirms telemetry off |
| 14 | `TestE2EPipelineRun` exists; under `E2E=1` it runs green in-container (materialization + lineage + no-plaintext-secret sweep) |
| 15 | `TestE2EDegradedNoBruin` exists; under `E2E=1` it runs green (typed "pipeline execution unavailable", P4) |

## Glossary additions

- **Cumulative audit** — the terminal, read-only full-system pass over every shipped phase
  (Wave-7), verifying the P1–P7 invariants and hygiene end to end where per-phase gates saw
  only their slice; the Soundings lesson made standing (§17). Its punch list lands as one
  `chore(checkpoint)` in the release PR.
- **E2E stack / reference stack** — `docker-compose.e2e.yml`: the secret-free, fresh-Postgres,
  `mock`-gateway composition of the reference image used for the from-scratch smoke and the
  README quickstart.
- **JWKS stub** — the local `httptest` JWKS server (`test/e2e/jwksstub`) that mints
  Pengui-shaped tokens for the external-issuer E2E, so the external validation path is proven
  without a live Pengui.

## Decisions filed

**No new decision.** This phase relies on existing entries: **D-036** (Bruin as the DE
write-path executor — the pinned second binary, telemetry disable, SQL-only rule, the
credential-hygiene and no-Python audit items), **D-037** (the container as the deployment
unit, superseding D-005 — the glibc base + the `chartworks` CGo-free preference proof +
the bare-metal degraded mode), **D-006** (dual-mode / dual-audience auth — both E2E walks),
**D-009** (preflight/coverage/drift machinery — the release gates), **D-010** (the live gate
as the release blocker), **D-004 / D-024** (store vs. upload workspace — the E2E's upload
leg), **D-020 / D-021** (access + SQL-safety — the through-surface adversarial probes),
**D-025** (the leased queue — the profile job), **D-030** (admin bootstrap — the self-issue
walk), **D-031** (eval — re-run by the audit), **D-013** (charts deferred — the E2E asserts a
spec, not a render), **D-019** (mcp-go / Dockyard re-eval — an audit checklist item). A punch-
list finding that would *change* a settled decision files its own superseding entry (§15) —
never a silent edit here.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3): every reasonable deviation from this
     plan, why, and confirmation this file was updated in the same PR. -->

_None yet — authoring time._
