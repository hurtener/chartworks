#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 21 (`http-api`). CLAUDE.md §4.2 / §16 step 7.
#
# Maps each acceptance criterion in docs/plans/phase-21-http-api.md to one
# ok()/fail()/skip() outcome. The script SKIPs *entirely* until the
# internal/api package exists, so `make preflight` stays green as the build
# grows. Criteria 10 and 15 (Docker-Postgres integration) SKIP on their own
# when the test DSN is unset — the test t.Skip's and run_group surfaces that as
# a genuine SKIP.

PHASE="21"

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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$REPO_ROOT"

# shellcheck source=scripts/smoke/lib.bash
. "${SCRIPT_DIR}/lib.bash"

# --- surface guard -----------------------------------------------------------
# Phase 21 is package-level: the surface is the internal/api package. Until it
# exists (and go is available), SKIP the whole script — an unbuilt surface is
# not a broken one.
if ! command -v go >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not testable yet (no go toolchain)"
  summarize_and_exit
fi
if [ ! -d "internal/api" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/api package)"
  summarize_and_exit
fi

PKG="./internal/api/..."

# --- assertions --------------------------------------------------------------
# One criterion per "TestName|label" pair. run_group collapses them into a
# single `go test` process, SKIPs any test not present yet, and preserves the
# per-criterion OK/FAIL/SKIP contract.
run_group "$PKG" "" \
  "TestRouteRegistryComplete|criterion 1: every mounted route is in the descriptor table (mechanical)" \
  "TestAuditCoverageFromRouteTable|criterion 2: audit coverage derived mechanically from the route table" \
  "TestCrossTenantRegistryProbe|criterion 3: registry-driven cross-tenant probe (only 400/401/403/404)" \
  "TestTenantMismatch400|criterion 4: path/param vs token tenant disagreement is a typed 400" \
  "TestDeniedResourceReads404|criterion 5: denied resource reads as 404 (existence-hiding)" \
  "TestMissingScope403|criterion 6: missing capability scope is a 403 before the handler" \
  "TestErrorMappingGolden|criterion 7: typed error vocabulary maps to HTTP status (content-free body)" \
  "TestTransportHardening|criterion 8: explicit timeouts/body-limit/Content-Type/Origin (413/415/403)" \
  "TestNoCookieTransport|criterion 9: bearer-only, no cookie read/set; cookie-only auth is 401" \
  "TestMiddlewareOrder|criterion 11: request-id→auth→envelope→resolve→scope→audit→handler, resolve-once" \
  "TestMetricsOperatorScoped|criterion 12: /metrics operator-scoped; health open; metrics exported" \
  "TestApiThinSurface|criterion 13: thin surface — no SDK/store/SQL, decode→core→encode" \
  "TestAskTierParitySkeleton|criterion 14: Ask/BYO parity skeleton vs MCP (shared capability registry)"

# Criteria 10 and 15 — Docker-Postgres integration, under -race. Each test
# t.Skip's when the DSN is unset; run_group reports that as a genuine SKIP.
run_group "$PKG" "-race" \
  "TestUploadMultipart|criterion 10: multipart upload streams to a queryable dataset (-race)" \
  "TestHTTPEndpointPerGroup|criterion 15: one endpoint per §11.2 group returns its typed shape (-race)"

summarize_and_exit
