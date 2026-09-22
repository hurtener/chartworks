# CW-07 adversarial review

Date: 2026-09-22. Scope: RTE-01 governed topic choice and RTE-02 deterministic
value, geography and temporal interpretation.

The implementation review found and fixed ten reachable failures before delivery:

1. Discovery gateway calls were absent from retained usage evidence. Topic-choice
   embedding and reranking receipts now remain in the route result.
2. Interpretation could run before the caller context matched every admitted
   publication. The context fence now precedes interpretation and all provider work.
3. A constrained route loaded source bindings for unrelated datasets. Binding reads
   now cover only datasets that own interpreted constraints.
4. Multiple temporal phrases or eligible unnamed temporal dimensions could select
   by iteration order. Both cases now return typed clarification outcomes.
5. Unsupported temporal grains and timestamp dimensions without a reviewed timezone
   could be inferred. Both now fail closed before provider work; date columns use the
   timezone-independent UTC representation.
6. Unknown correction targets and temporal replacements could be silently ignored.
   They are rejected; value replacement accepts only a reviewed governed-value ID.
7. Phrase grouping used map iteration order. Sorted phrase traversal and stable
   evidence ordering now make interpretation and replay deterministic.
8. The pre-provider context budget omitted interpreted constraints. The preflight
   now includes them and rejects a largest-tier overflow before embedding.
9. Mentioning two aliases of one governed value could create duplicate executable
   constraints. Aliases now collapse to one stable target, while conflicting
   include/exclude language returns a typed clarification.
10. A partial vector batch could silently remove a competing topic from the decision.
    Discovery now requires exactly one result envelope per admitted topic and rejects
    missing, unknown or duplicate result IDs.

`TestCW07/AC01` through `AC08` exercise current authorized server selection,
confidence floors and ambiguity, bilingual reviewed values and months, geography,
correction/removal, source-revision fencing, unsupported and ambiguous spans,
cross-tenant and same-tenant/different-context denial, deterministic replay, and
the interpretation budget. Existing phase-17 acceptance retains confirmed
same-source multi-topic relationship coverage.

Focused verification at the reviewed worktree head:

```text
CGO_ENABLED=0 go test ./internal/nlqroute ./internal/nlqexec ./internal/nlqapi ./sdk/chartworks
CGO_ENABLED=0 go vet ./internal/nlqroute ./internal/nlqexec ./internal/nlqapi ./sdk/chartworks
TMPDIR=/private/tmp make planning-check check-mirror
git diff --check
```

Native `CGO_ENABLED=1` linking is unavailable on this host because the pinned Bruin
Rust SQL parser library is absent. Real PostgreSQL phase acceptance requires
`CHARTWORKS_TEST_STORE_URL`; it is not present in this worktree environment. Live
confidence calibration, broader date grammar, stress/performance, race, native,
real-store, coverage, fuzz, container and browser evidence remain unproven. D-074
assigns those checks to the manually dispatched `Final gap and release verification`
workflow at the exact final-gap commit, including `make preflight-full` and
`make release-check`; the fast pull-request lane does not substitute for them.
