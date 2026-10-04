# Native chart presentation v1

This optional mapping extension changes displayed table headers and supported
numeric precision. It does not change physical column identity, SQL, data types,
units, currency, percent scale, semantic provenance, source partitions or exact
retained values. It is separate from the chart mapping generation version.

## Saved shape and computation

`Mapping.presentation` is omitted for legacy mappings and after the last override
is reset. When present it has version `1` and a nonempty, bounded `columns` array
ordered by the canonical mapping columns. Each entry identifies an existing
`column` and contains `display_label`, `fraction_digits`, or both. No other
properties are accepted. Explicit zero and empty labels remain distinct from
omission. Canonical-equivalent values normalize to inheritance.

The native builder computes using the unchanged canonical `Mapping.columns`.
Only after calculations finish does it project the overlay into detached
`Output.columns`. Consequently KPI percent-delta precision, totals, coordinates,
raw cells and every derived exact value remain unchanged. The retained mapping
still includes the exact saved overlay. Frozen-output validation compares the
effective columns with that deterministic projection; recomputing a content
digest cannot authorize a different label, format or semantic column property.

Existing mappings without the extension retain their existing serialized bytes
and build behavior. Version-one legacy KPI renderers do not share all display
paths, so they expose no presentation controls. A presentation edit does not
silently migrate them to a newer mapping generation.

## Available controls

- `display_label`: inert text, at most 256 UTF-8 bytes, for visible table headers.
  It is not an axis title, legend name, category-value rename or KPI role label.
- `fraction_digits`: integer from 0 through 20, only where a bound numeric,
  non-percent column's formatter is actually consumed. This is display rounding,
  not new data precision. Synthetic KPI percent deltas are not column targets.

The SQL-free block read advertises a versioned `presentation` capability per
output, listing eligible column IDs, native display roles and fields. This is
presentation metadata, never authority. Missing or unknown metadata keeps the
new editor controls unavailable. Unsupported percent precision, arbitrary
currency reassignment, conversion and arbitrary formatter code are rejected.

## Purpose-specific edits on existing operations

The existing `block_mapping` and `block_copy` transports accept exactly one of
`mapping` or `presentation`. Both members, neither member, explicit null,
duplicate JSON keys and unknown fields are rejected. Existing mapping-only SDK
requests and methods retain their original source and wire shape. No MCP tool or
execution budget is added.

A presentation patch contains version `1` and a nonempty bounded `edits` array.
Each edit has an exact canonical `column`, an optional nonempty `set` object and
an optional nonempty `reset` list. The only set/reset fields are `display_label`
and `fraction_digits`. A field cannot be both set and reset; duplicate columns
or reset fields reject. Reset means inherit the original reviewed field.

The server clones its exact stored definition and changes only the selected
mapping's optional presentation extension. It does not rebuild bindings or
recompute output intent or amount-disclosure declarations. Mapping-only edits
preserve valid existing overlays and reject dangling or newly unsupported
overrides rather than silently erasing them. A no-op returns `invalid_request`
without creating an immutable revision.

## Authority and lifecycle

Presentation edits reuse the mapping-edit admission and commit boundary:
`charts.bind`, tenant read, exact block read/write and native private-preview,
actor, topic and dependency eligibility, plus revision/digest/head CAS. Copying
requires independent source read **and preview** and exact target, tenant,
topic and dependency write reach. There is no implicit authority from a label,
app profile or creator name. The source revision remains immutable.

For an in-place private edit, the target head CAS is rechecked atomically at
commit. For copy, the source head/version/archive eligibility and exact immutable
revision are checked when the source snapshot is admitted; target creation uses
its independent atomic create-CAS. Copy does not promise a latest-source-head
lock through target commit. A concurrent later source-head change does not
rewrite the already admitted immutable snapshot. This is distinct from current
signed authority and dependency eligibility, which retain the existing native
checks and the documented JWT freshness boundary.

An edit or copy performs no warehouse or model calls. It creates a private,
unvalidated revision without transferring source validation, attestation,
publication or retained results. Definition identity changes; execution identity
does not. Explicit fresh validation and publication remain separate native
operations. A report is not rebound or saved implicitly.

On an uncertain response, inspect the original exact target and pinned revision.
Do not allocate another copy target or automatically repeat save/validation.
Concurrent changes still require reopening and reconciliation. Metadata may not
establish which concurrent request committed, so uncertainty must remain visible.

## Persistence and qualification

Forward-only migration 088 adds closed structural validation to immutable block
definitions across mapping generations. It does not rewrite historical JSON.
Native validation remains responsible for exact role applicability, canonical
ordering, no-op normalization and deterministic rendering semantics.

The implementation must pass legacy byte/digest goldens, pointer and reset
round-trips, precision and aliasing tests, real PostgreSQL authority/CAS/copy
checks, and actual shared HTML/SVG/PNG formatting checks. JavaScript formatting
and both host adapters require separate generated-resource and browser proof.
Existing renderer host isolation, full-report format limits and real host
integration requirements remain unchanged.
