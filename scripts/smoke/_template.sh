#!/usr/bin/env bash
# Copy to phase-NN.sh and add the phase/criteria to the active registry.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAME="$(basename "${BASH_SOURCE[0]}" .sh)"
PHASE="${NAME#phase-}"
if [[ ! "$PHASE" =~ ^[0-9][0-9]$ ]]; then
  echo 'FAIL: copy this template to scripts/smoke/phase-NN.sh first' >&2
  exit 1
fi
exec python3 "$ROOT/scripts/run_phase_acceptance.py" --root "$ROOT" --phase "$PHASE" "$@"
