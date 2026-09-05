# Gateway and completion decisions

Date: 2026-09-04. These entries append to the existing decision log and preserve its history.

### D-053 — Bifrost SDK is the only production inference path

Accepted owner directive: Chartworks uses the embedded Bifrost Go SDK for all remote completion, structured generation, embeddings and reranking. There are no local learned models, weight downloads, model-serving processes or direct-compatible alternate production driver. The initial reference pin is Soundings' `github.com/maximhq/bifrost/core v1.6.2`; the actual phase-05 implementation compiles/tests that pin. Reuse non-secret sibling provider/role configuration, not private credentials, auth modes or local storage. Test fixtures remain explicit test-only dependencies. This narrows D-003/D-049 wherever a generic gateway seam could be read as permitting a second production inference path.

The normative details are `docs/contracts/model-gateway.md`; `model-gateway-policy.json` and the example excerpt are mechanically checked. Phase 05 gains four acceptance criteria; G41 traces this new owner requirement through gateway, facets, context and zero-call reporting. Provider failure never silently invokes a local model or mixes embedding spaces.

### D-054 — Finish the actionable baseline and separate visual delivery from schedules

Recover the previously unattached phase-plan commit and complete its missing smoke/checker, active entrypoint/glossary and contributor-rule reconciliation. The master has 34 phases and 224 acceptance criteria after D-053; all remain planned until actual implementation evidence exists. Preserve all 63 source-feature rows and the first 40 corrected gates, adding G41. Planning checks prove document/registry coherence only. Named runtime tests must execute and pass; missing, skipped or empty parent tests cannot close a phase. Release mode never honors a planned-skip flag.

Phase 31 depends on the report/artifact and transport contracts, not phase 30 scheduling: viewing/reporting must not wait for unattended delivery. Scheduled runs appear through the same result catalog when phase 30 lands. No MCP host-compatibility qualification work is reintroduced. Historical source/research files remain reference evidence, not current authentication instructions.
