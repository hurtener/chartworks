# Consolidated recovery integration

PR74 is the single integration candidate for the previously stacked recovery
changes. It targets `main`; superseded PRs retain their branches and source history.
Consolidation is bookkeeping and source integration, not a full recovery or
release qualification claim.

## Exact source inclusion

The integration includes these exact predecessor heads by ancestry:

| PR | Scope | Included head |
| --- | --- | --- |
| [63](https://github.com/hurtener/chartworks/pull/63) | Captured reports, retention, rendering and narrative evidence | `9797f3518adcc2b037600172eb5a2b7badc04706` |
| [64](https://github.com/hurtener/chartworks/pull/64) | Grouped fact-owned periods | `3a3e5095940602ec5e091b3a7670e766ba6d87c0` |
| [65](https://github.com/hurtener/chartworks/pull/65) | Strict provider schema projection | `2c52670b55fd5bf3a3605e68db2f2c66b244e2a5` |
| [66](https://github.com/hurtener/chartworks/pull/66) | Authoring calendar vocabulary | `8bc87fff035fe9bd88667733d920b363b24e1ab4` |
| [67](https://github.com/hurtener/chartworks/pull/67) | Request-bound topic review | `48e759cfd9e4a39e2c613ec1a7485ae91299e978` |
| [68](https://github.com/hurtener/chartworks/pull/68) | Provider-safe review references | `a2567f816dce7c52981e2ef9e17af544f7bb6b50` |
| [69](https://github.com/hurtener/chartworks/pull/69) | Observed live-topic evidence | `a2e99154ce28560bf2a3825443833e9d14fac251` |
| [70](https://github.com/hurtener/chartworks/pull/70) | Grounded natural-language grouping intent | `8a67130afc2f2cbbeb33afb95b0b9e1488510d62` |
| [72](https://github.com/hurtener/chartworks/pull/72) | Scalar grouping decision shape | `112d327d417a90242e898d0e15c535aa39a531e2` |

The existing PR74 head `01d500a92ca8132f5799a7f32344f8dd0dd5136a`
(tree `748cefd68d63195adb60613eb328e35742feb198`) already includes every row.
Current main `9b29954bde16dc11f3b9e35d13441d7e1bf0d24f` is merged without
rewriting any predecessor. That merge adds only the four already-reviewed
renderer CI provisioning files from main; runtime source and renderer tests are
unchanged. The independent wording cleanup is already present in both histories.
This integration note is the only additional documentation change.

## Existing qualification and remaining gates

On the prior PR74 head, both Go1.26.4 and Go1.27.1 passed 3,518 unit/subtest events
across33 packages and477 PostgreSQL acceptance events, with no failed or missing
required tests and no acceptance skips. Only the opt-in live reranker unit skipped.
The tested merge `207283749f071d9f28c5e893ec21d3188184d6e2` has the exact same
runtime tree. [Hosted SQL qualification](https://github.com/hurtener/chartworks/actions/runs/36952168543).

The prior reporting run fails only the two mandatory renderer kernel tests and
Phase32 AC03/AC04/AC05 because charged-memory isolation is unavailable on the
ordinary runner. The [trusted hosted workflow](renderer-hosted-qualification.md)
must qualify the exact current open PR74 head with its delegated cgroup; its
resource limits, test requirements and cleanup remain unchanged. The integration
must not merge by skipping or disabling those failures. Current-main CI is rerun
for the consolidated head.

The recorded and live evidence remains bounded by its original fixtures and
lanes. The live generated-topic baseline and explicitly reviewed held-out
grouping lanes passed; the automatic grouping live cohort remains incomplete.
Scoped reusable learning and cold-result ownership fixes are being qualified
separately before inclusion. General analytical exclusions, additional engines,
broader quality cohorts and final performance/release gates remain listed in the
[full recovery tracker](sql-recovery-completion.md).
