#!/usr/bin/env bash
set -euo pipefail

# Smoke-check for phase 15 — topics-lifecycle (internal/semantics).
# CLAUDE.md §4.2 / §16 step 7. One ok()/fail()/skip() per acceptance criterion in
# docs/plans/phase-15-topics-lifecycle.md ("Smoke checks" table).
#
# SKIPs the whole script when internal/semantics has no Go files yet, so
# `make preflight` stays green as the build grows phase by phase. Criteria whose
# tests gate on a Docker Postgres / vindex URL surface as genuine SKIPs (the test
# t.Skip's) — no separate pre-check needed.

PHASE="15"

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
# The phase-15 surface is the internal/semantics package. Until it has Go files,
# SKIP the whole script (an unbuilt surface is not a broken one).
if ! ls internal/semantics/*.go >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no internal/semantics/*.go)"
  summarize_and_exit
fi

# Shared run_group helper: collapses the per-criterion `go test -run` calls into
# one process per package and recovers per-criterion OK/FAIL/SKIP from -v output.
# shellcheck source=scripts/smoke/lib.bash
. "scripts/smoke/lib.bash"

# --- assertions --------------------------------------------------------------
# Criteria 1–13 are Go tests in internal/semantics; run under -race (concurrency
# invariant #2 and shared-artifact reuse). A test absent from the built package
# is a SKIP; a t.Skip (store/vindex URL unset) is a SKIP; only a real --- FAIL is
# a FAIL.
run_group "./internal/semantics/..." "-race" \
  "TestPackValidators|criterion 1: pack validators (ids, KPI refs, equality-only joins, dangling-key prune)" \
  "TestExactlyOneActiveVersionConcurrent|criterion 2: exactly-one-active-version under concurrency" \
  "TestDraftOnlyMutationAndDiscardGuard|criterion 3: DRAFT-only mutation + active-version discard guard" \
  "TestAtomicPublishSwapAndRollback|criterion 4: atomic publish swap + rollback, no half-published state" \
  "TestTypedAuditOnEveryTransition|criterion 5: one typed topic_audit row per transition" \
  "TestTransitionAuthorityMatrix|criterion 6: transition-authority matrix (scope ∩ grant), typed deny" \
  "TestHealthGatingExcludesUnavailableTables|criterion 7: health gating excludes unavailable tables from contracts" \
  "TestRecheckSourceAndReferenceRewriting|criterion 8: re-check source + reference-rewriting, no dangling refs" \
  "TestFacetDecompositionOnPublish|criterion 9: facet decomposition → vindex (tenant,topic,version), batched" \
  "TestInvalidationMatrix|criterion 10: each transition carries its binding invalidation" \
  "TestCapabilityContractCaps|criterion 11: capability-contract hard caps, one owner, pure projection" \
  "TestTopicGenerationHumanGated|criterion 12: topic generation via enhance role, never auto-publishes" \
  "TestExportSanitizerNoInternalIds|criterion 13: export carries no internal ids (P6 sanitizer)"

# --- criterion 14: no `repair`/plumbing word on any surface (drift-audit) -----
# Mechanical P6 check for this package's wire-facing strings. drift-audit's
# plumbing-vocabulary scan is scoped to cmd/ internal/api/ internal/mcpserver/
# sdk/, so here we additionally assert directly against internal/semantics.
if [ -d internal/semantics ]; then
  SEM_HITS="$(grep -rIn --include='*.go' -iE '"[^"]*\b(repair|broken|reindex)\b' internal/semantics 2>/dev/null || true)"
  if [ -z "$SEM_HITS" ]; then
    ok "criterion 14: no repair/plumbing word in internal/semantics wire strings"
  else
    fail "criterion 14: forbidden P6 word in internal/semantics string literal(s)"
    echo "$SEM_HITS"
  fi
else
  skip "criterion 14: internal/semantics not present"
fi

summarize_and_exit
