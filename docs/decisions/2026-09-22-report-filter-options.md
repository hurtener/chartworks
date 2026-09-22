### D-082 — Selectable report filter options are explicit revision-bound source reads

Accepted for REP-01. A selectable filter source is a versioned immutable report
definition field that pins an exact governed block/topic/dataset/column chain.
Option enumeration uses the common validator and read executor with current
signed target, dependency and context reach. It is never inferred from a label,
profile sample or retained artifact.

Pages use bounded keyset reads and short-lived authenticated cursors bound to the
complete request, signed authority snapshot and current source revision. Values
are not cached. Binary/structured columns and text search on non-string values
are explicit unsupported/invalid outcomes. Viewer navigation remains retained
and source-free; only the explicit filter-options operation performs this read.
