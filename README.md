# Chartworks

**Governed analytics and publishing for Pengui.** Go services for semantic data access, NLQ/BYO SQL, reusable approved reporting blocks, reports/dashboards, scheduled runs and portable retained results.

Status: implementation planning. This repository does not yet claim a working Go service or completed source migration.

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
