# Pengui authority contract for Chartworks

Status: provider integration contract, 2026-09-04. Pengui owns authentication and authorization decisions. This document defines what the capability consumes; it does not claim all new scopes are already registered in Pengui.

## One boundary

Pengui authenticates a person/service and resolves permissions. It issues a short-lived asymmetric JWT for Chartworks. Chartworks verifies the token and enforces its signed authority. There are no local identities, roles, memberships, grants, token issuance, API-key exchange, OAuth endpoints or embed credential services.

Required verified identity fields are `iss`, intended `aud`, `exp`, `tenant`, `user`, `session`, and `scopes` (string array); `sub` agrees with `user` when present. Validate `nbf`/`iat` when supplied and a configured maximum token lifetime. The service identity and occurrence-specific session come from Pengui too, never a report body's actor hint. Normal issuer configuration supplies trusted JWKS and algorithm allowlists; token headers cannot select arbitrary key URLs.

## Scope contract

Use Pengui's existing opaque provider-scope minting capability. Chartworks operation scopes remain familiar dotted names (`topic.publish`, `query.execute`, `reporting.read`, etc.). To avoid a local grant resolver, bounded resource restrictions are also signed scope strings:

```
cw.<kind>.<permission>:<id>
```

Kinds: `source`, `dataset`, `topic`, `block`, `report`, `dashboard`, `run`, `execution_context`, `execution_binding`, `tenant`.
Permissions: `read`, `query`, `write`, `execute`, `preview`, `publish`, `certify`, `export`, `use`, `erase`.
IDs are canonical service IDs, not display names; reject reserved delimiters or ambiguous encodings. A literal `*` may replace the whole ID only when Pengui explicitly grants that permission tenant-wide. No prefix/glob/substring matching. Tenant isolation still applies. A bare `admin` or unknown scope grants nothing. Duplicate/malformed/oversized scope sets fail rather than truncate. Unknown well-formed operation scopes may be ignored with bounded diagnostics, never converted into a known permission.

A route registration defines the operation scope and required resource permission. Report-run creation, for example, requires `reporting.execute`, `cw.report.execute:<report>`, and query/execution reach for the resolved block/topic/dataset/source/context dependencies. Reading its retained result requires `reporting.read`, target read reach and `cw.execution_context.read:<recorded-context>`; it does not need query-execute authority. An exact run read scope can restrict the caller to one result; a report read scope may permit that report's eligible published results. Private previews additionally require `reporting.preview` and exact target preview reach. Creator identity alone is not an access bypass.

Creating an object checks signed write reach on its declared parent boundary: block under topic, dataset under source, source/report/dashboard under tenant. Operation scope still distinguishes the action, so parent reach alone does not permit every mutation. Publishing/certifying/exporting require their own operation and addressed-resource permission. Lifecycle, dependency and business-validation gates apply in addition; they do not create authority.

The chosen resource-scope encoding must be registered/documented in Pengui as part of phase 03/04's first consumer integration. It uses the ordinary signed scopes claim and does not require a second auth mechanism. If an existing Pengui resource-scope serializer is used instead, change this one contract and its decoder/fixtures together; do not keep parallel encodings.

## Execution context and data partitions

A source's registered execution context fixes its credentials/warehouse role or secure-view/RLS behavior and semantic source binding. A query selects it only within signed `use` reach. Context identity and effective version are recorded with the result. The service derives the actual partition from the context used; a client cannot label a broad result as a narrow partition.

Artifact reuse/read requires Pengui's signed entitlement to that actual context/partition and target. This is direct enforcement, not Chartworks deciding which teams share data. No fetch-wide-then-filter fallback. A changed context that would expose broader data is a new version/partition, not an unnoticed reuse of the old identity. Resource/presentation metadata cannot narrow an already broad result retroactively.

## Queued and scheduled work

The caller requests work with a valid JWT. An operation that may outlive it stores an opaque `execution_binding_ref` issued/authorized by Pengui, not the bearer itself. The caller needs `cw.execution_binding.use:<ref>` and all relevant operation/target authority at admission. Binding and attribution are immutable for the accepted occurrence.

An `ExecutionAuthorityProvider` is a thin client of Pengui's existing authority broker, not an issuer. At execution/retry it requests authority for `(binding_ref, operation_id, target, intended audience)`, receives a Pengui-signed JWT, verifies it with the same verifier and checks exact target/dependency scope. Requests use the configured platform connection credentials; secret bytes are never in the operation record. Scope requested from Pengui cannot be taken from arbitrary model/body fields.

The Pengui adapter's exact endpoint/request schema is wired from the existing broker contract in phase 30; absence/mismatch fails unattended execution explicitly. Do not implement a guessed compatibility endpoint or local signing fallback. Expired authority at a privileged checkpoint requires refresh/revalidation before publication/delivery. Token renewal changes authority, not the accepted revision/period manifest.

For direct short interactive operations no binding is needed if all privileged work completes within the valid token and configured budget. Accepted durable work must not rely on that coincidence. A caller disconnect does not automatically cancel a durable operation; cancellation is a separate authorized action.

## Freshness and revocation

Pengui controls new issuance/renewal. Without online introspection, an already-valid JWT remains usable until its enforced expiry (plus configured bounded skew). Do not promise immediate revocation or implement local revocation/role tables. Each new API/MCP/render request validates its supplied token. A queued attempt always obtains fresh authority; an unavailable/denying broker produces no new output under stale authority.

## Browser and Apps delivery

MCP Apps use the established Harbor/Pengui bridge. No platform bearer is inserted in UI resources or tool arguments. Iframes use an authenticated Pengui/client BFF route that forwards a scoped Pengui token server-side to Chartworks. Chartworks returns authorized HTML/SVG/data; it does not create an embed session, one-time bootstrap credential or signed URL. Other API clients obtain their credentials from Pengui too.

## Required negatives

Wrong issuer/audience/algorithm/key, invalid temporal or required identity claims, ambiguous scope encoding, unsigned actor/tenant overrides, missing resource reach, excessive claims, service-binding escalation, unauthorized partitions and expired tokens are rejected. These are tests of Chartworks' enforcement, not a reimplementation of Pengui login or a new host-compatibility certification.
