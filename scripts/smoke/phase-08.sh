#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 08 (`sources-core`). CLAUDE.md §4.2 / §16 step 7.
#
# Maps each acceptance criterion in docs/plans/phase-08-sources-core.md to one
# ok()/fail()/skip() outcome. The script SKIPs *entirely* until the
# internal/sources package exists, so `make preflight` stays green as the build
# grows. Criterion 12 (Docker-Postgres conformance) SKIPs on its own when the
# test DSN is unset — the test t.Skip's and run_group surfaces that as SKIP.

PHASE="08"

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
# Phase 08 is package-level: the surface is the internal/sources package. Until
# it exists (and go is available), SKIP the whole script — an unbuilt surface is
# not a broken one.
if ! command -v go >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not testable yet (no go toolchain)"
  summarize_and_exit
fi
if [ ! -d "internal/sources" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/sources package)"
  summarize_and_exit
fi

PKG="./internal/sources/..."

# --- assertions --------------------------------------------------------------
# One criterion per "TestName|label" pair. run_group collapses them into a
# single `go test` process, SKIPs any test not present yet, and preserves the
# per-criterion OK/FAIL/SKIP contract.
run_group "$PKG" "" \
  "TestSourceViewHasNoSecretField|criterion 1: registry read/list shape has no secret field (type-level)" \
  "TestEnvelopeEncryptRoundTrip|criterion 2: AES-256-GCM encrypt/decrypt round-trip + fresh nonce" \
  "TestKeyRingRotationDecrypt|criterion 3: decrypt tries all ring keys (rotation)" \
  "TestMissingRingWithStoredSecretsRefusesBoot|criterion 4: missing ring w/ stored secrets refuses boot" \
  "TestNullAdapterFailsLoud|criterion 5: null adapter fails loud on every operation" \
  "TestQueryRequiresValidatedSQL|criterion 6: raw-string execute is unexpressible against the seam" \
  "TestFactoryUnknownKind|criterion 7: unknown adapter kind is a typed error, never a nil adapter" \
  "TestNoAdapterTypeSwitch|criterion 8: capability gating via Supports, never type-switching" \
  "TestTypeCategoryClassificationGolden|criterion 9: TypeCategory computed once at discovery (golden)" \
  "TestCredentialEventsContentFree|criterion 10: credential events are content-free" \
  "TestConnectionStatusLifecycle|criterion 11: status lifecycle unverified→connected→unavailable"

# Criterion 12 — Docker-Postgres conformance, under -race. The test t.Skip's
# when the DSN is unset; run_group reports that as a genuine SKIP.
run_group "$PKG" "-race" \
  "TestPostgresAdapterConformance|criterion 12: postgres adapter discovery conformance on Docker Postgres (-race)"

summarize_and_exit
