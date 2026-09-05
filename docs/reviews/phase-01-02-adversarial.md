# Phase 01–02 adversarial implementation review

Date: 2026-09-05. Scope: first Go implementation, metadata SQL/migrations, command/lifecycle/configuration, observability and operator archive handling. This is the implementer's adversarial self-review requested before opening the PR, not an independent human security certification.

## Findings addressed

| Attack or failure | Correction | Executable evidence |
|---|---|---|
| Empty DSN could fall back to ambient libpq settings | Reject blank/oversized DSNs; require explicit secret reference and supplied value | Config negatives and PostgreSQL open error tests |
| Failed configuration/driver/output errors could echo a credential | Static safe field/container errors, driver category mapping, closed event/label vocabulary | Phase 01 AC01/AC03/AC05 and store safe-error corpus |
| Unknown/duplicate/null JSON could silently override authority settings | Bounded closed typed decoding and duplicate detection, including public-key documents | Configuration fuzz seeds and key-validation negatives |
| Key fetch redirected to another endpoint, private/symmetric key accepted, or stale success extended on failure | Configured HTTPS only, redirect rejection, public-key structure/type validation and hard last-success freshness deadline | Phase 01 AC02 and TLS key endpoint tests |
| Retention removed an idempotency record, allowing the old key to execute again | Preserve expired-key tombstones and return explicit expiry | Phase 02 AC05 `checkOperationExpiry` |
| Lease expired during deletion, or an old worker committed after reclaim | Check the fence and expiration at final transactional commit; rollback all data/audit on failure | Real PostgreSQL delay trigger and reclaim tests in AC05 |
| CAS loser left a revision/audit or advanced pointer after audit failure | Pointer, immutable revision and audit share one transaction with deferred composite reference | 20-writer race and injected audit failure in AC03 |
| Same-tenant actor or foreign tenant claimed another operation | Tenant + actor + operation + fence predicates; composite audit references | AC02 and AC05 |
| Changed retention settings broadened the meaning of accepted work | Pin revision in the manifest, lock/check current revision before effect | AC05 policy-change regression |
| Green tests skipped actual storage/recovery | Missing test DB or client tools fails; disposable real PostgreSQL databases; actual dump/restore | Both named phase suites, AC06 archive round-trip |
| Archive connection treated a URI as literal database name | Parse the explicit URI into bounded supported libpq environment fields, never argv | First real recovery failure reproduced, fixed, then round-trip verified |
| Backup overwrote a concurrent file or restore cleaned an existing database | Private temp file, atomic no-replace link, explicit empty-target requirement, no `--clean` | Archive unit tests and AC06 negative/positive restore |
| Test infrastructure inflated/ignored coverage | Instrument production packages across the real full suite; weighted statement totals and strict package inventory | Coverage-gate regression suite and CI package results |

## Deliberate limits

No business endpoint, authentication/token validation, scope decision, live warehouse query, inference, report, scheduler dispatcher or viewer is implemented here. The in-process storage scope is not claimed to authenticate a user. Bifrost remains the only permitted future production inference path; no model download or provider call is introduced.

The trusted operator controls database credentials, migration ownership and archive files. Database-superuser tampering and untrusted PostgreSQL archives are not sandboxed. Restore requires exclusive control of an empty target. Logical backups do not include cluster roles or promise point-in-time recovery; production backup/RPO procedures must be chosen by the operator.

Key-health validation is deliberately not a second JWT implementation. The actual verifier and protected metrics/admin transport belong to later phases. The foundation's loopback restriction prevents accidental public business exposure; it is not authentication.

## Verification record

The PR will name the exact final tested commit and hosted CI run. Named acceptance results, package coverage, vet/lint, strict formatting, cross-builds, planning coherence and benchmarks are recorded by the workflow. Passing these phases must not be reported as completion of the other 32 planned phases. No merge, deployment or release tag is part of this work.
