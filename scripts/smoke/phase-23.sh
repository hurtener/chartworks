#!/usr/bin/env bash
set -euo pipefail

# Smoke checks for phase 23 — sdk-cli-parity (CLAUDE.md §4.2 / §16 step 7).
#
# Each acceptance criterion in docs/plans/phase-23-sdk-cli-parity.md maps to one
# run_group entry below. The whole script SKIPs cleanly until the surface exists
# (no sdk/chartworks package yet) so `make preflight` stays green as the build
# grows. Per-criterion, run_group SKIPs any test not present or a store-gated
# integration test that t.Skip's without a Postgres URL.

PHASE="23"

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
# Phase 23 ships sdk/chartworks; until that package exists the whole surface is
# unbuilt — SKIP the entire script rather than failing it.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

if [ ! -d "sdk/chartworks" ]; then
  skip "phase-${PHASE} surface not built yet (no sdk/chartworks package)"
  summarize_and_exit
fi

# shellcheck source=scripts/smoke/lib.bash
. "${ROOT}/scripts/smoke/lib.bash"

# --- assertions --------------------------------------------------------------

# Criterion 1 — SDK one-interface, two-transport parity + shared core stack.
run_group ./sdk/chartworks "" \
  "TestClientInterfaceParity|criterion 1: Client interface satisfied by both transports" \
  "TestInProcessSharesCoreStack|criterion 1: in-process transport shares the one core stack (arch test)"

# Criterion 2 — SDK auth injection, no X-* identity header.
run_group ./sdk/chartworks "" \
  "TestSDKAuthInjection|criterion 2: bearer injected, no X-* header, no-token is a typed error"

# Criteria 3–6 — the admin CLI family (run(args,stdout,stderr) int).
run_group ./cmd/chartworks "" \
  "TestAdminRunDispatch|criterion 3: admin run(args,…) dispatch, bad verb ⇒ non-zero + usage" \
  "TestAdminScopeDebug|criterion 4: scope-debug replays the one resolver, prints failed predicate" \
  "TestAdminKeysRoundTrip|criterion 5: keys create/list/revoke, secret shown once, absent from list" \
  "TestAdminBootstrap|criterion 6: bootstrap local-only + idempotent-guarded"

# Criteria 7–8 — the three-surface parity suite and the erase cascade.
run_group ./test/integration "-race" \
  "TestThreeSurfaceParity|criterion 7: parity enumerated from route/tool registries across HTTP+MCP+SDK" \
  "TestEraseCascade|criterion 8: erase cascades store+vectors+workspace+files, loud on partial failure"

summarize_and_exit
