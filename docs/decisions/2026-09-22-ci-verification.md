### D-074 — Fast pull-request checks and explicit final-gap qualification

Date: 2026-09-22. Status: accepted by owner directive. Supersedes only the
ordinary-per-PR scheduling part of D-009; its coverage bands, full acceptance,
drift, security and release requirements remain unchanged.

Pull requests run a bounded exact-head gate with stable `mirror`, `drift`,
`build-test` and `lint` check names. It checks planning and mirror coherence,
format and script syntax, dependency integrity, focused deterministic tests,
static analysis and compilation without starting database, native, browser,
container or warehouse fixtures. Stale pull-request runs are cancelled.

The manually dispatched `Final gap and release verification` workflow owns the
complete qualification suite. It preserves the race-enabled real-database tests,
named acceptance, coverage thresholds, fuzz campaigns, native platform matrix,
container and browser checks, adversarial reporting suites, cumulative preflight
and no-skip release gate. A fast green pull request is implementation feedback,
not evidence that these final gates passed. Final qualification belongs to the
exact selected commit and its retained workflow evidence.

Branch protection should require only the four stable fast check names. Manual
final workflow jobs must not be configured as per-PR required contexts because a
manually dispatched workflow does not create checks on every pull request.
