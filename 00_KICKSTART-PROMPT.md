# Chartworks development kickoff

Read [RFC-001](RFC-001-Chartworks.md), [RFC-002](RFC-002-Governed-Reporting.md), [the master plan](docs/plans/README.md), [COMMON.md](docs/plans/COMMON.md), and the owning numbered phase before implementation. The repository is planned, not a shipped analytics service. Archived plans are historical references, not competing instructions.

## Binding boundaries

Pengui decides and signs identity/access authority. Chartworks validates and enforces its JWT/scopes; do not implement local users, roles/grants, API keys, issuer, login/OAuth, bootstrap administration or embed credentials. Read [the authority contract](docs/contracts/pengui-authority.md). Wire new scope and scheduled-authority requirements through actual Pengui APIs; do not invent or assume a deployed endpoint.

Harbor/Pengui MCP Apps support is established. Implement Chartworks tools/resources/viewer and functional tests for that code, without a host-compatibility project or framework/protocol detour. Iframe credentials remain behind Pengui/client BFF.

Production inference is **Bifrost Go SDK only, remote models only**, for completion, structured generation, embeddings and reranking. Read [the gateway contract](docs/contracts/model-gateway.md). Reuse non-secret sibling gateway patterns; do not copy credentials, local inference, direct-compatible alternate drivers or sibling auth/storage systems. Fixtures are test-only. Deterministic tokenization, SQL parsing, pgvector and rendering are not learned inference.

## Build the product vertically

Start the 01/02/03/04 foundations and05 gateway, bringing21/22/23 thin transport/client shells up early. Build read safety09->08->10, semantic lifecycle and output specs15/20, then27/28 governed blocks and retained results. Build NLQ and reporting composition on the documented dependencies. Viewer31 does not wait for scheduling30; both consume the same report/artifact contracts. Add32 static rendering/BFF delivery,33 guided onboarding,34 migration and25 final release.

Preserve primary source behavior through neutral synthetic regression tests. Compact context, templates, clarification/refinement, rules/replay, learning, source health, dynamic widgets and scheduling are required mapped work, not optional research notes. Remove unsupported source stubs rather than exposing nonfunctional operations. Do not import confidential source code, identifiers, prompts, schemas or data.

Frozen block refresh executes the approved definition with typed parameters and never re-enters NLQ or chart selection. Optional narratives consume bounded evidence only. Viewing/rerendering retained results makes zero model/warehouse calls. Publication, certification, current health and signed access are separate. Private previews, exact numeric values, revision/window pins and source-context partitions remain structural invariants.

## Completion evidence

Every criterion owns a real `TestPhaseNN/ACxx` assertion, not a no-op wrapper. New operations register schemas, signed authority requirements, side effects and audit, then expose the thin required surfaces. Use real storage/source boundaries and the real Bifrost SDK over recorded provider fixtures; live accuracy and deployment support have separate evidence.

Run `make planning-check`, the owning phase's smoke, and `make preflight-full`. Planned skips are explicitly unimplemented, not successes. `make release-check` disallows all skips and requires reviewed shipped state plus actual named test results. Keep RFCs, phase metadata, registry, coverage map, examples and mirrored contributor rules coherent. A dependency graph or row count is not functional proof.

Return changed contracts/migrations, commands and actual results, applicable source/model versions, operational limitations and unresolved required parity IDs. Open a reviewed branch PR; do not merge/tag/deploy automatically. The first reference-adapter demonstration is not complete migration parity.
