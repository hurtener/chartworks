# lib.bash — shared smoke helpers (CLAUDE.md §4.2). SOURCED by phase-NN.sh; it is
# NOT a smoke script itself (named `.bash` so the preflight `*.sh` loop never runs
# it directly).
#
# run_group collapses a package's N per-criterion `go test -run "^X$"` invocations
# into ONE `go test -run "^(A|B|…)$"` process, then recovers per-criterion OK /
# FAIL / SKIP by parsing the -v output — preserving the OK-count contract
# (OK ≥ count(criteria), FAIL=0) at a fraction of the process/compile overhead.
#
# The sourcing script must define ok(), fail(), skip() (they increment its
# OK_COUNT / FAIL_COUNT / SKIP_COUNT). A gated criterion whose test t.Skip's
# (e.g. a store URL unset) surfaces here as a genuine SKIP — no separate
# pre-check needed.

# _group_regex <name...> → ^(A|B|C)$ over the exact top-level test names.
_group_regex() {
  local re
  re="$(printf '%s|' "$@")"
  printf '^(%s)$' "${re%|}"
}

# run_group <pkg> <extra-go-test-flags> "TestName|human label" ...
#   <extra-go-test-flags> may be empty ("") or e.g. "-race".
run_group() {
  local pkg="$1"; shift
  local flags="$1"; shift
  local pairs=("$@")

  local p name label names=()
  for p in "${pairs[@]}"; do
    names+=("${p%%|*}")
  done

  # Which requested tests actually exist? An unbuilt criterion is a SKIP (not a
  # FAIL) — preserves the old per-test `-list` guard so a growing build stays green.
  local list_out
  list_out="$(go test -list "$(_group_regex "${names[@]}")" "$pkg" 2>/dev/null || true)"

  local present_pairs=()
  for p in "${pairs[@]}"; do
    name="${p%%|*}"
    label="${p#*|}"
    if printf '%s\n' "$list_out" | grep -qx "$name"; then
      present_pairs+=("$p")
    else
      skip "$label ($name not present yet)"
    fi
  done
  [ "${#present_pairs[@]}" -eq 0 ] && return 0

  names=()
  for p in "${present_pairs[@]}"; do
    names+=("${p%%|*}")
  done

  # ONE process for the whole present group. `|| true` so a FAIL inside never
  # aborts under `set -e`; the per-test parse below is the source of truth.
  local out
  # shellcheck disable=SC2086 # word-splitting of $flags is intentional.
  out="$(go test $flags -v -count=1 -run "$(_group_regex "${names[@]}")" "$pkg" 2>&1 || true)"

  for p in "${present_pairs[@]}"; do
    name="${p%%|*}"
    label="${p#*|}"
    if printf '%s\n' "$out" | grep -qE "^[[:space:]]*--- FAIL: ${name} \("; then
      fail "$label ($name)"
    elif printf '%s\n' "$out" | grep -qE "^[[:space:]]*--- SKIP: ${name} \("; then
      skip "$label ($name)"
    elif printf '%s\n' "$out" | grep -qE "^[[:space:]]*--- PASS: ${name} \("; then
      ok "$label ($name)"
    else
      # No result line: a build failure, a panic, or the name matched nothing.
      # That is a FAIL (never a silent pass). `make build` in preflight catches a
      # genuine compile break first.
      fail "$label ($name — no PASS/FAIL/SKIP line; build error or panic?)"
    fi
  done
}
