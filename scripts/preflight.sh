#!/usr/bin/env bash
# Conservative cumulative preflight. The old empty owning-path map could skip
# every phase; until a proven incremental selector exists, run the whole registry.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
make planning-check
make build
python3 scripts/run_phase_acceptance.py --all
printf '%s\n' 'PREFLIGHT: OK (planned SKIPs, if listed, are not runtime acceptance)'
