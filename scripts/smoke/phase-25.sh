#!/usr/bin/env bash
set -euo pipefail

# Smoke-check — Phase 25 `e2e-release` (CLAUDE.md §4.2 / §16 step 7).
#
# This is the release phase: it ships no product surface, so its smoke does
# cheap MECHANICAL checks on the release artifacts (Dockerfile / compose / docs /
# README / CHANGELOG / the cumulative-audit manifest) and gates the heavy runs —
# the actual E2E suites, the docker build, and the live-provider gate — behind
# E2E=1 / LIVE=1 so `make preflight` stays fast. The full runs execute under
# `make e2e`, the CI `preflight-full` job, and the release checklist.
#
# The script SKIPs entirely until the release artifacts exist, so the build can
# grow phase by phase with a green preflight.

PHASE="25"

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

E2E="${E2E:-0}"
LIVE="${LIVE:-0}"

# --- surface guard -----------------------------------------------------------
# The release surface is "built" once ANY release artifact exists. Until then,
# an unbuilt release is not a broken one — SKIP the whole script.
if [ ! -f "Dockerfile" ] && [ ! -f "docker-compose.e2e.yml" ] && [ ! -d "test/e2e" ]; then
  skip "phase-${PHASE} release surface not built yet (no Dockerfile / docker-compose.e2e.yml / test/e2e)"
  summarize_and_exit
fi

# --- criterion 1: self-issue E2E ---------------------------------------------
if [ -d "test/e2e" ] && go test -list 'TestE2ESelfIssue' ./test/e2e/... 2>/dev/null | grep -q 'TestE2ESelfIssue'; then
  if [ "$E2E" = "1" ]; then
    if go test -race -p 1 -count=1 -run '^TestE2ESelfIssue$' ./test/e2e/... >/dev/null 2>&1; then
      ok "criterion 1: TestE2ESelfIssue green (full self-issue walk, fresh DB)"
    else
      fail "criterion 1: TestE2ESelfIssue failed"
    fi
  else
    ok "criterion 1: TestE2ESelfIssue present (set E2E=1 to run the full walk)"
  fi
else
  skip "criterion 1: TestE2ESelfIssue not defined yet"
fi

# --- criterion 2: external-issuer E2E + JWKS stub ----------------------------
if [ -d "test/e2e" ] && go test -list 'TestE2EExternalIssuer' ./test/e2e/... 2>/dev/null | grep -q 'TestE2EExternalIssuer'; then
  if [ "$E2E" = "1" ]; then
    if go test -race -p 1 -count=1 -run '^TestE2EExternalIssuer$' ./test/e2e/... >/dev/null 2>&1; then
      ok "criterion 2: TestE2EExternalIssuer green (JWKS stub, fresh DB, fail-closed paths)"
    else
      fail "criterion 2: TestE2EExternalIssuer failed"
    fi
  else
    ok "criterion 2: TestE2EExternalIssuer present (set E2E=1 to run it)"
  fi
else
  skip "criterion 2: TestE2EExternalIssuer not defined yet"
fi

# --- criterion 3: CGo-free static binary (D-005) -----------------------------
if [ -f "Dockerfile" ]; then
  if grep -q 'CGO_ENABLED=0' Dockerfile; then
    if [ "$E2E" = "1" ] && [ -x "bin/chartworks" ]; then
      if file bin/chartworks | grep -qiE 'statically linked|static-pie'; then
        ok "criterion 3: Dockerfile sets CGO_ENABLED=0 and the binary is statically linked"
      else
        fail "criterion 3: binary is not statically linked (CGo may have crept in — D-005)"
      fi
    else
      ok "criterion 3: Dockerfile sets CGO_ENABLED=0 (set E2E=1 with a built bin/chartworks to prove static linkage)"
    fi
  else
    fail "criterion 3: Dockerfile does not set CGO_ENABLED=0 (D-005)"
  fi
else
  skip "criterion 3: Dockerfile not present yet"
fi

# --- criterion 4: non-root + /readyz healthcheck -----------------------------
if [ -f "Dockerfile" ]; then
  if grep -qiE '^\s*USER\s+' Dockerfile && ! grep -qiE '^\s*USER\s+root\b' Dockerfile \
     && grep -qi 'HEALTHCHECK' Dockerfile && grep -q '/readyz' Dockerfile; then
    ok "criterion 4: Dockerfile declares a non-root USER and a /readyz HEALTHCHECK"
  else
    fail "criterion 4: Dockerfile must declare a non-root USER and a HEALTHCHECK targeting /readyz"
  fi
else
  skip "criterion 4: Dockerfile not present yet"
fi

# --- criterion 5: from-scratch compose serves + smokes -----------------------
if [ -f "docker-compose.e2e.yml" ]; then
  if [ "$E2E" = "1" ]; then
    if command -v docker >/dev/null 2>&1; then
      if docker compose -f docker-compose.e2e.yml up --build -d >/dev/null 2>&1; then
        # poll /readyz for a bounded window, then tear down.
        ready=0
        for _ in $(seq 1 30); do
          if curl -fsS http://localhost:8080/readyz >/dev/null 2>&1; then ready=1; break; fi
          sleep 2
        done
        docker compose -f docker-compose.e2e.yml down -v >/dev/null 2>&1 || true
        if [ "$ready" = "1" ]; then
          ok "criterion 5: docker compose up --build serves and /readyz is ready from scratch"
        else
          fail "criterion 5: container never reached /readyz ready"
        fi
      else
        fail "criterion 5: docker compose up --build failed"
      fi
    else
      skip "criterion 5: docker unavailable in this environment"
    fi
  else
    ok "criterion 5: docker-compose.e2e.yml present (set E2E=1 to build + serve from scratch)"
  fi
else
  skip "criterion 5: docker-compose.e2e.yml not present yet"
fi

# --- criterion 6: preflight-full wiring --------------------------------------
if grep -q '^preflight-full:' Makefile 2>/dev/null; then
  ok "criterion 6: make preflight-full target present (CI/wave-end runs the full smoke sweep)"
else
  fail "criterion 6: make preflight-full target missing"
fi

# --- criterion 7: cumulative audit resolved ----------------------------------
if [ -f "scripts/audit-cumulative.sh" ] && [ -f "docs/audit/wave-7-cumulative.md" ]; then
  open_items="$(grep -c '^\s*- \[ \]' docs/audit/wave-7-cumulative.md 2>/dev/null || true)"
  open_items="${open_items:-0}"
  if [ "$open_items" -eq 0 ] && bash scripts/audit-cumulative.sh >/dev/null 2>&1; then
    ok "criterion 7: cumulative audit script exits 0 and the manifest has no open item"
  else
    fail "criterion 7: cumulative audit has open items or the script exits non-zero"
  fi
else
  skip "criterion 7: cumulative audit artifacts not present yet"
fi

# --- criterion 8: product README in the family voice -------------------------
if [ -f "README.md" ] \
   && grep -qiE '## Why Chartworks exists' README.md \
   && grep -qiE '## The ask call' README.md \
   && grep -qiE '## Core ideas' README.md \
   && grep -qiE '## Where Chartworks fits' README.md \
   && grep -qiE '## Quickstart' README.md; then
  ok "criterion 8: README carries the family-voice sections (why / ask call / core ideas / where it fits / quickstart)"
else
  skip "criterion 8: product README not yet rewritten in the family voice"
fi

# --- criterion 9: CHANGELOG 0.1.0 + tag procedure ----------------------------
if [ -f "CHANGELOG.md" ] && grep -qE '## \[0\.1\.0\]' CHANGELOG.md; then
  ok "criterion 9: CHANGELOG.md carries a [0.1.0] section"
else
  skip "criterion 9: CHANGELOG.md has no [0.1.0] section yet"
fi

# --- criterion 10: ops docs --------------------------------------------------
if [ -f "docs/ops.md" ]; then
  ok "criterion 10: docs/ops.md operator runbook present"
else
  skip "criterion 10: docs/ops.md not present yet"
fi

# --- criterion 11: live gate as release blocker (D-010) ----------------------
if [ -f ".env.example" ] || grep -q '^live-gate:\|LIVE=1' Makefile 2>/dev/null; then
  if [ "$LIVE" = "1" ]; then
    if [ -d "test/e2e" ] && go test -count=1 -tags=live ./test/e2e/... >/dev/null 2>&1; then
      ok "criterion 11: live gate green (-count=1, real provider via .env)"
    else
      fail "criterion 11: live gate failed"
    fi
  else
    ok "criterion 11: live-gate recipe present (set LIVE=1 with .env to run — manual, never CI)"
  fi
else
  skip "criterion 11: live-gate recipe (.env.example / Makefile target) not present yet"
fi

summarize_and_exit
