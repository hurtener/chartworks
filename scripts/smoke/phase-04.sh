#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 04 `access-grants` (CLAUDE.md §4.2 / §16 step 7).
#
# Maps each acceptance criterion in docs/plans/phase-04-access-grants.md to one
# ok()/fail()/skip() via run_group. The script SKIPs *entirely* until
# internal/access is built (no package, no go.mod, or `go` unavailable) so
# `make preflight` stays green as the build grows phase by phase.
#
# Excluded from scripts/preflight.sh's own loop (which runs every
# scripts/smoke/*.sh EXCEPT this template-derived one via its guard).

PHASE="04"

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
# Phase 04 ships a library (internal/access), not a new binary. SKIP the whole
# script until the module and the package exist and the Go toolchain is present.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if [ -z "$(command -v go 2>/dev/null || true)" ]; then
  skip "phase-${PHASE} surface not testable yet (no go toolchain)"
  summarize_and_exit
fi
if [ ! -f go.mod ] || [ ! -d internal/access ]; then
  skip "phase-${PHASE} surface not built yet (no go.mod or internal/access package)"
  summarize_and_exit
fi

# shellcheck source=scripts/smoke/lib.bash
. "${REPO_ROOT}/scripts/smoke/lib.bash"

# --- assertions --------------------------------------------------------------
# One criterion per run_group pair; unbuilt tests surface as SKIP, never FAIL.
PKG="./internal/access/..."

run_group "$PKG" "-race" \
  "TestDenyByDefaultIssuesNoQuery|criterion 1: deny-by-default issues no query (store call-count 0)" \
  "TestEmptySetShortCircuitMetric|criterion 2: empty-set short-circuit increments deny/empty_set metric" \
  "TestAgentResolvesOwnGrants|criterion 3: agent principals resolve their own grants, not the owner's" \
  "TestAdminSentinelTenantBounded|criterion 4: admin sentinel resolves manage-all but stays tenant-bounded" \
  "TestTenantIntersectedAfterResolution|criterion 5: foreign-tenant grant absent after resolution" \
  "TestRoleDefaults|criterion 6: role defaults (member catalog-read, viewer/unknown explicit-only)" \
  "TestScopeDebugNamesPredicate|criterion 7: scope-debug names the failed predicate" \
  "TestBothMustPass|criterion 8: scopes and grants both required" \
  "TestAdversarialRegistryCrossTenant|criterion 9: registry-driven cross-tenant + empty/forged/fetch-then-filter guards" \
  "TestResolveConcurrentReuse|criterion 10: resolver concurrency-safe under -race"

summarize_and_exit
