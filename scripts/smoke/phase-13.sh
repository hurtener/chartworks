#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — Phase 13 (engineering-pipelines). CLAUDE.md §4.2 / §16 step 7.
#
# One assertion per acceptance criterion in
# docs/plans/phase-13-engineering-pipelines.md. The whole script SKIPs cleanly
# until the internal/engineering package exists, and each individual criterion
# SKIPs until its test is present (via run_group in scripts/smoke/lib.bash) —
# so `make preflight` stays green as the build grows phase by phase.
#
# This script is deliberately excluded from scripts/preflight.sh's own runner
# only via the standard *.sh loop; it lives alongside every other phase smoke.

PHASE="13"

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

# Run from the repo root so ./internal/... package paths resolve regardless of
# the caller's working directory.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/../.."

# shellcheck source=scripts/smoke/lib.bash
. "${SCRIPT_DIR}/lib.bash"

# --- surface guard -----------------------------------------------------------
# Phase 13 owns core in internal/engineering (no new binary/CLI/route). SKIP the
# whole script until that package exists and builds — an unbuilt surface is not
# a broken one.
PKG="./internal/engineering"
if ! command -v go >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not testable yet (go toolchain unavailable)"
  summarize_and_exit
fi
if [ ! -d "internal/engineering" ] || ! go list "$PKG" >/dev/null 2>&1; then
  skip "phase-${PHASE} surface not built yet (no internal/engineering package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# One criterion per run_group entry; -race per CLAUDE.md §11. A not-yet-written
# test surfaces as SKIP; a t.Skip'd test (store URL unset, pinned bruin binary
# absent) surfaces as SKIP.
run_group "$PKG" "-race" \
  "TestPipelineStepWriteOutOfScopeRejected|criterion 1: step write outside declared output rejected" \
  "TestPipelineStepInputNotDeclaredRejected|criterion 2: step read of undeclared input rejected" \
  "TestRunUndeclaredDestinationRejectedBeforeRender|criterion 3: undeclared destination rejected before render" \
  "TestQualityCheckFailureFailsRunLoudly|criterion 4: failed quality check fails run loudly" \
  "TestP1cWriteSplitArchitecture|criterion 5: nlq/exec cannot reach PipelineRunner (P1c strengthened)" \
  "TestLineageRecordedPerMaterialization|criterion 6: lineage recorded per materialization" \
  "TestFreshnessStampedOnMaterialization|criterion 7: freshness stamped on materialization" \
  "TestDraftPublicationGate|criterion 8: draft-only publication gate" \
  "TestPipelineVersionImmutable|criterion 9: pipeline definition versioned + immutable" \
  "TestRenderGatedByBruinValidate|criterion 10: render from WriteValidatedSQL only, gated by bruin validate" \
  "TestSQLOnlyAssetEnforcement|criterion 11: Python/R/ingestion assets rejected (SQL-only, D-036)" \
  "TestNoPlaintextSecretPersists|criterion 12: plaintext secrets never persist (ENV/tmpfs only)" \
  "TestBruinTelemetryDisabledOnEveryInvocation|criterion 13: bruin telemetry disabled on every invocation" \
  "TestBruinOutcomeMappingTyped|criterion 14: exit codes/per-asset results map to typed outcomes" \
  "TestStrategyEngineCompatibility|criterion 15: strategy/engine compatibility (merge-on-Databricks rejected)" \
  "TestMissingBruinDegradesLoud|criterion 16: missing bruin binary degrades loud (typed unavailable)" \
  "TestScheduleAttachmentEnqueues|criterion 17: schedule attachment enqueues on the queue" \
  "TestCanonicalRegistryResolveThenCompare|criterion 18: canonical registry resolve-then-compare, fail-closed"

summarize_and_exit
