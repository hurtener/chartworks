# Phase 29 development recovery — 2026-09-11

Status: interrupted implementation; **no Phase 29 runtime acceptance, adversarial-review completion, or green PR is claimed**. This note is a recovery record, not a replacement for the [owning phase](../plans/phase-29-reports-dashboards.md), RFC-002, or COMMON.md.

## Verified remote baseline and delivery state

Development started from actual remote `main` at `6f0001dbd6370dff300cab472405088d7b85fead`, which includes the merged Phase 26/28 runtime (PR #19). The working branch is `feat/phase-29-reports-dashboards`. The unrelated draft PR #18 was not modified.

The implementation files described below were written in the local workspace but were **not pushed** before the execution tools stopped responding. The remote branch contains this recovery note, not those runtime changes. The temporary `.github/workflows/phase-29-source.yml` source-transfer workflow was removed in `2b81ee0b0fa401a9d26d53fb301a06cd9971aeb0`. No PR was opened; no main merge or force push occurred.

## Last-known local workspace

The last readable workspace was `/mnt/data/chartworks`. It was unpacked from an exact tracked-source archive at `a645e8a9db739e5ed6d6f8656b84d41b5cb62b50` and initialized as a local Git repository for change tracking. That archive differs from the baseline only by the initial temporary transfer workflow. Do not overwrite the workspace without first checking for and recovering uncommitted changes.

Last-known new files:

- `internal/config/reporting_composition.go`
- `internal/reporting/documents_model.go`
- `internal/reporting/documents_definition.go`
- `internal/reporting/documents_service.go`
- `internal/reporting/documents_proof.go`
- `internal/nlqexec/saved_question.go`
- `internal/store/postgres/migrations/029_reports_dashboards.sql`
- `internal/store/postgres/documents_read.go`
- `internal/store/postgres/documents_write.go`

`internal/config/reporting.go` was also modified. Local formatting was performed, but compilation, migration execution, race tests and acceptance tests were not completed. After the failure, neither the container nor either Python execution tool could re-read these files. Their current availability is therefore unverified, and there is no recoverable implementation patch on this remote branch.

## Local implementation direction — unverified

The draft introduces bounded report/dashboard definitions, closed block/query/text widget unions, typed filter bindings, safe presentation fields and non-mutating legacy-section read projection. Independent draft/review/published pointers, immutable revisions, CAS updates and tenant-composite references are handled through a shared document service and PostgreSQL repository. Creator/audience labels do not grant preview access.

The proposed `Documents` service uses the existing reporting block service for reference validation, not a new query engine. A private, encoded `PreparedDocument` proof binds mutations to the supplied verified envelope. Dashboard page references are authorized in SQL before being returned. Metadata listing avoids raw result reads. Session-bound query references retain origin actor/session/context and SQL/parameter/topic digests without returning the underlying SQL or result data.

These are implementation intentions and last-known local code structure, **not passing acceptance evidence**.

## Remaining implementation

The report/dashboard execution consumer, accepted immutable run manifests, selected-output union deduplication, dynamic execution adapters, retained widget outcomes/privacy, and their PostgreSQL run storage are not complete. Reuse `jobs.RequestRunner`, the existing reporting `Runs` service and the existing NLQ validator/executor; do not introduce another execution engine or queue.

Reserve the outer request key before resolving floating references. Resolve aliases, typed parameters and named-timezone periods once. Deduplicate only exact revision/parameter/context/policy semantics, execute the union of saved outputs, and preserve each widget's selected subset and provenance. Parent publication must neither certify dynamic SQL nor disclose earlier private previews. Dashboard execution should be the same report composition path over ordered exact page revisions.

Dynamic replayable planning is not inherently idempotent: inspect the current `nlqexec.Plan` implementation before composing it. Persist an appropriate planning/uncertainty boundary and do not blindly regenerate SQL after a lost response. Session-bound execution must use the originating authorized actor/session and fail explicitly when unavailable; never impersonate that origin.

HTTP registration, composition-root wiring, SDK/client parity, concrete examples/configuration documentation, Phase 21 cumulative coverage and the eight `TestPhase29/AC01` through `AC08` acceptance subtests remain to be implemented. Keep the existing phase criteria and coverage thresholds; do not claim completion from planning checks alone.

## Review items identified for the next pass

Recheck expiry at all repository-return/commit boundaries, including quarantine/import paths. Ensure session-query topic aggregation cannot silently drop a missing pin. Confirm dashboard read/write/execute page filtering checks the required action as well as resource reach. Exercise lock ordering against concurrent publication, query updates and archive. Validate the restricted Markdown policy against the promised import semantics rather than silently reducing source meaning.

Test private-preview-to-public transitions, root-versus-child authority, cross-tenant and same-tenant/different-context denial, zero-visible-page responses without hidden names, concurrent floating publication, output-subset fan-out, strict/partial outcomes, missing sessions, disabled live execution, cancellation and lost responses. Metadata summaries must not fetch retained values. These review items are not substitutes for the requested completed adversarial review.

## Execution environment and source-transfer evidence

The original local container had Go 1.23.2 and no external network access; the repository pins Go 1.26.4. GitHub access worked through the authorized connector. A read-only temporary Actions workflow built the pinned toolchain/dependencies and transferred tracked source. Its successful jobs prove environment preparation only, not the new implementation.

Relevant preparation runs:

- `34643907814`: exact tracked-source archive; artifact `10280704673` (`phase-29-source`).
- `34644121455`: pinned toolchain, locked modules, native parser/Bruin and PostgreSQL/pgvector fixture bundle; artifact `10282026016` (`phase-29-offline`).
- `34645042517`: unchanged bundle split into bounded artifact downloads.

The single offline bundle exceeds the connector's 512 MiB download ceiling. Split artifacts are `10281807228` (aa), `10281672357` (ab), `10282211530` (ac), `10281957091` (ad), and `10281957100` (ae). Extract each ZIP and concatenate the `offline.part-*` entries in order to restore the tar stream; stream the files rather than holding the whole bundle in memory. These preparation artifacts have short retention and may expire. The included source archive is the original source, **not** the uncommitted implementation.

Downloads aa through ad were reported as mounted under `/mnt/data`; ae was not downloaded. The local execution service began returning `TransportTimeoutError` and then `InvalidArgumentError`, including for a bare health-check command. Do not infer a test pass, a preserved patch, or a successful extraction from artifact-download success.

First establish readable execution and recover the workspace. Finish implementation, run the real acceptance/coverage/parity suites, perform and fix the adversarial review, and only then open a ready PR on a green exact commit. Final CI must remain read-only and contain no source-repair or temporary transfer workflow.


## Superseding continuation — 2026-09-12

The interrupted state above is historical. Runtime changes were recovered and committed through `3a4028987e6ffae863ccbb28c5ad7de08631a1a1`, then completed and regression-tested in the continuation described by [phase-29-adversarial.md](phase-29-adversarial.md). That record and the current PR's exact-source checks supersede the earlier unverified-workspace and remaining-implementation statements; the earlier environment-transfer runs remain preparation evidence only.
