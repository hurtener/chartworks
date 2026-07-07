#!/usr/bin/env bash
set -euo pipefail

# Smoke-check for phase 22 — mcp-server (RFC §11.1, D-019).
#
# Maps each acceptance criterion in docs/plans/phase-22-mcp-server.md to one
# ok()/fail() call. SKIPs entirely until the MCP surface is built (no
# `chartworks mcp` subcommand / no registered tools), so `make preflight` runs
# cleanly while the build grows.

PHASE="22"

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

# --- locate the binary -------------------------------------------------------
BIN=""
if [ -x "bin/chartworks" ]; then
  BIN="bin/chartworks"
elif command -v chartworks >/dev/null 2>&1; then
  BIN="$(command -v chartworks)"
fi

# --- surface guard -----------------------------------------------------------
# The MCP surface is present only when the binary exists AND exposes the `mcp`
# subcommand with a tool listing. Absent either, the phase-22 surface is not
# built yet — SKIP the whole script rather than fail it.
if [ -z "$BIN" ]; then
  skip "phase-${PHASE} surface not built yet (no chartworks binary)"
  summarize_and_exit
fi
if ! "$BIN" mcp --list-tools >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no 'mcp --list-tools' subcommand)"
  summarize_and_exit
fi

TOOLS="$("$BIN" mcp --list-tools 2>/dev/null || true)"

# --- assertions --------------------------------------------------------------

# Criterion 1 — every RFC §11.1 tool registered through the one list.
EXPECTED="list_topics describe_topic list_datasets describe_dataset preflight_question plan_query run_query refine_query get_query_context submit_sql submit_feedback"
MISSING=""
for t in $EXPECTED; do
  printf '%s\n' "$TOOLS" | grep -qw "$t" || MISSING="$MISSING $t"
done
if [ -z "$MISSING" ]; then
  ok "criterion 1: every RFC §11.1 tool is registered"
else
  fail "criterion 1: missing tool(s):$MISSING"
fi

# Criterion 2 — fail-closed annotation allowlist: only submit_feedback is write.
if "$BIN" mcp --list-tools --annotations 2>/dev/null | grep -qiE 'submit_feedback.*write' \
  && ! "$BIN" mcp --list-tools --annotations 2>/dev/null | grep -viE 'submit_feedback' | grep -qi 'write'; then
  ok "criterion 2: annotation allowlist — only submit_feedback is write, no unclassified tool"
else
  fail "criterion 2: annotation allowlist violated (unclassified or unexpected write tool)"
fi

# Criterion 3 — middleware installed + single registration path intact.
if "$BIN" mcp --self-check 2>/dev/null | grep -qi 'middleware: installed'; then
  ok "criterion 3: tool-handler middleware installed, single registration path intact"
else
  fail "criterion 3: middleware/registration self-check failed"
fi

# Criterion 4 — MCP-audience enforcement (HTTP-aud token rejected 401).
if "$BIN" mcp --self-check 2>/dev/null | grep -qi 'aud-enforcement: ok'; then
  ok "criterion 4: MCP-audience enforced (HTTP-aud token rejected)"
else
  fail "criterion 4: MCP-audience enforcement self-check failed"
fi

# Criterion 5 — in-process round-trip.
if "$BIN" mcp --self-check 2>/dev/null | grep -qi 'in-process-roundtrip: ok'; then
  ok "criterion 5: in-process client round-trip green"
else
  fail "criterion 5: in-process round-trip self-check failed"
fi

# Criterion 6 — panic never crosses the boundary.
if "$BIN" mcp --self-check 2>/dev/null | grep -qi 'panic-injection: typed-internal'; then
  ok "criterion 6: panic recovered to typed internal result, no stack crosses the boundary"
else
  fail "criterion 6: panic-injection self-check failed"
fi

# Criterion 7 — scope gate denies before the core; empty-access short-circuit.
if "$BIN" mcp --self-check 2>/dev/null | grep -qi 'scope-gate: deny-before-core' \
  && "$BIN" mcp --self-check 2>/dev/null | grep -qi 'empty-access: access_none'; then
  ok "criterion 7: scope gate denies before core; empty access short-circuits"
else
  fail "criterion 7: scope-gate / empty-access self-check failed"
fi

# Criterion 8 — typed errors, never a raise.
if "$BIN" mcp --self-check 2>/dev/null | grep -qi 'error-mapping: typed'; then
  ok "criterion 8: core errors mapped to typed isError results, never a raise"
else
  fail "criterion 8: error-mapping self-check failed"
fi

# Criterion 9 — registry-driven cross-tenant probe.
if "$BIN" mcp --self-check 2>/dev/null | grep -qi 'cross-tenant-probe: no-leak'; then
  ok "criterion 9: registry-driven cross-tenant probe reports no leak"
else
  fail "criterion 9: cross-tenant probe self-check failed"
fi

# Criterion 10 — management ops absent, not soft-failed.
if printf '%s\n' "$TOOLS" | grep -qiE '(^| )(create_source|create_pipeline|grant|publish_topic|archive_topic|run_pipeline)'; then
  fail "criterion 10: a management-plane tool is registered on MCP (D-030 violation)"
else
  ok "criterion 10: no management-plane tool registered on MCP (D-030)"
fi

# Criterion 11 — mcp-go pinned at v0.55.1.
if command -v go >/dev/null 2>&1; then
  PIN="$(go list -m github.com/mark3labs/mcp-go 2>/dev/null | awk '{print $2}' || true)"
  if [ "$PIN" = "v0.55.1" ]; then
    ok "criterion 11: mcp-go pinned at v0.55.1"
  else
    fail "criterion 11: mcp-go pin is '${PIN}', expected v0.55.1"
  fi
else
  skip "criterion 11: go toolchain unavailable to check the mcp-go pin"
fi

# Criterion 12 — config fail-loud + Ask-tier parity skeleton.
if "$BIN" mcp --self-check 2>/dev/null | grep -qi 'config: bad-mcp-path-refused' \
  && "$BIN" mcp --self-check 2>/dev/null | grep -qi 'parity-skeleton: ask-tier-ok'; then
  ok "criterion 12: invalid server.mcp.path refused; Ask-tier parity skeleton green"
else
  fail "criterion 12: config fail-loud / parity-skeleton self-check failed"
fi

summarize_and_exit
