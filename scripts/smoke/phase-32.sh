#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exec python3 "$ROOT/scripts/run_phase_acceptance.py" --root "$ROOT" --phase 32 "$@"
