# Bounded PNG rendition continuation

### D-091 — Single-output PNG preserves retained meaning and existing isolation

Status: implemented for review, 2026-10-01. Owns Phase 32 and extends D-079/D-083.
This supersedes D-083's statement that PNG is absent. PDF and paginated document
layout remain outside the implemented export matrix.

The pinned Go renderer shares a closed typed geometry scene between SVG and PNG.
PNG consumes one authorized retained table, chart or KPI; it does not parse SVG,
load URLs/fonts, invoke a browser, run SQL or call a model. Full report composition
PNG is unsupported. Existing HTML/SVG composition remains available.

Raster images visibly retain exact values, chart state, truncation and omitted-row
warnings, page scope, KPI roles, and reviewed amount disclosure. Mandatory text
that cannot fit causes a bounded failure rather than clipping or substitution.
The binary response uses canonical base64; byte count and digest describe decoded
PNG data. A strict inert chunk/decompression validator and independent encoded,
decoded, scene, glyph, viewport and pixel limits guard both worker and client paths.

The existing worker namespace, credential, deadline and hard memory rules remain.
Functional LocalProcessor tests are not process-isolation evidence. The reproduced
virtual-address compatibility issue and hosted namespace launch rejection remain
open qualification gates; this decision permits no isolation or memory fallback.

Phase 32 remains the registry owner, with its eight criteria unchanged in number;
AC08 now includes this explicit bounded format. The detailed contract and required
consumer tests are in [PNG renditions](../contracts/png-renditions-v1.md).
