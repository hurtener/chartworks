# Phase 32 adversarial review — 2026-09-22

Reviewed against RFC-002, Phase 32, COMMON, D-079/D-082 and the rebased reporting
deletion baseline. This is an implementation self-review; independent reviewers
remain a pull-request gate.

## Findings fixed before delivery

| Attack or failure | Correction and executable evidence |
|---|---|
| Durable MCP tools were bound against an ephemeral registry | Register the same durable operation set before binding; AC07 constructs all ten real reporting bindings. |
| Composition used a 24-column approximation for the accepted 12-column catalog | Preserve exact page/widget grid cells in HTML and SVG; AC03 checks both formats. |
| Nested HTML documents and invisible raw SVG text corrupted complete pages | Extract generated body fragments, embed bounded nested SVG viewports and emit escaped SVG text. |
| A rendition migration collided with the merged deletion migration | Rebase to migration 046 and keep both exact manifest identities. |
| Document deletion could leave derivative rendition bytes | Erase exact composition-run renditions inside the existing fenced deletion transaction. |
| A text-only SVG listing overstated chart SSR | Draw actual static geometry for area, bar, column, donut, grouped bar, heatmap, line, pie, scatter, stacked bar, stacked column and treemap; table/KPI retain their dedicated renderers. The catalog regression requires geometry for every drawable kind. |
| Missing line observations could be connected across a retained gap | Split line/area geometry into separate segments at null coordinates. |
| The iframe sample trusted an inconsistent upstream envelope | Require exact format/media type/byte count/content digest, bounded single JSON response and no redirects. |
| Host credentials could cross into the renderer environment | Replace the environment with the two memory-limit variables; AC05 launches a real probe process with a credential canary. |
| Frame ancestors accepted path/query variants | Parse exact HTTPS origins in both service configuration and the client-owned BFF example. |

## Executed evidence

Focused package tests, Phase 32 AC01–AC06/AC08, race tests for rendering, a full
repository compile-only sweep, affected-package vet, changed-code lint, Linux
amd64 static renderer build, planning coherence, drift audit, mirror comparison
and diff checks passed on the committed branch. The real PostgreSQL AC07 is
implemented and mandatory. This host could not execute it because Docker
Desktop returns input/output errors for both its existing PostgreSQL data and
the local image blob store; hosted CI owns that remaining execution evidence.

## Honest boundaries

The reference Linux worker applies both Go's memory limit and an address-space
rlimit. Darwin rejects the equivalent rlimit updates, so developer runs retain
the Go limit while the production container owns the hard process limit. CPU is
bounded by one-request workers, admission concurrency and supervised timeout.

The worker executable is fixed trusted configuration and the compiled worker has
no network, source, model, bearer, URL or script seam. It is not a general code
sandbox for an operator-supplied malicious executable. PDF and PNG remain
unsupported. Rendition reads and lists recheck current artifact reach and never
execute a warehouse query or model call.
