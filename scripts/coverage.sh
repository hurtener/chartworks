#!/usr/bin/env bash
set -euo pipefail

# Coverage band gate — CLAUDE.md §11 / §14.
#
# Reads scripts/coverage-bands.conf (<import-path-suffix> <min-percent> per
# line) and enforces it per package. SKIPs gracefully when there is nothing
# to gate yet (no conf entries, no go.mod, no Go packages). Once bands exist,
# ANY internal/ or cmd/ package with .go files but no configured band is a
# mechanical FAIL — CLAUDE.md §11: "a new package with no configured
# threshold fails the build."

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CONF="scripts/coverage-bands.conf"

if [ ! -f "$CONF" ]; then
  echo "SKIP: coverage — $CONF not found"
  exit 0
fi

ENTRIES="$(grep -v '^[[:space:]]*#' "$CONF" 2>/dev/null | grep -v '^[[:space:]]*$' || true)"

if [ -z "$ENTRIES" ]; then
  echo "SKIP: coverage — no bands configured yet in $CONF"
  exit 0
fi

if ! command -v go >/dev/null 2>&1; then
  echo "SKIP: coverage — go toolchain not found"
  exit 0
fi

if [ ! -f go.mod ]; then
  echo "SKIP: coverage — no go.mod present"
  exit 0
fi

MODULE="$(go list -m 2>/dev/null || true)"
if [ -z "$MODULE" ]; then
  echo "SKIP: coverage — go.mod present but module path not resolvable"
  exit 0
fi

ALL_PKGS="$(go list ./... 2>/dev/null || true)"
if [ -z "$ALL_PKGS" ]; then
  echo "SKIP: coverage — no Go packages found yet"
  exit 0
fi

FAIL=0
CHECKED=0
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

pkg_matches_suffix() {
  # $1 = relative import path (module prefix stripped, no leading slash)
  # $2 = configured suffix
  relpath="$1"
  suffix="$2"
  if [ "$relpath" = "$suffix" ]; then
    return 0
  fi
  case "$relpath" in
    */"$suffix") return 0 ;;
  esac
  return 1
}

band_for_pkg() {
  # $1 = relative import path. Echoes the configured min-percent, or nothing.
  relpath="$1"
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    entry_suffix="$(printf '%s' "$line" | awk '{print $1}')"
    entry_min="$(printf '%s' "$line" | awk '{print $2}')"
    if pkg_matches_suffix "$relpath" "$entry_suffix"; then
      printf '%s\n' "$entry_min"
      return 0
    fi
  done <<EOF_ENTRIES
$ENTRIES
EOF_ENTRIES
  return 1
}

while IFS= read -r pkg; do
  [ -z "$pkg" ] && continue
  case "$pkg" in
    "$MODULE"/internal/*) ;;
    "$MODULE"/cmd/*) ;;
    "$MODULE"/sdk/*) ;;
    "$MODULE"/eval/*) ;;
    "$MODULE"/eval) ;;
    *) continue ;;
  esac
  relpath="${pkg#"$MODULE"/}"

  min="$(band_for_pkg "$relpath" || true)"
  if [ -z "$min" ]; then
    echo "FAIL: $relpath — no coverage band configured in $CONF (mechanical gate, CLAUDE.md §11)"
    FAIL=1
    continue
  fi

  CHECKED=$((CHECKED + 1))
  safe_name="$(printf '%s' "$relpath" | tr '/' '_')"
  profile="$TMPDIR/${safe_name}.out"
  test_log="$TMPDIR/${safe_name}.log"

  if ! go test -race -coverprofile="$profile" "$pkg" >"$test_log" 2>&1; then
    cat "$test_log"
    echo "FAIL: $relpath — go test failed"
    FAIL=1
    continue
  fi

  pct_raw="$(go tool cover -func="$profile" 2>/dev/null | tail -1 | awk '{print $NF}' | tr -d '%')"
  if [ -z "$pct_raw" ]; then
    pct_raw="0"
  fi
  pct_int="${pct_raw%.*}"
  [ -z "$pct_int" ] && pct_int="0"

  if [ "$pct_int" -lt "$min" ]; then
    echo "FAIL: $relpath — coverage ${pct_raw}% < required ${min}%"
    FAIL=1
  else
    echo "OK: $relpath — coverage ${pct_raw}% >= required ${min}%"
  fi
done <<EOF_PKGS
$ALL_PKGS
EOF_PKGS

if [ "$CHECKED" -eq 0 ] && [ "$FAIL" -eq 0 ]; then
  echo "SKIP: coverage — no internal/ or cmd/ packages exist yet to gate"
  exit 0
fi

if [ "$FAIL" -ne 0 ]; then
  echo "COVERAGE: FAIL"
  exit 1
fi

echo "COVERAGE: OK ($CHECKED package(s) checked)"
