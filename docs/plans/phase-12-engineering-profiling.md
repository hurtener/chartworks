# Phase 12 — `engineering-profiling` (Wave 4)

> **Status:** draft
> **Owner:** orchestrator (Opus-first; medium difficulty)
> **Depends on:** phase-05-gateway, phase-06-jobs-scheduler, phase-08-sources-core

Authored per the CLAUDE.md §16 workflow. This is the first phase of
`internal/engineering` (the data-engineering stage that makes Chartworks more than
its predecessors, RFC §7 / D-013). It ships **profiling**: selective dataset
registration, the profile job, quality/freshness classification, profile
versioning, and the schema-drift re-discovery diff whose dataset-health signal
phase 15 (topics lifecycle) consumes. Transformations, pipelines, and the
materialization write path are phase 13.

---

## RFC / request sections

- **RFC §7.1** — Registration & discovery (selective, allowlist-starts-here, P1a;
  `TypeCategory` computed once at discovery, fork keeper).
- **RFC §7.2** — Profiling & quality (the normalized dataset profile: per-column
  stats, sampled+sanitized values, the six quality dimensions, value families,
  freshness classification, large-table safeguards).
- **RFC §7.5** — Versioning, lineage, freshness (schema-drift detection via a
  re-discovery diff that marks affected datasets and flags dependent topics'
  source health — §8.2 stage 7).
- Supporting: **RFC §1.2 P1a** (deny-by-default, computed in the query path),
  **§3.3** (async jobs, no LLM on the hot path), **§6.1** (the adapter seam and
  its structured read methods), **§8.2 rule 4** (source health as a lifecycle
  input — the interface this phase publishes), **§13** (the `profile_summary`
  gateway role), **§12** (`datasets` / `dataset_profiles` store tables),
  **§14** (config surface), **§15** (metrics, content-free audit).
- Decisions relied on: **D-013** (DE stage is V1 scope), **D-020** (grants /
  scope primitive), **D-024** (uploads are datasets like any other — profiled the
  same way), **D-025** (one generic leased queue), **D-003 / P5** (gateway seam
  for `profile_summary`), **D-005** (CGo-free — pure-Go profiling), **D-004**
  (profiles live in the `store`; source data is read only through the adapter
  seam).

## Depends on

- **phase-08-sources-core** — the data-source adapter seam (`DiscoverSchema`,
  `SampleValues`, `Query`, `Capabilities`/`Supports`), the connections registry,
  the `datasets` candidate rows produced by discovery, and the dialect-agnostic
  `TypeCategory` classification. Profiling reads customer data **only** through
  this seam, scoped by grants. This phase closes a seam phase 08 opened
  (discovery → registration → profile) ⇒ an integration test is required (§17).
- **phase-06-jobs-scheduler** — the generic leased queue. The `profile` operation
  runs as a typed handler on that queue (D-025); no bespoke profiling worker.
- **phase-05-gateway** — the `profile_summary` role for optional dataset
  descriptions (schema-constrained, metered, redaction-gated). Profiling is fully
  functional with `summaries_enabled=false`; the gateway is an optional enricher,
  never on a query's hot path.
- Transitively present (Wave 3 shipped before Wave 4): phase-02 store +
  `dataset_profiles` migration, phase-04 access resolver + frozen envelope,
  phase-09/10 (the P1b hard gate is already satisfied — see Non-goals for why
  profiling still does not touch them).

## Informing briefs

Per `docs/research/INDEX.md` (`internal/engineering` → 11 primary; 09, 10, 12, 02
secondary). This phase draws on **09** (profile/freshness/value-family shapes),
**12** (the six quality dimensions), and **02** (discovery evidence, sample
window, the `money`-column typing bug).

- `docs/research/09-agents-repo-ideas.md`
- `docs/research/12-genbi-landscape.md`
- `docs/research/02-predecessor-data-and-execution.md`

## Brief findings incorporated

- **Brief 09 §4 (structured, scored profile artifact)** → the normalized
  `DatasetProfile`: schema + per-column null%/distinct-bucket/min-max, row count,
  date range, quality assessment, provider-agnostic (CLAUDE.md §6 "outputs are
  first-class normalized shapes"), golden-tested. The brief's prose "potential
  issues" / "recommended queries" free text is **dropped** (see departures).
- **Brief 09 §5 (freshness scale)** → the `fresh / stale / very_stale / unknown`
  age-bucket classification is adopted verbatim (RFC §2). The brief's
  escalate-to-owning-pipeline step is dropped: Chartworks has no contract with
  customer ETL — the honest signal stops at "very_stale, source unknown."
- **Brief 09 §3 (value families)** → for low-cardinality text dimensions, group
  distinct values by common prefix/suffix into families (`Export*` over
  `ExportCSV/ExportJSON`) so a later NLQ filter does not miss variants. Stored per
  text column on the profile.
- **Brief 09 §2 (data-layer / large-table heuristic)** → the "large table —
  date-bound queries preferred" hint, recorded as dataset metadata when a row
  estimate exceeds the configured threshold.
- **Brief 12 (six quality dimensions)** → completeness, uniqueness, validity,
  consistency, integrity, timeliness — the full GX-standard set, **named in
  domain terms** (not GX's internal vocabulary; P6). At least one check per
  dimension, not an ad-hoc subset.
- **Brief 02 (`TypeCategory` computed once, the `money`-column bug)** → the
  profiler consumes the dialect-agnostic `TypeCategory` phase 08 computed at
  discovery; it never re-sniffs raw type strings per column (the ad-hoc
  per-module type-check scar).
- **Brief 02 (sample window; no full-table cache/scan)** → profiling reads a
  **bounded** sample through the adapter (capped by `sample_ceiling`), never an
  unbounded scan; stats are computed over that bounded sample.

## Findings I'm departing from

- **Pushed-down aggregate SQL for exact stats — deferred.** A naive profiler would
  push `COUNT DISTINCT`, exact `MIN/MAX`, `MAX(timestamp)` down as generated SQL.
  This phase deliberately computes profile stats **in Go over a bounded adapter
  sample** using the seam's structured read methods (`SampleValues`,
  `DiscoverSchema` catalog metadata), rather than emitting generated SQL. Reasons:
  (1) it keeps profiling within the deny-by-default structured adapter surface and
  off the arbitrary-SQL path; (2) it makes the sampling-ceiling guarantee
  mechanical (the profiler cannot express a full-table scan); (3) it matches the
  master-plan dependency set (05/06/08, not 09/10). Exact row count and global
  min/max/freshness are taken from adapter **catalog statistics** when the adapter
  `Supports` them, else marked sampled/approximate, else `unknown` (never
  fabricated — P4). Pushdown aggregation is a post-V1 optimization behind the same
  seam. This is a phase-local design choice inside D-013's mechanism; if it proves
  load-bearing across phases a decision entry lands in the same PR (no new D-NNN
  filed now).
- **Brief 09's "recommended queries" and prose issue list — not adopted.** They
  are free-text model output (P5/P6 risk) and duplicate the semantic model's job.
  The profile carries structured signals only; suggested questions, if ever, come
  from the semantic layer.
- **Confidence scores per inferred column-role (brief 12 Q2b) — not in V1.**
  Chartworks' topic model is *declared + assisted*, not inferred; the profile
  reports facts, not inference confidence. Revisit if agent-assisted topic
  generation (phase 15) wants it.

## Scope

New package **`internal/engineering`** (first phase to own it), delivering:

1. **Selective registration flow.** Promote discovery candidates (phase 08) into
   registered `datasets` (origin `source_table`) — an admin action gated by
   `source.manage`. Nothing is queryable until registered (the allowlist starts
   here, P1a). Registration enqueues a `profile` operation.
2. **The profile operation** (typed handler on the phase-06 queue): produces the
   normalized `DatasetProfile` — per-column stats (null%, distinct-count bucket,
   min/max, sanitized length-capped sample values), row count + date range,
   quality assessment over the six dimensions, value families for low-cardinality
   text columns, freshness classification, and the large-table hint.
3. **Profile versioning.** Each run appends a `dataset_profiles(dataset_id,
   version, profile_json, quality_json, profiled_at)` row; prior versions are
   retained (append, never overwrite).
4. **Schema-drift re-discovery diff.** A `re-check source` operation re-runs
   `DiscoverSchema`, diffs against stored dataset schemas (added / removed /
   renamed / retyped columns, dropped tables), marks affected datasets'
   availability/health, and flags **dependent datasets** (materializations whose
   lineage declares the drifted table upstream). Publishes the
   `SourceHealthReporter` interface that phase 15 consumes for §8.2 rule 4.
5. **Optional `profile_summary`.** When enabled, a schema-constrained gateway call
   attaches a short dataset description, fed **only sanitized aggregate metadata**
   (never raw customer rows — redaction before any gateway call, CLAUDE.md §7).
6. New **`engineering`** config domain (profiling ceilings + freshness thresholds)
   with a fail-loud validator.

## Non-goals

- Transformations, pipelines, quality **checks-as-run-gates**, the `Materializer`
  write path, runs/lineage-on-write, schedules — all **phase 13**. This phase
  computes quality *dimensions on a profile*; it does not enforce them against a
  pipeline run.
- Topic-level dependent flagging. Phase 12 marks **datasets** and publishes the
  health interface; **phase 15** reads it to flag dependent *topics* (topics do
  not exist yet). Cross-dataset **referential integrity** against a join graph is
  therefore partial here (single-dataset key-shape only) and completed once a join
  graph exists (phase 15 / pipeline checks phase 13).
- Any generated / pushed-down SQL execution against a source (see departures);
  profiling uses structured adapter reads only.
- Warehouse-driver breadth (`bigquery`/`snowflake`/`databricks`) — phase 14. V1
  profiling is validated on the `postgres` + `mock` adapters; it is
  adapter-agnostic by construction (capability-gated via `Supports`).
- Chart column metadata and topic generation *consume* profiles (RFC §10 / §8.1)
  but are later phases; this phase only produces and stores them.

## Design

### Data flow

```
discovery candidates (ph08) ──register (source.manage)──▶ datasets row (registered)
                                                              │ enqueue "profile"
                                                              ▼
        jobs queue (ph06) ── profile handler ── engineering.Profiler
                                                    │ reads via sources.Adapter
                                                    │   DiscoverSchema(ctx,scope)
                                                    │   SampleValues(ctx,scope,…)  ← bounded by sample_ceiling
                                                    │ computes in Go:
                                                    │   per-column stats · 6 quality dims · value families
                                                    │   freshness bucket · large-table hint
                                                    │ optional: gateway profile_summary (redacted)
                                                    ▼
                                        dataset_profiles (new version row)  +  audit + metrics

re-check source ──▶ DiscoverSchema diff vs stored schema ──▶ mark dataset availability/health
                                                          └▶ flag dependent (materialized) datasets via lineage
                                                          └▶ SourceHealthReporter  (consumed by ph15 §8.2 r4)
```

### Key types (all in `internal/engineering`)

- `DatasetProfile` — the normalized, golden-shaped artifact:
  `DatasetID, ProfileVersion, RowCount (+ Approximate bool), DateRange,
  Columns []ColumnProfile, Quality QualityAssessment, Freshness FreshnessClass,
  LargeTable *LargeTableHint, Summary string (optional), ProfiledAt`.
- `ColumnProfile` — `Name, TypeCategory (from ph08), NullPercent, DistinctBucket,
  Min, Max, SampleValues []string (sanitized, ≤ sample_value_max_len),
  ValueFamilies []ValueFamily`.
- `DistinctBucket` — a cardinality tier (`1`, `2–10`, `11–100`, `101–1k`,
  `1k–10k`, `>10k`) — exact counts avoided on large tables (RFC §7.2 "bucket").
- `QualityAssessment` — one scored/flagged entry per dimension, keyed by a
  **domain** name: `completeness` (null/missing rates), `uniqueness`
  (duplicate/key-candidate detection), `validity` (values conform to their
  `TypeCategory` / min-max / length shape), `consistency` (cross-column invariants
  within the sample; value-family conformance), `integrity` (single-dataset
  key-shape / null-key detection — cross-dataset FK checks deferred), `timeliness`
  (= the freshness signal).
- `FreshnessClass` — `fresh | stale | very_stale | unknown` (RFC §2), from the
  newest value of the dataset's temporal column (catalog stat where available,
  else the bounded sample) bucketed by the configured thresholds. No temporal
  column / no rows ⇒ `unknown` (never a fabricated bucket).
- `ValueFamily` — `Prefix/Suffix pattern + member values` (brief 09 §3).
- `Profiler` — the pure compute core: `Profile(ctx, scope, dataset, reads) →
  DatasetProfile`. Stateless, safe for concurrent reuse (per-call state in
  params, never receiver fields; proven under `-race`).
- `SourceHealthReporter` (published interface, phase 15's contract):
  `DatasetHealth(ctx, scope, datasetIDs …) ([]DatasetHealth, error)` where
  `DatasetHealth{DatasetID, IsAvailable, HealthStatus, LastCheckedAt}`. Drift
  marking writes these fields; §8.2 rule 4 reads them.
- `DriftDiff` — `Added/Removed/Renamed/Retyped columns, DroppedTables,
  AffectedDatasetIDs, DependentDatasetIDs`.

### Seam adherence & P1–P7

- **P1a (deny-by-default, in the query path).** Every adapter read takes the
  non-optional `scope` derived from the frozen envelope reconstructed from the
  job payload's `(tenant, principal)`. The access resolver (phase 04) computes
  effective access first; an **empty set short-circuits to a typed denial and
  issues no adapter read** (asserted by adapter call-count). Registration and
  re-check are `source.manage`-gated; profiling needs `read` on the source/dataset
  (grant intersection inside the read, never fetch-then-filter).
- **P1c (write split).** Profiling is strictly read: it uses only the adapter's
  read methods and never touches `Materializer`. Its only writes are to
  Chartworks' own `store` (`dataset_profiles`, dataset health fields) — the D-004
  boundary (source data via the adapter seam; own state via the `store` seam).
- **P3 (tenant isolation).** All store reads/writes and adapter reads carry the
  tenant predicate; a cross-tenant profile request resolves to nothing.
- **P4 (fail loud).** An adapter read error, a decrypt failure, or a partial
  sample fails the profile run with a typed error + metric + content-free audit
  and status — never a silently empty or partial profile written as success.
- **P5 (one intelligence seam).** `profile_summary` is the only model touch and it
  goes through `internal/gateway`, schema-constrained, metered; free-text JSON
  parsing forbidden. Sample rows are **redacted to aggregate metadata** before the
  call (CLAUDE.md §7).
- **P6 (domain vocabulary).** Quality dimensions and the large-table hint are
  named in user-recognizable domain terms; the profile carries no plumbing nouns;
  the drift operation is **`re-check source`** / **`revalidate`**, never "repair".
- **D-025 (one queue).** Profiling is a typed handler `kind` on the phase-06
  queue; no per-concern worker class. LLM enrichment is async, never on a query's
  hot path (RFC §3.3).

## Config keys added

New `engineering` domain (documented here, added to the example config, and
smoke-checked in the same PR per CLAUDE.md §4.2). Fail-loud validator: every
ceiling `> 0`; `very_stale_after > stale_after`.

| Key | Type | Default | Required | Notes |
| --- | --- | --- | --- | --- |
| `engineering.profiling.sample_ceiling` | int | `100000` | no | Max rows read per dataset for profiling. The sampling ceiling — the profiler cannot exceed it (no full-table scans). |
| `engineering.profiling.sample_values` | int | `20` | no | Sanitized sample values captured per column. |
| `engineering.profiling.sample_value_max_len` | int | `200` | no | Character cap per sanitized sample value. |
| `engineering.profiling.value_family_max_cardinality` | int | `50` | no | Max distinct count for a text column to receive value families. |
| `engineering.profiling.large_table_row_threshold` | int | `100000000` | no | Row estimate above which the "date-bound queries preferred" hint is recorded. |
| `engineering.profiling.summaries_enabled` | bool | `false` | no | Gates the `profile_summary` gateway role. Off ⇒ no gateway call. |
| `engineering.freshness.stale_after` | duration | `24h` | no | Age at/after which a dataset is `stale`. |
| `engineering.freshness.very_stale_after` | duration | `168h` | no | Age at/after which a dataset is `very_stale`. |

## Acceptance criteria

Numbered, mechanically checkable; each maps to a smoke assertion below. Criteria
1–4 are the master-plan minimum; 5–12 complete the surface.

1. **Golden profile shape.** A profile over a fixed mock dataset serializes to the
   pinned golden `DatasetProfile` JSON (all fields present, field order/shape
   stable). `TestProfileGoldenShape`.
2. **Sampling ceiling — no full-table scans.** Profiling issues only bounded
   sample reads capped at `sample_ceiling`; the mock adapter's call log shows no
   unbounded/full-table read. `TestSamplingCeilingNoFullScan`.
3. **Freshness buckets across fixtures.** `fresh/stale/very_stale` are computed
   correctly for age fixtures at the configured thresholds, and a dataset with no
   temporal column resolves to `unknown` (never a fabricated bucket).
   `TestFreshnessBuckets`.
4. **Drift diff flags dependents.** Re-checking a source whose re-discovery removes
   or renames a column marks the source-table dataset unavailable/unhealthy **and**
   flags every materialized dataset that declares it upstream (lineage), reported
   through `SourceHealthReporter`. `TestSchemaDriftFlagsDependents`.
5. **Six quality dimensions, domain-named.** The quality assessment populates all
   six dimensions (completeness, uniqueness, validity, consistency, integrity,
   timeliness) under domain-recognizable keys — no missing dimension, no GX
   internal term on the artifact. `TestQualitySixDimensions`.
6. **Value families.** A low-cardinality text column groups `ExportCSV`/
   `ExportJSON`/`ExportParquet` into an `Export*` family; a high-cardinality column
   (> `value_family_max_cardinality`) produces none. `TestValueFamilies`.
7. **Large-table hint.** A dataset whose row estimate exceeds
   `large_table_row_threshold` records the domain-phrased date-bound-preferred hint
   in metadata; one below it does not. `TestLargeTableHint`.
8. **Profile versioning.** Re-profiling a dataset appends a new profile version and
   **retains** the prior version (row count increases; the earlier `profile_json`
   is unchanged). `TestProfileVersioning`.
9. **Grant-scoped, empty-set short-circuit (P1a).** A profile request whose
   resolved effective-access set is empty returns a typed denial and issues **zero**
   adapter reads (adapter call-count assertion — the fetch-then-filter regression
   guard). `TestProfileScopeShortCircuit`.
10. **`profile_summary` optional + redacted.** With `summaries_enabled=false` no
    gateway call occurs; with it true, the gateway receives only sanitized
    aggregate metadata (no raw customer rows), via a schema-constrained, metered
    call. `TestProfileSummaryOptionalRedacted`.
11. **Fail loud.** An adapter read error mid-profile fails the run with a typed
    error, a metric, and a content-free audit row — no partial/empty profile is
    written as success. `TestProfileRunFailsLoud`.
12. **Config defaults + validation.** The `engineering` config block loads its
    documented defaults; a config with `very_stale_after <= stale_after` or a
    non-positive ceiling is refused at boot with a typed error.
    `TestProfilingConfigDefaults`.

## Test obligations

Per CLAUDE.md §11:

- **Unit (table-driven):** quality-dimension computation (per dimension), freshness
  bucketing (age → class incl. `unknown`), value-family grouping, distinct
  bucketing, the golden profile shape, sample-value sanitization/length-capping,
  drift-diff classification, config defaults/validation.
- **Integration (§17 — required; deps name phase 08 and this closes the
  discovery→profile seam):** profile a real dataset against a **Docker Postgres**
  data source through the phase-08 adapter seam (`make pg-up`). Proves scope
  propagation into adapter reads, the sampling ceiling against a real driver, and
  that `dataset_profiles` rows land with the tenant predicate. Lives in
  `test/integration/` (the wiring spans `engineering` + `sources` + `store`).
  Gateway stays mocked (the one sanctioned boundary mock), paired with a
  recorded-fixture `profile_summary` test.
- **Adversarial (touches the P1a access path):** empty-access short-circuit
  (criterion 9), a cross-tenant profile probe (a dataset in tenant B is invisible
  to tenant A's profile request), and a fetch-then-filter regression guard
  (profiling scopes *inside* the adapter read, never filters after). Forged-header
  is n/a — identity comes from the frozen envelope reconstructed from the job
  payload, never headers.
- **Fuzz:** `FuzzSanitizeSampleValue` — seed corpus of raw cell strings (control
  chars, over-long, multibyte, embedded quotes); invariant: never panics, output
  always `≤ sample_value_max_len`, no control characters leak into the profile.
- **Bench:** `BenchmarkProfileColumn` and `BenchmarkValueFamilies` over a
  ceiling-sized sample — the profiler is a hot reusable artifact (per column, per
  dataset). `make bench` baseline, not a CI gate.

## Coverage targets

Per CLAUDE.md §11 defaults. Entry added to `scripts/coverage-bands.conf` in the
implementation PR.

| Package | Target | Rationale (if not the default) |
| --- | --- | --- |
| `internal/engineering` | 80% | New `internal/` package — the §11 default. |

## Smoke checks

`scripts/smoke/phase-12.sh` runs each criterion's Go test via `run_group` against
`./internal/engineering` (SKIPs cleanly until the package/test exists).

| Acceptance criterion | Smoke assertion |
| --- | --- |
| 1 golden profile shape | `TestProfileGoldenShape` PASS |
| 2 sampling ceiling / no full scan | `TestSamplingCeilingNoFullScan` PASS |
| 3 freshness buckets | `TestFreshnessBuckets` PASS |
| 4 drift flags dependents | `TestSchemaDriftFlagsDependents` PASS |
| 5 six quality dimensions | `TestQualitySixDimensions` PASS |
| 6 value families | `TestValueFamilies` PASS |
| 7 large-table hint | `TestLargeTableHint` PASS |
| 8 profile versioning | `TestProfileVersioning` PASS |
| 9 empty-set short-circuit (P1a) | `TestProfileScopeShortCircuit` PASS |
| 10 profile_summary optional + redacted | `TestProfileSummaryOptionalRedacted` PASS |
| 11 fail loud | `TestProfileRunFailsLoud` PASS |
| 12 config defaults + validation | `TestProfilingConfigDefaults` PASS |

## Glossary additions

Pre-written for `docs/glossary.md` (same PR). Only terms new to the glossary —
`dataset`, `freshness`, `lineage`, `re-check source`/`revalidate` already exist.

- **Dataset profile** — the normalized, versioned per-dataset artifact: per-column
  stats (null%, distinct-count bucket, min/max, sanitized sample values), row
  count + date range, a six-dimension quality assessment, value families,
  freshness, and an optional description. Feeds topic generation, chart column
  metadata, and the dataset inspector (RFC §7.2).
- **Quality dimension** — one of the six standard data-quality axes a profile
  assesses: completeness, uniqueness, validity, consistency, integrity,
  timeliness (brief 12; named in domain terms, P6).
- **Value family** — a group of related low-cardinality text values sharing a
  common prefix/suffix (`Export*` over `ExportCSV`/`ExportJSON`), recorded on a
  text dimension's profile so a later NLQ filter does not miss variants (brief 09).
- **Distinct-count bucket** — the cardinality tier a column's distinct count falls
  into; exact counts are avoided on large tables (RFC §7.2).
- **Large-table hint** — dataset metadata recording that date-bound queries are
  preferred for a table above the configured row threshold (brief 09).
- **Selective registration** — the admin-gated promotion of discovery candidates
  into registered, queryable datasets (the allowlist origin, P1a; RFC §7.1).
- **Schema-drift diff (re-discovery diff)** — the operation comparing a fresh
  schema discovery against stored dataset schemas, marking affected datasets and
  flagging dependents; feeds source health (RFC §7.5, §8.2 rule 4).

## Decisions filed

No new `D-NNN`. This phase implements settled decisions: **D-013** (DE stage
scope), **D-020** (grant/scope primitive), **D-024** (uploads profiled like any
dataset), **D-025** (one leased queue), **D-003/P5** (gateway seam for
`profile_summary`), **D-005** (CGo-free, pure-Go profiling), **D-004** (profiles in
the `store`; source reads only via the adapter seam). The sample-based-profiling
choice (see departures) is a phase-local design decision within D-013's mechanism;
should it prove load-bearing across phases, a decision entry lands in the same PR.

## Deviation log

<!-- Filled DURING implementation (CLAUDE.md §4.3): every reasonable deviation
     from this plan, why, and confirmation this file was updated in the same PR. -->

- (none yet)
