#!/usr/bin/env bash
set -euo pipefail

# One-time, per-clone: installs scripts/hooks/pre-commit into .git/hooks/.

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT"

if [ ! -e .git ]; then
  echo "SKIP: install-hooks — not a git repository (no .git) at $REPO_ROOT"
  exit 0
fi

GIT_DIR="$(git rev-parse --git-dir 2>/dev/null || echo ".git")"
mkdir -p "$GIT_DIR/hooks"

SRC="$REPO_ROOT/scripts/hooks/pre-commit"
DEST="$GIT_DIR/hooks/pre-commit"

if [ ! -f "$SRC" ]; then
  echo "FAIL: install-hooks — $SRC not found"
  exit 1
fi

cp "$SRC" "$DEST"
chmod +x "$DEST"

echo "OK: installed pre-commit hook -> $DEST"
