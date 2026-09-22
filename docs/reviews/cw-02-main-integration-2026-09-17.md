# CW-02 main integration review - 2026-09-17

This bounded author review reconciles PR #22 head
`9fabc5d782c556c80bd8502df68109909e6289c2` with main
`2219fa29093253e0c51b94c4b9de3a4e52f19ee1`. It is not independent approval.

## Resolutions and integration findings

The merged tree retains the complete current reporting output-policy implementation,
including immutable v1 compatibility, v2 authored intent, sensitivity constraints,
accepted selection and query limits, alongside the rich chart contracts and fixes.
Both scoped documentation dispositions and phase continuations are preserved.

The shared viewer previously used `omitted` for two different concepts: omitted
source rows in chart transformations, and outputs not included in a retained run.
The chart label now has a separate `omittedRows` key. Real component assertions
check English and Spanish omitted-output labels while retaining the rich drawing,
exact-value, authority and no-implicit-execution checks from both branches.

Frozen reuse identity retains `charts.BuildVersion` and all current reporting
selection, query-limit, result-policy and effective-limit pins. The lifecycle
assertion checks that full identity and still rejects the legacy scalar builder
identity. The rich lifecycle fixture now authors a v2 definition through the
existing detached SDK migration; its default execution omits the output selection
rather than using the legacy helper's explicit empty list. Production validation
continues to reject an explicit empty v2 selection.

## Validation and attribution

The exact integrated tree was reconstructed using the common Git ancestor and a
hash-verified reviewed patch. Before storing its merge objects, the hosted branch
synchronization job passed Go 1.26.4 race tests for `./internal/charts`,
`./test/chartfixtures` and `./web/report-viewer`, JavaScript syntax, Go formatting,
diff checks and `make planning-check`. Chart-core statement coverage was 91.7%;
embedded-viewer Go package coverage was 100%, not browser JavaScript coverage.
The chart fixture package has no direct package tests; it is consumed by the
acceptance and browser suites. The v2 execution request was subsequently corrected
to use omitted selection and must pass the normal committed-source acceptance run.

Temporary synchronization helpers are removed from the final tree. They did not
update any branch themselves. Final branch advancement is non-forced, and normal
read-only CI validates the pushed source separately. No passing full CI, database
acceptance or browser run is inferred from the preparation job. Assess the exact
final PR head's checks; prior green jobs do not certify a later commit.

No coverage threshold, workflow gate, source authority, publication requirement,
query/model budget or scalar compatibility contract is weakened. Main is not
updated and the PR is not merged by this work.
