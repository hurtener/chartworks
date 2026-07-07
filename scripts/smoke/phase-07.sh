#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — CLAUDE.md §4.2 / §16 step 7. Phase 07 — internal/vindex
# (pgvector facet index). One ok()/fail()/skip() per acceptance criterion in
# docs/plans/phase-07-vindex.md. SKIPs entirely until the vindex package exists,
# so `make preflight` stays green as the build grows.

PHASE="07"

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

# Shared helper: run_group collapses N per-criterion `go test -run` invocations
# into one process and recovers per-criterion OK/FAIL/SKIP. A gated conformance
# test that t.Skip's (Docker-Postgres URL unset) surfaces as a genuine SKIP.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/smoke/lib.bash
. "${SCRIPT_DIR}/lib.bash"

# --- surface guard -----------------------------------------------------------
# vindex is a library seam, not a binary/route/tool. The guard is the package's
# existence: until phase 07 lands internal/vindex, SKIP the whole script rather
# than fail it — an unbuilt surface is not a broken one.
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
if [ ! -d "${REPO_ROOT}/internal/vindex" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/vindex package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
VINDEX_PKG="./internal/vindex/..."

# Criteria 1–8: correctness + conformance. Docker-Postgres-gated tests t.Skip
# cleanly (no store URL) and surface here as SKIP.
run_group "$VINDEX_PKG" "" \
  "TestCrossTenantSearchReturnsNothing|criterion 1: cross-tenant search returns nothing" \
  "TestSearchScopedToTopicVersion|criterion 2: search scoped to (tenant,topic,version)" \
  "TestDeleteByTenantCascades|criterion 3: delete-by-tenant cascades" \
  "TestDeleteByTopicCascades|criterion 4: delete-by-topic cascades" \
  "TestDeleteByVersionCascades|criterion 5: delete-by-version cascades" \
  "TestPgvectorConformance|criterion 6: pgvector HNSW cosine conformance" \
  "TestDimensionMismatchRejected|criterion 7: dimension mismatch is a typed error" \
  "TestBatchUpsertRoundTrip|criterion 8: batch upsert round-trips + idempotent"

# Criterion 9: concurrent-reuse safety under the race detector.
run_group "$VINDEX_PKG" "-race" \
  "TestConcurrentReuse|criterion 9: driver race-free under concurrent reuse"

summarize_and_exit
