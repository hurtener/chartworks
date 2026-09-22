# CW-05 adversarial review

Scope: VIS-01 and VIS-03 only. Reviewed against `origin/main` at
`6263d1062cd8015ee95917008ba42e7bebbd6055` before final commit.

## Threat and failure review

- **Authority:** static export checks both current `reporting.read` and
  `reporting.export`, exact `cw.run.export:<run>` reach, and then the ordinary
  retained-artifact read path. A denied or foreign run does not call storage.
- **Execution boundary:** v3 frozen builds use supplied normalized rows and exact
  mappings. Static rendering has only a retained-view interface. Neither path owns
  a source, SQL, model, credential, URL or network seam.
- **Open content:** options remain closed. Locale is bounded; date pattern and output
  format are enums. CSV neutralizes formula prefixes; HTML/SVG escape labels/data;
  static HTML carries a deny-all CSP and no client JavaScript.
- **Exactness:** derived KPI values use bounded exact decimal arithmetic. Divide by
  zero omits percent delta; nonfinite/underflow drawing coordinates reject. Static
  and interactive fixtures agree on fraction digits, Spanish separators/date order,
  year-month values, long dates and currency-symbol fallback.
- **Immutability/drift:** mapping v3 joins existing definition/output digests and
  increments frozen `BuildVersion`. Mapping validation pins every bound column.
  Rebind is detached/review-required and updates comparison, target, table visibility
  and order references together. Migration 042 does not rewrite old revisions.
- **Consumer completeness:** HTTP and SDK authoring build/replay KPI and table v3.
  Reporting definition validation round-trips display intent. Production registration
  exposes export over HTTP and MCP; the generated operation catalog used by SDK/CLI
  includes it. Apps and static consumers both use retained display intent.

The self-review found and fixed four material issues before this record: export had
action checks but no exact run-export reach; KPI/table rebinding did not update new
policy references; first-row previous comparison incorrectly chose the following
row; and year-month/long-date formatting was not faithfully consumed. It also added
derived-coordinate overflow rejection and explicit percent-delta formatting.

## Verification

- Focused Go packages and `TestCW05RichDisplay` pass with `CGO_ENABLED=0`.
- `internal/rendering` coverage is 82.6%, above its new 80% band.
- Node display-intent tests and JavaScript syntax checks pass.
- `go vet` passes on changed Go packages.
- `TMPDIR=/private/tmp make planning-check`, `make check-mirror`, `make drift-audit`
  and `git diff --check` pass.
- Migration manifest identity reports version 42 in the focused store test.

The real PostgreSQL acceptance rerun is blocked by the local Docker storage failure:
the daemon first failed loading the pinned pgvector image with an input/output error;
the existing port later accepted TCP but PostgreSQL returned
`could not open file "global/pg_filenode.map": Input/output error`. No database pass
is claimed from that environment. Hosted CI must supply the real PostgreSQL gate.

## Remaining phase boundaries

CW-05 does not close Phase 32. Durable rendition rows and retention coupling,
isolated renderer worker/crash limits, full report/dashboard layout composition,
complete plot geometry, PDF/PNG and BFF/embed integration remain planned. Phase 34
still owns foreign display-intent import/cutover. Final stress, migration and
behavioral qualification remain phases 24/34/25.
