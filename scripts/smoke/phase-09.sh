#!/usr/bin/env bash
set -euo pipefail

# Smoke-check for phase 09 — sql-validate-core (CLAUDE.md §4.2 / §16 step 7).
#
# The "surface" this phase ships is the internal validation library
# `internal/exec` — layers 1–2 of the D-038 layered validation (tokenizer
# screens + the parser seam) plus the ValidatedSQL layer record. Its checks
# are pure `go test` targets plus source-structural greps — no binary, no DB,
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

# Run a `go test -run <pattern>` for the exec package; OK if it ran and
# passed, FAIL if it ran and failed. A pattern matching no test yet (package
# exists, this phase's tests not written) is not-built-yet -> SKIP, so
# partial progress never reddens preflight.
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

# 1: conformance-reproduction harness gates parser-driver adoption.
run_test "criterion 1: conformance harness gates driver adoption (report golden + generated table)" \
  "TestConformanceHarness_GatesAdoption"

# 2: whole-tree DDL/DML rejection for every adopted (dialect, driver) pair.
run_test "criterion 2: whole-tree DDL/DML rejected across adopted V1 dialects (zero ast-passed)" \
  "TestValidate_BlocksWritesAllDialects"

# 3: CTE golden fixture passes on both parser drivers.
run_test "criterion 3: CTE golden fixture validates on crdb and sqlglotgo drivers" \
  "TestValidate_CTEGolden"

# 4: tokenizer screens — dialect-aware single-statement + quote/comment hygiene.
run_test "criterion 4: tokenizer screens (multi-statement dialect-aware, hygiene, caps)" \
  "TestTokenizer_Screens"

# 5: FuzzValidate — never panics; ast-passed never contains a write; honest record.
if go test -count=1 -run '^FuzzValidate$' ./internal/exec/... >/dev/null 2>&1; then
  ok "criterion 5: FuzzValidate seed corpus (no panic, no ast-passed write, honest layer record)"
else
  out="$(go test -count=1 -run '^FuzzValidate$' ./internal/exec/... 2>&1 || true)"
  if printf '%s' "$out" | grep -qE 'no tests to run|no test files|cannot find'; then
    skip "criterion 5: FuzzValidate (not present yet)"
  else
    fail "criterion 5: FuzzValidate seed corpus"
  fi
fi

# 6: ValidatedSQL unconstructible + engine-pending layer-record contract.
run_test "criterion 6a: ValidatedSQL unconstructible outside internal/exec" \
  "TestValidatedSQL_Unconstructible"
run_test "criterion 6b: layer record engine-pending refusal contract (phase-10 interface expectation)" \
  "TestLayerRecord_EnginePendingContract"

# 7: AST-skip is typed and fail-honest; vocabulary golden-pinned.
run_test "criterion 7a: unproven dialect/input yields ast-skipped + parse.unsupported marker" \
  "TestValidate_ASTSkipTypedMarker"
run_test "criterion 7b: error/marker vocabulary golden (reserved codes present, unemitted)" \
  "TestErrorVocabulary_Golden"

# 8: tokenizer never duplicates parser judgment.
run_test "criterion 8: tokenizer never duplicates parser judgment (arch test)" \
  "TestTokenizer_NoParserJudgment"

# 9: write-shape variant correct + type-split from the read path.
run_test "criterion 9: write-shape variant (declared output/inputs, ValidatedWriteSQL type-split)" \
  "TestValidateWriteShape"

# 10: dialect escapes caught structurally; no regex heuristic in the gate.
run_test "criterion 10a: dialect-escape corpus never yields an ast-passed ValidatedSQL" \
  "TestDialectEscapes_Structural"
if grep -rIlE 'regexp' internal/exec >/dev/null 2>&1; then
  # a regexp import in the validate source units is the anti-pattern; allow it
  # only if the accompanying architecture test asserts it's not in the gate.
  run_test "criterion 10b: no regex injection heuristic (arch test)" \
    "TestNoRegexInjectionHeuristic"
else
  ok "criterion 10b: no regex injection heuristic (no regexp import in internal/exec)"
fi

summarize_and_exit
