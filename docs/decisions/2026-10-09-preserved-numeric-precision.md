# Numeric display for unknown client datasets

### D-111 — Preserve precision for new typed selections

Accepted, 2026-10-09. New typed-field selections cannot assume a client's
numeric scale. Numeric physical projections and non-count measures therefore
set optional `Format.preserve_precision: true`; retained numeric text is displayed
without rounding in both the shared browser presentation and static renderer.
Count fields keep integer formatting. This is a display default, not an inferred
unit, currency, percent scale or change to aggregation, SQL or exact result values.

The additive flag is omitted from old definitions and preserves their canonical
bytes and behavior. Prepared revisions retain their originally sealed mappings;
consumption and frozen execution do not regenerate formats from new defaults.
No migration, new compiler request version or data re-execution is required.

The native presentation overlay still controls explicit fraction digits. Zero
is meaningful even when the canonical digits field is zero: it disables precision
preservation in detached effective columns. Reset restores the canonical default.
Conflicting digits, percent scale and nonnumeric columns reject the flag.
Browser and static formatter tests, overlay/reset tests and actual uploaded-field
acceptance cover this behavior. Existing publications are never rewritten.
