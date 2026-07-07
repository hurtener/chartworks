#!/usr/bin/env bash
set -euo pipefail

# Smoke-check for Phase 24 — `eval` (CLAUDE.md §4.2 / §16 step 7).
#
# Maps every acceptance criterion in docs/plans/phase-24-eval.md to one
# ok()/fail()/skip() call. The `eval` package is the real surface here (not the
# binary), so the guard is "does the eval package build". Per-criterion go tests
# SKIP cleanly (via run_group) until they are written, so `make preflight` stays
# green as the build grows. Criterion 10 (coverage) is enforced by
# `make coverage`, not here.

PHASE="24"

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

# shared run_group helper (collapses per-criterion go-test invocations).
# shellcheck source=scripts/smoke/lib.bash
. "$(dirname "$0")/lib.bash"

# --- surface guard -----------------------------------------------------------
# The eval harness lives in the `eval` package. If it does not exist / does not
# build yet, SKIP the whole script (an unbuilt surface is not a broken one).
if ! go list ./eval/... >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no buildable eval package)"
  summarize_and_exit
fi

# --- CLI surface (binary present only) --------------------------------------
# A new/expanded CLI command ⇒ a smoke check (CLAUDE.md §4.2). The `eval`
# subcommand family (golden/redteam/gate/accuracy/seed) must be discoverable.
BIN=""
if [ -x "bin/chartworks" ]; then
  BIN="bin/chartworks"
elif command -v chartworks >/dev/null 2>&1; then
  BIN="$(command -v chartworks)"
fi

if [ -n "$BIN" ]; then
  HELP="$("$BIN" eval -h 2>&1 || true)"
  if printf '%s\n' "$HELP" | grep -q "golden" \
    && printf '%s\n' "$HELP" | grep -q "redteam" \
    && printf '%s\n' "$HELP" | grep -q "gate" \
    && printf '%s\n' "$HELP" | grep -q "accuracy" \
    && printf '%s\n' "$HELP" | grep -q "seed"; then
    ok "CLI surface: 'chartworks eval -h' lists golden/redteam/gate/accuracy/seed"
  else
    fail "CLI surface: 'chartworks eval -h' missing a subcommand (golden/redteam/gate/accuracy/seed)"
  fi
else
  skip "CLI surface: chartworks binary not built yet"
fi

# --- assertions (one per acceptance criterion) ------------------------------
# run_group SKIPs any test name not present yet, so this stays green as the
# eval package fills in. Store-backed tests (crit 1/2/8) t.Skip when the Docker
# Postgres URL is unset; live/dockerized-accuracy tests (crit 7) t.Skip when
# their prerequisites are unset — both surface here as genuine SKIPs.
run_group ./eval/... "-race" \
  "TestGateMockPathGreen|criterion 1: mock-path gate green (pass-rate>=threshold, zero criticals)" \
  "TestSeededRegressionTripsGate|criterion 2: seeded regression trips the gate (self-test)" \
  "TestFiveGoldenSuitesPresent|criterion 3: five golden suites present + executable" \
  "TestValidationCTEFixturePasses|criterion 3: standing CTE fixture passes validation" \
  "TestGenerationComparator|criterion 4: comparator (normalized/alternatives/result-hash/partial), pure" \
  "TestRedTeamSixCategoriesMinCases|criterion 5: six red-team categories, >=N each, block-first" \
  "TestFailureTaxonomyTyped|criterion 6: every non-pass case carries a typed FailClass" \
  "TestAccuracyDockerizedGrounded|criterion 7a: dockerized grounded accuracy (postgres/mysql/sqlserver, public data)" \
  "TestAccuracyLiveCloud|criterion 7b: live cloud accuracy (-count=1, live gate)" \
  "TestSeedFromPositiveFeedback|criterion 8: golden-case seeding -> candidate, never auto-promoted" \
  "TestEvalConfigFailLoud|criterion 9: eval config validates fail-loud at boot"

summarize_and_exit
