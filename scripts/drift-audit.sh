#!/usr/bin/env bash
set -euo pipefail

# Mechanical design-coherence checks — CLAUDE.md §14 pre-merge checklist,
# §16 step 8. Each check prints OK/FAIL/SKIP lines; the script exits
# non-zero iff any check FAILs.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

OK_COUNT=0
FAIL_COUNT=0

ok() {
  echo "OK: $1"
  OK_COUNT=$((OK_COUNT + 1))
}

fail() {
  echo "FAIL: $1"
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

echo "== drift-audit =="

# --- 1. mirror: AGENTS.md == CLAUDE.md --------------------------------------
if [ -f AGENTS.md ] && [ -f CLAUDE.md ]; then
  if diff -q AGENTS.md CLAUDE.md >/dev/null 2>&1; then
    ok "mirror: AGENTS.md and CLAUDE.md are byte-identical"
  else
    fail "mirror: AGENTS.md and CLAUDE.md differ (run: diff AGENTS.md CLAUDE.md)"
  fi
else
  fail "mirror: AGENTS.md and/or CLAUDE.md missing"
fi

# --- 2. forbidden predecessor naming in Go source ---------------------------
# Chartworks has TWO Python predecessors under _ref/ (00_KICKSTART-PROMPT.md):
# _ref/original_wayfinder (the client predecessor) and
# _ref/forked_wayfinder_explorer (the generalistic fork). Neither predecessor
# name may appear in Go source — CLAUDE.md "Predecessor hygiene". Both share
# the root word "wayfinder", so a single case-insensitive grep for it covers
# both names (and any "wayfinder-explorer"/"wayfinder_explorer" variant).
PREDECESSOR_WORD="wayfinder"
CODE_DIRS=""
for d in cmd internal sdk eval test examples; do
  if [ -d "$d" ]; then
    CODE_DIRS="$CODE_DIRS $d"
  fi
done

if [ -z "$CODE_DIRS" ]; then
  echo "SKIP: predecessor-naming — no code dirs (cmd/ internal/ sdk/ eval/ test/ examples/) exist yet"
else
  # shellcheck disable=SC2086
  HITS="$(grep -rIn --include='*.go' -i -- "$PREDECESSOR_WORD" $CODE_DIRS 2>/dev/null || true)"
  if [ -z "$HITS" ]; then
    ok "predecessor-naming: no '$PREDECESSOR_WORD' reference in Go source"
  else
    fail "predecessor-naming: '$PREDECESSOR_WORD' referenced in Go source"
    echo "$HITS"
  fi
fi

# --- 3. forbidden plumbing vocabulary in wire-facing strings ----------------
VOCAB_DIRS=""
for d in cmd internal/api internal/mcpserver sdk; do
  if [ -d "$d" ]; then
    VOCAB_DIRS="$VOCAB_DIRS $d"
  fi
done

if [ -z "$VOCAB_DIRS" ]; then
  echo "SKIP: plumbing-vocabulary — none of cmd/ internal/api/ internal/mcpserver/ sdk/ exist yet"
else
  VOCAB_HITS=""
  # Conservative seed list for Chartworks (the domain vocabulary itself is
  # settled by the RFC, not this script) — the terms most likely to leak as
  # plumbing nouns regardless of domain: a "collection" instead of the
  # settled noun, "wiring"/"repair" as verbs for fixing a broken link, a raw
  # "shard" or "namespace". Extend this list once the RFC's glossary exists.
  for term in '"collection' '"wiring' '"repair' '"shard' '"namespace'; do
    # shellcheck disable=SC2086
    HIT="$(grep -rIn --include='*.go' -F -- "$term" $VOCAB_DIRS 2>/dev/null || true)"
    if [ -n "$HIT" ]; then
      VOCAB_HITS="$VOCAB_HITS
$HIT"
    fi
  done
  if [ -z "$VOCAB_HITS" ]; then
    ok "plumbing-vocabulary: no forbidden plumbing term in wire-facing string literals"
  else
    fail "plumbing-vocabulary: forbidden plumbing term(s) found in string literals (CLAUDE.md domain-vocabulary rule)"
    echo "$VOCAB_HITS"
  fi

  # --- 3b. plumbing vocabulary in wire-facing NAMES/DESCRIPTIONS -------------
  # These terms (mime, embedding, artifact, upload, index, sync) also appear as
  # legitimate internal identifiers (a var named `index`), stdlib imports
  # (`"sync"`), and comments — a bare quote-prefix match would false-positive.
  # So this scan is scoped to the two surfaces the model/UI actually see: a JSON
  # struct tag's field name, and an MCP tool declaration's field name or
  # description text (the `WithString`/`WithNumber`/`WithArray`/`WithBoolean`/
  # `Description`/`WithDescription` helpers). Comments and identifiers are ignored.
  EXT_TERMS="mime embedding artifact upload index sync"
  EXT_HITS=""
  for term in $EXT_TERMS; do
    # (a) JSON struct tag whose field name IS the plumbing term (mime|mime,omitempty).
    # shellcheck disable=SC2086
    TAG_HIT="$(grep -rIn --include='*.go' -E "json:\"${term}([\",])" $VOCAB_DIRS 2>/dev/null || true)"
    # (b) MCP tool declaration line carrying the term as a whole word (field name
    #     or free-text description the model reads).
    # shellcheck disable=SC2086
    DECL_HIT="$(grep -rIn --include='*.go' -E '(WithString|WithNumber|WithArray|WithBoolean|WithDescription|Description)\(' $VOCAB_DIRS 2>/dev/null | grep -iE "\\b${term}\\b" || true)"
    if [ -n "$TAG_HIT" ]; then
      EXT_HITS="$EXT_HITS
$TAG_HIT"
    fi
    if [ -n "$DECL_HIT" ]; then
      EXT_HITS="$EXT_HITS
$DECL_HIT"
    fi
  done
  if [ -z "$EXT_HITS" ]; then
    ok "plumbing-vocabulary: no forbidden plumbing term in a wire-facing field name or description"
  else
    fail "plumbing-vocabulary: forbidden plumbing term(s) in a wire-facing field name/description (CLAUDE.md domain-vocabulary rule)"
    echo "$EXT_HITS"
  fi
fi

# --- 4. cross-reference resolution: D-NNN + research briefs ----------------
CROSSREF_FILES=""
for f in docs/plans/*.md RFC-001-Chartworks.md; do
  if [ -f "$f" ]; then
    CROSSREF_FILES="$CROSSREF_FILES $f"
  fi
done

if [ -z "$CROSSREF_FILES" ]; then
  echo "SKIP: cross-reference (D-NNN) — no docs/plans/*.md or RFC-001-Chartworks.md yet"
else
  UNRESOLVED_D=""
  # shellcheck disable=SC2086
  for f in $CROSSREF_FILES; do
    REFS="$(grep -oE 'D-[0-9]{3}' "$f" 2>/dev/null || true)"
    for ref in $REFS; do
      if [ -f docs/decisions.md ] && grep -q "### $ref" docs/decisions.md 2>/dev/null; then
        continue
      fi
      UNRESOLVED_D="$UNRESOLVED_D
$f references $ref, not found in docs/decisions.md"
    done
  done

  if [ -z "$UNRESOLVED_D" ]; then
    ok "cross-reference: every referenced D-NNN resolves in docs/decisions.md"
  else
    fail "cross-reference: unresolved D-NNN reference(s):$UNRESOLVED_D"
  fi
fi

PLAN_ONLY_FILES=""
for f in docs/plans/*.md; do
  if [ -f "$f" ]; then
    PLAN_ONLY_FILES="$PLAN_ONLY_FILES $f"
  fi
done

if [ -z "$PLAN_ONLY_FILES" ]; then
  echo "SKIP: cross-reference (research briefs) — no docs/plans/*.md yet"
else
  UNRESOLVED_BRIEF=""
  # shellcheck disable=SC2086
  for f in $PLAN_ONLY_FILES; do
    BRIEFS="$(grep -oE 'docs/research/[0-9]{2}-[A-Za-z0-9_-]*(\.md)?' "$f" 2>/dev/null || true)"
    for b in $BRIEFS; do
      candidate="$b"
      case "$candidate" in
        *.md) ;;
        *) candidate="${candidate}.md" ;;
      esac
      if [ -f "$candidate" ] || [ -f "$b" ]; then
        continue
      fi
      UNRESOLVED_BRIEF="$UNRESOLVED_BRIEF
$f references $b, not found on disk"
    done
  done

  if [ -z "$UNRESOLVED_BRIEF" ]; then
    ok "cross-reference: every referenced research brief exists on disk"
  else
    fail "cross-reference: unresolved research brief reference(s):$UNRESOLVED_BRIEF"
  fi
fi

# --- 5. phase-plan hygiene ---------------------------------------------------
PHASE_FILES=""
for f in docs/plans/phase-*.md; do
  if [ -f "$f" ]; then
    PHASE_FILES="$PHASE_FILES $f"
  fi
done

if [ -z "$PHASE_FILES" ]; then
  echo "SKIP: phase-plan-hygiene — no docs/plans/phase-*.md yet"
else
  HYGIENE_FAIL=""
  # shellcheck disable=SC2086
  for f in $PHASE_FILES; do
    if ! grep -q "Brief findings incorporated" "$f" 2>/dev/null; then
      HYGIENE_FAIL="$HYGIENE_FAIL
$f missing 'Brief findings incorporated' section"
    fi
    if ! grep -q "Acceptance criteria" "$f" 2>/dev/null; then
      HYGIENE_FAIL="$HYGIENE_FAIL
$f missing 'Acceptance criteria' section"
    fi
    base="$(basename "$f" .md)"
    num="$(printf '%s' "$base" | sed -n 's/^phase-\([0-9][0-9]*\).*/\1/p')"
    if [ -n "$num" ]; then
      smoke="scripts/smoke/phase-${num}.sh"
      if [ ! -f "$smoke" ]; then
        HYGIENE_FAIL="$HYGIENE_FAIL
$f has no matching $smoke"
      fi
    fi
  done

  if [ -z "$HYGIENE_FAIL" ]; then
    ok "phase-plan-hygiene: every phase plan has required sections and a matching smoke script"
  else
    fail "phase-plan-hygiene:$HYGIENE_FAIL"
  fi
fi

# --- 6. decisions log append-only-shaped ------------------------------------
if [ -f docs/decisions.md ]; then
  DUPES="$(grep -oE '^### D-[0-9]{3}' docs/decisions.md | sort | uniq -d || true)"
  if [ -z "$DUPES" ]; then
    ok "decisions-log: every D-NNN heading appears exactly once"
  else
    fail "decisions-log: duplicate D-NNN heading(s): $DUPES"
  fi
else
  echo "SKIP: decisions-log — docs/decisions.md not found"
fi

echo "== drift-audit summary: OK=$OK_COUNT FAIL=$FAIL_COUNT =="

if [ "$FAIL_COUNT" -gt 0 ]; then
  exit 1
fi

exit 0
