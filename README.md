# Chartworks

**Governed analytics and publishing for Pengui.** Go services for semantic data access, NLQ/BYO SQL, reusable approved reporting blocks, reports/dashboards, scheduled runs and portable retained results.

Status: the Go foundation verified operational authority, remote Bifrost gateway and durable maintenance scheduling (phases 01–06) are implemented with real PostgreSQL acceptance tests. Analytics/reporting implementation and full source migration remain in the subsequent phases.

## Start here

Read [the actionable phase plan](docs/plans/README.md), [RFC-001](RFC-001-Chartworks.md), [RFC-002 reporting](RFC-002-Governed-Reporting.md), and [the contributor rules](AGENTS.md). The [source parity audit](docs/research/14-reporting-parity-audit.md) distinguishes inspected code, documentation, inventory and stubs.

Pengui is the sole issuer/authentication/access-policy owner; Chartworks validates its JWTs and applies signed scopes. Harbor/Pengui MCP Apps support is established. Chartworks owns analytics, SQL safety, reporting lifecycle, artifact retention, scheduling and its read viewer—not a second IAM platform or standalone builder UI.

**Production inference uses the embedded Bifrost Go SDK and remote providers only.** Embeddings and reranking follow reviewed Soundings/Stowage patterns. No local models/weights/downloads or alternate direct-compatible production client. See [gateway contract and configuration](docs/contracts/model-gateway.md) and [the example excerpt](examples/chartworks.gateway.json).

## Planning checks

```bash
make planning-check   # document/graph/criteria/coverage coherence plus tool tests
make preflight-full   # implemented phase tests; planned phases are explicit SKIPs
make release-check    # strict: actual tests, all phases shipped, no SKIPs
```

There are 34 phase plans, 224 acceptance criteria, 63 source-feature rows and 41 review gates. Those counts and a green planning check are not runtime proof. Historical plans remain under `docs/archive/`. Development follows the graph rather than numeric phase order; phases21–23 are early thin surfaces and phase25 is the final release gate.

## Phase 01–02 foundation

The first Go foundation now has strict configuration, lifecycle/health, PostgreSQL metadata migrations and real-store acceptance tests. JWT verification and signed-scope enforcement now protect the operational consumer; analytics and reporting remain later phases. Start with [GETTING-STARTED.md](GETTING-STARTED.md); the [adversarial review](docs/reviews/phase-01-02-adversarial.md) records failure probes and corrections.

## Verified operational access (phases 03/04)

The production `serve` command now protects retention policy, audit, synchronous retention sweep, diagnostics and metrics with Pengui JWTs and signed addressed scopes. The old health-only foundation boundary is superseded for these implemented operations, not for the later analytical/MCP features. The listener remains explicit-loopback; a trusted backend supplies credentials. See [operator registration](docs/contracts/pengui-provider-registration.md), [operation manifest](docs/contracts/chartworks-operations.json) and [authority contract](docs/contracts/pengui-authority.md). The public Go client is `sdk/chartworks`; its caller supplies a current Pengui token provider. Chartworks issues no credentials.

## Remote gateway and durable work (phases 05/06)

The embedded Bifrost v1.6.2 adapter implements all ten configured roles, strict JSON/indexed response validation, independent remote routing, conservative budgets and isolated embedding caches. The first HTTP consumer is a fixed synthetic operator probe, not an arbitrary-prompt endpoint.

One PostgreSQL operation ledger now owns queued maintenance, attempts, fencing, bounded cron/interval/manual occurrences, retries and cancellation. Every privileged attempt obtains fresh **Pengui-issued** authority bound to its accepted manifest. This requires the [companion Pengui endpoint](docs/contracts/execution-authority-v1.md); Chartworks has no local credential renewal or signing path. Retained metadata remains available with model inference and job dispatch disabled.

See [setup](GETTING-STARTED.md) and the [adversarial review and verification record](docs/reviews/phase-05-06-adversarial.md). The implementation has recorded-wire tests, not paid live-provider or production deployment acceptance. Reporting targets and the full MCP surface are still later workstreams.
