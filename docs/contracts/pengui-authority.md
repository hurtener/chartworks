# Pengui authority consumed by Chartworks

Status: integration implementation contract, 2026-09-04. Pengui alone authenticates, decides permissions and issues tokens. Newly described provider scopes/execution bindings are integration deliverables, not claims that current Pengui already exposes every proposed field/API.

## Request verification

Pengui-issued Authorization bearer -> configured asymmetric/JWKS verifier -> immutable envelope -> signed action/resource enforcement -> domain service. Verify iss, intended aud, exp and supplied nbf/iat plus configured temporal/size limits. Require tenant/user/session/scopes under the actual minter contract; sub agrees with user when present. Trust configured key locations/algorithm-key bindings, not arbitrary token-selected URLs.

No local issuer/signing key, login/OAuth, users/groups/roles/grants, service accounts, passwords/API keys, bootstrap admin or embed credential system. Metadata can establish reference ownership and business validity, not create access. A bare admin string, actor prefix or creator label grants nothing.

## Signed operation and addressed-resource scopes

Use Pengui's opaque provider-scope minting seam. Operations such as reporting.execute or reporting.publish are separate from bounded addressed reach. Proposed single representation: `cw.<kind>.<permission>:<id>`. Phases03/04 register and test it with Pengui; where a supported existing serializer already supplies equivalent authority, adopt that single contract here rather than maintain parallel encodings.

Kinds: source, dataset, topic, block, report, dashboard, run, execution_context, execution_binding, tenant. Permissions: read, query, write, execute, preview, publish, certify, export, use, erase. IDs are bounded canonical identifiers, not labels. Reject ambiguous delimiter/encoding forms. Only an explicit whole-ID `*` signed by Pengui permits tenant-wide reach; no prefix/glob/substring matching. Malformed/duplicate/excessive authority fails, never truncates.

Each operation registration specifies the action and target/parent/dependency reach it requires. Report execution checks reporting.execute, target execution and actual resolved executable dependencies/source contexts. Retained-result reading checks reporting.read, result/report read and the artifact's actual context partition, not query-execute authority. Exact run reach narrows to that result; report reach covers only eligible publications. Private previews additionally require reporting.preview and target preview reach; creator metadata never bypasses it.

Creation checks signed write reach on its parent: blocks under topics, datasets under sources, and sources/reports/dashboards under the tenant. Publishing/certifying/exporting has separate operation and addressed-resource checks. Immutable revisions, evidence/health/reference/SQL-safety checks still apply as domain validity, not independently invented identity policy.

## Actual source context

The registered context fixes credentials/warehouse role/secure-view or RLS behavior and semantic binding. Selecting it requires signed use authority. Its actual version defines the result data partition. Caller labels cannot narrow broad output after the fact; changes in exposure invalidate context equivalence and result reuse.

Reading/reusing retained values needs Pengui entitlement to the target and actual partition plus persisted preview privacy. Do not fetch broad data and filter for security afterward. No tenant-only shared cache or local team-sharing logic.

## Durable work: phase06 owns the first real adapter

Admission validates target/dependency and execution-binding use reach. Store the immutable accepted manifest, opaque Pengui-authorized binding and attribution, not token bytes. The binding cannot select a stronger identity than the signed request permits.

`ExecutionAuthorityProvider` is a thin injected client that obtains fresh Pengui JWTs at dispatch/retry/checkpoints and validates them through the same verifier. It is not a token issuer. Only server-accepted binding/operation/target/audience are sent; model/body hints cannot ask for broader scopes. Connector credentials are secret references, not embedded in job records.

**Concrete ownership:** phases03/04 wire actual claim/scope serialization; **phase06 implements the actual platform adapter and first bounded durable consumer**; phase30 reuses it for reporting targets. This corrects earlier phase-30-only wording (D-055). The source review did not establish this exact binding API as deployed. The phase06 implementation reads the actual Pengui broker/minter contract, reuses a supported operation or implements the required Pengui-owned extension, records exact schema/version/fixtures, then proves its real consumer. No guessed endpoint, parallel broker/auth service or local signing is acceptable.

Missing/refused renewal records blocked work, not ambient execution. New authority does not alter accepted revisions, parameters, occurrence window or attribution. A bounded interactive operation can finish under its valid supplied token; durable acceptance cannot rely on its token surviving indefinitely. Explicit authorized cancellation differs from client disconnect. Retained reads do not wait for a broker, warehouse or inference provider.

## Freshness and browser delivery

Offline signature validation cannot observe permissions changed after token issuance. Pengui controls lifetime/renewal/revocation. Chartworks enforces expiry plus bounded configured skew, validates each new supplied token and obtains fresh authority for queued attempts. No immediate offline-revocation promise or local revocation/membership database.

Apps use the established host bridge; UI resources/tool arguments contain no shared token. Iframe uses a Pengui/client BFF that authenticates and forwards scoped Pengui tokens server-side. Chartworks returns authorized HTML/SVG/data and issues no embed session/bootstrap code/signed capability URL. Other API consumers also obtain credentials from Pengui.

Bifrost provider credentials are separate remote-inference secrets: never accepted as Chartworks authority, copied into reports or sent to a browser. All learned-model calls use the SDK gateway; no local fallback bypasses permissions/budgets.

## Acceptance

Reject wrong issuer/audience/algorithm/key, malformed temporal/identity/scope claims, unsigned overrides, missing reach, stronger binding selection, unauthorized partitions and expired tokens. Phase06 proves the actual adapter/consumer and zero protected work on denial; phase30 adds reporting-specific target/window checks. These are Chartworks enforcement/integration tests, not a reimplementation of Pengui login or established Apps support.
