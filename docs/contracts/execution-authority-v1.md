# Pengui execution authority v1

Implemented pair: Chartworks `internal/jobs/pengui` and Pengui
`internal/httpserver/execution_authority.go`, `internal/executionauthority` and
`internal/minter/local/execution.go` on `feat/chartworks-execution-authority`.
This is an additive **Pengui-owned** endpoint, not a pre-existing platform feature
assumed by Chartworks. Merge/deploy that companion before enabling workers.
No Harbor change is required, and ordinary user/MCP token paths are unchanged.

## Transport and trust

`POST /exchange/execution-authority` on the trusted Pengui HTTPS backend uses
Basic authentication with an **existing Pengui vault broker** client. The broker
resolves the exact runtime/tenant; the body cannot submit identity, scope, audience,
SQL or credentials. Requests and responses are strict bounded closed JSON v1.
Redirects are refused, cookies are disabled and no caller URL can choose the sink.

```json
{"version":1,"binding_id":"maintenance","job_id":"0123456789abcdef0123456789abcdef","manifest_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
```

Successful response fields: `version:1`, `access_token`, `token_type:"Bearer"`,
`expires_in:30`, `binding_id`, `binding_revision`. Error responses are opaque;
429/5xx are transient and other refusals block the attempt. Neither body/error
nor credential values enter telemetry. There are no on-disk token caches.

## Pengui policy and issued scope

Set `PENGUI_EXECUTION_BINDINGS_FILE` to an absolute, regular, operator-controlled
JSON file with no group/world write permission. Replace it atomically for changes;
keep its parent directory operator-controlled. Pengui reads it on every request.
The file contains version 1 and at most128 unique bindings. A binding has only
`id`, `revision`, `tenant`, `runtime_id`, `capability_id`, `audience`, `operation`,
`enabled`, `not_after`. Maintenance bindings use `retention.sweep`. The scheduled-pipeline extension uses `pipeline.run` plus the exact operator-owned `pipeline` reach described below. The registered runtime
and MCP-server capability must both still be enabled. The binding tenant/runtime
must match the verified broker; an execution audience is never the ordinary
capability audience. The binding revision is a positive integer at most9999999999.

Pengui derives user `svc:chartworks:<binding>` and session `<job_id>`. Its existing
asymmetric issuer/key lifecycle signs only `ops.maintain`,
`cw.tenant.erase:<tenant>`, `cw.execution_binding.use:<binding>` and
`cw.run.execute:<job_id>`. The dedicated `:execution` audience and
`execution_version`, `execution_binding`, `execution_binding_revision`,
`execution_manifest` fields bind the token to the exact accepted manifest.
The response is refused when a 30s grant would outlive the approved binding.
Revocation is checked at each pull; an already-issued offline JWT remains usable
until its <=30s expiry. This is not an immediate-revocation promise.

## Chartworks enforcement and effects

Chartworks uses its existing Pengui JWKS verifier with the dedicated audience,
zero expiry leeway and maximum60s lifetime. `auth.Execution` is an opaque proof;
ordinary HTTP/MCP envelopes cannot be passed to the durable effect method.
The service verifies exact job/binding/manifest/session/executor and all addressed
scopes before work. The PostgreSQL commit repeats the check under the live lease,
monotonic fence and immutable policy revision. Expiration, cancellation, stale
owners, narrower reach or a different manifest cause zero committed erasure.

Only bounded metadata retention executes in this phase. Its queue completion,
attempt outcome, retention effects and audit are one database transaction. No
external warehouse/email/publication handler is registered, so this does **not**
promise exactly-once external side effects; later target implementations must
record/reconcile uncertainty before advertising those operations. The durable
initiator/session fields are attribution, never worker authorization.

## Verification

`TestPhase06/AC06` exercises the actual HTTP consumer, real JWKS verification and
PostgreSQL effects for success/refusal/missing reach/wrong binding/wrong audience/
wrong service identity/expired tokens. Companion `TestExecutionAuthority` exercises
Pengui's real vault, registered runtime/capability, strict binding loader and actual
issuer with128 concurrent calls. These are complementary fixture-backed producer
and consumer tests, not a claim of a deployed cross-service live smoke.


## Scheduled pipeline extension

Chartworks owns recurrence, occurrence admission, retries and managed execution.
The platform supplies authority only; its prompt scheduling is unrelated.
`pipeline.run` policy adds a closed `pipeline` object with `id`, `sources`,
`datasets` and `contexts`. Each list contains 1–16 distinct exact identifiers.
The issuer supplies `engineering.pipeline.run`, `sources.query`, `sources.read`,
pipeline source-write, external source read/query, dataset query and context-use
scopes, plus the existing exact execution-binding/job scopes. No tenant erasure,
publication or schedule-administration action is issued. Request/response transport,
issuer, audience, lifetime and manifest binding remain the v1 contract.

The consumer seals the pipeline ID/version/digest into each accepted occurrence.
Its pipeline effects use that occurrence's existing operation ID and lease fence;
no inner request task is admitted. An ordinary request runner cannot claim the
scheduled task, and the maintenance completion path rejects pipeline targets.
Every attempt obtains a fresh token and revalidates the exact published definition
and external data reach. Accepted due times, windows and versions survive schedule
replacement. Current issuer support is a separate companion change; enable this
target only after that provider change is available and the consumer passes its
actual native scheduled-execution tests. This document is not deployment evidence.
