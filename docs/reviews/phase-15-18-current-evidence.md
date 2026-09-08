# Phases 15–18 and 21 — current delivery evidence

Status snapshot: 2026-09-08, integration head `8db3ed58a4803beb8421a39bfbd468eebc1e44fd`.
This is the current cross-phase evidence index for the active plans. It records
verified slices and remaining gates; it does not change the registry, mark any
phase shipped, or claim final CI, merge, live provider quality, or release
readiness. Historical bounded evidence keeps its original date and exact head.

The registry correctly keeps phases 15, 16, 17, 18 and 21 `in_progress`. The
phase 25 full-release gate remains unimplemented.

## Current dispositions

### Phase 15 — topics lifecycle

The original review of `8291e84cf5bcb04c16ecbbae6cbf3d8e561d9d43` and the
forward migration repair `d103ba95af8a4f951b0a4d2589292ec269905a67` are cleared
by their narrow follow-up review. The later unresolved-rebind correction
`6884f23126dbf01e45c2d799e8f10dfee03c2955` is integrated, and root's exact
semantic checks passed. Root's committed-source Phase 02 and Phase 15 strict
runs passed all six children with zero skips, including health and enhanced
rebind. The bounded evidence remains in [the topic draft record](phase-15-topic-drafts.md)
and [the publication record](phase-15-topic-publication.md).

Phase 15 still awaits final integrated coverage, full lint/preflight, hosted CI
and release integration. Recorded Bifrost fixtures do not measure live semantic
quality. Its approved 84.5% exception remains limited to `internal/store/postgres`.

### Phase 16 — rules and clarification

The reviewed lifecycle and evidence slice is integrated through `cdbafc35`, with
the final local lint cleanup at `72a27bd`. Root's strict Phase 16 run passed all
six children with zero skips, and the readiness/lifecycle checks passed. The
narrow follow-up review of the Phase 16 fixes is clear. The test-only coverage
addition `192a8bb508197b06f0693bf5d2b8ba6590e2ee20` passed the native real-PG
race run in 9.340 seconds and is integrated at the current head.

The current cumulative `8db3ed5` suite passed all runtime tests and every other
coverage band, while `internal/store/postgres` measured 3010/3588 (83.89%), below
the approved 84.5% band. `internal/semantics/rulesets` measured 153/190 (80.53%).
The known lint cleanup is integrated at `72a27bd`; full-repository lint and
preflight remain final gates.
Root must verify the final coverage contribution from `192a8bb` and the remaining
Phase 18 core changes in the exact committed archive. Phase 18 remains the real
consumer of invalidation effects; Phase 16 stays `in_progress` pending the final
cumulative coverage, review, hosted CI and release gates. See [phase 16 evidence](phase-16-rules-evidence.md).

### Phase 17 — routing and context

The first routing review's two P1 fixes are recorded at
`ba65fa7b731a69182a8d93be86d5c5b86130b93f`. The follow-up review is clear, and
root's strict Phase 17 acceptance at `492c9fbf6f7fc7f6fe2f5659d2499afdbb3409b`
passed all six children with zero skips. The actual route, HTTP/SDK and runtime
OpenAPI checks are included in that disposition. The [routing evidence](phase-17-routing.md)
retains the exact relationship, metric ambiguity and sealed-context assertions.

Phase 17 remains `in_progress` for final integrated coverage, lint/preflight,
hosted CI and the downstream Phase 18/release gates. Recorded Bifrost responses
are deterministic fixtures, not live semantic-quality measurements.

### Phase 18 — generation and execution

The public HTTP/SDK execution surface landed at `6801a28acbd728578ab1f3329a06bff4fc830f76`;
root's strict Phase 18 acceptance passed all six children. The real invalidation
consumer landed at `c88dbcc`, and the author acceptance plus root's full `37f713c`
suite passed its six criteria. The public transport fixes at `459b91d6990fca2d0676bae6d5e0a76bdc81c3be`
were integrated as `ecd08fa`.

The final Phase 18 review record at `37f713c` contains six P1 and two P2 findings;
the core-fix worker is still addressing those findings. Therefore the public
surface and invalidation runtime are verified slices, not a Phase 18 closure.
The [runtime evidence](phase-18-nlq-runtime.md) records the retained-pin and
stale-evidence behavior; this index records the later public-surface disposition.

### Phase 21 — HTTP prerequisite

The Phase 21 narrow fix at `2395f8cd15a2bf6f23c78ee512c81810c8a1226d` is clear in
follow-up review. Root's strict verification at
`95e3be00df6efa36af076532aa455e6b978a4c3f` passed all six Phase 21 criteria with
zero skips, including composed registry/OpenAPI and actual HTTP/SDK boundaries.
This closes the bounded prerequisite slice while the phase remains `in_progress`
with later domain consumers and release gates still pending. See [Phase 21
evidence](phase-21-source-registry.md).

## Cumulative gate boundary

The current integration contains the verified Phase 15–18 and 21 slices, but no
phase is marked shipped. The full cumulative runtime/coverage result must be
repeated on the exact final source after the outstanding Phase 18 core fixes.
The known `internal/store/postgres` coverage shortfall above is not rounded,
lowered, or treated as a pass. No hosted final CI, merge, live cloud
qualification, or semantic-quality claim is made by this document.
