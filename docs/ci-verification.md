# CI verification lanes

Chartworks separates iteration feedback from final qualification. Both lanes are
read-only and test committed source without repair or generated commits.

## Pull-request lane

`.github/workflows/ci.yml` runs for every pull request and pushes to `main`.
Its four stable jobs are `mirror`, `drift`, `build-test` and `lint`. They run in
parallel with 5–12 minute job timeouts, and a newer commit cancels the stale run.
The lane records the exact commit and covers planning/mirror drift, formatting,
script syntax, dependency integrity, focused deterministic Go tests, compilation
and static analysis. It intentionally starts no PostgreSQL, warehouse, native,
browser, renderer or container fixture.

A green fast lane means the change is ready for continued review and integration.
It does not claim race safety, cumulative coverage, real-store behavior, browser
or renderer behavior, cross-platform native output, fuzz results, full preflight,
migration readiness or release readiness.

## Final-gap and release lane

Run `Final gap and release verification` manually against the exact commit being
qualified. The dispatcher requires a reason and invokes the complete reusable
workflow set. This includes all existing commands and thresholds for:

- race-enabled unit and integration tests with real database/source fixtures;
- every implemented phase criterion and the full no-skip release gate;
- cumulative package coverage bands and bounded security fuzz campaigns;
- Linux/macOS native builds, the reference container, actual browser/viewer and
  reporting adversarial suites;
- clients, MCP, clarification, CW-03, reporting, scheduling and dashboard suites;
- full preflight, formatting, lint, build, vet and unchanged-source checks.

The final lane is intentionally expensive and does not cancel an older run. Its
run URL, selected commit and artifacts are the qualification evidence. Failed or
unrun final jobs stay failed or unproven; the fast lane never substitutes for them.

## Branch protection

At the time D-074 was filed, GitHub reported no branch-protection rule or ruleset
for `main`. If protection is enabled, require the stable fast contexts `mirror`,
`drift`, `build-test` and `lint`. Do not require manual final-workflow job names as
pull-request contexts: those checks do not exist until someone dispatches the
manual workflow and would leave every ordinary pull request permanently pending.
