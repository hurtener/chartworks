#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 17 (nlq-routing-context). CLAUDE.md §4.2 / §16 step 7.
#
# Each acceptance criterion in docs/plans/phase-17-nlq-routing-context.md maps to
# one ok()/fail()/skip() below. The whole script SKIPs cleanly until the
# internal/nlq package exists, so `make preflight` stays green as the build grows.

PHASE="17"

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

# Shared run_group helper (collapses N per-criterion `go test -run` calls into one
# process, recovering per-criterion OK/FAIL/SKIP from -v output).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/smoke/lib.bash
. "${SCRIPT_DIR}/lib.bash"

REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$REPO_ROOT"

# --- surface guard -----------------------------------------------------------
# The routing/context surface is the internal/nlq package. Until phase 17 lands it,
# SKIP the whole script rather than failing it (an unbuilt surface is not broken).
if [ ! -d "internal/nlq" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/nlq package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# One entry per acceptance criterion. run_group SKIPs any test not yet present, so
# a partially-built package still surfaces per-criterion progress.

# Criteria 1-8, 10-13: the standard (non-race) suite in one process.
run_group "./internal/nlq/..." "" \
  "TestRoutingEligibilityMatrix|criterion 1: eligibility = published ∩ healthy ∩ granted" \
  "TestSpanHintsModelFree|criterion 2: span hints model-free (D-028)" \
  "TestFacetRetrievalTypedTenantScoped|criterion 3: typed k-per-type tenant-scoped retrieval" \
  "TestOneCalibratedConfidence|criterion 4: one calibrated confidence, example weight as prior" \
  "TestTierTokenBudgetsGolden|criterion 5: per-tier golden token budgets" \
  "TestCardCapsBeforeTierPruning|criterion 6: card caps applied before tier pruning" \
  "TestPruneNeverMutatesSource|criterion 7: pruning never mutates source" \
  "TestOneTokenizerCurrency|criterion 8: one tokenizer-backed budget currency" \
  "TestNoRouteClarifyTyped|criterion 10: no_route/clarify are typed, never empty" \
  "TestQueryContextEnvelopeStrategy|criterion 11: stable envelope + strategy discriminator + provenance" \
  "TestNLQConfigFailLoud|criterion 12: malformed nlq.* config fails loud at boot" \
  "TestRerankConfigGated|criterion 13: rerank gate off ⇒ byte-identical, on ⇒ reordered, timeout ⇒ loud fallback (D-043)"

# Criterion 9: shared-router concurrent reuse under the race detector.
run_group "./internal/nlq/..." "-race" \
  "TestRouterConcurrentReuse|criterion 9: router safe under concurrent reuse (-race)"

summarize_and_exit
