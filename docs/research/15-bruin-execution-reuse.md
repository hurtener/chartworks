# Bruin reuse for warehouse execution

Status: exploration draft, 2026-09-07. This report does not supersede D-036, change an active phase, or authorize implementation. Phase 13/14 production work remains held while the adoption boundary is evaluated.

## Question and current boundary

Could Chartworks reuse Bruin for reads across PostgreSQL, MySQL, SQL Server, BigQuery, Snowflake and Databricks instead of implementing six complete native adapters?

D-036 currently adopts pinned Bruin v0.11.666 only as the SQL-only data-engineering write-path CLI behind `PipelineRunner`. It explicitly keeps NLQ reads on Chartworks native adapters. The shipped PostgreSQL 17 adapter remains qualified and should not be discarded by an exploration result.

Chartworks' existing read contract is stricter than "run SQL": only the validator constructs a nonzero opaque plan, and execution rechecks the signed tenant/source/context/dataset reach against the exact source revision before credential access. The adapter must also prove the effective data partition, apply read-only/session controls, enforce typed parameter and result semantics, bound rows/encoded bytes/time/planner cost, journal physical attempts, cancel only the original owned query, and reconcile uncertain termination without replay. `exec.RemoteQuery` currently encodes a PostgreSQL PID, backend start and tagged transaction; it is not yet a cross-engine control type.

## Verified upstream observations

The original D-036 pin is [Bruin v0.11.666](https://github.com/bruin-data/bruin/tree/v0.11.666) at `e54f5f5e2c09f88ba7049c572178246000779d0c`. The current inspected tag is [v0.11.749](https://github.com/bruin-data/bruin/tree/v0.11.749), not a `v0.66.6` or `v0.74.9` release. Findings below come from the tagged public source, bounded package inspection and the prototype ledger; they are not inferred from Bruin's broad product claims.

Across `pkg/postgres`, `pkg/mysql`, `pkg/mssql`, `pkg/bigquery`, `pkg/snowflake` and `pkg/databricks`, v0.11.749 query paths still return `[][]interface{}` or `QueryResult.Rows [][]interface{}` and collect/append the complete result before returning. That is useful execution reuse, but it does not establish Chartworks' cursor-level row and encoded-byte limits.

Parameter behavior is not uniform. `query.Query` has `Query`, `Args []any` and `Annotations`. PostgreSQL and SQL Server use `Args` in `Select`, but their `SelectWithSchema` paths omit them; MySQL omits them in both inspected paths. The inspected BigQuery, Snowflake and Databricks paths execute rendered query strings without a common argument contract. A Bruin-backed adapter therefore cannot claim portable typed binding without per-engine proof or a narrow upstream/bridge extension.

Schema/result normalization is also insufficient as a common contract. The inspected schemas expose native type-name strings plus generic Go values; MySQL explicitly converts every `[]byte` value to string. Chartworks must independently preserve exact integer/decimal values, temporal rules, binary values, duplicate labels, JSON encoding and typed nulls across all engines.

BigQuery and Snowflake obtain native query IDs, but the inspected logging layer exposes them through logging/context rather than a generic returned durable control handle. BigQuery has a context-cancellation helper. A shared six-engine cancel-and-reconcile API, read-only session proof and bounded streaming result interface were not established in the bounded inspection. `DryRun` is optional: BigQuery exposes referenced tables, while other engines may return opaque explain rows. None of these observations proves Chartworks' positive dependency/function validation.

Bruin's CLI-first product includes pipeline execution, materializations, quality checks and lineage ([upstream overview](https://github.com/bruin-data/bruin)). Stock CLI query output materializes a result for presentation; it is not evidence of streaming admission or authority-bound cancellation. At inspected v0.11.749 source commit `e4ac0114`, `cmd/fetch.go` creates `logs/queries` under the repository root, modifies `.gitignore`, and writes raw SQL plus every result row to a mode-0600 query log; the directory is mode 0755. `BRUIN_QUERY_LOG_FILE` adds aggregate logging and does not disable those per-query files. A CLI wrapper would therefore require an isolated private ephemeral working tree or a purpose-built command before it could meet Chartworks' SQL/result-retention rules.

The same command path indexes `result.ColumnTypes[i]` without first proving matching lengths, while the inspected MySQL `SelectWithSchema` path returns empty types. The unmodified v0.11.749 CLI prototype reproduced an index-out-of-range panic in `cmd/fetch.go`; v0.11.666 has the same reachable shape. This makes the stock fetch command unsuitable as the normalized multi-engine read boundary without an upstream fix and version-specific regression test.

The universal connection manager has 170 direct imported package paths across its Go files. Corresponding inspected leaf-package totals are PostgreSQL 33, MySQL 28, SQL Server 27, BigQuery 45, Snowflake 39 and Databricks 31. `go list -deps` measured 419 packages for a PostgreSQL leaf import, 399 for MySQL and 411 for SQL Server; cloud and universal-manager counts were not completed and are not estimated. All six leaf implementations keep their underlying driver handles private. An external streaming wrapper would therefore require changes in Bruin or would recreate the connection with the native driver, removing most of the proposed read-path reuse.

## Work that Bruin cannot remove

These controls remain Chartworks-owned for every option:

- Pengui JWT verification, signed action/resource enforcement, tenant isolation and content-free audit.
- Server-side source/context lookup, secret custody, immutable revision binding and effective partition proof.
- Positive dialect validation and native safety proof before issuing the opaque plan; Bruin parsing, dry-run or execution cannot mint a plan.
- Per-engine read-only credentials/session settings, statement/function/dependency restrictions and supported-version qualification.
- Exact parameter binding, normalized schema/value encoding, row/encoded-byte/time/cost limits and no-value-on-uncertainty behavior.
- Durable attempt identity, engine-specific cancellation ownership, reconciliation and explicit retry semantics.
- Capability matrices, real PostgreSQL/MySQL/SQL Server fixtures, recorded plus applicable live cloud evidence, safe errors and lifecycle/pool shutdown.

The current PostgreSQL-shaped `RemoteQuery` must become an engine-tagged, opaque control union before any non-PostgreSQL backend can satisfy the same executor contract. That change is required whether the underlying client is a Bruin leaf package or an official SDK.

## Options

| Option | Reuse gained | Chartworks implementation still required | New or concentrated risk |
|---|---|---|---|
| Native official SDK adapters | Existing PostgreSQL contract and direct access to each engine's strongest cursor, types, IDs and controls | Five new adapters plus shared control union and every common invariant above | Largest repeated connector work; six dependency/lifecycle implementations |
| Stock Bruin CLI | Connection syntax and broad query execution with process-level timeout/kill; strongest reuse for already-adopted SQL pipeline writes | Validation/context proof before launch, sealed input/output protocol, exact typing/limits, query-ID capture, cancellation/reconciliation and per-engine conformance | Full buffered output, automatic raw-SQL/all-row query logs, repository mutation, subprocess uncertainty, broad CLI surface and versioned output stability; process death does not prove warehouse query termination |
| Bruin leaf APIs in-process | Per-engine connection/query code without CLI serialization | Same Chartworks contract plus upstream changes for streaming, args, typed results and controls; private handles otherwise force native reconnection | Hundreds of transitive packages enter the trusted core; leaf APIs are not a stable cross-engine contract; upstream changes couple directly to service builds |
| Narrow Bruin worker bridge | Isolates the same partially demonstrated leaf-extension reuse behind a versioned RPC/stdio protocol | Chartworks authority/plan admission remains in core; extension still needs complete typed streaming and controls | A new supervised component/protocol, crash handling, secret transfer and backpressure |

## Preliminary decision shape

Do not route all warehouse reads through the stock CLI. The currently verified interfaces do not close typed streaming, consistent arguments, durable control/reconciliation or positive safety proof. Do not replace the qualified native PostgreSQL adapter merely to make the architecture uniform.

The stock CLI cannot satisfy the current read contract unchanged. The remaining viable comparison is among native official SDK adapters, a pinned narrow Bruin leaf extension linked into Chartworks with strict build/race isolation, and that extension isolated behind a worker bridge. The extension may modify or fork the permitted pin to expose typed streaming, parameter binding and engine controls; it need not wait for upstream to publish those interfaces. Chartworks would still retain authority, plan admission, limits, attempts and audit. No source metadata, grant, publication or pipeline-policy authority should move into Bruin.

No read architecture is selected yet. Direct import counts and transitive package counts describe dependency shape, not engineering savings. The prototype must measure how much connector code a narrow extension actually reuses, the patch/maintenance surface it adds against the pin, worker protocol and lifecycle cost, and how that compares with native adapters. Seven touched files or a successful fixture alone is not a size or maintenance estimate.

The first leaf-extension prototype is positive feasibility evidence. An 85-line change across three new Go files added an original callback visitor around the public PostgreSQL/MySQL leaf behavior. Its synthetic tests passed exact fixture values, `Args` binding and callback row caps on both engines. An independent `CGO_ENABLED=0 go test -v -count=1 .` rerun passed in 6.250 seconds. This shows a narrow pinned extension can reuse some connection/query substrate without importing the universal manager or routing through the CLI.

It is not yet a safe adapter. The visitor's byte count excludes JSON/schema overhead and allocates the first oversized row before rejecting it. Read-only behavior came from restricted fixture credentials rather than verified per-session setup. The prototype has no close/rotation seam, remote control identity or reconciliation. Its unsigned maximum fixture does not implement Chartworks' signed-integer normalization policy, and its exact-value coverage is narrow. In the timeout case MySQL remained active until the query ended naturally at 5.152 seconds. These are acceptance gaps to design, not reasons to expand the exploratory patch indefinitely.

Adoption should require a real PostgreSQL parity control and one new-engine proof, preferably MySQL because it is locally reproducible. Both must demonstrate exact values/types, truncation without full materialization, parameter binding, timeout, owned cancellation, post-disconnect reconciliation, read-only enforcement, dependency proof and secret-safe lifecycle. Compare measured module/binary/process cost and code surface against the official-SDK adapter; do not estimate engineering hours. SQL Server and live cloud engines were not tested in this exploration; phase 14 acceptance still owns their eventual evidence.

## Prototype evidence

The prototype ran both pinned v0.11.666 and current v0.11.749 in a private temporary Git repository because the CLI requires a Git root and creates query logs. PostgreSQL JSON output preserved an int64, a 38-digit decimal, microsecond timestamp text and base64 binary in the tested result. That demonstrates those fixture values survived this one output path; it does not establish the full cross-engine normalization contract. The current pin is eligible for a future decision; continued comparison against v0.11.666 is not required by this report.

The unmodified v0.11.749 MySQL JSON fetch panicked when its empty `ColumnTypes` met returned values. The same code shape exists in v0.11.666. On the independent v0.11.749 rerun, the five-row `--limit 2` probes passed in 6.283 seconds for PostgreSQL and 7.184 seconds for MySQL. Both engines also emitted more than 1 MiB for a single large cell despite `--limit 1`, confirming that the flag is a row/display limit rather than Chartworks' encoded-byte ceiling. Earlier 15-second timeouts did not recur and remain an unexplained environment/startup observation, not evidence of a persistent limit defect.

Cancellation behavior varied. The first probes observed the native server query still active immediately and 300 milliseconds after the Bruin client process exited for PostgreSQL and MySQL. On the independent rerun PostgreSQL reported inactive, while MySQL remained active at both observations. This proves stock process termination does not reliably provide the required cross-engine termination evidence. It does not establish how long any query continued beyond the last observation, guarantee PostgreSQL cancellation, or rule out an engine-specific cancellation extension.

No BigQuery, Snowflake or Databricks credentials were used, and no live cloud-engine behavior is claimed. The completed direct-import and `go list -deps` measurements are reported above; remaining dependency counts are unknown. The compiled harness reported 57 module paths including the harness and Bruin, and a 78,956,594-byte test executable. The current CLI binary measured 242,370,354 bytes versus 235,545,842 bytes for v0.11.666. A Go test executable is not comparable to either production binary, so these are separate artifact measurements on the prototype host, not a production-size ratio, deployment-memory result or maintenance-cost estimate.

Host-local synthetic evidence is retained under `/tmp/chartworks-bruin-prototype`: `cli-report-root749.json` records the independent CLI rerun, while `harness/probe_test.go` and `harness/stream_test.go` define the leaf-extension checks. These paths are not durable repository evidence and contain local fixture artifacts. Any adoption proposal should first preserve a sanitized, independently runnable harness without copied upstream source or credentials.

The reproducible harness command is `CGO_ENABLED=0 go -C /tmp/chartworks-bruin-prototype/harness test -v -count=1 .`. That build uses Bruin's upstream parser stub, so it does not test parser validation. The default CGo build requires the Rust static parser library and currently fails to link because that library is missing. The original harness visitor counts canonical payload strings, not the complete serialized Chartworks envelope, transport bytes or driver memory. Query IDs, owned cancellation/reconciliation, close/rotation and authority validation are absent.

Host-local evidence checksums at report time:

| File | SHA-256 |
|---|---|
| `README.md` | `86526fce889af81870cbb3bf8a2506d194c5b49fd7bfdb19af0693d5ba3f4013` |
| `stream-extension.patch` | `c7c9e6e19e3a34e1bca3fce80a10f3c6a135b59b7912bcf6a22c29449f8199cd` |
| `prototype-metadata.json` | `faeeab030edb77c3c788ed308db5e8566b364e8c9c7e9a50e66807cc203df195` |
| `cli-report-749.json` | `e69cbf90ba252052e1656539ad98e5058896a3ad9a3fe265d3732a47e8019727` |
| `cli-report-root749.json` | `b1dabdf231213a85d5a7b7a02d6b15fb3de2ac8f52143a1b8702fb4bc8cc9e2a` |
| `inprocess-unmodified.log` | `2c7a2022e8c18ed0c885d382564f316a550720210964b864632c8f4cdf4b296b` |
| `inprocess-patched.log` | `5197b3c719e575055a183278c780d0a90585af774d92504b04ffa9dbd0ea587d` |

This draft makes no adoption decision. D-036 remains the active write-path-only Bruin boundary while the narrow-extension comparison is incomplete. Any broadening requires completed evidence, an explicit superseding decision and coordinated updates to RFC-001, phases 13/14, configuration/build matrices and acceptance criteria.
