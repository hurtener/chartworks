# Team reporting product pass — local qualification

Status: locally qualified and delivered, 2026-10-09. Owning work:
[team-ready reporting](../plans/team-ready-reporting.md). This is a bounded manual
reporting increment; phase 34 migration and phase 25 release remain separate.

## Implemented behavior

- Exact provider authority grows to 128 scopes / 16,384 aggregate bytes, with
  unchanged individual size, claim encoding, issuer, audiences and temporal
  checks. App projection reserves the MCP action on both transports. Legacy
  explicit verifier limits still reject larger requests. D-105 and Pengui PD-251
  require a coordinated rollout; no grant is created by increasing capacity.
- The immutable App has a 512 KiB resource ceiling and a 448 KiB compiled asset
  budget. Its current MCP HTML is 272,529 bytes. No remote code, credentials or
  tenant content enters the resource. The small viewer remains bounded at 256 KiB.
- One confirmation can cover the entire exact revisions and all outputs of ready
  charts. Independent publication CAS operations run sequentially. Failure,
  uncertainty or navigation stops unsent operations; confirmed publications stay
  published. Rebinding, report review, report publication and execution remain
  separately chosen actions. All eligible component links can be selected at once.
- Executive review and operational scorecard starters create native editable
  pages and headings. Every chart is chosen explicitly. Suggested positions
  survive reopening and avoid existing components. There are no fabricated data
  values or saved placeholder widgets. The creation form initially hides the
  report catalog, and authors can expand the canvas without changing the report.

## Predecessor portability assessment

Two owner-provided current private source trees were inspected read-only. Exact
locations, working-tree fingerprints and source-file hashes remain in private task
evidence. No implementation, prompts, schemas, identifiers or data were copied.
These are behavior comparisons, not a claim of complete feature parity.

| Observed behavior | Chartworks disposition |
| --- | --- |
| Null-like optional formula strings must not become SQL | The manual compiler accepts stable typed column references and a closed aggregation set, with no optional formula string in its measure DTO. No blanket string rewriting was introduced. Broader imports must retain their own typed validation. |
| Group or aggregate physical columns according to actual type; roles guide defaults | A useful next authoring capability, owned by phase 29 with semantic contracts in phases 14/16. Current manual authoring deliberately exposes reviewed dimensions and measures separately. Extending this needs a versioned field capability and positive compiler/filter tests; this pass does not silently relax it. |
| Blank reports and per-widget topic authority | Blank native v3 pages remain valid. Starters use that capability. Dependency projection checks every distinct chart/topic; a twelve-chart negative with the final grant missing is refused before execution. Live reports in this pass use one reviewed topic; mixed-topic native parity is not claimed. |
| Serialized saves and recovery | Explicit CAS saves already block overlapping writes and preserve the dirty baseline on failure. The same discipline now covers batch publication, including partially confirmed and unknown outcomes. No automatic save or mutation retry was added. |
| Shared field typing, including numeric-year versus date distinctions | Keep actual typed source/retained values authoritative. A broader field-role/date-grain authoring extension remains with the compiler capability above; temporal labels alone must not become physical-type proof. |
| Exact typed rendering and filter applicability | Existing typed rendering and explicit filter bindings remain in use. Twelve real retained outputs, exact decimal evidence, separate reader denial and mobile tables are qualified. The operational starter additionally exercises an explicit date filter. |
| Selected-page loading and revision-bound results | The real two-page journeys retain exact revision identities and load selected-page values. Warm navigation is measured locally; predecessor performance targets were not copied as measured results or production SLOs. |

## Local behavioral evidence

The isolated native source has synthetic regional sales for six months, three
reviewed measures and two reviewed dimensions. Models are disabled for the live
manual journey. Two distinct reports each contain twelve independently authored
charts over two pages. Each chart was prepared against PostgreSQL, created,
validated and added through Pengui's actual App UI.

HTTP and restricted no-chat MCP both completed twelve-chart preview, chart and
report publication, return-to-edit, a new private revision, review, republication
and an explicit public run. Both ordinary-reader journeys were repeated on the
final selected-page bundle. The operational starter also created and previewed
a real two-page report with an explicitly bound date-range filter. Separate ordinary readers
received all twelve retained outputs and the exact revenue value `4374060.000`;
they could not edit, execute, read another login's private run or a guessed run.
Withdrawing the original execution context removed discovery and retained access.

An additional reader received access only through a Pengui Team and group grants.
Removing membership denied both the existing session and a fresh App admission;
restoration admitted a fresh session. This uses configured canonical audience
policy. Creation does not confer ownership or grant-management rights. Self-service
report audience management is not added by this increment; the App continues to
say when audience details are unavailable instead of inventing an owner or team.

Actual desktop, tablet and mobile reader renders were inspected. Keyboard opening
and reload recovered the current publication's retained result without execution.
Both transports show the verified layout before all selected-page values arrive.
Initial opening reads the root heading and six selected-page charts; the first
second-page visit reads its own heading and six charts. Twenty cached switches
per transport issue zero requests. No source/model operation is invoked by reading.

The local measurements below include host admission and exact authorized retained
reads. They are two report opens (initial and reload), not a production latency
study. These final reader checks used binaries built from the committed
implementation, immediately after a service restart. MCP first-open admission
was colder than the reload. The warm navigation range is twenty alternating page
selections. A narrow table check additionally reached the sixth retained monthly
row on desktop/mobile without another request; tables retain a bounded scrolling
viewport. Two HTTP response-body capture warnings affected diagnostic recording,
not required UI/state/request assertions; both readers had zero page errors.

| Transport | Layout visible | Six selected-page outputs | First second-page visit | Warm page navigation |
| --- | --- | --- | --- | --- |
| HTTP iframe | 346 / 338 ms | 571 / 564 ms | 367 / 378 ms | 57–75 ms, median 58.5 ms |
| Restricted MCP | 2941 / 347 ms | 3790 / 1183 ms | 882 / 876 ms | 51–80 ms, median 58 ms |

An earlier eager-loading build took about seven seconds to open the twelve-chart
MCP report. Selected-page loading removes the unnecessary sibling reads from
first opening and reveals the validated structure before values. Fixture timing
and service warmth differ; this is local improvement evidence, not a comparative
production benchmark.

## Bounded single-agent adversarial review

No subagents or independent reviewers were used, as the owner requested. The
review covered the issuer/projection/verifier boundary, batch consent and uncertain
custody, starter persistence, progressive rendering and late-response fences.

- Larger token capacity does not grant a thirteenth chart, another context/tenant,
  or a new action. Exact final dependency absence, byte/count overflow, legacy
  configured bounds, audience and expiration remain negative tests.
- Batch consent binds report coordinates and every eligible chart/output. Changing
  evidence, versions, outputs or the eligible set invalidates it. A failed second
  publication leaves the first confirmed and every unsent request untouched.
- Progressive reads initially exposed a swallowed asynchronous failure in the
  loader. The correction propagates denial while clearing all retained buffers;
  focused denial, overlap, close and delayed-sibling regressions pass. Saved
  revision, run, privacy, expiry, selection and aggregate-byte fences remain.
- Compiled browser checks now assert first-time sibling-page reads and zero reads
  on cached/empty pages. Old eager-load test expectations were updated to check
  the new bounded behavior explicitly, rather than removed.
- Starter pages use native DTOs and headings only. Placement respects collision
  checks; no source, grant, publication or fake result is created implicitly.

No unresolved P0/P1 was found within this increment. Broader field-role/date-grain
selection, self-service audience controls and production-scale performance remain
explicit follow-up scope, not passed parity criteria.

## Verification and finalization

Passed locally:

- 447 App unit tests, compiled asset reproducibility and App Go/race resource tests.
- 362 actual compiled-App browser assertions per transport, including responsive
  geometry, keyboard, interruption, private/public isolation and selected-page reads.
- Go race tests for identity, configuration and MCP resources; Pengui minter and
  the full App test group (`TestApp*`) covering authority, projection, effects,
  reads, writes, catalog and browser boundaries. The final broader App group
  found five legacy overflow fixtures still using the old 35-dependency input;
  the fixtures now exceed the shared provider ceiling while preserving their
  refusal/no-dispatch assertions. This correction changes tests only.
- Native Linux race acceptance for the selected Phase03, Phase29, Phase31 and
  ReportApp tests. The first combined run failed only because Node/Chrome were
  missing. After installing Node 22 and Chromium and correcting its executable
  setting, all failed Phase31 AC04/AC06/AC08 criteria passed in a focused rerun.
  The initial failed run remains in the private evidence; it is not called green.
- Exact signed twelve-chart authority acceptance passes under the race detector.
- Planning (`TMPDIR=/private/tmp make planning-check`) and generated-source
  checks; both mirrored contributor files match. The host default temporary path
  exposed a pre-existing macOS `/var` versus `/private/var` test-fixture mismatch;
  the canonical temporary directory runs the same full planning checks.

Implementation binaries were built from clean Chartworks `9c4128a0` and Pengui
`48c883ea`; both final ordinary-reader journeys passed against them. Later
qualification changes are tests/documentation only. The private task evidence
contains the complete source hashes, binary hashes, current PR heads, raw logs,
measurements and screenshot manifest. The standalone walkthrough has sixteen
embedded captures; all load with working desktop/mobile navigation, zero remote
requests and no browser errors.

The fixture completed normally after 249 live source attempts and zero additional
fixture model calls; runtime model services were disabled. All four owned Linux
services and the local TLS proxy are stopped. Fixture databases/roles were cleaned
up, container volumes preserved, and unrelated user services left running.

Chartworks #76 and Pengui #367 remain draft/open/unmerged, with unchanged runtime
companion #784. Hosted CI is billing-blocked under the owner's local-testing
instruction. This is local product qualification, not phase 34 migration or phase
25 release. Production is unchanged.
