#!/usr/bin/env bash
set -euo pipefail

# Smoke-check for phase 18 — nlq-generation-execution (Wave 5).
# CLAUDE.md §4.2 / §16 step 7. One ok()/fail() per acceptance criterion in
# docs/plans/phase-18-nlq-generation-execution.md ("Smoke checks" section).
#
# This phase's surface is exercised through Go tests in internal/nlq and
# internal/exec (there is no new binary/route/tool). run_group (lib.bash)
# SKIPs any criterion whose test is not present yet, so the script stays
# green as the build grows phase by phase.
#
# Excluded from scripts/preflight.sh's *.sh loop only by being invoked there
# explicitly; safe to run standalone.

PHASE="18"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

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

# shellcheck source=scripts/smoke/lib.bash
. "${REPO_ROOT}/scripts/smoke/lib.bash"

# --- surface guard -----------------------------------------------------------
# Phase 18 lives in internal/nlq (generation, precedence, plan/run services,
# feedback/learning) and internal/exec (the semantics-aware allowlist stage +
# bounded repair). If neither package source exists yet, the surface is not
# built — SKIP the whole script rather than failing it.
if [ ! -d "internal/nlq" ] && [ ! -d "internal/exec" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/nlq or internal/exec package)"
  summarize_and_exit
fi

NLQ_PKG="./internal/nlq"
EXEC_PKG="./internal/exec"

# --- assertions: internal/nlq -------------------------------------------------
if [ -d "internal/nlq" ]; then
  run_group "$NLQ_PKG" "-race" \
    "TestPlanRunGolden|criterion 1: plan→run golden round-trip on the mock stack" \
    "TestGenerationSchemaConstrained|criterion 2: sqlgen generation is schema-constrained" \
    "TestPrecedenceResolution|criterion 3: one precedence resolution function (thresholds)" \
    "TestRepairValidationBounded|criterion 7: bounded validation repair, typed terminal" \
    "TestRepairExecutionBounded|criterion 8: bounded execution repair, typed terminal" \
    "TestPlanScopeCannotExecute|criterion 9: plan-scope caller cannot reach execution (P1b)" \
    "TestRunIdempotency|criterion 10: run idempotency short-circuits, no second execution" \
    "TestLearnPositiveSurvivesRestart|criterion 11: learn-positive DB-first survives restart" \
    "TestExamplesLifecycle|criterion 12: examples lifecycle + paraphrase dedup, audited" \
    "TestNLQAdversarialCrossTenant|criterion 14: cross-tenant NLQ probe, concurrent-safe generator"
else
  skip "criteria 1,2,3,7,8,9,10,11,12,14: internal/nlq not built yet"
fi

# --- assertions: internal/exec ------------------------------------------------
if [ -d "internal/exec" ]; then
  run_group "$EXEC_PKG" "-race" \
    "TestAllowlistIntersection|criterion 4: D-021 intersection (not_granted vs not_in_topic)" \
    "TestJoinReachability|criterion 5: join reachability against declared graph" \
    "TestSemanticsValidatedSQLUnbypassable|criterion 6: ValidatedSQL only after full walk" \
    "TestInjectionCorpus|criterion 13: injection corpus rejected with typed codes" \
    "FuzzValidateAllowlist|criterion 13: fuzz seed corpus — never panics, never passes a write"
else
  skip "criteria 4,5,6,13: internal/exec not built yet"
fi

# --- config assertion ---------------------------------------------------------
# The three new config keys must appear in the example config (CLAUDE.md §4.2).
EXAMPLE_CFG=""
for c in config/example.yaml config/config.example.yaml docs/config.example.yaml examples/config.yaml; do
  if [ -f "$c" ]; then
    EXAMPLE_CFG="$c"
    break
  fi
done

if [ -z "$EXAMPLE_CFG" ]; then
  skip "config: example config not present yet (nlq/exec repair keys)"
else
  MISSING=""
  for key in example_weight_threshold example_similarity_threshold self_repair; do
    if ! grep -q "$key" "$EXAMPLE_CFG"; then
      MISSING="$MISSING $key"
    fi
  done
  if [ -z "$MISSING" ]; then
    ok "config: nlq/exec generation-execution keys present in $EXAMPLE_CFG"
  else
    fail "config: missing key(s) in $EXAMPLE_CFG:$MISSING"
  fi
fi

summarize_and_exit
