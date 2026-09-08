# Phase 15 private draft stage evidence

Status: bounded implementation independently reviewed and verified at
`c7bf66a1fe4f4de78eabcd7dc8f98bd84ca61e77`.
Based on clean source-registry handoff `37f3a1c004f14ab78eebc3a1d67d053ef95209fc`.
No remote push, PR, merge, deployment or whole-phase acceptance is claimed.

## Implemented and checked boundary

The [draft service contract](../contracts/topic-drafts-v1.md) records seven actual
shared-registry HTTP operations and their Go SDK consumer: create/CAS edit, current
and exact-version read, history, diff, neutral export and mapped import. Migration
012 persists private immutable snapshots, dependency records and atomic draft audit.
Existing source/profile public services provide admission evidence. The metadata
transaction fences source revision, active private profile and digest before commit.

The focused real PostgreSQL fixtures cover:

- actual HTTP/SDK request/response schemas and manifest/OpenAPI parity;
- immutable exact revisions, unique authored versions, descending bounded history,
  deterministic diff and a neutral service import/export round trip;
- concurrent same-revision CAS with one winner and no silent retry;
- cross-tenant, different actor/session, wrong topic/source/dataset/context and
  missing action denials, plus independent export and creation requirements;
- source rotation and profile replacement specifically between admission and commit;
- audit failure rolling back the draft/head/dependencies, immutable SQL rows and
  erased profile evidence becoming unreadable;
- retained reads with source configuration unavailable and no credential lookups;
- multi-dataset admission across two registered sources, prior-dependency checks
  before an edit can remove references, and zero source lookups on that denial;
- compiler-generated diffs exceeding 2 MiB delivered through the concrete SDK
  operation with its separate 16 MiB cap;
- malformed/duplicate/oversized requests, semantic limits, canonical registry
  rejection and a zero-value admission proof denied by the store.

Focused validation uses Go 1.26.4/Linux, `-race`, `GOMAXPROCS=2`, `-p=2`, the pinned
5f Go modules and unchanged 026 native library, plus real isolated PostgreSQL 17
fixtures. The source/engineering fixture uses synthetic SQL data; no provider,
customer schema, production credentials or live cloud environment is involved.

## Frozen-source results

All Go files and `go.mod`/`go.sum` matched the final frozen fixture copy byte for
byte. The container was `chartworks-local-verification:go1.26.4-pg17` with
`CGO_ENABLED=1`, `CGO_LDFLAGS=-L/native-02638a8/release`, the existing native/module/
build cache volumes and the configured local PostgreSQL fixture network.

- Full focused unit run: semantics 2.015s, api 1.333s, topicapi 1.559s,
  store/postgres 1.128s, SDK 1.068s, foundation 2.556s under race. The two new
  packages are also instrumented by the real acceptance tests below.
- Final frozen registered HTTP/schema tests: topicapi 1.260s; final SDK tests,
  including the compiler-generated large diff and path rejection, 2.817s.
- Six actual topic-draft PostgreSQL fixture groups plus the existing source HTTP/SDK
  regression: 11.805s under race. No named phase acceptance parent is substituted.
- Combined focused statement coverage: `internal/semantics/drafts` 102/111
  (91.89%), `internal/topicapi` 89/100 (89.00%), and pure `internal/semantics`
  668/710 (94.08%). The added PostgreSQL file is 124/144 (86.11%); the new SDK
  file is 24/24. File coverage is not a whole-package/full-suite coverage claim.
- Focused `go vet`, planning (224 criteria, 63 features, 41 gates, 34 phases),
  mirrored contributor files and `git diff --check`: pass.

`coverage-bands.conf` only adds the two new packages at 80%. No existing band is
lowered; the existing 84.5% exception remains solely `internal/store/postgres`.

Exact Go commands (common environment above):

```sh
go test -race -count=1 -p=2 -coverpkg=./internal/semantics,./internal/semantics/drafts,./internal/topicapi,./internal/store/postgres,./sdk/chartworks -coverprofile=/evidence/units.cover ./internal/semantics/... ./internal/api ./internal/topicapi ./internal/store/postgres ./sdk/chartworks ./internal/foundation
go test -race -count=1 -p=2 -coverpkg=./internal/semantics,./internal/semantics/drafts,./internal/topicapi,./internal/store/postgres,./sdk/chartworks -coverprofile=/evidence/handoff.cover -run '^(TestTopic|TestRegistryManifestAndConcreteSchemas|TestBodyRejectsMalformedAndOversizedRequests|TestFailureClassifications|TestSourceAPIAndSDK)' ./internal/topicapi ./sdk/chartworks ./test/acceptance
go vet -p=2 ./internal/semantics/... ./internal/api ./internal/topicapi ./internal/store/postgres ./internal/foundation ./sdk/chartworks ./test/acceptance
make planning-check check-mirror
git diff --check
```

The full unit run preceded only the SDK diff-cap correction, keyed struct literals
and added focused tests;
the final frozen run covers those changes. Source-registry/full phase acceptance,
whole-suite coverage and independent review are not inferred from this evidence.

## Independent review and root verification closure

Two independent first-round reviews covered only
`37f3a1c004f14ab78eebc3a1d67d053ef95209fc..c7bf66a1fe4f4de78eabcd7dc8f98bd84ca61e77`:

- Astra high found no actionable P0/P1 or local P2. Its independent race checks
  passed for semantics (2.965s) and SDK `TestTopic|TestPortable` (6.074s), and its
  diff check passed. Topicapi dependency resolution was interrupted, so no
  topicapi test pass is attributed to that reviewer.
- Astra medium reported no actionable findings. Its SDK tests passed with
  `GOMAXPROCS=2` and `-p=2`, and its diff check passed. Topicapi dependencies were
  unavailable in that reviewer environment; that attempt is not a test pass.

No implementation findings required a fix or a second full review. The root
independently tested an exact frozen copy of
`c7bf66a1fe4f4de78eabcd7dc8f98bd84ca61e77` using Linux Go 1.26.4, race detection,
`GOMAXPROCS=2`, `-p=2`, the pinned native dependencies and real PostgreSQL fixtures:

```sh
go test -race -count=1 -p=2 ./internal/semantics/... ./internal/api ./internal/topicapi ./internal/store/postgres ./sdk/chartworks ./internal/foundation
go test -race -count=1 -p=2 -v ./test/acceptance -run '^(TestTopic|TestSourceAPIAndSDK)'
```

All focused unit packages passed: semantics 1.487s, api 1.330s, topicapi 1.596s,
store/postgres 1.111s, SDK 4.600s and foundation 1.880s. The draft service is
exercised by acceptance. All six topic PostgreSQL groups and the existing source
HTTP/SDK regression passed in 15.997s. This closes review and root verification of
this bounded slice only; it supplies no whole-phase acceptance or cloud CI claim.

## Current delivery disposition

This section supersedes only the old remaining-work description; the bounded
`c7bf66a` evidence above remains historical and exact. The current phase 15 candidate
now includes publication/facet activation, review/rollback/archive, canonical registry,
onboarding/entity and reviewed source-rebind APIs, durable public health/recheck,
bounded resumable generation and lifecycle portability through the shared HTTP/SDK
surface. `TestPhase15/AC01` through `AC06` exist with real assertions and recorded
PostgreSQL/pgvector/Bifrost-fixture evidence.

Independent review A was clear at `e90ac24e173a8e6750d84b632b89a70f03b8fa33`.
Review B found one reachable rebind defect: enhanced drafts retained unresolved
references to the old dataset. Author fix
`8291e84cf5bcb04c16ecbbae6cbf3d8e561d9d43`, integrated as
`39bc68c753ff65e1d382102dcd6bcfdd0ea8387b`, rewrites that dataset coordinate while
preserving the stable unresolved ID, column and reason. Its pure regression, real
HTTP/SDK AC03 path and full six-child Phase 15 run passed. Its required narrow
independent follow-up review cleared the fix; the later unresolved-rebind correction
`6884f23126dbf01e45c2d799e8f10dfee03c2955` is integrated and root's semantic checks
passed.

The first exact `39bc68c` full-chain run then exposed a separate migration interaction:
migration 017's replacement audit constraint omitted the `topic.health_rechecked`
action introduced by migration 016, so a valid source-drift observation rolled back at
its audit insert and the HTTP operation returned 400. Forward migration 019 at
`d103ba95af8a4f951b0a4d2589292ec269905a67` preserves the complete closed action union,
restores the health action and continues to reject an unknown action. Author strict
Phase 02 and Phase 15 runs passed all six children with zero skips, and the repair
integrated as `dd6f79e`. Root verified its SHA-256-checked committed-source archive:
strict Phase 02 and Phase 15 each passed all six children with zero skips, including
AC04 health and enhanced rebind, and native race `TestSafeErrors` passed.

Phase 15 therefore remains `in_progress`. Remaining delivery gates are exact final
integrated-head coverage, full lint/preflight, hosted CI and release integration. The owner-approved 84.5%
exception remains limited to
`internal/store/postgres`. Recorded model fixtures are not a live semantic-quality
measurement, and no such live result is claimed.
