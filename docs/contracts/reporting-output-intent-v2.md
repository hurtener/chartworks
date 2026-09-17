# Reporting output intent and bounded narrative policies v2

CW-03 implements BLK-01, BLK-05 and the native mappings for BLK-07 under
[D-073](../decisions/2026-09-16-reporting-output-policies.md). It extends the
existing block/frozen-run/composition/delivery services; it does not replace their
authority, query validator, chart engine, queue, operation keys or artifact store.
This contract does not claim foreign-system cutover or broader reporting parity.

## Versions and immutable migration

`Definition.schema_version=1` remains readable and executable with its legacy
selection interpretation. New explicit output intent uses `schema_version=2`.
`CanonicalizationVersion` remains `block-definition-v1`: the original v1
canonical encoding, publication bytes and hashes are not rewritten. V2 query
policy has a separate execution-policy digest; full revision/output selection,
metadata, effective evidence policy and accepted limits also bind reuse.

Migration 035 adds a generated `definition_version` and database constraints over
the immutable definition JSON. It does **not** update existing JSON, publications,
validation receipts, attestations, manifests or artifacts. The real populated
schema-34 upgrade test verifies original bytes/hashes and immutable triggers.

`reporting.MigrateDefinition`, exposed as `sdk/chartworks.MigrateBlockDefinition`,
returns a detached draft candidate. It performs no persistence, SQL, model,
validation or publication work. Submit the candidate through ordinary CAS edit,
new validation and publication. Historical published revisions stay accessible
under their existing exact-revision/private rules. Calling migration on valid v2
is a detached identity; malformed v2 omissions are rejected, not repaired with
legacy defaults.

V1 output defaults are deterministic: keep each ID and its definition-array
position; enable and default-select every output; assign display order equal to
that position. For every authored block locale, derive an output name from the
block title, a kind label and the unchanged ID. English/Spanish kind labels are
fixed; no model translates them. If the derived name exceeds its byte limit,
use the kind and ID without truncating the original stored title. Preserve the
block description. This is a legacy display projection, not an in-place rewrite.

## Output intent and selection

Each v2 `Output.intent` contains `metadata` (locale, display_name, description),
`enabled`, `default_selected`, and `display_order`. All are authored at output
level. IDs are unique stable identifiers. Order is unique within a definition,
0–63, and persists independently of array/execution order. Locale lists are
bounded by deployment configuration; names are nonblank and at most 256 bytes,
descriptions at most 4,096 bytes. Locale matching is exact tag, then first authored
same-language tag, then first authored locale; no environment or map order enters
that choice. Operational output-count ceilings still apply.

| Input | V1 | V2 |
|---|---|---|
| Omitted or JSON null outputs | All declared outputs, legacy order | Enabled outputs with `default_selected=true`, display order |
| Explicit `[]` | Legacy all | Reject `output_selection_empty` |
| Explicit nonempty IDs | Exact caller execution order | Exact caller execution order; display remains authored order |
| Duplicate identifier | Reject `output_duplicate` | Same |
| Unknown identifier | Reject `output_unknown` | Same |
| Explicit disabled output | Not a v1 authored state | Reject `output_disabled`; never execute it |
| Disabled during default selection | Not a v1 authored state | Skip; retain a disabled metadata choice |
| No enabled defaults | Not applicable to valid v1 | Reject execution with `output_selection_empty`; description may still return choices plus this code |

These selection errors are content-free typed errors registered on the existing
operations (HTTP 400), not strings that echo private names. Default omission is
not permission to replace a disabled output. An enabled but unselected retained
output returns `output_not_selected` (incomplete/409); an unknown retained output
is not found. Authorization/private checks happen before either response.

`OutputSelection` retains definition version, interpretation mode, requested
nil-versus-empty, selected execution sequence, and display-ordered choices.
Choices distinguish selected, omitted and disabled with typed codes. The wire
schema and SDK preserve nil-versus-empty without adding a second selection model.

## Authored export, imports and projections

`GET /v1/blocks/{id}/sql` / `ReadBlockSQL` now includes an exact native
`definition` beside SQL, revision digest and existing protected provenance. It
requires the existing SQL-inspection **and** ordinary-read authority, plus current
reference/private eligibility. It performs no warehouse/model work. Ordinary
`ReadBlock`, catalogs and viewer descriptions do not return `Definition` or SQL.
The native export retains original definition-array order and authored policies;
metadata read projections deliberately expose resolved policy and display intent,
so they must not be reverse-engineered into an authoring export.

Native v1/v2 JSON uses the same shared domain and SDK types for import/export.
Create/edit validates the complete definition; it never transfers validation,
certification, execution authority or trust badges from an exported object.
Source/context/topic identifiers must resolve under the current signed authority.
Server-owned capture/template provenance cannot be asserted by manual import.
Unsupported/unknown fields fail the closed request schema. Wider foreign import,
identifier mapping, calibrated migration and cutover remain phase 34 work.

Metadata, validation/certification projections, immutable publication snapshots,
frozen manifests, retained outputs, composition summaries and delivery selectors
carry the applicable intent/policy. Effective result policy is beside the shared
ordered `exec.Field` schema, rather than a duplicate result-schema or sensitivity
type. The same native export preserves the authored restrictive policy separately.

## Reviewed sensitivity and evidence egress

The semantic column's existing `LiteralSensitivity` is retained by compiled and
portable semantic contracts. Reporting derives policy from the exact reviewed
topic pins and validator-derived source dependencies, including predicate
columns. Authored `ResultFieldPolicy` and `Narrative.redacted_fields` may only
restrict that decision. Metadata readability does not grant SQL access, and SQL
inspection does not grant raw-result access.

The current validator proves query dependencies, not per-expression result
lineage. Therefore reporting conservatively applies the **whole query's**
reviewed dependency classification to each expected result field. A derived
aggregate or a safe-looking alias is not proof that its inputs are non-sensitive.
This intentionally may exclude otherwise-safe fields when another dependency is
sensitive; narrower per-expression inheritance is unsupported, not guessed.

Only effectively `allowed` fields enter narrative evidence. Missing/unreviewed
classification is `unknown` and excluded. Sensitive wins over unknown; conflicting
reviewed sensitive/non-sensitive metadata is `conflicting` and excluded. An
authored non-sensitive assertion cannot declassify reviewed sensitive input; the
conflict remains excluded. Explicit redaction is final. An authored policy entry
with no sensitivity is unknown, not a public assertion. Policy provenance binds
the reviewed inputs, dependency closure and authored restrictions.

The allowlist intersects this effective policy and manual redaction **before**
reduction, evidence serialization, prompt assembly, reservation and provider input.
Protected values therefore cannot leak by being aggregated first. No surviving
eligible evidence produces retained `narrative_evidence_unavailable` with no
model reservation or call. V2 evaluates this deterministic result before model
availability or lower current model budgets. Raw retained values remain under
existing signed raw-result/context/private rules; narrative egress is a separate
restriction, not a new authorization system or a promise to erase source data.

## Closed narrative controls and limits

New policies pin `policy_version=bounded-narrative-v2` and
`schema_version=grounded-narrative-v1`; `instructions` must be `evidence_only`.
No executable instructions, arbitrary essay mode, SQL/tools, URLs, causation or
unrestricted provider prose are accepted. Existing prompt/model versions remain
pinned. Retained v1 narrative rendering keeps its original hash/wording contract.

| Control | Meaning and enforcement |
|---|---|
| `fields`, `redacted_fields` | Declared expected-schema names; intersect reviewed egress before input |
| `reduction=first_rows` | Deterministic retained row order, bounded by max_rows and max_bytes |
| `reduction=aggregate_evidence` | Exact decimal sum/count over permitted bounded retained observations; never represented as full-source totals |
| `type=summary` | Value claims only, constrained in the provider schema and checked again locally |
| `type=comparison` | Same-field numeric difference claims only; two eligible retained observations required before provider work |
| `type=explanation` | Factual values and exact differences only; no causal explanation or free prose |
| `tone` | `neutral`, `concise`, `technical`; deterministic local wording/datum annotations, not unchecked provider style |
| `locale` | Explicit English/Spanish tag; fixed localized wording, no implicit translation work |
| evidence/caveat requirements | Both required; closed evidence IDs, citation/provenance hashes, explicit retained/bounded/truncation caveats |
| max_claims | 1–32, enforced in provider JSON schema and local grounding |
| max_rows | 1–1,000, enforced before evidence serialization |
| max_bytes | 128–65,536 evidence JSON bytes; at most 256 evidence items; prompt envelope additionally bounded to max_bytes + 8,192 |
| max_characters | 1–16,384 Unicode code points, including mandatory caveat, checked on deterministic rendered text before retention |
| max_calls | 1–4, reserved durably before SDK submission; nested provider retries count against the same gateway budget |
| max_tokens | 64–32,768, reserved durably and enforced by the gateway's tokenizer/usage accounting |
| timeout_ms | 100–60,000, clamped to accepted and current narrative/overall deadlines; cancellable gateway call |

Cumulative per-run calls/tokens also clamp to accepted and current deployment or
schedule budgets. Current limits cannot enlarge old accepted reservations. A
crash after reservation remains indeterminate; explicit retry does not implicitly
regenerate the narrative or refund uncertain work. Provider failures retain the
existing typed output failure and gateway receipt, not unchecked answer text.
Reservation is not measured usage; unavailable usage/cost/remote outcome remains
unknown. Fixtures demonstrate enforcement, not live token-price/quality claims.

Migration accepts the native `evidence_only` instruction and two existing exact
bounded descriptions: `Summarize only the returned evidence` and
`Describe the displayed evidence without inventing values.` It maps them to
`evidence_only`, sets missing legacy max_claims to 32, and versions rendering.
Other instructions/schema versions, non-English/Spanish generation, and
comparison plus aggregate_evidence return `narrative_policy_unsupported` rather
than silently becoming a summary. Review and author a supported new candidate
before publication; no historical narrative is rewritten.

## Source/query limits, acceptance and retries

`Definition.query_limits`, invocation `limits`, and `BlockWidget.limits` use one
`QueryLimits` type: max_rows (0–10,000), max_bytes (0 or 1,024–4,194,304), timeout_ms
(0 or 1,000–60,000), query_attempts (0–3). Zero inherits; positive values only
narrow. Accepted limits are the minimum of deployment, authored, invocation/widget
and current authorized schedule budgets. They bind the immutable run manifest,
reuse key and eligible composition group identity.

At actual execution, the original accepted caps intersect current deployment and
current authorized caps again. Raising deployment cannot bypass an accepted lower
cap; lowering deployment cannot be bypassed by an old higher cap. An intermediate
or reusable result above a current cap is rejected, not regenerated. Different
caps separate reusable results and composition groups. Original accepted limits
remain visible even when actual runtime limits become stricter.

Rows/bytes bound normalized returned results and retained payloads. Timeout and
attempt ceilings bound the service/driver operation, subject to the existing
uncertain-attempt reconciliation contract. They are **not warehouse scan-byte,
compute-cost, physical cancellation or exactly-once guarantees**. Existing native
read-only validation, source revision fence, cancellation intent, attempt journal,
query identifier ownership and live commit fence remain in force.

One eligible frozen query fans out into its declared selected outputs. Frozen
refresh never interprets a question, generates/corrects SQL or selects a chart.
Approved saved mappings and bounded retained evidence are its only output inputs.
Retained reads, paging and redraw perform zero warehouse/model work; expired or
unavailable retained evidence returns the existing typed expired/incomplete
result. A redraw is never an implicit refresh.

## Composition, schedules and viewer

Each block widget retains its own selection and accepted limits. Same eligible
query/context/revision/caps can form one fan-out group; different caps do not
share a broader child's result. Display order does not rewrite execution order.
Truncated evidence remains visibly partial (or fail-closed according to the
existing report policy), never relabeled as complete because a smaller cap was
authored.

Existing schedules keep explicit selected IDs, resolved publication revisions,
periods and the existing `ReportingBudget` through retries. The queue's fresh
signed authority, current eligibility, physical-attempt reservations and catalog
transaction remain authoritative. A delivery retry after a successful retained
query does not execute again or adopt a later publication. Recipients/labels are
not access grants or notification receipts. No new notification integration is
added.

Descriptions and the read viewer show localized names/descriptions, display order,
selected/omitted/disabled state and typed errors. Retained navigation selection is
separate from `accepted_selection`; changing a filter preserves the latter's
exact execution sequence and accepted limits in the explicit new-run request.
The viewer does not create an authoring application or a second authority store.

## Executable evidence and boundary

See [CW-03 acceptance](../../test/acceptance/cw03_reporting_test.go),
[real database upgrade](../../test/acceptance/cw03_migration_test.go),
[scheduled retry](../../test/acceptance/cw03_schedule_test.go),
[provider hard bounds](../../test/acceptance/cw03_narrative_bounds_test.go),
[domain policy tests](../../internal/reporting/output_intent_test.go),
[narrative type/tone tests](../../internal/reporting/narrative_policy_test.go), and
[actual browser component tests](../../web/report-viewer/component.test.mjs).
The [review record](../reviews/cw-03-adversarial.md) identifies concrete findings
and the read-only workflow that retains exact-source execution logs. Existing
strict phase 27–31 criteria remain required. This does not change phase statuses,
close other gap IDs, qualify live providers/warehouses, or deliver phase 32/34/25.
