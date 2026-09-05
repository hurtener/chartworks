#!/usr/bin/env bash
# Active planning coherence, not a runtime security proof. Historical documents
# do not override the current RFCs, owner directives or decision annexes.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec python3 "$ROOT/scripts/planning_check.py" --root "$ROOT"
