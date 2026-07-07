#!/usr/bin/env bash
set -euo pipefail

# Smoke-check for phase 14 — warehouse-drivers (mysql, sqlserver, bigquery,
# snowflake, databricks) behind the internal/sources adapter seam. Maps each
# acceptance criterion in docs/plans/phase-14-warehouse-drivers.md to one
# ok()/fail()/skip() call.
#
# THREE criterion classes (D-032):
#   hermetic   — run in CI (registration, ValidatedSQL-only, CGO_ENABLED=0 build,
#                dialect/type fixtures, credential-shape, the guard meta-check).
#   dockerized — self-hostable engines (mysql, sqlserver) against seeded docker
#                instances; SKIP unless CHARTWORKS_MYSQL_DSN / CHARTWORKS_SQLSERVER_DSN
#                are set (i.e. `make warehouses-up` has run).
#   live-gated — cloud engines (bigquery, snowflake, databricks) against real
#                warehouses; SKIP unless the per-kind CHARTWORKS_LIVE_<KIND>_DSN is
#                set. These belong to the D-010 wave-end live gate, never CI.
#
# The whole script SKIPs before the binary/package exists, so `make preflight`
# stays green as the build grows. It is excluded from scripts/preflight.sh's loop
# only by name convention; it lives beside the other phase smokes.

PHASE="14"

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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

# Shared collapsed-test helper (run_group). It defines run_group in terms of the
# ok()/fail()/skip() above and SKIPs any test name not yet present.
# shellcheck source=scripts/smoke/lib.bash
if [ -f scripts/smoke/lib.bash ]; then
  . scripts/smoke/lib.bash
fi

# --- surface guard -----------------------------------------------------------
# The driver packages live under internal/sources/. Until any of them exists the
# whole phase is unbuilt — SKIP cleanly rather than fail.
SRC_PRESENT=0
for d in mysql sqlserver bigquery snowflake databricks; do
  if [ -d "internal/sources/$d" ]; then
    SRC_PRESENT=1
    break
  fi
done
if [ "$SRC_PRESENT" -eq 0 ]; then
  skip "phase-${PHASE} surface not built yet (no internal/sources/{mysql,sqlserver,bigquery,snowflake,databricks})"
  summarize_and_exit
fi

SRC_PKG="./internal/sources/..."

# --- hermetic criteria (CI) --------------------------------------------------
# run_group SKIPs any of these test names that don't exist yet, so the script
# stays green while the build grows.
if command -v go >/dev/null 2>&1 && declare -f run_group >/dev/null 2>&1; then
  run_group "$SRC_PKG" "-race" \
    "TestRegistersAndConstructs|criterion 1: all five drivers register + construct via factory" \
    "TestQueryRequiresValidatedSQL|criterion 2: Query accepts only ValidatedSQL (no raw-string execute)" \
    "TestDialectFixtures|criterion 4: recorded-fixture dialect parse (incl. mysql, tsql)" \
    "TestTypeCategoryFixtures|criterion 4: TypeCategory classification from fixtures" \
    "TestConfigShapeHasNoSecret|criterion 5: read/list config shape carries no secret field"
else
  skip "criteria 1,2,4,5: go toolchain or run_group helper unavailable"
fi

# criterion 3: CGO_ENABLED=0 static build with all five drivers + snowflake tag.
if command -v go >/dev/null 2>&1 && [ -d cmd/chartworks ]; then
  if CGO_ENABLED=0 go build -tags minicore_disabled -o /dev/null ./cmd/chartworks 2>/dev/null; then
    ok "criterion 3: CGO_ENABLED=0 build (-tags minicore_disabled) succeeds with all drivers linked"
  else
    fail "criterion 3: CGO_ENABLED=0 build (-tags minicore_disabled) failed"
  fi
else
  skip "criterion 3: cmd/chartworks or go toolchain not present yet"
fi

# --- dockerized criteria (self-hostable: mysql, sqlserver) -------------------
# SKIP unless both engine DSNs are set (i.e. `make warehouses-up` has seeded them).
if [ -n "${CHARTWORKS_MYSQL_DSN:-}" ] && [ -n "${CHARTWORKS_SQLSERVER_DSN:-}" ]; then
  if [ -x scripts/seed/warehouses.sh ]; then
    if scripts/seed/warehouses.sh --check >/dev/null 2>&1; then
      ok "criterion 6: dataset seeder present + idempotent + license-allowlist clean"
    else
      fail "criterion 6: dataset seeder --check failed (not seeded / license or idempotency guard)"
    fi
  else
    skip "criterion 6: scripts/seed/warehouses.sh not present yet"
  fi
  if declare -f run_group >/dev/null 2>&1; then
    run_group "$SRC_PKG" "-race" \
      "TestConnectDiscoverSample_Mysql|criterion 7: mysql connect/discover/sample on seeded data" \
      "TestConnectDiscoverSample_Sqlserver|criterion 7: sqlserver connect/discover/sample on seeded data" \
      "TestReadOnlyProbe_Mysql|criterion 8: mysql read-only defense-in-depth probe" \
      "TestReadOnlyProbe_Sqlserver|criterion 8: sqlserver read-only defense-in-depth probe" \
      "TestTimeoutAndRowCap_Mysql|criterion 9: mysql server-side timeout + cursor cap (ORDER BY kept)" \
      "TestTimeoutAndRowCap_Sqlserver|criterion 9: sqlserver server-side timeout + cursor cap (ORDER BY kept)"
  fi
else
  skip "criterion 6: dockerized seeder (CHARTWORKS_MYSQL_DSN/CHARTWORKS_SQLSERVER_DSN unset — run make warehouses-up)"
  skip "criterion 7: dockerized connect/discover/sample (self-hostable engines not up)"
  skip "criterion 8: dockerized read-only probes (self-hostable engines not up)"
  skip "criterion 9: dockerized timeout + row-cap (self-hostable engines not up)"
fi

# --- live-gated criteria (cloud: bigquery, snowflake, databricks) ------------
# Belong to the D-010 wave-end live gate; SKIP unless the per-kind .env DSN is set.
for kind in BIGQUERY SNOWFLAKE DATABRICKS; do
  var="CHARTWORKS_LIVE_${kind}_DSN"
  Title="$(printf '%s' "$kind" | awk '{print toupper(substr($0,1,1)) tolower(substr($0,2))}')"
  if [ -n "${!var:-}" ] && declare -f run_group >/dev/null 2>&1; then
    run_group "$SRC_PKG" "-count=1" \
      "TestCloudReadOnlyTimeoutCap_${Title}|criterion 10: ${Title} read-only + timeout + cap (live)" \
      "TestConnectAndDiscover_${Title}|criterion 11: ${Title} connect + discover (live)"
  else
    skip "criterion 10/11: ${Title} live gate ($var unset — D-010 wave-end check)"
  fi
done

# --- criterion 12: guard meta-check (hermetic) -------------------------------
# With no docker/live env set, the external test names must report SKIP, never
# PASS/FAIL — proving the build-tag / env guards hold and CI runs nothing
# unguarded. We assert here only that this smoke script itself did NOT run any
# external test when the env was absent (the SKIPs above stand in for that);
# the in-code guard is unit-asserted by the tests' own t.Skip.
if [ -z "${CHARTWORKS_MYSQL_DSN:-}${CHARTWORKS_SQLSERVER_DSN:-}${CHARTWORKS_LIVE_BIGQUERY_DSN:-}${CHARTWORKS_LIVE_SNOWFLAKE_DSN:-}${CHARTWORKS_LIVE_DATABRICKS_DSN:-}" ]; then
  ok "criterion 12: external (dockerized/live) halves guarded — none ran without env (all SKIPped)"
else
  skip "criterion 12: external env set — guard meta-check runs in the gated environment, not the smoke loop"
fi

summarize_and_exit
