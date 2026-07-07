#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — phase 01 `binary-config-telemetry` (CLAUDE.md §4.2 / §16 step 7).
#
# Each acceptance criterion in docs/plans/phase-01-binary-config-telemetry.md maps to
# one ok()/fail() call below. The script SKIPs *entirely* until the surface it tests is
# built (no chartworks binary) so `make preflight` runs cleanly as the build grows.
#
# Excluded from scripts/preflight.sh's *.sh loop only via the surface guard below —
# today, nothing is built, so every assertion is guarded out and the script SKIPs.

PHASE="01"

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

# Shared helper: run_group collapses N per-criterion `go test -run` into one process
# and recovers per-criterion OK/FAIL/SKIP. Sourced only when present (it ships with the
# scaffolding).
# shellcheck source=/dev/null
[ -f scripts/smoke/lib.bash ] && . scripts/smoke/lib.bash

# --- surface guard -----------------------------------------------------------
# Phase 01's surface is the `chartworks` binary (config + telemetry + CLI). Until it is
# built, SKIP the whole script — an unbuilt surface is not a broken one.
BIN="bin/chartworks"
if [ ! -x "$BIN" ] && [ -z "$(command -v chartworks 2>/dev/null || true)" ]; then
  skip "phase-${PHASE} surface not built yet (no chartworks binary)"
  summarize_and_exit
fi
# Prefer the repo-local build; fall back to a chartworks on PATH.
[ -x "$BIN" ] || BIN="$(command -v chartworks)"

# --- assertions ----------------------------------------------------------------
# One ok()/fail() (or run_group) per acceptance criterion. Commented until the phase
# lands; uncomment as each criterion becomes checkable in the implementation PR.

# Criterion 1 — version / --version print ldflags-injected build metadata, exit 0.
# if "$BIN" version | grep -qE '.' && "$BIN" --version | grep -qE '.'; then
#   ok "1: version and --version print build metadata"
# else
#   fail "1: version / --version output missing"
# fi

# Criterion 2 — serve --check validates the shipped example config, exits 0, no socket.
# if "$BIN" serve --config config.example.yaml --check 2>/dev/null | grep -q '^OK'; then
#   ok "2: serve --check accepts config.example.yaml"
# else
#   fail "2: serve --check rejected the shipped example config"
# fi

# Criterion 3 — unknown config key ⇒ non-zero exit + ErrUnknownKey on stderr.
# tmp="$(mktemp)"; printf 'env: dev\nbogus_key: 1\n' > "$tmp"
# if ! "$BIN" serve --config "$tmp" --check 2>&1 | grep -qi 'ErrUnknownKey\|unknown key'; then
#   fail "3: unknown key not rejected with ErrUnknownKey"
# else
#   ok "3: unknown config key rejected"
# fi; rm -f "$tmp"

# Criterion 4 — required env-indirected secret unset ⇒ ErrMissingSecret, non-zero exit.
# if env -u CHARTWORKS_STORE_DSN "$BIN" serve --config config.example.yaml --check 2>&1 \
#      | grep -qi 'ErrMissingSecret\|missing secret'; then
#   ok "4: missing required secret refuses boot"
# else
#   fail "4: missing required secret did not refuse boot"
# fi

# Criterion 5 — env: indirection round-trip (in-package).
# run_group internal/config "" "TestLoad_EnvIndirection|5: env: indirection resolves a set var"

# Criterion 6 — aggregate validation reports all failures (errors.Join).
# run_group internal/config "" "TestLoad_AggregateErrors|6: aggregate validation is fail-loud-complete"

# Criterion 7 — slog format selection + secret redaction.
# run_group internal/telemetry "" \
#   "TestNew_LogFormat|7a: log_format selects JSON/text" \
#   "TestRedaction|7b: secrets/token bytes redacted"

# Criterion 8 — telemetry conformance: manifest ⊆ exposition; detects unexported metric.
# run_group internal/telemetry "" \
#   "TestMetricsConformance|8a: every registered metric is exported" \
#   "TestMetricsConformance_DetectsUnexported|8b: unexported metric fails conformance"

# Criterion 9 — request-id is correlation-only; AuditEvent is content-free.
# run_group internal/telemetry "" \
#   "TestRequestID_NoIdentityFromHeader|9a: no identity derived from a header" \
#   "TestAuditEvent_ContentFreeShape|9b: AuditEvent has no free-form data field"

# Criterion 10 — run() dispatch; unknown subcommand ⇒ non-zero + usage, no panic.
# if ! "$BIN" bogus-subcommand >/dev/null 2>&1; then
#   ok "10a: unknown subcommand exits non-zero"
# else
#   fail "10a: unknown subcommand did not fail"
# fi
# run_group cmd/chartworks "" "TestRun_Dispatch|10b: run(args,stdout,stderr) dispatch table"

# Criterion 11 — CGo-free static build of the binary (D-005).
# if CGO_ENABLED=0 go build -o /dev/null ./cmd/chartworks 2>/dev/null; then
#   ok "11: CGO_ENABLED=0 build succeeds"
# else
#   fail "11: CGo-free build failed"
# fi

# Criterion 12 — the shipped example config is itself valid.
# run_group internal/config "" "TestExampleConfig_Valid|12: config.example.yaml loads clean"

summarize_and_exit
