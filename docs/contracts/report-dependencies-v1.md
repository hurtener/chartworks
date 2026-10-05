# Report dependency discovery

Source contract under D-098. Pengui is the policy owner and first consumer. This
endpoint supplies native requirements so the BFF can make its independent policy
decision before minting an operation bearer. A response grants no permission.

`POST /v1/reporting/authoring/v1/dependencies` accepts a closed object containing
`kind: report|block`, exact `id`, `revision` (0 through 256), and optional `stage`.
A positive revision excludes stage; otherwise stage selects published (default),
draft, report review, or report editing. Editing resolves the current draft,
falling back to review only when the draft pointer is absent, matching native
manual reopening. It never falls back to a published revision. Unknown fields, wildcard reach, invalid identifiers and
unsupported kinds deny. The HTTP request is at most 8 KiB. The SDK method is
`ReportDependencies`; registration ID is `reportAppDependenciesV1`.

The request needs `reporting.discover` and exact target read reach. Private
revisions additionally need `reporting.preview` and exact target preview reach.
Unpublished blocks retain original-actor custody. Reports containing private
blocks retain their original block actor restriction. Reports with query widgets
are outside this manual contract and deny. No envelope is constructed or widened
to call an ordinary read, and no existing content-read check is weakened.

The response contains `version: report-dependencies-v1`, kind, ID, resolved
revision, digest, persisted publication privacy, `references`, and `blocks`.
References contain only kind/permission/ID; blocks contain only ID, parent topic,
resolved revision, digest and private-pin status. It contains no names,
definitions, SQL, schemas, result values, identities or credentials. It reads
existing tenant-composite native reference indexes; there is no dependency cache
or duplicate authority store. Native metadata access performs no source query or
model call.

Report discovery includes stored report requirements and the requirements of
each actual selected block revision. A latest-publication widget resolves the
current publication pointer, so an old observed dependency set cannot silently
stand in for a newer one. Private widget pins stay private even after that block
revision is published. There are at most 128 distinct references and 128 block
pins; excess or incomplete metadata fails without a partial manifest. Pengui's
32-scope/4096-byte issuer ceiling remains separate and may reject a smaller set.

Pengui resolves every returned reference through its current canonical
ShareGrant/Team/Project machinery, plus the independently allowed operation and
target permissions. It rechecks identity, installation and policy generations
after minting. Provider operations still verify actual dependencies at their
effect boundary: discovery is neither a safety proof nor a promise that a later
operation will succeed. Publication movement, revocation or schema changes can
require a fresh user attempt. No query is rerun to resolve a denied retained read.

This is an HTTP BFF metadata seam shared by both eventual delivery modes. It is
not an iframe method or MCP App tool; the existing 96-tool ceiling is unchanged.
It uses the normal configured Pengui verifier and HTTP audience. No credential is
sent to the frame. Deployment does not add any policy grants automatically.

Current Pengui projections consume this contract for exact SQL-free block read
and manual report reopening, with independent report read/write and App editor
admission. Private roots and pins additionally require preview. Report mutations, topic/preparation discovery, retained private-run discovery,
restricted MCP admission and the complete Builder journey remain separate work.
`TestReportAppDependencyDiscovery` and
`TestReportAppDependenciesResolveCurrentPublishedPin` exercise the real PostgreSQL
and verifier boundaries; local results are recorded in the integration review.
