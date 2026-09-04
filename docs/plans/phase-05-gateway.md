# Phase 05 — gateway

Status: planned. Owner: internal/gateway. Hard dependencies: 01.

## Authority and design

RFC-001 §13, D-049 and [COMMON.md](COMMON.md) apply. One provider-neutral gateway serves every role; reporting adds consumers, not another provider client.

## Brief findings incorporated

Briefs 03, 14: lean structured context, role-specific configuration, observable metering, independent provider choices and explicit fallback.

## Findings I'm departing from

Do not restore a mono-provider configuration or make healthy frozen/artifact operations depend on unrelated model availability. No free-text JSON guessing or unbounded retries.

## Scope and implementation tasks

1. Implement the existing provider-neutral gateway with Bifrost and deterministic/recorded fixture drivers; independent provider/model settings per role.
2. Support embedding, semantics, SQL generation/correction, clarification, pipeline draft, profile summary, optional rerank, narrative and exploratory visual-rank roles.
3. Reserve/account model budgets before calls, count retries and propagate cancellation; validate structured output and enforce role-specific capabilities.

## Non-goals

No direct provider SDK use elsewhere, local model download service, new billing/entitlement system or narrative query tools.

## Config and persistence

`gateway.roles.<role>.{provider,model,base_url,credential_ref,timeout,max_tokens}`; explicit optional-role enablement and operation/tenant call/token ceilings. Usage records and prompt/model versions are data, not authority. Required credentials use the established secret seam.

## Acceptance criteria

1. **AC01** — Provider SDK/HTTP use outside the gateway is structurally rejected; role routing and credential references are independent and tested.
2. **AC02** — Structured output follows its schema or fails; no guessed free-text JSON parsing or unvalidated partial output is accepted.
3. **AC03** — Call/token/time reservations bound concurrent and retried work; measured usage and unknown cost remain truthful.
4. **AC04** — Embedding identity/dimensions mismatch cannot serve mixed facets; frozen/no-narrative execution works with model services unavailable.
5. **AC05** — Optional reranking only reorders permitted candidates; fallback is explicit. Narrative/visual ranking cannot acquire query/write tools.
6. **AC06** — All roles have recorded-wire fixtures, cancellation/timeout/error tests and race-safe reuse; per-role configuration is documented.

## Tests, coverage and smoke

Implement `TestPhase05/AC01` through `TestPhase05/AC06`, real gateway driver with recorded provider responses plus the explicit test adapter. Test reservation races and exhausted retry budgets, not post-hoc accounting alone. COMMON.md sets coverage; `scripts/smoke/phase-05.sh` requires all six results.

## Glossary, decisions and deviations

D-049 preserves per-role provider/rerank work without claiming the old separate branch was merged. Add new role vocabulary in the same implementation change. No runtime completion is claimed.
