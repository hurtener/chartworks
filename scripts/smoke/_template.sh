#!/usr/bin/env bash
set -euo pipefail

# Smoke-check template — CLAUDE.md §4.2 / §16 step 7.
#
# Copy this file when authoring a phase plan:
#   cp scripts/smoke/_template.sh scripts/smoke/phase-NN.sh
#
# Every acceptance criterion in docs/plans/phase-NN-*.md maps to one
# ok()/fail() call below ("Smoke checks" section of the plan). A script SKIPs
# *entirely* when the surface it tests hasn't been built yet (no binary, no
# registered route, no MCP tool) so `make preflight` runs cleanly as the
# build grows phase by phase.
#
# This script is deliberately excluded from scripts/preflight.sh (which runs
# every scripts/smoke/*.sh EXCEPT this one).

PHASE="NN" # <- replace with the phase number when copying this template

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
# Replace with the real guard for this phase: a built binary, a registered
# HTTP route, an MCP tool listed in the tool registry, a migration applied,
# etc. When the guard fails, SKIP the whole script rather than failing it —
# an unbuilt surface is not a broken one.
if [ ! -x "bin/chartworks" ] && [ -z "$(command -v chartworks 2>/dev/null || true)" ]; then
  skip "phase-${PHASE} surface not built yet (no chartworks binary)"
  summarize_and_exit
fi

# --- assertions ----------------------------------------------------------------
# One ok()/fail() call per acceptance criterion, e.g.:
#
#   if <criterion 1 holds>; then
#     ok "criterion 1: <short description>"
#   else
#     fail "criterion 1: <short description>"
#   fi

summarize_and_exit
