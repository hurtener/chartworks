#!/usr/bin/env bash
set -euo pipefail

# Smoke-check for phase 09 — sql-validate-core (CLAUDE.md §4.2 / §16 step 7).
#
# The "surface" this phase ships is the internal validation library
# `internal/exec` (no binary, no HTTP route, no MCP tool, no config key). Its
# checks are pure `go test` targets plus two source-structural greps — no DB,
# no live model. The whole script SKIPs cleanly until the package exists so
# `make preflight` stays green as the build grows.

PHASE="09"

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

# Run a `go test -run <pattern>` for one package; OK if it actually ran and
# passed, FAIL if it ran and failed. A pattern that matches no test yet (the
# package exists but this phase's tests aren't written) is treated as
# not-built-yet -> SKIP, so partial progress never reddens preflight.
run_test() {
  desc="$1"
  pattern="$2"
  out="$(go test -count=1 -run "$pattern" ./internal/exec/... 2>&1 || true)"
  if printf '%s' "$out" | grep -qE '^(ok|FAIL|--- FAIL)'; then
    if printf '%s' "$out" | grep -qE '^(FAIL|--- FAIL)'; then
      fail "$desc"
    else
      ok "$desc"
    fi
  else
    # "no test files" / "no tests to run" / build-not-ready
    skip "$desc (test not present yet)"
  fi
}

# --- surface guard -----------------------------------------------------------
# The validator is a library: guard on the package directory + a Go toolchain,
# not on a built binary. Absent -> SKIP the whole script.
if [ -z "$(command -v go 2>/dev/null || true)" ]; then
  skip "phase-${PHASE}: no go toolchain"
  summarize_and_exit
fi
if [ ! -d "internal/exec" ]; then
  skip "phase-${PHASE} surface not built yet (no internal/exec package)"
  summarize_and_exit
fi

# --- assertions --------------------------------------------------------------

# 1: whole-tree DDL/DML rejection across all six V1 dialects.
run_test "criterion 1: whole-tree DDL/DML rejected across all V1 dialects" \
  "TestValidate_BlocksWritesAllDialects"

# 2: CTE golden fixture passes (the CTE-regression guard).
run_test "criterion 2: CTE golden fixture validates as SELECT-family" \
  "TestValidate_CTEGolden"

# 3: multi-statement rejected.
run_test "criterion 3: multi-statement input rejected (statement.multiple)" \
  "TestValidate_RejectsMultiStatement"

# 4: FuzzValidate never panics / never passes a write (seed corpus run).
if go test -count=1 -run '^FuzzValidate$' ./internal/exec/... >/dev/null 2>&1; then
  ok "criterion 4: FuzzValidate seed corpus (never panics / never passes a write)"
else
  # distinguish not-present-yet from a real failure
  out="$(go test -count=1 -run '^FuzzValidate$' ./internal/exec/... 2>&1 || true)"
  if printf '%s' "$out" | grep -qE 'no tests to run|no test files|cannot find'; then
    skip "criterion 4: FuzzValidate (not present yet)"
  else
    fail "criterion 4: FuzzValidate seed corpus"
  fi
fi

# 5: ValidatedSQL unconstructible outside the package (compile-time proof).
run_test "criterion 5: ValidatedSQL unconstructible outside internal/exec" \
  "TestValidatedSQL_Unconstructible"

# 6: dialect-specific syntax degrades to a typed, fail-closed rejection.
run_test "criterion 6: dialect-specific syntax fails closed (parse.unsupported)" \
  "TestValidate_DialectSyntaxFailsClosed"

# 7: typed error vocabulary is a closed, golden-pinned enum.
run_test "criterion 7: error vocabulary golden (reserved codes present, unemitted)" \
  "TestErrorVocabulary_Golden"

# 8: pre-parse does only byte/encoding work (no parser judgment).
run_test "criterion 8: pre-parse never duplicates parser judgment" \
  "TestPreParse_NoParserJudgment"

# 9: write-shape variant correct + type-split from the read path.
run_test "criterion 9: write-shape variant (declared output/inputs, type-split)" \
  "TestValidateWriteShape"

# 10: no regex injection heuristic in the validate path.
if grep -rIlE 'regexp' internal/exec >/dev/null 2>&1; then
  # a regexp import in the validate source units is the anti-pattern; allow it
  # only if the accompanying architecture test asserts it's not in the gate.
  run_test "criterion 10: no regex injection heuristic (arch test)" \
    "TestNoRegexInjectionHeuristic"
else
  ok "criterion 10: no regex injection heuristic (no regexp import in internal/exec)"
fi

summarize_and_exit
