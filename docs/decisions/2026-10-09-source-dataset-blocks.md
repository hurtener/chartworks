# Registered source datasets as report origins

### D-107 — Topic-independent blocks preserve exact source custody

Accepted implementation scope, 2026-10-09. Extends D-106 without changing
identity, access-policy, publication or execution ownership.

A client can author a chart from a registered table without manufacturing a
semantic topic. A block has one immutable parent: a real topic or a registered
source. Source-backed definitions carry an exact `source_dataset` pin containing
source, execution context, dataset, source revision and registered relation digest.
They contain no topic pins, reviewed rules, template bindings or implied semantic
approval. A physical field name never supplies business meaning.

Source-backed create/edit requires block authority and source read; it does not
mutate the registration and therefore does not require source write. Current
signed dataset and context reach remain mandatory. Preparation and subsequent
execution still require source query authority, opaque validation, native read
admission and exact source revision fences. The browser cannot supply schema,
SQL, reference grants or a replacement authority envelope.

Migration 089 adds the exclusive source parent and retains it through native
preparation cleanup. Legacy definitions and omitted JSON fields keep their prior
hashes. Published revisions remain immutable; source drift requires explicit
review rather than implicit rebase. Query result policy without reviewed semantic
classification remains unknown, never automatically nonsensitive.

This decision establishes the native lifecycle and companion authority carrier.
Paged table discovery, the table picker, physical-field filters and signed-in host
qualification remain required work under the flexible authoring plan. No rollout,
whole-product release or upload-provider qualification is implied by local tests.
