#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 11 `uploads-workspace` (CLAUDE.md §4.2 / §16 step 7).
#
# One ok()/fail()/skip() per acceptance criterion in
# docs/plans/phase-11-uploads-workspace.md ("Smoke checks" table). The whole
# script SKIPs when the surface it tests is not built yet (the uploads package
# does not exist), so `make preflight` runs cleanly as the build grows.
#
# Excluded from scripts/preflight.sh's own runner loop by name only; preflight
# invokes it like every other scripts/smoke/*.sh.

PHASE="11"

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

# Shared run_group helper (collapses per-criterion `go test -run` calls into one
# process and recovers per-criterion OK/FAIL/SKIP). Defined in lib.bash.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/smoke/lib.bash
. "scripts/smoke/lib.bash"

# --- surface guard -----------------------------------------------------------
# The phase-11 surface is the uploads core package, not a binary route. If it
# does not exist yet, SKIP the whole script (an unbuilt surface is not broken).
PKG="./internal/engineering/uploads/..."
if ! go list "$PKG" >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no internal/engineering/uploads package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# Each pair is "TestName|human label". Integration tests that require the Docker
# Postgres URL t.Skip() when it is unset and surface here as genuine SKIPs.
# Architecture / unit tests carry no external dependency and run everywhere.

run_group "$PKG" "-race" \
  "TestUploadCSVRoundTrip_Integration|criterion 1: CSV upload → queryable dataset via shared adapter+grants path (no grant ⇒ access.none, no query)" \
  "TestUploadXLSXParquetLoadCGoFree|criterion 2: XLSX + Parquet load through the same loader, CGo-free" \
  "TestUploadMalformedTypedError|criterion 3: malformed upload ⇒ typed upload.malformed, no partial load" \
  "TestUploadLimitsTypedError|criterion 4: oversized/over-rows/unsupported-format ⇒ typed errors" \
  "TestUploadCellSanitization|criterion 5: cell sanitization + header-name sanitization (golden)" \
  "TestUploadTypeInferenceOnce|criterion 6: type inference computed once, dialect-agnostic (money regression guard)" \
  "TestWorkspaceUnreachableViaStore|criterion 7: workspace DB unreachable through the store seam (architecture)" \
  "TestUploadLoaderUnreachableFromNLQExec|criterion 8: loader write path unreachable from nlq/exec (architecture, P1c)" \
  "TestEraseTenantRemovesTablesAndFiles_Integration|criterion 9: tenant erase removes workspace tables + custody files, loud on partial failure" \
  "TestWorkspaceProvisionerSeamAndTenantPaths|criterion 10: provisioner seam + tenant-derived custody/workspace paths (no cross-tenant path)"

summarize_and_exit
