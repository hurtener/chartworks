# Governed reporting blocks: HTTP and Go SDK v1

Phase 27 implements authoring and private validation, not recurring execution or
retained report artifacts. RFC-002 and the Pengui authority contract remain the
owners of publication, certification, privacy and source execution semantics.

## One core, independent trust facts

`internal/reporting.Service` is shared by HTTP and in-process consumers. Migration
022 adds tenant-composite block heads/revisions, resource and topic/source pins,
validation receipts, publications, attestations, withdrawals, health and events.
SQL belongs to the protected definition, never ordinary metadata or audit payloads.

A new or captured definition is an **unvalidated private draft**. Validation makes
an explicit, bounded warehouse read using the existing native validator's opaque
execution plan. It binds exact SQL/parameters, immutable definition and execution
hashes, semantic/source dependencies, result schema, actor/time and the durable
read-attempt receipt. Client-supplied success claims are not validation evidence.

Publication uses the expected head version and a fresh exact evidence identifier.
Certification is a separate action on a published immutable revision. Withdrawing
certification preserves the historical attestation. Current dependency health is
separate from both facts; unavailable, stale or unknown health cannot reuse a
still-unexpired validation receipt to certify. Edits, restore and accepted impact
proposals create new private drafts without transferring validation or approval.
Archival removes default publication but preserves authorized exact history.

Every addressed operation rechecks current signed block, parent topic, source,
dataset and execution-context reach. Private revisions retain their actor boundary
after later publication. Reading private SQL/values additionally requires preview
reach. Authority is obtained from the verified Pengui envelope, never a JSON field,
creator name, old certificate or locally issued credential.

## Registered operations

The cumulative phase-21 registry is the source for OpenAPI and the phase-23 generic
SDK/CLI operation matrix. Under the configured mount, the concrete routes are:

| Method and path | Typed SDK method | Action |
| --- | --- | --- |
| POST `/v1/blocks` | `CreateBlock` | `reporting.write` |
| GET `/v1/blocks` | `ListBlocks` | `reporting.read` |
| POST `/v1/blocks/capture` | `CaptureBlock` | `reporting.write` plus SQL/query inspection |
| POST `/v1/blocks/questions/assess` | `AssessBlockQuestions` | `reporting.read` |
| GET `/v1/blocks/{id}` | `ReadBlock` | `reporting.read` |
| GET `/v1/blocks/{id}/sql` | `ReadBlockSQL` | `reporting.sql.read` |
| GET `/v1/blocks/{id}/history` | `BlockHistory` | `reporting.read` |
| PUT `/v1/blocks/{id}` | `EditBlock` | `reporting.write` |
| POST `/v1/blocks/{id}/validate` | `ValidateBlock` | `reporting.validate` |
| POST `/v1/blocks/{id}/preview` | `PreviewBlock` | `reporting.preview` |
| POST `/v1/blocks/{id}/publish` | `PublishBlock` | `reporting.publish` |
| POST `/v1/blocks/{id}/certify` | `CertifyBlock` | `reporting.certify` |
| POST `/v1/blocks/{id}/withdraw` | `WithdrawBlockCertification` | `reporting.certify` |
| POST `/v1/blocks/{id}/reject` | `RejectBlock` | `reporting.write` |
| POST `/v1/blocks/{id}/restore` | `RestoreBlock` | `reporting.write` |
| POST `/v1/blocks/{id}/archive` | `ArchiveBlock` | `reporting.write` |
| POST `/v1/blocks/{id}/parameters/resolve` | `ResolveBlockParameters` | `reporting.read` |
| POST `/v1/blocks/{id}/parameters/assist` | `ParameterizeBlock` | `reporting.write` |
| POST `/v1/blocks/{id}/impact` | `RecheckBlockImpact` | `reporting.read` |
| POST `/v1/blocks/{id}/impact/apply` | `ApplyBlockImpact` | `reporting.write` |

The registry omits validation/preview, capture or observation-dependent operations
when their concrete service dependencies are absent. Metadata authoring is not
coupled to model-provider availability. The installed capability is
`governed_blocks`; this does not advertise future reporting execution/rendering.

Definitions and requests are closed schemas. Optional union members and nil
collections may be JSON null, including fields flattened from embedded DTOs;
required scalars cannot be null. Unknown, duplicated or injected identity fields
fail. Requests require `application/json` without media parameters, have a 2 MiB
wire ceiling, and reject content encoding and undeclared query/header semantics.
Responses are bounded to 16 MiB and use `Cache-Control: no-store`.

GET reference queries accept either a positive `revision` or `draft=true`, never
both. Default reads resolve publication, not a private amendment. Lists accept
`after`, `limit` (1–100, default 20) and `include_drafts`; eligibility is applied in
storage before pagination. History and SQL remain separate projections.

Mutations are **never automatically replayed**. There is no invented idempotency
key or implicit recovery of a lost response; callers read current authorized state
and reconcile its version/history. Errors use the shared bounded envelope:
invalid request 400, missing authentication 401, forbidden 403, unavailable/private
reference 404, CAS conflict or stale validation 409, body/result limit 413, rejected
query 422, bounded admission busy 429, unavailable 503 and deadline/cancelled 504.

## Parameters, output definitions and proposals

The closed parameter types are `date`, `datetime`, `relative_period`,
`dimension_value`, `number`, `integer`, `boolean`, `grain` and `top_n`. Defaults,
required flags, bounds/enums and exact dimension references are validated before
source work. Values remain typed bind arguments, never SQL interpolation. Numbers
preserve exact scalar text. Each relative period consumes two physical scalar
slots; the execution contract allows at most 64 slots, independently of the
logical parameter limit.

The pure resolver takes an explicit logical time and IANA timezone. Periods use
half-open windows, explicit DST-fold and month-end policies, and declared first
schedule-window behavior. Calendar/DST/leap-year fixtures are not a running
scheduler. Parameter assistance accepts only exact, native-parsed PostgreSQL date
predicates with an expected definition digest/version. It replaces the selected
literals, retains unrelated SQL/comments/filters and creates an unvalidated draft.
Ambiguous predicates, unsupported dialects and stale proposals fail closed.

Stable output IDs address saved chart, KPI, table or narrative definitions. Empty
selection means all outputs; duplicate/unknown IDs fail; selected outputs retain
saved order. Preview uses the shared lossless read-result/chart adapter and remains
private and ephemeral. Narratives store bounded instructions and evidence rules
only: preview performs no narrative model call and creates no retained artifact.

Impact rechecks compare exact source/semantic pins and observed catalog identity.
Unchanged, cosmetic, rename, review-required and unavailable are distinct outcomes.
PostgreSQL database/role authority, relation OID and column ordinal continuity are
required for rename proof. Same-name replacements and lost native proof require
review. Other engines cannot manufacture native rename identity. An accepted
server-derived proposal is recomputed and CAS-fenced, creates a private amendment,
and never rewrites approved SQL. Output aliases preserve the approved schema.

Query capture uses the current actor's completed private query, same session,
current query/source/topic reach and `reporting.sql.read`. Planned, stale,
foreign-session or unauthorized queries cannot be captured. Capture transfers SQL,
exact parameters/schema and publication provenance but neither re-executes the
source/model nor confers block validation/certification.

## Configuration and verification

`reporting` configuration uses closed bounds and explicit defaults: 64 KiB SQL,
512 KiB definition, 128 columns, 32 outputs, 32 logical parameters, 16 locales,
32 aliases, 128 revisions, 10,000 blocks, four concurrent reads, 1,000 preview rows,
1 MiB preview data, 30-second read timeout, 24-hour evidence TTL and 0.8 lexical
question-match threshold. These limits grant no access. Question discovery is
bounded lexical assessment over authorized definitions, not learned semantic
similarity or a guarantee of global uniqueness.

`TestPhase27/AC01`–`AC08`, `TestPhase21CumulativeRegistryGuard`, the unit regression
and fuzz suites and `scripts/smoke/phase-27.sh` are the executable contract. See
[the review record](../reviews/phase-27-adversarial.md) for actual results and
qualification boundaries. Phase 28 owns recurring reads and retained artifacts.

## Template-origin boundary

A template pin is accepted only from a server-verified capture handoff. The
current query service does not retain a template identity in its completed-query
record, so that adapter leaves the optional template pin absent rather than
inferring one from SQL or accepting a client assertion. Exact semantic topic and
dependency pins are still captured and revalidated; manual authoring cannot
claim template provenance.
