#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — Phase 26 (engineering-autopilot). CLAUDE.md §4.2 / §16 step 7.
#
# One assertion per acceptance criterion in
# docs/plans/phase-26-engineering-autopilot.md. The whole script SKIPs cleanly
# until the internal/engineering/autopilot package exists, and each individual
# criterion SKIPs until its test is present (via run_group in
# scripts/smoke/lib.bash) — so `make preflight` stays green as the build grows.
#
# This phase is the L2/L3 agentic layer over phases 12/13/15/16/21; it adds
# management-plane HTTP + SDK surfaces (NO MCP tools). The autonomy engine lives
# in internal/engineering/autopilot; the deterministic L3 policy evaluator lives
# in internal/engineering/autopilot/policy.

PHASE="26"

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

# Run from the repo root so ./internal/... package paths resolve regardless of
# the caller's working directory.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/../.."

# shellcheck source=scripts/smoke/lib.bash
. "${SCRIPT_DIR}/lib.bash"

# --- surface guard -----------------------------------------------------------
# Phase 26 owns core in internal/engineering/autopilot (+ .../policy) and appends
# routes to internal/api's route table (no new binary/CLI/MCP tool). SKIP the
# whole script until the autopilot package exists and builds — an unbuilt surface
# is not a broken one.
PKG="./internal/engineering/autopilot"
POLICY_PKG="./internal/engineering/autopilot/policy"
if ! command -v go >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not testable yet (go toolchain unavailable)"
  summarize_and_exit
fi
if [ ! -d "internal/engineering/autopilot" ] || ! go list "$PKG" >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no internal/engineering/autopilot package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# One criterion per run_group entry; -race per CLAUDE.md §11. A not-yet-written
# test surfaces as SKIP; a t.Skip'd test (store DSN unset for the integration
# criteria, mock PipelineRunner absent) surfaces as SKIP.
run_group "$PKG" "-race" \
  "TestProposalApplyAtomicAndRevertRestoresPriorState|criterion 1: apply atomic all-or-nothing + revert restores prior state as a unit" \
  "TestEveryProposedObjectHasQueryableDecisionRecord|criterion 2: every proposed object carries a queryable decision record" \
  "TestDecisionRecordShapeGolden|criterion 3: decision-record content contract (golden)" \
  "TestBlindPlannerPromptCarriesNoCatalog|criterion 4: blind planner prompt carries no catalog; gaps detectable" \
  "TestMatchResolveThenCompareFailClosed|criterion 5: resolve-then-compare matching, fail-closed (unsure ⇒ build)" \
  "TestDriftEnqueuesProposedAmendment|criterion 8: drift/freshness/quality → proposed amendment (integration)" \
  "TestGoalToMedallionRoundTripGolden|criterion 9: goal→medallion round-trip on the mock stack (golden)" \
  "TestAutopilotNeverPublishesTopic|criterion 10: no autonomous topic publication at any level" \
  "TestAutonomyScopeAndGrantGating|criterion 11: scope + manage-grant gating; cross-tenant/empty-access covered" \
  "TestAutonomyManagementPlaneNoMcpTool|criterion 12: management-plane HTTP+SDK; MCP registry unchanged (11 tools)" \
  "TestAutonomyMutationsIdempotent|criterion 13: idempotent goal-submit/approve/revert (first-write-wins)" \
  "TestAutonomyStoreConformanceAndForwardOnlyMigrations|criterion 14: store conformance + forward-only migrations"

# The D-040 adversarial probe (criterion 7) and the pure L3 policy matrix
# (criterion 6) live in the deterministic policy evaluator package — run them
# there (still one criterion per entry). SKIPs cleanly until that package exists.
if [ -d "internal/engineering/autopilot/policy" ] && go list "$POLICY_PKG" >/dev/null 2>&1; then
  run_group "$POLICY_PKG" "-race" \
    "TestL3PolicyMatrixAndLoudDegrade|criterion 6: L3 policy matrix + loud typed degrade to L2" \
    "TestNoProposalTargetsNonManagedSchema|criterion 7: D-040 probe — no proposal targets a non-managed schema (any level)"
else
  skip "criterion 6: L3 policy matrix (policy package not built yet)"
  skip "criterion 7: D-040 non-managed-schema probe (policy package not built yet)"
fi

summarize_and_exit
