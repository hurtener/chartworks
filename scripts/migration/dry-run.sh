#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || ! -f "$1" || -L "$1" ]]; then
  echo "usage: scripts/migration/dry-run.sh REQUEST_JSON" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
client="$root/bin/chartworks"
if [[ ! -x "$client" ]]; then
  echo "chartworks binary missing; run make build" >&2
  exit 1
fi

exec "$client" client call migrationDryRun --execute --input - < "$1"
