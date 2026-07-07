#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 16 (rules-clarification), CLAUDE.md §4.2 / §16 step 7.
#
# Governed rules + proactive clarification live in internal/semantics (RFC §8.4).
# Each acceptance criterion in docs/plans/phase-16-rules-clarification.md maps to
# one go-test invocation below, collapsed into per-package run_group calls. A
# criterion whose test does not exist yet surfaces as a SKIP (the build is still
# growing), so `make preflight` stays green phase by phase.

PHASE="16"

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

# --- shared helpers ----------------------------------------------------------
SMOKE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/smoke/lib.bash
. "${SMOKE_DIR}/lib.bash"

# --- surface guard -----------------------------------------------------------
# Phase 16 is package-test-backed, not binary-backed: SKIP the whole script until
# the internal/semantics rules/clarify subpackages exist. run_group also SKIPs any
# individual not-yet-present test, so this guard just avoids a noisy `go test` on a
# missing tree.
RULES_PKG="./internal/semantics/rules"
CLARIFY_PKG="./internal/semantics/clarify"
if [ ! -d "internal/semantics/rules" ] && [ ! -d "internal/semantics/clarify" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/semantics/{rules,clarify})"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------
# Criteria 1–8, 11 (rules) live in the rules subpackage; 9–10 (clarification) in
# clarify. Criterion 11 runs under -race (concurrency + tenant isolation).

run_group "$RULES_PKG" "" \
  "TestRuleStructuralValidation|criterion 1: structural validation rejects invalid category/scope/definition" \
  "TestSensitiveLiteralEnforced|criterion 2: unmarked sensitive literal rejected, marked one redacted" \
  "TestRuleLifecycleAudited|criterion 3: proposed→active→retired audited, illegal rejected, only active selected" \
  "TestRulesLaneBudgetIndependent|criterion 4: rules block within independent tokenizer-backed budget" \
  "TestRulePrecedenceDeterministic|criterion 5: deterministic precedence ordering" \
  "TestDroppedRulesEnumerated|criterion 6: dropped rules enumerated, never silent" \
  "TestContradictionsNamed|criterion 7: contradictions surfaced as a named field" \
  "TestCanonicalRegistryResolution|criterion 8: entity refs resolved via canonical registry, unresolvable typed error"

run_group "$RULES_PKG" "-race" \
  "TestRulesLaneConcurrentTenantIsolation|criterion 11: concurrent-safe, per-call copies, cross-tenant probe empty"

run_group "$CLARIFY_PKG" "" \
  "TestClarifyGenerationSchemaConstrained|criterion 9: clarify-role generation schema-constrained, malformed rejected, pack-stored" \
  "TestClarificationSlotsProduced|criterion 10: ambiguous fixture ⇒ named slots, unambiguous ⇒ none"

summarize_and_exit
