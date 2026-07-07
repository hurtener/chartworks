#!/usr/bin/env bash
set -euo pipefail

# Smoke checks for phase 06 — jobs-scheduler (internal/jobs).
# CLAUDE.md §4.2 / §16 step 7. One ok()/fail()/skip() per acceptance criterion
# in docs/plans/phase-06-jobs-scheduler.md ("Smoke checks" table).
#
# The whole script SKIPs cleanly until internal/jobs exists; individual
# integration criteria SKIP when the Docker Postgres test URL is unset (their
# Go tests t.Skip, surfaced as SKIP by run_group). Deliberately excluded from
# scripts/preflight.sh's own runner (which runs every scripts/smoke/*.sh).

PHASE="06"

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

# --- surface guard -----------------------------------------------------------
# The jobs queue is a library, not a binary command — guard on the package
# existing and being listable rather than on bin/chartworks. Before the phase
# lands (no go, no package), SKIP the whole script.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if ! command -v go >/dev/null 2>&1; then
  skip "phase-${PHASE}: go toolchain not available"
  summarize_and_exit
fi

if ! go list ./internal/jobs >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no internal/jobs package)"
  summarize_and_exit
fi

# shellcheck source=scripts/smoke/lib.bash
. "$(dirname "${BASH_SOURCE[0]}")/lib.bash"

# --- assertions --------------------------------------------------------------
# One entry per acceptance criterion. run_group collapses them into a single
# `go test -race` process, recovers per-criterion OK/FAIL/SKIP from -v output,
# and SKIPs any criterion whose test is not present yet (growing build stays
# green). Integration criteria whose test t.Skip's (store URL unset) surface as
# SKIP automatically.
run_group ./internal/jobs "-race" \
  "TestClaimSingleDeliveryConcurrent|criterion 1: single delivery under concurrency (-race)" \
  "TestReclaimAfterKilledWorker|criterion 2: reclaim after a killed worker" \
  "TestReclaimRespectsMaxAttempts|criterion 3: reclaim bounded by max_attempts" \
  "TestHeartbeatExtendsLease|criterion 4: heartbeat extends the lease" \
  "TestUnknownKindFailsLoud|criterion 5: unknown kind fails loud" \
  "TestGracefulShutdownReleasesLeases|criterion 6: graceful shutdown releases leases (-race)" \
  "TestJobTenantScopePropagates|criterion 7: tenant scope propagates to the handler" \
  "TestScheduleOverlapSkipsLogsMetric|criterion 8: overlap policy skips + logs + metric" \
  "TestScheduleDispatchIdempotentPerWindow|criterion 9: idempotent dispatch per due-window" \
  "TestTriggerCatalogAndUnsupported|criterion 10: cron/interval correct, condition/event rejected" \
  "TestJobsConfigValidationFailsLoud|criterion 11: jobs config validation fails loud"

summarize_and_exit
