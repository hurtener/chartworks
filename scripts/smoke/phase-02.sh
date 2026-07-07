#!/usr/bin/env bash
set -euo pipefail

# Smoke-check for phase 02 — store-migrations (CLAUDE.md §4.2 / §16 step 7).
#
# Every acceptance criterion in docs/plans/phase-02-store-migrations.md maps to
# one ok()/fail()/skip() below. The script SKIPs *entirely* until the store
# package exists, so `make preflight` stays green as the build grows.
#
# Criteria 1–11 are backed by named Go tests in internal/store (run via
# run_group from lib.bash, which recovers per-criterion OK/FAIL/SKIP and turns
# an unbuilt test into a SKIP, not a FAIL). Criterion 12 (coverage) is not a
# smoke assertion — it is enforced by the `make coverage` band gate.
#
# This script is deliberately excluded from scripts/preflight.sh (which runs
# every scripts/smoke/*.sh EXCEPT the template).

PHASE="02"

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

# Resolve repo root so the script runs from anywhere.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

# shellcheck source=scripts/smoke/lib.bash
. "$SCRIPT_DIR/lib.bash"

# --- surface guard -----------------------------------------------------------
# Phase 02's surface is the internal/store package + its embedded migrations —
# not the chartworks binary. Until the package exists, SKIP the whole script
# (an unbuilt surface is not a broken one).
if [ ! -d "internal/store" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/store package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# One entry per acceptance criterion (criterion 12 is the make-coverage gate,
# not a smoke test). run_group collapses these into one `go test` invocation and
# recovers per-criterion OK/FAIL/SKIP; a not-yet-written test surfaces as SKIP.
#
# Criterion 9's conformance suite is the one run under -race (it is the
# tenant-isolation contract); the rest run without the race flag.
STORE_PKG="./internal/store/..."

run_group "$STORE_PKG" "" \
  "TestMigrateFreshToHead|criterion 1: fresh-DB apply, each version recorded once" \
  "TestMigrateIdempotent|criterion 2: re-apply is a no-op" \
  "TestMigrateForwardOnlyGuard|criterion 3: forward-only integrity, no down path" \
  "TestSchemaInventoryMatchesRFC|criterion 4: exactly the 25 RFC §12 tables exist" \
  "TestTenantIdNotNullCheckEverywhere|criterion 5a: tenant_id NOT NULL CHECK on every table" \
  "TestBlankTenantRejected|criterion 5b: blank tenant_id rejected by the CHECK" \
  "TestNoScopelessQueryMethod|criterion 6: every method carries a non-optional scope" \
  "TestPerDomainInterfacesNoGodInterface|criterion 7: composition, no god-interface" \
  "TestCrossTenantProbeReturnsNothing|criterion 8: cross-tenant read returns nothing" \
  "TestMigrateConcurrentAdvisoryLock|criterion 10: concurrent runners never double-apply" \
  "TestStoreConfigValidation|criterion 11: missing DSN / unknown key refused"

# Criterion 9 — the conformance suite is the tenant-isolation contract, run
# under -race against a fresh Docker Postgres (convention 8).
run_group "$STORE_PKG" "-race" \
  "TestStoreConformance|criterion 9: conformance green under -race on a fresh DB"

summarize_and_exit
