#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 20 `charts-spec` (CLAUDE.md §4.2 / §16 step 7).
#
# docs/plans/phase-20-charts-spec.md maps each acceptance criterion to one
# ok()/fail() call below. `internal/charts` is a pure, I/O-free Go package
# (RFC §10, D-026) — there is no binary, route, or MCP tool to probe, so the
# surface guard is the package itself: the script SKIPs entirely until
# internal/charts exists, keeping `make preflight` green as the build grows.
#
# Deliberately excluded from scripts/preflight.sh's *.sh loop's self-run; run
# by preflight across all phases.

PHASE="20"

OK_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

ok() {
  echo "OK: $1"
  OK_COUNT=$((OK_COUNT + 1))
}

fail() {
  echo "FAIL: $1"
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

skip() {
  echo "SKIP: $1"
  SKIP_COUNT=$((SKIP_COUNT + 1))
}

summarize_and_exit() {
  echo "SMOKE phase-${PHASE}: OK=${OK_COUNT} FAIL=${FAIL_COUNT} SKIP=${SKIP_COUNT}"
  if [ "$FAIL_COUNT" -gt 0 ]; then
    exit 1
  fi
  exit 0
}

# Resolve repo root so the guard and `go test` run from the module dir.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

# --- surface guard -----------------------------------------------------------
# The charts selector is a pure package. When it doesn't exist yet, SKIP the
# whole script rather than failing it — an unbuilt surface is not a broken one.
if [ ! -d "internal/charts" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/charts package)"
  summarize_and_exit
fi
if ! command -v go >/dev/null 2>&1; then
  skip "phase-${PHASE}: go toolchain not available"
  summarize_and_exit
fi

# Shared run_group helper: one `go test` process for the whole group, per-
# criterion OK/FAIL/SKIP recovered from -v output; a not-yet-written test SKIPs.
# shellcheck source=scripts/smoke/lib.bash
. "$SCRIPT_DIR/lib.bash"

# --- assertions --------------------------------------------------------------
# One criterion per pair "TestName|human label". -race on the group because the
# selector is a hot reusable artifact exercised under concurrent reuse (§11).
run_group "./internal/charts/..." "-race" \
  "TestDeriveColumnMetadataGolden|criterion 1: ColumnMetadata derivation is pure and I/O-free (source_ref + bucketed cardinality golden)" \
  "TestCatalogFourteenKinds|criterion 2: catalog holds exactly the 14 kinds, each with a slot contract" \
  "TestBindSlotsReclaimTiebreaker|criterion 3: slot binding honors the required-slot-reclaim tiebreaker (scatter x==y guard)" \
  "TestSuitabilityScoringWeights|criterion 4: suitability weighted from one tunable file, reconstructable from events" \
  "TestAdaptiveAlternativesReducer|criterion 5: adaptive-alternatives reducer applies all six rules" \
  "TestSelectDeterministic|criterion 6: selection is deterministic and pure (same input => same output)" \
  "TestTableFallbackNeverRaises|criterion 7: table fallback never raises (rules_fallback at score 0.1)" \
  "TestResultPresentationGolden|criterion 8: ResultPresentation envelope + golden suite over brief-06 shapes" \
  "TestNoRendererTypes|criterion 9: no renderer types anywhere (no prepared_options/ECharts, no gateway/I/O import)"

summarize_and_exit
