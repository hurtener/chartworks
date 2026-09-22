# CW-03 final adversarial follow-up

Scope: BLK-01, BLK-05 and BLK-07, following PRs #24 and #26. The owner merged
PR #24 before its last repository-wide CI finished; PR #26 subsequently delivered
the retry-cap, output-locale and retained-catalog corrections. This follow-up is
reconciled with main `0b29587a3d9fab9532c8cc65207e065b5c5fea11`, including
CW-01, CW-02, CW-04, CW-06, PR #26 and the
fast/manual CI split. It does not rewrite those
merged implementations or restore their former automatic heavy workflow triggers.

## Final historical CI disposition

Run [35120624081](https://github.com/hurtener/chartworks/actions/runs/35120624081)
failed in **Bounded security mutation fuzz campaigns**:
`FuzzActualVerifier`, after 25 executions, returned `context deadline exceeded`
when its five-second campaign ended. The actual downloaded log archive, rather
than a truncated diagnostic, is the evidence. Race/integration/recovery coverage,
implemented phase acceptance, build/vet, container execution and compiled smoke
had passed; the subsequent full preflight and clean-tree steps were skipped.
The focused CW-03, reporting, client and MCP runs were green, but that was not an
all-green final merge gate. No schema-reflection race or HTTP helper deadlock is
established by the verified archive.

Read-only evidence run
[35232138496](https://github.com/hurtener/chartworks/actions/runs/35232138496)
retained the original logs and exact review checkout in artifact
`cw03-final-review-source` (10502041607). Verified archive SHA-256:
`7b6955f5c29ee96bee41150da53dd042284772fbe55b75904c32d31985ed0896`.
The nested original CI ZIP is
`ed47578714b019825a17511cfd1c2c41a01639d5c646f3baa8e476e181bb9410`.
The temporary snapshot workflow is removed from the delivered tree; validation
uses the existing read-only workflows and committed source.

## Concrete findings and fixes

| Finding | Correction and executable evidence |
|---|---|
| The API substituted the block's fallback locale before looking at output-level translations. An English-only block could suppress an authored Spanish output label. | PR #26 already corrected this on main and added `TestCW03OutputLocaleIndependentOfBlockLocale`. Reconciliation keeps that implementation and removes this branch's overlapping delivery change/test instead of creating a second locale rule. |
| Migration 035 limited locale tags to 35 characters although the existing domain accepts canonical tags up to 64 bytes. A valid legacy-to-v2 candidate could fail persistence. | Forward-only migration 041 follows the merged clarification, rule-snapshot and template-evidence migrations 036–040 and aligns the storage byte ceiling without rewriting definitions or migration 035. `TestCW03OutputLocalePersistenceBounds` exercises native migration, publication/export and over-limit refusal. `TestCW03PopulatedV2LocaleUpgrade` installs real schema 35, populates an immutable v2 revision, proves the original constraint rejects the supported extended tag, runs all intervening production upgrades, and verifies unchanged original JSON/digests and immutable-update rejection. The existing populated-v1 upgrade remains required. |
| A report using a floating block with no remaining enabled defaults lost the authorized disabled/omitted choices, even though the direct block description preserved them. | Preserve the resolved metadata-only selection alongside `output_selection_empty`; never choose a replacement or execute it. The real publication-drift test checks report description, disabled choices and zero additional warehouse/model work. Explicit invalid selections still have no fabricated snapshot. |
| Two security fuzz gates had a five-second wall-time campaign that could end while the race-instrumented worker was completing a case. | The manually dispatched final-core workflow uses 256-iteration campaigns with a separate three-minute hang timeout and the same two workers. The fast PR workflow remains bounded and does not pretend to execute fuzzing. No verifier behavior, seed, assertion, race flag, linter, coverage threshold or failure handling is relaxed. The remaining native/parser/renderer fuzz gates keep their existing bounds. |

## Review of the complete reporting contract

The follow-up traced definition validation/migration and SQL-authorized native
export; publication and accepted revision pins; `ResolveOutputSelection`, frozen
proof/checkpoint validation and display/execution ordering; query-limit
intersection and result/reuse checks; compiled reviewed sensitivity and narrative
allowlist/redaction before serialization; bounded structured claims and durable
reservations; composition groups and scheduled retry pins; retained reads,
privacy, context reach and consumer selectors. The new semantic fields on current
main remain owned by their existing shared contracts.

Existing tests remain mandatory for unknown/conflicting sensitivity, attempted
declassification, manual redaction, provider hard bounds, one-query fan-out,
legacy compatibility, private/expired artifacts, same-tenant/different-context
separation, lost replies, uncertain attempts, cancellation and signed commit
fences. The corrections do not add query/model work to frozen or retained paths,
change accepted run identity, widen authority, add an issuer or create a new
client contract. No clarification or chart-binding internals are changed.

## Validation identity and remaining qualification boundaries

The follow-up PR records the final exact head/test-merge tree, workflow URLs,
actual results and reviewed fixes. A passing historical SHA is not attributed to
new source. The automatic fast lane covers planning/mirror, formatting, static
analysis and focused deterministic compilation/tests. The manually dispatched
final-gap/release lane owns CW-03 and strict phase 27–31 acceptance, real
PostgreSQL, native driver, browser/provider fixtures, client/MCP/reporting suites,
race/coverage/fuzz and the no-skip release gate. A fast green check is never
reported as full qualification. Local native Go execution is blocked because the
pinned Rust parser build requires unavailable `rustup`; planning/mirror and the
fast deterministic commands remain separately reportable.

No live commercial-provider quality/cost, cloud-warehouse, stress, foreign
cutover or phase-25 release certificate is implied. Sensitivity remains
conservative at reviewed query-dependency scope, not invented expression lineage.
Returned-byte/row caps and timeouts are not warehouse scan/cost or physical
cancellation guarantees. No merge, force-push, tag, deployment, notification
integration, identity change, table styling or broader gap closure is included.
