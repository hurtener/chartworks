#!/usr/bin/env bash
# A phase closes only when its real named acceptance tests pass.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAME="$(basename "${BASH_SOURCE[0]}" .sh)"
PHASE="${NAME#phase-}"
exec python3 "$ROOT/scripts/run_phase_acceptance.py" --root "$ROOT" --phase "$PHASE" "$@"
