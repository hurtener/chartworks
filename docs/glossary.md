# Chartworks glossary

Current implementation vocabulary, 2026-09-04. RFCs and active plans govern; historical terminology in archived plans is not an alternative contract.

| Term | Meaning |
|---|---|
| Pengui authority | Identity, action and resource permissions signed by the sole issuer. Chartworks enforces them, not local memberships/roles/grants. |
| Verified envelope | Immutable validated identity/scope/context data supplied to all protected core operations. |
| Scope | Signed permission string for an operation or addressed resource, never a client-provided role hint. |
| Execution context | Registered source credential/warehouse-role/secure-view context actually used to access data. |
| Data partition | The real access context and version of a result. A report label cannot narrow broad source data retroactively. |
| Execution binding | Opaque Pengui-authorized reference for obtaining fresh authority for durable work. Not a retained bearer or local service account. |
| Source | Registered customer database/warehouse or upload workspace reached through an adapter. |
| Dataset | Registered source table/view or managed materialization with schema, lineage and health. |
| Managed object | An output whose ownership is verified in the managed registry and database privileges; a name prefix is not proof. |
| Profile | Versioned sampled data/schema/quality/freshness evidence with its method and observation time. |
| Topic / topic pack | Versioned business semantic contract containing tables, measures, dimensions, KPIs, joins, rules and routing context. |
| Canonical entity | Tenant-wide stable business ID with immutable sequential meaning revisions. Names and aliases are global meaning; physical keys remain in each reviewed topic. |
| Capability card | Compact published semantic projection used by routing/context, distinct from full authoring metadata. |
| Facet | Typed semantic retrieval unit scoped to tenant/topic/version/embedding generation. |
| Embedding space | Provider/model revision, dimensions and preprocessing/input/normalization contract; same dimensions do not imply compatibility. |
| Bifrost SDK | In-process Go client for remote model providers. Embedding the SDK does not mean executing models locally. |
| Remote inference | Completion, structured generation, embedding or rerank produced by configured external providers through Bifrost. |
| Deterministic local computation | Tokenization, rules, SQL parsing, pgvector search and rendering; not a local learned model. |
| Rerank | Relevance ordering of an already authorized candidate set, with validated IDs/scores and no authority expansion. |
| Context assembler | Sole owner of query-time token budgeting, pins, mandatory constraints, examples and provenance. |
| BYO context reference | Stored bounded context handle, reauthorized using Pengui JWTs; not a locally signed capability. |
| Validated plan | Nonzero validator-issued source/dialect/context/semantic/parameter-bound executable plan. Parsing alone is not a safety proof. |
| Block | Reusable analytical definition with typed parameters, exact dependencies and saved outputs. |
| Revision | Versioned definition; published payloads are immutable and amendments start new drafts. |
| Validation evidence | Proof that the exact definition/dependencies underwent the declared validation against observed schema/results. |
| Publication | Explicit eligibility transition for an exact revision. It grants neither data access nor certification. |
| Certification / attestation | Review approval of an exact revision/evidence set, distinct from current health or authorization. |
| Health | Current dependency/schema/source status; not the same as historical approval. |
| Frozen execution | Approved SQL and outputs refreshed without NLQ interpretation, generation/correction or chart reselection. |
| Narrative | Optional bounded evidence-based post-query model output, with no query/write tools. |
| Report | Versioned composition of block, explicitly dynamic-query and safe text widgets. |
| Dynamic widget | Explicit query execution with its own provenance and policy; report publication does not certify changing SQL. |
| Query durability | Replayable question versus a reference requiring the originating authorized session. |
| Dashboard | Versioned ordered collection of exact report-revision pages, not another execution engine. |
| Run / logical operation | Accepted request with a sealed revision/parameter/window/context manifest and one or more attempts. |
| Attempt / fence | Leased execution and stale-owner commit protection. Neither guarantees physical exactly-once remote work. |
| Artifact | Retained immutable result/evidence until retention removes payloads; opening it does not run SQL or models. |
| Rendition | Rendered representation of an artifact under a recorded renderer/theme/viewport version, inheriting privacy/expiry. |
| Preview | Private draft/review execution; remains private after later report publication. |
| Idempotency | Same accepted key/request resolves to the same logical operation, not a universal remote exactly-once promise. |
| Occurrence | Schedule due time with exact target revisions and half-open period preserved across retries. |
| Catalog delivery | Authorized pull access to a retained result; recipients in metadata do not prove outbound email. |
| SSR | Actual server-generated visual content, not an HTML shell requiring client chart JavaScript. |
| BFF | Pengui/client backend that authenticates browser requests and forwards scoped authority server-side. |
| MCP App | Chartworks's read viewer/resources over the established Harbor/Pengui host bridge; no host qualification project. |
| L2 proposal | Reviewed, evidenced managed-engineering changes with staged external effects and compensation. |
| Planned / shipped | Work specification versus runtime acceptance plus reviewed evidence. A planning SKIP never means shipped. |

Business data and diagnostic metadata may use ordinary technical names such as artifact, MIME and index where appropriate. Protocol-standard field names are not renamed to satisfy an overbroad lexical check. Hygiene checks are not authorization controls.

## Output specification vocabulary (phase 20)

A **saved output mapping** is a versioned kind, closed options, slot bindings, order and exact column-metadata pins. A **rebind proposal** is a detached `review_required` mapping, never an edit to a published output. A **complete-result total** covers all rows provided to the specification, not all rows in a warehouse; a **returned-rows total** explicitly marks truncated input. Exact labels are separate from approximate plotting coordinates. See the [version-one contract](contracts/chart-specifications-v1.md).

## Client operation matrix

Detached registration metadata linking an installed HTTP operation to its real
SDK/CLI dispatch and, when queried under separate authority, MCP binding. It is
not a permission grant or another authoritative business store. Replay eligibility
is an explicit owner contract (`read`, `never`, `keyed`), not an inference from a
header. See [clients v1](contracts/clients-v1.md).
