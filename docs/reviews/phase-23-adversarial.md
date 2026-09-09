# Phase 23 adversarial review and verification record

Date: 2026-09-09. Scope: SDK/CLI parity and the necessary cumulative phase-21
contract extension. This is a **same-author adversarial, test-driven review**, not
independent human approval or an external security audit. The implementation stays
on `feat/phase-23-sdk-cli-parity`; no merge, release or deployment is authorized.

## Recovery and evidence provenance

The actual merged base is `d6dbd31899449f6e042b4ab069c9c30c01fb844b` (PR #14).
The interrupted implementation was recovered at
`51758889692a4983cfe50387667da8a2b4193dcd`, not rewritten. Despite the prior chat's
incorrect closing message, that branch already contained substantial runtime and
acceptance work. Temporary source/dependency publication helpers are development
artifacts on separate branches, never part of the delivered implementation tree.

The recovered SDK/CLI unit race tests passed. Its real phase-23 acceptance failed
immediately because server configuration still rejected the non-root fixture
mount. Unit success was not substituted for end-to-end proof. A new real fixture
run after the correction passed phases 21, 22 and 23. The expanded run additionally
exercises nine client paths and the actual configured 100,000-row ceiling.

## Findings and corrections

| Finding | Correction and executable regression |
| --- | --- |
| SDK supported prefixes while server configuration still required root, so the owning acceptance could not start. | `internal/httpmount` is the one canonical grammar consumed by server config, OpenAPI and clients. `TestPhase23/AC01` runs on a real non-root mount; AC03 rejects unprefixed/lookalike/encoded aliases and proves zero dependency work. Root defaults and earlier APIs remain. |
| Required Idempotency-Key metadata was being treated as replay proof. A future header-bearing non-deduplicated mutation could be replayed. | `Definition.Replay` and `x-chartworks-replay` require explicit owner classification. Four existing ledger-backed operations classify keyed replay; other mutations default never. Header-only legacy documents cannot enable replay. Registry/SDK mutation tests and AC05 cover the distinction. |
| Go's default HTTP transport can independently rewind a keyed request body when `GetBody` is available, outside the explicit SDK attempt loop. | Mutation requests no longer expose a rewind callback. Regression observes the actual prepared request. SDK retry tests freeze key/body/path and reacquire caller provider authority; transport errors do not enter that loop. Safe-read/custom-transport behavior is documented, not claimed as an exact physical attempt count. |
| Shared model/input collection limits rejected otherwise valid 100,000-row results in generic SDK/MCP/CLI output paths. | Output-only decoding and schema validation use the existing 100,000-row ceiling. Inputs retain 65,536 items, and byte/depth/duplicate-key/finite-number restrictions remain. AC01 executes actual PostgreSQL rows through typed baseline and generic/MCP/CLI consumers, comparing exact ordered output. The shared wire assertion now uses the output mode only for responses. |
| Earlier in-process request contexts could accidentally carry previously verified authority or middleware markers. | The recovered transport strips all ambient values while retaining deadline/cancellation; only the provider sees the original context. AC03 supplies a valid prior envelope together with invalid/wrong-audience/new tokens and verifies current authority wins. Concurrent client tests exercise distinct per-call token providers. |
| Older typed SDK identifiers could represent dot segments even though generic calls rejected them. | The single credential transport validates the assembled literal route before asking for a token. Typed-source dot-segment and arbitrary/encoded route tests require zero provider calls. The actual router rejects encoded path aliases before mount dispatch. |
| CLI protocol/tool failures could be confused with successful HTTP status or incomplete output. | Recovered CLI checks JSON-RPC/tool outcomes, returns nonzero status for rejection, strips raw protocol diagnostics, preserves bounded owner receipts and detects short writes. Configuration reads no credential, and explicit effects still need `--execute`. |
| A cancellation fixture used a 40ms timeout that could expire during metadata lookup rather than stdin. Its subsequent open-pipe assertion hung under concurrent race load. | Cancellation now occurs deterministically at the actual input Read; closable input is closed and cleanup is joined. A separate deadline test remains. No production timeout, race setting or failure assertion was removed. A deferred response-body test cleanup also captures the original body rather than a later reassigned nil response. |

## Acceptance, real boundaries and negative cases

`TestPhase23/AC01` executes all eleven established discovery/question/BYO/feedback
contracts through raw HTTP, typed/generic HTTP SDK, typed/generic in-process SDK,
network/in-process MCP, CLI HTTP and CLI MCP. Outputs come from real PostgreSQL,
reviewed published semantics, pgvector, the native validator/executor and recorded
provider responses. Exact values, publication identity, query receipt/outcome and
feedback acceptance are checked. The output-ceiling test inserts 100,000 actual
rows and does not synthesize a prebuilt result to bypass execution.

AC02 joins every installed HTTP contract and actual MCP owner metadata; future
reporting, artifacts and identity-management calls fail absent. AC03 checks
current bearer/audience, cross-tenant/same-tenant wrong-context restrictions,
ambient-envelope isolation and canonical mount boundaries. AC04 exercises the
actual binary's injected command routing, exits, safe inputs and no server startup.
AC05 injects a lost response **after a real retained sweep commit**, then verifies
same-key replay does not apply a second effect; expired BYO lookup/submission never
reruns native execution. AC06 reads diagnostics without mutation and erases only
explicitly authorized tenant data with a real audit/receipt.

The full existing phase-21 and phase-22 acceptance is retained and rerun. No
unimplemented reporting endpoint was added to satisfy a test. Model fixtures are
recorded behavior, not live cloud/model quality or final migration qualification.

## Reproduction and qualification

Local continuation environment: Linux amd64, Go 1.26.4, real PostgreSQL 17.10 and
pgvector 0.8.2, pinned Bruin native parser source
`5f562c2959496a04d57f5f199f5e3ad22159fa9f`. Local source is recovered by immutable
Git bundle, with no production credentials or source data. The local parser
library is not a claim that the managed runner or all other warehouse engines
were executed locally. Full runner, MySQL/SQL Server, native platform/container
and all-package qualification are performed by the normal committed-source CI.

Verification commands:

```bash
make planning-check
make drift-audit
make check-mirror
go test -race -count=1 -timeout=5m ./sdk/chartworks ./internal/clientcli \
  ./internal/api ./internal/config ./internal/httpmount ./internal/gateway \
  ./internal/mcpserver ./internal/foundation
go test -race -count=1 -json -timeout=15m ./test/acceptance \
  -run '^TestPhase(21|22|23)$'
python3 scripts/run_phase_acceptance.py --phase 23
go test -race ./sdk/chartworks -run '^$' -fuzz '^FuzzOperationCatalog$' \
  -fuzztime=128x -timeout=3m -parallel=2
golangci-lint run
make coverage
make preflight-full
```

The Clients workflow strictly validates all eighteen named phase-21/22/23 pass
events and retains the exact source SHA, actual race coverage and acceptance JSON.
The normal full CI covers all implemented phases, every unchanged package band,
real integration/compiled smoke, native builds, containers and source hygiene.
CLI's 70% and shared mount's 80% bands are registered; existing bands are not
lowered. Package-only local coverage is not presented as full-repository coverage.

**Final-head execution results and CI links belong in the PR verification section.**
This document records review scope, corrections and reproduction rather than
claiming an earlier-head run proves a later commit. Readiness requires passing
implemented acceptance and final-source CI, not a phase status or this document.

## Explicit remaining product boundaries

No new issuer, token exchange, access-policy engine, business shadow store,
warehouse executor, standalone frontend or speculative endpoint is introduced.
In-process handlers and supplied providers must honor context; custom transports
are trusted dependencies. Streaming is not emulated, and disconnect does not prove
durable cancellation. Metadata is re-read rather than cached as authority. Reports,
retained artifacts, rendering/Apps delivery, migration and the phase-25 full-release
gate remain separately planned. Phase 23 remains in progress until reviewed/merged.
