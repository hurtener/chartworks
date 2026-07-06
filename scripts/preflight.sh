#!/usr/bin/env bash
set -euo pipefail

# The preflight gate — CLAUDE.md §4.1. The same gate the pre-commit hook and
# CI enforce: build, the per-phase smoke scripts (SKIPping gracefully where the
# surface isn't built yet), then drift-audit.
#
# MODES
#   default (fast) — the pre-commit hook's mode. Runs build + drift-audit always,
#     but only the smoke scripts for phases whose OWNING packages changed vs HEAD
#     (git diff --name-only HEAD, plus untracked files). A change to a global build
#     input (go.mod/go.sum/Makefile/.golangci.yml) or to drift-audit/preflight/
#     coverage itself forces a full run; a change to a phase's own smoke script
#     re-runs that phase. This keeps the commit loop fast without losing coverage
#     of what actually changed.
#   PREFLIGHT_FULL=1 — the CI and wave-end mode. Runs every smoke script
#     regardless of the diff (the §14 pre-merge checklist expects this).
#
# The OK-count contract is unchanged: preflight fails if any smoke reports FAIL,
# any smoke script exits non-zero, or drift-audit fails.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

FULL="${PREFLIGHT_FULL:-0}"

echo "== preflight: build =="
if ! make build; then
  echo "FAIL: make build"
  exit 1
fi

# --- fast-mode change detection ---------------------------------------------
# CHANGED is the set of files that differ from HEAD (tracked modifications) plus
# untracked-but-not-ignored files (a brand-new source/test file in a package must
# still trigger that package's phase). Empty (or unavailable git) ⇒ no phase is
# selected in fast mode; a forced-full path ⇒ every phase runs.
CHANGED=""
FORCE_FULL=0
if [ "$FULL" -ne 1 ]; then
  # PREFLIGHT_CHANGED (optional) overrides the git diff — a newline/space list of
  # repo-relative paths. CI can pass the PR's changed files; a test can inject a
  # scenario. When unset, the change set is `git diff --name-only HEAD` plus
  # untracked-but-not-ignored files (a brand-new source/test file must still
  # trigger its package's phase).
  if [ -n "${PREFLIGHT_CHANGED+x}" ]; then
    CHANGED="$(printf '%s\n' $PREFLIGHT_CHANGED | sort -u)"
  elif CHANGED="$( { git diff --name-only HEAD 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null; } | sort -u )"; then
    :
  else
    # git unavailable / not a repo: be safe and run everything.
    FORCE_FULL=1
  fi
  # A change to a global build input, or to the shared smoke helper library
  # (which every collapsed smoke sources), forces a full run — the blast radius is
  # every phase. (drift-audit runs unconditionally; a change to preflight.sh or a
  # single phase smoke only re-runs the relevant surface, handled below.)
  if printf '%s\n' "$CHANGED" | grep -qE '^(go\.mod|go\.sum|Makefile|\.golangci\.yml|scripts/smoke/lib\.bash)$'; then
    FORCE_FULL=1
  fi
fi

# phase_owning_paths maps a phase number to the repo-relative directory prefixes
# whose changes should re-run that phase's smoke (derived from the packages each
# smoke script exercises). Empty until Chartworks' own phase plans land — populate
# this the same way Soundings did, one case per phase-NN, as docs/plans/phase-NN-*.md
# and scripts/smoke/phase-NN.sh are authored (CLAUDE.md §16).
phase_owning_paths() {
  case "$1" in
    *) echo "" ;;
  esac
}

# should_run_phase decides whether a phase's smoke runs in the current mode.
should_run_phase() {
  local nn="$1"
  [ "$FULL" -eq 1 ] && return 0
  [ "$FORCE_FULL" -eq 1 ] && return 0
  # The phase's own smoke script changed → re-run it.
  if printf '%s\n' "$CHANGED" | grep -q "^scripts/smoke/phase-${nn}\.sh$"; then
    return 0
  fi
  local p
  for p in $(phase_owning_paths "$nn"); do
    if printf '%s\n' "$CHANGED" | grep -q "^${p}/"; then
      return 0
    fi
  done
  return 1
}

echo
if [ "$FULL" -eq 1 ]; then
  echo "== preflight: smoke scripts (FULL) =="
elif [ "$FORCE_FULL" -eq 1 ]; then
  echo "== preflight: smoke scripts (fast → forced full: a global build input changed) =="
else
  echo "== preflight: smoke scripts (fast: only phases whose owning packages changed vs HEAD) =="
fi

TOTAL_OK=0
TOTAL_FAIL=0
TOTAL_SKIP=0
ANY_SMOKE=0
SMOKE_SCRIPT_FAIL=0

if [ -d scripts/smoke ]; then
  for f in scripts/smoke/*.sh; do
    [ -f "$f" ] || continue
    base="$(basename "$f")"
    if [ "$base" = "_template.sh" ]; then
      continue
    fi
    # live.sh is the manual/wave-end live-verification gate (docs/live-verification.md):
    # it spends real money against a live provider and must NEVER run as part of
    # `make preflight` or the pre-commit hook, so it is excluded here exactly like
    # _template.sh rather than by the phase-NN naming convention.
    if [ "$base" = "live.sh" ]; then
      continue
    fi
    ANY_SMOKE=1
    # Extract NN from phase-NN.sh; a non-conforming name always runs.
    nn="${base#phase-}"
    nn="${nn%.sh}"
    if [ "$base" = "phase-${nn}.sh" ] && ! should_run_phase "$nn"; then
      echo "--- $f (fast: skipped — no owning-package change) ---"
      echo "SKIP: phase-${nn} not selected (fast mode)"
      TOTAL_SKIP=$((TOTAL_SKIP + 1))
      continue
    fi
    echo "--- $f ---"
    set +e
    OUTPUT="$(bash "$f" 2>&1)"
    RC=$?
    set -e
    echo "$OUTPUT"
    N_OK=$(printf '%s\n' "$OUTPUT" | grep -c '^OK' || true)
    N_FAIL=$(printf '%s\n' "$OUTPUT" | grep -c '^FAIL' || true)
    N_SKIP=$(printf '%s\n' "$OUTPUT" | grep -c '^SKIP' || true)
    TOTAL_OK=$((TOTAL_OK + N_OK))
    TOTAL_FAIL=$((TOTAL_FAIL + N_FAIL))
    TOTAL_SKIP=$((TOTAL_SKIP + N_SKIP))
    if [ "$RC" -ne 0 ]; then
      SMOKE_SCRIPT_FAIL=1
    fi
  done
fi

if [ "$ANY_SMOKE" -eq 0 ]; then
  echo "SKIP: no smoke scripts yet (besides _template.sh)"
fi

echo
echo "== preflight: drift-audit =="
DRIFT_RC=0
bash scripts/drift-audit.sh || DRIFT_RC=$?

echo
echo "== preflight summary =="
echo "mode: $([ "$FULL" -eq 1 ] && echo FULL || { [ "$FORCE_FULL" -eq 1 ] && echo 'fast(forced-full)' || echo fast; })"
echo "smoke: OK=$TOTAL_OK FAIL=$TOTAL_FAIL SKIP=$TOTAL_SKIP"
echo "drift-audit: $([ "$DRIFT_RC" -eq 0 ] && echo PASS || echo FAIL)"

if [ "$TOTAL_FAIL" -gt 0 ] || [ "$SMOKE_SCRIPT_FAIL" -ne 0 ] || [ "$DRIFT_RC" -ne 0 ]; then
  echo "PREFLIGHT: FAIL"
  exit 1
fi

echo "PREFLIGHT: OK"
