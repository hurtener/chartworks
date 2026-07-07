#!/usr/bin/env bash
set -euo pipefail

# Smoke checks for phase 03 — auth-identity (CLAUDE.md §4.2 / §16 step 7).
#
# Maps each acceptance criterion in docs/plans/phase-03-auth-identity.md to one
# ok()/fail()/skip() outcome, driven through the shared run_group helper so a
# not-yet-built criterion SKIPs (never FAILs) while the build grows. The whole
# script SKIPs cleanly until internal/auth exists (no go.mod / package yet at
# bootstrap).

PHASE="03"

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

# --- surface guard -----------------------------------------------------------
# The auth/identity packages are the surface. Until phase 03 lands them (and the
# module exists), SKIP the whole script — an unbuilt surface is not a broken one.
if [ ! -f go.mod ] || [ ! -d internal/auth ]; then
  skip "phase-${PHASE} surface not built yet (no go.mod or internal/auth package)"
  summarize_and_exit
fi

# shellcheck source=scripts/smoke/lib.bash
. "scripts/smoke/lib.bash"

# --- assertions --------------------------------------------------------------
# internal/auth criteria (1–6, 8–11) — one process for the whole present group.
run_group ./internal/auth "-race" \
  "TestParserRejectsHSAndNone|criterion 1: HS*/none rejected at the parser, before claims" \
  "TestIssExpSkewEnforced|criterion 2: iss/exp/skew enforced with distinct typed errors" \
  "TestJWKSStaleFailsClosed|criterion 3: stale JWKS fails closed (typed 401 + not-ready)" \
  "TestPerSurfaceAudienceCrossRejected|criterion 4: HTTP/MCP audiences cross-rejected" \
  "TestAPIKeyExchangeConstantTimeAndScoped|criterion 5: API-key exchange constant-time + scoped" \
  "TestMissingSigningKeyRefusesBoot|criterion 6: missing signing key ⇒ refused boot (no silent gen)" \
  "TestNoHeaderExchangeMintPath|criterion 8: no header-exchange mint path exists (D-030)" \
  "TestAdminBootstrapLocalOnly|criterion 9: admin bootstrap local-only (Postgres half skips pre-02)" \
  "FuzzParseToken|criterion 10: FuzzParseToken seed corpus (never panics / never passes non-asymmetric)" \
  "TestForgedHeaderNoEffect|criterion 11: forged X-* headers have no effect on identity/tenant"

# internal/identity criterion (7).
run_group ./internal/identity "-race" \
  "TestEnvelopeImmutable|criterion 7: envelope immutability is API-enforced"

summarize_and_exit
