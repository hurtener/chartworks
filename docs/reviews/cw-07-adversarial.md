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

`TestCW07/AC01` through `AC09` exercise current authorized server selection,
confidence floors and ambiguity, bilingual reviewed values and months, geography,
correction/removal, source-revision fencing, unsupported and ambiguous spans,
cross-tenant and same-tenant/different-context denial, deterministic replay, and
the interpretation budget. Existing phase-17 acceptance retains confirmed
same-source multi-topic relationship coverage.

## Consolidated dual-review fix round

The single post-PR fix round closed all four findings without widening the workflow:

1. Interpretation-only plans now enter the same protected business-evidence gate as
   clarification filters. Active evidence requires base SQL/parameters, a complete
   binding and validator receipt, current replay/rebinding, and exact reconstructed
   SQL, parameters and receipt equality. Ordinary execution, terminal replay and
   saved session-bound clones reject source, publication, vocabulary, parser or
   interpretation drift. Reviewed replacement rebinds; complete removal leaves no
   active predicate.
2. `timestamptz` periods now build local month boundaries in the reviewed IANA zone,
   reject missing or ambiguous midnight boundaries, and bind their UTC RFC3339
   instants. Date and wall-clock timestamp contracts keep calendar-date bounds.
3. Spanish `de`/`del` and English `of` named-month years parse deterministically.
   Missing, malformed, out-of-range or adjacent competing years clarify before any
   provider call instead of silently falling back to the server anchor year.
4. Geography is an explicit reviewed categorical-dimension field carried by compile,
   digest, enhancement, rebind and neutral import/export. Label keywords no longer
   infer geography; absent metadata omits the flag.

Focused regression evidence includes `TestCW07/AC09`,
`TestCW07InterpretationBusinessEvidencePlanRunAndDrift`, rich semantic compile,
portable round-trip, enhancement and rebind suites.

Focused verification at the reviewed worktree head:

```text
CGO_ENABLED=0 go test ./internal/semantics ./internal/semantics/drafts ./internal/semantics/rulesets ./internal/nlqroute ./internal/nlqexec ./internal/nlqapi ./internal/reporting ./sdk/chartworks
CGO_ENABLED=0 go vet ./internal/semantics ./internal/semantics/drafts ./internal/semantics/rulesets ./internal/nlqroute ./internal/nlqexec ./internal/nlqapi ./internal/reporting ./sdk/chartworks
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
