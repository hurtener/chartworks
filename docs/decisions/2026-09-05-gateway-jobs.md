# Phase 05–06 implementation decisions — 2026-09-05

### D-062 — Actual pinned SDK capability overrides historical route examples

All inference uses Bifrost core v1.6.2. OpenRouter's rerank method is unsupported
in this pin, so the implemented reference uses native Cohere `rerank-4-fast` with
its own environment-indirected credential. This corrects the route inference from
the historical Soundings example, not the remote-only or one-SDK decision.
Strict raw SDK observation checks reject missing/null indices/scores before typed
zero defaults; raw content is never projected or logged. Scoped caches include
signed reach, and observed budget overages cannot fund more calls. No alternate
provider HTTP client, model-download step or live-support claim is added.

### D-063 — Deliver the actual Pengui-owned execution extension with bounded maintenance

The existing broker token path did not supply durable manifest-bound service
execution. Implement the additive Pengui `/exchange/execution-authority` v1
endpoint with its existing vault and asymmetric minter, not a guessed URL or
Chartworks issuer. An operator binding file lives only in Pengui; every pull
rechecks its exact broker/runtime/capability and bounds a 30s execution token to
the accepted job/manifest and binding expiry. Chartworks persists opaque binding
and attribution but no bearer. The sole initial handler is bounded retention in
the same PostgreSQL transaction as attempt completion and audit.

One existing operation ledger is extended by migration003; no historical
migration is rewritten. Cron/interval/manual triggers have explicit overlap and
missed policies; cursor continuity survives pause/restart. Metadata reads,
cancellation and pause remain available with dispatch/models disabled. Unbuilt
external target handlers are rejected rather than given empty success or a false
exactly-once claim. Review and tests are recorded separately from production
provider/deployment acceptance.
