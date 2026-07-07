#!/usr/bin/env bash
set -euo pipefail

# Smoke checks for phase 12 — engineering-profiling (CLAUDE.md §4.2 / §16 step 7).
#
# One assertion per acceptance criterion in
# docs/plans/phase-12-engineering-profiling.md. The whole script SKIPs when
# internal/engineering hasn't been built yet, so `make preflight` runs cleanly
# as the build grows phase by phase. Individual criteria whose Go test does not
# exist yet SKIP via run_group's -list guard.

PHASE="12"

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

# Shared run_group helper (collapses N per-criterion `go test -run` calls into
# one process, recovers per-criterion OK/FAIL/SKIP). Sourced, never executed.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/smoke/lib.bash
. "${SCRIPT_DIR}/lib.bash"

# --- surface guard -----------------------------------------------------------
# Phase 12 owns the internal/engineering package (no new binary surface). If the
# Go toolchain or the package is absent, the surface isn't built yet — SKIP the
# whole script rather than fail it.
if ! command -v go >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no go toolchain)"
  summarize_and_exit
fi

if ! go list ./internal/engineering >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (internal/engineering absent)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# One entry per acceptance criterion; run_group SKIPs any test not present yet.
PKG="./internal/engineering"

run_group "$PKG" "-race" \
  "TestProfileGoldenShape|criterion 1: golden profile shape" \
  "TestSamplingCeilingNoFullScan|criterion 2: sampling ceiling, no full-table scan" \
  "TestFreshnessBuckets|criterion 3: freshness buckets across fixtures" \
  "TestSchemaDriftFlagsDependents|criterion 4: drift diff flags dependents" \
  "TestQualitySixDimensions|criterion 5: six quality dimensions, domain-named" \
  "TestValueFamilies|criterion 6: value families for low-cardinality text" \
  "TestLargeTableHint|criterion 7: large-table date-bound hint" \
  "TestProfileVersioning|criterion 8: profile versioning appends, retains prior" \
  "TestProfileScopeShortCircuit|criterion 9: empty-access short-circuit, no adapter read (P1a)" \
  "TestProfileSummaryOptionalRedacted|criterion 10: profile_summary optional + redacted" \
  "TestProfileRunFailsLoud|criterion 11: fail loud on adapter read error" \
  "TestProfilingConfigDefaults|criterion 12: engineering config defaults + validation"

summarize_and_exit
