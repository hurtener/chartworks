# Phase 32 adversarial review — 2026-09-22

Reviewed against RFC-002, Phase 32, COMMON, D-079/D-083 and the rebased reporting
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
| Disabled rendering still mounted an in-process static fallback | Register and mount no static endpoint unless the mandatory managed worker is enabled. |
| The worker parent trusted a partial/spoofed response | Bind request and response with a canonical digest, then recompute content digest and validate the entire rendition envelope, projection and safe syntax. |
| Host credentials/files/network/processes could cross the renderer boundary | Replace the environment with four bounded renderer variables and use a fresh Linux user/mount/network/IPC/UTS/PID namespace plus empty chroot. AC04/AC05 cover hostile input, symlinks, flags, credentials, crashes and limits. |
| Composition limits and height were per-widget/request metadata only | Enforce one aggregate deadline and widget/input/output budgets, then return the computed SVG height. |
| Grouped/stacked/heatmap output only had catalog markers | Group bars by category and series, accumulate separate positive/negative stacks, and map heat cells to retained X/Y axes; geometry tests assert exact coordinates and stack totals. |
| The iframe sample trusted an inconsistent upstream envelope or unauthenticated browser | Authenticate a client-owned browser binding before token lookup, require the fresh token to match that binding, and validate the returned static envelope. |
| Frame ancestors had unused Chartworks configuration | Remove it from Chartworks and keep exact HTTPS parent parsing in the client-owned BFF where the header is emitted. |
| Ordinary artifact expiry could leave derivative rendition bytes | Delete exact run-owned renditions in the frozen/composition expiry transactions and exercise both ordinary retention paths. |
| Full SVG composition omitted tables and returned requested height | Render table/KPI widgets through the same worker and bind rendition height to computed page geometry. |
| Linux acceptance selected the non-Linux development mode | Select `linux_namespaces` under the Linux build tag and build static worker/probe executables so process tests cross the real namespace/chroot boundary. |
| The empty chroot omitted system zoneinfo | Embed IANA tzdata and assert non-UTC date rollover plus the repeated DST hour through the real worker protocol. |
| Lexical event checks allowed whitespace/entity/case evasions | Parse HTML and SVG and enforce explicit element/attribute/CSS allowlists; adversarial tests cover casing, whitespace, entities, event handlers, scripts and external URLs. |
| Durable creation inherited read-only audit metadata and retention had no per-record receipt | Append content-free `rendition.created`/`rendition.expired` audit events transactionally and advertise the actual effect. |

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
Each request also runs from a copied statically linked worker in an empty chroot
with fresh user, mount, network, IPC, UTS and PID namespaces. Production isolation
fails closed on unsupported platforms; the development mode is explicit and is
used only by local process-boundary tests.

The worker executable is fixed trusted configuration and the compiled worker has
no network, source, model, bearer, URL or script seam. It is not a general code
sandbox for an operator-supplied malicious executable. PDF and PNG remain
unsupported. Rendition reads and lists recheck current artifact reach and never
execute a warehouse query or model call.
