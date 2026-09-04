# Phase 22 — mcp-server

Status: planned. Owner: internal/mcpserver. Hard dependencies: 21.

## Authority and design

RFC-001 §11, D-046/D-050 and [COMMON.md](COMMON.md) apply. Harbor/Pengui MCP Apps compatibility is established. Implement the Chartworks server against the supported ecosystem profile without framework-selection or host-qualification work.

## Brief findings incorporated

Briefs 08, 09, 13, 14: one tool registration/middleware path, typed contracts, accurate annotations and clean failure boundaries. Historical dependency notes do not reopen the owner-confirmed host capability.

## Findings I'm departing from

Remove Dockyard/framework re-evaluation, host interoperability qualification and any unrelated mandated protocol migration. Test Chartworks' new behavior instead.

## Scope and implementation tasks

1. Build MCP registration/middleware over the established ecosystem integration; retain discovery/question/BYO/feedback operations as their domain implementations land.
2. Require scope/resource checks, safe typed errors and accurate side-effect annotations in one registry; add no framework-selection checkpoint.
3. Expose resources through the same core; phase 31 supplies the reporting Apps resource and viewer.

The established core operations are list/describe topics and datasets; preflight/plan/run/refine question; get context/submit SQL; submit feedback. Register each only when its domain owner ships it, and enforce cumulative coverage in phase 25.

## Non-goals

No host compatibility project, new credential channel, local issuer, separate business core or successful tool stubs.

## Config and persistence

MCP listen/shared-port mount, request/size limits and enabled real tool groups; use the deployment-supported protocol/library profile. Transport session state is not analytical run state or access authority. No extra domain tables.

## Acceptance criteria

1. **AC01** — Every registered tool/resource uses the shared verified envelope and policy enforcement with no per-tool bypass.
2. **AC02** — The established eleven core operation contracts bind to real services as their owner phases land; unbuilt operations are absent, never success-returning placeholders.
3. **AC03** — Annotations distinguish persisted/paid operations from pure reads; unknown annotations/registration omissions fail tests.
4. **AC04** — MCP errors/panic boundaries disclose no stack/secret/foreign metadata; intended audience is enforced.
5. **AC05** — Transport and in-process clients preserve per-request identity/context without shared-token cross-talk; no separate local credential channel is introduced.
6. **AC06** — Ordinary tool/resource functional tests cover Chartworks behavior; no test requalifies Harbor/Pengui Apps compatibility or requires an unrelated protocol upgrade.

## Tests, coverage and smoke

Implement `TestPhase22/AC01` through `TestPhase22/AC06`; every domain owner reruns registration/call negatives when adding its real tools. Actual core-operation coverage is required at full release, without making shell delivery depend on later features. COMMON.md sets coverage; `scripts/smoke/phase-22.sh` requires all six results.

## Glossary, decisions and deviations

D-046 establishes host support; D-050 permits early shell delivery. No runtime completion is claimed.
