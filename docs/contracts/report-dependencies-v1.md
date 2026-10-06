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
admission. Private roots and pins additionally require preview. Proposed report writes use the extension below. Topic/preparation discovery uses D-100 below; D-101 below covers retained report-run discovery;
restricted MCP admission and the complete Builder journey remain separate work.
`TestReportAppDependencyDiscovery` and
`TestReportAppDependenciesResolveCurrentPublishedPin` exercise the real PostgreSQL
and verifier boundaries; local results are recorded in the integration review.

## Proposed report writes (D-099)

`POST /v1/reporting/authoring/v1/write-dependencies` accepts a closed object with
`operation: create|save`, `id`, `expected_version`, `revision`, and `definition`.
Create requires zero version/revision; save requires a positive expected version
and exact baseline revision 1–256. The body limit is 1 MiB. The typed SDK method is
`ReportWriteDependencies`, registration ID `reportAppWriteDependenciesV1`.

The metadata seed requires `reporting.discover` and exact report write, plus exact
tenant write for create. The provider normalizes and structurally validates the
same manual definition as the real operation. Query/narrative widgets deny.
Proposed public blocks resolve native publication pointers; private pins must
match their original actor, revision and digest before metadata projection. A
private pin stays private after publication. Save also discovers its native
baseline under write authority and rejects a stale expected version. Its old
dependencies remain required even when the proposed edit removes those widgets.

The response is `report-write-dependencies-v1` with operation/ID, expected
version, base revision/digest (zero/empty for create), normalized definition
digest, references, blocks and `metadata_actions`. The last field is empty unless
governed option definitions require native semantic/source metadata reads, in
which case it is exactly `sources.read` and `topics.read`. It never includes
source query, execution or publishing. Bounds remain 128 references/block pins
and the SDK accepts at most 128 KiB. No content is returned or persisted.

Pengui verifies the server-owned allocation receipt for creation, current App
editor and independent read/write/creation policy, then every native dependency.
Private block preview is independently checked; report write alone does not grant
it. Native writes still perform complete content/binding validation and CAS under
the final bearer. A successful manifest does not promise those checks will pass.
`TestReportAppWriteDependencyDiscovery` covers native create/save consumption,
removed baseline requirements, private custody, tenant/target/input negatives,
zero source/model work and the actual registered HTTP schema.

## Reviewed data and original preparation custody (D-100)

`POST /v1/reporting/authoring/v1/data-dependencies` accepts the closed fields
`topic` (topic/version/digest), `dataset`, `new_block`, `preparation`, and
`operation`. Empty strings select the unused branch. A topic-only request may
omit the version/digest together to select the current active publication, or
pin both. A dataset, when supplied, must belong to that exact publication.
Preparation requests select an exact target and exactly one preparation or
operation. Fresh Prepare supplies the original operation, exact reviewed topic
pin and dataset; existing operation custody takes precedence over proposed pins.

The metadata seed requires reporting.discover with exact topic read, or exact
block read/write plus separate reporting.preview/block preview. Private custody
is filtered by tenant, original actor, native session and target before any
coordinates are projected. No normal content-read envelope is forged. The
provider reads only publication dependency indexes and bounded fields of the
existing preparation/consumed records; it never decodes SQL, schemas, results,
source operation coordinates, identities or credentials for this response.

`version: report-data-dependencies-v1` returns the native topic pin, selected
dataset, target, preparation/operation identifiers, complete `references` and
`query_references`. References include the entire publication, even unselected
datasets. Query references identify only the selected dataset's source query,
dataset query and execution context. Original preparation requirements survive
publication movement and native consumed-record retention. Native Inspect,
Prepare, Consume and Control still enforce their distinct full authority and
current-state requirements. Metadata never retries a physical source attempt.

At most 128 distinct references are returned; overflow rejects. Body limit is
8 KiB. Registration ID is `reportAppDataDependenciesV1`; typed SDK method is
`ReportDataDependencies`. This endpoint adds no MCP tool or schema migration.
`TestReportAppDataDependencyDiscovery` exercises actual PostgreSQL custody,
complete publication dependencies, missing roots, cross-tenant/user/session,
original operation replay and zero extra source/model work. Local results and
real-service acceptance must be reported separately.

## Saved effects and exact retained report runs (D-101)

`POST /v1/reporting/authoring/v1/effect-dependencies` accepts exactly `operation`,
`id`, `revision`. Operations are `block_validate`, `preview`, `execute`, `view`.
The first two select an exact block/report revision (1–256); the last two select
an exact run with revision zero. Discovery requires reporting.discover and exact
root read reach. Block/report roots also require explicit root preview. A run
seed carries no artifact-read action: private runs must match the original
signed tenant, actor and session before the metadata query returns any parent.
Execute accepts private report compositions only. View also accepts public
retained reports. Dynamic query groups are excluded from this manual lane.

The `report-effect-dependencies-v1` response contains operation, root kind/ID,
revision/digest, run (empty for block/report selectors), privacy, required actions
and at most 128 resource references. It contains no SQL, definitions, payloads,
rows, source credentials or bearer. Root and dependency queries share a read-only
repeatable-read transaction. Validation includes source query only for the
block's saved execution source; other reviewed datasets retain their read/query
entitlements. Preview requires independent report write/execute/preview and
block execute/preview, including published blocks used in a private composition.
Execute includes the frozen run's requirements and the native original report
read requirements. Manual execution rejects narrative outputs rather than
inferring model permission.

View uses the immutable run reference index, never a current report definition.
It maps execution targets to read, sources to read and preserves all dataset and
context reach. Private preview requirements stay private after later publication.
Pengui checks every returned reference against its canonical policy, including
the original report read and preview. It derives fresh `cw.run.read:<exact-run>`
only for that operation. Retained reads receive no source query, validate, write,
execute or model action. The iframe never receives a bearer or metadata seed.

Request limit is 8 KiB; response limit is 128 KiB. Registration is
`reportAppEffectDependenciesV1`, SDK `ReportEffectDependencies`. Source/model
work remains absent from discovery. `TestReportAppEffectDependencyDiscovery`
exercises native validation, preview, execute and retained values using only the
returned exact projection, missing-scope and cross-tenant/actor/session denials,
registered wire closure and query/model counters. This is local native evidence;
real issuer and browser acceptance are recorded separately.
