# Semantic foundation v1

Status: bounded phase 15/16 implementation foundations, 2026-09-07. Phases 15/16 are
`in_progress`; phase 21 now has a bounded source HTTP registry consumer and is also
`in_progress`. This contract records implemented behavior only where it points
to executable code and tests; the later-state sections are implementation inputs,
not runtime claims.

The [bounded review evidence](../reviews/phase-15-16-foundation.md) pins the reviewed
code and focused checks. Full phase acceptance remains unavailable until the
remaining service work is implemented.

## Implemented boundary

`internal/semantics` now owns a stable, typed topic-pack definition and a compiler
that returns a detached canonical model. It includes:

- exact source, execution-context, dataset, source revision, profile version, and
  profile-content digest provenance for each dataset;
- stable dataset, column, measure, dimension, KPI, join, and revisioned
  canonical-entity IDs;
- exact typed references, with dataset qualification required for columns and no
  display-name or source-name fallback;
- bounded closed enums for aggregations, dimension roles, equality-only joins, and
  join cardinality;
- whole-pack reference validation, duplicate rejection, KPI-cycle rejection, and
  typed validation codes;
- one pinned revision per canonical stable ID, same-source/context/revision join
  validation, and preserved per-dataset composite-key order;
- deterministic ordering and hashing over a detached copy; and
- Unicode compatibility/case-folded canonical-term lookup that returns stable
  entity IDs and exact key-column references.

The pack is authoring data. It contains no lifecycle stage, active pointer, ready
facet pointer, signed authority, current source-health assertion, SQL, bearer,
credential, or model prompt. A compiler success therefore does not publish a topic,
prove current source health, authorize a caller, or make a definition executable.
Source names remain bounded opaque profile evidence; the semantic compiler does not
impose a PostgreSQL identifier grammar on future qualified warehouse adapters.
Structural entity and text bounds are checked before copying or sorting. Accepted
canonical JSON is limited to 1 MiB. Joins require the same source, execution context,
and source revision on both sides; profiles can have different observation versions.
Canonical key references preserve their declared order within each dataset. They
are identity metadata and do not themselves declare a join or prove key uniqueness.

## Portable authoring definitions and version diff

`ExportPortable` consumes a compiled model plus complete explicit dataset and column
logical-slot mappings. Its `PortablePack` DTO structurally excludes installation
topic/version IDs, source/context/profile coordinates, physical column names and
native types. It has no actor, session, credentials, authority, or lifecycle fields.
Semantic names, descriptions, units, expressions, stable measure/dimension/KPI/join
IDs, and exact canonical-entity revisions are retained. Dataset/column references
are rewritten to the supplied logical slots everywhere, including join endpoints
and ordered canonical keys. Collection order is deterministic and the result is
detached from the model. Free authoring text is preserved; this is a structural
projection, not automatic detection or removal of confidential text.

`ImportDraftCandidate` requires a destination topic/version and complete explicit
`DraftBindings`. Every logical dataset/column slot must map once to concrete source
and profile provenance plus column identity, physical name, native type, category,
and nullability. It rejects missing/duplicate mappings and category/nullability
mismatches, preserves semantic names and exact canonical revisions, rewrites every
column reference, then invokes the existing `Compile` function. The normal cycle,
join-context, source-revision, duplicate, and reference checks therefore also apply
to imported candidates. Portable JSON and the resulting compiled pack each have
the existing 1 MiB ceiling; structural and text bounds apply before deep copying.

The result type is deliberately `DraftCandidate`: an untrusted authoring candidate,
with detached pack/digest access and no publication or execution API. Supplied
`SourceReference` values are not verified-binding proofs. A consuming service must
revalidate the entire mapping against actual source/profile health and current
Pengui reach before storing a draft. It must also resolve canonical registry
collisions and validate exact revision meaning; carrying a revision number does not
approve a registry entry in the destination. No destination ID is allocated, no
profile is refreshed, and no rule-set/pattern lifecycle bundle is imported by this
pure helper. The [private draft service](topic-drafts-v1.md) now supplies mapped import
admission, export authorization, immutable private revision history and canonical
collision preflight through the phase 21 registry. Reviewed publication owns registry
approval.

`DiffModels` compares two compiled versions of the same topic and reports their
exact version/digest pins, topic metadata changes, ordered entity changes, and
per-kind added/removed/modified counts. Dataset metadata/provenance is compared
separately from columns, avoiding double-counting a column-only edit. Canonical
revision changes are modifications of the same stable entity. Changed values are
represented by content hashes; raw source names and authoring text are not included
in the diff. Version-only changes retain the different version/digest pins with
zero entity changes. This helper neither infers renames/moves nor supplies persisted
history, actor audit evidence, or lifecycle transitions.

## Phase 21 prerequisite assessment

The first shared consumer now exists: `internal/api` owns immutable registration
metadata, DTO-derived wire schemas, exact source-style route matching and OpenAPI
3.1.1 generation. The source API registries supply all 33 existing catalog/validation,
execution, engineering and pipeline operations to their actual routers, legacy
manifests and generated documents. Existing Pengui verification, domain resource
checks, transport limits, error mapping and audit behavior are preserved. Resource-loader
and audit labels describe the existing service paths, not new enforcement callbacks.

The [source registry evidence](../reviews/phase-21-source-registry.md) records actual
HTTP/SDK/PostgreSQL schema checks. This replaces that consumer's independent route
inventory; it does not introduce another parallel business API. Foundation
health/capabilities, security and work adapters still require migration.
`/capabilities` continues to
report `business_api: false`; fourteen concrete topic draft/lifecycle operations are
now registered separately through the same shared contract. Public document
delivery, cumulative generated SDK/isolation/audit checks and all six named phase
21 criteria remain incomplete.

Phase 15 keeps its hard dependency on phase 21. The semantic compiler remains an
independently useful domain foundation. Topic lifecycle operations must register
their concrete schemas and authority/resource paths through the shared contract as
those services land; this bounded source adapter does not establish full phase 21
acceptance or satisfy the missing topic service work.

## Required phase 15 continuation

The private draft service now consumes current phase 12 profile evidence, verifies
dataset provenance against source discovery and commits immutable CAS revisions with
audit and dependency fences. This supplies draft mutation, scoped history/diff,
neutral mapped import/export and unapproved canonical proposals.
The bounded [publication service](topic-publication-v1.md) now supplies the immutable
review/publication model, Bifrost embedding generation, current source contract and
version-fenced facet activation using `internal/vindex`, including atomic approval of
new canonical meaning and context-local canonical facets. Reviewed source-reference
rewrite, entity/onboarding, retained public health and bounded resumable enhancement
are now service-backed consumers.
Phase 12 profile records are private to their originating actor/session. Their
retained evidence can seed an authorized draft, but published-topic readers cannot
use that private profile lookup as the current-health service. A topic consumer
needs current source/context health through the source service and deliberately
published semantic evidence, without exposing the original private profile.

The phase 15 gateway bridge must expose one typed, immutable, non-secret embedding-space
descriptor. `internal/vindex` requires the full provider/route/endpoint/model/revision/
dimension/preprocessing/input/normalization identity; reconstructing it independently
from configuration can drift from the actual Bifrost response.

Publication finalizes the complete ready facet generation and active semantic
version in one PostgreSQL transaction that locks both heads in a fixed order, or leave
the prior version usable. Calling the existing self-transactional vector publication
method followed by a separate topic pointer update is insufficient. Current source health stays a
separate observation checked when contracts are read. Archive, rollback, current
source recheck, Pengui authorization, shared domain registration and SDK paths are
implemented by that bounded consumer. The phase 15 service also owns profile-backed
onboarding, reviewed source rebinding, durable public health observations and bounded
resumable enhancement checkpoints. Recorded gateway and real PostgreSQL fixtures
exercise all six named cumulative phase acceptance criteria; live provider quality
remains a separately measured deployment concern.

## Phase 16 integration contract

### Implemented authoring compiler

`semantics.CompileRules` consumes a nonzero compiled semantic `Model` and a
`RuleSetDefinition` pinned to its exact topic, topic version, and pack digest. It
returns a detached `RuleModel` with a deterministic digest. Rules and patterns have
stable IDs and explicit versions. Duplicate stable IDs, including duplicates at a
different version, are rejected within each definition namespace. Rules, patterns,
and target sets are canonically ordered; slot and choice order is preserved as
author-selected presentation order.

The closed rule categories are computation, semantic, and structural. A rule has
either an execution constraint or advisory guidance, never both. Scope is the whole
topic or an explicit set of exact references; it identifies the objects governed
by the rule and is not a query-matching predicate. Priority is retained in the
bounded range -1000 through 1000. Provenance identifies human/model/feedback/import
authoring evidence and grants no authority or review approval.

The initial constraint vocabulary is `require_reference` and `exclude_reference`.
These describe presence in a semantic dependency graph, not visible output columns,
SQL predicates, row restrictions, or data permissions. The compiler follows existing
measure/dimension/KPI/join-to-column/dataset references and rejects a requirement
whose direct or transitive dependency is excluded. It reports a bounded typed
`RuleConflictError` with the two rule IDs and conflicting reference; the error string
contains no definition content. Priority cannot suppress mandatory constraints.
Canonical registry references are allowed in scopes and choices with exact revision
pins, but cannot stand for one executable dependency because their keys may span
datasets. This does not implement general business-expression satisfiability or query
execution enforcement; additional constraint kinds require their actual consumers.

Clarification patterns carry exact target references and ordered typed slots for
choice, text, number, boolean, or date. Choices have unique stable IDs, labels, and
optional exact targets restricted to the pattern's declared targets. Labels never
become guessed references. Advisory text and slots require explicit `non_sensitive`
or `sensitive` declarations. The compiler checks declarations, not whether natural
language has been truthfully classified; literal detection and supplied-value
validation remain runtime work.

Bounds are 256 rules, 128 patterns, 32 entity targets per rule/pattern, 16 slots per
pattern, and 2–32 choices for a choice slot; non-choice slots cannot contain choices.
Advisory text is limited to 4096 bytes, slot prompts to 1024 bytes, and choice labels
to 256 bytes. Accepted canonical rule-set JSON is limited to 1 MiB. The compiler
does no prompt trimming, token estimation, model/source I/O, pattern matching, rule
activation, or query execution. Its authoring definition has no lifecycle stage or
review claim. These remain obligations of the service described below.

### Implemented bounded lifecycle and remaining integration

The [rule lifecycle v1 contract](rule-lifecycle-v1.md) now supplies immutable
private draft revisions, explicit review receipts, CAS publication/retirement,
retained exact reads, and deterministic hard-constraint evaluation over explicit
semantic references. It uses the existing topic actions and persisted dependency
reaches through the shared HTTP/SDK consumer. Evaluation is neither execution
authority nor a validated query plan. Clarification matching, real-token advisory
assembly, replay/shadow comparison and affected-evidence invalidation remain open.

Phase 16 builds rule and clarification types on `semantics.Reference`. It must not add
a second string-addressed entity namespace or resolve rule targets by display name.
The initial persisted rule definition is:

```text
RuleDefinition {
  id, version, topic_id, topic_version_id
  targets[]: semantics.Reference
  category: computation | semantic | structural
  class: execution_constraint | advisory_context
  priority, provenance, review_evidence
  stage: proposed | active | retired
  constraint?: closed typed constraint
  guidance?: bounded advisory text
}
```

Exactly one of `constraint` or `guidance` is present according to `class`. An execution
constraint is structured data consumed by validation/binding and is never deleted to
meet a prompt budget. Advisory guidance has a separately configured real-token budget;
evaluation reports included and omitted rule IDs. Activation compiles one immutable
rule-set version against one exact semantic pack digest, rejects missing targets and
conflicts, and requires explicit review evidence. Feedback may create a proposal only.

The initial clarification definition is:

```text
ClarificationPattern {
  id, version, topic_id, topic_version_id, targets[]: semantics.Reference
  slots[]: {
    id, prompt, required, value_type, sensitive_literal
    choices[]?: { id, label, target?: semantics.Reference }
  }
}
```

A matched required slot stops generation until the caller supplies a typed choice.
Sensitive literal handling is explicit on the slot and supplied value; unmarked
sensitive input is a typed rejection. Choice labels are presentation only. Choice IDs
and optional targets bind the accepted value. Pattern generation uses only the existing
Bifrost `clarify` role with a closed schema; deterministic evaluation and editing make
no model call.

The evaluator returns one immutable result containing the exact topic/rule-set/pattern
versions, mandatory constraints, included and omitted advisory rules, contradictions,
and unresolved slots. Replay and shadow execution persist those pins and use the shared
reader/evaluation path under current Pengui authority. A comparison cannot mutate the
active rule set or widen its source/context reach.

## Phase 16 implementation sequence and evidence

1. Extend the implemented authoring types and compiler with additional actual
   execution-constraint consumers and the immutable evaluation-result contract,
   reusing `semantics.Reference` and canonical pack digests.
2. Add PostgreSQL immutable rule-set/pattern versions, reviewed lifecycle transitions,
   CAS active pointers, and affected-evidence invalidation with their first service
   consumer.
3. Implement deterministic scope, priority, conflict, sensitive-literal, and slot
   evaluation. Token accounting applies only to advisory text and reports omissions.
4. Add the schema-constrained Bifrost `clarify` producer and bounded replay/shadow
   orchestration. Replay resolves current Pengui authority and uses the existing reader;
   it does not introduce another executor.
5. Register the actual rule/pattern/comparison operations through the phase 21 contract,
   then add SDK methods. The Pengui operation manifest must be extended from the actual
   platform registration; this document does not invent deployed action strings.
6. Implement `TestPhase16/AC01` through `AC06` with real versioned PostgreSQL state,
   deterministic multilingual fixtures, zero-call assertions for deterministic paths,
   current-authority negatives, and immutable shadow evidence.

Until those steps and their named tests pass, phase 16 remains `in_progress`, not shipped.

## Foundation verification

The foundation has focused unit and concurrent-reuse tests in
`internal/semantics/compile_test.go`, including exact references, revision collisions,
KPI cycles, source/context join mismatches, canonical terms, composite-key ordering,
detached results, and structural/serialized size limits. The 2026-09-07 local run of
`GOMAXPROCS=2 go test -race -count=1 -p=2 -coverprofile=/tmp/chartworks-semantics-final.cover ./internal/semantics`
passed with 87.8% statement coverage against the 80% package band. Focused `go vet`,
`make planning-check`, mirrored contributor rules, and `git diff --check` passed.

Those historical checks cover the pure compiler and documentation foundation. The
later [private draft stage](../reviews/phase-15-topic-drafts.md) records separate
PostgreSQL and HTTP/SDK evidence. Neither stage claims a complete phase 15/16
acceptance criterion, publication, browser interaction, live provider quality or
cloud CI result.

The rule compiler adds focused tests in `internal/semantics/rules_test.go` for stale
semantic and canonical-revision pins, nested mutation/concurrent reuse, closed unions,
scope containment, unmarked sensitivity declarations, direct/transitive conflicts,
priority preservation, ordered choices, and structural/serialized size bounds. They
do not establish required-slot gating, real-token advisory injection, reviewed
activation, replay/shadow execution, or cache invalidation.
The 2026-09-07 combined compiler run passed with race detection and 91.9% statement
coverage (`/tmp/chartworks-semantics-rules-final.cover`); focused `go vet`, planning,
diff, and mirrored-rule checks also passed. Independent review and cloud acceptance
remain separate gates.

Portable mapping and diff tests are in `internal/semantics/portable_test.go` and
`internal/semantics/diff_test.go`. Synthetic round trips cover every reference-bearing
entity, destination names/types, exact canonical revision/key order, structural DTO
exclusions, deterministic output, missing/duplicate mappings, type/nullability
mismatches, invalid references/cycles, mutation isolation, concurrent reuse, and
entity-level changes/counts. The 2026-09-07 combined race run passed with 93.9%
statement coverage (`/tmp/chartworks-semantics-portable-final.cover`); focused `go vet`,
planning, diff, and mirror checks also passed. These are pure compiler/projection checks, not phase 15
AC06, stateful import, source-health verification, or export authorization evidence.
