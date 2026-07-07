#!/usr/bin/env bash
set -euo pipefail

# Smoke checks for phase 05 — internal/gateway (the intelligence seam).
# CLAUDE.md §4.2 / §16 step 7. One ok()/fail() per acceptance criterion in
# docs/plans/phase-05-gateway.md. The script SKIPs *entirely* until the
# internal/gateway package exists, so `make preflight` stays green as the build
# grows phase by phase.
#
# Deliberately excluded from scripts/preflight.sh's own loop only via the
# `*.sh` convention already documented in _template.sh.

PHASE="05"

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
SMOKE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "${SMOKE_DIR}/lib.bash"

# --- surface guard -----------------------------------------------------------
# The gateway is a library package (no CLI command / HTTP route / MCP tool), so
# the guard is: does the package compile as a Go package yet? Until it exists,
# SKIP the whole script — an unbuilt surface is not a broken one.
if ! go list ./internal/gateway >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (internal/gateway absent)"
  summarize_and_exit
fi

# --- assertions ----------------------------------------------------------------
# One criterion → one top-level Go test, collapsed into a single `go test`
# process by run_group; a test that isn't present yet surfaces as a SKIP (not a
# FAIL), so a partially-built package still runs cleanly. Run under -race to
# also satisfy criterion 7's concurrent-reuse obligation.
run_group ./internal/gateway "-race" \
  "TestArch_NoProviderSDKOutsideGateway|criterion 1: no provider SDK imported outside internal/gateway (P5)" \
  "TestSchemaViolationTypedError|criterion 2: schema violation ⇒ typed error, never a partial parse" \
  "TestEveryCallMetered|criterion 3: every call emits one gateway_call_events row + counter tick" \
  "TestEmbeddingDimsMismatchRefusesBoot|criterion 4: embedding dims mismatch ⇒ refused boot (D-029)" \
  "TestArch_NoFreeTextJSONParse|criterion 5: free-text JSON parse of model output forbidden (lint)" \
  "TestUnknownRoleRejectedAtConfig|criterion 6: role enum is closed; unknown/missing role rejected" \
  "TestGatewayConcurrentReuse|criterion 7: Gateway safe under concurrent reuse (-race)" \
  "TestRecordedFixturePerRole|criterion 8: ≥1 recorded-fixture test per role" \
  "TestMockDriverAllRolesResolvable|criterion 9: mock driver resolves all seven roles"

summarize_and_exit
