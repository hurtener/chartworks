# Phases 05/06 adversarial review — 2026-09-05

Reviewed against the phase contracts and the merged phases 01–04 baseline
`087245e2ebbf19e5581d51804c649a6b0c1fe9d9`. The interrupted implementation branch
was recovered rather than replaced. The final PR carries the code and all regression
cases; this is a self-review, not an independent external security audit.

## Findings fixed before PR creation

| Attack/failure | Correction and executable evidence |
|---|---|
| Ordinary structured completion panics when an optional assistant payload is nil | Guard the promoted optional field; all seven structured roles traverse the actual SDK in `TestPhase05/AC01`. |
| Rerank through OpenRouter is unsupported by the pinned SDK | Native Cohere route and independent key; strict role/provider validation and reference roundtrip in AC01/AC10. |
| Missing/null index or score silently becomes a Go zero | Validate the raw SDK observation before typed projection; `TestGatewayAdversarialWire` covers missing/null/duplicate keys through real SDK calls. |
| Narrowed authority reuses previously sealed candidates or an embedding cache | Hash exact sorted scopes/resources/action with tenant/user/session/context; prove no narrowed reuse before provider calls. |
| Unknown usage/cost falsely reported as zero; aggregate bill double-counted | Explicit presence-aware observation; never sum aggregate plus components, and overages consume the remaining budget. |
| Resume resets the durable occurrence cursor and loses missed windows | Preserve the cursor, then apply bounded catch-up/skip with retained outcomes; `TestJobsResumeRetentionAndIsolation`. |
| Manual runs bypass no-overlap policy | Enforce after replay lookup, so retries still receive their original receipt; SDK/manual overlap regression. |
| Queued lifetime ignores retention policy | Snapshot actual operation_hours, not a hardcoded 24h; real database assertion. |
| Abandoned-attempt cleanup is unbounded | Bound row selection and lock it before updates. |
| Worker failures disappear in ignored errors | Fixed-stage, content-free observer wired into service logging; `TestJobsObserverAndMetadataMode`. |
| Disabling dispatch removes retained metadata routes | Metadata-only construction has no dummy authority and cannot execute; real SDK/HTTP tests retain read/cancel/pause. |
| Pengui approval expires before its issued token | Refuse near-expiry issuance and check issued expiry before returning; companion producer test. |
| Incomplete dependencies leave a typed-nil store inside the assembly | Composition-root validation fails before client/worker construction; `TestWorkAssemblyLifecycle`. |

## Executed validation

Go 1.26.4, Linux amd64, PostgreSQL 17 and the actual Bifrost core v1.6.2 module.
The full `go test -race -count=1` suite passes, with real temporary PostgreSQL
schemas, migration/backup/restore tests, all ten phase 05 criteria and six phase 06
criteria. The full HTTP/Go-client path exercises all new operations, denial before
work, scope-constrained lists, revision conflicts, cancellation and offline reads.
There are 128 concurrent SDK HTTP reads and 128 actual shared model-SDK calls with
unique response associations. Cron tests cover both DST transitions and anchored
intervals. Broker tests reject redirects, wrong audience/binding/manifest/service,
expired/narrowed tokens, malformed responses and missing tenant credentials.

Statement coverage gates remain unchanged: >=85% storage/security, >=80% other
internal/SDK packages, >=70% CLI. New production packages are in the exact coverage
inventory. The final CI runs planning, dependency/format checks, build, vet, race,
coverage, named acceptance and lifecycle smoke against the committed source.
Temporary source-materialization/tooling scripts are not part of the final tree.

## Honest boundaries

No paid remote model service was called for ordinary validation. Provider fixtures
prove selected SDK wire behavior, not live quality, provider availability or pricing.
Deployment still requires separately authorized live model probes and the matching
Pengui companion. The consumer and actual-issuer tests are complementary fixture
runs, not a claim of an already-deployed two-service smoke.

Only retention effects execute, transactionally inside the metadata database.
No warehouse/email/report/export handler is advertised, and no universal exactly-once
external-effect guarantee is claimed. Published pgvector generations, reporting
artifacts and full MCP tools remain with their planned phases. Public authentication
and signed authorization are never disabled for a model or broker outage.

Already-issued execution authority has a <=30s offline revocation window. Process
input/output and pessimistic reservation bounds are not an exact tokenizer/billing
contract or a transport-level allocation sandbox for third-party SDK internals.
