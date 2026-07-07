#!/usr/bin/env bash
set -euo pipefail

# Smoke checks for phase-19 (`byo-mode`) — CLAUDE.md §4.2 / §16 step 7.
#
# One ok()/fail()/skip() per acceptance criterion in
# docs/plans/phase-19-byo-mode.md ("Smoke checks" table). This phase adds NO
# new CLI command / HTTP endpoint / MCP tool / config key surface that the
# binary exposes directly in phase 19 (the two BYO tools land in phase 22, the
# HTTP endpoints in phase 21) — so the surface guard is the presence of the
# `internal/nlq` BYO tests themselves, and the whole script SKIPs cleanly until
# they exist. `make preflight` stays green as the build grows.

PHASE="19"

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

# shellcheck source=scripts/smoke/lib.bash
. "scripts/smoke/lib.bash"

# --- surface guard -----------------------------------------------------------
# The BYO surface lives in internal/nlq (bundle projection + get_query_context /
# submit_sql services). Until that package exists AND go can resolve it, SKIP the
# whole script — an unbuilt surface is not a broken one.
BYO_PKG="./internal/nlq/..."
if [ ! -d "internal/nlq" ] || ! go list "$BYO_PKG" >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no internal/nlq BYO package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# run_group collapses the per-criterion `go test -run` invocations into one
# process per package and recovers per-criterion OK/FAIL/SKIP. A test that is not
# present yet surfaces as a SKIP, keeping the growing build green.

run_group "$BYO_PKG" "-race" \
  "TestBundleVersionGolden|criterion 1: bundle carries bundle_version (golden)" \
  "TestBundleBackwardCompatGolden|criterion 1: prior bundle_version still parses (backward-compat golden)" \
  "TestBundleGovernanceRestated|criterion 2: governed rules restated in-bundle + dropped_rules named" \
  "TestBundleSQLRequirementsEnumerated|criterion 3: allowlisted tables/columns + dialect enumerated" \
  "TestBundlePriorSQLProvenanceLabeled|criterion 4: prior SQL is provenance-labeled guidance, capped" \
  "TestSubmitAdversarialCorpusRejected|criterion 5: adversarial submission corpus all typed-rejected" \
  "TestModeParityFromRegistry|criterion 6: mode-parity enumerated from the validator registry" \
  "TestByoProvenanceRecorded|criterion 7: generator=byo recorded on row + envelope" \
  "TestContextScopeCannotSubmit|criterion 8: query.context without query.submit cannot execute" \
  "TestGetContextIssuesNoQuery|criterion 8: get_query_context issues no query (call-count 0)" \
  "TestBundleRefBinding|criterion 9: cross-principal / expired / tampered bundle_ref rejected" \
  "TestRevokedGrantRejectedAtSubmit|criterion 10: revoked grant rejected at submit (context != capability)"

summarize_and_exit
