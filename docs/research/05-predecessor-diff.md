# Brief 05 — Predecessor diff: client predecessor vs. generalistic fork

> Status: draft · 2026-07-06 · sources: both predecessors (functional diff)

> Hygiene: this brief contains **no copied code** — prose, tables, and pseudocode only.
> The two predecessors are referred to only as **the client predecessor**
> (`_ref/original_wayfinder/`) and **the generalistic predecessor** / **the fork**
> (`_ref/forked_wayfinder_explorer/`). No product name, client name, employee name,
> confidential schema/table/column name, prompt vocabulary, or data sample appears —
> everything domain-specific is generalized. Paths cite each predecessor's own root.

---

## Summary

The two trees fork from a common ancestor and are **structurally identical** — the same
~20 packages under `src/<pkg>/`, the same file names, the same class and function
inventory in the overwhelming majority of modules. The divergence is therefore not
architectural; it is a set of **targeted differences** concentrated in a handful of
subsystems. The single largest theme:

- **The client predecessor hardened multi-tenant access and source-table health**; it
  carries an RBAC + visibility + topic-access data model with an inescapable
  (`NOT NULL`) tenant predicate, source-table health tracking, a source re-check flow,
  full table-update-with-reference-rewriting, and topic sharing + export/import.
- **The fork generalized data-source connectivity and access computation**; it replaced
  the single env-pinned warehouse with a runtime **connection registry** (factory +
  driver seam, dialect normalization), and replaced inline access checks with a unified
  **access_resolver** service (RBAC + ACL + service accounts). It also cleaned up the NLQ
  pipeline into instrumented stages and added tenant-embedding cleanup on delete.
- **The topic-lifecycle *engine* is functionally identical** between the two (the version
  manager, the diff engine, the replay service differ only by method ordering). The
  lifecycle *divergences* live at the edges: the client's visibility/sharing/health/
  table-update features vs. the fork's connection-scoped access model.

Per kickoff answer 5, **where they diverge the client predecessor's fixes take precedence**;
per kickoff answer 4, **the topic-pack lean context-engineering layer must be kept/
enhanced**. The right synthesis for Chartworks is: **carry the client's hardened tenant/
health/table-integrity fixes, expressed through the fork's cleaner seams** (the connection
registry, the unified access resolver, the staged pipeline) — not one tree wholesale.

---

## Tree alignment map

Both trees are the same package graph; only the top package directory name differs
(generalized here as `<pkg>`). Alignment is 1:1 except for the files listed as
added/removed below.

| Package (`src/<pkg>/…`) | Role (generalized) | Divergence weight |
|---|---|---|
| `domain/` | Pydantic domain models: topic pack, measures/dimensions/KPIs, metadata enums, routing, SQL gen/validation, context packing | **High** — topic model, metadata enums, context packing |
| `services/` | Orchestration: version manager, table/tenant/template/relationship lifecycles, NLQ pipeline, caches, validators, access resolver (fork) | **High** — table lifecycle, NLQ pipeline, access resolver |
| `stores/sql/metadata/` | Postgres metadata store (Chartworks' *own* state) + SQLite (dev) | **High** — schema (tenant/RBAC/visibility) |
| `stores/vector/` | Vector index for topic/routing embeddings | Low |
| `adapters/` | Customer **data-source** warehouse adapters | **High** — connection registry, dialects, driver set |
| `api/routes/` | HTTP surface | **High** — sharing/export/import/recheck (client); access ctx (fork) |
| `config/` | Typed settings, env indirection | **High** — single-warehouse env (client) vs. connection secrets (fork) |
| `flows/` | Pipeline flow graphs (routing, retrieval, SQL gen, enhancement) | Medium — mlflow node wrap (client); staged timings (fork) |
| `presentation/` | Chart catalog, generators, suitability scoring | Low (near-identical) |
| `retrieval/`, `inference/`, `enhancers/`, `rules/`, `evidence/`, `scheduling/`, `workers/`, `security/`, `observability/`, `extraction/`, `client/`, `cli/`, `utils/` | Supporting subsystems | Low–Medium |

**Files present in only one tree** (the concrete fork points):

| Only in **client predecessor** | Only in the **fork** |
|---|---|
| `observability/mlflow_tracing.py` (provider-specific tracer) | `services/access_resolver.py` (unified RBAC+ACL+svc-account) |
| `scheduling/triggers/condition.py`, `.../event.py` (richer schedule triggers) | `adapters/postgres.py`, `adapters/sqlserver.py`, `adapters/null.py` (driver set) |
| `api/main 2.py` (stray editor backup — ignore) | `utils/dialects.py`, `utils/dsn.py` (dialect/DSN normalization) |
| `client/…_client.py` (in-tree HTTP client variant) | `config/warehouse_secrets.py` (per-connection secret model) |
| | `domain/warehouse_connection.py`, `domain/column_types.py` |
| | `api/routes/warehouse_connections.py`, `api/routes/tenant_business_domain.py` |
| | `services/metadata_warehouse.py`, `security/canvas_tokens.py` |

Note the *asymmetry*: the client's "extra" files are mostly production add-ons (health,
richer triggers, MLflow); the fork's "extra" files are almost entirely the **data-source
generalization** (connections, dialects, DSN, secrets, adapters) plus the **access
generalization** (resolver, business-domain routes). This is the whole story in one table.

---

## Topic lifecycle (deepest section)

The topic (the semantic-model unit — the "topic pack" of measures, dimensions, derived
KPIs, join graph, and semantic/routing context) is versioned. A **topic** is a stable
identity with a pointer to its **active version**; a **topic version** is an immutable-ish
snapshot that moves through a stage machine. All lifecycle mutations are audited.

### Core domain shape (shared)

`domain/topics.py` defines the pack: enhanced tables (with columns, keys, foreign keys,
measures, dimensions, join paths), keyed dictionaries of enhanced measures / enhanced
dimensions / derived KPIs, a semantic context bundle (triggers, business priority,
example questions, groupings), query patterns, and a compact **capability contract**
(`TopicCapabilityContract` → `to_prompt_dict()`/`to_prompt_json()`) that is the
prompt-safe projection used for routing and SQL generation. **This capability-contract
projection is the "lean context engineering" the kickoff wants preserved** (answer 4): the
pack carries rich authoring metadata, but only a compact, active-only, prompt-shaped view
reaches the model. The pack's validators canonicalize entity ids, resolve KPI→measure
references by id-then-name, prune dangling semantic-context keys (recording
`migration_warnings`), and reject join paths that reference undefined tables.

### The state machine

**Version stage** (`domain/metadata.py :: TopicVersionStage`), identical in both:

```
DRAFT ──▶ REVIEW ──▶ PROMOTED ──▶ DEPRECATED
  │                     ▲   │
  └── discard (delete)  │   └── rollback ──▶ (a prior PROMOTED becomes active again)
                        └── promote sets topic.active_version_id
```

- **DRAFT** — mutable working copy; can be updated in place or discarded.
- **REVIEW** — a candidate under validation (a holding stage; transitions are actor-driven).
- **PROMOTED** — a promoted version; exactly one promoted version is the topic's
  `active_version_id` at a time. Promotion triggers an embedding reindex.
- **DEPRECATED** — superseded/retired version, retained for audit and rollback lineage.

**Topic status** (`TopicStatus`): `ACTIVE` / `ARCHIVED` — the *topic*-level lifecycle,
orthogonal to version stage. `ARCHIVED` removes the topic from routing.

**Audit actions** (`TopicVersionAuditAction`), identical: `create`, `update`, `discard`,
`promote`, `rollback`, `diff`. Every transition records an audit row with actor,
version numbers, and change counts.

**Client-only enum — `TopicVisibility`** (`PRIVATE` / `TENANT_PUBLIC`): the client carries
a per-topic visibility axis; **the fork removed it** and folded access into its
`access_resolver` + business-domain model. This is the pivotal lifecycle divergence
(detail below).

Adjacent state machines (shared): `SQLTemplateStatus` (`candidate→active→deprecated→
retired`), `TemplateAuditAction` (`seed/promote/deprecate/retire/validate/repair`),
`CrossTopicRelationshipStatus` (`pending→confirmed→deprecated`), `JobStatus`
(`queued→running→succeeded/failed/cancelled`).

> Hygiene flag: the template machine and its audit actions include a **`repair`** action
> and a template **`repair`**-class surface. This is precisely the plumbing-word /
> "repair surface" anti-pattern P6 exists to prevent (CLAUDE.md §1 P6 calls it out by
> name). Chartworks must **not** carry the `repair` vocabulary onto any wire/UI/log
> surface; the *operation* (revalidate + rebind a stale template) is legitimate, the
> *name* is not.

### Stage 1 — Creation (generation)

`POST /topics` enqueues a **topic-generation job** (`create_topic_generation_job`) — async,
returns `202` with a job status. The job runs the enhancement/engineering flow
(schema discovery → column semantics → join inference → measure/dimension/KPI synthesis →
semantic-context authoring) and writes an initial **DRAFT** version. Identical in both;
the fork additionally resolves a warehouse **connection id** at creation and maps it to a
display name (`_connection_name_map`), because a topic in the fork is bound to a named
connection rather than the single global warehouse.

### Stage 2 — Draft authoring

The version manager (`services/topic_version_manager.py`) exposes the draft primitives
(**functionally identical** in both trees — the 337-line diff is pure method reordering):

- `create_draft_from_version(topic_id, base_version_id, …)` — branch a new DRAFT from any
  existing version (copy pack, stamp `updated_at`).
- `update_draft_version(topic_id, version_id, pack, …)` — **update a DRAFT in place**
  (same `version_number`), compute a `TopicVersionDiff`, persist, and record an `UPDATE`
  audit with `change_counts`. Guard: only `DRAFT` stage may be updated in place; a
  cross-topic version id is rejected.
- `discard_draft_version(…)` — delete a DRAFT; **guard: the active version can never be
  discarded**; also deletes its embeddings via the vector store; records a `DISCARD` audit.
- `persist_mutation(…)` — the smart entry point: if base is a DRAFT and target stage is
  DRAFT, update in place; otherwise create a new version and diff against the base.

Entity-level authoring (all under `api/routes/topics.py`, both trees): CRUD for
dimensions, measures, derived KPIs, tables, and join paths; `move_topic_entity` (move a
measure/dimension between tables, rewriting semantic context and cleaning references);
per-entity semantic-context PATCH endpoints; and starter-prompts GET/PATCH. Each mutation
routes through `_persist_topic_pack_mutation` → the version manager, so every edit is
diffed and audited.

### Stage 3 — Review / validation

`REVIEW` is a holding stage between DRAFT and PROMOTED. Validation is not a single gate but
the sum of: pack-model validators (structural), `services/sql_validator.py` +
`services/pattern_validator.py` (generated-SQL / template validity), `coherence.py` and
`relationship_lifecycle.py` (cross-topic relationship confirmation), and the
`topic_replay` service (below). Both trees share this; the divergence is only in
*who may transition* (access model, below).

### Stage 4 — Promotion

`promote_topic_version` (route) → version manager promotion:

1. Sets `topic.active_version_id` to the promoted version, capturing the
   `previous_active_version` id in audit metadata.
2. Records a `PROMOTE` audit (with `previous_active_version_id`).
3. **Enqueues an embedding reindex job** (`_enqueue_reindex_job` →
   `TopicReindexJobPayload`, operation `PROMOTION`) so the routing index reflects the new
   active pack. This is the **context-card / evidence regeneration** trigger.

Both trees share this. Only one version is active per topic; promotion is the atomic swap.

### Stage 5 — Versioning & diff

Versions carry a monotonic `version_number`; the diff engine (`_build_diff`, exposed as
`compute_diff`) produces a structured `TopicVersionDiff` with `change_counts` per entity
class (measures/dimensions/KPIs/joins/tables added/removed/modified). `GET …/diff` and
`GET …/versions` (history, with actor display-name enrichment) expose it. Identical in
both.

### Stage 6 — Update / mutation of a promoted topic

Editing a promoted topic does **not** mutate the active version in place: `persist_mutation`
branches a new DRAFT (or a new version at the requested stage) and diffs against the active
base, preserving the promoted snapshot until a new promotion. This is the immutability
discipline that makes rollback safe.

### Stage 7 — Table lifecycle (**major divergence**)

`services/table_lifecycle.py`. **The client predecessor's version is ~2× the fork's
(429 vs. 204 lines) and carries capability the fork dropped:**

- **`update_table` (client-only).** Rename/replace a source table and **rewrite every
  reference** across the pack — measures, dimensions, derived KPIs, and join paths —
  via `_swap_table_prefix`, `_rewrite_measure_table_references`,
  `_rewrite_dimension_table_references`, `_rewrite_kpi_table_references`,
  `_rewrite_join_path_table_references`. The fork has **no `update_table` at all** — it is
  remove-only.
- **`_mark_source_issue_resolved` (client-only).** Clears a table's source-health issue
  after a re-check succeeds — coupled to the health fields below.
- Both share `remove_table` + orphan filtering (`_filter_orphaned_measures/dimensions`,
  `_filter_join_paths`) so removing a table prunes now-dangling entities.

**Source-table health (client-only), in `domain/topics.py :: EnhancedTable`:**
`is_active: bool`, `health_status: "healthy" | "inaccessible" | "disabled"`,
`health_message`. Active-only accessors (`active_measures/dimensions/tables`,
`fact_tables()` filtering on `is_active`). **The fork stripped these** — its accessors do
not filter on health/active. This is a production-hardening the fork regressed: when a
source table becomes inaccessible (dropped, permissions revoked, catalog moved), the
client marks it and excludes it from routing/generation instead of emitting broken SQL.

**Source re-check (client-only), `POST /topics/{id}:recheck-source`:** enqueues a
schema-refresh job (`recalculate_joins=True`, `preserve_manual_edits=True`) whose summary
is literally "source re-check after catalog/table repair" — i.e. the operational fix for
"a source table changed underneath a published topic." The fork lacks the endpoint.

### Stage 8 — Rollback

`rollback_topic_version` (route) → the version manager selects the previous promoted
version (`_previous_promoted_version` walking version history), re-points
`active_version_id`, records a `ROLLBACK` audit (capturing `original_active_version_id`),
and enqueues a reindex with operation `ROLLBACK`. Shared by both.

### Stage 9 — Deprecation / archive

Two distinct mechanisms:
- **Version-level `DEPRECATED`** stage — a superseded version, kept for lineage/rollback.
- **Topic-level archive** — `POST /topics/{id}:archive` sets `TopicStatus.ARCHIVED`. The
  client hardens this with **cache invalidation on archive**: it refreshes the router cache
  to drop the topic from routing **and** explicitly clears the template-prior cache for the
  topic (`services/router_cache.py`, `template_prior_cache.py`). Archiving also passes the
  vector store so embeddings are cleaned. This is a client fix against stale routing after
  archive; verify the fork clears both caches (it shares the route but the client's
  cache-clear discipline is the reference).

### Stage 10 — Deletion & cache/embedding invalidation

There is no hard-delete of a *promoted* topic in the surfaces reviewed — retirement is
archive (topic) + deprecate (version); hard delete exists only for **DRAFT** versions
(`discard_draft_version` → `delete_topic_version` → `vector_store.delete_embeddings`). The
invalidation matrix (both trees, client is the reference where they differ):

| Event | Metadata effect | Vector/index effect | Cache effect |
|---|---|---|---|
| DRAFT update | new payload, same version_number | (draft not indexed until promotion) | — |
| DRAFT discard | row deleted, `DISCARD` audit | `delete_embeddings(version)` | — |
| Promote | `active_version_id` swap, `PROMOTE` audit | reindex job (`PROMOTION`) | router cache refreshed on next read |
| Rollback | `active_version_id` re-point, `ROLLBACK` audit | reindex job (`ROLLBACK`) | router cache refreshed |
| Archive (topic) | status `ARCHIVED` | embeddings cleaned | **router cache + template-prior cache cleared (client)** |
| Tenant delete | cascade | **`_delete_tenant_embeddings` (fork-only)** | — |

Context "cards" (the compact routing/evidence projection) are **not** stored mutable state
you invalidate directly — they are **regenerated** from the active pack on reindex and
recomputed at pack time by the `ContextPacker` (`domain/context_packing.py`), which builds
the prompt-safe context per query from the active capability contract. Invalidation is
therefore driven by the reindex-on-promotion/rollback jobs plus the caches above, not by a
separate card store.

### Sharing & visibility (client-only lifecycle surface)

`GET /topics/{id}/sharing` and `PUT /topics/{id}/sharing` (`TopicSharingResponse`), backed
by `TopicVisibility` (`PRIVATE`/`TENANT_PUBLIC`) and a `topic_access(tenant_id, topic_id,
role)` table. This is the client's **deny-by-default-with-explicit-grant** access model at
the topic grain — directly relevant to Chartworks P1. **The fork removed the sharing
endpoints and the visibility enum**, moving to `access_resolver` (below). The *data model*
here is a strong carry candidate; the *computation* is better in the fork.

### Export / import (client-only lifecycle surface)

`GET /topics/{id}:export` (`TopicExportBundle`) and `POST /topics:import` — topic
portability with **sanitization** (`_sanitize_export_topic_pack`,
`_sanitize_import_topic_payload`) that strips internal ids/metadata before export and
re-canonicalizes on import, plus a post-import sample-values refresh
(`_enqueue_post_import_sample_values_refresh`). The fork lacks all of this. Relevant to
Chartworks both as a feature and as a **P6 hygiene exemplar** (the sanitizer is where
plumbing ids are stripped from an externally-visible artifact).

### Recommended unified topic state machine for Chartworks

Adopt the shared four-stage version machine, keep it explicit, and pin the transition
rules the predecessors only imply:

```
            create-generation-job
                     │
                     ▼
   ┌───────────────DRAFT───────────────┐
   │  update-in-place (diff+audit)      │
   │  discard (delete+deindex)          │  submit-for-review
   └──────────────────┬─────────────────┘
                      ▼
                   REVIEW ──(validation gate: structural + SQL-safety + coherence)──┐
                      │  reject ▲                                                    │
                      ▼         └────────────────────────────────────────────────── │
   promote (atomic active_version_id swap ─▶ reindex job) ─▶ PROMOTED ◀─ rollback ── ┘
                      │                                         │
                      │  supersede-on-next-promote              │
                      ▼                                         ▼
                  DEPRECATED (retained for lineage)     topic-level: ACTIVE ⇄ ARCHIVED
```

Binding rules Chartworks should make explicit (the predecessors enforce most implicitly):

1. **Exactly one PROMOTED-active version per topic**; promotion is an atomic swap that
   captures the prior active id (for rollback lineage) and enqueues reindex.
2. **The active version is never mutated in place and never discarded** — edits branch a
   DRAFT (carry the client's `discard` guard verbatim).
3. **Every transition is audited** (actor + change counts) via a typed audit action; no
   silent stage change (P4).
4. **Carry source-table health as a first-class lifecycle input** — `is_active` +
   `health_status` gate what routing/SQL-generation sees, and a re-check flow repairs it.
   (This is the highest-value client fix the fork lost.)
5. **Transition authority is computed by one access resolver** (fork shape) against the
   client's grant/visibility data model, tenant predicate `NOT NULL` (P1/P3).
6. **Reindex/cache invalidation is part of the transition, not a follow-up** — promote,
   rollback, and archive each carry their invalidation (client's archive cache-clear is
   the reference). Regenerate context projections from the active pack; do not keep a
   separate mutable card store.
7. Rename the template **`repair`** action to a domain-clean verb (e.g. "revalidate"/
   "rebind") before it touches any surface (P6).

---

## Divergence — Routing & NLQ pipeline

Shared: the router (`domain/semantic_routing.py`, ~1580 lines, near-identical), retrieval
flows, the context packer, SQL generation (`domain/sql_generation.py`,
`flows/sql_generation.py`), SQL validation (`services/sql_validator.py`,
`domain/sql_validation.py`), and the auto-fixer (`services/sql_auto_fixer.py`).

Divergences:

- **NLQ pipeline structure (fork improved).** `services/nlq_pipeline.py` in the fork is
  refactored into discrete instrumented stages: `_run_stage`, `_emit_pipeline_timings`,
  `_cache_attribution_for_stage`, `_build_telemetry` (an `…FlowTelemetry`), and
  `_effective_routing_config`. It also **filters topic packs by governance/version before
  routing** (`_filter_topic_packs`, `_governed_version_ids`) — only governed/promoted
  versions are eligible to route. The client's pipeline is flatter and less instrumented.
  The fork's staged-timings + governance-version filter are worth keeping.
- **Context packing (fork cleaned).** `domain/context_packing.py` — the fork factored
  entity resolution into helpers (`_resolve_measure_definition`,
  `_resolve_dimension_definition`, `_resolve_derived_kpi_definition`, `_primary_identifier`,
  `_payload_identifiers`) replacing inline `.get()` chains, with id-then-name fallback.
  Functionally equivalent, cleaner — a good base for the "enhanced" lean context layer.
- **SQL validator logging (fork).** The fork adds dialect-aware diagnostics and a
  `_sql_snippet(sql, max_length)` truncator in validation-failure logs. Useful, **but
  logging SQL text risks leaking data/identifiers** — under P4/§7 Chartworks logs must be
  content-free; carry the *dialect-aware structured error*, not the raw-SQL log line.
- **MLflow node wrapping (client-only).** `flows/sql_generation.py` wraps flow nodes with
  `wrap_mlflow_node` and `observability/mlflow_tracing.py` traces the pipeline. This is a
  **provider-specific tracer** — under P5 all model-call instrumentation goes through the
  gateway/telemetry seam. Carry the *concept* (per-stage tracing of routing/gen/validate),
  not the MLflow dependency.

## Divergence — Data-source / connectivity (**fork is the reference here**)

The client predecessor pins **one global warehouse** at boot via env vars (a single
`WAREHOUSE_KIND` ∈ Databricks/Postgres/BigQuery/Snowflake with per-kind secrets like host/
http-path/token or DSN/project/account). The fork **generalized to runtime-managed
connections**:

- `domain/warehouse_connection.py` + `api/routes/warehouse_connections.py` +
  `stores/metadata_warehouse.py` + `config/warehouse_secrets.py` — connections are
  first-class, tenant-scoped entities with managed secrets, not env constants.
- `adapters/registry.py` — a **factory registry** (`WarehouseKind → factory`) with
  `MOCK`/`POSTGRES`/`SQLSERVER` registered via factory functions; drivers imported lazily.
  This is *exactly* Chartworks' §4.4 interface+factory+driver seam shape.
- `utils/dialects.py` + `utils/dsn.py` — dialect-label → sqlglot-dialect normalization with
  an **`ansi` sentinel** for "generic/unknown warehouse" (the null adapter's dialect and
  the default fallback), plus DSN canonicalization. `adapters/null.py` is the no-op driver.
- `domain/column_types.py` — normalized cross-dialect column typing.

Trade-off: the fork's registry ships fewer *live* drivers (mock/postgres/sqlserver; the
Databricks adapter is present but the BigQuery/Snowflake env paths were dropped), whereas
the client had all four kinds wired via env. Chartworks §4.4 says the V1 driver set is
RFC-owned; **carry the fork's registry+dialect+connection model as the seam, and let the
RFC pick the V1 driver set** — but reconcile with tenant scoping (next section).

## Divergence — API surface

| Endpoint / helper | Client | Fork | Note |
|---|---|---|---|
| `GET/PUT /topics/{id}/sharing` | ✅ | ✖ | client visibility/RBAC sharing |
| `GET /topics/{id}:export`, `POST /topics:import` | ✅ | ✖ | portability + sanitization |
| `POST /topics/{id}:recheck-source` | ✅ | ✖ | source-health re-check |
| `warehouse_connections` routes | ✖ | ✅ | runtime connection mgmt |
| `tenant_business_domain` routes | ✖ | ✅ | business-domain scoping |
| access helpers | `_require_topic_read/_admin`, `require_topic_access` | `_resolve_topic_read/_write_access` → `TopicAccessContext`, `_check_sa_topic_access` (service accounts), `_enforce_topic_tenant_access`, `_connection_name_map` | fork centralizes into a context object + resolver |

The rest of the topic CRUD surface is 1:1. The access-helper divergence is the API face of
the deeper access-model divergence below.

## Divergence — Access model, tenant isolation & config/ops (the crux for P1/P3)

- **Client metadata schema (hardened):** `tenants`, `tenant_memberships`, `invite_codes`,
  `topic_access(tenant_id, topic_id, role)`, a `visibility` column on topics, and
  **`tenant_id NOT NULL`** on topics, saved-queries, schedules, schedule-runs, with
  tenant-scoped indexes. This is an **inescapable tenant predicate** + role-based topic
  grants — a near-perfect match for Chartworks P1 (deny-by-default, computed in query path)
  and P3 (tenant predicate at the storage layer).
- **Fork changes:** replaced membership/invite/`topic_access` RBAC-in-schema with a
  `services/access_resolver.py` ("unified topic access resolution: RBAC + ACL + service
  accounts") — `resolve_user_visible_topics`, `resolve_service_account_visible_topics`,
  `allowed_grant_roles` (role hierarchy admin ≥ member ≥ viewer), an `_AllTopics`
  admin-bypass sentinel — plus `security/canvas_tokens.py` and business-domain routes.
  **But the fork relaxed `tenant_id` to nullable in places** (e.g. `tenant_id TEXT
  REFERENCES …` instead of `NOT NULL`). A nullable tenant predicate **violates P3** and is
  a regression Chartworks must not inherit.
- **Config (client):** single warehouse via env + MLflow telemetry env. **Config (fork):**
  per-connection secrets, no MLflow. Chartworks is CGo-free/OTel-seam (D-005, §8/§10), so
  neither config surface transfers verbatim; the fork's per-connection secret model is the
  better base, hardened with fail-closed-at-boot secret validation (§7).

**Synthesis for Chartworks (P1/P3/P7):** take the **client's data model** (explicit
per-topic grants + visibility + `NOT NULL` tenant everywhere) and compute it through the
**fork's single `access_resolver`** (one access representation, service-account path
matching P2's `svc:` model, admin bypass as an explicit sentinel), returning a visible-set
predicate **intersected inside the query** — never fetch-then-filter.

---

## Carry table — client-predecessor fixes Chartworks must consider carrying

| Area | Fix (client has, fork lacks) | Bug it fixed | Evidence path (client root) | Carry? |
|---|---|---|---|---|
| Table lifecycle | `update_table` with full reference-rewriting across measures/dims/KPIs/joins | Renaming/replacing a source table left dangling references / broken SQL | `src/<pkg>/services/table_lifecycle.py` | **Yes** — core semantic-model integrity (P4) |
| Source health | `EnhancedTable.health_status`/`health_message`/`is_active` + active-only accessors | A dropped/inaccessible source table silently produced broken or empty SQL | `src/<pkg>/domain/topics.py` | **Yes** — fail-loud on source drift (P4) |
| Source health | `POST …:recheck-source` schema-refresh flow + `_mark_source_issue_resolved` | No way to repair a topic after the underlying catalog/table changed | `src/<pkg>/api/routes/topics.py`, `services/table_lifecycle.py` | **Yes** — as a domain-clean re-check op (not "repair") |
| Tenant isolation | `tenant_id NOT NULL` on topics/queries/schedules + tenant-scoped indexes | Rows without a tenant could escape isolation (cross-tenant leak) | `src/<pkg>/stores/sql/metadata/postgres.py` | **Yes** — mandated by P3 |
| Access model | `topic_access(tenant,topic,role)` grants + `TopicVisibility` (private/tenant-public) | Implicit/over-broad topic visibility | `src/<pkg>/stores/sql/metadata/postgres.py`, `domain/metadata.py`, `api/routes/topics.py` | **Yes (data model)** — compute via fork's resolver |
| Archive | Router-cache **and** template-prior-cache clear on archive | Archived topic kept being routed to / kept stale template priors | `src/<pkg>/api/routes/topics.py` (`archive_topic`) | **Yes** — invalidation as part of transition (P4) |
| Portability | Export/import with id sanitization + post-import sample-value refresh | Topics not portable across environments; raw internal ids would leak on export | `src/<pkg>/api/routes/topics.py` | **Yes** — also a P6 sanitizer exemplar |
| Scheduling | Richer schedule triggers (`condition`, `event`) | Time-only triggers insufficient for event/condition-driven refresh | `src/<pkg>/scheduling/triggers/` | **Maybe** — RFC to scope scheduling |
| Observability | Per-node pipeline tracing (concept, via MLflow) | No stage-level trace of routing→gen→validate | `src/<pkg>/observability/mlflow_tracing.py` | **Concept only** — via telemetry seam, not MLflow (P5) |

## Fork-keeper table — fork improvements worth keeping the client lacks

| Area | Improvement (fork has, client lacks) | Value | Evidence path (fork root) | Keep? |
|---|---|---|---|---|
| Data-source seam | Connection **factory registry** (`WarehouseKind → factory`, lazy driver import) | Matches Chartworks §4.4 interface+factory+driver seam | `src/<pkg>/adapters/registry.py` + `adapters/{null,postgres,sqlserver}.py` | **Yes** |
| Data-source seam | Runtime **managed connections** (not env-pinned) + per-connection secrets | Multi-source, tenant-scoped; separates customer data sources from Chartworks' Store (D-004) | `src/<pkg>/domain/warehouse_connection.py`, `api/routes/warehouse_connections.py`, `config/warehouse_secrets.py`, `services/metadata_warehouse.py` | **Yes** |
| Dialects | Dialect normalization with `ansi` generic sentinel + DSN canonicalization + normalized column types | Portable multi-warehouse SQL generation/validation | `src/<pkg>/utils/dialects.py`, `utils/dsn.py`, `domain/column_types.py` | **Yes** |
| Access | Unified `access_resolver` (RBAC + ACL + service accounts, role hierarchy, admin sentinel) | One access representation (P7); svc-account path (P2 `svc:`) | `src/<pkg>/services/access_resolver.py` | **Yes (computation)** — over client's data model |
| NLQ pipeline | Staged, instrumented pipeline + governance/version topic-pack filtering before routing | Per-stage timings/telemetry; only governed versions route | `src/<pkg>/services/nlq_pipeline.py` | **Yes** |
| Context packing | Extracted entity-resolution helpers (id-then-name fallback) | Cleaner lean-context layer to build on (kickoff answer 4) | `src/<pkg>/domain/context_packing.py` | **Yes** |
| Tenant cleanup | `_delete_tenant_embeddings` on tenant deletion | Prevents orphaned vector state on tenant removal | `src/<pkg>/services/tenant_lifecycle.py` | **Yes** |
| SQL diagnostics | Dialect-aware validation-failure structured errors | Better multi-dialect debugging | `src/<pkg>/services/sql_validator.py` | **Partial** — keep dialect in *structured error*, not raw-SQL logs (P4/§7) |

---

## Open questions for the RFC

1. **Access primitive (P1).** Adopt the client's explicit `topic_access` role grants +
   visibility, or the fork's business-domain scoping, or both? Recommendation: client data
   model + fork resolver, with `tenant_id NOT NULL` restored. RFC must pin the exact grant
   primitive and the "empty effective-access ⇒ no query" short-circuit.
2. **Source-table health as lifecycle input.** Should `health_status` gate routing/SQL
   generation (recommended, per P4), and what is the domain-clean name for the "re-check"
   op (must avoid the `repair` plumbing word, P6)?
3. **Data-source driver set (V1).** The fork's registry ships mock/postgres/sqlserver;
   client had Databricks/BigQuery/Snowflake via env. Which drivers are V1 (§4.4 says
   RFC-owned)? Is the `ansi`/null generic-warehouse sentinel V1?
4. **Template `repair` action & surface.** Confirm the forbidden-word list bans `repair`/
   `index`/`shard`/`embedding` etc. on the wire (P6) and specify the replacement verbs for
   the template revalidate/rebind operation.
5. **Reindex & cache invalidation contract.** Formalize which transitions (promote /
   rollback / archive / discard / tenant-delete) trigger which invalidation (metadata /
   vector / router cache / template-prior cache) — the matrix above should become a binding
   table so no surface omits an invalidation (P7).
6. **Pipeline tracing under the gateway/telemetry seam.** The client's per-node tracing is
   MLflow-coupled; the RFC must define the telemetry-seam equivalent (P5) so routing/gen/
   validate stages are observable without a provider tracer.
7. **Export/import sanitization scope.** Is topic export/import in V1 scope, and does its
   sanitizer become the reference implementation for the P6 id-stripping discipline on any
   externally-visible artifact?
8. **REVIEW-stage authority.** Neither predecessor pins *who* may move DRAFT→REVIEW→
   PROMOTED beyond the access role; the RFC should state the transition-authority matrix
   explicitly (which role/grant authorizes which transition).
