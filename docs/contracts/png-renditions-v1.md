# Bounded PNG renditions

PNG exports one retained table, chart, or KPI output through the configured isolated renderer. Full report composition export is not part of this format. Current source and report authorization are checked through the same retained-view service as other renditions; exporting never reruns SQL or calls a model.

## Content and identity

The JSON response has `format: "png"`, `media_type: "image/png"`, and `content_encoding: "base64"`. `content` is canonical standard base64. `bytes` and `digest` describe the decoded PNG bytes. `rendering.ContentBytes` validates and decodes the envelope for binary HTTP delivery. Historical text formats omit `content_encoding` and retain their existing byte identity.

The supervising service validates signature, chunk structure, CRCs, exact dimensions, a closed RGB/RGBA format, and bounded scanline decompression. Ancillary chunks, trailing bytes, malformed compressed streams, interlacing, and excess decoded rows are rejected. Both the encoded response and decoded content must fit their configured byte bounds.

## Drawing contract

SVG and PNG share a closed internal geometry scene. No SVG parser, browser, network fetch, external image, or system font is used by PNG. Embedded Go fonts provide deterministic text. Unsupported glyphs, missing text space, or a page too large for its viewport cause an explicit failure.

The existing fourteen chart kinds retain their typed mappings and exact formatted labels. PNG reserves a visible legend area and does not depend on SVG hover titles. At most 128 exact point labels fit within the bounded scene, and every requested table row and total must fit; export never silently crops a requested page. Amount disclosure must also fit visibly, with its evidence origin and query or visible-row scope preserved. Empty/no-value states, truncation, omitted-row warnings, table page bounds, chart titles, and KPI value/comparison/target roles are visible text in the image. They do not rely on hidden SVG attributes or hover titles.

The viewport is bounded by 4096 pixels per axis and 16,777,216 total pixels. Scene node, point, text, raster, encoded-byte, and elapsed-time bounds are independent. Mandatory amount disclosure is part of the authorized retained projection and cannot be dropped to make an image fit.

## Qualification boundary

Functional raster tests do not qualify a deployment's process isolation. The existing hard address-space limit remains unchanged. A deployment that cannot start the bounded worker fails closed; no in-process production fallback is added. The separately tracked worker virtual-memory compatibility issue remains a release gate until resolved and qualified.
