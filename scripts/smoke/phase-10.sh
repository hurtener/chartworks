#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 10 (exec-read). CLAUDE.md §4.2 / §16 step 7.
#
# Maps each acceptance criterion in docs/plans/phase-10-exec-read.md to one
# ok()/fail()/skip() via run_group (scripts/smoke/lib.bash): the present tests
# run in one `go test` process, absent ones SKIP (the surface isn't built yet),
# and integration criteria that t.Skip when the Docker Postgres URL is unset
# surface as SKIP. So `make preflight` stays green as the build grows.

PHASE="10"

OK_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

ok()   { echo "OK: $1";   OK_COUNT=$((OK_COUNT + 1)); }
fail() { echo "FAIL: $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
skip() { echo "SKIP: $1"; SKIP_COUNT=$((SKIP_COUNT + 1)); }

summarize_and_exit() {
  echo "SMOKE phase-${PHASE}: OK=${OK_COUNT} FAIL=${FAIL_COUNT} SKIP=${SKIP_COUNT}"
  [ "$FAIL_COUNT" -gt 0 ] && exit 1
  exit 0
}

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

# shellcheck source=scripts/smoke/lib.bash
. "$(dirname "${BASH_SOURCE[0]}")/lib.bash"

# --- surface guard -----------------------------------------------------------
# Phase 10 lives in internal/exec (execution half). Until that package exists,
# SKIP the whole script rather than fail it — an unbuilt surface is not broken.
EXEC_PKG="./internal/exec"
if [ ! -d "internal/exec" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/exec package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# Integration-tier criteria (1,2,4,7,8) t.Skip when the Docker Postgres URL is
# unset; run_group surfaces that as a genuine SKIP. Run integration under -race.

run_group "$EXEC_PKG" "-race" \
  "TestExec_ReadOnly_RejectsForgedWrite|criterion 1: write forged past validator rejected at the engine (defense-in-depth)" \
  "TestExec_Timeout_ServerSide_PgStatActivity|criterion 2: over-budget query killed server-side (pg_stat_activity)" \
  "TestExec_Cap_PreservesOrderBy|criterion 4: ORDER BY preserved under cursor-level capping (regression)" \
  "TestExec_ContextCancel_Stops|criterion 7: context-deadline cancellation stops execution" \
  "TestExec_Idempotency_NoReExecute|criterion 8: idempotency key short-circuits — zero re-execution"

# Unit-tier criteria (no external dependency).
run_group "$EXEC_PKG" "" \
  "TestExec_RowCap_ClampsToCeiling|criterion 3: row cap clamps to ceiling regardless of caller input" \
  "TestExec_NoWriteSurface_Architecture|criterion 5: internal/exec exports no write surface (P1c)" \
  "TestExec_RequiresValidatedSQL|criterion 6: execution requires ValidatedSQL (unforgeable outside the validator)" \
  "TestExec_ResultPreview_OrderAndCap|criterion 9: QueryResult -> ResultPreview shaping is order-preserving and capped" \
  "TestExec_Metrics_Exported|criterion 10: exec metrics exported on /metrics (telemetry conformance)" \
  "TestExec_Config_FailLoud|criterion 11: exec config validators fail loud on bad values"

summarize_and_exit
