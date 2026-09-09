# Phase 22 — MCP implementation and adversarial review

Status: implementation and adversarial fixes in PR #14; final exact-source CI is
required before readiness and merge. This record is a self-review with executable
regressions, not an independent third-party audit or a production qualification.

## Scope and preserved boundaries

The base is merged PR #13, `2fa80a404518e59db5d6157b50a7521aa9c1512f`.
Phase 22 adds the bounded, stateless Streamable HTTP mount, eighteen concrete
service bindings, three pure metadata resource bindings, and per-call in-process
and HTTP clients. All eleven established discovery/question/BYO/feedback
contracts have actual domain owners and bindings. Disabled groups and absent
services contribute no successful placeholders. Reporting Apps, artifact viewing,
exports and the full CLI/SDK phase remain with their later owners.

The [MCP contract](../contracts/mcp-v1.md) documents actual names, schemas,
authority, effect classifications, deployment settings, output and failure
semantics. The existing ecosystem SDK is pinned; no host-compatibility or protocol
migration gate has been introduced.

Phase 21 extends its actual immutable registry with list topics, list datasets,
describe dataset and the MCP transport mount. The first three also have typed
public Go SDK methods. Their services and the existing services, not transport
handlers, own business validation, authorization, persistence and execution.
Pengui remains the only issuer and policy owner.

## Findings and remedies

| Finding / attack | Remedy | Executable evidence |
|---|---|---|
| A transport session or initialized client could become reusable authority. | Stateless JSON responses; fresh MCP-audience verification and `mcp.use` on every request; no session, URL, cookie or locally issued credential fallback. In-process calls obtain and verify a fresh caller-provided bearer too. | MCP HTTP and shared-client tests; `TestPhase22/AC01`, `AC04`, `AC05`. |
| The SDK detaches stream context; a disconnected caller or expired bearer could leave domain work running. | Reattach the exact admitted request context, combine SDK cancellation, and bound the call by caller deadline, configured deadline and verified authority expiry. Admission happens before body reading and has a non-queued concurrency bound. | Cancellation, expiry, capacity and concurrent-request regressions in `internal/mcpserver`. |
| Topic or dataset pagination before resource filtering could leak inaccessible identifiers or starve an authorized page. | Tenant, signed resource reach and actual source/context/publication dependencies are predicates of the bounded store query before `LIMIT`. Topic discovery includes all published dependencies; draft and archived content is not substituted. | Actual PostgreSQL restricted-pagination and foreign-tenant checks in `TestPhase22/AC06`, plus domain service checks. |
| NLQ preflight appeared pure although it performs routing inference and commits a receipt. | Correct the owning HTTP registry and checked manifest to `nlq_routing_and_preflight_commit`; derive MCP annotations from the same effect contract. | `TestPhase22/AC03` and the NLQ registration manifest test. |
| Ambiguous resource templates could make dispatch depend on registration order. | Reject intersecting template/literal patterns during immutable registry construction, including patterns with different variable names. | `TestRegistrationAndResourceAmbiguity` and resource boundary regressions. |
| A hostile Origin on an unsupported HTTP method could avoid the intended origin boundary. | Exact configured Origin and Host checks precede the method response. Forwarding headers cannot replace the admitted Host. Origin is denied by default when present. | Host, Origin, duplicate-header and unsupported-method cases in MCP HTTP tests. |
| Accept declared a byte bound that was not enforced; empty/duplicate encoding headers were ambiguous. | Enforce the aggregate 1,024-byte Accept bound before parsing and reject any Content-Encoding header occurrence. Only the documented protocol headers are forwarded to the SDK. | Oversized/aggregate Accept and empty/duplicate encoding cases in `internal/mcpserver/adversarial_test.go`. |
| SDK parse errors, raw domain errors or panics could disclose request content, stack traces or foreign metadata. | Fixed public protocol errors, discarded SDK diagnostic logger, panic boundaries, closed input and output schemas, bounded result/receipt projection, and no raw error interpolation. | Malformed JSON/schema/resource/unknown-method/panic tests, `TestPhase22/AC04`, and `FuzzMCPBoundaries`. |
| A paid/persisted call interrupted after entry could be represented as a guaranteed rollback or silently retried. | Preserve truthful unknown/cancelled outcomes and safe receipts. No transport automatic retry. BYO replay returns its content-free receipt without repeating SQL or reconstructing lost values. | `TestPhase22/AC02`; MCP fault/receipt and SDK no-retry tests. |
| Shared clients could retain another caller's token or envelope. | No mutable client bearer; request-local TokenProvider, immutable verified envelope, per-request SDK context, resource reauthorization and safe bounded admission. | Race-enabled concurrent callers across tenants and contexts in `TestPhase22/AC05`. |
| The new concrete discovery routes left older static manifests and route counts stale. | Update source, topic and NLQ checked manifests and add valid schema samples for both new dataset operations. Preserve exhaustive extra-field rejection and metadata parity tests. | `internal/sourceapi`, `internal/topicapi` and `internal/nlqapi` registration tests; cumulative `TestPhase21`. |

The next exact-source acceptance run identified a concrete resource defect:
source context/dataset IDs may contain colons, but simple URI-template expansion
did not match those valid canonical resource URIs in the SDK. Templates now use
RFC 6570 reserved expansion while retaining exact identifier/component checks and
rejection of percent aliases, extra paths, credentials and traversal. Network and
in-process regressions cover colon-bearing IDs as well as the real source
resource in AC06. The same run found two stale test expectations: the capability
phase label still expected phase 21, and malformed-request tests expected an HTTP
status even when the SDK rejected JSON before sending it. The latter tests now
exercise the actual HTTP handler directly, rather than weakening the assertion to
accept any client-side error. These changes require a new qualifying run.

No finding is closed merely by documentation or by an annotation string. Domain
resource checks still execute after mount-level audience and action verification.
Discovery metadata is not authority to execute SQL or read an artifact. Listing a
tool does not certify a caller's eventual arguments or all their dependencies.

## Named acceptance and real consumers

| Criterion | Implemented assertions |
|---|---|
| AC01 | Full registered-action denial inventory, MCP audience, `mcp.use`, wrong tenant/context/session and actual service resource checks; rejected work does not invoke a provider or warehouse. |
| AC02 | All eleven core contracts exercised using real semantic publications, PostgreSQL/native SQL validation and read execution, recorded Bifrost responses, refinement, feedback and BYO receipt/replay behavior. |
| AC03 | Eighteen concrete tool names, shared action/schema/audit/effect mappings, paid preflight, accurate annotations and absence of missing/disabled service placeholders. |
| AC04 | Safe typed protocol and tool failures; invalid envelope and arguments, wrong audience, resource traversal and foreign metadata negatives. Unit regressions separately force panics and size/encoding failures. |
| AC05 | A shared in-process client called concurrently with independent current tokens and identities; HTTP requests and resource reads preserve the exact verified request context. |
| AC06 | Real source/topic/dataset discovery, all five chart operations and three metadata resources; restricted SQL pagination; HTTP/public Go SDK discovery parity. No test requalifies Harbor/Pengui Apps support. |

`TestPhase21/AC01` through `AC06` also enumerate the added registrations and
select the declared transport audience when exercising denial ordering. The MCP
mount has typed bounded JSON-RPC schemas in the generated OpenAPI registry.

## Verification record

Local focused checks use the pinned Go 1.26.4 compiler and race detector. The MCP,
configuration, shared API and real chart-binding unit packages passed. After the
header findings above were fixed, isolated MCP package statement coverage was
**87.4%**, above its unchanged 80% band. This isolated figure is not the full-suite
cross-package coverage report. The bounded MCP protocol fuzz campaign is run
again on the final source and by the dedicated read-only workflow.

The first PR run exposed an unused acceptance-test import before the acceptance
package could compile. That import was removed; this initial run is not qualifying
acceptance evidence. Stale domain registration fixtures were corrected in the
same follow-up. Their updated real database assertions still require the final
exact-source run, rather than being credited from unit-only compilation.

Final closure requires all of the following against committed source:

- The dedicated `MCP` workflow: package race tests, all twelve named phase-21/22
  children without missing/skipped results, protocol fuzzing and source hygiene.
- Existing full `CI`: pinned native dependencies and Linux/macOS shipping builds,
  the reference container, vet, lint, the complete race-enabled coverage bands,
  implemented-phase acceptance, executable smoke and cumulative preflight.
- Planning/manifest/mirror checks, a clean delivered tree, and no temporary source
  transfer files or write-enabled development workflows in the PR's final diff.

CI and acceptance artifacts, not this status paragraph, establish the final
result. The PR records the exact qualifying commit and runs before merge.
Recorded remote-provider responses establish deterministic integration behavior;
they do not assert live provider quality, production cutover, host qualification
or completion of the remaining twelve workstreams.
