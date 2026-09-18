# CW-02 committed-source CI follow-up — 2026-09-18

This bounded author review starts at PR #22 head
`40aa6452671d502ef531439ff59b327075987c1a`, already integrated with main
`2219fa29093253e0c51b94c4b9de3a4e52f19ee1`. It is not independent approval.
The preceding merge resolutions, output-policy implementation and viewer timing
fix are preserved. No main merge, forced branch update or gate waiver is made.

## Verified failure and fix

The `race-coverage` artifact from CI run `35278803488`, artifact `10523677125`,
contains **5759 / 6856** covered PostgreSQL statements. The actual coverage-gate
parser computes **83.999416569%**, below the existing **84%** threshold even though
the diagnostic rounds it to 84.00%. Every other package meets its band in that
profile. A failed suite cannot be certified by coverage alone.

The exact source archive was checked against Git tree
`542249f4ca4ecdd46c3983d8974b4915e786b8a5`; its tree is byte-identical to the
reviewed head. A retrieved job-log excerpt named a handler/test absent from that
source, so it was not used as justification for changing request authority or
adding a speculative concurrency patch. The separate source-pinned reporting API
diagnostic lacked the required native parser library and did not complete tests.
The ordinary repository CI includes the native build and remains the final gate.

`TestCW02RichCharts/saved-lifecycle/stored-integrity` adds actual PostgreSQL and
HTTP/SDK regression coverage for a detached unmodified rich revision, malformed
repeated bindings, reordered measures with an old digest, changed source pins,
a mismatched execution digest, and malformed retained labels. The tests verify
empty results on invalid repository reads, correct public failure classes, no
partial malformed catalog, unchanged original revision bytes/identities, and
zero source/model calls while handling retained data. Each fault is inserted into
a new disposable test revision; existing immutable rows, triggers and publication
gates are never disabled or edited. Tests assert that each fault changes an
existing serialized field, rather than silently testing an absent JSON path.

This closes missing persistence-decoder regression coverage instead of reducing
coverage bands, rounding comparisons or changing production validation behavior.

## Validation at authoring

- Go 1.26.4 `go test -race -count=1 -cover ./internal/charts
  ./test/chartfixtures ./web/report-viewer` passed before this test-only change:
  chart-core 91.7%, embedded-viewer Go package 100%. The fixture package has no
  package-local tests; these percentages are not browser JavaScript coverage.
- Coverage-gate unit tests, formatting, diff checks and `make planning-check` run
  locally. Planning coherence is not runtime acceptance.
- New real-driver tests must pass on the newly pushed head in normal CI, with
  actual PostgreSQL, native parser, browser and strict cumulative coverage gates.
  A fresh successful whole-repository run is not inferred from these local checks.

The review uses synthetic data only and makes no live-model, performance,
rendering/export or expanded chart-variant claim.
