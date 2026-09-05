# Configuration examples

`chartworks.gateway.json` is the remote-inference configuration excerpt for the phase-01/05 decoder. It is not a complete runnable server configuration or proof of live provider access: Pengui verification, PostgreSQL and source configuration are still required. All named model settings are non-secret references observed in sibling configuration; provider availability and actual account access are checked when deploying.

Production uses the embedded Bifrost Go SDK, not local models and not a required separately deployed Bifrost proxy. The example follows Soundings' nested provider/role pattern; Stowage supplies additional guidance for independent embedding/rerank routing and batching. Do not import either application's auth/bootstrap or store settings.

Set `CHARTWORKS_OPENROUTER_API_KEY` through the deployment secret manager. No real key belongs in this file. The 1024-dimensional embedding identity is pinned per facet generation; changing it requires reindexing. The other roles may use independently configured remote providers. All rerank failures under `preserve_candidates` must be visible; narrative and optional visual ranking are disabled in the reference excerpt until explicitly enabled.

The configuration is JSON so planning checks can validate its structure without installing another parser; JSON is valid YAML syntax. Phase 01/05 must verify that the actual typed server config consumes it, rejects unknown keys and reports missing required values without exposing secrets. See [the complete gateway contract](../docs/contracts/model-gateway.md).
